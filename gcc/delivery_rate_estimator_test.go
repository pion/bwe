// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeliveryRateEstimator(t *testing.T) {
	type ack struct {
		arrival time.Time
		size    int
	}
	cases := []struct {
		window       time.Duration
		acks         []ack
		expectedRate int
	}{
		{
			window:       0,
			acks:         []ack{},
			expectedRate: 0,
		},
		{
			window:       time.Second,
			acks:         []ack{},
			expectedRate: 0,
		},
		{
			window: time.Second,
			acks: []ack{
				{time.Time{}, 1200},
			},
			expectedRate: 0,
		},
		{
			window: time.Second,
			acks: []ack{
				{time.Time{}.Add(time.Millisecond), 1200},
			},
			expectedRate: 0,
		},
		{
			// Two packets arrived in the millisecond following the first one,
			// which marks the start of the interval.
			window: time.Second,
			acks: []ack{
				{time.Time{}.Add(time.Second), 1200},
				{time.Time{}.Add(1500 * time.Millisecond), 1200},
				{time.Time{}.Add(2 * time.Second), 1200},
			},
			expectedRate: 19200,
		},
		{
			// An idle period before the oldest packet in the history counts
			// towards the interval, but only up to the configured window.
			window: time.Second,
			acks: []ack{
				{time.Time{}.Add(time.Second), 1200},
				{time.Time{}.Add(3 * time.Second), 1200},
				{time.Time{}.Add(3001 * time.Millisecond), 1200},
				{time.Time{}.Add(3002 * time.Millisecond), 1200},
			},
			expectedRate: 28800,
		},
		{
			window: time.Second,
			acks: []ack{
				{time.Time{}.Add(500 * time.Millisecond), 1200},
				{time.Time{}.Add(time.Second), 1200},
				{time.Time{}.Add(1500 * time.Millisecond), 1200},
				{time.Time{}.Add(2 * time.Second), 1200},
			},
			expectedRate: 28800,
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			e := newDeliveryRateEstimator(tc.window)
			for _, ack := range tc.acks {
				e.onPacketAcked(ack.arrival, ack.size)
			}
			assert.Equal(t, tc.expectedRate, e.getRate())
		})
	}
}

func TestDeliveryRateEstimatorSteadyStream(t *testing.T) {
	// One 1200 byte packet every millisecond is a steady 9.6 Mbps, regardless
	// of how many of them fit into the window.
	for _, packets := range []int{2, 11, 101, 1001, 1501} {
		t.Run(fmt.Sprintf("%v_packets", packets), func(t *testing.T) {
			e := newDeliveryRateEstimator(time.Second)
			for i := range packets {
				e.onPacketAcked(time.Time{}.Add(time.Duration(i)*time.Millisecond), 1200)
			}
			// Once packets are evicted, the one that arrived exactly on the
			// deadline is counted towards a full window, which is off by a
			// single packet spacing.
			assert.InEpsilon(t, 9_600_000, e.getRate(), 0.002)
		})
	}
}
