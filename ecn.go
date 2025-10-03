// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package bwe

// ECN represents the ECN bits of an IP packet header.
type ECN uint8

const (
	// ECNNonECT signals Non ECN-Capable Transport, Non-ECT.
	// nolint:misspell
	ECNNonECT ECN = iota // 00

	// ECNECT1 signals ECN Capable Transport, ECT(0).
	// nolint:misspell
	ECNECT1 // 01

	// ECNECT0 signals ECN Capable Transport, ECT(1).
	// nolint:misspell
	ECNECT0 // 10

	// ECNCE signals ECN Congestion Encountered, CE.
	// nolint:misspell
	ECNCE // 11
)
