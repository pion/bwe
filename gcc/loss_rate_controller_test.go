// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLossRateController(t *testing.T) {
	// Every case opens a window with a first update, reports its packets into
	// it and then closes it with a second one. elapsed is how far apart the two
	// are, so a case can hold the window open by staying under it.
	cases := []struct {
		init, min, max int
		rtt            time.Duration
		elapsed        time.Duration
		acked          int
		lost           int
		deliveredRate  int
		expectedRate   int
		expectedUpdate bool
	}{
		{}, // all zeros
		{
			init:           100_000,
			min:            100_000,
			max:            1_000_000,
			rtt:            50 * time.Millisecond,
			elapsed:        minLossWindow,
			acked:          0,
			lost:           0,
			deliveredRate:  0,
			expectedRate:   100_000,
			expectedUpdate: false,
		},
		{
			init:           100_000,
			min:            100_000,
			max:            1_000_000,
			rtt:            50 * time.Millisecond,
			elapsed:        minLossWindow,
			acked:          99,
			lost:           1,
			deliveredRate:  100_000,
			expectedRate:   105_000,
			expectedUpdate: true,
		},
		{
			init:           100_000,
			min:            100_000,
			max:            1_000_000,
			rtt:            50 * time.Millisecond,
			elapsed:        minLossWindow,
			acked:          99,
			lost:           1,
			deliveredRate:  90_000,
			expectedRate:   105_000,
			expectedUpdate: true,
		},
		{
			init:           100_000,
			min:            100_000,
			max:            1_000_000,
			rtt:            50 * time.Millisecond,
			elapsed:        minLossWindow,
			acked:          95,
			lost:           5,
			deliveredRate:  99_000,
			expectedRate:   100_000,
			expectedUpdate: true,
		},
		{
			init:           100_000,
			min:            50_000,
			max:            1_000_000,
			rtt:            50 * time.Millisecond,
			elapsed:        minLossWindow,
			acked:          89,
			lost:           11,
			deliveredRate:  90_000,
			expectedRate:   94_500,
			expectedUpdate: true,
		},
		{
			init:           100_000,
			min:            100_000,
			max:            1_000_000,
			rtt:            50 * time.Millisecond,
			elapsed:        minLossWindow,
			acked:          89,
			lost:           11,
			deliveredRate:  90_000,
			expectedRate:   100_000,
			expectedUpdate: true,
		},
		{
			// Enough packets, but the window has not run its time, so the
			// counts stay open and the target is left alone.
			init:           100_000,
			min:            100_000,
			max:            1_000_000,
			rtt:            50 * time.Millisecond,
			elapsed:        minLossWindow - time.Millisecond,
			acked:          89,
			lost:           11,
			deliveredRate:  90_000,
			expectedRate:   100_000,
			expectedUpdate: false,
		},
		{
			// The round trip is longer than the floor, so it is the round trip
			// that sets the window and this one is still short of it.
			init:           100_000,
			min:            100_000,
			max:            1_000_000,
			rtt:            500 * time.Millisecond,
			elapsed:        300 * time.Millisecond,
			acked:          89,
			lost:           11,
			deliveredRate:  90_000,
			expectedRate:   100_000,
			expectedUpdate: false,
		},
		{
			// The window has run well past its time but holds too few packets
			// for a ratio, so it keeps accumulating instead.
			init:           100_000,
			min:            100_000,
			max:            1_000_000,
			rtt:            50 * time.Millisecond,
			elapsed:        5 * time.Second,
			acked:          minLossPackets - 2,
			lost:           1,
			deliveredRate:  90_000,
			expectedRate:   100_000,
			expectedUpdate: false,
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			lrc := newLossRateController(tc.init, tc.min, tc.max)
			start := time.Time{}.Add(time.Hour)

			rate, updated := lrc.update(start, tc.rtt, tc.deliveredRate)
			assert.Equal(t, tc.init, rate, "the first update only opens the window")
			assert.False(t, updated, "the first update only opens the window")

			for range tc.acked {
				lrc.onPacketAcked()
			}
			for range tc.lost {
				lrc.onPacketLost()
			}

			rate, updated = lrc.update(start.Add(tc.elapsed), tc.rtt, tc.deliveredRate)
			assert.Equal(t, tc.expectedRate, rate)
			assert.Equal(t, tc.expectedUpdate, updated)
		})
	}

	t.Run("oneDecreasePerWindow", func(t *testing.T) {
		// The point of the window: the same congestion event reported over
		// several feedback reports is one decrease, not one per report.
		lrc := newLossRateController(1_000_000, 100_000, 10_000_000)
		start := time.Time{}.Add(time.Hour)
		lrc.update(start, 100*time.Millisecond, 1_000_000)

		decreases := 0
		previous := lrc.bitrate
		// Twenty reports at the 20ms cadence the simulation uses cover two
		// 200ms windows, so a controller reacting per report would cut twenty
		// times over.
		for i := 1; i <= 20; i++ {
			for range 5 {
				lrc.onPacketLost()
			}
			rate, _ := lrc.update(
				start.Add(time.Duration(i)*20*time.Millisecond),
				100*time.Millisecond,
				1_000_000,
			)
			if rate < previous {
				decreases++
			}
			previous = rate
		}
		assert.Equal(t, 2, decreases, "one decrease per window, not one per report")
	})

	t.Run("windowStretchesUntilItHasPackets", func(t *testing.T) {
		lrc := newLossRateController(1_000_000, 100_000, 10_000_000)
		start := time.Time{}.Add(time.Hour)
		lrc.update(start, 10*time.Millisecond, 1_000_000)

		at := start
		for range minLossPackets - 1 {
			at = at.Add(minLossWindow)
			lrc.onPacketAcked()
			_, updated := lrc.update(at, 10*time.Millisecond, 1_000_000)
			assert.False(t, updated, "too few packets to read a ratio off")
		}

		at = at.Add(minLossWindow)
		lrc.onPacketAcked()
		_, updated := lrc.update(at, 10*time.Millisecond, 1_000_000)
		assert.True(t, updated, "the window closes once it has its packets")
	})
}
