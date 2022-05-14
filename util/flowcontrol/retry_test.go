package flowcontrol

import (
	"context"
	"errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/clock"
	"math/rand"
	"testing"
	"time"
)

func TestGroup(t *testing.T) {
	g := wait.Group{}
	ctx ,cancel := context.WithTimeout(context.Background(),100*time.Millisecond)
	defer cancel()
	output := make(chan struct{})
	g.StartWithContext(ctx, func(ctx context.Context) {
		time.Sleep(time.Duration(rand.Intn(100))*time.Millisecond)
		output<- struct{}{}
	})
	select {
	case <-ctx.Done():
		t.Log("context timeout")
	case <-output:
		t.Log("output success")
	}

}

func TestUntil(t *testing.T) {
	timer := clock.RealClock{}.NewTimer(100 * time.Millisecond)
	stop := make(chan struct{})
	count := 0
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
		20 * time.Millisecond,
		time.Second,
		2,
		0.1,
		clock.RealClock{})

	stop := make(chan struct{})


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
	bf := wait.Backoff{
		Duration: time.Millisecond, //base 1ms
		Factor:   2, //每次放大2倍
		Jitter:   0.1, //0.1的抖动
		Steps:    5, //最多执行5次
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
	err := retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return true
	}, func() error {
		if count == 0 {
			return nil
		}
		count --
		return errors.New("not zero")
	})

	if err != nil {
		t.Logf("failed to set count to zero err: %v",err)
	}else {
		t.Logf("success set count to zero count : %v",count)
	}
}












