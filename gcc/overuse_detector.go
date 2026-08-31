// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"time"
)

const (
	defaultOveruseTimeThreshold = 5 * time.Millisecond
	defaultDelayThreshold       = 12.5
	defaultThresholdGain        = 4.0
	defaultKUp                  = 0.0087
	defaultKDown                = 0.039
	minNumDeltas                = 60
	maxAdaptOffset              = 15.0
	maxThresholdUpdateDelta     = 100 * time.Millisecond
	minDelayThreshold           = 6.0
	maxDelayThreshold           = 600.0
)

type overuseDetector struct {
	overuseTimeThreshold time.Duration
	delayThreshold       float64
	thresholdGain        float64
	kUp                  float64
	kDown                float64
	timeOverusing        time.Duration
	overusing            bool
	overuseCounter       int
	previousTrend        float64
	// modifiedTrend and comparedThreshold record the most recent comparison
	// for logging.
	modifiedTrend     float64
	comparedThreshold float64
	lastUpdate        time.Time
	hasLastUpdate     bool
	usage             usage
}

func newOveruseDetector() *overuseDetector {
	return &overuseDetector{
		overuseTimeThreshold: defaultOveruseTimeThreshold,
		delayThreshold:       defaultDelayThreshold,
		thresholdGain:        defaultThresholdGain,
		kUp:                  defaultKUp,
		kDown:                defaultKDown,
		timeOverusing:        0,
		overusing:            false,
		overuseCounter:       0,
		previousTrend:        0,
		modifiedTrend:        0,
		comparedThreshold:    defaultDelayThreshold,
		lastUpdate:           time.Time{},
		hasLastUpdate:        false,
		usage:                usageNormal,
	}
}

// update processes a trend estimate, where arrivalTime is the arrival time of
// the most recent group and sendDelta is the departure time difference between
// the two most recent arrival groups.
func (d *overuseDetector) update(
	arrivalTime time.Time,
	sendDelta time.Duration,
	trend float64,
	numDeltas int,
) usage {
	if numDeltas < 2 {
		d.modifiedTrend = 0
		d.comparedThreshold = d.delayThreshold
		d.usage = usageNormal

		return d.usage
	}

	modifiedTrend := math.Min(float64(numDeltas), minNumDeltas) * trend * d.thresholdGain
	d.modifiedTrend = modifiedTrend
	d.comparedThreshold = d.delayThreshold

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
	d.updateThreshold(modifiedTrend, arrivalTime)

	return d.usage
}

func (d *overuseDetector) updateThreshold(modifiedTrend float64, arrivalTime time.Time) {
	if !d.hasLastUpdate {
		d.lastUpdate = arrivalTime
		d.hasLastUpdate = true
	}

	if math.Abs(modifiedTrend) > d.delayThreshold+maxAdaptOffset {
		d.lastUpdate = arrivalTime

		return
	}

	k := d.kUp
	if math.Abs(modifiedTrend) < d.delayThreshold {
		k = d.kDown
	}
	delta := min(arrivalTime.Sub(d.lastUpdate), maxThresholdUpdateDelta)
	d.delayThreshold += k * (math.Abs(modifiedTrend) - d.delayThreshold) * durationToMs(delta)
	d.delayThreshold = min(max(d.delayThreshold, minDelayThreshold), maxDelayThreshold)
	d.lastUpdate = arrivalTime
}
