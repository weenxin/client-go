



## 1.1 令牌桶

[wiki](https://en.wikipedia.org/wiki/Token_bucket)

### 1.1.1 令牌桶基本结构

```go
type Limiter struct {
	mu     sync.Mutex // 互斥锁
	limit  Limit    //限流器，没秒钟按照这个频率下发令牌
	burst  int      //桶的最大令牌数量，超过这个数量的令牌将会被丢弃
	tokens float64  //当前令牌数量
	// last is the last time the limiter's tokens field was updated
	last time.Time //最后一次计算令牌的时间
	// lastEvent is the latest time of a rate-limited event (past or future)
	lastEvent time.Time //最后一次限流发生时间
}
```

```go
// Limit 获取令牌颁发频率
func (lim *Limiter) Limit() Limit {
	lim.mu.Lock()
	defer lim.mu.Unlock()
	return lim.limit
}

// Burst 获取桶最大数量
// 一个最大令牌数的令牌桶不允许任何时间除非limit为无限
func (lim *Limiter) Burst() int {
	lim.mu.Lock()
	defer lim.mu.Unlock()
	return lim.burst
}
```

```go
// 消耗一个令牌
func (lim *Limiter) Allow() bool {
	return lim.AllowN(time.Now(), 1)
}

// AllowN 报告当前是否有n个令牌立刻可得
// 当你想要忽略超出限流的事件时，使用该方法；
// 否则应该使用Reserve或者Wait方法.
func (lim *Limiter) AllowN(now time.Time, n int) bool {
	return lim.reserveN(now, n, 0).ok
}
```



### 1.1.2 Reservation

```go
type Reservation struct {
	ok        bool // 是否有效
	lim       *Limiter //限流器父亲
	tokens    int //需要多少token
	timeToAct time.Time //何时可以出发这个事件
	// This is the Limit at reservation time, it can change later.
	limit Limit // 父限流器的令牌发送频率
}
```

```go
//是否可以获得指定数量的token
//如果OK返回false，则Delay将返回InfDuration，Cancel不做任何事情
func (r *Reservation) OK() bool {
	return r.ok
}

//需要等待的时间
func (r *Reservation) Delay() time.Duration {
	return r.DelayFrom(time.Now())
}

// InfDuration is the duration returned by Delay when a Reservation is not OK.
const InfDuration = time.Duration(1<<63 - 1)

//DelayFrom 指定还需要多久可以执行相关动作，返回zero时间代表着相关动作可以立即执行，InfDuration代表永远无法提供指定数量的令牌
func (r *Reservation) DelayFrom(now time.Time) time.Duration {
	if !r.ok { //如果不OK则返回无限长的时间
		return InfDuration
	}
	delay := r.timeToAct.Sub(now)
	if delay < 0 {
		return 0
	}
	return delay
}

// 取消Reservation，并返还已经获取的令牌
func (r *Reservation) Cancel() {
	r.CancelAt(time.Now())
}

// CancelAt 代表相关的动作将不会执行，尽量归还所有已经获取的令牌
func (r *Reservation) CancelAt(now time.Time) {
	if !r.ok { //如果不OK不需要做任何事情
		return
	}

    获取锁
	r.lim.mu.Lock()
	defer r.lim.mu.Unlock()

    //如果没有限流，或者没有获取任何token，或者事件应该在更早之前就发生，则不做任何事情
	if r.lim.limit == Inf || r.tokens == 0 || r.timeToAct.Before(now) {
		return
	}

	// calculate tokens to restore
	// The duration between lim.lastEvent and r.timeToAct tells us how many tokens were reserved
	// after r was obtained. These tokens should not be restored.
	// 由于limiter的令牌颁发频率可能发生变化，因此需按照 `r.limit.tokensFromDuration(r.lim.lastEvent.Sub(r.timeToAct))` 而不是直接归还指定数量的token， 计算下需要规划多少token，此时token可能是负值的
	restoreTokens := float64(r.tokens) - r.limit.tokensFromDuration(r.lim.lastEvent.Sub(r.timeToAct))
	//如果不需要返回任何token，则返回
	if restoreTokens <= 0 {
		return
	}
	// 更新下令牌数量（按照当前时间与最后一次更新时间的差）
	now, _, tokens := r.lim.advance(now)
	// 增加token
	tokens += restoreTokens
	if burst := float64(r.lim.burst); tokens > burst {
		tokens = burst
	}
	// update state
	r.lim.last = now
	r.lim.tokens = tokens
	if r.timeToAct == r.lim.lastEvent { //如果本次就是最后一个Reservation那么尝试计算上一个事件时间
		prevEvent := r.timeToAct.Add(r.limit.durationFromTokens(float64(-r.tokens)))
		if !prevEvent.Before(now) {
			r.lim.lastEvent = prevEvent
		}
	}
}
```

```go
// Reserve is shorthand for ReserveN(time.Now(), 1).
func (lim *Limiter) Reserve() *Reservation {
	return lim.ReserveN(time.Now(), 1)
}

// ReserveN returns a Reservation that indicates how long the caller must wait before n events happen.
// The Limiter takes this Reservation into account when allowing future events.
// The returned Reservation’s OK() method returns false if n exceeds the Limiter's burst size.
// Usage example:
//   r := lim.ReserveN(time.Now(), 1)
//   if !r.OK() {
//     // Not allowed to act! Did you remember to set lim.burst to be > 0 ?
//     return
//   }
//   time.Sleep(r.Delay())
//   Act()
// Use this method if you wish to wait and slow down in accordance with the rate limit without dropping events.
// If you need to respect a deadline or cancel the delay, use Wait instead.
// To drop or skip events exceeding rate limit, use Allow instead.
func (lim *Limiter) ReserveN(now time.Time, n int) *Reservation {
	r := lim.reserveN(now, n, InfDuration)
	return &r
}


// reserveN is a helper method for AllowN, ReserveN, and WaitN.
// maxFutureReserve specifies the maximum reservation wait duration allowed.
// reserveN returns Reservation, not *Reservation, to avoid allocation in AllowN and WaitN.
func (lim *Limiter) reserveN(now time.Time, n int, maxFutureReserve time.Duration) Reservation {
    //加锁
	lim.mu.Lock()
	defer lim.mu.Unlock()

    //如果不限流则直接返回
	if lim.limit == Inf {
		return Reservation{
			ok:        true,
			lim:       lim,
			tokens:    n,
			timeToAct: now,
		}
	} else if lim.limit == 0 {
	    //限流但是不会定时颁发令牌
		var ok bool
		//如果桶的令牌数还够用
		if lim.burst >= n {
			ok = true
			lim.burst -= n
		}
		//返回结果
		return Reservation{
			ok:        ok,
			lim:       lim,
			tokens:    lim.burst,
			timeToAct: now,
		}
	}

    //更新令牌数量
	now, last, tokens := lim.advance(now)

	// Calculate the remaining number of tokens resulting from the request.
	//分配令牌
	tokens -= float64(n)

	// Calculate the wait duration
	// 计算等待时间
	var waitDuration time.Duration
	if tokens < 0 {
		waitDuration = lim.limit.durationFromTokens(-tokens)
	}

	// Decide result
	// 要求的令牌数量必须小于桶的最大值，并且最大等待时间必须小于最大等待时间
	ok := n <= lim.burst && waitDuration <= maxFutureReserve

	// Prepare reservation
	r := Reservation{
		ok:    ok,
		lim:   lim,
		limit: lim.limit,
	}
	//如果成功获取令牌
	if ok {
		r.tokens = n
		//计算时间发生时间
		r.timeToAct = now.Add(waitDuration)
	}

	// Update state
	if ok {
		lim.last = now
		//tokens 可能是负数
		lim.tokens = tokens
		//更新最后分配时间
		lim.lastEvent = r.timeToAct
	} else {
		lim.last = last
	}

	return r
}

```

```go

// Wait is shorthand for WaitN(ctx, 1).
func (lim *Limiter) Wait(ctx context.Context) (err error) {
	return lim.WaitN(ctx, 1)
}

//WaitN  等待限流器有N个令牌，如果n大于 limiter的桶大小，context超时，或者context被取消则返回失败信息
func (lim *Limiter) WaitN(ctx context.Context, n int) (err error) {
	lim.mu.Lock()
	burst := lim.burst
	limit := lim.limit
	lim.mu.Unlock()

    //如果要求令牌大于桶大小，并且限流器不是无限大则返回错误
	if n > burst && limit != Inf {
		return fmt.Errorf("rate: Wait(n=%d) exceeds limiter's burst %d", n, burst)
	}
	// Check if ctx is already cancelled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	// Determine wait limit
	now := time.Now()
	waitLimit := InfDuration
	if deadline, ok := ctx.Deadline(); ok {
		waitLimit = deadline.Sub(now)
	}
	// Reserve，获取reserve，如果在指定时间内不能提供数量的令牌将会返回失败
	r := lim.reserveN(now, n, waitLimit)
	if !r.ok {
		return fmt.Errorf("rate: Wait(n=%d) would exceed context deadline", n)
	}
	// Wait if necessary，计算等待时间
	delay := r.DelayFrom(now)
	if delay == 0 {
		return nil
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-t.C:
		// We can proceed.
		return nil
	case <-ctx.Done():
		// Context was canceled before we could proceed.  Cancel the
		// reservation, which may permit other events to proceed sooner.
		//如果context被取消，则规划令牌
		r.Cancel()
		return ctx.Err()
	}
}

```


```go
// 计算到目标时间时会有多少token
// advance requires that lim.mu is held.（可以在函数名中加上locked，如advanceLocked）
func (lim *Limiter) advance(now time.Time) (newNow time.Time, newLast time.Time, newTokens float64) {
	last := lim.last
	if now.Before(last) {
		last = now
	}

	// Calculate the new number of tokens, due to time that passed.
	elapsed := now.Sub(last)
	delta := lim.limit.tokensFromDuration(elapsed)
	tokens := lim.tokens + delta
	if burst := float64(lim.burst); tokens > burst {
		tokens = burst
	}
	return now, last, tokens
}

// token和时间的换算
func (limit Limit) durationFromTokens(tokens float64) time.Duration {
	if limit <= 0 {
		return InfDuration
	}
	seconds := tokens / float64(limit)
	return time.Duration(float64(time.Second) * seconds)
}

// 时间和token的换算
func (limit Limit) tokensFromDuration(d time.Duration) float64 {
	if limit <= 0 {
		return 0
	}
	return d.Seconds() * float64(limit)
}

```






