# Clock

## Interface

为了方便测试做的一个对于Clock的一次封装

```go

// PassiveClock allows for injecting fake or real clocks into code
// that needs to read the current time but does not support scheduling
// activity in the future.
type PassiveClock interface {
    //当前时间
	Now() time.Time
	//当前时间与目标之间的时间差
	Since(time.Time) time.Duration
}

// Clock allows for injecting fake or real clocks into code that
// needs to do arbitrary things based on time.
type Clock interface {
	PassiveClock
	// After returns the channel of a new Timer.
	// This method does not allow to free/GC the backing timer before it fires. Use
	// NewTimer instead.
	After(d time.Duration) <-chan time.Time
	// NewTimer returns a new Timer.
	NewTimer(d time.Duration) Timer
	// Sleep sleeps for the provided duration d.
	// Consider making the sleep interruptible by using 'select' on a context channel and a timer channel.
	Sleep(d time.Duration)
	// Tick returns the channel of a new Ticker.
	// This method does not allow to free/GC the backing ticker. Use
	// NewTicker from WithTicker instead.
	Tick(d time.Duration) <-chan time.Time
}

// WithTicker allows for injecting fake or real clocks into code that
// needs to do arbitrary things based on time.
type WithTicker interface {
	Clock
	// NewTicker returns a new Ticker.
	NewTicker(time.Duration) Ticker
}

// WithDelayedExecution allows for injecting fake or real clocks into
// code that needs to make use of AfterFunc functionality.
type WithDelayedExecution interface {
	Clock
	// AfterFunc executes f in its own goroutine after waiting
	// for d duration and returns a Timer whose channel can be
	// closed by calling Stop() on the Timer.
	AfterFunc(d time.Duration, f func()) Timer
}

// WithTickerAndDelayedExecution allows for injecting fake or real clocks
// into code that needs Ticker and AfterFunc functionality
type WithTickerAndDelayedExecution interface {
	WithTicker
	// AfterFunc executes f in its own goroutine after waiting
	// for d duration and returns a Timer whose channel can be
	// closed by calling Stop() on the Timer.
	AfterFunc(d time.Duration, f func()) Timer
}

// Ticker defines the Ticker interface.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

```

## RealClock

真实的Clock，大部分用的 `time`包的函数


```go

// RealClock 是真的Clock
type RealClock struct{}

// Now 返回当前时间
func (RealClock) Now() time.Time {
	return time.Now()
}
```

```go
// 返回当前时间和目标时间的时间差
func (RealClock) Since(ts time.Time) time.Duration {
	return time.Since(ts)
}

// 过多久之后返回一个信号
func (RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// 新建timer
func (RealClock) NewTimer(d time.Duration) Timer {
	return &realTimer{
		timer: time.NewTimer(d),
	}
}

// AfterFunc is the same as time.AfterFunc(d, f).
func (RealClock) AfterFunc(d time.Duration, f func()) Timer {
	return &realTimer{
		timer: time.AfterFunc(d, f),
	}
}



// Tick is the same as time.Tick(d)
// This method does not allow to free/GC the backing ticker. Use
// NewTicker instead.
func (RealClock) Tick(d time.Duration) <-chan time.Time {
	return time.Tick(d)
}

// NewTicker returns a new Ticker.
func (RealClock) NewTicker(d time.Duration) Ticker {
	return &realTicker{
		ticker: time.NewTicker(d),
	}
}

// Sleep is the same as time.Sleep(d)
// Consider making the sleep interruptible by using 'select' on a context channel and a timer channel.
func (RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// Timer allows for injecting fake or real timers into code that
// needs to do arbitrary things based on time.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

var _ = Timer(&realTimer{})

// realTimer is backed by an actual time.Timer.
type realTimer struct {
	timer *time.Timer
}

// C returns the underlying timer's channel.
func (r *realTimer) C() <-chan time.Time {
	return r.timer.C
}

// Stop calls Stop() on the underlying timer.
func (r *realTimer) Stop() bool {
	return r.timer.Stop()
}

// Reset calls Reset() on the underlying timer.
func (r *realTimer) Reset(d time.Duration) bool {
	return r.timer.Reset(d)
}

type realTicker struct {
	ticker *time.Ticker
}

func (r *realTicker) C() <-chan time.Time {
	return r.ticker.C
}

func (r *realTicker) Stop() {
	r.ticker.Stop()
}

```

