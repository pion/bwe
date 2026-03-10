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
		ts            time.Time
		modifiedTrend float64
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
				{time.Time{}, 0},
				{time.Time{}.Add(5 * time.Millisecond), 40},
				{time.Time{}.Add(20 * time.Millisecond), 90},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name:     "normaluse",
			values:   []estimate{{modifiedTrend: 0}},
			expected: []usage{usageNormal},
		},
		{
			name:     "confirmsUnderUse",
			values:   []estimate{{time.Time{}, -40}},
			expected: []usage{usageUnder},
		},
		{
			name: "noOverUseBeforeDelay",
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 0},
				{time.Time{}.Add(2 * time.Millisecond), 60},
				{time.Time{}.Add(30 * time.Millisecond), 150},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name: "noNewOverUseIfEstimateDecreased",
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 0},
				{time.Time{}.Add(10 * time.Millisecond), 80},
				{time.Time{}.Add(30 * time.Millisecond), 60},
			},
			expected: []usage{usageNormal, usageNormal, usageNormal},
		},
		{
			name: "normalUseWhenTrendFallsBelowThreshold",
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 0},
				{time.Time{}.Add(10 * time.Millisecond), 80},
				{time.Time{}.Add(30 * time.Millisecond), 150},
				{time.Time{}.Add(35 * time.Millisecond), 1.2},
			},
			expected: []usage{usageNormal, usageNormal, usageOver, usageNormal},
		},
		{
			name: "keepsOverUseWhileTrendStaysHigh",
			values: []estimate{
				{time.Time{}.Add(5 * time.Millisecond), 0},
				{time.Time{}.Add(10 * time.Millisecond), 1},
				{time.Time{}.Add(15 * time.Millisecond), 1.5},
				{time.Time{}.Add(20 * time.Millisecond), 2},
				{time.Time{}.Add(25 * time.Millisecond), 2.5},
				{time.Time{}.Add(30 * time.Millisecond), 3},
				{time.Time{}.Add(35 * time.Millisecond), 3.5},
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
				u := od.update(e.ts, e.modifiedTrend)
				received = append(received, u)
			}
			assert.Equal(t, tc.expected, received)
		})
	}
}
