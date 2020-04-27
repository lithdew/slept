package sleepy

import (
	"github.com/lithdew/bytesutil"
	"github.com/valyala/bytebufferpool"
)

var _ EndpointDispatcher = (*Channel)(nil)

type Channel struct {
	endpoint *Endpoint
	window   *PacketBuffer

	pool  bytebufferpool.Pool
	queue []*bytebufferpool.ByteBuffer

	oldestUnacked uint16
}

func NewChannel(config *Config) *Channel {
	channel := new(Channel)
	channel.endpoint = NewEndpoint(channel, config)
	channel.window = NewPacketBuffer(uint16(channel.endpoint.config.SentPacketBufferSize))
	return channel
}

func (c *Channel) SendPacket(buf []byte) {
	seq := c.endpoint.Next()

	if c.oldestUnacked+uint16(c.endpoint.config.RecvPacketBufferSize) == seq {
		b := c.pool.Get()
		b.B = bytesutil.ExtendSlice(b.B, len(buf))
		copy(b.B, buf)

		c.queue = append(c.queue, b)

		return
	}

	c.endpoint.SendPacket(buf)
}

func (c *Channel) RecvPacket(buf []byte) error {
	return c.endpoint.RecvPacket(buf)
}

func (c *Channel) Update(time float64) {
	c.endpoint.Update(time)
	c.update()
}

func (c *Channel) update() {
	// Resend messages that have yet to be ACK'ed after 0.1 seconds from the moment we sent them.

	max := c.oldestUnacked + uint16(c.endpoint.config.RecvPacketBufferSize)

	for seq := c.oldestUnacked; seqLTE(seq, max); seq++ {
		packet := c.window.Find(seq)
		if packet == nil {
			continue
		}

		if c.endpoint.time-packet.time < 0.1 {
			continue
		}

		// REAL transmit
		c.transmit(packet.buf.B)
	}
}

func (c *Channel) transmit(buf []byte) {

}

func (c *Channel) Transmit(seq uint16, buf []byte) {
	b := c.pool.Get()
	b.B = bytesutil.ExtendSlice(b.B, len(buf))
	copy(b.B, buf)

	packet := c.window.Insert(seq)
	packet.Reset()

	packet.time = c.endpoint.time
	packet.buf = b
}

func (c *Channel) Process(seq uint16, buf []byte) {

}

func (c *Channel) ACK(seq uint16) {
	packet := c.window.Find(seq)
	if packet == nil {
		return
	}

	c.window.Remove(seq)
	c.pool.Put(packet.buf)

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

		c.endpoint.SendPacket(popped.B)
		c.pool.Put(popped)
	}
}
