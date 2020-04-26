package sleepy

type RecvPacketBuffer struct {
	buf     SequenceBuffer
	entries []RecvPacket
}

func NewRecvPacketBuffer(cap uint16) *RecvPacketBuffer {
	return &RecvPacketBuffer{buf: NewSequenceBuffer(cap), entries: make([]RecvPacket, cap, cap)}
}

func (s *RecvPacketBuffer) NextACK() (ack uint16, acks uint32) {
	ack = s.buf.latest - 1

	for i, m := uint16(0), uint32(1); i < 32; i, m = i+1, m<<1 {
		if i := ack - i; s.buf.entries[i%uint16(cap(s.entries))] != uint32(i) {
			continue
		}

		acks |= m
	}

	return ack, acks
}

func (s *RecvPacketBuffer) Find(seq uint16) *RecvPacket {
	if i := seq % uint16(cap(s.entries)); s.buf.entries[i] == uint32(seq) {
		return &s.entries[i]
	}

	return nil
}

func (s *RecvPacketBuffer) IsOutdated(seq uint16) bool {
	return seqLessThan(seq, s.buf.latest-uint16(cap(s.entries)))
}

func (s *RecvPacketBuffer) Insert(seq uint16) *RecvPacket {
	// Packet is outdated. Ignore.

	if s.IsOutdated(seq) {
		return nil
	}

	// Packet received has a sequence number larger than all other packets in the buffer. Empty out stale entries and
	// increment the latest known-so-far sent packet sequence number.

	if seqGreaterThan(seq+1, s.buf.latest) {
		s.buf.RemoveRange(s.buf.latest, seq)
		s.buf.latest = seq + 1
	}

	i := seq % uint16(cap(s.entries))
	s.buf.entries[i] = uint32(seq)

	return &s.entries[i]
}
