// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"time"
)

type arrivalGroupItem struct {
	SequenceNumber uint64
	Departure      time.Time
	Arrival        time.Time
	Size           int
}

// arrivalGroup is a set of packets grouped by send time. departure and
// arrival are the max Departure and Arrival across items.
type arrivalGroup struct {
	items     []arrivalGroupItem
	departure time.Time
	arrival   time.Time
}

func (g arrivalGroup) empty() bool {
	return len(g.items) == 0
}

func (g *arrivalGroup) append(item arrivalGroupItem) {
	g.items = append(g.items, item)
	if item.Departure.After(g.departure) {
		g.departure = item.Departure
	}
	if item.Arrival.After(g.arrival) {
		g.arrival = item.Arrival
	}
}

type arrivalGroupAccumulator struct {
	next             arrivalGroup
	burstInterval    time.Duration
	maxBurstDuration time.Duration
}

func newArrivalGroupAccumulator() *arrivalGroupAccumulator {
	return &arrivalGroupAccumulator{
		burstInterval:    5 * time.Millisecond,
		maxBurstDuration: 5 * time.Millisecond,
	}
}

// onPacketAcked returns the completed group when the packet closes one, or
// nil when it was added to the group still being accumulated.
func (a *arrivalGroupAccumulator) onPacketAcked(
	sequenceNumber uint64,
	size int,
	departure, arrival time.Time,
) *arrivalGroup {
	item := arrivalGroupItem{
		SequenceNumber: sequenceNumber,
		Size:           size,
		Departure:      departure,
		Arrival:        arrival,
	}

	if a.next.empty() {
		a.next.append(item)

		return nil
	}

	sendTimeDelta := departure.Sub(a.next.items[0].Departure)
	if sendTimeDelta < a.burstInterval {
		a.next.append(item)

		return nil
	}

	arrivalTimeDeltaFirst := arrival.Sub(a.next.items[0].Arrival)
	propagationDelta := arrivalTimeDeltaFirst - sendTimeDelta

	if propagationDelta < 0 && arrivalTimeDeltaFirst < a.maxBurstDuration {
		a.next.append(item)

		return nil
	}

	group := a.next
	a.next = arrivalGroup{}
	a.next.append(item)

	return &group
}
