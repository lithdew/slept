package sleepy

type FragmentReassemblyBuffer struct {
	buf     SequenceBuffer
	entries []Fragment
}

func NewFragmentReassemblyBuffer(cap uint16) *FragmentReassemblyBuffer {
	return &FragmentReassemblyBuffer{buf: NewSequenceBuffer(cap), entries: make([]Fragment, cap, cap)}
}

func (s *FragmentReassemblyBuffer) Insert(seq uint16) *Fragment {
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

func (s *FragmentReassemblyBuffer) Remove(seq uint16) {
	s.buf.entries[seq%uint16(cap(s.entries))] = EmptySequenceBufferEntry
}

func (s *FragmentReassemblyBuffer) IsOutdated(seq uint16) bool {
	return seqLessThan(seq, s.buf.latest-uint16(cap(s.entries)))
}

func (s *FragmentReassemblyBuffer) Find(seq uint16) *Fragment {
	if i := seq % uint16(cap(s.entries)); s.buf.entries[i] == uint32(seq) {
		return &s.entries[i]
	}

	return nil
}

func (s *FragmentReassemblyBuffer) Entry(i int) *Fragment {
	if s.buf.entries[i] != EmptySequenceBufferEntry {
		return &s.entries[i]
	}

	return nil
}
