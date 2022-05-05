# Queue

## Interface

```go
type Interface interface {
	Add(item interface{}) //增加对象
	Len() int //队列长度
	Get() (item interface{}, shutdown bool) //获取对象
	Done(item interface{}) //结束对象
	ShutDown() //停止对象
	ShutDownWithDrain()
	ShuttingDown() bool
}
```

```go
// New constructs a new work queue (see the package comment).
func New() *Type {
	return NewNamed("")
}
//新建Named
func NewNamed(name string) *Type {
	rc := clock.RealClock{}
	return newQueue(
		rc,
		globalMetricsFactory.newQueueMetrics(name, rc), //metric信息
		defaultUnfinishedWorkUpdatePeriod,
	)
}

func newQueue(c clock.WithTicker, metrics queueMetrics, updatePeriod time.Duration) *Type {
	t := &Type{
		clock:                      c,
		dirty:                      set{},
		processing:                 set{},
		cond:                       sync.NewCond(&sync.Mutex{}),
		metrics:                    metrics,
		unfinishedWorkUpdatePeriod: updatePeriod,
	}

	// Don't start the goroutine for a type of noMetrics so we don't consume
	// resources unnecessarily
	if _, ok := metrics.(noMetrics); !ok {
		go t.updateUnfinishedWorkLoop()
	}

	return t
}
```


## Type

```go
// Type  是一个工作队列
type Type struct {
	// queue defines the order in which we will work on items. Every
	// element of queue should be in the dirty set and not in the
	// processing set.
	//每个待工作元素应该在`dirty`中，但是不应该在processing中，queue也指定了执行顺序
	queue []t

	// dirty 定义所有待处理的对象
	dirty set

	// Things that are currently being processed are in the processing set.
	// These things may be simultaneously in the dirty set. When we finish
	// processing something and remove it from this set, we'll check if
	// it's in the dirty set, and if so, add it to the queue.
	// 正在被处理的对象，这些对象有可能也在dirty中，当我们处理完成对象后，会检查dirty中是否有该对象，如果有该对象则代表对象有更新，我们会将对象增加到queue中
	processing set

	cond *sync.Cond

	shuttingDown bool
	drain        bool

	metrics queueMetrics

	unfinishedWorkUpdatePeriod time.Duration
	clock                      clock.WithTicker
}
```


```go
type empty struct{}
type t interface{}
//set是map的封装
type set map[t]empty

func (s set) has(item t) bool {
	_, exists := s[item]
	return exists
}

func (s set) insert(item t) {
	s[item] = empty{}
}

func (s set) delete(item t) {
	delete(s, item)
}

func (s set) len() int {
	return len(s)
}
```

## Add

```go
// Add 标示对象需要被处理
func (q *Type) Add(item interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	//如果队列正在被关闭，则什么都不做
	if q.shuttingDown {
		return
	}
	//如果dirty中有对象，则什么都不做
	if q.dirty.has(item) {
		return
	}

	q.metrics.add(item)

    //将对象设置为dirty状态
	q.dirty.insert(item)
	//如果processing中有对象，则不需要做什么事情了
	if q.processing.has(item) {
		return
	}

    //增加对象到队列
	q.queue = append(q.queue, item)
	//通知一下目前已经有对象更新了
	q.cond.Signal()
}

```

```go
//返回队列长度
func (q *Type) Len() int {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return len(q.queue)
}

```


```go
// Get blocks until it can return an item to be processed. If shutdown = true,
// the caller should end their goroutine. You must call Done with item when you
// have finished processing it.
func (q *Type) Get() (item interface{}, shutdown bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	//如果队列长度为空，并且没有被关闭，则等待
	for len(q.queue) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}
	//如果队列为空，则返回空对象，并且标示已经被关闭
	if len(q.queue) == 0 {
		// We must be shutting down.
		return nil, true
	}

    //拿第一个对象
	item = q.queue[0]
	// The underlying array still exists and reference this object, so the object will not be garbage collected.
	//释放对象
	q.queue[0] = nil
	//更新队列
	q.queue = q.queue[1:]

	q.metrics.get(item)

    //增加到正在处理列表中
	q.processing.insert(item)
	//从dirty中删除对象
	q.dirty.delete(item)

	//最终的结果是： processing：有item， dirty中没有对象， queue没有对象

    //返回对象
	return item, false
}
```

```go
// Done marks item as done processing, and if it has been marked as dirty again
// while it was being processed, it will be re-added to the queue for
// re-processing.
func (q *Type) Done(item interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.metrics.done(item)

    //删除对象
	q.processing.delete(item)
	//如果脏队列中有这个对象，则需要重新处理下这个对象
	if q.dirty.has(item) {
		q.queue = append(q.queue, item)
		//通知下
		q.cond.Signal()
	} else if q.processing.len() == 0 { //当队列中没有元素时，需要通知waitForProcessing 可以结束。
		q.cond.Signal()
	}
}
```

```go
func (q *Type) ShutDown() {
	q.setDrain(false)
	//通知所有work，处理结束了
	q.shutdown()
}

// 忽略所有新的请求，等待所有对象被执行结束
func (q *Type) ShutDownWithDrain() {
	q.setDrain(true)
	//通知所有worker可以停止了
	q.shutdown()
	//等待所有对象处理结束
	for q.isProcessing() && q.shouldDrain() {
		q.waitForProcessing()
	}
}

// isProcessing indicates if there are still items on the work queue being
// processed. It's used to drain the work queue on an eventual shutdown.
func (q *Type) isProcessing() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return q.processing.len() != 0
}

//等待处理结束，依赖于Done函数当队列为空时的通知
func (q *Type) waitForProcessing() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	// Ensure that we do not wait on a queue which is already empty, as that
	// could result in waiting for Done to be called on items in an empty queue
	// which has already been shut down, which will result in waiting
	// indefinitely.
	if q.processing.len() == 0 {
		return
	}
	q.cond.Wait()
}

func (q *Type) setDrain(shouldDrain bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	q.drain = shouldDrain
}

func (q *Type) shouldDrain() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return q.drain
}

//结束队列
func (q *Type) shutdown() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	q.shuttingDown = true
	q.cond.Broadcast()
}

func (q *Type) ShuttingDown() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	return q.shuttingDown
}

```


## 测试

```go
func TestBasic(t *testing.T) {
    //table testing
	tests := []struct {
		queue         *workqueue.Type
		queueShutDown func(workqueue.Interface)
	}{
		{
			queue:         workqueue.New(), //新建对象
			queueShutDown: workqueue.Interface.ShutDown, //获取函数地址
		},
		{
			queue:         workqueue.New(), //新建对象
			queueShutDown: workqueue.Interface.ShutDownWithDrain,//获取函数地址
		},
	}
	for _, test := range tests {
		// If something is seriously wrong this test will never complete.

		// Start producers
		const producers = 50
		producerWG := sync.WaitGroup{}
		producerWG.Add(producers)
		for i := 0; i < producers; i++ {
			go func(i int) {
				defer producerWG.Done()
				for j := 0; j < 50; j++ {
				    //增加对象，如果已经在dirty队列中，则直接忽略，所以多个producer会有非常多重复的
					test.queue.Add(i)
					time.Sleep(time.Millisecond)
				}
			}(i)
		}

		// Start consumers
		const consumers = 10
		consumerWG := sync.WaitGroup{}
		consumerWG.Add(consumers)
		for i := 0; i < consumers; i++ {
			go func(i int) {
				defer consumerWG.Done()
				for {
				    //获取对象，如果此时有对象更新（相同对象重新入队），则将对象重新加入到queue中
					item, quit := test.queue.Get()
					if item == "added after shutdown!" {
						t.Errorf("Got an item added after shutdown.")
					}
					if quit {
						return
					}
					t.Logf("Worker %v: begin processing %v", i, item)
					time.Sleep(3 * time.Millisecond)
					t.Logf("Worker %v: done processing %v", i, item)
					test.queue.Done(item)
				}
			}(i)
		}

		producerWG.Wait()
		test.queueShutDown(test.queue) //使用方法（作用对象）的方式调用对象
		test.queue.Add("added after shutdown!")
		consumerWG.Wait()
		if test.queue.Len() != 0 {
			t.Errorf("Expected the queue to be empty, had: %v items", test.queue.Len())
		}
	}
}
```
