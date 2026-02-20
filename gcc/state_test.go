// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestState(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		assert.Equal(t, stateIncrease, state(17).transition(29))
	})

	t.Run("hold", func(t *testing.T) {
		assert.Equal(t, stateDecrease, stateHold.transition(usageOver))
		assert.Equal(t, stateIncrease, stateHold.transition(usageNormal))
		assert.Equal(t, stateHold, stateHold.transition(usageUnder))
		assert.Equal(t, stateIncrease, stateHold.transition(17))
	})

	t.Run("increase", func(t *testing.T) {
		assert.Equal(t, stateDecrease, stateIncrease.transition(usageOver))
		assert.Equal(t, stateIncrease, stateIncrease.transition(usageNormal))
		assert.Equal(t, stateHold, stateIncrease.transition(usageUnder))
		assert.Equal(t, stateIncrease, stateIncrease.transition(17))
	})

	t.Run("decrease", func(t *testing.T) {
		assert.Equal(t, stateDecrease, stateDecrease.transition(usageOver))
		assert.Equal(t, stateHold, stateDecrease.transition(usageNormal))
		assert.Equal(t, stateHold, stateDecrease.transition(usageUnder))
		assert.Equal(t, stateIncrease, stateDecrease.transition(17))
	})
}

func TestStateString(t *testing.T) {
	cases := []struct {
		name     string
		value    state
		expected string
	}{
		{
			name:     "decrease",
			value:    stateDecrease,
			expected: "decrease",
		},
		{
			name:     "hold",
			value:    stateHold,
			expected: "hold",
		},
		{
			name:     "increase",
			value:    stateIncrease,
			expected: "increase",
		},
		{
			name:     "invalid",
			value:    17,
			expected: "invalid state: 17",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
		})
	}
}
