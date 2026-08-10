// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDelayRateController(t *testing.T) {
	t.Run("init", func(t *testing.T) {
		controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
		assert.Nil(t, controller.log)
		assert.Equal(t, controller.decreaseFactor, defaultDecreaseFactor)
		assert.NotNil(t, controller.arrivalGroups)
		assert.NotNil(t, controller.lastArrivalGroup)
		assert.NotNil(t, controller.trend)
		assert.NotNil(t, controller.overuse)
		assert.Equal(t, controller.samples, 0)
		assert.Equal(t, controller.usage, usage(0))
		assert.Equal(t, controller.state, state(0))
		assert.NotNil(t, controller.lastDecreaseRate)
		assert.Zero(t, controller.lastUpdate)
		assert.Equal(t, controller.minTarget, 500_000)
		assert.Equal(t, controller.maxTarget, 2_000_000)
		assert.Equal(t, controller.targetRate, 1_000_000)
	})

	t.Run("canIncreaseMultiplicatively", func(t *testing.T) {
		cases := []struct {
			deliveredRate float64
			decreaseRate  ewma
			expected      bool
		}{
			{deliveredRate: 1000, decreaseRate: ewma{average: 0, variance: 0}, expected: true},
			{deliveredRate: 1000, decreaseRate: ewma{average: 1500, variance: 100}, expected: true},
			{deliveredRate: 1000, decreaseRate: ewma{average: 1020, variance: 100}, expected: false},
			{deliveredRate: 1000, decreaseRate: ewma{average: 800, variance: 50}, expected: true},
			{deliveredRate: 1000, decreaseRate: ewma{average: 995, variance: 100}, expected: false},
		}

		for i, c := range cases {
			t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
				controller := newDelayRateController(1000, 500, 2000, nil)
				controller.lastDecreaseRate = &c.decreaseRate
				assert.Equal(t, c.expected, controller.canIncreaseMultiplicatively(c.deliveredRate))
			})
		}
	})

	t.Run("update", func(t *testing.T) {
		cases := []struct {
			name         string
			usage        usage
			initialRate  int
			minRate      int
			maxRate      int
			deliveryRate int
			expected     int
		}{
			{
				name:         "hold_on_underuse",
				usage:        usageUnder,
				initialRate:  1_000_000,
				minRate:      500_000,
				maxRate:      2_000_000,
				deliveryRate: 1_000_000,
				expected:     1_000_000,
			},
			{
				name:         "no_delivery_rate_holds_target_on_increase",
				usage:        usageNormal,
				initialRate:  1_000_000,
				minRate:      500_000,
				maxRate:      2_000_000,
				deliveryRate: 0,
				expected:     1_000_000,
			},
			{
				name:         "no_delivery_rate_holds_target_on_decrease",
				usage:        usageOver,
				initialRate:  1_000_000,
				minRate:      500_000,
				maxRate:      2_000_000,
				deliveryRate: 0,
				expected:     1_000_000,
			},
			{
				name:         "multiplicative_increase",
				usage:        usageNormal,
				initialRate:  1_000_000,
				minRate:      500_000,
				maxRate:      2_000_000,
				deliveryRate: 1_000_000,
				expected:     1_080_000,
			},
			{
				name:         "increase_capped_at_1.5x_delivered",
				usage:        usageNormal,
				initialRate:  1_000_000,
				minRate:      500_000,
				maxRate:      2_000_000,
				deliveryRate: 600_000,
				expected:     900_000,
			},
			{
				name:         "increase_clamped_at_max",
				usage:        usageNormal,
				initialRate:  1_990_000,
				minRate:      500_000,
				maxRate:      2_000_000,
				deliveryRate: 10_000_000,
				expected:     2_000_000,
			},
			{
				name:         "decrease_relative_to_delivered",
				usage:        usageOver,
				initialRate:  1_000_000,
				minRate:      500_000,
				maxRate:      2_000_000,
				deliveryRate: 1_000_000,
				expected:     850_000,
			},
			{
				name:         "decrease_clamped_at_min",
				usage:        usageOver,
				initialRate:  1_000_000,
				minRate:      500_000,
				maxRate:      2_000_000,
				deliveryRate: 200_000,
				expected:     500_000,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				controller := newDelayRateController(tc.initialRate, tc.minRate, tc.maxRate, nil)
				controller.usage = tc.usage
				controller.usageUpdated = true
				now := time.Time{}.Add(time.Hour)
				controller.lastUpdate = now.Add(-time.Second)

				res := controller.update(now, tc.deliveryRate, 100*time.Millisecond)
				assert.InDelta(t, tc.expected, res, 1)
			})
		}
	})

	t.Run("updateWithoutNewArrivalGroup", func(t *testing.T) {
		controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
		controller.usage = usageOver
		controller.usageUpdated = true
		now := time.Time{}.Add(time.Hour)
		controller.lastUpdate = now.Add(-time.Second)

		// The first update acts on the overuse.
		assert.Equal(t, 850_000, controller.update(now, 1_000_000, 100*time.Millisecond))

		// The sender follows the lowered target, so the delivery rate drops with
		// it. No arrival group completed since, so there is no new delay signal
		// and the same overuse must not decrease the target again.
		now = now.Add(100 * time.Millisecond)
		assert.Equal(t, 850_000, controller.update(now, 850_000, 100*time.Millisecond))
		now = now.Add(100 * time.Millisecond)
		assert.Equal(t, 850_000, controller.update(now, 722_500, 100*time.Millisecond))
	})

	t.Run("multiplicativeIncrease", func(t *testing.T) {
		cases := []struct {
			initialRate int
			rate        int
			window      time.Duration
			expected    float64
		}{
			{initialRate: 1000, rate: 1000, window: 100 * time.Millisecond, expected: 1007},
		}
		for i, c := range cases {
			t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
				res := multiplicativeIncrease(c.rate, c.window)
				assert.InDelta(t, res, c.expected, 1)
			})
		}
	})

	t.Run("additiveIncrease", func(t *testing.T) {
		cases := []struct {
			initialRate int
			rate        int
			window      time.Duration
			expected    int
		}{
			{initialRate: 1000, rate: 1000, window: 100 * time.Millisecond, expected: 2000},
			{initialRate: 1_000_000, rate: 1_500_000, window: 100 * time.Millisecond, expected: 1_500_000 + 2083},
		}
		for i, c := range cases {
			t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
				res := additiveIncrease(c.rate, 100*time.Millisecond, c.window)
				assert.InDelta(t, res, c.expected, 1)
			})
		}
	})
}
