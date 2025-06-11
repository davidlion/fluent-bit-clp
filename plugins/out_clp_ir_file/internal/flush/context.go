package flush

import (
	"errors"
	"sync"
	"time"
)

type FlushContext struct {
	defaultLogLevel int
	hardDeltas      []time.Duration
	hardTimer       *time.Timer
	hardTimeout     time.Time
	softDelta       time.Duration
	softDeltas      []time.Duration
	softTimer       *time.Timer
	userCallback    func()
	mutex           sync.Mutex
}

func NewFlushContext(
	hardDeltas []time.Duration,
	softDeltas []time.Duration,
	defaultLogLevel int,
	userCallback func(),
) (*FlushContext, error) {
	if len(hardDeltas) == 0 {
		return nil, errors.New("hard flush deltas cannot be empty")
	}
	if len(softDeltas) == 0 {
		return nil, errors.New("soft flush deltas cannot be empty")
	}
	return &FlushContext{
		hardDeltas:      hardDeltas,
		hardTimer:       time.NewTimer(0),
		softDeltas:      softDeltas,
		softTimer:       time.NewTimer(0),
		defaultLogLevel: defaultLogLevel,
		userCallback:    userCallback,
	}, nil
}
