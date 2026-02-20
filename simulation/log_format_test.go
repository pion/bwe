// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js && go1.25 && simulation

package simulation

import (
	"fmt"
	"log/slog"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const (
	maxSequenceNumberPlusOne = int64(65536)
	breakpoint               = 32768 // half of max uint16
)

type packetLogger struct {
	logger    *slog.Logger
	direction string
	seq       *unwrapper
}

func newPacketLogger(logger *slog.Logger, direction string) *packetLogger {
	return &packetLogger{
		logger:    logger,
		direction: direction,
		seq:       &unwrapper{},
	}
}

func (l *packetLogger) LogRTPPacket(header *rtp.Header, payload []byte, attributes interceptor.Attributes) {
	u := l.seq.Unwrap(header.SequenceNumber)
	l.logger.Info(
		"rtp",
		"direction", l.direction,
		"pt", header.PayloadType,
		"ssrc", header.SSRC,
		"sequence-number", header.SequenceNumber,
		"unwrapped-sequence-number", u,
		"rtp-timestamp", header.Timestamp,
		"marker", header.Marker,
		"payload-size", len(payload),
	)
}

func (l *packetLogger) LogRTCPPackets(pkts []rtcp.Packet, attributes interceptor.Attributes) {
	for _, pkt := range pkts {
		l.logger.Info(
			"rtcp",
			"direction", l.direction,
			"type", fmt.Sprintf("%T", pkt),
		)
	}
}

// Unwrapper stores an unwrapped sequence number.
type unwrapper struct {
	init          bool
	lastUnwrapped int64
}

func isNewer(value, previous uint16) bool {
	if value-previous == breakpoint {
		return value > previous
	}

	return value != previous && (value-previous) < breakpoint
}

// Unwrap unwraps the next sequencenumber.
func (u *unwrapper) Unwrap(i uint16) int64 {
	if !u.init {
		u.init = true
		u.lastUnwrapped = int64(i)

		return u.lastUnwrapped
	}

	lastWrapped := uint16(u.lastUnwrapped) //nolint:gosec // G115
	delta := int64(i - lastWrapped)
	if isNewer(i, lastWrapped) {
		if delta < 0 {
			delta += maxSequenceNumberPlusOne
		}
	} else if delta > 0 && u.lastUnwrapped+delta-maxSequenceNumberPlusOne >= 0 {
		delta -= maxSequenceNumberPlusOne
	}

	u.lastUnwrapped += delta

	return u.lastUnwrapped
}
