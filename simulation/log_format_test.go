// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js

package simulation

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type packetLogger struct {
	vantagePoint string
	direction    string
}

func (l *packetLogger) LogRTPPacket(header *rtp.Header, payload []byte, attributes interceptor.Attributes) {
	ts := time.Now()
	slog.Info(
		"rtp",
		"vantage-point", l.vantagePoint,
		"direction", l.direction,
		"ts", ts,
		"pt", header.PayloadType,
		"ssrc", header.SSRC,
		"sequence-number", header.SequenceNumber,
		"rtp-timestamp", header.Timestamp,
		"marker", header.Marker,
		"payload-size", len(payload),
	)
}

func (l *packetLogger) LogRTCPPackets(pkts []rtcp.Packet, attributes interceptor.Attributes) {
	for _, pkt := range pkts {
		slog.Info("rtcp", "vantage-point", l.vantagePoint, "direction", l.direction, "type", fmt.Sprintf("%T", pkt))
	}
}
