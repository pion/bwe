// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDelayRateControllerOnPacketAcked(t *testing.T) {
	const departureStep = 10 * time.Millisecond

	cases := []struct {
		name            string
		packets         int
		delayStep       time.Duration
		expectedSamples int
		expectedUsage   usage
		expectedUpdated bool
	}{
		{
			name:            "no_completed_group",
			packets:         1,
			delayStep:       0,
			expectedSamples: 0,
			expectedUsage:   usageNormal,
			expectedUpdated: false,
		},
		{
			name:            "first_group_has_no_predecessor",
			packets:         2,
			delayStep:       0,
			expectedSamples: 0,
			expectedUsage:   usageNormal,
			expectedUpdated: false,
		},
		{
			name:            "constant_delay_is_normal",
			packets:         15,
			delayStep:       0,
			expectedSamples: 13,
			expectedUsage:   usageNormal,
			expectedUpdated: true,
		},
		{
			name:            "increasing_delay_is_overuse",
			packets:         15,
			delayStep:       5 * time.Millisecond,
			expectedSamples: 13,
			expectedUsage:   usageOver,
			expectedUpdated: true,
		},
		{
			name:            "decreasing_delay_is_underuse",
			packets:         15,
			delayStep:       -2 * time.Millisecond,
			expectedSamples: 13,
			expectedUsage:   usageUnder,
			expectedUpdated: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
			base := time.Time{}.Add(time.Hour)

			for i := range tc.packets {
				departure := base.Add(time.Duration(i) * departureStep)
				arrival := departure.Add(20*time.Millisecond + time.Duration(i)*tc.delayStep)
				controller.onPacketAcked(uint64(i), 1200, departure, arrival)
			}

			assert.Equal(t, tc.expectedSamples, controller.samples)
			assert.Equal(t, tc.expectedUsage, controller.usage)
			assert.Equal(t, tc.expectedUpdated, controller.usageUpdated)
		})
	}
}

func TestDelayRateControllerDropsReorderd(t *testing.T) {
	controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
	base := time.Time{}.Add(time.Hour)
	ack := func(seq uint64, departure, arrival time.Duration) {
		controller.onPacketAcked(seq, 1200, base.Add(departure), base.Add(arrival))
	}

	ack(0, 0, 0)
	ack(1, 50*time.Millisecond, 2*time.Millisecond)
	ack(2, 6*time.Millisecond, 100*time.Millisecond)
	assert.False(t, controller.usageUpdated)
	assert.Zero(t, controller.samples)

	ack(3, 15*time.Millisecond, 150*time.Millisecond)
	assert.False(t, controller.usageUpdated)
	assert.Zero(t, controller.samples)
	assert.Equal(t, 6*time.Millisecond, controller.lastArrivalGroup.departure.Sub(base))

	ack(4, 25*time.Millisecond, 200*time.Millisecond)
	assert.True(t, controller.usageUpdated)
	assert.Equal(t, 1, controller.samples)
	assert.Equal(t, 15*time.Millisecond, controller.lastArrivalGroup.departure.Sub(base))
}

func TestDelayRateControllerUpdate(t *testing.T) {
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
			name:         "decrease_does_not_raise_target",
			usage:        usageOver,
			initialRate:  1_000_000,
			minRate:      500_000,
			maxRate:      2_000_000,
			deliveryRate: 1_400_000,
			expected:     1_000_000,
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
			controller.hasLastUpdate = true

			res := controller.update(now, tc.deliveryRate, 100*time.Millisecond)
			assert.InDelta(t, tc.expected, res, 1)
		})
	}
}

func TestDelayRateControllerInit(t *testing.T) {
	controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
	assert.NotNil(t, controller.log)
	assert.Equal(t, defaultDecreaseFactor, controller.decreaseFactor)
	assert.NotNil(t, controller.arrivalGroups)
	assert.NotNil(t, controller.lastArrivalGroup)
	assert.NotNil(t, controller.trend)
	assert.NotNil(t, controller.overuse)
	assert.Equal(t, 0, controller.samples)
	assert.Equal(t, usage(0), controller.usage)
	assert.Equal(t, state(0), controller.state)
	assert.NotNil(t, controller.lastDecreaseRate)
	assert.Zero(t, controller.lastUpdate)
	assert.False(t, controller.hasLastUpdate)
	assert.Equal(t, 500_000, controller.minTarget)
	assert.Equal(t, 2_000_000, controller.maxTarget)
	assert.Equal(t, 1_000_000, controller.targetRate)
}

func TestDelayRateControllerInitClampsTheTargetRate(t *testing.T) {
	below := newDelayRateController(100_000, 500_000, 2_000_000, nil)
	assert.Equal(t, 500_000, below.targetRate)

	above := newDelayRateController(9_000_000, 500_000, 2_000_000, nil)
	assert.Equal(t, 2_000_000, above.targetRate)
}

func TestDelayRateControllerInitNormalisesAnInvertedRange(t *testing.T) {
	controller := newDelayRateController(1_000_000, 2_000_000, 500_000, nil)
	assert.Equal(t, 2_000_000, controller.minTarget)
	assert.Equal(t, 2_000_000, controller.maxTarget)
	assert.Equal(t, 2_000_000, controller.targetRate)
}

func TestDelayRateControllerCanIncreaseMultiplicatively(t *testing.T) {
	cases := []struct {
		deliveredRate float64
		decreaseRate  ewma
		expected      bool
	}{
		{deliveredRate: 1000, decreaseRate: ewma{}, expected: true},
		{deliveredRate: 10, decreaseRate: ewma{initialized: true, variance: 100}, expected: false},
		{deliveredRate: 1000, decreaseRate: ewma{initialized: true, average: 1500, variance: 100}, expected: true},
		{deliveredRate: 1000, decreaseRate: ewma{initialized: true, average: 1020, variance: 100}, expected: false},
		{deliveredRate: 1000, decreaseRate: ewma{initialized: true, average: 800, variance: 50}, expected: true},
		{deliveredRate: 1000, decreaseRate: ewma{initialized: true, average: 995, variance: 100}, expected: false},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			controller := newDelayRateController(1000, 500, 2000, nil)
			controller.lastDecreaseRate = &c.decreaseRate
			assert.Equal(t, c.expected, controller.canIncreaseMultiplicatively(c.deliveredRate))
		})
	}
}

func TestDelayRateControllerAdditiveIncreaseNearDecreaseRate(t *testing.T) {
	controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
	now := time.Time{}.Add(time.Hour)
	controller.lastUpdate = now
	controller.hasLastUpdate = true

	// Repeated overuse at similar rates leaves an average decrease rate of
	// about 1_000_000.
	for _, rate := range []int{1_000_000, 1_020_000, 980_000, 1_010_000, 990_000, 1_000_000} {
		now = now.Add(100 * time.Millisecond)
		controller.usage = usageOver
		controller.usageUpdated = true
		controller.update(now, rate, 100*time.Millisecond)
	}
	assert.Equal(t, 833_000, controller.targetRate)

	// Decrease transitions to hold before increase, so the first normal
	// signal leaves the target alone.
	now = now.Add(100 * time.Millisecond)
	controller.usage = usageNormal
	controller.usageUpdated = true
	assert.Equal(t, 833_000, controller.update(now, 1_015_000, 100*time.Millisecond))

	// A delivery rate within three standard deviations of the average
	// decrease rate is close to the link capacity, so the increase is
	// additive (835_313) rather than multiplicative (839_435).
	now = now.Add(100 * time.Millisecond)
	controller.usage = usageNormal
	controller.usageUpdated = true
	assert.Equal(t, 835_313, controller.update(now, 1_015_000, 100*time.Millisecond))
}

func TestDelayRateControllerDecreaseFallsBackToLinkCapacityEstimate(t *testing.T) {
	controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
	controller.lastDecreaseRate = &ewma{
		initialized: true,
		alpha:       defaultDecreaseRateAlpha,
		average:     1_000_000,
		variance:    100,
	}
	now := time.Time{}.Add(time.Hour)
	controller.lastUpdate = now.Add(-time.Second)
	controller.hasLastUpdate = true
	controller.usage = usageOver
	controller.usageUpdated = true

	// 0.85 * 1_400_000 = 1_190_000 is above the current target, which
	// would otherwise hold the target instead of backing off. Falling
	// back to 0.85 * the last decrease rate estimate (1_000_000) produces
	// a real decrease.
	res := controller.update(now, 1_400_000, 100*time.Millisecond)
	assert.Equal(t, 850_000, res)

	// The raw delivered rate is still recorded, not the substituted value.
	assert.InDelta(t, 1_020_000, controller.lastDecreaseRate.avg(), 1)
}

func TestDelayRateControllerFirstUpdateOnlySetsTheWindowReference(t *testing.T) {
	controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
	now := time.Time{}.Add(time.Hour)
	controller.usage = usageOver
	controller.usageUpdated = true

	// No window to measure against yet, so the target and the delay signal
	// are both left alone.
	assert.Equal(t, 1_000_000, controller.update(now, 1_000_000, 100*time.Millisecond))
	assert.Equal(t, now, controller.lastUpdate)
	assert.True(t, controller.usageUpdated)

	// The overuse survives to the next update and is acted on there.
	now = now.Add(100 * time.Millisecond)
	assert.Equal(t, 850_000, controller.update(now, 1_000_000, 100*time.Millisecond))
}

func TestDelayRateControllerBackwardsTimestampDoesNotLowerTheTarget(t *testing.T) {
	cases := []struct {
		name         string
		decreaseRate *ewma
		expected     int
	}{
		{
			name:         "multiplicative",
			decreaseRate: newEWMA(defaultDecreaseRateAlpha),
			expected:     1_000_000,
		},
		{
			name: "additive",
			decreaseRate: &ewma{
				initialized: true,
				alpha:       defaultDecreaseRateAlpha,
				average:     1_000_000,
				variance:    100,
			},
			expected: 1_001_000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
			controller.lastDecreaseRate = tc.decreaseRate
			now := time.Time{}.Add(time.Hour)
			controller.lastUpdate = now
			controller.hasLastUpdate = true
			controller.usage = usageNormal
			controller.usageUpdated = true

			res := controller.update(now.Add(-time.Second), 1_000_000, 100*time.Millisecond)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func TestDelayRateControllerOveruseSurvivesAMissingDeliveryRate(t *testing.T) {
	controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
	now := time.Time{}.Add(time.Hour)
	controller.lastUpdate = now.Add(-time.Second)
	controller.hasLastUpdate = true
	controller.usage = usageOver
	controller.usageUpdated = true

	// Nothing to decrease against, so the target holds and neither the
	// state machine nor the delay signal moves.
	assert.Equal(t, 1_000_000, controller.update(now, 0, 100*time.Millisecond))
	assert.Equal(t, stateHold, controller.state)
	assert.True(t, controller.usageUpdated)

	// The next update with a usable delivery rate acts on that overuse.
	now = now.Add(100 * time.Millisecond)
	assert.Equal(t, 850_000, controller.update(now, 1_000_000, 100*time.Millisecond))
}

func TestDelayRateControllerUpdateWithoutNewArrivalGroup(t *testing.T) {
	controller := newDelayRateController(1_000_000, 500_000, 2_000_000, nil)
	controller.usage = usageOver
	controller.usageUpdated = true
	now := time.Time{}.Add(time.Hour)
	controller.lastUpdate = now.Add(-time.Second)
	controller.hasLastUpdate = true

	// The first update acts on the overuse.
	assert.Equal(t, 850_000, controller.update(now, 1_000_000, 100*time.Millisecond))

	// The sender follows the lowered target, so the delivery rate drops with
	// it. No arrival group completed since, so there is no new delay signal
	// and the same overuse must not decrease the target again.
	now = now.Add(100 * time.Millisecond)
	assert.Equal(t, 850_000, controller.update(now, 850_000, 100*time.Millisecond))
	now = now.Add(100 * time.Millisecond)
	assert.Equal(t, 850_000, controller.update(now, 722_500, 100*time.Millisecond))
}

func TestDelayRateControllerMultiplicativeIncrease(t *testing.T) {
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
			assert.InDelta(t, c.expected, res, 1)
		})
	}
}

func TestDelayRateControllerAdditiveIncrease(t *testing.T) {
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
			assert.InDelta(t, c.expected, res, 1)
		})
	}
}
