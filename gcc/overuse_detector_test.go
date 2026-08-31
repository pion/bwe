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
		sendDelta time.Duration
		trend     float64
		numDeltas int
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
				{5 * time.Millisecond, 0, 60},
				{5 * time.Millisecond, 0.2, 60},
				{15 * time.Millisecond, 0.5, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name: "normalUseWithFewerThanTwoDeltas",
			values: []estimate{
				{5 * time.Millisecond, 1, 0},
				{5 * time.Millisecond, 1, 1},
			},
			expected: []usage{usageNormal, usageNormal},
		},
		{
			name: "normaluse",
			values: []estimate{
				{sendDelta: 5 * time.Millisecond, trend: 0, numDeltas: 60},
			},
			expected: []usage{usageNormal},
		},
		{
			name: "confirmsUnderUse",
			values: []estimate{
				{5 * time.Millisecond, -0.2, 60},
			},
			expected: []usage{usageUnder},
		},
		{
			name: "noOveruseUntilCounterExceedsOne",
			values: []estimate{
				{time.Millisecond, 0, 60},
				{20 * time.Millisecond, 0.25, 60},
				{5 * time.Millisecond, 0.5, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name: "noNewOveruseIfEstimateDecreased",
			values: []estimate{
				{time.Millisecond, 0, 60},
				{9 * time.Millisecond, 0.4, 60},
				{20 * time.Millisecond, 0.3, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageNormal},
		},
		{
			name: "noOveruseIfRawTrendDecreasesWhileDeltaCountGrows",
			values: []estimate{
				{time.Millisecond, 0, 10},
				{9 * time.Millisecond, 1, 20},
				{20 * time.Millisecond, 0.9, 40},
			},
			expected: []usage{usageNormal, usageNormal, usageNormal},
		},
		{
			name: "normalUseWhenTrendFallsBelowThreshold",
			values: []estimate{
				{time.Millisecond, 0, 60},
				{9 * time.Millisecond, 0.4, 60},
				{20 * time.Millisecond, 0.5, 60},
				{5 * time.Millisecond, 0.005, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver, usageNormal},
		},
		{
			name: "noOveruseBeforeTimeThreshold",
			values: []estimate{
				{time.Millisecond, 0.010, 60},
				{time.Millisecond, 0.011, 60},
				{time.Millisecond, 0.012, 60},
				{time.Millisecond, 0.013, 60},
				{time.Millisecond, 0.014, 60},
				{time.Millisecond, 0.015, 60},
			},
			expected: []usage{
				usageNormal, usageNormal, usageNormal,
				usageNormal, usageNormal, usageOver,
			},
		},
		{
			name: "underuseToOveruse",
			values: []estimate{
				{5 * time.Millisecond, -0.2, 60},
				{5 * time.Millisecond, 0.01, 60},
				{5 * time.Millisecond, 0.02, 60},
			},
			expected: []usage{usageUnder, usageUnder, usageOver},
		},
		{
			name: "keepsOveruseWhileTrendDecreasesAboveThreshold",
			values: []estimate{
				{5 * time.Millisecond, 0, 60},
				{5 * time.Millisecond, 0.01, 60},
				{5 * time.Millisecond, 0.02, 60},
				{5 * time.Millisecond, 0.015, 60},
				{5 * time.Millisecond, 0.014, 60},
			},
			expected: []usage{
				usageNormal, usageNormal, usageOver, usageOver, usageOver,
			},
		},
		{
			name: "keepsOveruseWhileTrendStaysHigh",
			values: []estimate{
				{5 * time.Millisecond, 0, 60},
				{5 * time.Millisecond, 0.004, 60},
				{5 * time.Millisecond, 0.00625, 60},
				{5 * time.Millisecond, 0.008, 60},
				{5 * time.Millisecond, 0.01, 60},
				{5 * time.Millisecond, 0.012, 60},
				{5 * time.Millisecond, 0.014, 60},
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
			received := []usage{}
			for _, e := range tc.values {
				u := od.update(e.sendDelta, e.trend, e.numDeltas)
				received = append(received, u)
			}
			assert.Equal(t, tc.expected, received)
		})
	}
}
