// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"time"

	"github.com/pion/logging"
)

// Option is a functional option for a SendSideController.
type Option func(*SendSideController) error

// WithLoggerFactory configures a custom logger factory for a
// SendSideController.
func WithLoggerFactory(lf logging.LoggerFactory) Option {
	return func(ssc *SendSideController) error {
		ssc.logFactory = lf

		return nil
	}
}

// SendSideController is a sender side congestion controller.
type SendSideController struct {
	logFactory logging.LoggerFactory
	log        logging.LeveledLogger
	dre        *deliveryRateEstimator
	lrc        *lossRateController
	drc        *delayRateController
	targetRate int
}

// NewSendSideController creates a new SendSideController with initial, min and
// max rates.
func NewSendSideController(initialRate, minRate, maxRate int, opts ...Option) (*SendSideController, error) {
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
	ssc.drc = newDelayRateController(initialRate, ssc.logFactory.NewLogger("bwe_delay_rate_controller"))

	return ssc, nil
}

func (c *SendSideController) OnLoss() {
	c.lrc.onPacketLost()
}

// OnAck must be called when new acknowledgments arrive. Packets MUST not be
// acknowledged more than once.
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
// arrival timestamp of the feedback report. rtt is the latest RTT sample. It
// returns the new target rate.
func (c *SendSideController) OnFeedback(ts time.Time, rtt time.Duration) int {
	delivered := c.dre.getRate()
	lossTarget := c.lrc.update(delivered)
	delayTarget := c.drc.update(ts, delivered, rtt)
	c.targetRate = min(lossTarget, delayTarget)
	c.log.Tracef("rttduration=%v", rtt)
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
