package sleepy

type SequenceBuffer struct {
	latest  uint16
	entries []uint32
}

func NewSequenceBuffer(cap uint16) SequenceBuffer {
	s := SequenceBuffer{entries: make([]uint32, cap, cap)}
	s.Reset()
	return s
}

func (s *SequenceBuffer) Reset() {
	s.latest = 0

	resetSequenceBuffer(s.entries)
}

func (s *SequenceBuffer) Remove(seq uint16) {
	s.entries[seq%uint16(cap(s.entries))] = EmptySequenceBufferEntry
}

func (s *SequenceBuffer) RemoveRange(start, end uint16) {
	start, end = start%(uint16(cap(s.entries))+1), end%(uint16(cap(s.entries))+1)

	if end < start {
		resetSequenceBuffer(s.entries[start:])
		resetSequenceBuffer(s.entries[:end])
		return
	}

	resetSequenceBuffer(s.entries[start:end])
}
