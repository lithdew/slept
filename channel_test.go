package sleepy

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelFullBufferWorstCase(t *testing.T) {
	channel := NewChannel(nil)

	i := 0
	for len(channel.queue) == 0 {
		i++
		channel.Write(nil)
		require.NoError(t, channel.Update(0))
	}

	require.EqualValues(t, channel.endpoint.config.RecvPacketBufferSize+1, i)
	require.EqualValues(t, channel.window.buf.latest, channel.endpoint.config.RecvPacketBufferSize)

	// ACK all packets except sequence number 0.

	for i := uint16(255); i >= 1; i-- {
		channel.ACK(i)
	}

	// ACK sequence number 0.

	require.EqualValues(t, 0, channel.oldestUnacked)
	require.Nil(t, channel.window.Find(uint16(channel.endpoint.config.RecvPacketBufferSize)))

	channel.ACK(0)

	require.EqualValues(t, channel.endpoint.config.RecvPacketBufferSize, channel.oldestUnacked)
	require.NotNil(t, channel.window.Find(uint16(channel.endpoint.config.RecvPacketBufferSize)))
}

func TestChannelEmptyQueue(t *testing.T) {
	channel := NewChannel(nil)

	i := 0
	for len(channel.queue) == 0 {
		i++
		channel.Write(nil)
		require.NoError(t, channel.Update(0))
	}

	require.EqualValues(t, channel.endpoint.config.RecvPacketBufferSize+1, i)
	require.EqualValues(t, channel.window.buf.latest, channel.endpoint.config.RecvPacketBufferSize)

	// ACK seq 0.

	for i := uint16(0); i < 1; i++ {
		require.EqualValues(t, i, channel.oldestUnacked)
		channel.ACK(i)
	}

	// Queue should have emptied.

	require.Len(t, channel.queue, 0)
	require.EqualValues(t, channel.window.buf.latest, channel.endpoint.config.RecvPacketBufferSize+1)
}
