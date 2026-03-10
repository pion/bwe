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
