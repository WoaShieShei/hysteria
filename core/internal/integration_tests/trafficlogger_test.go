package integration_tests

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/apernet/hysteria/core/client"
	"github.com/apernet/hysteria/core/internal/integration_tests/mocks"
	"github.com/apernet/hysteria/core/server"
)

// TestClientServerTrafficLoggerTCP tests that the traffic logger is correctly called for TCP connections,
// and that the client is disconnected when the traffic logger returns false.
func TestClientServerTrafficLoggerTCP(t *testing.T) {
	// Create server
	udpConn, udpAddr, err := serverConn()
	assert.NoError(t, err)
	serverOb := mocks.NewMockOutbound(t)
	auth := mocks.NewMockAuthenticator(t)
	auth.EXPECT().Authenticate(mock.Anything, mock.Anything, mock.Anything).Return(true, "nobody")
	trafficLogger := mocks.NewMockTrafficLogger(t)
	s, err := server.NewServer(&server.Config{
		TLSConfig:     serverTLSConfig(),
		Conn:          udpConn,
		Outbound:      serverOb,
		Authenticator: auth,
		TrafficLogger: trafficLogger,
	})
	assert.NoError(t, err)
	defer s.Close()
	go s.Serve()

	// Create client
	c, _, err := client.NewClient(&client.Config{
		ServerAddr: udpAddr,
		TLSConfig:  client.TLSConfig{InsecureSkipVerify: true},
	})
	assert.NoError(t, err)
	defer c.Close()

	addr := "dontcare.cc:4455"

	sobConn := mocks.NewMockConn(t)
	sobConnCh := make(chan []byte, 1)
	sobConn.EXPECT().Read(mock.Anything).RunAndReturn(func(bs []byte) (int, error) {
		b := <-sobConnCh
		if b == nil {
			return 0, io.EOF
		} else {
			return copy(bs, b), nil
		}
	})
	sobConn.EXPECT().Close().RunAndReturn(func() error {
		close(sobConnCh)
		return nil
	}).Once()
	serverOb.EXPECT().TCP(addr).Return(sobConn, nil).Once()

	conn, err := c.TCP(addr)
	assert.NoError(t, err)

	// Client reads from server
	trafficLogger.EXPECT().Log("nobody", uint64(0), uint64(11)).Return(true).Once()
	sobConnCh <- []byte("knock knock")
	buf := make([]byte, 100)
	n, err := conn.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 11, n)
	assert.Equal(t, "knock knock", string(buf[:n]))

	// Client writes to server
	trafficLogger.EXPECT().Log("nobody", uint64(12), uint64(0)).Return(true).Once()
	sobConn.EXPECT().Write([]byte("who is there")).Return(12, nil).Once()
	n, err = conn.Write([]byte("who is there"))
	assert.NoError(t, err)
	assert.Equal(t, 12, n)
	time.Sleep(1 * time.Second) // Need some time for the server to receive the data

	// Client reads from server again but blocked
	trafficLogger.EXPECT().Log("nobody", uint64(0), uint64(4)).Return(false).Once()
	sobConnCh <- []byte("nope")
	n, err = conn.Read(buf)
	assert.Zero(t, n)
	assert.Error(t, err)

	// The client should be disconnected
	_, err = c.TCP("whatever")
	assert.Error(t, err)
}

// TestClientServerTrafficLoggerUDP tests that the traffic logger is correctly called for UDP sessions,
// and that the client is disconnected when the traffic logger returns false.
func TestClientServerTrafficLoggerUDP(t *testing.T) {
	// Create server
	udpConn, udpAddr, err := serverConn()
	assert.NoError(t, err)
	serverOb := mocks.NewMockOutbound(t)
	auth := mocks.NewMockAuthenticator(t)
	auth.EXPECT().Authenticate(mock.Anything, mock.Anything, mock.Anything).Return(true, "nobody")
	trafficLogger := mocks.NewMockTrafficLogger(t)
	s, err := server.NewServer(&server.Config{
		TLSConfig:     serverTLSConfig(),
		Conn:          udpConn,
		Outbound:      serverOb,
		Authenticator: auth,
		TrafficLogger: trafficLogger,
	})
	assert.NoError(t, err)
	defer s.Close()
	go s.Serve()

	// Create client
	c, _, err := client.NewClient(&client.Config{
		ServerAddr: udpAddr,
		TLSConfig:  client.TLSConfig{InsecureSkipVerify: true},
	})
	assert.NoError(t, err)
	defer c.Close()

	addr := "shady.org:43211"

	sobConn := mocks.NewMockUDPConn(t)
	sobConnCh := make(chan []byte, 1)
	sobConn.EXPECT().ReadFrom(mock.Anything).RunAndReturn(func(bs []byte) (int, string, error) {
		b := <-sobConnCh
		if b == nil {
			return 0, "", io.EOF
		} else {
			return copy(bs, b), addr, nil
		}
	})
	sobConn.EXPECT().Close().RunAndReturn(func() error {
		close(sobConnCh)
		return nil
	}).Once()
	serverOb.EXPECT().UDP(addr).Return(sobConn, nil).Once()

	conn, err := c.UDP()
	assert.NoError(t, err)

	// Client writes to server
	trafficLogger.EXPECT().Log("nobody", uint64(9), uint64(0)).Return(true).Once()
	sobConn.EXPECT().WriteTo([]byte("small sad"), addr).Return(9, nil).Once()
	_, err = conn.WriteTo([]byte("small sad"), addr)
	assert.NoError(t, err)
	time.Sleep(1 * time.Second) // Need some time for the server to receive the data

	buf := make([]byte, udpBufferSize)
	// Client reads from server
	trafficLogger.EXPECT().Log("nobody", uint64(0), uint64(7)).Return(true).Once()
	sobConnCh <- []byte("big mad")
	n, rAddr, err := conn.ReadFrom(buf)
	assert.NoError(t, err)
	assert.Equal(t, rAddr, addr)
	assert.Equal(t, "big mad", string(buf[:n]))

	// Client reads from server again but blocked
	trafficLogger.EXPECT().Log("nobody", uint64(0), uint64(4)).Return(false).Once()
	sobConnCh <- []byte("nope")
	n, rAddr, err = conn.ReadFrom(buf)
	assert.Equal(t, err, io.EOF)
	assert.Empty(t, rAddr)
	assert.Empty(t, buf[:n])

	// The client should be disconnected
	_, err = c.UDP()
	assert.Error(t, err)
}
