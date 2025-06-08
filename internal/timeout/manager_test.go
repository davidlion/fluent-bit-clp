package timeout

import (
	"log"
	"testing"
	"time"
)

func TestManager(t *testing.T) {
	timeoutManager, err := NewManager(
		[]time.Duration{
			50 * time.Millisecond,
		},
		[]time.Duration{
			50 * time.Millisecond,
		},
		0,
		func() {
			log.Printf("timeout occurred")
		},
	)
	if nil != err {
		t.Fatalf("failed to create timeout manager")
	}

	timeoutManager.Update(0, time.Now())

	time.Sleep(1 * time.Second)
}
