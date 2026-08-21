// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"errors"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

var errTestOption = errors.New("test option")

func TestSendSideController(t *testing.T) {
	t.Run("init", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)
		assert.NotNil(t, ssc.logFactory)
		assert.NotNil(t, ssc.log)
		assert.NotNil(t, ssc.dre)
		assert.NotNil(t, ssc.lrc)
		assert.NotNil(t, ssc.drc)
		assert.Equal(t, 1_000_000, ssc.targetRate)
	})

	t.Run("withLoggerFactory", func(t *testing.T) {
		factory := logging.NewDefaultLoggerFactory()
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000, WithLoggerFactory(factory))
		assert.NoError(t, err)
		assert.Same(t, factory, ssc.logFactory)
	})

	t.Run("optionError", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000, func(*SendSideController) error {
			return errTestOption
		})
		assert.ErrorIs(t, err, errTestOption)
		assert.Nil(t, ssc)
	})

	t.Run("noFeedbackKeepsTargetRate", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)
		assert.Equal(t, 1_000_000, ssc.OnFeedback(time.Time{}.Add(time.Hour), 100*time.Millisecond))
	})

	t.Run("targetIsMinimumOfLossAndDelayTarget", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)

		// A single lost packet is a loss rate of 100%, so the loss controller
		// halves its target. The delay controller has not seen any arrival
		// group and keeps the initial rate, so the loss target wins.
		ssc.OnLoss()
		rate := ssc.OnFeedback(time.Time{}.Add(time.Hour), 100*time.Millisecond)
		assert.Equal(t, 500_000, rate)
		assert.Equal(t, rate, ssc.targetRate)
	})

	t.Run("ackWithoutArrivalOnlyCountsForLoss", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)

		ssc.OnAck(0, 1200, time.Time{}.Add(time.Hour), time.Time{})
		assert.Equal(t, 1, ssc.lrc.packetsSinceLastUpdate)
		assert.Zero(t, ssc.dre.getRate())
		assert.Zero(t, ssc.drc.samples)
	})

	t.Run("increasesWhileDelivering", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)

		// 100 packets at a steady rate and a constant one way delay: no queue
		// builds up, so both controllers are free to increase.
		departure := time.Time{}.Add(time.Hour)
		arrival := departure.Add(50 * time.Millisecond)
		for i := range 100 {
			offset := time.Duration(i) * time.Millisecond
			ssc.OnAck(uint64(i), 1200, departure.Add(offset), arrival.Add(offset)) // nolint:gosec // loop index
		}

		rate := ssc.OnFeedback(departure.Add(150*time.Millisecond), 100*time.Millisecond)
		assert.Greater(t, rate, 1_000_000)
		assert.Equal(t, rate, ssc.targetRate)
	})

	t.Run("decreasesWhileDelayIncreases", func(t *testing.T) {
		ssc, err := NewSendSideController(8_000_000, 100_000, 10_000_000)
		assert.NoError(t, err)

		// Packets depart every millisecond but arrive 1.5 milliseconds apart, so
		// a queue is building up and the overuse detector reports an overuse.
		departure := time.Time{}.Add(time.Hour)
		arrival := departure.Add(50 * time.Millisecond)
		for i := range 100 {
			ssc.OnAck(
				uint64(i), // nolint:gosec // loop index
				1200,
				departure.Add(time.Duration(i)*time.Millisecond),
				arrival.Add(time.Duration(i)*1500*time.Microsecond),
			)
		}

		// 99 packets delivered over 148.5ms is 6.4Mbps, and the delay controller
		// decreases to 85% of that.
		rate := ssc.OnFeedback(departure.Add(200*time.Millisecond), 100*time.Millisecond)
		assert.Equal(t, usageOver, ssc.drc.usage)
		assert.Equal(t, stateDecrease, ssc.drc.state)
		assert.Equal(t, 5_440_000, rate)
		assert.Equal(t, rate, ssc.targetRate)
	})
}
