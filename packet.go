// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package bwe

import (
	"fmt"
	"time"
)

// An Packet stores send and receive information about a packet.
type Packet struct {
	// StreamID is the ID of the stream to which the packet belongs. The
	// StreamID MUST be unique among all streams controlled by the congestion
	// controller.
	StreamID uint64

	// SequenceNumber is the sequence number of the packet within its stream.
	// SequenceNumbers of consecutive packets might have gaps.
	SequenceNumber uint64

	// TransportWideSequenceNumber is a transport wide sequence number of the
	// packet. It MUST be unique over all streams and it MUST increase by 1 for
	// every outgoing packet.
	TransportWideSequenceNumber uint64

	// Size is the size of the packet in bytes.
	Size int

	// Arrived indicates if the packet arrived at the receiver. False does not
	// necessarily mean the packet was lost, it might still be in transit.
	Arrived bool

	// Departure is the departure time of the packet taken at the sender. It
	// should be the time measured at the latest possible moment before sending
	// the packet.
	Departure time.Time

	// Arrival is the arrival time of the packet at the receiver. Arrival and
	// Departure do not require synchronized clocks and can therefore not
	// directly be compared.
	Arrival time.Time

	// ECN marking of the packet when it arrived.
	ECN ECN
}

func (a Packet) String() string {
	return fmt.Sprintf("seq=%v, departure=%v, arrival=%v", a.SequenceNumber, a.Departure, a.Arrival)
}
