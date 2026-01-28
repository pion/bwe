// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"time"

	"github.com/pion/logging"
)

type arrivalGroupItem struct {
	SequenceNumber uint64
	Departure      time.Time
	Arrival        time.Time
	Size           int
}

type arrivalGroup []arrivalGroupItem

type arrivalGroupAccumulator struct {
	log              logging.LeveledLogger
	next             arrivalGroup
	burstInterval    time.Duration
	maxBurstDuration time.Duration
}

func newArrivalGroupAccumulator() *arrivalGroupAccumulator {
	return &arrivalGroupAccumulator{
		log:              logging.NewDefaultLoggerFactory().NewLogger("bwe_arrival_group_accumulator"),
		next:             make([]arrivalGroupItem, 0),
		burstInterval:    5 * time.Millisecond,
		maxBurstDuration: 100 * time.Millisecond,
	}
}

func (a *arrivalGroupAccumulator) onPacketAcked(
	sequenceNumber uint64,
	size int,
	departure, arrival time.Time,
) arrivalGroup {
	if len(a.next) == 0 {
		a.next = append(a.next, arrivalGroupItem{
			SequenceNumber: sequenceNumber,
			Size:           size,
			Departure:      departure,
			Arrival:        arrival,
		})

		return nil
	}

	sendTimeDelta := departure.Sub(a.next[0].Departure)
	if sendTimeDelta < a.burstInterval {
		a.next = append(a.next, arrivalGroupItem{
			SequenceNumber: sequenceNumber,
			Size:           size,
			Departure:      departure,
			Arrival:        arrival,
		})

		return nil
	}

	arrivalTimeDeltaLast := arrival.Sub(a.next[len(a.next)-1].Arrival)
	arrivalTimeDeltaFirst := arrival.Sub(a.next[0].Arrival)
	propagationDelta := arrivalTimeDeltaFirst - sendTimeDelta

	if propagationDelta < 0 && arrivalTimeDeltaLast <= a.burstInterval && arrivalTimeDeltaFirst < a.maxBurstDuration {
		a.next = append(a.next, arrivalGroupItem{
			SequenceNumber: sequenceNumber,
			Size:           size,
			Departure:      departure,
			Arrival:        arrival,
		})

		return nil
	}

	a.log.Tracef("sendTimeDelta=%v, propagationDelta=%v, arrivalTimeDeltaLast=%v, arrivalTimeDeltaFirst=%v", sendTimeDelta, propagationDelta, arrivalTimeDeltaLast, arrivalTimeDeltaFirst)

	group := make(arrivalGroup, len(a.next))
	copy(group, a.next)
	a.next = arrivalGroup{arrivalGroupItem{
		SequenceNumber: sequenceNumber,
		Size:           size,
		Departure:      departure,
		Arrival:        arrival,
	}}

	return group
}
