package dashboardui

import "time"

const (
	throughputWindow   = 10 * time.Minute
	throughputCapacity = 200
)

type throughputSample struct {
	At         time.Time
	Generation *float64
}

type throughputSummary struct {
	Current *float64
	Average *float64
	Peak    *float64
}

type throughputHistory struct {
	samples []throughputSample
}

func copyThroughputValue(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func (history *throughputHistory) add(at time.Time, generation *float64) {
	history.prune(at)
	history.samples = append(history.samples, throughputSample{
		At:         at,
		Generation: copyThroughputValue(generation),
	})
	if len(history.samples) > throughputCapacity {
		history.samples = history.samples[len(history.samples)-throughputCapacity:]
	}
}

func (history *throughputHistory) reset() {
	history.samples = nil
}

func (history *throughputHistory) prune(now time.Time) {
	cutoff := now.Add(-throughputWindow)
	first := 0
	for first < len(history.samples) && history.samples[first].At.Before(cutoff) {
		first++
	}
	if first > 0 {
		history.samples = history.samples[first:]
	}
}

func (history *throughputHistory) snapshot(now time.Time) []throughputSample {
	history.prune(now)
	result := make([]throughputSample, len(history.samples))
	for index, sample := range history.samples {
		result[index] = throughputSample{At: sample.At, Generation: copyThroughputValue(sample.Generation)}
	}
	return result
}

func (history *throughputHistory) summary(now time.Time) throughputSummary {
	samples := history.snapshot(now)
	var summary throughputSummary
	if len(samples) == 0 {
		return summary
	}
	summary.Current = copyThroughputValue(samples[len(samples)-1].Generation)
	var total, peak float64
	count := 0
	for _, sample := range samples {
		if sample.Generation == nil {
			continue
		}
		value := *sample.Generation
		total += value
		if count == 0 || value > peak {
			peak = value
		}
		count++
	}
	if count > 0 {
		average := total / float64(count)
		summary.Average = &average
		summary.Peak = &peak
	}
	return summary
}
