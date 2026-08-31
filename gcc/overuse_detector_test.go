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
		ts        time.Time
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
			name: "confirmsOverUse",
			values: []estimate{
				{time.Time{}, 0, 60},
				{time.Time{}.Add(5 * time.Millisecond), 0.2, 60},
				{time.Time{}.Add(20 * time.Millisecond), 0.5, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name:     "normaluse",
			values:   []estimate{{trend: 0, numDeltas: 60}},
			expected: []usage{usageNormal},
		},
		{
			name:     "confirmsUnderUse",
			values:   []estimate{{time.Time{}, -0.2, 60}},
			expected: []usage{usageUnder},
		},
		{
			name: "noOverUseBeforeDelay",
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 0, 60},
				{time.Time{}.Add(2 * time.Millisecond), 0.25, 60},
				{time.Time{}.Add(30 * time.Millisecond), 0.5, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name: "noNewOverUseIfEstimateDecreased",
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 0, 60},
				{time.Time{}.Add(10 * time.Millisecond), 0.4, 60},
				{time.Time{}.Add(30 * time.Millisecond), 0.3, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageNormal},
		},
		{
			name: "noOverUseIfRawTrendDecreasesWhileDeltaCountGrows",
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 0, 10},
				{time.Time{}.Add(10 * time.Millisecond), 1, 20},
				{time.Time{}.Add(30 * time.Millisecond), 0.9, 40},
			},
			expected: []usage{usageNormal, usageNormal, usageNormal},
		},
		{
			name: "normalUseWhenTrendFallsBelowThreshold",
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 0, 60},
				{time.Time{}.Add(10 * time.Millisecond), 0.4, 60},
				{time.Time{}.Add(30 * time.Millisecond), 0.5, 60},
				{time.Time{}.Add(35 * time.Millisecond), 0.005, 60},
			},
			expected: []usage{usageNormal, usageNormal, usageOver, usageNormal},
		},
		{
			name: "keepsOverUseWhileTrendStaysHigh",
			values: []estimate{
				{time.Time{}.Add(5 * time.Millisecond), 0, 60},
				{time.Time{}.Add(10 * time.Millisecond), 0.004, 60},
				{time.Time{}.Add(15 * time.Millisecond), 0.00625, 60},
				{time.Time{}.Add(20 * time.Millisecond), 0.008, 60},
				{time.Time{}.Add(25 * time.Millisecond), 0.01, 60},
				{time.Time{}.Add(30 * time.Millisecond), 0.012, 60},
				{time.Time{}.Add(35 * time.Millisecond), 0.014, 60},
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
				u := od.update(e.ts, e.trend, e.numDeltas)
				received = append(received, u)
			}
			assert.Equal(t, tc.expected, received)
		})
	}
}
