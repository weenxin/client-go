# Rate limiting queue

## Interface

```go

//有限流的延时队列
type RateLimitingInterface interface {
	DelayingInterface

	// AddRateLimited  当限流允许时将对象加入到工作队列中
	AddRateLimited(item interface{})


	// Forget 限流器忽略该对象，但是需要手动调用队列的Done才能从队列中忽略该对象
	Forget(item interface{})

	// 重新入队次数
	NumRequeues(item interface{}) int
}

```


## 新建

```go
// NewRateLimitingQueue 基于限流器构建一个延时队列
func NewRateLimitingQueue(rateLimiter RateLimiter) RateLimitingInterface {
	return &rateLimitingType{
		DelayingInterface: NewDelayingQueue(),
		rateLimiter:       rateLimiter,
	}
}

func NewNamedRateLimitingQueue(rateLimiter RateLimiter, name string) RateLimitingInterface {
	return &rateLimitingType{
		DelayingInterface: NewNamedDelayingQueue(name),
		rateLimiter:       rateLimiter,
	}
}
```


## 方法

```go
//  限流延时队列实现
type rateLimitingType struct {
	DelayingInterface

	rateLimiter RateLimiter
}

// 实现接口
func (q *rateLimitingType) AddRateLimited(item interface{}) {
	q.DelayingInterface.AddAfter(item, q.rateLimiter.When(item))
}

//实现接口
func (q *rateLimitingType) NumRequeues(item interface{}) int {
	return q.rateLimiter.NumRequeues(item)
}
//实现接口
func (q *rateLimitingType) Forget(item interface{}) {
	q.rateLimiter.Forget(item)
}
```
