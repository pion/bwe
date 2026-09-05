// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"errors"
	"time"

	"github.com/pion/logging"
)

var (
	// ErrNilLoggerFactory is returned by WithLoggerFactory if the given logger
	// factory is nil.
	ErrNilLoggerFactory = errors.New("logger factory must not be nil")

	// ErrInvalidRateRange is returned by NewSendSideController if minRate is
	// greater than maxRate.
	ErrInvalidRateRange = errors.New("minRate must not be greater than maxRate")
)

// Option is a functional option for a SendSideController.
type Option func(*SendSideController) error

// WithLoggerFactory configures a custom logger factory for a
// SendSideController.
func WithLoggerFactory(lf logging.LoggerFactory) Option {
	return func(ssc *SendSideController) error {
		if lf == nil {
			return ErrNilLoggerFactory
		}
		ssc.logFactory = lf

		return nil
	}
}

// SendSideController is a sender side congestion controller.
//
// A SendSideController must not be used concurrently from multiple goroutines.
type SendSideController struct {
	logFactory logging.LoggerFactory
	log        logging.LeveledLogger
	dre        *deliveryRateEstimator
	lrc        *lossRateController
	drc        *delayRateController
	lastTS     time.Time
	targetRate int
}

// NewSendSideController creates a new SendSideController with initial, min and
// max rates. initialRate is clamped to [minRate, maxRate].
func NewSendSideController(initialRate, minRate, maxRate int, opts ...Option) (*SendSideController, error) {
	if minRate > maxRate {
		return nil, ErrInvalidRateRange
	}
	initialRate = min(max(initialRate, minRate), maxRate)
	ssc := &SendSideController{
		logFactory: logging.NewDefaultLoggerFactory(),
		dre:        newDeliveryRateEstimator(time.Second),
		lrc:        newLossRateController(initialRate, minRate, maxRate),
		targetRate: initialRate,
	}
	for _, opt := range opts {
		if err := opt(ssc); err != nil {
			return nil, err
		}
	}
	ssc.log = ssc.logFactory.NewLogger("bwe_send_side_controller")
	ssc.drc = newDelayRateController(initialRate, minRate, maxRate, ssc.logFactory.NewLogger("bwe_delay_rate_controller"))

	return ssc, nil
}

// OnLoss must be called when a packet is reported as lost. Packets MUST not be
// reported more than once.
func (c *SendSideController) OnLoss() {
	c.lrc.onPacketLost()
}

// OnAck must be called when new acknowledgments arrive. Packets MUST not be
// acknowledged more than once. Packets from the same feedback report must be
// passed in ascending arrival-time order.
func (c *SendSideController) OnAck(sequenceNumber uint64, size int, departure, arrival time.Time) {
	c.lrc.onPacketAcked()
	if !arrival.IsZero() {
		c.dre.onPacketAcked(arrival, size)
		c.drc.onPacketAcked(
			sequenceNumber,
			size,
			departure,
			arrival,
		)
	}
}

// OnFeedback must be called when a new feedback report arrives. ts is the
// arrival timestamp of the feedback report and must not be earlier than that of
// the previous report. rtt is the latest RTT sample. It returns the new target
// rate.
func (c *SendSideController) OnFeedback(ts time.Time, rtt time.Duration) int {
	if ts.Before(c.lastTS) {
		c.log.Warnf("ignoring feedback timestamp %v, earlier than previous timestamp %v", ts, c.lastTS)
		ts = c.lastTS
	}
	c.lastTS = ts
	delivered := c.dre.getRate()
	lossTarget, _ := c.lrc.update(ts, rtt, delivered)
	delayTarget := c.drc.update(ts, delivered, rtt)
	c.targetRate = min(lossTarget, delayTarget)
	c.lrc.setTargetRate(c.targetRate)
	c.log.Tracef(
		"rtt=%v, delivered=%v, lossTarget=%v, delayTarget=%v, target=%v",
		rtt.Nanoseconds(),
		delivered,
		lossTarget,
		delayTarget,
		c.targetRate,
	)

	return c.targetRate
}
