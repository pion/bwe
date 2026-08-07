// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"container/heap"
	"time"
)

type deliveryRateHeapItem struct {
	arrival time.Time
	size    int
}

type deliveryRateHeap []deliveryRateHeapItem

// Len implements heap.Interface.
func (d deliveryRateHeap) Len() int {
	return len(d)
}

// Less implements heap.Interface.
func (d deliveryRateHeap) Less(i int, j int) bool {
	return d[i].arrival.Before(d[j].arrival)
}

// Pop implements heap.Interface.
func (d *deliveryRateHeap) Pop() any {
	old := *d
	n := len(old)
	x := old[n-1]
	*d = old[0 : n-1]

	return x
}

// Push implements heap.Interface.
func (d *deliveryRateHeap) Push(x any) {
	// nolint
	*d = append(*d, x.(deliveryRateHeapItem))
}

// Swap implements heap.Interface.
func (d deliveryRateHeap) Swap(i int, j int) {
	d[i], d[j] = d[j], d[i]
}

type deliveryRateEstimator struct {
	window        time.Duration
	latestArrival time.Time
	// lastEvicted is the arrival time of the most recently evicted packet.
	// Everything left in the history arrived after it, which makes it the start
	// of the interval over which the delivered bytes are measured.
	lastEvicted time.Time
	// historyBytes is the total size of all packets in history.
	historyBytes int
	history      *deliveryRateHeap
}

func newDeliveryRateEstimator(window time.Duration) *deliveryRateEstimator {
	return &deliveryRateEstimator{
		window:        window,
		latestArrival: time.Time{},
		lastEvicted:   time.Time{},
		historyBytes:  0,
		history:       &deliveryRateHeap{},
	}
}

func (e *deliveryRateEstimator) onPacketAcked(arrival time.Time, size int) {
	if arrival.After(e.latestArrival) {
		e.latestArrival = arrival
	}
	e.historyBytes += size
	heap.Push(e.history, deliveryRateHeapItem{
		arrival: arrival,
		size:    size,
	})
}

func (e *deliveryRateEstimator) getRate() int {
	deadline := e.latestArrival.Add(-e.window)
	for len(*e.history) > 0 && (*e.history)[0].arrival.Before(deadline) {
		oldest := (*e.history)[0]
		heap.Pop(e.history)
		e.lastEvicted = oldest.arrival
		e.historyBytes -= oldest.size
	}
	if len(*e.history) == 0 {
		return 0
	}

	sum := e.historyBytes
	var start time.Time
	if e.lastEvicted.IsZero() {
		// Nothing has been evicted yet, so there is no measurement of when the
		// interval started. Use the oldest packet in the history, which is the
		// root of the heap, and don't count its size: it arrived at the very
		// start of the interval and therefore took none of the measured time to
		// deliver.
		oldest := (*e.history)[0]
		start = oldest.arrival
		sum -= oldest.size
	} else {
		// Evicted packets are older than the deadline by definition, so cap the
		// interval at the configured window. Without the cap an idle period
		// before the oldest packet in the history would stretch it.
		start = e.lastEvicted
		if start.Before(deadline) {
			start = deadline
		}
	}

	d := e.latestArrival.Sub(start)
	if d <= 0 {
		return 0
	}
	rate := 8 * float64(sum) / d.Seconds()

	return int(rate)
}
