// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"time"
)

const (
	kUp   = 0.0087
	kDown = 0.039

	minNumDeltas = 60
)

const (
	defaultThresholdGain        = 4.0
	defaultOveruseTimeThreshold = 5 * time.Millisecond
)

type overuseDetector struct {
	adaptiveThreshold    bool
	thresholdGain        float64
	overUseTimeThreshold time.Duration
	delayThreshold       float64
	lastUpdate           time.Time
	firstOverUse         time.Time
	overUseCounter       int
	previousTrend        float64
}

func newOveruseDetector(adaptive bool) *overuseDetector {
	return &overuseDetector{
		adaptiveThreshold:    adaptive,
		thresholdGain:        defaultThresholdGain,
		overUseTimeThreshold: defaultOveruseTimeThreshold,
		delayThreshold:       6,
		lastUpdate:           time.Time{},
		firstOverUse:         time.Time{},
		overUseCounter:       0,
		previousTrend:        0,
	}
}

func (d *overuseDetector) update(ts time.Time, trend float64, numDeltas int) usage {
	if d.lastUpdate.IsZero() {
		d.lastUpdate = ts
	}
	if numDeltas < 2 {
		return usageNormal
	}
	modifiedTrend := float64(min(numDeltas, minNumDeltas)) * trend * d.thresholdGain

	var currentUsage usage
	switch {
	case modifiedTrend > d.delayThreshold:
		if d.firstOverUse.IsZero() {
			delta := ts.Sub(d.lastUpdate)
			d.firstOverUse = ts.Add(-delta / 2)
		}
		d.overUseCounter++
		if ts.Sub(d.firstOverUse) > d.overUseTimeThreshold && d.overUseCounter > 1 && trend >= d.previousTrend {
			d.firstOverUse = time.Time{}
			d.overUseCounter = 0
			currentUsage = usageOver
		}
	case modifiedTrend < -d.delayThreshold:
		d.firstOverUse = time.Time{}
		d.overUseCounter = 0
		currentUsage = usageUnder
	default:
		d.firstOverUse = time.Time{}
		d.overUseCounter = 0
		currentUsage = usageNormal
	}
	d.adaptThreshold(ts, modifiedTrend)
	d.previousTrend = trend
	d.lastUpdate = ts

	return currentUsage
}

func (d *overuseDetector) adaptThreshold(ts time.Time, modifiedTrend float64) {
	if !d.adaptiveThreshold {
		return
	}
	if math.Abs(modifiedTrend) > d.delayThreshold+15 {
		return
	}
	k := kUp
	if math.Abs(modifiedTrend) < d.delayThreshold {
		k = kDown
	}
	delta := min(ts.Sub(d.lastUpdate), 100*time.Millisecond)
	d.delayThreshold += k * (math.Abs(modifiedTrend) - d.delayThreshold) * float64(delta.Milliseconds())
	d.delayThreshold = min(d.delayThreshold, 600.0)
	d.delayThreshold = max(d.delayThreshold, 6.0)
}
