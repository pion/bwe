// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"time"
)

const (
	defaultOveruseTimeThreshold = 5 * time.Millisecond
	defaultDelayThreshold       = 1.5
	defaultThresholdGain        = 4.0
	minNumDeltas                = 60
)

type overuseDetector struct {
	overUseTimeThreshold time.Duration
	delayThreshold       float64
	thresholdGain        float64
	lastUpdate           time.Time
	firstOverUse         time.Time
	overUseCounter       int
	previousTrend        float64
	usage                usage
}

func newOveruseDetector() *overuseDetector {
	return &overuseDetector{
		overUseTimeThreshold: defaultOveruseTimeThreshold,
		delayThreshold:       defaultDelayThreshold,
		thresholdGain:        defaultThresholdGain,
		lastUpdate:           time.Time{},
		firstOverUse:         time.Time{},
		overUseCounter:       0,
		previousTrend:        0,
		usage:                usageNormal,
	}
}

func (d *overuseDetector) update(ts time.Time, trend float64, numDeltas int) usage {
	if d.lastUpdate.IsZero() {
		d.lastUpdate = ts
	}

	modifiedTrend := math.Min(float64(numDeltas), minNumDeltas) * trend * d.thresholdGain

	switch {
	case modifiedTrend > d.delayThreshold:
		if d.firstOverUse.IsZero() {
			delta := ts.Sub(d.lastUpdate)
			d.firstOverUse = ts.Add(-delta / 2)
		}
		d.overUseCounter++
		if ts.Sub(d.firstOverUse) > d.overUseTimeThreshold &&
			d.overUseCounter > 1 &&
			trend >= d.previousTrend {
			d.firstOverUse = time.Time{}
			d.overUseCounter = 0
			d.usage = usageOver
		}
	case modifiedTrend < -d.delayThreshold:
		d.firstOverUse = time.Time{}
		d.overUseCounter = 0
		d.usage = usageUnder
	default:
		d.firstOverUse = time.Time{}
		d.overUseCounter = 0
		d.usage = usageNormal
	}
	d.previousTrend = trend
	d.lastUpdate = ts

	return d.usage
}
