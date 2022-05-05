# Wait

## Pacage

- k8s.io/apimachinery/pkg/util/wait

## Group

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

## Backoff

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


## Until

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

## ExponentialBackoff

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

## JitteredBackoffManager

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

## Poll

```go
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

