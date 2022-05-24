# Event-Recorder

记录对象发生的各类事件。

[blog](https://www.cncf.io/blog/2021/12/21/extracting-value-from-the-kubernetes-events-feed/)

## 架构图

![架构图](/tools/event-record/images/archtect.png])

## 使用方式

[record.go](/tools/event-record/record.go)


- 创建rest-client
- 基于rest-client，建立一个`EventBroadCaster`
  - `events.NewEventBroadcasterAdapter(client)`
- 开启EventBroadcaster消息处理循环；
  - `go eventBroadcast.StartRecordingToSink(closeCh)`
- 基于 `EventBroadCaster`创建一个`Record`
  - `eventBroadcast.NewRecorder("test")`
- 基于`Record`开始记录事件
`- `r.Eventf(pod, pod, v1.EventTypeNormal, "test event record", "add", "")`
- 关闭Broadcaster
  - `eventBroadcast.Shutdown()`

## tools/record 包代码分解

### EventBroadcaster 
EventBroadcaster 是event处理的中间组件，可以介于一个`EventSinker`开始一个EventBroadcaster。基于同一个`EventBroadcaster`创建的`Recorder`可以在任何时间记录事件。Broadcaster负责将事件消费并存储到`Sinker`中。

EventBroadcaster的接口如下所示：

```go
// EventBroadcaster knows how to receive events and send them to any EventSink, watcher, or log.
type EventBroadcaster interface {
	// StartEventWatcher starts sending events received from this EventBroadcaster to the given
	// event handler function. The return value can be ignored or used to stop recording, if
	// desired.
	//添加一个事件监听器
	StartEventWatcher(eventHandler func(*v1.Event)) watch.Interface

	// StartRecordingToSink starts sending events received from this EventBroadcaster to the given
	// sink. The return value can be ignored or used to stop recording, if desired.
	// 开始将数据写入到sinker中
	StartRecordingToSink(sink EventSink) watch.Interface

	// StartLogging starts sending events received from this EventBroadcaster to the given logging
	// function. The return value can be ignored or used to stop recording, if desired.
	//将事件写入到日志中
	StartLogging(logf func(format string, args ...interface{})) watch.Interface

	// StartStructuredLogging starts sending events received from this EventBroadcaster to the structured
	// logging function. The return value can be ignored or used to stop recording, if desired.
	// 按照structed形式记录事件
	StartStructuredLogging(verbosity klog.Level) watch.Interface

	// NewRecorder returns an EventRecorder that can be used to send events to this EventBroadcaster
	// with the event source set to the given event source.
	// 新建一个Recorder，后续基于此Recorder记录的事件都会写入到存储中
	NewRecorder(scheme *runtime.Scheme, source v1.EventSource) EventRecorder

	// Shutdown shuts down the broadcaster
	// 结束
	Shutdown()
}
```

具体的实现如下所示：

```go
type eventBroadcasterImpl struct {
	*watch.Broadcaster // 事件广播器，recorder会将数据写入到这里，eventBroadercaster会开启单独的监听协程完成数据处理
	sleepDuration time.Duration //每次发送到sinker后失败需要等待的时间
	options       CorrelatorOptions // 聚合与改正相关的参数
}
```

```go
// StartEventWatcher starts sending events received from this EventBroadcaster to the given event handler function.
// The return value can be ignored or used to stop recording, if desired.
// 开始一个监听器
func (e *eventBroadcasterImpl) StartEventWatcher(eventHandler func(*v1.Event)) watch.Interface {
	//新建一个监听器，这样底层的broadcater之间接收到数据后，就可以通知此watcher
	watcher := e.Watch()
	go func() {
		defer utilruntime.HandleCrash()
		for watchEvent := range watcher.ResultChan() {
			event, ok := watchEvent.Object.(*v1.Event)
			if !ok {
				// This is all local, so there's no reason this should
				// ever happen.
				continue
			}
			eventHandler(event)
		}
	}()
	return watcher
}
```


```go
// StartRecordingToSink starts sending events received from the specified eventBroadcaster to the given sink.
// The return value can be ignored or used to stop recording, if desired.
// TODO: make me an object with parameterizable queue length and retry interval
// 开始向Sinker中写入
func (e *eventBroadcasterImpl) StartRecordingToSink(sink EventSink) watch.Interface {
	// 新建一个改写器
	eventCorrelator := NewEventCorrelatorWithOptions(e.options)
	return e.StartEventWatcher(
		func(event *v1.Event) {
			// 将数据写入到sinker中
			recordToSink(sink, event, eventCorrelator, e.sleepDuration)
		})
}
```

```go
func recordToSink(sink EventSink, event *v1.Event, eventCorrelator *EventCorrelator, sleepDuration time.Duration) {
	// Make a copy before modification, because there could be multiple listeners.
	// Events are safe to copy like this.
	eventCopy := *event
	event = &eventCopy
	// 基于新建的corrector处理下数据
	result, err := eventCorrelator.EventCorrelate(event)
	if err != nil {
		utilruntime.HandleError(err)
	}
	// 如果需要跳过，则直接跳过
	if result.Skip {
		return
	}
	tries := 0
	for {
		//开始写如数据到sinker
		if recordEvent(sink, result.Event, result.Patch, result.Event.Count > 1, eventCorrelator) {
			//如果成功则直接返回
			break
		}
		//重试
		tries++
		if tries >= maxTriesPerEvent {
			klog.Errorf("Unable to write event '%#v' (retry limit exceeded!)", event)
			break
		}
		// Randomize the first sleep so that various clients won't all be
		// synced up if the master goes down.
		//第一次增加一些随机事件，这样大部分APIServer访问不会同时发生
		if tries == 1 {
			time.Sleep(time.Duration(float64(sleepDuration) * rand.Float64()))
		} else {
			time.Sleep(sleepDuration)
		}
	}
}
```


```go
// recordEvent attempts to write event to a sink. It returns true if the event
// was successfully recorded or discarded, false if it should be retried.
// If updateExistingEvent is false, it creates a new event, otherwise it updates
// existing event.
// 尝试写数据到sinker中
func recordEvent(sink EventSink, event *v1.Event, patch []byte, updateExistingEvent bool, eventCorrelator *EventCorrelator) bool {
	var newEvent *v1.Event
	var err error
	// 似乎是对于eventSeries的处理，但是没有找到series的打印入口
	if updateExistingEvent {
		newEvent, err = sink.Patch(event, patch)
	}
	// Update can fail because the event may have been removed and it no longer exists.
	if !updateExistingEvent || (updateExistingEvent && util.IsKeyNotFoundError(err)) {
		//如果不是更新操作，或者更新过程中发现错误，则新建
		// Making sure that ResourceVersion is empty on creation
		event.ResourceVersion = ""
		newEvent, err = sink.Create(event)
	}
	if err == nil {
		// we need to update our event correlator with the server returned state to handle name/resourceversion
		//否则更新
		eventCorrelator.UpdateState(newEvent)
		return true
	}

	// If we can't contact the server, then hold everything while we keep trying.
	// Otherwise, something about the event is malformed and we should abandon it.
	switch err.(type) {
	case *restclient.RequestConstructionError:
		// We will construct the request the same next time, so don't keep trying.
		klog.Errorf("Unable to construct event '%#v': '%v' (will not retry!)", event, err)
		return true
	case *errors.StatusError:
		if errors.IsAlreadyExists(err) {
			klog.V(5).Infof("Server rejected event '%#v': '%v' (will not retry!)", event, err)
		} else {
			klog.Errorf("Server rejected event '%#v': '%v' (will not retry!)", event, err)
		}
		return true
	case *errors.UnexpectedObjectError:
		// We don't expect this; it implies the server's response didn't match a
		// known pattern. Go ahead and retry.
	default:
		// This case includes actual http transport errors. Go ahead and retry.
	}
	klog.Errorf("Unable to write event: '%#v': '%v'(may retry after sleeping)", event, err)
	return false
}
```


### Recorder

```go
//recorder 实现
type recorderImpl struct {
	scheme *runtime.Scheme //scheme 用于解析某个对象GVR等信息
	source v1.EventSource // 事件的创造者
	*watch.Broadcaster //将消息发送到Broadcaster
	clock clock.Clock //时钟
}

//创建事件
func (recorder *recorderImpl) generateEvent(object runtime.Object, annotations map[string]string, eventtype, reason, message string) {
	//创建索引对象
	ref, err := ref.GetReference(recorder.scheme, object)
	if err != nil {
		klog.Errorf("Could not construct reference to: '%#v' due to: '%v'. Will not report event: '%v' '%v' '%v'", object, err, eventtype, reason, message)
		return
	}

	// 只能是warninng或者normal
	if !util.ValidateEventType(eventtype) {
		klog.Errorf("Unsupported event type: '%v'", eventtype)
		return
	}

	//新建一个对象
	event := recorder.makeEvent(ref, annotations, eventtype, reason, message)
	//设置来源
	event.Source = recorder.source

	// NOTE: events should be a non-blocking operation, but we also need to not
	// put this in a goroutine, otherwise we'll race to write to a closed channel
	// when we go to shut down this broadcaster.  Just drop events if we get overloaded,
	// and log an error if that happens (we've configured the broadcaster to drop
	// outgoing events anyway).
	//发送或者丢弃事件，调用底层broadcaster方法，触发整理逻辑
	if sent := recorder.ActionOrDrop(watch.Added, event); !sent {
		klog.Errorf("unable to record event: too many queued events, dropped event %#v", event)
	}
}

//发送一个事件
func (recorder *recorderImpl) Event(object runtime.Object, eventtype, reason, message string) {
	recorder.generateEvent(object, nil, eventtype, reason, message)
}

func (recorder *recorderImpl) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	recorder.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

func (recorder *recorderImpl) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	recorder.generateEvent(object, annotations, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

//创建一个事件对象
func (recorder *recorderImpl) makeEvent(ref *v1.ObjectReference, annotations map[string]string, eventtype, reason, message string) *v1.Event {
	t := metav1.Time{Time: recorder.clock.Now()}
	namespace := ref.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	return &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%v.%x", ref.Name, t.UnixNano()),
			Namespace:   namespace,
			Annotations: annotations,
		},
		InvolvedObject: *ref,
		Reason:         reason,
		Message:        message,
		FirstTimestamp: t,
		LastTimestamp:  t,
		Count:          1,
		Type:           eventtype,
	}
}
```


### Sinker

```go
// EventSink knows how to store events (client.Client implements it.)
// EventSink must respect the namespace that will be embedded in 'event'.
// It is assumed that EventSink will return the same sorts of errors as
// pkg/client's REST client.
// 事件存储接口，可创建，可以更新，可删除
type EventSink interface {
	Create(event *v1.Event) (*v1.Event, error)
	Update(event *v1.Event) (*v1.Event, error)
	Patch(oldEvent *v1.Event, data []byte) (*v1.Event, error)
}
```


可以基于`k8s.io/client-go/kubernetes/typed/core/v1`包的`EventSinkImpl`的实现

```go
sinker := typedv1core.EventSinkImpl{Interface: client.CoreV1().Events("")}
```


### EventCorrelator

可以对Event做聚合或者做限流。

#### 限流器的实现

```go

// getSpamKey builds unique event key based on source, involvedObject
// 限流器的Key
func getSpamKey(event *v1.Event) string {
	return strings.Join([]string{
		event.Source.Component,
		event.Source.Host,
		event.InvolvedObject.Kind,
		event.InvolvedObject.Namespace,
		event.InvolvedObject.Name,
		string(event.InvolvedObject.UID),
		event.InvolvedObject.APIVersion,
	},
		"")
}

// EventFilterFunc is a function that returns true if the event should be skipped
type EventFilterFunc func(event *v1.Event) bool

// EventSourceObjectSpamFilter is responsible for throttling
// the amount of events a source and object can produce.
//具体实现
type EventSourceObjectSpamFilter struct {
	sync.RWMutex

	// the cache that manages last synced state
	// cache
	cache *lru.Cache

	// burst is the amount of events we allow per source + object
	burst int

	// qps is the refill rate of the token bucket in queries per second
	qps float32

	// clock is used to allow for testing over a time interval
	clock clock.Clock
}

// NewEventSourceObjectSpamFilter allows burst events from a source about an object with the specified qps refill.
func NewEventSourceObjectSpamFilter(lruCacheSize, burst int, qps float32, clock clock.Clock) *EventSourceObjectSpamFilter {
	return &EventSourceObjectSpamFilter{
		cache: lru.New(lruCacheSize),
		burst: burst,
		qps:   qps,
		clock: clock,
	}
}

// spamRecord holds data used to perform spam filtering decisions.
type spamRecord struct {
	// rateLimiter controls the rate of events about this object
	rateLimiter flowcontrol.RateLimiter
}

// Filter controls that a given source+object are not exceeding the allowed rate.
func (f *EventSourceObjectSpamFilter) Filter(event *v1.Event) bool {
	var record spamRecord

	// controls our cached information about this event (source+object)
	// 拿到key
	eventKey := getSpamKey(event)

	// do we have a record of similar events in our cache?
	f.Lock()
	defer f.Unlock()
	value, found := f.cache.Get(eventKey)
	if found {
		record = value.(spamRecord)
	}

	// verify we have a rate limiter for this record
	if record.rateLimiter == nil {
		//令牌桶限流
		record.rateLimiter = flowcontrol.NewTokenBucketRateLimiterWithClock(f.qps, f.burst, f.clock)
	}

	// ensure we have available rate
	filter := !record.rateLimiter.TryAccept()

	// update the cache
	f.cache.Add(eventKey, record)

	return filter
}
```

#### 聚合器

```go

// EventAggregatorKeyFunc is responsible for grouping events for aggregation
// It returns a tuple of the following:
// aggregateKey - key the identifies the aggregate group to bucket this event
// localKey - key that makes this event in the local group
type EventAggregatorKeyFunc func(event *v1.Event) (aggregateKey string, localKey string)

// EventAggregatorByReasonFunc aggregates events by exact match on event.Source, event.InvolvedObject, event.Type,
// event.Reason, event.ReportingController and event.ReportingInstance
//搞一个聚合key
func EventAggregatorByReasonFunc(event *v1.Event) (string, string) {
	return strings.Join([]string{
		event.Source.Component,
		event.Source.Host,
		event.InvolvedObject.Kind,
		event.InvolvedObject.Namespace,
		event.InvolvedObject.Name,
		string(event.InvolvedObject.UID),
		event.InvolvedObject.APIVersion,
		event.Type,
		event.Reason,
		event.ReportingController,
		event.ReportingInstance,
	},
		""), event.Message
}

// EventAggregatorMessageFunc is responsible for producing an aggregation message
type EventAggregatorMessageFunc func(event *v1.Event) string

// EventAggregratorByReasonMessageFunc returns an aggregate message by prefixing the incoming message
// 聚合之后使用的消息
func EventAggregatorByReasonMessageFunc(event *v1.Event) string {
	return "(combined from similar events): " + event.Message
}

// EventAggregator identifies similar events and aggregates them into a single event
type EventAggregator struct {
	sync.RWMutex

	// The cache that manages aggregation state
	// cache中存储record
	cache *lru.Cache

	// The function that groups events for aggregation
	// 返回2个string，第一个作为cache外层的key，第二个作为record内部的key
	keyFunc EventAggregatorKeyFunc

	// The function that generates a message for an aggregate event
	// 当信息被聚合后，应该如何显示信息
	messageFunc EventAggregatorMessageFunc

	// The maximum number of events in the specified interval before aggregation occurs
	// record中多于多少个events时会聚合
	maxEvents uint

	// The amount of time in seconds that must transpire since the last occurrence of a similar event before it's considered new
	// event最大保存时间
	maxIntervalInSeconds uint

	// clock is used to allow for testing over a time interval
	clock clock.Clock
}

// NewEventAggregator returns a new instance of an EventAggregator
func NewEventAggregator(lruCacheSize int, keyFunc EventAggregatorKeyFunc, messageFunc EventAggregatorMessageFunc,
	maxEvents int, maxIntervalInSeconds int, clock clock.Clock) *EventAggregator {
	return &EventAggregator{
		cache:                lru.New(lruCacheSize), //存储对象多少
		keyFunc:              keyFunc,// 聚合key，localkey的产生方法
		messageFunc:          messageFunc, // 当消息需要聚合时,聚合信息产生方法
		maxEvents:            uint(maxEvents), //单个record event最多数量
		maxIntervalInSeconds: uint(maxIntervalInSeconds), // 最大事件缓存事件
		clock:                clock,
	}
}

// aggregateRecord holds data used to perform aggregation decisions
type aggregateRecord struct {
	// we track the number of unique local keys we have seen in the aggregate set to know when to actually aggregate
	// if the size of this set exceeds the max, we know we need to aggregate
	// record内部的多个local key
	localKeys sets.String
	// The last time at which the aggregate was recorded
	lastTimestamp metav1.Time
}

// EventAggregate checks if a similar event has been seen according to the
// aggregation configuration (max events, max interval, etc) and returns:
//
// - The (potentially modified) event that should be created
// - The cache key for the event, for correlation purposes. This will be set to
//   the full key for normal events, and to the result of
//   EventAggregatorMessageFunc for aggregate events.
// 聚合方法
func (e *EventAggregator) EventAggregate(newEvent *v1.Event) (*v1.Event, string) {
	now := metav1.NewTime(e.clock.Now())
	var record aggregateRecord
	// eventKey is the full cache key for this event
	// 一个event的全部信息key，包括message和其他元数据
	eventKey := getEventKey(newEvent)
	// aggregateKey is for the aggregate event, if one is needed.
	//默认情况下aggregateKey 不包含 message，localKey为message，也就是说按照除了message的元数据做record分类，基于message做record内记录
	aggregateKey, localKey := e.keyFunc(newEvent)

	// Do we have a record of similar events in our cache?
	e.Lock()
	defer e.Unlock()
	//从缓存中获取记录
	value, found := e.cache.Get(aggregateKey)
	if found {
		record = value.(aggregateRecord)
	}

	// Is the previous record too old? If so, make a fresh one. Note: if we didn't
	// find a similar record, its lastTimestamp will be the zero value, so we
	// create a new one in that case.
	maxInterval := time.Duration(e.maxIntervalInSeconds) * time.Second
	interval := now.Time.Sub(record.lastTimestamp.Time)
	// 如果事件太长了，就当没有任何event发生过
	if interval > maxInterval {
		record = aggregateRecord{localKeys: sets.NewString()}
	}

	// Write the new event into the aggregation record and put it on the cache
	//加入一条localkey
	record.localKeys.Insert(localKey)
	// 更新时间
	record.lastTimestamp = now
	// 增加记录
	e.cache.Add(aggregateKey, record)

	// If we are not yet over the threshold for unique events, don't correlate them
	// 如果单个record中的localKey数量没有限流，则正常增加
	if uint(record.localKeys.Len()) < e.maxEvents {
		// eventKey为圈梁数据
		return newEvent, eventKey
	}

	// do not grow our local key set any larger than max
	// 删除任何一个,因为刚刚添加了一个，所以不会影响限流
	record.localKeys.PopAny()

	// create a new aggregate event, and return the aggregateKey as the cache key
	// (so that it can be overwritten.)
	// 新建一个对象
	eventCopy := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%v.%x", newEvent.InvolvedObject.Name, now.UnixNano()),
			Namespace: newEvent.Namespace,
		},
		Count:          1, // 将数值设置为1，表示后期需要不断更新操作
		FirstTimestamp: now,
		InvolvedObject: newEvent.InvolvedObject,
		LastTimestamp:  now,
		Message:        e.messageFunc(newEvent),
		Type:           newEvent.Type,
		Reason:         newEvent.Reason,
		Source:         newEvent.Source,
	}
	// 返回，copyEvent,并返回聚合的key，这样可以和其他限流的对象公用一个事件，也就是后面会走更新操作
	return eventCopy, aggregateKey
}
```


####  eventLogger

记录已经发生的事件，查看事件是否存在，如果已经存在了则尽力更新事件。

```go

// eventLogger logs occurrences of an event
type eventLogger struct {
	sync.RWMutex
	cache *lru.Cache
	clock clock.Clock
}

// newEventLogger observes events and counts their frequencies
func newEventLogger(lruCacheEntries int, clock clock.Clock) *eventLogger {
	return &eventLogger{cache: lru.New(lruCacheEntries), clock: clock}
}

// eventObserve records an event, or updates an existing one if key is a cache hit
// 查看key是否存在，如果已经存在，并且count>0 则生成patch需要的body
func (e *eventLogger) eventObserve(newEvent *v1.Event, key string) (*v1.Event, []byte, error) {
	var (
		patch []byte
		err   error
	)
	eventCopy := *newEvent
	event := &eventCopy

	e.Lock()
	defer e.Unlock()

	// Check if there is an existing event we should update
	//  获得记录
	lastObservation := e.lastEventObservationFromCache(key)

	// If we found a result, prepare a patch
	// 大于0, 则更新
	if lastObservation.count > 0 {
		// update the event based on the last observation so patch will work as desired
		event.Name = lastObservation.name
		event.ResourceVersion = lastObservation.resourceVersion
		event.FirstTimestamp = lastObservation.firstTimestamp
		event.Count = int32(lastObservation.count) + 1

		eventCopy2 := *event
		eventCopy2.Count = 0
		eventCopy2.LastTimestamp = metav1.NewTime(time.Unix(0, 0))
		eventCopy2.Message = ""

		newData, _ := json.Marshal(event)
		oldData, _ := json.Marshal(eventCopy2)
		patch, err = strategicpatch.CreateTwoWayMergePatch(oldData, newData, event)
	}

	// record our new observation
	// 由于Record内的部分限流key，长时间不存在，增加过程会将老对象删除
	e.cache.Add(
		key,
		eventLog{
			count:           uint(event.Count),
			firstTimestamp:  event.FirstTimestamp,
			name:            event.Name,
			resourceVersion: event.ResourceVersion,
		},
	)
	return event, patch, err
}

// updateState updates its internal tracking information based on latest server state
// 更新最新事件
func (e *eventLogger) updateState(event *v1.Event) {
	key := getEventKey(event)
	e.Lock()
	defer e.Unlock()
	// record our new observation
	e.cache.Add(
		key,
		eventLog{
			count:           uint(event.Count),
			firstTimestamp:  event.FirstTimestamp,
			name:            event.Name,
			resourceVersion: event.ResourceVersion,
		},
	)
}

// lastEventObservationFromCache returns the event from the cache, reads must be protected via external lock
func (e *eventLogger) lastEventObservationFromCache(key string) eventLog {
	value, ok := e.cache.Get(key)
	if ok {
		observationValue, ok := value.(eventLog)
		if ok {
			return observationValue
		}
	}
	return eventLog{}
}

```

#### EventCorrelator

```go
func NewEventCorrelator(clock clock.Clock) *EventCorrelator {
	cacheSize := maxLruCacheEntries
	spamFilter := NewEventSourceObjectSpamFilter(cacheSize, defaultSpamBurst, defaultSpamQPS, clock)
	return &EventCorrelator{
	    // 基于限流器做限流
		filterFunc: spamFilter.Filter,
		// 聚合器
		aggregator: NewEventAggregator(
			cacheSize,
			EventAggregatorByReasonFunc,
			EventAggregatorByReasonMessageFunc,
			defaultAggregateMaxEvents,
			defaultAggregateIntervalInSeconds,
			clock),
	    // eventlogger
		logger: newEventLogger(cacheSize, clock),
	}
}

func NewEventCorrelatorWithOptions(options CorrelatorOptions) *EventCorrelator {
	optionsWithDefaults := populateDefaults(options)
	spamFilter := NewEventSourceObjectSpamFilter(optionsWithDefaults.LRUCacheSize,
		optionsWithDefaults.BurstSize, optionsWithDefaults.QPS, optionsWithDefaults.Clock)
	return &EventCorrelator{
		filterFunc: spamFilter.Filter,
		aggregator: NewEventAggregator(
			optionsWithDefaults.LRUCacheSize,
			optionsWithDefaults.KeyFunc,
			optionsWithDefaults.MessageFunc,
			optionsWithDefaults.MaxEvents,
			optionsWithDefaults.MaxIntervalInSeconds,
			optionsWithDefaults.Clock),
		logger: newEventLogger(optionsWithDefaults.LRUCacheSize, optionsWithDefaults.Clock),
	}
}

// populateDefaults populates the zero value options with defaults
// 增加默认值
func populateDefaults(options CorrelatorOptions) CorrelatorOptions {
	if options.LRUCacheSize == 0 {
		options.LRUCacheSize = maxLruCacheEntries
	}
	if options.BurstSize == 0 {
		options.BurstSize = defaultSpamBurst
	}
	if options.QPS == 0 {
		options.QPS = defaultSpamQPS
	}
	if options.KeyFunc == nil {
		options.KeyFunc = EventAggregatorByReasonFunc
	}
	if options.MessageFunc == nil {
		options.MessageFunc = EventAggregatorByReasonMessageFunc
	}
	if options.MaxEvents == 0 {
		options.MaxEvents = defaultAggregateMaxEvents
	}
	if options.MaxIntervalInSeconds == 0 {
		options.MaxIntervalInSeconds = defaultAggregateIntervalInSeconds
	}
	if options.Clock == nil {
		options.Clock = clock.RealClock{}
	}
	return options
}

// EventCorrelate filters, aggregates, counts, and de-duplicates all incoming events
func (c *EventCorrelator) EventCorrelate(newEvent *v1.Event) (*EventCorrelateResult, error) {
	if newEvent == nil {
		return nil, fmt.Errorf("event is nil")
	}
	// 拿到聚合key aggregateEvent,
	//如果没有发生限流，则aggregateEvent所谓原始event，ckey为event全部字段生成的key
	//如果生成限流，则aggregateEvent为聚合厚度额event，key为aggregateKey
	aggregateEvent, ckey := c.aggregator.EventAggregate(newEvent)
	// 查看事件是否存在，并且是否是聚合后的key，如果是聚合后的key，则生成patch需要更新的数据
	observedEvent, patch, err := c.logger.eventObserve(aggregateEvent, ckey)
	if c.filterFunc(observedEvent) {
		return &EventCorrelateResult{Skip: true}, nil
	}
	return &EventCorrelateResult{Event: observedEvent, Patch: patch}, err
}
```






