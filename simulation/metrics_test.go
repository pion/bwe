// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package simulation

import (
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/pion/rtp"
)

type packetTrace struct {
	// size is the RTP header plus payload. The shaper bills the full SRTP
	// packet, so the wire carries another 10 byte auth tag per packet.
	size      int
	departure time.Time
	arrival   time.Time
}

// rateSample is one target rate the controller reported, and when.
type rateSample struct {
	time time.Time
	rate int
}

type metrics struct {
	mu sync.Mutex

	packets []packetTrace
	// index maps an unwrapped RTP sequence number to the position of that
	// packet in packets.
	index map[int64]int
	// sentSeq and arrivedSeq unwrap the sequence numbers of each direction
	// separately. Both start from the same first packet, so they agree.
	sentSeq    unwrapper
	arrivedSeq unwrapper

	targetRates []rateSample

	// sentBytes and sentPayloadBytes are the running totals the observed
	// header overhead is derived from.
	sentBytes        int
	sentPayloadBytes int
}

func newMetrics() *metrics {
	return &metrics{
		mu:          sync.Mutex{},
		packets:     []packetTrace{},
		index:       map[int64]int{},
		targetRates: []rateSample{},
	}
}

func (m *metrics) onSent(header *rtp.Header, payload []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	size := header.MarshalSize() + len(payload)
	m.sentBytes += size
	m.sentPayloadBytes += len(payload)

	m.index[m.sentSeq.Unwrap(header.SequenceNumber)] = len(m.packets)
	m.packets = append(m.packets, packetTrace{
		size:      size,
		departure: time.Now(),
		arrival:   time.Time{},
	})
}

func (m *metrics) onArrived(header *rtp.Header, _ []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	i, ok := m.index[m.arrivedSeq.Unwrap(header.SequenceNumber)]
	if !ok {
		return
	}
	m.packets[i].arrival = time.Now()
}

func (m *metrics) onTargetRate(rate int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.targetRates = append(m.targetRates, rateSample{time: time.Now(), rate: rate})
}

// headerOverheadFactor is the size of the sent packets divided by the size of
// the payloads they carried, so it is the share the RTP headers add to a
// stream.
func (m *metrics) headerOverheadFactor() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sentPayloadBytes == 0 {
		return 1
	}

	return float64(m.sentBytes) / float64(m.sentPayloadBytes)
}

func (m *metrics) targetRateRange() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.targetRates) == 0 {
		return 0, 0
	}

	lowest, highest := m.targetRates[0].rate, m.targetRates[0].rate
	for _, sample := range m.targetRates[1:] {
		lowest = min(lowest, sample.rate)
		highest = max(highest, sample.rate)
	}

	return lowest, highest
}

// window returns the statistics of every packet that was sent between start
// and end, both relative to the departure of the first packet of the scenario.
//
// Windowing is by departure time throughout, including for the delivered
// bytes: a packet that was sent inside the window but arrives just after it
// still counts as delivered. That keeps the delivered rate and the loss rate
// based on the same set of packets, and the error it introduces at the window
// edges is one network delay out of a window of tens of seconds.
func (m *metrics) window(start, end time.Duration) windowStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := windowStats{
		duration:       end - start,
		sentBytes:      0,
		arrivedBytes:   0,
		sentPackets:    0,
		arrivedPackets: 0,
		queuingDelays:  []time.Duration{},
	}
	if len(m.packets) == 0 {
		return stats
	}

	propagationDelay := m.minOneWayDelay()
	first := m.packets[0].departure
	from, to := first.Add(start), first.Add(end)

	for _, packet := range m.packets {
		if packet.departure.Before(from) || !packet.departure.Before(to) {
			continue
		}
		stats.sentPackets++
		stats.sentBytes += packet.size
		if packet.arrival.IsZero() {
			continue
		}
		stats.arrivedPackets++
		stats.arrivedBytes += packet.size
		stats.queuingDelays = append(
			stats.queuingDelays,
			packet.arrival.Sub(packet.departure)-propagationDelay,
		)
	}
	slices.Sort(stats.queuingDelays)

	return stats
}

const (
	// rampWindow is the sliding window the delivered rate is measured over
	// while looking for the point where the flow reached the link.
	rampWindow = 1 * time.Second
	// rampStep is how far that window slides between measurements, and so the
	// resolution of the ramp time.
	rampStep = 200 * time.Millisecond
	// rampFraction is the share of the achievable rate a flow has to deliver
	// to count as ramped up.
	rampFraction = 0.9
)

// rampTime returns how long after its first packet the flow first delivered
// rampFraction of achievable over a rampWindow long window, or -1 if it never
// got there. The -1 keeps "never ramped up" distinguishable from "ramped up
// immediately", which a zero would not.
//
// achievable is the rate the flow could have reached: the bottleneck rate, or
// the application limit where that is lower. Note that it is a payload rate
// while the delivered rate includes the RTP headers, so an application limited
// flow passes the threshold slightly early.
func (m *metrics) rampTime(achievable float64) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.packets) == 0 || achievable <= 0 {
		return -1
	}

	// Bucket the delivered bytes by departure, so that sliding the window is a
	// rolling sum over the buckets rather than a rescan of every packet.
	first := m.packets[0].departure
	last := m.packets[len(m.packets)-1].departure
	buckets := make([]int, int(last.Sub(first)/rampStep)+1)
	for _, packet := range m.packets {
		if packet.arrival.IsZero() {
			continue
		}
		buckets[packet.departure.Sub(first)/rampStep] += packet.size
	}

	perWindow := int(rampWindow / rampStep)
	delivered := 0
	for i, bytes := range buckets {
		delivered += bytes
		if i >= perWindow {
			delivered -= buckets[i-perWindow]
		}
		if i+1 < perWindow {
			continue
		}
		if bitsPerSecond(delivered, rampWindow) >= rampFraction*achievable {
			return time.Duration(i+1) * rampStep
		}
	}

	return -1
}

func (m *metrics) minOneWayDelay() time.Duration {
	minimum := time.Duration(0)
	for _, packet := range m.packets {
		if packet.arrival.IsZero() {
			continue
		}
		delay := packet.arrival.Sub(packet.departure)
		if minimum == 0 || delay < minimum {
			minimum = delay
		}
	}

	return minimum
}

type windowStats struct {
	duration       time.Duration
	sentBytes      int
	arrivedBytes   int
	sentPackets    int
	arrivedPackets int
	// queuingDelays holds, sorted ascending, the one way delay of every
	// arrived packet minus the propagation delay of the path. What remains is
	// the time the packet spent waiting in a queue.
	queuingDelays []time.Duration
}

func (s windowStats) sentRate() float64 {
	return bitsPerSecond(s.sentBytes, s.duration)
}

func (s windowStats) deliveredRate() float64 {
	return bitsPerSecond(s.arrivedBytes, s.duration)
}

func (s windowStats) lossRate() float64 {
	if s.sentPackets == 0 {
		return 0
	}

	return 1 - float64(s.arrivedPackets)/float64(s.sentPackets)
}

func (s windowStats) queuingDelayPercentile(pct float64) time.Duration {
	if len(s.queuingDelays) == 0 {
		return 0
	}
	rank := int(pct * float64(len(s.queuingDelays)))

	return s.queuingDelays[min(rank, len(s.queuingDelays)-1)]
}

func (s windowStats) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Float64("sent-rate", s.sentRate()),
		slog.Float64("delivered-rate", s.deliveredRate()),
		slog.Float64("loss-rate", s.lossRate()),
		slog.Duration("queuing-delay-p50", s.queuingDelayPercentile(0.5)),
		slog.Duration("queuing-delay-p95", s.queuingDelayPercentile(0.95)),
		slog.Duration("queuing-delay-max", s.queuingDelayPercentile(1)),
		slog.Int("sent-packets", s.sentPackets),
		slog.Int("arrived-packets", s.arrivedPackets),
	)
}

func bitsPerSecond(bytes int, duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}

	return 8 * float64(bytes) / duration.Seconds()
}
