package xxl

import (
	"context"
	"fmt"
	"runtime/debug"
)

// 任务执行函数
type TaskFunc func(cxt context.Context, param *RunReq) string

const (
	MESSAGE_SUCCESS = "ok"
)

// Task 任务
type Task struct {
	Id        int64
	Name      string
	Ext       context.Context
	Param     *RunReq
	fn        TaskFunc
	Cancel    context.CancelFunc
	StartTime int64
	EndTime   int64
	Desc      string
	log       Logger
}

// Run 运行任务
func (t *Task) Run(callback func(code int64, msg string)) {
	defer func(cancel func()) {
		if err := recover(); err != nil {
			t.log.Info(t.Info()+" panic: %v", err)
			debug.PrintStack() //堆栈跟踪
			callback(FailureCode, "task panic:"+fmt.Sprintf("%v", err))
			cancel()
		}
	}(t.Cancel)
	msg := t.fn(t.Ext, t.Param)

	code := SuccessCode
	if msg != MESSAGE_SUCCESS {
		code = FailureCode
	}
	callback(int64(code), msg)
	return
}

// Info 任务信息
func (t *Task) Info() string {
	return fmt.Sprintf("TaskId\t\t:%d\nLogId\t\t:%d\nExecutorHandler\t:%s\nDescription\t:%s\nBlockStrategy\t%s\nBroadcastIndex\t:%d\nBroadcastTotal\t:%d",
		t.Id, t.Param.LogID, t.Name, t.Desc, t.Param.ExecutorBlockStrategy, t.Param.BroadcastIndex, t.Param.BroadcastTotal)
}
