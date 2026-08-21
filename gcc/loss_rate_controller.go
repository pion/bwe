// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import "time"

const (
	minLossWindow  = 200 * time.Millisecond
	minLossPackets = 20
)

type lossRateController struct {
	bitrate  int
	min, max float64

	windowStart            time.Time
	packetsSinceLastUpdate int
	lostSinceLastUpdate    int
}

func newLossRateController(initialRate, minRate, maxRate int) *lossRateController {
	return &lossRateController{
		bitrate:                initialRate,
		min:                    float64(minRate),
		max:                    float64(maxRate),
		windowStart:            time.Time{},
		packetsSinceLastUpdate: 0,
		lostSinceLastUpdate:    0,
	}
}

func (l *lossRateController) onPacketAcked() {
	l.packetsSinceLastUpdate++
}

func (l *lossRateController) onPacketLost() {
	l.packetsSinceLastUpdate++
	l.lostSinceLastUpdate++
}

func (l *lossRateController) lossRate() float64 {
	if l.packetsSinceLastUpdate == 0 {
		return 0
	}

	return float64(l.lostSinceLastUpdate) / float64(l.packetsSinceLastUpdate)
}

func (l *lossRateController) windowClosed(ts time.Time, rtt time.Duration) bool {
	return ts.Sub(l.windowStart) >= max(minLossWindow, rtt) &&
		l.packetsSinceLastUpdate >= minLossPackets
}

func (l *lossRateController) update(ts time.Time, rtt time.Duration, lastDeliveryRate int) (int, bool) {
	if l.windowStart.IsZero() {
		l.windowStart = ts
	}
	if !l.windowClosed(ts, rtt) {
		return l.bitrate, false
	}
	lossRate := l.lossRate()
	var target float64
	if lossRate > 0.1 {
		target = float64(l.bitrate) * (1 - 0.5*lossRate)
		target = max(target, l.min)
	} else if lossRate < 0.02 {
		target = float64(l.bitrate) * 1.05
		// Cap at 1.5 times the previously delivered rate to ensure we don't
		// increase the target rate indefinitely, while being application
		// limited.
		target = min(target, 1.5*float64(lastDeliveryRate))
		// Cap at previous target rate. In case lastDeliveryRate was much lower
		// than our target, we don't want to decrease the target rate.
		target = max(target, float64(l.bitrate))
		// Cap at configured max bitrate.
		target = min(target, l.max)
	}
	if target != 0 {
		l.bitrate = int(target)
	}

	l.windowStart = ts
	l.packetsSinceLastUpdate = 0
	l.lostSinceLastUpdate = 0

	return l.bitrate, true
}
