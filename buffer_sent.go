package sleepy

type SentPacketBuffer struct {
	buf     SequenceBuffer
	entries []SentPacket
}

func NewSentPacketBuffer(cap uint16) *SentPacketBuffer {
	return &SentPacketBuffer{buf: NewSequenceBuffer(cap), entries: make([]SentPacket, cap, cap)}
}

func (s *SentPacketBuffer) Insert(seq uint16) *SentPacket {
	// Packet is outdated. Ignore.

	if s.IsOutdated(seq) {
		return nil
	}

	// Packet sent has a sequence number larger than all other packets in the sent back buffer. Empty out stale entries
	// and increment the latest known-so-far sent packet sequence number.

	if seqGreaterThan(seq+1, s.buf.latest) {
		s.buf.RemoveRange(s.buf.latest, seq)
		s.buf.latest = seq + 1
	}

	i := seq % uint16(cap(s.entries))
	s.buf.entries[i] = uint32(seq)

	return &s.entries[i]
}

func (s *SentPacketBuffer) Find(seq uint16) *SentPacket {
	if i := seq % uint16(cap(s.entries)); s.buf.entries[i] == uint32(seq) {
		return &s.entries[i]
	}

	return nil
}

func (s *SentPacketBuffer) IsOutdated(seq uint16) bool {
	return seqLessThan(seq, s.buf.latest-uint16(cap(s.entries)))
}
