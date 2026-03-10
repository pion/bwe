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
	targetRate       float64
}

func newDelayRateController(initialRate int, logger logging.LeveledLogger) *delayRateController {
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
		targetRate:       float64(initialRate),
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
		if c.canIncreaseMultiplicatively(deliveredRate) {
			window := ts.Sub(c.lastUpdate)
			c.targetRate = max(c.targetRate, c.multiplicativeIncrease(c.targetRate, window))
		} else {
			bitsPerFrame := c.targetRate / 30.0
			packetsPerFrame := math.Ceil(bitsPerFrame / (1200 * 8))
			expectedPacketSizeBits := bitsPerFrame / packetsPerFrame
			c.targetRate = c.additiveIncrease(c.targetRate, int(expectedPacketSizeBits), rtt)
		}
		c.targetRate = min(c.targetRate, 1.5*deliveredRate)
	}
	if c.state == stateDecrease {
		c.lastDecreaseRate.update(float64(deliveryRate))
		c.targetRate = c.decreaseFactor * float64(deliveryRate)
	}
	c.lastUpdate = ts

	return int(c.targetRate)
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

func (c delayRateController) multiplicativeIncrease(rate float64, window time.Duration) float64 {
	exponent := min(window.Seconds(), 1.0)
	eta := math.Pow(1.08, exponent)
	target := eta * rate

	return target
}

func (c *delayRateController) additiveIncrease(rate float64, expectedPacketSizeBits int, window time.Duration) float64 {
	alpha := 0.5 * min(window.Seconds(), 1.0)
	target := rate + max(1000, alpha*float64(expectedPacketSizeBits))

	return target
}
