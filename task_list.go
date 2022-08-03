package xxl

import "sync"

//任务列表 [JobID]执行函数,并行执行时[+LogID]
type taskList struct {
	mu   sync.RWMutex
	data map[string]*Task
}

//设置数据
func (t *taskList) Set(key string, val *Task) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data[key] = val
}

//获取数据
func (t *taskList) Get(key string) *Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.data[key]
}

//获取数据
func (t *taskList) GetAll() map[string]*Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.data
}

//设置数据
func (t *taskList) Del(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.data, key)
}

//长度
func (t *taskList) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.data)
}

//Key是否存在
func (t *taskList) Exists(key string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.data[key]
	return ok
}
