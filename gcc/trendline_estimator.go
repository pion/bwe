// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"time"
)

type trendlineEstimtatorOption func(*trendlineEstimator)

func trendlineEstimatorSmoothingCoeff(coeff float64) trendlineEstimtatorOption {
	return func(te *trendlineEstimator) {
		te.smoothingCoeff = coeff
	}
}

func trendlineEstimatorWindowSize(size int) trendlineEstimtatorOption {
	return func(te *trendlineEstimator) {
		te.windowSize = size
	}
}

type packetDelay struct {
	arrivalTimeMS   float64
	smoothedDelayMS float64
}

type trendlineEstimator struct {
	smoothingCoeff float64
	windowSize     int

	firstArrival     time.Time
	accumulatedDelay time.Duration
	smoothedDelayMs  float64

	history []packetDelay
}

func newTrendlineEstimator(options ...trendlineEstimtatorOption) *trendlineEstimator {
	te := &trendlineEstimator{
		smoothingCoeff:   0.8,
		windowSize:       10,
		firstArrival:     time.Time{},
		accumulatedDelay: 0,
		smoothedDelayMs:  0,
		history:          []packetDelay{},
	}
	for _, opt := range options {
		opt(te)
	}

	return te
}

func (e *trendlineEstimator) update(arrivalTime time.Time, interGroupDelay time.Duration) float64 {
	e.accumulatedDelay += interGroupDelay
	e.smoothedDelayMs = e.smoothingCoeff*e.smoothedDelayMs +
		(1-e.smoothingCoeff)*float64(e.accumulatedDelay.Milliseconds())

	if e.firstArrival.IsZero() {
		e.firstArrival = arrivalTime
	}

	timeSinceFirst := arrivalTime.Sub(e.firstArrival).Milliseconds()
	e.history = append(e.history, packetDelay{
		arrivalTimeMS:   float64(timeSinceFirst),
		smoothedDelayMS: e.smoothedDelayMs,
	})
	if len(e.history) > e.windowSize {
		e.history = e.history[1:]
	}

	trend, ok := fitSlope(e.history)
	if !ok {
		return 0
	}

	return trend
}

func fitSlope(packets []packetDelay) (float64, bool) {
	sumX := 0.0
	sumY := 0.0

	for _, p := range packets {
		sumX += p.arrivalTimeMS
		sumY += p.smoothedDelayMS
	}
	avgX := sumX / float64(len(packets))
	avgY := sumY / float64(len(packets))

	numerator := 0.0
	denominator := 0.0

	for _, p := range packets {
		x := p.arrivalTimeMS
		y := p.smoothedDelayMS
		numerator += (x - avgX) * (y - avgY)
		denominator += (x - avgX) * (x - avgX)
	}

	if denominator == 0 {
		return 0, false
	}

	return numerator / denominator, true
}
