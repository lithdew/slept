package sleepy

import "math"

const EmptySequenceBufferEntry = math.MaxUint32

// A cache of EmptySequenceBufferEntry's that may be copied into slices to clear out entries.
var emptySequenceBufferEntryCache [4096]uint32

func init() {
	for i := range emptySequenceBufferEntryCache {
		emptySequenceBufferEntryCache[i] = EmptySequenceBufferEntry
	}
}

func resetSequenceBuffer(b []uint32) {
	copy(b, emptySequenceBufferEntryCache[:len(b)])
}
