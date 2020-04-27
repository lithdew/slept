package sleepy

import "math"

const HalfMaxUint16 = uint16(math.MaxUint16/2) + 1

func seqLTE(a, b uint16) bool {
	return a == b || seqGreaterThan(b, a)
}

func seqLessThan(a, b uint16) bool {
	return seqGreaterThan(b, a)
}

func seqGreaterThan(a, b uint16) bool {
	return ((a > b) && (a-b <= HalfMaxUint16)) || ((a < b) && (b-a > HalfMaxUint16))
}
