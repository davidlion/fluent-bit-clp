package timeout

import (
	"errors"
	"log"
	"math"
	"sync"
	"time"
)

type Manager interface {
	Update(level int, timestamp time.Time)
}

type manager struct {
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

func NewManager(
	hardDeltas []time.Duration,
	softDeltas []time.Duration,
	defaultLogLevel int,
	userCallback func(),
) (Manager, error) {
	if len(hardDeltas) == 0 {
		return nil, errors.New("hard timeout deltas cannot be empty")
	}
	if len(softDeltas) == 0 {
		return nil, errors.New("soft timeout deltas cannot be empty")
	}
	return &manager{
		hardDeltas:      hardDeltas,
		hardTimer:       time.NewTimer(0),
		softDeltas:      softDeltas,
		softTimer:       time.NewTimer(0),
		defaultLogLevel: defaultLogLevel,
		userCallback:    userCallback,
	}, nil
}

func (m *manager) callback() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.hardTimer.Stop()
	m.softTimer.Stop()
	m.hardTimeout = time.Time{}
	m.softDelta = time.Duration(math.MaxInt64)
	m.userCallback()
}

func (m *manager) Update(level int, timestamp time.Time) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var hardDelta time.Duration
	if level < len(m.hardDeltas) {
		hardDelta = m.hardDeltas[level]
	} else {
		hardDelta = m.hardDeltas[m.defaultLogLevel]
		log.Printf(
			"[warn] no hard timeout delta found for log level %v; defaulting to level %v (%v).",
			level,
			m.defaultLogLevel,
			hardDelta,
		)
	}
	nextHardTimeout := timestamp.Add(hardDelta)
	if nextHardTimeout.IsZero() || nextHardTimeout.Before(m.hardTimeout) {
		m.hardTimer.Stop()
		m.hardTimer = time.AfterFunc(
			time.Until(nextHardTimeout),
			m.callback,
		)
		m.hardTimeout = nextHardTimeout
	}

	var softDelta time.Duration
	if level < len(m.softDeltas) {
		softDelta = m.softDeltas[level]
	} else {
		softDelta = m.softDeltas[m.defaultLogLevel]
		log.Printf(
			"[warn] no soft timeout delta found for log level %v; defaulting to level %v (%v).",
			level,
			m.defaultLogLevel,
			softDelta,
		)
	}
	if softDelta < m.softDelta {
		m.softDelta = softDelta
	}
	nextSoftTimeout := timestamp.Add(softDelta)
	m.softTimer.Stop()
	m.softTimer = time.AfterFunc(
		time.Until(nextSoftTimeout),
		m.callback,
	)
}
