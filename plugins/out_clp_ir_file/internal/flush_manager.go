package internal

import (
	"log"
	"math"
	"time"
)

// FlushManager allows updating flush strategy based on log level and timestamp.
type FlushManager interface {
	Update(level int, timestamp time.Time)
}

// Callback is called when a flush timer fires.
func (m *FlushContext) Callback() {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	m.stopAndClearTimers()
	m.hardTimeout = time.Time{}
	m.softDelta = time.Duration(math.MaxInt64)
	m.userCallback()
}

// Update schedules new hard/soft flush timers based on the log level and timestamp.
func (m *FlushContext) Update(level int, timestamp time.Time) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	hardDelta := getDeltaSafe(level, m.hardDeltas, m.defaultLogLevel, "hard")
	nextHardTimeout := timestamp.Add(hardDelta)
	if nextHardTimeout.IsZero() || nextHardTimeout.Before(m.hardTimeout) {
		replaceTimer(&m.HardTimer, time.Until(nextHardTimeout), m.Callback)
		m.hardTimeout = nextHardTimeout
	}

	softDelta := getDeltaSafe(level, m.softDeltas, m.defaultLogLevel, "soft")
	if softDelta < m.softDelta {
		m.softDelta = softDelta
	}
	nextSoftTimeout := timestamp.Add(softDelta)
	replaceTimer(&m.SoftTimer, time.Until(nextSoftTimeout), m.Callback)
}

// getDeltaSafe returns the delta for the level, or defaults and logs a warning.
func getDeltaSafe(level int, deltas []time.Duration, defaultLevel int, label string,
) time.Duration {
	if level >= 0 && level < len(deltas) {
		return deltas[level]
	}
	// Defensive: check defaultLevel is in range
	defaultDelta := time.Second
	if defaultLevel >= 0 && defaultLevel < len(deltas) {
		defaultDelta = deltas[defaultLevel]
	}
	log.Printf(
		"[warn] No %s flush delta found for log level %v; defaulting to level %v (%v).",
		label, level, defaultLevel, defaultDelta,
	)
	return defaultDelta
}

// stopAndClearTimers stops and clears both hard and soft timers.
func (m *FlushContext) stopAndClearTimers() {
	stopTimer(&m.HardTimer)
	stopTimer(&m.SoftTimer)
}

// stopTimer safely stops a timer if it is not nil.
func stopTimer(timer **time.Timer) {
	if *timer != nil {
		(*timer).Stop()
		*timer = nil
	}
}

// replaceTimer stops the old timer and creates a new one.
func replaceTimer(timer **time.Timer, duration time.Duration, callback func()) {
	stopTimer(timer)
	*timer = time.AfterFunc(duration, callback)
}
