package sleepy

import (
	"github.com/stretchr/testify/require"
	"net"
	"testing"
)

var _ Conn = (*MockConn)(nil)

type MockConn struct {
	buf [][]byte
}

func (c *MockConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	c.buf = append(c.buf, b)
	return len(b), nil
}

func newTestEndpoint(t *testing.T) (*Endpoint, *MockConn) {
	t.Helper()

	conn := new(MockConn)

	endpoint := NewEndpoint()
	endpoint.conn = conn

	return endpoint, conn
}

func TestEndpointSendRecvCompactPacket(t *testing.T) {
	var err error

	client, clientConn := newTestEndpoint(t)
	server, serverConn := newTestEndpoint(t)

	data := []byte("test")

	// Have client send data.

	_, err = client.SendPacket(data, nil)

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

	_, err = server.SendPacket(data, nil)

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
