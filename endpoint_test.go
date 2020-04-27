package sleepy

import (
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

var _ EndpointDispatcher = (*MockDispatcher)(nil)

type MockDispatcher struct {
	r, w [][]byte
}

func (m *MockDispatcher) Transmit(seq uint16, buf []byte) {
	m.w = append(m.w, make([]byte, len(buf)))
	copy(m.w[len(m.w)-1], buf)
}

func (m *MockDispatcher) Process(seq uint16, buf []byte) {
	m.r = append(m.r, make([]byte, len(buf)))
	copy(m.r[len(m.r)-1], buf)
}

func (m *MockDispatcher) ACK(_ uint16) {}

func newTestEndpoint(t *testing.T) (*Endpoint, *MockDispatcher) {
	t.Helper()

	endpoint := NewEndpoint(new(MockDispatcher), nil)
	return endpoint, endpoint.dispatcher.(*MockDispatcher)
}

func TestEndpointSendRecvCompactPacket(t *testing.T) {
	var err error

	client, clientConn := newTestEndpoint(t)
	server, serverConn := newTestEndpoint(t)

	data := []byte("test")

	// Have client send data.

	client.WritePacket(data)

	require.NoError(t, err)
	require.Len(t, clientConn.w, 1)

	// Have server receive data.

	require.NoError(t, server.ReadPacket(clientConn.w[0]))

	require.EqualValues(t, client.sent.buf.latest, 1)
	require.EqualValues(t, client.recv.buf.latest, 0)
	require.EqualValues(t, server.sent.buf.latest, 0)
	require.EqualValues(t, server.recv.buf.latest, 1)

	require.EqualValues(t, 0, server.recv.buf.entries[0])

	// Have server send data.

	server.WritePacket(data)

	require.NoError(t, err)
	require.Len(t, serverConn.w, 1)

	// Have client receive data.

	require.NoError(t, client.ReadPacket(serverConn.w[0]))

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

	client.config.MaxFragments, server.config.MaxFragments = 256, 256
	client.config.MaxPacketSize = client.config.MaxFragments * client.config.FragmentSize
	server.config.MaxPacketSize = server.config.MaxFragments * server.config.FragmentSize

	// Ensure that default settings are correct.

	require.EqualValues(t, client.config.FragmentSize*client.config.MaxFragments, client.config.MaxPacketSize)
	require.EqualValues(t, server.config.FragmentSize*server.config.MaxFragments, server.config.MaxPacketSize)

	require.EqualValues(t, client.config.MaxPacketSize, server.config.MaxPacketSize)

	_, _ = clientConn, serverConn

	// Create a packet of max size.

	src := rand.New(rand.NewSource(1337))
	data := make([]byte, client.config.MaxPacketSize)

	_, err = src.Read(data)
	require.NoError(t, err)

	client.WritePacket(data)
	require.Len(t, clientConn.w, int(client.config.MaxFragments))

	// Have server receive data.

	for _, i := range src.Perm(len(clientConn.w)) {
		require.NoError(t, server.ReadPacket(clientConn.w[i]))
	}

	require.EqualValues(t, client.sent.buf.latest, 1)
	require.EqualValues(t, client.recv.buf.latest, 0)
	require.EqualValues(t, server.sent.buf.latest, 0)
	require.EqualValues(t, server.recv.buf.latest, 1)
	require.EqualValues(t, server.assembler.buf.latest, 1)

	require.EqualValues(t, 0, server.recv.buf.entries[0])

	// Make sure all fragments are received.

	for id := uint(0); id < client.config.MaxFragments; id++ {
		require.Error(t, server.assembler.entries[0].MarkReceived(byte(id)))
	}
}
