// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestArrivalGroupAccumulator(t *testing.T) {
	type logItem struct {
		SequenceNumber uint64
		Departure      time.Time
		Arrival        time.Time
	}
	triggerNewGroupElement := logItem{
		Departure: time.Time{}.Add(time.Second),
		Arrival:   time.Time{}.Add(time.Second),
	}
	cases := []struct {
		name string
		log  []logItem
		exp  []arrivalGroup
	}{
		{
			name: "emptyCreatesNoGroups",
			log:  []logItem{},
			exp:  []arrivalGroup{},
		},
		{
			name: "createsSingleElementGroup",
			log: []logItem{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					{
						Departure: time.Time{},
						Arrival:   time.Time{}.Add(time.Millisecond),
					},
				},
			},
		},
		{
			name: "createsTwoElementGroup",
			log: []logItem{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
			}},
		},
		{
			name: "createsTwoArrivalGroups1",
			log: []logItem{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(9 * time.Millisecond),
					Arrival:   time.Time{}.Add(24 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					{
						Departure: time.Time{},
						Arrival:   time.Time{}.Add(15 * time.Millisecond),
					},
					{
						Departure: time.Time{}.Add(3 * time.Millisecond),
						Arrival:   time.Time{}.Add(20 * time.Millisecond),
					},
				},
				{
					{
						Departure: time.Time{}.Add(9 * time.Millisecond),
						Arrival:   time.Time{}.Add(24 * time.Millisecond),
					},
				},
			},
		},
		{
			name: "ignoresOutOfOrderPackets",
			log: []logItem{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(6 * time.Millisecond),
					Arrival:   time.Time{}.Add(34 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(8 * time.Millisecond),
					Arrival:   time.Time{}.Add(30 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					{
						Departure: time.Time{},
						Arrival:   time.Time{}.Add(15 * time.Millisecond),
					},
				},
				{
					{
						Departure: time.Time{}.Add(6 * time.Millisecond),
						Arrival:   time.Time{}.Add(34 * time.Millisecond),
					},
					{
						Departure: time.Time{}.Add(8 * time.Millisecond),
						Arrival:   time.Time{}.Add(30 * time.Millisecond),
					},
				},
			},
		},
		{
			name: "newGroupBecauseOfInterDepartureTime",
			log: []logItem{
				{
					SequenceNumber: 0,
					Departure:      time.Time{},
					Arrival:        time.Time{}.Add(4 * time.Millisecond),
				},
				{
					SequenceNumber: 1,
					Departure:      time.Time{}.Add(3 * time.Millisecond),
					Arrival:        time.Time{}.Add(4 * time.Millisecond),
				},
				{
					SequenceNumber: 2,
					Departure:      time.Time{}.Add(6 * time.Millisecond),
					Arrival:        time.Time{}.Add(10 * time.Millisecond),
				},
				{
					SequenceNumber: 3,
					Departure:      time.Time{}.Add(9 * time.Millisecond),
					Arrival:        time.Time{}.Add(10 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					{
						SequenceNumber: 0,
						Departure:      time.Time{},
						Arrival:        time.Time{}.Add(4 * time.Millisecond),
					},
					{
						SequenceNumber: 1,
						Departure:      time.Time{}.Add(3 * time.Millisecond),
						Arrival:        time.Time{}.Add(4 * time.Millisecond),
					},
				},
				{
					{
						SequenceNumber: 2,
						Departure:      time.Time{}.Add(6 * time.Millisecond),
						Arrival:        time.Time{}.Add(10 * time.Millisecond),
					},
					{
						SequenceNumber: 3,
						Departure:      time.Time{}.Add(9 * time.Millisecond),
						Arrival:        time.Time{}.Add(10 * time.Millisecond),
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aga := newArrivalGroupAccumulator()
			received := []arrivalGroup{}
			for _, ack := range tc.log {
				next := aga.onPacketAcked(ack.SequenceNumber, 0, ack.Departure, ack.Arrival)
				if next != nil {
					received = append(received, next)
				}
			}
			assert.Equal(t, tc.exp, received)
		})
	}
}
