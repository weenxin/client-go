package flowcontrol

import (
	"testing"
	testclock "k8s.io/utils/clock/testing"
	"k8s.io/client-go/util/flowcontrol"
	"time"
)

func TestFlowControlBackoff(t *testing.T) {
	id := "test1"
	tc := testclock.NewFakeClock(time.Now())
	b := flowcontrol.NewFakeBackOff(time.Millisecond,time.Second,tc)

	//第一次做一件事情
	duration := b.Get(id)
	t.Logf("first time : %v ", duration)
	//失败了
	b.Next(id,time.Now())
	//过了一段时间，重试下
	tc.Step(duration)
	duration = b.Get(id)
	t.Logf("second time , retry : %v ", duration)


	//又失败了
	b.Next(id,time.Now())
	//过了一段时间继续重试
	tc.Step(duration)
	duration = b.Get(id)
	t.Logf("third time , retry  : %v ", duration)

	//成功了
	b.Reset(id)

	//过了一段时间需要继续做这个事情
	duration = b.Get(id)
	t.Logf("first time  : %v ", duration)
	//失败，则继续累加
	b.Next(id,time.Now())
	duration = b.Get(id)
	t.Logf("second time , retry  : %v ", duration)



	timer := tc.NewTimer(3 * time.Second)

	go func() {
		select {
		case <- timer.C():
			//需要手动调用gc，才会删除map中的条目
			b.GC()
		}
	}()


	t.Logf("IsInBackOffSince : %v", b.IsInBackOffSince(id,tc.Now()))
	//最后一次更新时间（调用next时间）到eventTime，是否小于backoff时间窗口
	t.Logf("IsInBackOffSinceUpdate : %v", b.IsInBackOffSinceUpdate(id,tc.Now()))
	begin := tc.Now()

	tc.Step(3*time.Millisecond)
	//eventTime到当前时间，是否小于backoff时间窗口
	t.Logf("IsInBackOffSince : %v", b.IsInBackOffSince(id,begin))
	//最后一次更新时间（调用next时间）到eventTime，是否小于backoff时间窗口
	t.Logf("IsInBackOffSinceUpdate : %v", b.IsInBackOffSinceUpdate(id,tc.Now()) )


	tc.Step( 3 * time.Second ) //由于长时间没有重试则会将数据
	time.Sleep(1 * time.Millisecond)


	duration = b.Get(id)
	t.Logf("after gc dration is reset , %v",duration)
}
