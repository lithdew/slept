package sleepy

type PacketBuffer struct {
	buf     SequenceBuffer
	entries []BufferedPacket
}

func NewPacketBuffer(cap uint16) *PacketBuffer {
	return &PacketBuffer{buf: NewSequenceBuffer(cap), entries: make([]BufferedPacket, cap, cap)}
}

func (s *PacketBuffer) IsOutdated(seq uint16) bool {
	return seqLessThan(seq, s.buf.latest-uint16(cap(s.entries)))
}

func (s *PacketBuffer) Insert(seq uint16) *BufferedPacket {
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

func (s *PacketBuffer) Remove(seq uint16) {
	s.buf.entries[seq%uint16(cap(s.entries))] = EmptySequenceBufferEntry
}

func (s *PacketBuffer) Find(seq uint16) *BufferedPacket {
	if i := seq % uint16(cap(s.entries)); s.buf.entries[i] == uint32(seq) {
		return &s.entries[i]
	}

	return nil
}
