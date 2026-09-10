// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package simulation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// collapsedRate is the delivered rate below which the controller counts
	// as collapsed. Every network in the matrix carries several times this,
	// so no working controller comes near it.
	collapsedRate = 2 * gccMinRate

	// steadyStateStart is where the window the assertions are evaluated over
	// begins. Everything before it is startup: connection setup, the initial
	// ramp and the first delay based backoff are all expected to look bad.
	steadyStateStart = 30 * time.Second
)

func assertScenario(
	t *testing.T,
	collected *metrics,
	network *virtualNetwork,
) {
	t.Helper()

	steady := collected.window(steadyStateStart, scenarioDuration)

	require.NotEmpty(t, collected.targetRates, "controller never reported a target rate")

	lowest, highest := collected.targetRateRange()
	assert.GreaterOrEqual(t, lowest, gccMinRate, "target rate dropped below the configured minimum")
	assert.LessOrEqual(t, highest, gccMaxRate, "target rate rose above the configured maximum")

	assert.Greater(
		t, steady.deliveredRate(), float64(collapsedRate),
		"controller collapsed: steady state delivered rate on a %v bps link", network.rate,
	)
}
