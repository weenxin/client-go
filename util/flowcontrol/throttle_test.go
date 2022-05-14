package flowcontrol

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"k8s.io/client-go/util/flowcontrol"
	"time"
)

//悲观锁
func TestThrottle(t *testing.T){
	r := flowcontrol.NewTokenBucketRateLimiter(100, 5)
	count := int32(0)
	var wg sync.WaitGroup
	ctx,cancel := context.WithCancel(context.Background())
	process := runtime.GOMAXPROCS(0)
	wg.Add(process)
	for i := 0 ; i < process ; i ++ {
		go func() {
			defer wg.Done()
			for {
				err := r.Wait(ctx)
				if err != nil {
					return
				}else {
					atomic.AddInt32(&count,1)
				}
			}

		}()
	}
	//r.Stop()//这个好像没有啥用
	time.Sleep(time.Second)
	cancel()
	wg.Wait()

	t.Logf("TestThrottle counter is : %v", count)
}

//乐观用法
func TestThrottlePassive(t *testing.T){
	r := flowcontrol.NewTokenBucketRateLimiter(100, 5)
	count := int32(0)
	var wg sync.WaitGroup
	ctx,cancel := context.WithCancel(context.Background())
	process := runtime.GOMAXPROCS(0)
	wg.Add(process)
	for i := 0 ; i < process ; i ++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <- ctx.Done():
					return
				default:
					if r.TryAccept() {
						atomic.AddInt32(&count,1)
					}
				}
			}

		}()
	}
	//r.Stop()//这个好像没有啥用
	time.Sleep(time.Second)
	cancel()
	wg.Wait()

	t.Logf("TestPassiveThrottle counter is : %v", count)
}
