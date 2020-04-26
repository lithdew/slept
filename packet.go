package sleepy

import (
	"fmt"
	"github.com/lithdew/bytesutil"
	"github.com/valyala/bytebufferpool"
	"io"
	"math"
	"math/bits"
)

const (
	MaxPacketHeaderSize = 9
	FragmentHeaderSize  = 5
)

type SentPacket struct {
	time  float64
	acked bool
	size  int
}

func (p *SentPacket) Reset() {
	*p = SentPacket{}
}

type RecvPacket struct {
	time float64
	size int
}

func (p *RecvPacket) Reset() {
	*p = RecvPacket{}
}

type PacketHeader struct {
	seq  uint16
	ack  uint16
	acks uint32
}

func (p PacketHeader) AppendTo(dst []byte) []byte {
	// Mark a flag byte to RLE-encode the ACK bitset.

	flag := uint8(0)
	if p.acks&0x000000FF != 0x000000FF {
		flag |= 1 << 1
	}
	if p.acks&0x0000FF00 != 0x0000FF00 {
		flag |= 1 << 2
	}
	if p.acks&0x00FF0000 != 0x00FF0000 {
		flag |= 1 << 3
	}
	if p.acks&0xFF000000 != 0xFF000000 {
		flag |= 1 << 4
	}

	// If the difference between the sequence number and the latest ACK'd sequence number can be represented by a
	// single byte, then represent it as a single byte and set the 5th bit of flag.

	diff := int(p.seq) - int(p.ack)
	if diff < 0 {
		diff += 65536
	}
	if diff <= 255 {
		flag |= 1 << 5
	}

	// Marshal the flag and sequence number and latest ACK'd sequence number.

	dst = append(dst, flag)
	dst = bytesutil.AppendUint16BE(dst, p.seq)

	if diff <= 255 {
		dst = append(dst, uint8(diff))
	} else {
		dst = bytesutil.AppendUint16BE(dst, p.ack)
	}

	// Marshal ACK bitset.

	if p.acks&0x000000FF != 0x000000FF {
		dst = append(dst, uint8(p.acks&0x000000FF))
	}
	if p.acks&0x0000FF00 != 0x0000FF00 {
		dst = append(dst, uint8((p.acks&0x0000FF00)>>8))
	}
	if p.acks&0x00FF0000 != 0x00FF0000 {
		dst = append(dst, uint8((p.acks&0x00FF0000)>>16))
	}
	if p.acks&0xFF000000 != 0xFF000000 {
		dst = append(dst, uint8((p.acks&0xFF000000)>>24))
	}

	return dst
}

func UnmarshalPacketHeader(b []byte) (header PacketHeader, leftover []byte, err error) {
	var flag uint8

	// Read first 3 bytes (header, flag).

	if len(b) < 3 {
		return header, b, io.ErrUnexpectedEOF
	}

	flag, b = b[0], b[1:]

	if flag&1 != 0 {
		return header, b, io.ErrUnexpectedEOF
	}

	header.seq, b = bytesutil.Uint16BE(b[:2]), b[2:]

	// Read and decode the latest ACK'ed sequence number (either 1 or 2 bytes) using the RLE flag marker.

	if flag&(1<<5) != 0 {
		if len(b) < 1 {
			return header, b, io.ErrUnexpectedEOF
		}
		header.ack, b = header.seq-uint16(b[0]), b[1:]
	} else {
		if len(b) < 2 {
			return header, b, io.ErrUnexpectedEOF
		}
		header.ack, b = bytesutil.Uint16BE(b[:2]), b[2:]
	}

	if len(b) < bits.OnesCount8(flag&(1<<1|1<<2|1<<3|1<<4)) {
		return header, b, io.ErrUnexpectedEOF
	}

	// Read and decode ACK bitset using the RLE flag marker.

	header.acks = 0xFFFFFFFF

	if flag&(1<<1) != 0 {
		header.acks &= 0xFFFFFF00
		header.acks |= uint32(b[0])
		b = b[1:]
	}

	if flag&(1<<2) != 0 {
		header.acks &= 0xFFFF00FF
		header.acks |= uint32(b[0]) << 8
		b = b[1:]
	}

	if flag&(1<<3) != 0 {
		header.acks &= 0xFF00FFFF
		header.acks |= uint32(b[0]) << 16
		b = b[1:]
	}

	if flag&(1<<4) != 0 {
		header.acks &= 0x00FFFFFF
		header.acks |= uint32(b[0]) << 24
		b = b[1:]
	}

	return header, b, nil
}

type Fragment struct {
	recv  byte
	total byte

	buf *bytebufferpool.ByteBuffer

	headerSize int
	packetSize int

	marked [math.MaxUint8]bool
}

func (f *Fragment) Reset() {
	*f = Fragment{}
}

type FragmentHeader struct {
	seq   uint16
	id    uint8
	total uint8
}

func (f FragmentHeader) Validate(maxFragments uint8) error {
	if f.id > f.total {
		return fmt.Errorf("fragment id > total num fragments (fragment id: %d, total num fragments: %d)",
			f.id,
			f.total,
		)
	}

	if f.total > maxFragments {
		return fmt.Errorf("max amount of fragments a packet can be partitioned into is %d, but got %d fragment(s)",
			f.total,
			maxFragments,
		)
	}

	return nil
}

func (f FragmentHeader) AppendTo(dst []byte) []byte {
	dst = append(dst, 1)
	dst = bytesutil.AppendUint16BE(dst, f.seq)
	dst = append(dst, f.id, f.total-1)
	return dst
}

func UnmarshalFragmentHeader(buf []byte) (header FragmentHeader, leftover []byte, err error) {
	if len(buf) < FragmentHeaderSize {
		return header, buf, fmt.Errorf("got %d byte(s), expected at least %d byte(s): %w",
			len(buf),
			FragmentHeaderSize,
			io.ErrUnexpectedEOF,
		)
	}

	buf = buf[1:]

	header.seq, buf = bytesutil.Uint16BE(buf[0:2]), buf[2:]
	header.id, buf = buf[0], buf[1:]
	header.total, buf = buf[0]+1, buf[1:]

	return header, buf, nil
}
