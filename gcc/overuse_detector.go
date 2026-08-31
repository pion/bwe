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
	timeOverUsing        time.Duration
	overUsing            bool
	overUseCounter       int
	previousTrend        float64
	usage                usage
}

func newOveruseDetector() *overuseDetector {
	return &overuseDetector{
		overUseTimeThreshold: defaultOveruseTimeThreshold,
		delayThreshold:       defaultDelayThreshold,
		thresholdGain:        defaultThresholdGain,
		timeOverUsing:        0,
		overUsing:            false,
		overUseCounter:       0,
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
		if d.overUsing {
			d.timeOverUsing += sendDelta
		} else {
			d.timeOverUsing = sendDelta / 2
			d.overUsing = true
		}
		d.overUseCounter++
		if d.timeOverUsing > d.overUseTimeThreshold &&
			d.overUseCounter > 1 &&
			trend >= d.previousTrend {
			d.timeOverUsing = 0
			d.overUseCounter = 0
			d.usage = usageOver
		}
	case modifiedTrend < -d.delayThreshold:
		d.timeOverUsing = 0
		d.overUsing = false
		d.overUseCounter = 0
		d.usage = usageUnder
	default:
		d.timeOverUsing = 0
		d.overUsing = false
		d.overUseCounter = 0
		d.usage = usageNormal
	}
	d.previousTrend = trend

	return d.usage
}
