// Ligolo-ng
// Copyright (C) 2025 Nicolas Chatelain (nicocha30)

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package relay

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"
)

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }

type refusingPacketConn struct {
	written   chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
	writeOnce sync.Once
	payload   []byte
}

func newRefusingPacketConn() *refusingPacketConn {
	return &refusingPacketConn{
		written: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (c *refusingPacketConn) Read([]byte) (int, error) {
	select {
	case <-c.written:
		return 0, syscall.ECONNREFUSED
	case <-c.closed:
		return 0, net.ErrClosed
	}
}

func (c *refusingPacketConn) Write(payload []byte) (int, error) {
	c.payload = append([]byte(nil), payload...)
	c.writeOnce.Do(func() { close(c.written) })
	return len(payload), nil
}

func (c *refusingPacketConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *refusingPacketConn) LocalAddr() net.Addr              { return testAddr("local") }
func (c *refusingPacketConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (c *refusingPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *refusingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *refusingPacketConn) SetWriteDeadline(time.Time) error { return nil }

// TestStartUDPListenerRelayReturnTraffic reproduces
// https://github.com/nicocha30/ligolo-ng/issues/147: a reverse UDP listener
// must keep relaying after the redirect target replies. The agent listens on
// an unconnected UDP socket (net.ListenUDP) and relays datagrams over the
// tunnel. The previous implementation pumped raw bytes with io.Copy, so the
// return path called Write on the unconnected socket, which always fails and
// tore the whole listener down on the first reply.
func TestStartUDPListenerRelayReturnTraffic(t *testing.T) {
	// Agent-side UDP listener, exactly as NewUDPListener creates it.
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer udpConn.Close()
	listenerAddr := udpConn.LocalAddr().(*net.UDPAddr)

	// net.Pipe stands in for the yamux tunnel back to the proxy.
	proxySide, tunnelSide := net.Pipe()
	defer proxySide.Close()
	defer tunnelSide.Close()

	go StartUDPListenerRelay(tunnelSide, udpConn)

	// A UDP client reaches the agent listener and sends a datagram.
	client, err := net.DialUDP("udp", nil, listenerAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("client write: %v", err)
	}

	// Forward direction: the datagram must reach the proxy side of the tunnel.
	if err := proxySide.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	buf := make([]byte, 1024)
	n, err := proxySide.Read(buf)
	if err != nil {
		t.Fatalf("reading forwarded datagram from tunnel: %v", err)
	}
	if got := buf[:n]; !bytes.Equal(got, []byte("ping")) {
		t.Fatalf("forwarded datagram = %q, want %q", got, "ping")
	}

	// Return direction: a reply written back through the tunnel must reach the
	// original UDP client. This is the behavior that issue #147 reports broken.
	if _, err := proxySide.Write([]byte("pong")); err != nil {
		t.Fatalf("proxy write: %v", err)
	}

	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	n, err = client.Read(buf)
	if err != nil {
		t.Fatalf("client did not receive return traffic (issue #147): %v", err)
	}
	if got := buf[:n]; !bytes.Equal(got, []byte("pong")) {
		t.Fatalf("client received %q, want %q", got, "pong")
	}
}

func TestFramedPacketRelayReportsPortUnreachableWithOffendingDatagram(t *testing.T) {
	proxyTunnel, agentTunnel := net.Pipe()
	applicationConn, proxyPacketConn := net.Pipe()
	agentPacketConn := newRefusingPacketConn()
	defer applicationConn.Close()

	remoteError := make(chan struct {
		relayError PacketRelayError
		payload    []byte
	}, 1)
	proxyDone := make(chan error, 1)
	agentDone := make(chan error, 1)

	go func() {
		proxyDone <- StartFramedPacketRelay(proxyTunnel, proxyPacketConn, nil, func(relayError PacketRelayError, payload []byte) {
			remoteError <- struct {
				relayError PacketRelayError
				payload    []byte
			}{relayError: relayError, payload: payload}
		})
	}()
	go func() {
		agentDone <- StartFramedPacketRelay(agentTunnel, agentPacketConn, func(err error) PacketRelayError {
			if errors.Is(err, syscall.ECONNREFUSED) {
				return PacketRelayPortUnreachable
			}
			return PacketRelayNoError
		}, nil)
	}()

	wantPayload := []byte("closed-port-probe")
	if _, err := applicationConn.Write(wantPayload); err != nil {
		t.Fatalf("write UDP datagram: %v", err)
	}

	select {
	case got := <-remoteError:
		if got.relayError != PacketRelayPortUnreachable {
			t.Fatalf("relay error = %d, want %d", got.relayError, PacketRelayPortUnreachable)
		}
		if !bytes.Equal(got.payload, wantPayload) {
			t.Fatalf("offending datagram = %q, want %q", got.payload, wantPayload)
		}
		if !bytes.Equal(agentPacketConn.payload, wantPayload) {
			t.Fatalf("agent received datagram = %q, want %q", agentPacketConn.payload, wantPayload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for UDP Port Unreachable")
	}

	select {
	case <-proxyDone:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy relay did not stop after remote error")
	}
	select {
	case <-agentDone:
	case <-time.After(2 * time.Second):
		t.Fatal("agent relay did not stop after connection refused")
	}
}
