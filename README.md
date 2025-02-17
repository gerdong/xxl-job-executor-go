forked from forked from https://github.com/ainiaa/xxl-job-executor-go
进行了一些调整：
- 执行器执行完成后，调用 callback 方法：如果 `xxl-job admin` 已经断开，则会导致本应用 panic
- 执行 returnCall 方法，向xxl-job admin 提交的数据，其结构与 xxl-job 所要求的不符（可能是xxl-job已经更新的缘故）
- 心跳 log 太啰嗦，简化
- server.ListenAndServe 没有处理异常，导致明明启动失败，确看起来一切正常
- 给Task增加Description

# xxl-job-executor-go
很多公司java与go开发共存，java中有xxl-job做为任务调度引擎，为此也出现了go执行器(客户端)，使用起来比较简单：
# 支持
```	
1.执行器注册
2.耗时任务取消
3.任务注册，像写http.Handler一样方便
4.任务panic处理
5.阻塞策略处理
6.任务完成支持返回执行备注
7.任务超时取消 (单位：秒，0为不限制)
8.失败重试次数(在参数param中，目前由任务自行处理)
9.可自定义日志
10.自定义日志查看handler
11.支持外部路由（可与gin集成）
```


# Example
```
import (
	"fmt"
	xxl "github.com/ainiaa/xxl-job-executor-go"
	"github.com/ainiaa/xxl-job-executor-go/example/task"
	"log"
)

// Option style
func initViaOption() xxl.Executor{
	return xxl.NewExecutor(
		xxl.ServerAddr("http://127.0.0.1/xxl-job-admin"),
		xxl.AccessToken(""),            //请求令牌(默认为空)
		xxl.ExecutorIp("127.0.0.1"),    //可自动获取
		xxl.ExecutorPort("9999"),       //默认9999（非必填）
		xxl.RegistryKey("golang-jobs"), //执行器名称
		xxl.SetLogger(&logger{}),       //自定义日志
	)
}
// Config style
func initViaConfig() xxl.Executor{
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
		return &xxl.LogRes{Code: 200, Msg: "", Content: xxl.LogResContent{
			FromLineNum: req.FromLineNum,
			ToLineNum:   2,
			LogContent:  "这个是自定义日志handler",
			IsEnd:       true,
		}}
	})
	//注册任务handler
	exec.RegTask("task.test", task.Test)
	exec.RegTask("task.test2", task.Test2)
	exec.RegTask("task.panic", task.Panic)
	log.Fatal(exec.Run())
}

//xxl.Logger接口实现
type logger struct{}

func (l *logger) Fatalf(format string, a ...interface{}) {
	panic("implement me")
}

func (l *logger) Infof(format string, a ...interface{}) {
	fmt.Println(fmt.Sprintf("自定义日志 - "+format, a...))
}

func (l *logger) Errorf(format string, a ...interface{}) {
	log.Println(fmt.Sprintf("自定义日志 - "+format, a...))
}

```

# 示例项目
github.com/ainiaa/xxl-job-executor-go/example/
# 与gin框架集成
https://github.com/ainiaa/gin-xxl-job-executor
# xxl-job-admin配置
### 添加执行器
执行器管理->新增执行器,执行器列表如下：
```
AppName		名称		注册方式	OnLine 		机器地址 		操作
golang-jobs	golang执行器	自动注册 		查看 ( 1 ）   
```
查看->注册节点
```
http://127.0.0.1:9999
```
### 添加任务
任务管理->新增(注意，使用BEAN模式，JobHandler与RegTask名称一致)
```
1	测试panic	BEAN：task.panic	* 0 * * * ?	admin	STOP	
2	测试耗时任务	BEAN：task.test2	* * * * * ?	admin	STOP	
3	测试golang	BEAN：task.test		* * * * * ?	admin	STOP
```

