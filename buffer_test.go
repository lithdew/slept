package sleepy

import (
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

func TestEmptySequenceBuffer(t *testing.T) {
	s := NewSequenceBuffer(1024)

	for i := range s.entries {
		s.entries[i] = rand.Uint32()
	}

	s.RemoveRange(0, uint16(cap(s.entries)))

	for i := range s.entries {
		require.EqualValues(t, EmptySequenceBufferEntry, s.entries[i])
	}
}

func TestSentPacketBuffer(t *testing.T) {
	s := NewSentPacketBuffer(1024)

	for i := range s.entries {
		s.buf.entries[i] = rand.Uint32()
	}

	s.buf.RemoveRange(0, uint16(cap(s.entries)))

	for i := range s.buf.entries {
		require.EqualValues(t, EmptySequenceBufferEntry, s.buf.entries[i])
	}
}

func TestRecvPacketBuffer(t *testing.T) {
	s := NewRecvPacketBuffer(1024)

	for i := range s.entries {
		s.buf.entries[i] = rand.Uint32()
	}

	s.buf.RemoveRange(0, uint16(cap(s.entries)))

	for i := range s.buf.entries {
		require.EqualValues(t, EmptySequenceBufferEntry, s.buf.entries[i])
	}
}

func BenchmarkTestEmptySequenceBuffer(b *testing.B) {
	s := NewSequenceBuffer(1024)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s.RemoveRange(0, uint16(cap(s.entries)))
	}
}
