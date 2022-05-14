package flowcontrol

import (
	"context"
	"errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/clock"
	"testing"
	"time"
)

func TestGroup(t *testing.T) {
	//类似于WaitGroup，但是只支持执行一个函数
	g := wait.Group{}
	ctx ,cancel := context.WithTimeout(context.Background(),100*time.Millisecond)
	defer cancel()
	output := make(chan struct{})
	g.StartWithContext(ctx, func(ctx context.Context) {
		select {
		case output<- struct{}{}:
			t.Logf("success add item")
		case <-ctx.Done():
			t.Logf("context timeout")
		}

	})
	select {
	case <-ctx.Done():
		t.Log("context timeout")
	case <-output:
		t.Log("get item success")
	}
	//等待函数执行结束
	g.Wait()

}

func TestUntil(t *testing.T) {
	timer := clock.RealClock{}.NewTimer(100 * time.Millisecond)
	stop := make(chan struct{})
	count := 0

	//直到stop收到信号为止
	go wait.Until(func() {
		t.Logf("count is : %v" ,count)
		count ++
	},time.Millisecond * 10 ,stop )

	select {
	case <-timer.C():
		close(stop)
	}

	time.Sleep(time.Second)
}

func TestBackoffUntil(t *testing.T) {
	count := 0
	begin := time.Now()

	bm := wait.NewExponentialBackoffManager(
		1 * time.Millisecond, // 1ms 为base
		20 * time.Millisecond, //最大20ms
		time.Second, //1秒钟内没有backoff，则清空backoff时间
		2, //放大因子
		0.1, //抖动因子
		clock.RealClock{})

	stop := make(chan struct{})


	//相对于Util来说加入了backoff策略，对后端更加友好
	go wait.BackoffUntil(func() {
		t.Logf("count : %v , duration : %v ms", count,time.Now().Sub(begin).Round(time.Millisecond) )
		count ++
	},bm,
	false,//是否每隔一段时间执行操作【timer， proc， timer】，还是保证每隔间隔内执行操作即可【timer（proc），timer（proc）】
	// true : 【timer， proc， timer】
	//false :  【timer（proc），timer（proc）
	stop)



	time.Sleep(1 *time.Second)

	close(stop)

	t.Logf("stop BackoffUntil")
	time.Sleep(1 * time.Second)
}


func TestExponentialBackoff(t *testing.T){
	//指数级别的回退
	bf := wait.Backoff{
		Duration: time.Millisecond, //base 1ms
		Factor:   2, //每次放大2倍
		Jitter:   0.1, //0.1的抖动
		Steps:    5, //最多执行5次时间变更，后面就不变了
		Cap:      20 * time.Millisecond, //最多20ms
	}

	counter := 3
	begin := time.Now()
	err := wait.ExponentialBackoff(bf, func() (done bool, err error) {
		t.Logf("counter : %v, duration : %v", counter, time.Now().Sub(begin).Round(time.Millisecond))
		counter --
		if counter == 0 {
			return true ,nil
		}
		return false,nil
	})

	if err != nil {
		t.Logf("run failed , err : %v", err)
	}else {
		t.Logf("run success, counter : %v",counter)
	}
}



func TestPool(t *testing.T){
	counter := 3
	begin := time.Now()
	//每5毫秒执行一次，最多执行一秒钟，执行成功或者遇到错误为止
	err := wait.Poll( 5 * time.Millisecond , time.Second, func() (done bool, err error) {
		t.Logf("counter : %v, duration : %v", counter, time.Now().Sub(begin).Round(time.Millisecond))
		counter --
		if counter == 0 {
			return true ,nil
		}
		return false,nil
	})

	if err != nil {
		t.Logf("run failed , err : %v", err)
	}else {
		t.Logf("run success, counter : %v",counter)
	}
}

func TestRetry(t *testing.T) {
	count := 2

	err := retry.OnError(retry.DefaultBackoff,
		//判断错误是否可重试
		func(err error) bool {
			return true
		},
		//执行函数
		func() error {
			if count == 0 {
				return nil
			}
			count --
			return errors.New("not zero")
		},
	)

	if err != nil {
		t.Logf("failed to set count to zero err: %v",err)
	}else {
		t.Logf("success set count to zero count : %v",count)
	}
}












