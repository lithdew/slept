package sleepy

import (
	"fmt"
	"github.com/lithdew/bytesutil"
	"github.com/valyala/bytebufferpool"
)

var _ EndpointDispatcher = (*Channel)(nil)

type Channel struct {
	endpoint *Endpoint
	window   *PacketBuffer

	queue []*bytebufferpool.ByteBuffer

	readQueue  chan []byte
	writeQueue chan []byte
	outQueue   chan []byte

	oldestUnacked uint16
}

func NewChannel(config *Config) *Channel {
	channel := new(Channel)

	channel.endpoint = NewEndpoint(channel, config)
	channel.readQueue = make(chan []byte, channel.endpoint.config.RecvPacketBufferSize)
	channel.writeQueue = make(chan []byte, channel.endpoint.config.SentPacketBufferSize)
	channel.outQueue = make(chan []byte, channel.endpoint.config.SentPacketBufferSize)
	channel.window = NewPacketBuffer(uint16(channel.endpoint.config.SentPacketBufferSize))

	return channel
}

func (c *Channel) Read(buf []byte) {
	c.readQueue <- buf
}

func (c *Channel) Write(buf []byte) {
	c.writeQueue <- buf
}

func (c *Channel) Update(time float64) error {
	c.endpoint.Update(time)

Reading:
	for {
		select {
		case b := <-c.readQueue:
			err := c.endpoint.ReadPacket(b)
			if err != nil {
				return fmt.Errorf("failed to receive packet: %w", err)
			}
		default:
			break Reading
		}
	}

Writing:
	for {
		select {
		case buf := <-c.writeQueue:
			if c.oldestUnacked+uint16(c.endpoint.config.RecvPacketBufferSize) == c.endpoint.seq {
				b := c.endpoint.pool.Get()
				b.B = bytesutil.ExtendSlice(b.B, len(buf))
				copy(b.B, buf)

				c.queue = append(c.queue, b)
				continue
			}

			c.endpoint.WritePacket(buf)
		default:
			break Writing
		}
	}

	// Write packets that have yet to be written, and also write packets that have yet to be ACK'ed after 0.1 seconds
	// from the moment we originally wrote them.

	max := c.oldestUnacked + uint16(c.endpoint.config.RecvPacketBufferSize)

	for seq := c.oldestUnacked; seqLTE(seq, max); seq++ {
		packet := c.window.Find(seq)
		if packet == nil {
			continue
		}

		if packet.written && time-packet.time < 0.1 {
			continue
		}

		if !packet.written {
			packet.written, packet.time = true, time
		}

		c.outQueue <- packet.buf.B
	}

	return nil
}

func (c *Channel) Transmit(seq uint16, buf []byte) {
	b := c.endpoint.pool.Get()
	b.B = bytesutil.ExtendSlice(b.B, len(buf))
	copy(b.B, buf)

	packet := c.window.Insert(seq)
	packet.Reset()
	packet.buf = b
}

func (c *Channel) Process(_ uint16, buf []byte) {
}

func (c *Channel) ACK(seq uint16) {
	packet := c.window.Find(seq)
	if packet == nil {
		return
	}

	c.window.Remove(seq)
	c.endpoint.pool.Put(packet.buf)

	if seq != c.oldestUnacked {
		return
	}

	// Find the next oldest un-ACK'ed packet sequence number.

	oldestUnacked := c.oldestUnacked

	updated, max := false, oldestUnacked+uint16(c.endpoint.config.RecvPacketBufferSize)

	for seq := oldestUnacked + 1; seqLTE(seq, max); seq++ {
		packet := c.window.Find(seq)
		if packet == nil {
			continue
		}

		c.oldestUnacked, updated = seq, true
		break
	}

	// If the oldest ACK was not updated, set the oldest ACK to be the latest sent packet sequence number.

	if !updated {
		c.oldestUnacked = max
	}

	// Send packets that were previously queued up due to the oldest un-ACK'ed packet.

	diff := c.oldestUnacked - oldestUnacked

	for i := uint16(0); len(c.queue) > 0 && i < diff; i++ {
		popped := c.queue[0]
		c.queue = c.queue[1:]
		c.endpoint.WritePacket(popped.B)
		c.endpoint.pool.Put(popped)
	}
}
