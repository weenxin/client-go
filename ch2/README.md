# Retry

```go
//10ms base， 不放大，0.1抖动
var DefaultRetry = wait.Backoff{
	Steps:    5,
	Duration: 10 * time.Millisecond,
	Factor:   1.0,
	Jitter:   0.1,
}

//10ms base， 每次放大5倍，0.1抖动 ，指数放大4次（或者重试4次）
var DefaultBackoff = wait.Backoff{
	Steps:    4,
	Duration: 10 * time.Millisecond,
	Factor:   5.0,
	Jitter:   0.1,
}

```

```go

func OnError(backoff wait.Backoff, retriable func(error) bool, fn func() error) error {
	var lastErr error

	err := wait.ExponentialBackoff(backoff, func() (bool, error) { //封装 type ConditionFunc func() (done bool, err error)
		err := fn()
		switch {
		case err == nil:
			return true, nil
		case retriable(err): //如果错误可以重试，则不算失败
			lastErr = err
			return false, nil
		default:
			return false, err
		}
	})
	if err == wait.ErrWaitTimeout {
		err = lastErr
	}
	return err
}

```


```go
func RetryOnConflict(backoff wait.Backoff, fn func() error) error {
	return OnError(backoff, errors.IsConflict, fn)
}
```

可以按照如下使用

```go
err := retry.RetryOnConflict()
```


