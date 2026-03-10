// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"time"

	"github.com/pion/logging"
)

const (
	defaultDecreaseFactor = 0.85
)

type delayRateController struct {
	log              logging.LeveledLogger
	decreaseFactor   float64
	arrivalGroups    *arrivalGroupAccumulator
	lastArrivalGroup arrivalGroup
	trend            *trendlineEstimator
	overuse          *overuseDetector
	samples          int
	usage            usage
	state            state
	lastDecreaseRate *ewma
	lastUpdate       time.Time
	targetRate       int
	minTarget        int
	maxTarget        int
}

func newDelayRateController(initialRate, minRate, maxRate int, logger logging.LeveledLogger) *delayRateController {
	return &delayRateController{
		log:              logger,
		decreaseFactor:   defaultDecreaseFactor,
		arrivalGroups:    newArrivalGroupAccumulator(),
		lastArrivalGroup: []arrivalGroupItem{},
		trend:            newTrendlineEstimator(),
		overuse:          newOveruseDetector(false),
		usage:            0,
		samples:          0,
		state:            0,
		lastDecreaseRate: newEWMA(0.95),
		targetRate:       initialRate,
		minTarget:        minRate,
		maxTarget:        maxRate,
	}
}

func (c *delayRateController) onPacketAcked(sequenceNumber uint64, size int, departure, arrival time.Time) {
	next := c.arrivalGroups.onPacketAcked(
		sequenceNumber,
		size,
		departure,
		arrival,
	)
	if next == nil {
		return
	}
	if len(next) == 0 {
		// ignore empty groups, should never occur
		return
	}
	if len(c.lastArrivalGroup) == 0 {
		c.lastArrivalGroup = next

		return
	}

	interArrivalTime := next[len(next)-1].Arrival.Sub(c.lastArrivalGroup[len(c.lastArrivalGroup)-1].Arrival)
	interDepartureTime := next[len(next)-1].Departure.Sub(c.lastArrivalGroup[len(c.lastArrivalGroup)-1].Departure)
	interGroupDelay := interArrivalTime - interDepartureTime

	trend := c.trend.update(arrival, interGroupDelay)
	c.samples++
	c.usage = c.overuse.update(arrival, trend, c.samples)
	c.lastArrivalGroup = next

	c.log.Tracef(
		"ts=%v.%06d, seq=%v, interArrivalTime=%v, interDepartureTime=%v, interGroupDelay=%v, estimate=%f, threshold=%f, usage=%v, state=%v", // nolint
		c.lastArrivalGroup[0].Departure.UTC().Format("2006/01/02 15:04:05"),
		c.lastArrivalGroup[0].Departure.UTC().Nanosecond()/1e3,
		next[0].SequenceNumber,
		interArrivalTime.Microseconds(),
		interDepartureTime.Microseconds(),
		interGroupDelay.Microseconds(),
		trend,
		c.overuse.delayThreshold,
		int(c.usage),
		int(c.state),
	)
}

func (c *delayRateController) update(ts time.Time, deliveryRate int, rtt time.Duration) int {
	deliveredRate := float64(deliveryRate)
	c.state = c.state.transition(c.usage)
	if c.state == stateIncrease {
		window := ts.Sub(c.lastUpdate)
		if c.canIncreaseMultiplicatively(deliveredRate) {
			c.targetRate = max(c.targetRate, multiplicativeIncrease(c.targetRate, window))
		} else {
			c.targetRate = additiveIncrease(c.targetRate, rtt, window)
		}
		c.targetRate = min(c.targetRate, int(1.5*deliveredRate))
	}
	if c.state == stateDecrease {
		c.lastDecreaseRate.update(float64(deliveryRate))
		c.targetRate = int(c.decreaseFactor * float64(deliveryRate))
	}
	c.lastUpdate = ts

	c.targetRate = max(c.targetRate, c.minTarget)
	c.targetRate = min(c.targetRate, c.maxTarget)

	return c.targetRate
}

func (c *delayRateController) canIncreaseMultiplicatively(deliveredRate float64) bool {
	avg := c.lastDecreaseRate.avg()
	if avg == 0 {
		return true
	}
	stdDev := math.Sqrt(c.lastDecreaseRate.varr())
	lower := avg - 3*stdDev
	upper := avg + 3*stdDev

	return deliveredRate < lower || deliveredRate > upper
}

func multiplicativeIncrease(rate int, window time.Duration) int {
	exponent := min(window.Seconds(), 1.0)
	eta := math.Pow(1.08, exponent)

	return int(eta * float64(rate))
}

func additiveIncrease(rate int, rtt, window time.Duration) int {
	responseTime := 100 + rtt.Milliseconds()
	alpha := 0.5 * min(float64(window.Milliseconds())/float64(responseTime), 1.0)
	bitsPerFrame := float64(rate) / 30.0
	packetsPerFrame := math.Ceil(bitsPerFrame / (1200 * 8))
	expectedPacketSizeBits := bitsPerFrame / packetsPerFrame

	return rate + max(1000, int(alpha*float64(expectedPacketSizeBits)))
}
