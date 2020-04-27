package sleepy

import (
	"fmt"
	"github.com/lithdew/bytesutil"
	"github.com/valyala/bytebufferpool"
	"math"
)

type EndpointDispatcher interface {
	Transmit(seq uint16, buf []byte)
	Process(seq uint16, buf []byte)
	ACK(seq uint16)
}

type Endpoint struct {
	config *Config

	dispatcher EndpointDispatcher
	seq        uint16

	time       float64
	rtt        float64
	packetLoss float64

	sentBandwidthKbps     float64
	receivedBandwidthKbps float64
	ackedBandwidthKbps    float64

	sent      *SentPacketBuffer
	recv      *RecvPacketBuffer
	assembler *FragmentReassemblyBuffer

	pool bytebufferpool.Pool
}

func NewEndpoint(dispatcher EndpointDispatcher, config *Config) *Endpoint {
	if config == nil {
		config = NewConfig()
	}

	e := &Endpoint{
		config:     config,
		dispatcher: dispatcher,
	}

	e.sent = NewSentPacketBuffer(uint16(e.config.SentPacketBufferSize))
	e.recv = NewRecvPacketBuffer(uint16(e.config.RecvPacketBufferSize))
	e.assembler = NewFragmentReassemblyBuffer(uint16(e.config.FragmentReassemblyBufferSize))

	return e
}

func (e *Endpoint) SendPacket(buf []byte) (written int) {
	seq, size := e.seq, uint(len(buf))

	if size > e.config.MaxPacketSize {
		return 0
	}

	// Increment the last sent sequence number for the next packet.

	e.seq++

	// Insert the sequence number into our buffer to indicate that we are waiting for an ACK from our peer that
	// our packet was successfully received by them.

	packet := e.sent.Insert(seq)
	packet.Reset()

	packet.time = e.time
	packet.size = e.config.PacketHeaderSize + size

	// Get the last latest acknowledge sequence number, and a bitset of the last 32 acknowledged packet sequence
	// numbers.

	ack, acks := e.recv.NextACK()

	// Allocate a byte buffer from a pool of buffers dedicated to this endpoint.

	scratch := e.pool.Get()
	defer e.pool.Put(scratch)

	// Create the packets header with the assigned sequence number, last latest acknowledeged sequence number, and a
	// bitset of the last 32 acknowledged packet sequence numbers.

	header := PacketHeader{
		seq:  seq,
		ack:  ack,
		acks: acks,
	}

	// If the packet is small enough, we don't need to fragment it and can prepend a header to it and directly
	// send it out. Otherwise, we will fragment the packet out.

	if size <= e.config.FragmentAbove {
		// Allocate enough space for the packet header and the packets data.
		scratch.B = bytesutil.ExtendSlice(scratch.B, int(MaxPacketHeaderSize+size))

		// Write down the packet header, and copy the packets data into the rest of the scratch buffer.
		written = len(header.AppendTo(scratch.B[:0]))
		written += copy(scratch.B[written:], buf)

		// Write to the connection all data written to the scratch buffer.
		e.dispatcher.Transmit(seq, scratch.B[:written])

		return written
	}

	// Figure out how many fragments we need to partition our data into.

	total := size / e.config.FragmentSize
	if size%e.config.FragmentSize != 0 {
		total++
	}

	// Generate fragment header.
	fh := FragmentHeader{seq: header.seq, total: uint8(total - 1)}

	// Allocate enough space for the fragment header, the packet header, and the fragments data.
	scratch.B = bytesutil.ExtendSlice(scratch.B[:0], int(FragmentHeaderSize+MaxPacketHeaderSize+e.config.FragmentSize))

	for id := uint(0); id < total; id++ {
		scratch.B = scratch.B[:0]

		// Write fragment header.

		fh.id = uint8(id)
		scratch.B = fh.AppendTo(scratch.B)

		// For the first fragment, write the packet header.

		if id == 0 {
			scratch.B = header.AppendTo(scratch.B)
		}

		// Write the fragments data, capped at most FragmentSize bytes.

		cutoff := uint(len(buf))
		if cutoff > e.config.FragmentSize {
			cutoff = e.config.FragmentSize
		}

		scratch.B, buf = append(scratch.B, buf[:cutoff]...), buf[cutoff:]

		// Write the fragment to the connection.

		e.dispatcher.Transmit(seq, scratch.B)

		// Keep track of the total number of bytes written to the connection.

		written += len(scratch.B)
	}

	return written
}

func (e *Endpoint) RecvPacket(buf []byte) error {
	// If the first bit is set, process the packet as a fragmented packet. Otherwise, process it as a
	// compact, non-fragmented packet.

	if flag := PacketHeaderFlag(buf[0]); flag.Toggled(FlagFragment) {
		return e.recvFragmentedPacket(buf)
	}

	return e.recvCompactPacket(buf)
}

func (e *Endpoint) recvFragmentedPacket(buf []byte) error {
	// Decode fragment header from buf, and validate it.

	var (
		header FragmentHeader
		err    error
	)

	header, buf, err = UnmarshalFragmentHeader(buf)
	if err != nil {
		return fmt.Errorf("failed to decode fragment header: %w", err)
	}

	if err := header.Validate(e.config.MaxFragments); err != nil {
		return fmt.Errorf("got invalid fragment header: %w", err)
	}

	// If we received the first partition, decode the packet header that should have followed after the fragment header
	// and validate it against the fragment header. Keep a copy of the packet headers should it be valid for later
	// processing the entire assembled packet as a single, compact un-fragmented packet.

	phb := buf

	if header.id == 0 {
		var ph PacketHeader

		ph, buf, err = UnmarshalPacketHeader(buf)
		if err != nil {
			return fmt.Errorf("failed to unmarshal packet header in fragment header: %w", err)
		}

		if header.seq != ph.seq {
			return fmt.Errorf("got seq %d from packet header in fragment, but expected seq %d", header.seq, ph.seq)
		}

		phb = phb[:len(phb)-len(buf)]
	}

	// See if we have this particular packet sequence number being assembled as a fragment right now. If not,
	// instantiate the assembly of the fragmented packet by its sequence number.

	entry := e.assembler.Find(header.seq)
	if entry == nil {
		entry = e.assembler.Insert(header.seq)
		if entry == nil {
			return fmt.Errorf("got invalid fragment with sequence number %d: failed to insert into reassembly buffer",
				header.seq,
			)
		}
		entry.Reset()

		entry.total = uint(header.total) + 1

		// Instantiate a scratch buffer of max packet byte capacity that is to be used for assembling together incoming
		// fragment partitions.

		entry.buf = e.pool.Get()
		entry.buf.B = bytesutil.ExtendSlice(entry.buf.B, int(MaxPacketHeaderSize+entry.total*e.config.FragmentSize))
	}

	// Assert that the total fragment count is what is expected, and that the specific fragment we received by its
	// ID has not been marked to have been received before.

	if uint(header.total)+1 != entry.total {
		return fmt.Errorf("got invalid fragment: total fragment count mismatch (expected %d, got %d)",
			entry.total,
			uint(header.total)+1,
		)
	}

	// If we have not yet received this particular fragment ID, mark that we have received it.

	if err := entry.MarkReceived(header.id); err != nil {
		return err
	}

	// Leaves a gap in the front of the buffer that is to be removed once the fragment is fully assembled should
	// we have received the first fragment which contains the packet header. If we received the last fragment, we
	// are able to compute the entire assembled packets size. For any fragment that is received, copy its data
	// into the assemblers scratch buffer.

	if header.id == 0 {
		entry.headerSize = uint(copy(entry.buf.B[MaxPacketHeaderSize-uint(len(phb)):], phb))
	}

	if header.id == header.total {
		entry.packetSize = uint(header.total)*e.config.FragmentSize + uint(len(buf))
	}

	copy(entry.buf.B[MaxPacketHeaderSize+uint(header.id)*e.config.FragmentSize:], buf)

	// Increment the number of fragment partitions we have received. If we have received all the fragments, assemble
	// it together in one packet and process it as a compact, non-fragmented packet.

	entry.recv++
	if entry.recv == entry.total {
		buf := entry.buf.B[MaxPacketHeaderSize-entry.headerSize : MaxPacketHeaderSize+entry.packetSize]

		err := e.recvCompactPacket(buf)
		if err != nil {
			err = fmt.Errorf("failed to recv reassembled packet: %w", err)
		}

		e.assembler.Remove(header.seq)
		e.pool.Put(entry.buf)

		return err
	}

	return nil
}

func (e *Endpoint) recvCompactPacket(buf []byte) error {
	var (
		header PacketHeader
		err    error
	)

	// Unmarshal packet header.

	header, buf, err = UnmarshalPacketHeader(buf)
	if err != nil {
		return fmt.Errorf("failed to unmarshal packet header: %w", err)
	}

	// Mark packets that have been ACKed by our peer.

	recv := e.recv.Insert(header.seq)
	if recv == nil {
		return fmt.Errorf("packet received w/ sequence number %d is stale", header.seq)
	}

	// Process the packets contents now that it has successfully been received.

	e.dispatcher.Process(header.seq, buf)

	recv.Reset()

	recv.time = e.time
	recv.size = e.config.PacketHeaderSize + uint(len(buf))

	// Mark new ACKs from our peer.

	e.processACKs(header.ack, header.acks)

	return nil
}

func (e *Endpoint) processACKs(ack uint16, bitset uint32) {
	for i := uint16(0); i < 32; i, bitset = i+1, bitset>>1 {
		if bitset&1 == 0 {
			continue
		}

		seq := ack - i

		sent := e.sent.Find(seq)
		if sent == nil || sent.acked {
			continue
		}

		sent.acked = true

		// Mark the packet sequence number as ACK'ed.

		e.dispatcher.ACK(seq)

		rtt := (e.time - sent.time) * 1000
		if e.rtt == 0 && rtt > 0 || math.Abs(e.rtt-rtt) < 0.00001 {
			e.rtt = rtt
		} else {
			e.rtt += (rtt - e.rtt) * e.config.RTTSmoothingFactor
		}
	}
}

func (e *Endpoint) Update(time float64) {
	e.time = time
	e.updateStatistics()
}

func (e *Endpoint) updateStatistics() {
	sentBase, sentSamples := (e.sent.buf.latest-uint16(cap(e.sent.entries))+1)+0xFFFF, e.config.SentPacketBufferSize/2
	recvBase, recvSamples := (e.recv.buf.latest-uint16(cap(e.recv.entries))+1)+0xFFFF, e.config.RecvPacketBufferSize/2

	dropped := 0

	written, startWriting, finishWriting := 0, math.MaxFloat64, float64(0)
	acked, startACKing, finishACKing := 0, math.MaxFloat64, float64(0)
	received, startReceiving, finishReceiving := 0, math.MaxFloat64, float64(0)

	for i := uint(0); i < sentSamples; i++ {
		entry := e.sent.Find(sentBase + uint16(i))
		if entry == nil {
			continue
		}

		if !entry.acked {
			dropped++
		} else {
			acked += int(entry.size)
			if entry.time < startACKing {
				startACKing = entry.time
			}
			if entry.time > finishACKing {
				finishACKing = entry.time
			}
		}

		written += int(entry.size)
		if entry.time < startWriting {
			startWriting = entry.time
		}
		if entry.time > finishWriting {
			finishWriting = entry.time
		}
	}

	for i := uint(0); i < recvSamples; i++ {
		entry := e.recv.Find(recvBase + uint16(i))
		if entry == nil {
			continue
		}

		received += int(entry.size)
		if entry.time < startReceiving {
			startReceiving = entry.time
		}
		if entry.time > finishReceiving {
			finishReceiving = entry.time
		}
	}

	// Measure and smooth out packet loss.

	if packetLoss := float64(dropped) / float64(sentSamples) * 100; math.Abs(e.packetLoss-packetLoss) > 0.00001 {
		e.packetLoss += (packetLoss - e.packetLoss) * e.config.PacketLossSmoothingFactor
	} else {
		e.packetLoss = packetLoss
	}

	// Measure and smooth out sent bandwidth kbps.

	if startWriting != math.MaxFloat64 && finishWriting != 0 {
		sentBandwidthKbps := float64(written) / (finishWriting - startWriting) * 8 / 1000
		if math.Abs(sentBandwidthKbps-sentBandwidthKbps) > 0.00001 {
			e.sentBandwidthKbps += (sentBandwidthKbps - e.sentBandwidthKbps) * e.config.BandwidthSmoothingFactor
		} else {
			e.sentBandwidthKbps = sentBandwidthKbps
		}
	}

	// Measure and smooth out received bandwidth kbps.

	if startReceiving != math.MaxFloat64 && finishReceiving != 0 {
		receivedBandwidthKbps := float64(received) / (finishReceiving - startReceiving) * 8 / 1000

		if math.Abs(receivedBandwidthKbps-receivedBandwidthKbps) > 0.00001 {
			e.receivedBandwidthKbps += (receivedBandwidthKbps - e.receivedBandwidthKbps) * e.config.BandwidthSmoothingFactor
		} else {
			e.receivedBandwidthKbps = receivedBandwidthKbps
		}
	}

	// Measure and smooth out ACK'ed bandwidth kbps.

	if startACKing != math.MaxFloat64 && finishACKing != 0 {
		ackedBandwidthKbps := float64(acked) / (finishACKing - startACKing) * 8 / 1000

		if math.Abs(ackedBandwidthKbps-ackedBandwidthKbps) > 0.00001 {
			e.ackedBandwidthKbps += (ackedBandwidthKbps - e.ackedBandwidthKbps) * e.config.BandwidthSmoothingFactor
		} else {
			e.ackedBandwidthKbps = ackedBandwidthKbps
		}
	}
}

func (e *Endpoint) Next() uint16 {
	return e.seq
}

func (e *Endpoint) RTT() float64 {
	return e.rtt
}

func (e *Endpoint) PacketLoss() float64 {
	return e.packetLoss
}

func (e *Endpoint) Bandwidth() (sent float64, received float64, acked float64) {
	return e.sentBandwidthKbps, e.receivedBandwidthKbps, e.ackedBandwidthKbps
}
