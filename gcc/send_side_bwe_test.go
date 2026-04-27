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

	t.Run("withDeliveryRateWindow", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000, WithDeliveryRateWindow(150*time.Millisecond))
		assert.NoError(t, err)
		assert.Equal(t, 150*time.Millisecond, ssc.dre.window)
	})

	t.Run("withInvalidDeliveryRateWindow", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000, WithDeliveryRateWindow(0))
		assert.ErrorIs(t, err, ErrInvalidDeliveryRateWindow)
		assert.Nil(t, ssc)
	})

	t.Run("defaultDeliveryRateWindow", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)
		assert.Equal(t, defaultDeliveryRateWindow, ssc.dre.window)
	})

	t.Run("targetRate", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)
		assert.Equal(t, 1_000_000, ssc.TargetRate())

		start := time.Time{}.Add(time.Hour)
		ssc.OnFeedback(start, 100*time.Millisecond)
		for range 20 {
			ssc.OnLoss()
		}
		rate := ssc.OnFeedback(start.Add(minLossWindow), 100*time.Millisecond)
		assert.Equal(t, rate, ssc.TargetRate())
	})

	t.Run("smoothsRTT", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)

		start := time.Time{}.Add(time.Hour)
		ssc.OnFeedback(start, 100*time.Millisecond)
		assert.Equal(t, float64(100*time.Millisecond), ssc.rtt.avg())

		// A single spiked sample moves the smoothed RTT by alpha only.
		ssc.OnFeedback(start.Add(time.Millisecond), 1100*time.Millisecond)
		assert.Equal(t, float64(200*time.Millisecond), ssc.rtt.avg())
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

		start := time.Time{}.Add(time.Hour)
		ssc.OnFeedback(start, 100*time.Millisecond)

		// 20 lost packets is a loss rate of 100%, so the loss controller
		// halves its target. The delay controller has not seen any arrival
		// group and keeps the initial rate, so the loss target wins.
		for range 20 {
			ssc.OnLoss()
		}
		rate := ssc.OnFeedback(start.Add(minLossWindow), 100*time.Millisecond)
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

		departure := time.Time{}.Add(time.Hour)
		arrival := departure.Add(50 * time.Millisecond)
		ssc.OnFeedback(departure, 100*time.Millisecond)

		// 100 packets at a steady rate and a constant one way delay: no queue
		// builds up, so both controllers are free to increase.
		for i := range 100 {
			offset := time.Duration(i) * time.Millisecond
			ssc.OnAck(uint64(i), 1200, departure.Add(offset), arrival.Add(offset)) // nolint:gosec // loop index
		}

		rate := ssc.OnFeedback(departure.Add(minLossWindow), 100*time.Millisecond)
		assert.Greater(t, rate, 1_000_000)
		assert.Equal(t, rate, ssc.targetRate)
	})

	t.Run("decreasesWhileDelayIncreases", func(t *testing.T) {
		ssc, err := NewSendSideController(8_000_000, 100_000, 10_000_000)
		assert.NoError(t, err)

		departure := time.Time{}.Add(time.Hour)
		arrival := departure.Add(50 * time.Millisecond)
		ssc.OnFeedback(departure, 100*time.Millisecond)

		// Packets depart every millisecond but arrive 1.5 milliseconds apart, so
		// a queue is building up and the overuse detector reports an overuse.
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

// TestSendSideControllerTargetCombination covers how the loss and delay
// targets combine into the reported target rate.
func TestSendSideControllerTargetCombination(t *testing.T) {
	t.Run("followsDelayTargetWhileLossWindowIsOpen", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)

		departure := time.Time{}.Add(time.Hour)
		arrival := departure.Add(50 * time.Millisecond)
		ssc.OnFeedback(departure, 100*time.Millisecond)

		// A steady rate at a constant one way delay, so no queue builds up and
		// both controllers increase. Feed enough packets and enough time for
		// the loss window to close once, which puts the loss estimate ahead of
		// the delay estimate.
		ack := func(from, count int, at time.Duration) {
			for i := from; i < from+count; i++ {
				offset := at + time.Duration(i)*time.Millisecond
				ssc.OnAck(uint64(i), 1200, departure.Add(offset), arrival.Add(offset)) // nolint:gosec // loop index
			}
		}
		ack(0, 50, 0)
		ssc.OnFeedback(departure.Add(minLossWindow), 100*time.Millisecond)
		assert.Greater(t, ssc.lrc.bitrate, ssc.drc.targetRate)

		// The loss window is open again, so the loss controller holds an
		// estimate above the delay controller's and the target must follow the
		// delay controller on every feedback, not only on the ones the loss
		// window closes on.
		for i := range 3 {
			at := minLossWindow + time.Duration(i+1)*20*time.Millisecond
			ack(50+i*10, 10, at)
			rate := ssc.OnFeedback(departure.Add(at), 100*time.Millisecond)
			assert.False(t, ssc.lrc.windowClosed(departure.Add(at), 100*time.Millisecond))
			assert.Equal(t, ssc.drc.targetRate, rate)
			assert.Equal(t, rate, ssc.targetRate)
		}
	})

	t.Run("holdsLossCutWhileLossWindowIsOpen", func(t *testing.T) {
		ssc, err := NewSendSideController(1_000_000, 100_000, 2_000_000)
		assert.NoError(t, err)

		departure := time.Time{}.Add(time.Hour)
		arrival := departure.Add(50 * time.Millisecond)
		ssc.OnFeedback(departure, 100*time.Millisecond)

		// A steady rate at a constant one way delay, so the delay controller
		// increases, but a quarter of the packets are lost.
		for i := range 40 {
			offset := time.Duration(i) * time.Millisecond
			if i%4 == 0 {
				ssc.OnLoss()

				continue
			}
			ssc.OnAck(uint64(i), 1200, departure.Add(offset), arrival.Add(offset)) // nolint:gosec // loop index
		}

		cut := ssc.OnFeedback(departure.Add(minLossWindow), 100*time.Millisecond)
		assert.Less(t, cut, 1_000_000)
		assert.Greater(t, ssc.drc.targetRate, cut)

		// The next feedback reopens the loss window. The cut must survive it
		// rather than being replaced by the delay controller's higher target.
		for i := 40; i < 50; i++ {
			offset := minLossWindow + time.Duration(i)*time.Millisecond
			ssc.OnAck(uint64(i), 1200, departure.Add(offset), arrival.Add(offset)) // nolint:gosec // loop index
		}
		rate := ssc.OnFeedback(departure.Add(minLossWindow+20*time.Millisecond), 100*time.Millisecond)
		assert.Equal(t, cut, rate)
	})
}
