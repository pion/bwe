// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"time"
)

const (
	defaultOveruseTimeThreshold = 5 * time.Millisecond
	defaultDelayThreshold       = 1.5
)

type overuseDetector struct {
	overUseTimeThreshold time.Duration
	delayThreshold       float64
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
		lastUpdate:           time.Time{},
		firstOverUse:         time.Time{},
		overUseCounter:       0,
		previousTrend:        0,
		usage:                usageNormal,
	}
}

func (d *overuseDetector) update(ts time.Time, trend float64) usage {
	if d.lastUpdate.IsZero() {
		d.lastUpdate = ts
	}

	switch {
	case trend > d.delayThreshold:
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
	case trend < -d.delayThreshold:
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
