package sleepy

import (
	"fmt"
	"github.com/lithdew/bytesutil"
	"github.com/valyala/bytebufferpool"
	"io"
	"math/bits"
)

const (
	MaxPacketHeaderSize = uint(9)
	FragmentHeaderSize  = uint(5)
)

type BufferedPacket struct {
	time float64
	buf  *bytebufferpool.ByteBuffer
}

func (p *BufferedPacket) Reset() {
	*p = BufferedPacket{}
}

type SentPacket struct {
	time  float64
	acked bool
	size  uint
}

func (p *SentPacket) Reset() {
	*p = SentPacket{}
}

type RecvPacket struct {
	time float64
	size uint
}

func (p *RecvPacket) Reset() {
	*p = RecvPacket{}
}

type PacketHeaderFlag uint8

const (
	FlagFragment PacketHeaderFlag = 1 << iota
	FlagA
	FlagB
	FlagC
	FlagD
	FlagACKEncoded
)

func (p PacketHeaderFlag) Toggle(flag PacketHeaderFlag) PacketHeaderFlag {
	return p | flag
}

func (p PacketHeaderFlag) Toggled(flag PacketHeaderFlag) bool {
	return p&flag != 0
}

func (p PacketHeaderFlag) AppendTo(dst []byte) []byte {
	return append(dst, byte(p))
}

type PacketHeader struct {
	seq  uint16
	ack  uint16
	acks uint32
}

func (p PacketHeader) AppendTo(dst []byte) []byte {
	// Mark a flag byte to RLE-encode the ACK bitset.

	flag := PacketHeaderFlag(0)
	if p.acks&0x000000FF != 0x000000FF {
		flag = flag.Toggle(FlagA)
	}
	if p.acks&0x0000FF00 != 0x0000FF00 {
		flag = flag.Toggle(FlagB)
	}
	if p.acks&0x00FF0000 != 0x00FF0000 {
		flag = flag.Toggle(FlagC)
	}
	if p.acks&0xFF000000 != 0xFF000000 {
		flag = flag.Toggle(FlagD)
	}

	// If the difference between the sequence number and the latest ACK'd sequence number can be represented by a
	// single byte, then represent it as a single byte and set the 5th bit of flag.

	diff := int(p.seq) - int(p.ack)
	if diff < 0 {
		diff += 65536
	}
	if diff <= 255 {
		flag = flag.Toggle(FlagACKEncoded)
	}

	// Marshal the flag and sequence number and latest ACK'd sequence number.

	dst = flag.AppendTo(dst)
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

func UnmarshalPacketHeader(buf []byte) (header PacketHeader, leftover []byte, err error) {
	flag := PacketHeaderFlag(0)

	// Read first 3 bytes (header, flag).

	if len(buf) < 3 {
		return header, buf, io.ErrUnexpectedEOF
	}

	flag, buf = PacketHeaderFlag(buf[0]), buf[1:]

	if flag.Toggled(FlagFragment) {
		return header, buf, io.ErrUnexpectedEOF
	}

	header.seq, buf = bytesutil.Uint16BE(buf[:2]), buf[2:]

	// Read and decode the latest ACK'ed sequence number (either 1 or 2 bytes) using the RLE flag marker.

	if flag.Toggled(FlagACKEncoded) {
		if len(buf) < 1 {
			return header, buf, io.ErrUnexpectedEOF
		}
		header.ack, buf = header.seq-uint16(buf[0]), buf[1:]
	} else {
		if len(buf) < 2 {
			return header, buf, io.ErrUnexpectedEOF
		}
		header.ack, buf = bytesutil.Uint16BE(buf[:2]), buf[2:]
	}

	if len(buf) < bits.OnesCount8(byte(flag)&(1<<1|1<<2|1<<3|1<<4)) {
		return header, buf, io.ErrUnexpectedEOF
	}

	// Read and decode ACK bitset using the RLE flag marker.

	header.acks = 0xFFFFFFFF

	if flag.Toggled(FlagA) {
		header.acks &= 0xFFFFFF00
		header.acks |= uint32(buf[0])
		buf = buf[1:]
	}

	if flag.Toggled(FlagB) {
		header.acks &= 0xFFFF00FF
		header.acks |= uint32(buf[0]) << 8
		buf = buf[1:]
	}

	if flag.Toggled(FlagC) {
		header.acks &= 0xFF00FFFF
		header.acks |= uint32(buf[0]) << 16
		buf = buf[1:]
	}

	if flag.Toggled(FlagD) {
		header.acks &= 0x00FFFFFF
		header.acks |= uint32(buf[0]) << 24
		buf = buf[1:]
	}

	return header, buf, nil
}

type Fragment struct {
	recv  uint
	total uint

	buf *bytebufferpool.ByteBuffer

	headerSize uint
	packetSize uint

	marked [4]uint64
}

func (f *Fragment) MarkReceived(id byte) error {
	idx, pos := id>>6, uint64(1<<(id&63))
	if f.marked[idx]&pos != 0 {
		return fmt.Errorf("ignoring fragment: fragment %d was already received", id)
	}
	f.marked[idx] |= pos
	return nil
}

func (f *Fragment) Reset() {
	*f = Fragment{}
}

type FragmentHeader struct {
	seq   uint16
	id    uint8
	total uint8
}

func (f FragmentHeader) Validate(maxFragments uint) error {
	if f.id > f.total {
		return fmt.Errorf("fragment id > total num fragments (fragment id: %d, total num fragments: %d)",
			f.id,
			f.total,
		)
	}

	if uint(f.total)+1 > maxFragments {
		return fmt.Errorf("max amount of fragments a packet can be partitioned into is %d, but got %d fragment(s)",
			f.total,
			maxFragments,
		)
	}

	return nil
}

func (f FragmentHeader) AppendTo(dst []byte) []byte {
	dst = FlagFragment.AppendTo(dst)
	dst = bytesutil.AppendUint16BE(dst, f.seq)
	dst = append(dst, f.id, f.total)
	return dst
}

func UnmarshalFragmentHeader(buf []byte) (header FragmentHeader, leftover []byte, err error) {
	if uint(len(buf)) < FragmentHeaderSize {
		return header, buf, fmt.Errorf("got %d byte(s), expected at least %d byte(s): %w",
			len(buf),
			FragmentHeaderSize,
			io.ErrUnexpectedEOF,
		)
	}

	buf = buf[1:]

	header.seq, buf = bytesutil.Uint16BE(buf[0:2]), buf[2:]
	header.id, buf = buf[0], buf[1:]
	header.total, buf = buf[0], buf[1:]

	return header, buf, nil
}
