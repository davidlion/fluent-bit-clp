package flush

import (
	"log"
	"math"
	"time"
)

func (m *FlushContext) callback() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.hardTimer.Stop()
	m.softTimer.Stop()
	m.hardTimeout = time.Time{}
	m.softDelta = time.Duration(math.MaxInt64)
	m.userCallback()
}

func (m *FlushContext) Update(level int, timestamp time.Time) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var hardDelta time.Duration
	if level < len(m.hardDeltas) {
		hardDelta = m.hardDeltas[level]
	} else {
		hardDelta = m.hardDeltas[m.defaultLogLevel]
		log.Printf(
			"[warn] No hard flush delta found for log level %v; defaulting to level %v (%v).",
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
			"[warn] No soft flush delta found for log level %v; defaulting to level %v (%v).",
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
