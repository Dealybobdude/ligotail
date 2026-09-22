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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// maxUDPDatagramSize is large enough to hold the biggest possible UDP payload,
// so ReadFromUDP never truncates a datagram.
const maxUDPDatagramSize = 65535

// PacketRelayError is an error reported by the remote side of a framed packet
// relay.
type PacketRelayError uint8

const (
	PacketRelayNoError PacketRelayError = iota
	PacketRelayPortUnreachable
)

const (
	packetRelayFrameData uint8 = iota
	packetRelayFrameError
)

const packetRelayFrameHeaderSize = 6

var errRemotePacketRelay = errors.New("remote packet relay error")

func relay(src net.Conn, dst net.Conn, stop chan error, closeOnError bool) {
	_, err := io.Copy(dst, src)
	if closeOnError {
		dst.Close()
		src.Close()
	}

	stop <- err
	return
}

func StartRelay(src net.Conn, dst net.Conn) error {
	stop := make(chan error, 2)

	go relay(src, dst, stop, true)
	go relay(dst, src, stop, true)

	select {
	case err := <-stop:
		return err
	}
}

func StartPacketRelay(src net.Conn, dst net.Conn) error {
	stop := make(chan error, 2)

	go relay(src, dst, stop, true)
	go relay(dst, src, stop, true)

	select {
	case err := <-stop:
		return err
	}
}

// StartFramedPacketRelay relays datagrams without losing packet boundaries and
// carries asynchronous network errors over the stream tunnel. classifyError is
// called for errors raised by packetConn. onRemoteError receives errors raised
// by the packet socket on the other side together with the last datagram sent
// to it.
func StartFramedPacketRelay(
	tunnel net.Conn,
	packetConn net.Conn,
	classifyError func(error) PacketRelayError,
	onRemoteError func(PacketRelayError, []byte),
) error {
	stop := make(chan error, 2)

	var tunnelWriteMu sync.Mutex
	var lastOutboundMu sync.Mutex
	var lastOutboundPacket []byte
	writeAll := func(data []byte) error {
		for len(data) != 0 {
			n, err := tunnel.Write(data)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			data = data[n:]
		}
		return nil
	}
	writeFrame := func(frameType uint8, relayError PacketRelayError, payload []byte) error {
		if len(payload) > maxUDPDatagramSize {
			return fmt.Errorf("packet relay payload too large: %d", len(payload))
		}

		header := make([]byte, packetRelayFrameHeaderSize)
		header[0] = frameType
		header[1] = byte(relayError)
		binary.BigEndian.PutUint32(header[2:], uint32(len(payload)))

		tunnelWriteMu.Lock()
		defer tunnelWriteMu.Unlock()
		if err := writeAll(header); err != nil {
			return err
		}
		return writeAll(payload)
	}

	reportLocalError := func(err error) {
		if classifyError != nil {
			if relayError := classifyError(err); relayError != PacketRelayNoError {
				if writeErr := writeFrame(packetRelayFrameError, relayError, nil); writeErr != nil {
					stop <- writeErr
					return
				}
			}
		}
		stop <- err
	}

	// Packet socket -> tunnel.
	go func() {
		buf := make([]byte, maxUDPDatagramSize)
		for {
			n, err := packetConn.Read(buf)
			if err == nil || n > 0 {
				lastOutboundMu.Lock()
				lastOutboundPacket = append(lastOutboundPacket[:0], buf[:n]...)
				lastOutboundMu.Unlock()
				if writeErr := writeFrame(packetRelayFrameData, PacketRelayNoError, buf[:n]); writeErr != nil {
					stop <- writeErr
					return
				}
			}
			if err != nil {
				reportLocalError(err)
				return
			}
		}
	}()

	// Tunnel -> packet socket.
	go func() {
		header := make([]byte, packetRelayFrameHeaderSize)
		for {
			if _, err := io.ReadFull(tunnel, header); err != nil {
				stop <- err
				return
			}

			payloadLen := binary.BigEndian.Uint32(header[2:])
			if payloadLen > maxUDPDatagramSize {
				stop <- fmt.Errorf("invalid packet relay payload length: %d", payloadLen)
				return
			}
			payload := make([]byte, int(payloadLen))
			if _, err := io.ReadFull(tunnel, payload); err != nil {
				stop <- err
				return
			}

			switch header[0] {
			case packetRelayFrameData:
				n, err := packetConn.Write(payload)
				if err == nil && n != len(payload) {
					err = io.ErrShortWrite
				}
				if err != nil {
					reportLocalError(err)
					return
				}
			case packetRelayFrameError:
				if payloadLen != 0 {
					stop <- errors.New("packet relay error frame has a payload")
					return
				}
				if onRemoteError != nil {
					lastOutboundMu.Lock()
					lastPacket := append([]byte(nil), lastOutboundPacket...)
					lastOutboundMu.Unlock()
					onRemoteError(PacketRelayError(header[1]), lastPacket)
				}
				stop <- errRemotePacketRelay
				return
			default:
				stop <- fmt.Errorf("unknown packet relay frame type: %d", header[0])
				return
			}
		}
	}()

	err := <-stop
	_ = packetConn.Close()
	_ = tunnel.Close()
	return err
}

// StartUDPListenerRelay relays datagrams between a reverse UDP listener
// (udpConn, an unconnected socket from net.ListenUDP) and the tunnel back to
// the proxy. Unlike StartRelay, it cannot pump raw bytes with io.Copy: the
// listening socket has no fixed peer, so Write always fails and return traffic
// must be sent with WriteToUDP. The address of the most recent client is
// tracked from ReadFromUDP so replies coming back through the tunnel reach it.
//
// Return traffic is routed to the most recently seen client, which serves the
// common single-client case; the listening socket carries no per-client
// demultiplexing, so concurrent clients would share the return path.
func StartUDPListenerRelay(tunnel net.Conn, udpConn *net.UDPConn) error {
	stop := make(chan error, 2)

	var mu sync.Mutex
	var clientAddr *net.UDPAddr

	// Listener -> tunnel: forward client datagrams to the proxy, remembering
	// where each one came from so return traffic can be addressed back.
	go func() {
		buf := make([]byte, maxUDPDatagramSize)
		for {
			n, addr, err := udpConn.ReadFromUDP(buf)
			if n > 0 {
				mu.Lock()
				clientAddr = addr
				mu.Unlock()
				if _, werr := tunnel.Write(buf[:n]); werr != nil {
					stop <- werr
					return
				}
			}
			if err != nil {
				stop <- err
				return
			}
		}
	}()

	// Tunnel -> listener: deliver replies to the last known client using
	// WriteToUDP, since the listening socket is not connected to a peer.
	go func() {
		buf := make([]byte, maxUDPDatagramSize)
		for {
			n, err := tunnel.Read(buf)
			if n > 0 {
				mu.Lock()
				addr := clientAddr
				mu.Unlock()
				if addr != nil {
					if _, werr := udpConn.WriteToUDP(buf[:n], addr); werr != nil {
						stop <- werr
						return
					}
				}
			}
			if err != nil {
				stop <- err
				return
			}
		}
	}()

	err := <-stop
	udpConn.Close()
	tunnel.Close()
	return err
}
