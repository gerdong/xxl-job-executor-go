package xxl

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/robfig/cron/v3"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Executor 执行器
type Executor interface {
	// Init 初始化
	Init(...Option)
	// LogHandler 日志查询
	LogHandler(handler LogHandler)
	// RegTask 注册任务
	RegTask(pattern string, desc string, task TaskFunc)
	// RunTask 运行任务
	RunTask(writer http.ResponseWriter, request *http.Request)
	// KillTask 杀死任务
	KillTask(writer http.ResponseWriter, request *http.Request)
	// TaskLog 任务日志
	TaskLog(writer http.ResponseWriter, request *http.Request)
	// Beat 心跳检测
	Beat(writer http.ResponseWriter, request *http.Request)
	// IdleBeat 忙碌检测
	IdleBeat(writer http.ResponseWriter, request *http.Request)
	// Run 运行服务
	Run() error
	// Stop 停止服务
	Stop()
	// SetLogger logger
	SetLogger(log Logger)
	// RegistryRemove 摘除 xxljob
	RegistryRemove()
	// GetAllTasks 显示所有已经注册的任务
	GetAllTasks() map[string]*Task
	// ExpireLog 自动清理日志
	ExpireLog(days int, dir string)
}

// 创建执行器
func New(c Conf) Executor {
	opts := newOptionsFromConf(c)
	executor := &executor{
		opts: opts,
	}
	return executor
}

// NewExecutor 创建执行器
func NewExecutor(opts ...Option) Executor {
	return newExecutor(opts...)
}

func newExecutor(opts ...Option) *executor {
	options := newOptions(opts...)
	executor := &executor{
		opts: options,
	}
	return executor
}

type executor struct {
	opts    Options
	address string
	regList *taskList //注册任务列表
	runList *taskList //正在执行任务列表
	mu      sync.RWMutex
	log     Logger

	logHandler LogHandler //日志查询handler
}

func (e *executor) Init(opts ...Option) {
	for _, o := range opts {
		o(&e.opts)
	}
	e.log = e.opts.l
	e.regList = &taskList{
		data: make(map[string]*Task),
	}
	e.runList = &taskList{
		data: make(map[string]*Task),
	}
	e.address = e.opts.ExecutorIp + ":" + e.opts.ExecutorPort
	go e.registry()
}

// LogHandler 日志handler
func (e *executor) LogHandler(handler LogHandler) {
	e.logHandler = handler
}

func (e *executor) Run() (err error) {
	// 创建路由器
	mux := http.NewServeMux()
	// 设置路由规则
	mux.HandleFunc("/run", e.runTask)
	mux.HandleFunc("/kill", e.killTask)
	mux.HandleFunc("/log", e.taskLog)
	mux.HandleFunc("/beat", e.beat)
	mux.HandleFunc("/idleBeat", e.idleBeat)
	// 创建服务器
	server := &http.Server{
		Addr:         e.address,
		WriteTimeout: time.Second * 3,
		Handler:      mux,
	}
	// 监听端口并提供服务
	e.log.Info("Starting server at " + e.address)
	//go server.ListenAndServe()
	go func() {
		err = server.ListenAndServe()
		if err != nil {
			e.log.Error("Failed: %v", err)
			return
		}
	}()

	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGKILL, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	e.RegistryRemove()
	return nil
}

func (e *executor) Stop() {
	e.RegistryRemove()
}

// RegTask 注册任务
func (e *executor) RegTask(pattern string, desc string, task TaskFunc) {
	var t = &Task{}
	t.fn = task
	t.Desc = desc
	e.regList.Set(pattern, t)
}

// 运行一个任务
func (e *executor) runTask(writer http.ResponseWriter, request *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	req, _ := io.ReadAll(request.Body)
	param := &RunReq{}
	err := json.Unmarshal(req, &param)
	if err != nil {
		_, _ = writer.Write(returnCall(param, FailureCode, "params err"))
		e.log.Error("参数解析错误:%s error:%s", string(req), err.Error())
		return
	}

	if !e.regList.Exists(param.ExecutorHandler) {
		_, _ = writer.Write(returnCall(param, FailureCode, "Task not registered: "+param.ExecutorHandler))
		e.log.Error("任务 [%d] (%s) 还没有被注册", param.JobID, param.ExecutorHandler)
		return
	}

	//阻塞策略处理
	if e.runList.Exists(Int64ToStr(param.JobID)) {
		if param.ExecutorBlockStrategy == coverEarly { //覆盖之前调度
			oldTask := e.runList.Get(Int64ToStr(param.JobID))
			if oldTask != nil {
				oldTask.Cancel()
				e.runList.Del(Int64ToStr(oldTask.Id))
			}
		} else { //单机串行,丢弃后续调度 都进行阻塞
			_, _ = writer.Write(returnCall(param, FailureCode, "There are tasks running"))
			e.log.Error("任务[%s](%x) 已经在运行中", param.JobID, param.ExecutorHandler)
			return
		}
	}

	cxt := context.Background()
	task := e.regList.Get(param.ExecutorHandler)
	if param.ExecutorTimeout > 0 {
		task.Ext, task.Cancel = context.WithTimeout(cxt, time.Duration(param.ExecutorTimeout)*time.Second)
	} else {
		task.Ext, task.Cancel = context.WithCancel(cxt)
	}
	task.Id = param.JobID
	task.Name = param.ExecutorHandler
	task.Param = param
	task.log = e.log

	e.log.Info("Task info: \n%s", task.Info())

	e.runList.Set(Int64ToStr(task.Id), task)
	go task.Run(func(code int64, msg string) {
		e.callback(task, code, msg)
	})
	e.log.Info("任务 [%d] (%s) 开始执行:", param.JobID, param.ExecutorHandler)
	_, _ = writer.Write(returnGeneral())
}

// 删除一个任务
func (e *executor) killTask(writer http.ResponseWriter, request *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	req, _ := io.ReadAll(request.Body)
	param := &killReq{}
	_ = json.Unmarshal(req, &param)
	if !e.runList.Exists(Int64ToStr(param.JobID)) {
		_, _ = writer.Write(returnKill(param, FailureCode))
		e.log.Error("任务 [%d] 没有运行", param.JobID)
		return
	}
	task := e.runList.Get(Int64ToStr(param.JobID))
	task.Cancel()
	e.runList.Del(Int64ToStr(param.JobID))
	_, _ = writer.Write(returnGeneral())
}

// 任务日志
func (e *executor) taskLog(writer http.ResponseWriter, request *http.Request) {
	var res *LogRes
	data, err := io.ReadAll(request.Body)
	req := &LogReq{}
	if err != nil {
		e.log.Error("日志请求失败:%s", err.Error())
		reqErrLogHandler(writer, req, err)
		return
	}
	err = json.Unmarshal(data, &req)
	if err != nil {
		e.log.Error("日志请求解析失败:%s", err.Error())
		reqErrLogHandler(writer, req, err)
		return
	}
	e.log.Info("日志请求参数:%+v", req)
	if e.logHandler != nil {
		res = e.logHandler(req)
	} else {
		res = defaultLogHandler(req)
	}
	str, _ := json.Marshal(res)
	_, _ = writer.Write(str)
}

// 心跳检测
func (e *executor) beat(writer http.ResponseWriter, _ *http.Request) {
	e.log.Info("心跳检测")
	_, _ = writer.Write(returnGeneral())
}

// 忙碌检测
func (e *executor) idleBeat(writer http.ResponseWriter, request *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	req, _ := io.ReadAll(request.Body)
	param := &idleBeatReq{}
	err := json.Unmarshal(req, &param)
	if err != nil {
		_, _ = writer.Write(returnIdleBeat(FailureCode))
		e.log.Error("参数解析错误:" + string(req))
		return
	}
	if e.runList.Exists(Int64ToStr(param.JobID)) {
		_, _ = writer.Write(returnIdleBeat(FailureCode))
		e.log.Error("idleBeat任务[" + Int64ToStr(param.JobID) + "]正在运行")
		return
	}
	e.log.Info("忙碌检测任务参数:%v", param)
	_, _ = writer.Write(returnGeneral())
}

var (
	lastRegistStatus = -1 // 最后一次regist心跳的状态。 避免重复打印
)

func (e *executor) setlastRegistStatus(status int, msg string, iserr bool) {
	if status == lastRegistStatus {
		return
	}

	lastRegistStatus = status
	if iserr {
		e.log.Info(msg)
	} else {
		e.log.Error(msg)
	}
}

// 注册执行器到调度中心
func (e *executor) registry() {

	t := time.NewTimer(time.Second * 0) //初始立即执行
	defer t.Stop()
	req := &Registry{
		RegistryGroup: "EXECUTOR",
		RegistryKey:   e.opts.RegistryKey,
		RegistryValue: "http://" + e.address,
	}
	param, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("执行器注册信息解析失败:%s:", err.Error())
	}
	for {
		<-t.C
		t.Reset(time.Second * time.Duration(20)) //20秒心跳防止过期
		func() {
			result, err := e.post("/api/registry", string(param))
			if err != nil {
				e.setlastRegistStatus(1, fmt.Sprintf("执行器注册失败1 :%s", err.Error()), true)
				//e.log.Error("request /api/registry post failure error:%s", err.Error())
				return
			}
			defer func() { _ = result.Body.Close() }()
			body, err := io.ReadAll(result.Body)
			if err != nil {
				e.setlastRegistStatus(2, fmt.Sprintf("执行器注册失败2: %s:", err.Error()), true)
				//e.log.Error("request /api/registry read body failure error:%s:",  err.Error())
				return
			}
			res := &res{}
			err = json.Unmarshal(body, &res)
			if err != nil {
				e.setlastRegistStatus(3, fmt.Sprintf("执行器注册失败3: %s:", err.Error()), true)
				//e.log.Error("request /api/registry json unmarshal failure error:%s:",  err.Error())
				return
			}
			if res.Code != SuccessCode {
				e.setlastRegistStatus(4, fmt.Sprintf("执行器注册失败 4: %+v:", res), true)
				//e.log.Error("request /api/registry response failure response:%+v:",  res)
				return
			}
			e.setlastRegistStatus(0, fmt.Sprintf("执行器注册成功: %+v", res), true)
			//e.log.Info("request /api/registry success response:%+v",res)
		}()
	}
}

// 执行器注册摘除
func (e *executor) RegistryRemove() {
	t := time.NewTimer(time.Second * 0) //初始立即执行
	defer t.Stop()
	req := &Registry{
		RegistryGroup: "EXECUTOR",
		RegistryKey:   e.opts.RegistryKey,
		RegistryValue: "http://" + e.address,
	}
	param, err := json.Marshal(req)
	if err != nil {
		e.log.Error("执行器摘除失败: %s", err.Error())
		return
	}

	res, err := e.post("/api/registryRemove", string(param))
	if err != nil {
		e.log.Error("执行器摘除失败: %s", err.Error())
		return
	}

	if res == nil {
		e.log.Error("执行器摘除失败: response is nil.")
		return
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		e.log.Error("执行器摘除失败: %s", err.Error())
		return
	}

	e.log.Info("执行器摘除成功: %s", string(body))
	_ = res.Body.Close()
}

// 返回所有任务
func (e *executor) GetAllTasks() map[string]*Task {
	return e.regList.data
}

// 回调任务列表
func (e *executor) callback(task *Task, code int64, msg string) {
	defer func() {
		e.runList.Del(Int64ToStr(task.Id))
	}()

	res, err := e.post("/api/callback", string(returnCall(task.Param, code, msg)))
	if err != nil {
		e.log.Error("callback post err : ", err.Error())
		return
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		e.log.Error("callback ReadAll err:%s ", err.Error())
		return
	}

	e.log.Info("任务 [%d] 回调成功: %s", task.Id, string(body))
}

// post
func (e *executor) post(action, body string) (resp *http.Response, err error) {
	request, err := http.NewRequest("POST", e.opts.ServerAddr+action, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	request.Header.Set("XXL-JOB-ACCESS-TOKEN", e.opts.AccessToken)
	client := http.Client{
		Timeout: e.opts.Timeout,
	}
	return client.Do(request)
}

// RunTask 运行任务
func (e *executor) RunTask(writer http.ResponseWriter, request *http.Request) {
	e.runTask(writer, request)
}

// KillTask 删除任务
func (e *executor) KillTask(writer http.ResponseWriter, request *http.Request) {
	e.killTask(writer, request)
}

// TaskLog 任务日志
func (e *executor) TaskLog(writer http.ResponseWriter, request *http.Request) {
	e.taskLog(writer, request)
}

// SetLogger
func (e *executor) SetLogger(log Logger) {
	e.log = log
	e.opts.l = log
}

// Beat 心跳检测
func (e *executor) Beat(writer http.ResponseWriter, request *http.Request) {
	e.beat(writer, request)
}

// IdleBeat 忙碌检测
func (e *executor) IdleBeat(writer http.ResponseWriter, request *http.Request) {
	e.idleBeat(writer, request)
}

func (e *executor) ExpireLog(days int, dir string) {
	if days <= 0 || dir == "" || !exists(dir) {
		e.log.Info("Warn: ExpireLog(%d, %s): 'days' must be greater than 0, and 'dir' must exist", days, dir)
		return
	}

	c := cron.New()
	_, err := c.AddFunc("* 0/2 * * *", func() {
		sub := findExpireDir(days, dir)
		for _, s := range sub {
			removeDir(filepath.Join(dir, s))
		}
	})

	if err != nil {
		e.log.Error("Error: ExpireLog(), " + err.Error())
		return
	}
	c.Start()
}
