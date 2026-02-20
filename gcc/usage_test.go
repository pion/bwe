// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsageString(t *testing.T) {
	cases := []struct {
		name     string
		value    usage
		expected string
	}{
		{
			name:     "under",
			value:    usageUnder,
			expected: "underuse",
		},
		{
			name:     "normal",
			value:    usageNormal,
			expected: "normal",
		},
		{
			name:     "over",
			value:    usageOver,
			expected: "overuse",
		},
		{
			name:     "invalid",
			value:    17,
			expected: "invalid usage: 17",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
		})
	}
}
