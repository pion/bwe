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
	overuseTimeThreshold time.Duration
	delayThreshold       float64
	thresholdGain        float64
	timeOverusing        time.Duration
	overusing            bool
	overuseCounter       int
	previousTrend        float64
	usage                usage
}

func newOveruseDetector() *overuseDetector {
	return &overuseDetector{
		overuseTimeThreshold: defaultOveruseTimeThreshold,
		delayThreshold:       defaultDelayThreshold,
		thresholdGain:        defaultThresholdGain,
		timeOverusing:        0,
		overusing:            false,
		overuseCounter:       0,
		previousTrend:        0,
		usage:                usageNormal,
	}
}

// update processes a trend estimate, where sendDelta is the departure time
// difference between the two most recent arrival groups.
func (d *overuseDetector) update(sendDelta time.Duration, trend float64, numDeltas int) usage {
	if numDeltas < 2 {
		d.usage = usageNormal

		return d.usage
	}

	modifiedTrend := math.Min(float64(numDeltas), minNumDeltas) * trend * d.thresholdGain

	switch {
	case modifiedTrend > d.delayThreshold:
		if d.overusing {
			d.timeOverusing += sendDelta
		} else {
			d.timeOverusing = sendDelta / 2
			d.overusing = true
		}
		d.overuseCounter++
		if d.timeOverusing > d.overuseTimeThreshold &&
			d.overuseCounter > 1 &&
			trend >= d.previousTrend {
			d.timeOverusing = 0
			d.overuseCounter = 0
			d.usage = usageOver
		}
	case modifiedTrend < -d.delayThreshold:
		d.timeOverusing = 0
		d.overusing = false
		d.overuseCounter = 0
		d.usage = usageUnder
	default:
		d.timeOverusing = 0
		d.overusing = false
		d.overuseCounter = 0
		d.usage = usageNormal
	}
	d.previousTrend = trend

	return d.usage
}
