// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"time"
)

const (
	kU = 0.01
	kD = 0.00018

	minNumDeltas = 60
)

type overuseDetector struct {
	adaptiveThreshold    bool
	thresholdGain        float64
	overUseTimeThreshold time.Duration
	delayThreshold       float64
	lastEstimate         time.Duration
	lastUpdate           time.Time
	firstOverUse         time.Time
	inOveruse            bool
	lastUsage            usage
}

func newOveruseDetector(adaptive bool) *overuseDetector {
	return &overuseDetector{
		adaptiveThreshold:    adaptive,
		thresholdGain:        4.0,
		overUseTimeThreshold: 5 * time.Millisecond,
		delayThreshold:       6,
		lastEstimate:         0,
		lastUpdate:           time.Time{},
		firstOverUse:         time.Time{},
		inOveruse:            false,
		lastUsage:            0,
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

	switch {
	case modifiedTrend > d.delayThreshold:
		if d.firstOverUse.IsZero() {
			// TODO: Set firstOverUse to the average of now and last.
			delta := ts.Sub(d.lastUpdate)
			d.firstOverUse = ts.Add(-delta / 2)
		}
		if ts.Sub(d.firstOverUse) > d.overUseTimeThreshold {
			d.firstOverUse = time.Time{}
			d.lastUsage = usageOver
		}
	case modifiedTrend < -d.delayThreshold:
		d.firstOverUse = time.Time{}
		d.lastUsage = usageUnder
	default:
		d.firstOverUse = time.Time{}
		d.lastUsage = usageNormal
	}
	if d.adaptiveThreshold {
		d.adaptThreshold(ts, modifiedTrend)
	}

	d.lastUpdate = ts
	return d.lastUsage
}

func (d *overuseDetector) adaptThreshold(ts time.Time, modifiedTrend float64) {
	if math.Abs(modifiedTrend) > d.delayThreshold+15 {
		d.lastUpdate = ts
		return
	}
	k := kU
	if math.Abs(modifiedTrend) < d.delayThreshold {
		k = kD
	}
	delta := min(ts.Sub(d.lastUpdate), 100*time.Millisecond)
	d.delayThreshold += k * (math.Abs(modifiedTrend) - d.delayThreshold) * float64(delta.Milliseconds())
	d.delayThreshold = max(min(d.delayThreshold, 600.0), 6.0)
}
