package gcc

import "time"

type packetDelay struct {
	arrivalTimeMS   float64
	smoothedDelayMS float64
}

type trendlineEstimator struct {
	smoothingCoeff float64
	windowSize     int

	firstArrival     time.Time
	accumulatedDelay time.Duration
	smoothedDelay    time.Duration

	history []packetDelay

	previousTrend float64
}

func newTrendlineEstimator() *trendlineEstimator {
	return &trendlineEstimator{
		smoothingCoeff:   0.9,
		windowSize:       20,
		firstArrival:     time.Time{},
		accumulatedDelay: 0,
		smoothedDelay:    0,
		history:          []packetDelay{},
		previousTrend:    0,
	}
}

func (e *trendlineEstimator) update(arrivalTime time.Time, interGroupDelay time.Duration) float64 {
	e.accumulatedDelay += interGroupDelay
	smoothedDelayMs := e.smoothingCoeff*float64(e.smoothedDelay.Milliseconds()) + (1-e.smoothingCoeff)*float64(e.accumulatedDelay.Milliseconds())
	e.smoothedDelay = time.Duration(smoothedDelayMs * float64(time.Millisecond))

	if e.firstArrival.IsZero() {
		e.firstArrival = arrivalTime
	}

	timeSinceFirst := arrivalTime.Sub(e.firstArrival).Milliseconds()
	e.history = append(e.history, packetDelay{
		arrivalTimeMS:   float64(timeSinceFirst),
		smoothedDelayMS: smoothedDelayMs,
	})
	if len(e.history) > e.windowSize {
		e.history = e.history[1:]
	}

	trend, ok := fitSlope(e.history)
	if !ok {
		trend = e.previousTrend
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
