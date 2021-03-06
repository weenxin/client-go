# Retry 包

## Wait

package :  `k8s.io/apimachinery/pkg/util/wait`

### 做什么

定时执行某类操作。
- `给定context结束`
- `给定stop chan结束`
- 定时执行直到成功或者失败

### 如何使用
[retry_test](/util/flowcontrol/retry_test.go)

### 如何实现
####  Group

```go
//  等同与WaitGroup
type Group struct {
	wg sync.WaitGroup
}
// 等同与WaitGroup.Wait()
func (g *Group) Wait() {
	g.wg.Wait()
}

//接收chan的包装
func (g *Group) StartWithChannel(stopCh <-chan struct{}, f func(stopCh <-chan struct{})) {
	g.Start(func() {
		f(stopCh)
	})
}

//接收context的包装
func (g *Group) StartWithContext(ctx context.Context, f func(context.Context)) {
	g.Start(func() {
		f(ctx)
	})
}

//开始运行f
func (g *Group) Start(f func()) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		f()
	}()
}

```

#### Backoff

```go
// Backoff holds parameters applied to a Backoff function.
type Backoff struct {
	// 最初的duration
	Duration time.Duration
	// 每次backoff 都会对Duration * Factor 增加 backoff时间，最大到Cap，同时最大放大Steps次数
	Factor float64
	// 抖动率，在duration增加 Duration * Jitter * Rand（0,1）
	Jitter float64
	// 可以放大次数
	Steps int
	// 最大backoff时间
	Cap time.Duration
}

// 计算需要backoff时间
func (b *Backoff) Step() time.Duration {
    //如果不需要在放大factor了
	if b.Steps < 1 {
	    //如果需要加入抖动
		if b.Jitter > 0 {
		    //增加抖动
			return Jitter(b.Duration, b.Jitter)
		}
		//返回duration
		return b.Duration
	}
	//次数减1
	b.Steps--

	duration := b.Duration

	// calculate the next step
	//如果factor不为0
	if b.Factor != 0 {
	    //计算duration
		b.Duration = time.Duration(float64(b.Duration) * b.Factor)
		//如果duration 大于 cap则设置为cap
		if b.Cap > 0 && b.Duration > b.Cap {
			b.Duration = b.Cap
			b.Steps = 0
		}
	}
	//如果需要抖动则加入抖动
	if b.Jitter > 0 {
		duration = Jitter(duration, b.Jitter)
	}
	return duration
}
//抖动函数
func Jitter(duration time.Duration, maxFactor float64) time.Duration {
	if maxFactor <= 0.0 {
		maxFactor = 1.0
	}
	wait := duration + time.Duration(rand.Float64()*maxFactor*float64(duration))
	return wait
}

```


#### Until

```go

// Forever 每period执行一下f
func Forever(f func(), period time.Duration) {
	Until(f, period, NeverStop)
}

//Until 直到stopCh，每period执行一次函数
func Until(f func(), period time.Duration, stopCh <-chan struct{}) {
	JitterUntil(f, period, 0.0, true, stopCh)
}


//每period执行一次，直到ctx结束
func UntilWithContext(ctx context.Context, f func(context.Context), period time.Duration) {
	JitterUntilWithContext(ctx, f, period, 0.0, true)
}


//Sliding 指先使用Backoff计算backoff时间，还是先执行f， Sliding： 为true则后执行
func NonSlidingUntil(f func(), period time.Duration, stopCh <-chan struct{}) {
	JitterUntil(f, period, 0.0, false, stopCh)
}

//接收context的函数包装
func NonSlidingUntilWithContext(ctx context.Context, f func(context.Context), period time.Duration) {
	JitterUntilWithContext(ctx, f, period, 0.0, false)
}

// 每period 执行f，加入抖动因子，sliding为true则后计算backoff时间，否则先计算backoff时间
func JitterUntil(f func(), period time.Duration, jitterFactor float64, sliding bool, stopCh <-chan struct{}) {
	BackoffUntil(f, NewJitteredBackoffManager(period, jitterFactor, &clock.RealClock{}), sliding, stopCh)
}

//具体的Until函数
func BackoffUntil(f func(), backoff BackoffManager, sliding bool, stopCh <-chan struct{}) {
	var t clock.Timer
	for {
	//如果已经停止了，则直接结束
		select {
		case <-stopCh:
			return
		default:
		}

        //如果sliding为false则先计算backoff时间
		if !sliding {
			t = backoff.Backoff()
		}

        //执行函数
		func() {
			defer runtime.HandleCrash()
			f()
		}()

		if sliding {
			t = backoff.Backoff()
		}

		// NOTE: b/c there is no priority selection in golang
		// it is possible for this to race, meaning we could
		// trigger t.C and stopCh, and t.C select falls through.
		// In order to mitigate we re-check stopCh at the beginning
		// of every loop to prevent extra executions of f().
		//select没有优先级可言，因此为了防止当stopCh和timer都发信号这种场景，我们在循环前
		//re-check一下，防止多执行一次f
		select {
		case <-stopCh:
			if !t.Stop() {
				<-t.C()
			}
			return
		case <-t.C():
		}
	}
}

```

#### ExponentialBackoff

```go


//BackoffManager 提供了一个返回timer的interface，调用者应该backoff到Timer.C()返回信号，如果第一个backoff返回的timer还没有成功等待结束，又重新调用backoff，那么timer行为是无法预测的。 BackoffManager不是线程安全的
type BackoffManager interface {
	Backoff() clock.Timer
}
// 指数backoff实现
type exponentialBackoffManagerImpl struct {
	backoff              *Backoff //backoff结构体
	backoffTimer         clock.Timer // backoffManager返回的timer
	lastBackoffStart     time.Time //最后一次开始时间
	initialBackoff       time.Duration //最初的backoff Duration
	backoffResetDuration time.Duration //多长时间没有backoff，则reset一次
	clock                clock.Clock //时钟
}


func NewExponentialBackoffManager(initBackoff, maxBackoff, resetDuration time.Duration, backoffFactor, jitter float64, c clock.Clock) BackoffManager {
	return &exponentialBackoffManagerImpl{
		backoff: &Backoff{
			Duration: initBackoff, //base时间
			Factor:   backoffFactor, //每次指数增长倍数
			Jitter:   jitter, //抖动因子

			// the current impl of wait.Backoff returns Backoff.Duration once steps are used up, which is not
			// what we ideally need here, we set it to max int and assume we will never use up the steps
			Steps: math.MaxInt32,  //无限大的次数
			Cap:   maxBackoff, //最大backoff
		},
		backoffTimer:         nil,
		initialBackoff:       initBackoff, //初始时间段
		lastBackoffStart:     c.Now(), //最后一次backoff时间
		backoffResetDuration: resetDuration, //多长时间没有调用backoff则reset一次
		clock:                c, //时钟
	}
}

// 获得下一次backoff
func (b *exponentialBackoffManagerImpl) getNextBackoff() time.Duration {
    //如果很长时间没有backoff了，则清零backoff时间
	if b.clock.Now().Sub(b.lastBackoffStart) > b.backoffResetDuration {
		b.backoff.Steps = math.MaxInt32
		b.backoff.Duration = b.initialBackoff
	}
	//设置最新backoff时间
	b.lastBackoffStart = b.clock.Now()
	return b.backoff.Step()
}

// Backoff implements BackoffManager.Backoff, it returns a timer so caller can block on the timer for exponential backoff.
// The returned timer must be drained before calling Backoff() the second time
func (b *exponentialBackoffManagerImpl) Backoff() clock.Timer {
    //如果没有创建过timer，则创建
	if b.backoffTimer == nil {
		b.backoffTimer = b.clock.NewTimer(b.getNextBackoff())
	} else {//否则复用之前的timer，此时连续来两次调用backoff会返回同一个timer，同时等待一个timer.C()返回结果是随机的
		b.backoffTimer.Reset(b.getNextBackoff())
	}
	return b.backoffTimer
}


//指数执行condition，最多执行backoff.steps次
func ExponentialBackoff(backoff Backoff, condition ConditionFunc) error {
	for backoff.Steps > 0 {
		if ok, err := runConditionWithCrashProtection(condition); err != nil || ok {
			return err
		}
		if backoff.Steps == 1 {
			break
		}
		time.Sleep(backoff.Step())
	}
	return ErrWaitTimeout
}

```

#### JitteredBackoffManager

```go

type jitteredBackoffManagerImpl struct {
	clock        clock.Clock //时钟
	duration     time.Duration // bash时间
	jitter       float64 //抖动因子
	backoffTimer clock.Timer //定时器
}


//NewJitteredBackoffManager 新建BackoffManager
func NewJitteredBackoffManager(duration time.Duration, jitter float64, c clock.Clock) BackoffManager {
	return &jitteredBackoffManagerImpl{
		clock:        c,
		duration:     duration,
		jitter:       jitter,
		backoffTimer: nil,
	}
}
//时间
func (j *jitteredBackoffManagerImpl) getNextBackoff() time.Duration {
	jitteredPeriod := j.duration
	if j.jitter > 0.0 {
		jitteredPeriod = Jitter(j.duration, j.jitter)
	}
	return jitteredPeriod
}

func (j *jitteredBackoffManagerImpl) Backoff() clock.Timer {
    //计算backoff时间
	backoff := j.getNextBackoff()
	//如果timer为空
	if j.backoffTimer == nil {
	    //新建一个timer
		j.backoffTimer = j.clock.NewTimer(backoff)
	} else {//否则复用已有的timer
		j.backoffTimer.Reset(backoff)
	}
	return j.backoffTimer
}

```

#### Poll


```go
//返回一个函数，该函数返回一个chan，这个chan会定时收到事件
//如果timeout为0 ，则需要通过ctx，关闭chan，防止go-routine泄漏
func poller(interval, timeout time.Duration) WaitWithContextFunc {
	return WaitWithContextFunc(func(ctx context.Context) <-chan struct{} {
		ch := make(chan struct{})

		go func() {
			defer close(ch)

			tick := time.NewTicker(interval)
			defer tick.Stop()

			var after <-chan time.Time
			if timeout != 0 {
				// time.After is more convenient, but it
				// potentially leaves timers around much longer
				// than necessary if we exit early.
				timer := time.NewTimer(timeout)
				after = timer.C
				defer timer.Stop()
			}

			for {
				select {
				case <-tick.C:
					// If the consumer isn't ready for this signal drop it and
					// check the other channels.
					select {
					case ch <- struct{}{}:
					default:
					}
				case <-after:
					return
				case <-ctx.Done():
					return
				}
			}
		}()

		return ch
	})
}
```

```go
type ConditionWithContextFunc func(context.Context) (done bool, err error)
//WaitForWithContext wait也就是poller驱动执行fn，直到ctx.Done 或者fn返回（true，nil）
func WaitForWithContext(ctx context.Context, wait WaitWithContextFunc, fn ConditionWithContextFunc) error {
	waitCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := wait(waitCtx)
	for {
		select {
		case _, open := <-c:
			ok, err := runConditionWithCrashProtectionWithContext(ctx, fn)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
			if !open {
				return ErrWaitTimeout
			}
		case <-ctx.Done():
			// returning ctx.Err() will break backward compatibility
			return ErrWaitTimeout
		}
	}
}

//对于chan的封装，wait的封装
func WaitFor(wait WaitFunc, fn ConditionFunc, done <-chan struct{}) error {
	ctx, cancel := contextForChannel(done)
	defer cancel()
	return WaitForWithContext(ctx, wait.WithContext(), fn.WithContext())
}

//如果immediate为true，则立即执行一次，后面就基于wait驱动执行condition
func poll(ctx context.Context, immediate bool, wait WaitWithContextFunc, condition ConditionWithContextFunc) error {
	if immediate {
		done, err := runConditionWithCrashProtectionWithContext(ctx, condition)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}

	select {
	case <-ctx.Done():
		// returning ctx.Err() will break backward compatibility
		return ErrWaitTimeout
	default:
		return WaitForWithContext(ctx, wait, condition)
	}
}
```

#### ExponentialBackoffWithContext

```go
//指数等待方式
func ExponentialBackoffWithContext(ctx context.Context, backoff Backoff, condition ConditionFunc) error {
	for backoff.Steps > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if ok, err := runConditionWithCrashProtection(condition); err != nil || ok {
			return err
		}

		if backoff.Steps == 1 {
			break
		}

		waitBeforeRetry := backoff.Step()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitBeforeRetry):
		}
	}

	return ErrWaitTimeout
}

```

# flowcontrol 包

## Backoff

### 如何使用

[TestBackoff](/util/flowcontrol/backoff_test.go)

### 实现逻辑


#### 结构定义

```go

type backoffEntry struct {
	backoff    time.Duration
	lastUpdate time.Time
}

type Backoff struct {
	sync.RWMutex //锁，保护perItemBackoff
	Clock           clock.Clock //时钟
	defaultDuration time.Duration // base时间
	maxDuration     time.Duration // 最大时间
	perItemBackoff  map[string]*backoffEntry //各个元素的backoff的情况
	rand            *rand.Rand //rand种子

	// maxJitterFactor adds jitter to the exponentially backed off delay.
	// if maxJitterFactor is zero, no jitter is added to the delay in
	// order to maintain current behavior.
	maxJitterFactor float64 //抖动因子
}
```
#### 对象构建

```go
//用于测试
func NewFakeBackOff(initial, max time.Duration, tc *testingclock.FakeClock) *Backoff {
	return newBackoff(tc, initial, max, 0.0)
}

//新建一个backoff， initial为初始时间， max为对最大backoff时间
func NewBackOff(initial, max time.Duration) *Backoff {
	return NewBackOffWithJitter(initial, max, 0.0)
}
//initial为初始时间， max为对最大backoff时间， maxJitterFactor为抖动时间
func NewFakeBackOffWithJitter(initial, max time.Duration, tc *testingclock.FakeClock, maxJitterFactor float64) *Backoff {
	return newBackoff(tc, initial, max, maxJitterFactor)
}
//initial为初始时间， max为对最大backoff时间， maxJitterFactor为抖动时间
func NewBackOffWithJitter(initial, max time.Duration, maxJitterFactor float64) *Backoff {
	clock := clock.RealClock{}
	return newBackoff(clock, initial, max, maxJitterFactor)
}

func newBackoff(clock clock.Clock, initial, max time.Duration, maxJitterFactor float64) *Backoff {
	var random *rand.Rand
	if maxJitterFactor > 0 {
		random = rand.New(rand.NewSource(clock.Now().UnixNano()))
	}
	return &Backoff{
		perItemBackoff:  map[string]*backoffEntry{}, //保存对象backoff当前状态
		Clock:           clock,
		defaultDuration: initial,
		maxDuration:     max,
		maxJitterFactor: maxJitterFactor,
		rand:            random,
	}
}
```

```go
// 获取当前backoff时间，用于sleep，等待时间ok在执行具体动作
func (p *Backoff) Get(id string) time.Duration {
	p.RLock()
	defer p.RUnlock()
	var delay time.Duration
	entry, ok := p.perItemBackoff[id]
	if ok {
		delay = entry.backoff
	}
	return delay
}
```

```go
// 本次动作执行失败，会调用Next，增加backoff时间
func (p *Backoff) Next(id string, eventTime time.Time) {
	p.Lock()
	defer p.Unlock()
	entry, ok := p.perItemBackoff[id]
	if !ok || hasExpired(eventTime, entry.lastUpdate, p.maxDuration) {
		entry = p.initEntryUnsafe(id)
		entry.backoff += p.jitter(entry.backoff)
	} else {
		delay := entry.backoff * 2       // exponential
		delay += p.jitter(entry.backoff) // add some jitter to the delay
		entry.backoff = time.Duration(integer.Int64Min(int64(delay), int64(p.maxDuration)))
	}
	entry.lastUpdate = p.Clock.Now()
}
```

```go
//如果成功了，则reset该对象，reset backoff时间
func (p *Backoff) Reset(id string) {
	p.Lock()
	defer p.Unlock()
	delete(p.perItemBackoff, id)
}
```

```go

// Returns True if the elapsed time since eventTime is smaller than the current backoff window
func (p *Backoff) IsInBackOffSince(id string, eventTime time.Time) bool {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.perItemBackoff[id]
	if !ok {
		return false
	}
	if hasExpired(eventTime, entry.lastUpdate, p.maxDuration) {
		return false
	}
	return p.Clock.Since(eventTime) < entry.backoff
}

// Returns True if time since lastupdate is less than the current backoff window.
func (p *Backoff) IsInBackOffSinceUpdate(id string, eventTime time.Time) bool {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.perItemBackoff[id]
	if !ok {
		return false
	}
	if hasExpired(eventTime, entry.lastUpdate, p.maxDuration) {
		return false
	}
	return eventTime.Sub(entry.lastUpdate) < entry.backoff
}

```

```go
//删除时间过长，不需要追踪的对象
func (p *Backoff) GC() {
	p.Lock()
	defer p.Unlock()
	now := p.Clock.Now()
	for id, entry := range p.perItemBackoff {
		if now.Sub(entry.lastUpdate) > p.maxDuration*2 {
			// GC when entry has not been updated for 2*maxDuration
			delete(p.perItemBackoff, id)
		}
	}
}

//删除对象，有点向reset
func (p *Backoff) DeleteEntry(id string) {
	p.Lock()
	defer p.Unlock()
	delete(p.perItemBackoff, id)
}

```

## Throttle

### 如何使用

[TestThrottle](/util/flowcontrol/throttle_test.go)


### 实现逻辑

#### 结构定义

```go
//乐观锁，可以尽量尝试获取lock
type PassiveRateLimiter interface {
	// TryAccept returns true if a token is taken immediately. Otherwise,
	// it returns false.
	TryAccept() bool
	// Stop stops the rate limiter, subsequent calls to CanAccept will return false
	Stop()
	// QPS returns QPS of this rate limiter
	QPS() float32
}

//悲观锁
type RateLimiter interface {
	PassiveRateLimiter
	// Accept returns once a token becomes available.
	// goroutine 会sleep到可以获取到锁的时间
	Accept()
	// Wait returns nil if a token is taken before the Context is done.
	//等待到可以获取到令牌
	Wait(ctx context.Context) error
}

type tokenBucketPassiveRateLimiter struct {
	limiter *rate.Limiter //基于令牌桶实现
	qps     float32 //令牌发送频率
	clock   clock.PassiveClock //时钟
}

type tokenBucketRateLimiter struct {
	tokenBucketPassiveRateLimiter //
	clock Clock //需要等待，所以需要Sleep等待，clock.PassiveClock满足不了需求
}
```

#### 新建对象

```go

// NewTokenBucketRateLimiterWithClock is identical to NewTokenBucketRateLimiter
// but allows an injectable clock, for testing.
func NewTokenBucketRateLimiterWithClock(qps float32, burst int, c Clock) RateLimiter {
	limiter := rate.NewLimiter(rate.Limit(qps), burst)
	return newTokenBucketRateLimiterWithClock(limiter, c, qps)
}

// NewTokenBucketPassiveRateLimiterWithClock is similar to NewTokenBucketRateLimiterWithClock
// except that it returns a PassiveRateLimiter which does not have Accept() and Wait() methods
// and uses a PassiveClock.
func NewTokenBucketPassiveRateLimiterWithClock(qps float32, burst int, c clock.PassiveClock) PassiveRateLimiter {
	limiter := rate.NewLimiter(rate.Limit(qps), burst)
	return newTokenBucketRateLimiterWithPassiveClock(limiter, c, qps)
}

func newTokenBucketRateLimiterWithClock(limiter *rate.Limiter, c Clock, qps float32) *tokenBucketRateLimiter {
	return &tokenBucketRateLimiter{
		tokenBucketPassiveRateLimiter: *newTokenBucketRateLimiterWithPassiveClock(limiter, c, qps),
		clock:                         c,
	}
}

func newTokenBucketRateLimiterWithPassiveClock(limiter *rate.Limiter, c clock.PassiveClock, qps float32) *tokenBucketPassiveRateLimiter {
	return &tokenBucketPassiveRateLimiter{
		limiter: limiter,
		qps:     qps,
		clock:   c,
	}
}

```

#### 相关方法和实现

```go
//stop需要基于limiter做stop，limiter还不支持，因此什么都不做，看起来还是有些问题的
func (tbprl *tokenBucketPassiveRateLimiter) Stop() {

}

func (tbprl *tokenBucketPassiveRateLimiter) QPS() float32 {
	return tbprl.qps
}
//尝试获取，使用AllowN不等待
func (tbprl *tokenBucketPassiveRateLimiter) TryAccept() bool {
	return tbprl.limiter.AllowN(tbprl.clock.Now(), 1)
}

// Accept will block until a token becomes available
// goroutine等待
func (tbrl *tokenBucketRateLimiter) Accept() {
	now := tbrl.clock.Now()
	tbrl.clock.Sleep(tbrl.limiter.ReserveN(now, 1).DelayFrom(now))
}
//调用令牌桶的等待
func (tbrl *tokenBucketRateLimiter) Wait(ctx context.Context) error {
	return tbrl.limiter.Wait(ctx)
}

```







