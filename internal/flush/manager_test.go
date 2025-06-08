package flush

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
			log.Printf("flush occurred")
		},
	)
	if nil != err {
		t.Fatalf("failed to create flush manager")
	}

	timeoutManager.Update(0, time.Now())

	time.Sleep(1 * time.Second)
}
