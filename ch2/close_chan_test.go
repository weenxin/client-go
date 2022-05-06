package ch2

import (
	"errors"
	"k8s.io/client-go/util/retry"
	"testing"
)

func TestCloseChannel(t *testing.T) {
	t.Skipf("will panic")

	ch := make(chan struct{})
	close(ch)

	select {
	case ch<- struct{}{}:
		t.Logf("Adding a item to a closed chan")
	default:
		t.Logf("do nothging")
	}

}

func TestRetry(t *testing.T) {
	count := 2
	retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return true
	}, func() error {
		if count == 0 {
			t.Logf("zero OK")
			return nil
		}
		count --
		t.Logf("not zero")
		return errors.New("not zero")
	})
}
