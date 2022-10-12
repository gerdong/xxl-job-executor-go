package main

import (
	"fmt"
	xxl "github.com/gerdong/xxl-job-executor-go"
	"github.com/gerdong/xxl-job-executor-go/example/task"
	"log"
)

// Config
func initViaConfig() xxl.Executor {
	c := xxl.Conf{
		ServerAddr:   "http://127.0.0.1/xxl-job-admin",
		ExecutorPort: "9999",
		RegistryKey:  "golang-jobs",
		AccessToken:  "",
	}
	return xxl.New(c)
}

func main() {
	exec := initViaConfig()
	exec.SetLogger(&logger{})
	exec.Init()
	//设置日志查看handler
	exec.LogHandler(func(req *xxl.LogReq) *xxl.LogRes {
		return &xxl.LogRes{Code: xxl.SuccessCode, Msg: "", Content: xxl.LogResContent{
			FromLineNum: req.FromLineNum,
			ToLineNum:   2,
			LogContent:  "这个是自定义日志handler",
			IsEnd:       true,
		}}
	})
	//注册任务handler
	exec.RegTask("task.test", "", task.Test)
	exec.RegTask("task.test2", "", task.Test2)
	exec.RegTask("task.panic", "", task.Panic)
	log.Fatal(exec.Run())
}

// xxl.Logger接口实现
type logger struct{}

func (l *logger) Info(format string, a ...interface{}) {
	fmt.Println(fmt.Sprintf("自定义日志 - "+format, a...))
}

func (l *logger) Error(format string, a ...interface{}) {
	log.Println(fmt.Sprintf("自定义日志 - "+format, a...))
}
