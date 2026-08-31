// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOveruseDetectorUpdate(t *testing.T) {
	type estimate struct {
		arrivalDelta time.Duration
		sendDelta    time.Duration
		trend        float64
		numDeltas    int
	}
	cases := []struct {
		name     string
		values   []estimate
		expected []usage
	}{
		{
			name:     "noEstimateNoUsage",
			values:   []estimate{},
			expected: []usage{},
		},
		{
			name: "confirmsOveruse",
			values: []estimate{
				{5 * time.Millisecond, 5 * time.Millisecond, 0, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.2, 60},
				{15 * time.Millisecond, 15 * time.Millisecond, 0.5, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name: "normalUseWithFewerThanTwoDeltas",
			values: []estimate{
				{5 * time.Millisecond, 5 * time.Millisecond, 1, 0},
				{5 * time.Millisecond, 5 * time.Millisecond, 1, 1},
			},
			expected: []usage{usageNormal, usageNormal},
		},
		{
			name: "normaluse",
			values: []estimate{
				{arrivalDelta: 5 * time.Millisecond, sendDelta: 5 * time.Millisecond, trend: 0, numDeltas: 60},
			},
			expected: []usage{usageNormal},
		},
		{
			name: "confirmsUnderUse",
			values: []estimate{
				{5 * time.Millisecond, 5 * time.Millisecond, -0.2, 60},
			},
			expected: []usage{usageUnder},
		},
		{
			name: "noOveruseUntilCounterExceedsOne",
			values: []estimate{
				{time.Millisecond, time.Millisecond, 0, 60},
				{20 * time.Millisecond, 20 * time.Millisecond, 0.25, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.5, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name: "noNewOveruseIfEstimateDecreased",
			values: []estimate{
				{time.Millisecond, time.Millisecond, 0, 60},
				{9 * time.Millisecond, 9 * time.Millisecond, 0.4, 60},
				{20 * time.Millisecond, 20 * time.Millisecond, 0.3, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageNormal},
		},
		{
			name: "noOveruseIfRawTrendDecreasesWhileDeltaCountGrows",
			values: []estimate{
				{time.Millisecond, time.Millisecond, 0, 10},
				{9 * time.Millisecond, 9 * time.Millisecond, 1, 20},
				{20 * time.Millisecond, 20 * time.Millisecond, 0.9, 40},
			},
			expected: []usage{usageNormal, usageNormal, usageNormal},
		},
		{
			name: "normalUseWhenTrendFallsBelowThreshold",
			values: []estimate{
				{time.Millisecond, time.Millisecond, 0, 60},
				{9 * time.Millisecond, 9 * time.Millisecond, 0.4, 60},
				{20 * time.Millisecond, 20 * time.Millisecond, 0.5, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.04, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver, usageNormal},
		},
		{
			name: "noOveruseBeforeTimeThreshold",
			values: []estimate{
				{time.Millisecond, time.Millisecond, 0.10, 60},
				{time.Millisecond, time.Millisecond, 0.11, 60},
				{time.Millisecond, time.Millisecond, 0.12, 60},
				{time.Millisecond, time.Millisecond, 0.13, 60},
				{time.Millisecond, time.Millisecond, 0.14, 60},
				{time.Millisecond, time.Millisecond, 0.15, 60},
			},
			expected: []usage{
				usageNormal, usageNormal, usageNormal,
				usageNormal, usageNormal, usageOver,
			},
		},
		{
			name: "noOveruseAtExactlyTheThreshold",
			values: []estimate{
				{20 * time.Millisecond, 20 * time.Millisecond, 0.125, 25},
				{20 * time.Millisecond, 20 * time.Millisecond, 0.125, 25},
				{20 * time.Millisecond, 20 * time.Millisecond, 0.125, 25},
			},
			expected: []usage{usageNormal, usageNormal, usageNormal},
		},
		{
			name: "underuseToOveruse",
			values: []estimate{
				{5 * time.Millisecond, 5 * time.Millisecond, -0.2, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.1, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.2, 60},
			},
			expected: []usage{usageUnder, usageUnder, usageOver},
		},
		{
			name: "keepsOveruseWhileTrendDecreasesAboveThreshold",
			values: []estimate{
				{5 * time.Millisecond, 5 * time.Millisecond, 0, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.1, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.2, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.15, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.14, 60},
			},
			expected: []usage{
				usageNormal, usageNormal, usageOver, usageOver, usageOver,
			},
		},
		{
			name: "keepsOveruseWhileTrendStaysHigh",
			values: []estimate{
				{5 * time.Millisecond, 5 * time.Millisecond, 0, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.04, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.05, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.07, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.1, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.12, 60},
				{5 * time.Millisecond, 5 * time.Millisecond, 0.14, 60},
			},
			expected: []usage{
				usageNormal, usageNormal, usageNormal, usageNormal,
				usageOver, usageOver, usageOver,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			od := newOveruseDetector()
			od.kUp, od.kDown = 0, 0
			received := []usage{}
			arrival := time.Time{}
			for _, e := range tc.values {
				arrival = arrival.Add(e.arrivalDelta)
				u := od.update(arrival, e.sendDelta, e.trend, e.numDeltas)
				received = append(received, u)
			}
			assert.Equal(t, tc.expected, received)
		})
	}
}

func TestOveruseDetectorRecordsComparison(t *testing.T) {
	od := newOveruseDetector()

	// Fewer than two deltas compares nothing.
	od.update(time.Time{}, 5*time.Millisecond, 1, 1)
	assert.Zero(t, od.modifiedTrend)
	assert.InDelta(t, defaultDelayThreshold, od.comparedThreshold, 1e-9)

	arrival := time.Time{}.Add(5 * time.Millisecond)
	od.update(arrival, 5*time.Millisecond, 0.09, 60)
	assert.InDelta(t, 21.6, od.modifiedTrend, 1e-9)
	assert.InDelta(t, defaultDelayThreshold, od.comparedThreshold, 1e-9)

	// The threshold adapts after the comparison, so the recorded threshold is
	// the one the trend was measured against and not the adapted one.
	arrival = arrival.Add(10 * time.Millisecond)
	od.update(arrival, 5*time.Millisecond, 0.09, 60)
	assert.InDelta(t, 21.6, od.modifiedTrend, 1e-9)
	assert.InDelta(t, defaultDelayThreshold, od.comparedThreshold, 1e-9)
	assert.InDelta(t, 13.2917, od.delayThreshold, 1e-4)
}

func TestOveruseDetectorUpdateThreshold(t *testing.T) {
	type estimate struct {
		arrivalDelta time.Duration
		trend        float64
		numDeltas    int
	}
	cases := []struct {
		name             string
		initialThreshold float64
		values           []estimate
		expected         []float64
	}{
		{
			name: "rampsUpTowardsTrendAboveThreshold",
			values: []estimate{
				{5 * time.Millisecond, 0.09, 60},
				{10 * time.Millisecond, 0.09, 60},
				{10 * time.Millisecond, 0.09, 60},
			},
			expected: []float64{12.5, 13.2917, 14.0145221},
		},
		{
			name: "rampsDownTowardsTrendBelowThresholdAndClampsAtMinimum",
			values: []estimate{
				{5 * time.Millisecond, 0.01, 60},
				{10 * time.Millisecond, 0.01, 60},
				{10 * time.Millisecond, 0.01, 60},
				{10 * time.Millisecond, 0.01, 60},
			},
			expected: []float64{12.5, 8.561, 6.15821, minDelayThreshold},
		},
		{
			name: "skipsOutliersAndMeasuresDeltaFromTheOutlier",
			values: []estimate{
				{5 * time.Millisecond, 0.2, 60},
				{50 * time.Millisecond, 0.2, 60},
				{10 * time.Millisecond, 0.09, 60},
			},
			expected: []float64{12.5, 12.5, 13.2917},
		},
		{
			name: "capsTheTimeDelta",
			values: []estimate{
				{5 * time.Millisecond, 0.09, 60},
				{500 * time.Millisecond, 0.09, 60},
			},
			expected: []float64{12.5, 20.417},
		},
		{
			name:             "clampsAtMaximum",
			initialThreshold: 599.9,
			values: []estimate{
				{5 * time.Millisecond, 610.0 / 240.0, 60},
				{100 * time.Millisecond, 610.0 / 240.0, 60},
			},
			expected: []float64{599.9, maxDelayThreshold},
		},
		{
			name: "adaptsOnTheMagnitudeOfTheTrend",
			values: []estimate{
				{5 * time.Millisecond, -0.09, 60},
				{10 * time.Millisecond, -0.09, 60},
			},
			expected: []float64{12.5, 13.2917},
		},
		{
			name: "ignoresEstimatesWithFewerThanTwoDeltas",
			values: []estimate{
				{10 * time.Millisecond, 1, 1},
				{10 * time.Millisecond, 1, 0},
			},
			expected: []float64{12.5, 12.5},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			od := newOveruseDetector()
			if tc.initialThreshold != 0 {
				od.delayThreshold = tc.initialThreshold
			}
			arrival := time.Time{}
			for i, e := range tc.values {
				arrival = arrival.Add(e.arrivalDelta)
				od.update(arrival, e.arrivalDelta, e.trend, e.numDeltas)
				assert.InDelta(t, tc.expected[i], od.delayThreshold, 1e-6, "sample %v", i)
			}
		})
	}
}
