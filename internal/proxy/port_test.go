package proxy

import (
	"net"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func silentLogger() zerolog.Logger {
	return zerolog.Nop()
}

func TestListenWithFallback_OSAssigned(t *testing.T) {
	ln, err := listenWithFallback(0, silentLogger())
	require.NoError(t, err)
	defer ln.Close()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	assert.Greater(t, tcpAddr.Port, 0, "OS should assign a port > 0")
}

func TestListenWithFallback_IncrementsWhenBusy(t *testing.T) {
	// Occupy a specific port.
	occupied, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer occupied.Close()

	busyPort := occupied.Addr().(*net.TCPAddr).Port

	ln, err := listenWithFallback(busyPort, silentLogger())
	require.NoError(t, err)
	defer ln.Close()

	boundPort := ln.Addr().(*net.TCPAddr).Port
	assert.NotEqual(t, busyPort, boundPort, "fallback must pick a different port")
	assert.GreaterOrEqual(t, boundPort, busyPort+1)
	assert.Less(t, boundPort, busyPort+portSearchWindow)
}

func TestIsAddrInUse(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	_, err = net.Listen("tcp", ln.Addr().String())
	require.Error(t, err)
	assert.True(t, isAddrInUse(err), "real EADDRINUSE should be detected: %v", err)

	// Unrelated error must not match.
	_, notInUse := net.Listen("tcp", "127.0.0.1:-1")
	require.Error(t, notInUse)
	assert.False(t, isAddrInUse(notInUse))

	_ = port
}
