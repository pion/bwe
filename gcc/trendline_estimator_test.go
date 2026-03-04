// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTrendlineEstimator(t *testing.T) {
	type value struct {
		arrivalTime     time.Time
		interGroupDelay time.Duration
		expectedTrend   float64
	}
	cases := []struct {
		name           string
		smoothingCoeff float64
		windowSize     int
		values         []value
	}{
		{
			name:           "single_value_zero",
			smoothingCoeff: 0.8,
			windowSize:     10,
			values: []value{
				{
					arrivalTime:     time.Time{},
					interGroupDelay: 0,
					expectedTrend:   0,
				},
			},
		},
		{
			name:           "single_value",
			smoothingCoeff: 0.8,
			windowSize:     10,
			values: []value{
				{
					arrivalTime:     time.Time{}.Add(time.Second),
					interGroupDelay: 30 * time.Millisecond,
					expectedTrend:   0,
				},
			},
		},
		{
			name:           "multiple_values",
			smoothingCoeff: 0, // no smoothing
			windowSize:     10,
			values: []value{
				{
					arrivalTime:     time.Time{}.Add(time.Second),
					interGroupDelay: 0,
					expectedTrend:   0,
				},
				{
					arrivalTime:     time.Time{}.Add(2 * time.Second),
					interGroupDelay: time.Second,
					expectedTrend:   1.0,
				},
			},
		},
		{
			name:           "multiple_values",
			smoothingCoeff: 0.8,
			windowSize:     10,
			values: []value{
				{
					arrivalTime:     time.Time{}.Add(time.Second),
					interGroupDelay: 0,
					expectedTrend:   0,
				},
				{
					arrivalTime:     time.Time{}.Add(2 * time.Second),
					interGroupDelay: time.Second,
					expectedTrend:   0.2,
				},
			},
		},
		{
			name:           "clear_window",
			smoothingCoeff: 0,
			windowSize:     2,
			values: []value{
				{
					arrivalTime:     time.Time{}.Add(time.Second),
					interGroupDelay: 0,
					expectedTrend:   0,
				},
				{
					arrivalTime:     time.Time{}.Add(2 * time.Second),
					interGroupDelay: time.Second,
					expectedTrend:   1,
				},
				{
					arrivalTime:     time.Time{}.Add(3 * time.Second),
					interGroupDelay: 2 * time.Second,
					expectedTrend:   2,
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			te := newTrendlineEstimator(
				trendlineEstimatorSmoothingCoeff(tc.smoothingCoeff),
				trendlineEstimatorWindowSize(tc.windowSize),
			)
			for _, v := range tc.values {
				trend := te.update(v.arrivalTime, v.interGroupDelay)
				assert.InDelta(t, v.expectedTrend, trend, 0.01)
			}
		})
	}
}
