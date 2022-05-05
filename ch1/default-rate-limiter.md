# Default Rate Limiter

## RateLimiter

```go
type RateLimiter interface {
	// 执行当前对象，需要等待多久
	When(item interface{}) time.Duration
	// 不追踪该对象
	Forget(item interface{})
	// 该对象已经重试了多少次
	NumRequeues(item interface{}) int
}
```


## DefaultControllerRateLimiter

```go
// 默认controller限流器，
func DefaultControllerRateLimiter() RateLimiter {
    //取最大等待时间
	return NewMaxOfRateLimiter(
	    //失败指数等待
		NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
		// 整个限流器通用，只对重试起效
		&BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}
```

## BucketRateLimiter

```go
// 对于令牌桶的封装
type BucketRateLimiter struct {
	*rate.Limiter
}

var _ RateLimiter = &BucketRateLimiter{}

//按照令牌桶计算等待时间
func (r *BucketRateLimiter) When(item interface{}) time.Duration {
	return r.Limiter.Reserve().Delay()
}
//多少次重新入队
func (r *BucketRateLimiter) NumRequeues(item interface{}) int {
	return 0
}
//放弃对象
func (r *BucketRateLimiter) Forget(item interface{}) {
}
```

## MaxOfRateLimiter

```go
//最大限流器
type MaxOfRateLimiter struct {
	limiters []RateLimiter
}
//取最大等待时间
func (r *MaxOfRateLimiter) When(item interface{}) time.Duration {
	ret := time.Duration(0)
	for _, limiter := range r.limiters {
		curr := limiter.When(item)
		if curr > ret {
			ret = curr
		}
	}

	return ret
}

func NewMaxOfRateLimiter(limiters ...RateLimiter) RateLimiter {
	return &MaxOfRateLimiter{limiters: limiters}
}

func (r *MaxOfRateLimiter) NumRequeues(item interface{}) int {
	ret := 0
	//取所有最大重试次数中最大的
	for _, limiter := range r.limiters {
		curr := limiter.NumRequeues(item)
		if curr > ret {
			ret = curr
		}
	}

	return ret
}

func (r *MaxOfRateLimiter) Forget(item interface{}) {
    //所有都忽略
	for _, limiter := range r.limiters {
		limiter.Forget(item)
	}
}
```

## ItemExponentialFailureRateLimiter

```go
// 每次失败都会指数增加delay时间
type ItemExponentialFailureRateLimiter struct {
	failuresLock sync.Mutex //lock
	failures     map[interface{}]int //统计失败次数

	baseDelay time.Duration //失败delay
	maxDelay  time.Duration // 最大delay
}

//确保ItemExponentialFailureRateLimiter 实现了RateLimiter 接口
var _ RateLimiter = &ItemExponentialFailureRateLimiter{}

func NewItemExponentialFailureRateLimiter(baseDelay time.Duration, maxDelay time.Duration) RateLimiter {
	return &ItemExponentialFailureRateLimiter{
		failures:  map[interface{}]int{},
		baseDelay: baseDelay,
		maxDelay:  maxDelay,
	}
}
//默认的DefaultItemBasedRateLimiter， base时间为1毫秒，最大时间为1000s
func DefaultItemBasedRateLimiter() RateLimiter {
	return NewItemExponentialFailureRateLimiter(time.Millisecond, 1000*time.Second)
}

//每次when都会认为是一次尝试，基于尝试次数计算delay时间
func (r *ItemExponentialFailureRateLimiter) When(item interface{}) time.Duration {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

    //获取时间
	exp := r.failures[item]
	r.failures[item] = r.failures[item] + 1

	// 计算等待时间
	backoff := float64(r.baseDelay.Nanoseconds()) * math.Pow(2, float64(exp))
	if backoff > math.MaxInt64 {
		return r.maxDelay
	}

	calculated := time.Duration(backoff)
	if calculated > r.maxDelay {
		return r.maxDelay
	}

	return calculated
}

//获取失败次数
func (r *ItemExponentialFailureRateLimiter) NumRequeues(item interface{}) int {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

    //获取失败次数
	return r.failures[item]
}

//Forget 放弃该对象的记录
func (r *ItemExponentialFailureRateLimiter) Forget(item interface{}) {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	delete(r.failures, item)
}
```

## ItemFastSlowRateLimiter

```go

// ItemFastSlowRateLimiter does a quick retry for a certain number of attempts, then a slow retry after that
type ItemFastSlowRateLimiter struct {
	failuresLock sync.Mutex //锁
	failures     map[interface{}]int //失败次数统计

	maxFastAttempts int //快速队列，重试次数
	fastDelay       time.Duration //快速队列base
	slowDelay       time.Duration //慢速队列base
}

//确保ItemFastSlowRateLimiter 实现了RateLimiter 接口
var _ RateLimiter = &ItemFastSlowRateLimiter{}

func NewItemFastSlowRateLimiter(fastDelay, slowDelay time.Duration, maxFastAttempts int) RateLimiter {
	return &ItemFastSlowRateLimiter{
		failures:        map[interface{}]int{},
		fastDelay:       fastDelay,
		slowDelay:       slowDelay,
		maxFastAttempts: maxFastAttempts,
	}
}

func (r *ItemFastSlowRateLimiter) When(item interface{}) time.Duration {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	r.failures[item] = r.failures[item] + 1

    //如果失败次数小于快速最大重试次数，则返回快速时间
	if r.failures[item] <= r.maxFastAttempts {
		return r.fastDelay
	}

    //否则返回慢速时间
	return r.slowDelay
}

// 返回重试次数
func (r *ItemFastSlowRateLimiter) NumRequeues(item interface{}) int {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	return r.failures[item]
}

//删除对象记录
func (r *ItemFastSlowRateLimiter) Forget(item interface{}) {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	delete(r.failures, item)
}
```

## NewWithMaxWaitRateLimiter

```go
func NewWithMaxWaitRateLimiter(limiter RateLimiter, maxDelay time.Duration) RateLimiter {
	return &WithMaxWaitRateLimiter{limiter: limiter, maxDelay: maxDelay}
}

//基于最大limiter的时间和最大时间做比较
func (w WithMaxWaitRateLimiter) When(item interface{}) time.Duration {
	delay := w.limiter.When(item)
	if delay > w.maxDelay {
		return w.maxDelay
	}

	return delay
}
// 忽略对象记录
func (w WithMaxWaitRateLimiter) Forget(item interface{}) {
	w.limiter.Forget(item)
}

// 重新入队次数
func (w WithMaxWaitRateLimiter) NumRequeues(item interface{}) int {
	return w.limiter.NumRequeues(item)
}

```

