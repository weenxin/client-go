package workqueue

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"golang.org/x/time/rate"
	"time"
)

func TestAllow(t *testing.T) {

	lim := rate.NewLimiter(10,100)

	lim.AllowN(time.Now(),100)

	numOk := int32(0)

	timer := time.NewTimer(1 * time.Second)

Looper:	for {
		select {
		case <-timer.C:
			break Looper
		default:
			if lim.Allow(){
				atomic.AddInt32(&numOk, 1)
				time.Sleep(1 * time.Millisecond)
			}
		}
	}

	t.Logf("rate.NewLimiter(10,100) will produce %d num ok", numOk)
}

func TestReserve(t *testing.T) {
	lim := rate.NewLimiter(10,100)
	time.Sleep(100 * time.Millisecond)
	r := lim.ReserveN(time.Now(),110)
	t.Logf("rate.NewLimiter(10,100) request more than bust will return : %v" , r.OK())


	r = lim.ReserveN(time.Now(),100)
	t.Logf("ReserveN empty will return : %v" , r.DelayFrom(time.Now()))
	r = lim.ReserveN(time.Now(),10)
	t.Logf("ReserveN empty will return : %v" , r.DelayFrom(time.Now()))

	r2 := lim.ReserveN(time.Now(),5)
	t.Logf("ReserveN empty 2 will return : %v" , r2.DelayFrom(time.Now()))

	r.Cancel()
	r3 := lim.ReserveN(time.Now(),5)
	t.Logf("ReserveN empty 3 will return : %v" , r3.DelayFrom(time.Now()))
}

func TestWait(t *testing.T) {
	lim := rate.NewLimiter(10,100)

	numOk := int32(0)

	ctx , cancel := context.WithTimeout(context.Background(),1 * time.Second )
	defer cancel()

	numProc := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	wg.Add(numProc)
	for i := 0 ; i < numProc ; i ++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					if err := lim.Wait(context.Background()) ; err != nil {
						t.Logf("wait limiter failed , err : %s",err)
					}else {
						atomic.AddInt32(&numOk,1 )
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()

	t.Logf("after 1 second , %d works numOk is : %d", numProc ,numOk )

}
