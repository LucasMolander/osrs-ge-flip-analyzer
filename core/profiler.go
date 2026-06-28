package core

import (
	"sync"
	"time"
)

// PhaseStats holds performance statistics for a specific critical code path.
type PhaseStats struct {
	TotalTime   time.Duration `json:"total_time_ms"`
	Count       int64         `json:"count"`
	AverageTime time.Duration `json:"average_time_ms"`
	LastTime    time.Duration `json:"last_time_ms"`
}

// PhaseProfiler tracks the execution times of different labeled phases.
type PhaseProfiler struct {
	mu     sync.RWMutex
	phases map[string]*PhaseStats
	active map[string]time.Time
}

// GlobalProfiler is the singleton profiler used across the application.
var GlobalProfiler = &PhaseProfiler{
	phases: make(map[string]*PhaseStats),
	active: make(map[string]time.Time),
}

// Start marks the beginning of a profiling phase.
// It returns the start time, which can be passed to Stop() for concurrent safety,
// though the phaseName is sufficient if the phase is strictly sequential.
func (p *PhaseProfiler) Start(phaseName string) time.Time {
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active[phaseName] = now
	return now
}

// Stop marks the end of a profiling phase and records the elapsed time.
// Optional startTime can be passed if the phase was called concurrently.
func (p *PhaseProfiler) Stop(phaseName string, startTime ...time.Time) {
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()

	var start time.Time
	if len(startTime) > 0 {
		start = startTime[0]
	} else {
		var ok bool
		start, ok = p.active[phaseName]
		if !ok {
			return // Start was not called or already stopped
		}
	}

	elapsed := now.Sub(start)

	stats, ok := p.phases[phaseName]
	if !ok {
		stats = &PhaseStats{}
		p.phases[phaseName] = stats
	}

	stats.TotalTime += elapsed
	stats.Count++
	stats.AverageTime = time.Duration(int64(stats.TotalTime) / stats.Count)
	stats.LastTime = elapsed

	// Clear active if we relied on it
	if len(startTime) == 0 {
		delete(p.active, phaseName)
	}
}

// GetMetrics returns a snapshot of the current profiling metrics.
// Duration values are returned as milliseconds for JSON serialization.
func (p *PhaseProfiler) GetMetrics() map[string]map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]map[string]interface{})
	for name, stats := range p.phases {
		result[name] = map[string]interface{}{
			"total_time_ms":   float64(stats.TotalTime.Microseconds()) / 1000.0,
			"count":           stats.Count,
			"average_time_ms": float64(stats.AverageTime.Microseconds()) / 1000.0,
			"last_time_ms":    float64(stats.LastTime.Microseconds()) / 1000.0,
		}
	}
	return result
}
