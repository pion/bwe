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

	// Weight of the newest sample in the decrease rate average, the complement
	// of the 0.95 smoothing factor recommended by draft-ietf-rmcat-gcc-02.
	defaultDecreaseRateAlpha = 0.05
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
	usageUpdated     bool
	state            state
	lastDecreaseRate *ewma
	lastUpdate       time.Time
	hasLastUpdate    bool
	targetRate       int
	minTarget        int
	maxTarget        int
}

//nolint:unparam // logger is supplied once the congestion controller creates this.
func newDelayRateController(initialRate, minRate, maxRate int, logger logging.LeveledLogger) *delayRateController {
	if logger == nil {
		logger = logging.NewDefaultLoggerFactory().NewLogger("gcc")
	}

	controller := &delayRateController{
		log:              logger,
		decreaseFactor:   defaultDecreaseFactor,
		arrivalGroups:    newArrivalGroupAccumulator(),
		trend:            newTrendlineEstimator(),
		overuse:          newOveruseDetector(),
		usage:            0,
		usageUpdated:     false,
		samples:          0,
		state:            0,
		lastDecreaseRate: newEWMA(defaultDecreaseRateAlpha),
		lastUpdate:       time.Time{},
		hasLastUpdate:    false,
		targetRate:       initialRate,
		minTarget:        minRate,
		// An inverted range would otherwise let clampTarget return a target
		// below the configured minimum.
		maxTarget: max(minRate, maxRate),
	}
	controller.clampTarget()

	return controller
}

// onPacketAcked processes a single acknowledged packet. When packets belong
// to the same feedback report, they must be passed in ascending arrival-time
// order.
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
	if c.lastArrivalGroup.empty() {
		c.lastArrivalGroup = *next

		return
	}

	groupDeparture := next.departure
	groupArrival := next.arrival
	lastGroupDeparture := c.lastArrivalGroup.departure
	lastGroupArrival := c.lastArrivalGroup.arrival

	interArrivalTime := groupArrival.Sub(lastGroupArrival)
	interDepartureTime := groupDeparture.Sub(lastGroupDeparture)

	// A non-positive delta means the two groups' departures were reordered
	// relative to their arrivals, which the trend estimator and overuse
	// detector aren't defined for. Drop this sample but still advance past
	// it, so the next group compares against current data.
	if interDepartureTime <= 0 {
		c.lastArrivalGroup = *next

		return
	}

	interGroupDelay := interArrivalTime - interDepartureTime

	trend := c.trend.update(groupArrival, interGroupDelay)
	c.samples++

	c.usage = c.overuse.update(groupArrival, interDepartureTime, trend, c.samples)
	c.usageUpdated = true
	c.lastArrivalGroup = *next

	c.log.Tracef(
		"ts=%v.%06d, seq=%v, interArrivalTime=%v, interDepartureTime=%v, interGroupDelay=%v, estimate=%f, modifiedTrend=%f, threshold=%f, usage=%v, state=%v", // nolint
		next.items[0].Departure.UTC().Format("2006/01/02 15:04:05"),
		next.items[0].Departure.UTC().Nanosecond()/1e3,
		next.items[0].SequenceNumber,
		interArrivalTime.Microseconds(),
		interDepartureTime.Microseconds(),
		interGroupDelay.Microseconds(),
		trend,
		c.overuse.modifiedTrend,
		c.overuse.comparedThreshold,
		c.usage,
		c.state,
	)
}

//nolint:unparam // rtt is varied once the congestion controller supplies it.
func (c *delayRateController) update(ts time.Time, deliveryRate int, rtt time.Duration) int {
	// The first update has no window to measure the rate change over, so it
	// only records the reference point and leaves the delay signal for the
	// next one.
	if !c.hasLastUpdate {
		c.hasLastUpdate = true
		c.lastUpdate = ts

		return c.clampTarget()
	}

	// The usage is only updated when an arrival group completes. Without a new
	// one there is no new delay signal, and running the state machine again
	// would act on the previous one twice.
	if !c.usageUpdated {
		return c.clampTarget()
	}

	// Both the increase cap and the decrease are relative to the rate that was
	// actually delivered. Without an estimate (too few acknowledgements in the
	// delivery rate window) there is nothing to base a new target on, so keep
	// the current target and leave the delay signal for the next update.
	if deliveryRate <= 0 {
		return c.clampTarget()
	}
	c.usageUpdated = false

	deliveredRate := float64(deliveryRate)
	c.state = c.state.transition(c.usage)
	// A backwards timestamp would otherwise shrink the target in the increase
	// state.
	window := max(ts.Sub(c.lastUpdate), 0)
	c.lastUpdate = ts

	switch c.state {
	case stateIncrease:
		if c.canIncreaseMultiplicatively(deliveredRate) {
			c.targetRate = multiplicativeIncrease(c.targetRate, window)
		} else {
			c.targetRate = additiveIncrease(c.targetRate, rtt, window)
		}
		c.targetRate = min(c.targetRate, int(1.5*deliveredRate))
	case stateDecrease:
		decreased := c.decreaseFactor * deliveredRate
		// A delivery rate above the current target would otherwise just hold
		// the target instead of backing off, so retry against the last
		// decrease rate estimate.
		if int(decreased) > c.targetRate && c.lastDecreaseRate.hasEstimate() {
			decreased = c.decreaseFactor * c.lastDecreaseRate.avg()
		}
		c.targetRate = min(c.targetRate, int(decreased))
		c.lastDecreaseRate.update(deliveredRate)
	case stateHold:
		// The target is held while the queues drain.
	}

	return c.clampTarget()
}

// clampTarget limits the target rate to the configured min and max rates.
func (c *delayRateController) clampTarget() int {
	c.targetRate = max(c.targetRate, c.minTarget)
	c.targetRate = min(c.targetRate, c.maxTarget)

	return c.targetRate
}

func (c *delayRateController) canIncreaseMultiplicatively(deliveredRate float64) bool {
	if !c.lastDecreaseRate.hasEstimate() {
		return true
	}
	avg := c.lastDecreaseRate.avg()
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
