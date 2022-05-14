# Delaying Queue

## Interface

```go
// DelayingInterface 支持将一个失败的对象重新加入到队列中重新处理
type DelayingInterface interface {
	Interface
	// AddAfter adds an item to the workqueue after the indicated duration has passed
	AddAfter(item interface{}, duration time.Duration)
}
```

## 新建

```go
// NewDelayingQueue constructs a new workqueue with delayed queuing ability.
// NewDelayingQueue does not emit metrics. For use with a MetricsProvider, please use
// NewNamedDelayingQueue instead.
func NewDelayingQueue() DelayingInterface {
	return NewDelayingQueueWithCustomClock(clock.RealClock{}, "")
}

// NewDelayingQueueWithCustomQueue constructs a new workqueue with ability to
// inject custom queue Interface instead of the default one
func NewDelayingQueueWithCustomQueue(q Interface, name string) DelayingInterface {
	return newDelayingQueue(clock.RealClock{}, q, name)
}

// NewNamedDelayingQueue constructs a new named workqueue with delayed queuing ability
func NewNamedDelayingQueue(name string) DelayingInterface {
	return NewDelayingQueueWithCustomClock(clock.RealClock{}, name)
}

// NewDelayingQueueWithCustomClock constructs a new named workqueue
// with ability to inject real or fake clock for testing purposes
func NewDelayingQueueWithCustomClock(clock clock.WithTicker, name string) DelayingInterface {
	return newDelayingQueue(clock, NewNamed(name), name)
}
```


## delayingType 延迟队列实现

```go
// delayingType wraps an Interface and provides delayed re-enquing
type delayingType struct {
	Interface //具体底层队列

	// clock tracks time for delayed firing
	clock clock.Clock //具体时钟

	// stopCh lets us signal a shutdown to the waiting loop
	stopCh chan struct{} // 停止chan
	// stopOnce guarantees we only signal shutdown a single time
	stopOnce sync.Once //确保只会被关闭一次，否则多次关闭chan会导致panic

	// heartbeat ensures we wait no more than maxWait before firing
	heartbeat clock.Ticker //定时唤醒

	// waitingForAddCh is a buffered channel that feeds waitingForAdd
	waitingForAddCh chan *waitFor //等待加入到队列的元素

	// metrics counts the number of retries
	metrics retryMetrics
}
```

```go
// waitFor holds the data to add and the time it should be added
type waitFor struct {
	data    t //具体的数据
	readyAt time.Time //何时处理
	// index in the priority queue (heap)
	index int //在优先队列中的位置
}
```



## waitForPriorityQueue

构造一个小顶堆的优先队列，使用golang的源码包： `src/container/heap`

heap对于Interface的定义如下所示：

```go

type heap.Interface interface {
	sort.Interface
	Push(x any) // add x as element Len()
	Pop() any   // remove and return element Len() - 1.
}

type sort.Interface interface {
	// Len is the number of elements in the collection.
	Len() int

	Less(i, j int) bool
	// Swap swaps the elements with indexes i and j.
	Swap(i, j int)
}

```
定义结构满足接口

```go
type waitForPriorityQueue []*waitFor

//实现sort.Interface
func (pq waitForPriorityQueue) Len() int {
	return len(pq)
}
//实现sort.Interface
func (pq waitForPriorityQueue) Less(i, j int) bool {
	return pq[i].readyAt.Before(pq[j].readyAt)
}
//实现sort.Interface
func (pq waitForPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

// 实现heap.Interface
func (pq *waitForPriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*waitFor)
	item.index = n
	*pq = append(*pq, item)
}

// 实现heap.Interface
func (pq *waitForPriorityQueue) Pop() interface{} {
	n := len(*pq)
	item := (*pq)[n-1]
	item.index = -1
	*pq = (*pq)[0:(n - 1)]
	return item
}

// 查看小顶堆的堆顶元素
func (pq waitForPriorityQueue) Peek() interface{} {
	return pq[0]
}

```

## 关闭队列

```go
// 关闭队列
func (q *delayingType) ShutDown() {
    //可重入
	q.stopOnce.Do(func() {
	    //关闭底层队列
		q.Interface.ShutDown()
		//发送关闭信号
		close(q.stopCh)
		//停止心跳
		q.heartbeat.Stop()
	})
}
```

## 增加元素

```go
// 过指定时间后，将对象加入到队列
func (q *delayingType) AddAfter(item interface{}, duration time.Duration) {
	// 如果队列已经结束，则结束
	if q.ShuttingDown() {
		return
	}

	q.metrics.retry()

	// immediately add things with no delay
	// 如果需要立刻加入，则直接加入到底层队列
	if duration <= 0 {
		q.Add(item)
		return
	}

    //加入等待队列，或者等待结束
	select {
	case <-q.stopCh:
		// unblock if ShutDown() is called
	case q.waitingForAddCh <- &waitFor{data: item, readyAt: q.clock.Now().Add(duration)}:
	}
}
```

## waitingLoop

```go
// waitingLoop runs until the workqueue is shutdown and keeps a check on the list of items to be added.
func (q *delayingType) waitingLoop() {
    //处理异常
	defer utilruntime.HandleCrash()

	// Make a placeholder channel to use when there are no items in our list
	//当没有元素时使用
	never := make(<-chan time.Time)

	// Make a timer that expires when the item at the head of the waiting queue is ready
	//优先队列中最小等待时间
	var nextReadyAtTimer clock.Timer

    //优先队列，所有待加入对象都在这里
	waitingForQueue := &waitForPriorityQueue{}
	heap.Init(waitingForQueue)
	//多次加入同一个对象，需要去重就好
	waitingEntryByData := map[t]*waitFor{}

	for {
	    //如果底层队列已经关闭则直接退出
		if q.Interface.ShuttingDown() {
			return
		}
		//获取当前时间
		now := q.clock.Now()

		// Add ready entries
		//如果队列中有元素
		for waitingForQueue.Len() > 0 {
		    //从优先队列中找到最早需要处理的对象
			entry := waitingForQueue.Peek().(*waitFor)
			//如果还不需要处理，则进入等待队列
			if entry.readyAt.After(now) {
				break
			}
			//从优先队列中获取元素
			entry = heap.Pop(waitingForQueue).(*waitFor)
			//增加到底层队列
			q.Add(entry.data)
			//删除数据记录
			delete(waitingEntryByData, entry.data)
		}

		// Set up a wait for the first item's readyAt (if one exists)
		nextReadyAt := never
		//如果等待队中有元素，则定时唤醒loop
		if waitingForQueue.Len() > 0 {
			if nextReadyAtTimer != nil {
				nextReadyAtTimer.Stop()
			}
			entry := waitingForQueue.Peek().(*waitFor)
			nextReadyAtTimer = q.clock.NewTimer(entry.readyAt.Sub(now))
			nextReadyAt = nextReadyAtTimer.C()
		}

		select {
		//结束了
		case <-q.stopCh:
			return
		//心跳
		case <-q.heartbeat.C():
			// continue the loop, which will add ready items
		//下一个对象ready
		case <-nextReadyAt:
			// continue the loop, which will add ready items
		//又有新的元素加入到队列，那么就处理新元素，尝试将它加入到优先队列，或者直接加入底层的队列
		case waitEntry := <-q.waitingForAddCh:
		    //如果需要延时加入
			if waitEntry.readyAt.After(q.clock.Now()) {
				insert(waitingForQueue, waitingEntryByData, waitEntry)
			} else {
			    //不需要延时，直接加入
				q.Add(waitEntry.data)
			}

			drained := false
			//获取chan中的所有元素，一次性处理完
			for !drained {
				select {
				case waitEntry := <-q.waitingForAddCh:
					if waitEntry.readyAt.After(q.clock.Now()) {
						insert(waitingForQueue, waitingEntryByData, waitEntry)
					} else {
						q.Add(waitEntry.data)
					}
				default:
					drained = true
				}
			}
		}
	}
}

```

```go

// 将对象加入到优先队列
func insert(q *waitForPriorityQueue, knownEntries map[t]*waitFor, entry *waitFor) {
	// 如果优先队列已经有这个元素了
	existing, exists := knownEntries[entry.data]
	if exists {
	    //并且新加入的元素更早，则需要更新下时间，并更新下优先队列
		if existing.readyAt.After(entry.readyAt) {
			existing.readyAt = entry.readyAt
			heap.Fix(q, existing.index)
		}
		return
	}

    //不存在，则将对象加入到优先队列，等待加入
	heap.Push(q, entry)
	//标记优先队列已经有这个对象了
	knownEntries[entry.data] = entry
}
```


## 测试

```go
func TestSimpleQueue(t *testing.T) {
    //初始化FackClock
	fakeClock := testingclock.NewFakeClock(time.Now())
	q := NewDelayingQueueWithCustomClock(fakeClock, "")

	first := "foo"
    //增加一个对象
	q.AddAfter(first, 50*time.Millisecond) //50秒后增加item
	//等待结束
	if err := waitForWaitingQueueToFill(q); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	//由于现在时间未到，会被消费到优先队列中
	if q.Len() != 0 {
		t.Errorf("should not have added")
	}

    //向前推进60ms，此时会触发`nextReadyAt`timer，进而将对象加入到底层队列中
	fakeClock.Step(60 * time.Millisecond)

	if err := waitForAdded(q, 1); err != nil {
		t.Errorf("should have added")
	}
	//获取对象
	item, _ := q.Get()
	q.Done(item)

    //向前推进10s，导致heartbeat timer触发
	// step past the next heartbeat
	fakeClock.Step(10 * time.Second)

    //获取一下队列
	err := wait.Poll(1*time.Millisecond, 30*time.Millisecond, func() (done bool, err error) {
		if q.Len() > 0 {
			return false, fmt.Errorf("added to queue")
		}

		return false, nil
	})
	//应该等待超时
	if err != wait.ErrWaitTimeout {
		t.Errorf("expected timeout, got: %v", err)
	}

    //队列长队应该为空
	if q.Len() != 0 {
		t.Errorf("should not have added")
	}
}
```





