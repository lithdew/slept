package sleepy

import (
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
	"time"
)

var _ Conn = (*MockConn)(nil)

type MockConn struct {
	buf [][]byte
}

func (c *MockConn) Write(b []byte) (int, error) {
	c.buf = append(c.buf, make([]byte, len(b)))
	return copy(c.buf[len(c.buf)-1], b), nil
}

func newTestEndpoint(t *testing.T) (*Endpoint, *MockConn) {
	t.Helper()

	endpoint := NewEndpoint(new(MockConn))
	return endpoint, endpoint.conn.(*MockConn)
}

func TestEndpointSendRecvCompactPacket(t *testing.T) {
	var err error

	client, clientConn := newTestEndpoint(t)
	server, serverConn := newTestEndpoint(t)

	data := []byte("test")

	// Have client send data.

	_, err = client.SendPacket(data)

	require.NoError(t, err)
	require.Len(t, clientConn.buf, 1)

	// Have server receive data.

	require.NoError(t, server.RecvPacket(clientConn.buf[0]))

	require.EqualValues(t, client.sent.buf.latest, 1)
	require.EqualValues(t, client.recv.buf.latest, 0)
	require.EqualValues(t, server.sent.buf.latest, 0)
	require.EqualValues(t, server.recv.buf.latest, 1)

	require.EqualValues(t, 0, server.recv.buf.entries[0])

	// Have server send data.

	_, err = server.SendPacket(data)

	require.NoError(t, err)
	require.Len(t, serverConn.buf, 1)

	// Have client receive data.

	require.NoError(t, client.RecvPacket(serverConn.buf[0]))

	require.EqualValues(t, client.sent.buf.latest, 1)
	require.EqualValues(t, client.recv.buf.latest, 1)
	require.EqualValues(t, server.sent.buf.latest, 1)
	require.EqualValues(t, server.recv.buf.latest, 1)

	require.EqualValues(t, 0, client.recv.buf.entries[0])

	// The client should have received an ACK from the server.

	require.True(t, client.sent.entries[0].acked)
}

func TestEndpointSendRecvFragmentedPacket(t *testing.T) {
	var err error

	client, clientConn := newTestEndpoint(t)
	server, serverConn := newTestEndpoint(t)

	client.MaxFragments, server.MaxFragments = 256, 256
	client.MaxPacketSize = client.MaxFragments * client.FragmentSize
	server.MaxPacketSize = server.MaxFragments * server.FragmentSize

	// Ensure that default settings are correct.

	require.EqualValues(t, client.FragmentSize*client.MaxFragments, client.MaxPacketSize)
	require.EqualValues(t, server.FragmentSize*server.MaxFragments, server.MaxPacketSize)

	require.EqualValues(t, client.MaxPacketSize, server.MaxPacketSize)

	_, _ = clientConn, serverConn

	// Create a packet of max size.

	src := rand.New(rand.NewSource(time.Now().Unix()))
	data := make([]byte, client.MaxPacketSize)

	_, err = src.Read(data)
	require.NoError(t, err)

	_, err = client.SendPacket(data)

	require.NoError(t, err)
	require.Len(t, clientConn.buf, int(client.MaxFragments))

	// Have server receive data.

	for _, i := range src.Perm(len(clientConn.buf)) {
		require.NoError(t, server.RecvPacket(clientConn.buf[i]))
	}

	require.EqualValues(t, client.sent.buf.latest, 1)
	require.EqualValues(t, client.recv.buf.latest, 0)
	require.EqualValues(t, server.sent.buf.latest, 0)
	require.EqualValues(t, server.recv.buf.latest, 1)
	require.EqualValues(t, server.assembler.buf.latest, 1)

	require.EqualValues(t, 0, server.recv.buf.entries[0])

	// Make sure all fragments are received.

	for id := uint(0); id < client.MaxFragments; id++ {
		require.Error(t, server.assembler.entries[0].MarkReceived(byte(id)))
	}
}
