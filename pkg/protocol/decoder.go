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

package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/shamaton/msgpack/v2"
	"io"
)

const (
	// MaxPacketSize caps one Ligolo control-packet payload. Data streams are
	// relayed outside msgpack, so control messages should remain small.
	MaxPacketSize = 4 * 1024 * 1024
	maxDepth      = 128
)

// LigoloDecoder is the struct containing the decoded Envelope and the reader
type LigoloDecoder struct {
	reader  io.Reader
	Payload interface{}
}

// NewDecoder decode Ligolo-ng packets
func NewDecoder(reader io.Reader) LigoloDecoder {
	return LigoloDecoder{reader: reader}
}

func interfaceFromPayloadType(payloadType uint8) (interface{}, error) {
	switch payloadType {
	case MessageInfoRequest:
		return &InfoRequestPacket{}, nil
	case MessageInfoReply:
		return &InfoReplyPacket{}, nil
	case MessageConnectRequest:
		return &ConnectRequestPacket{}, nil
	case MessageConnectResponse:
		return &ConnectResponsePacket{}, nil
	case MessageHostPingRequest:
		return &HostPingRequestPacket{}, nil
	case MessageHostPingResponse:
		return &HostPingResponsePacket{}, nil
	case MessageListenerRequest:
		return &ListenerRequestPacket{}, nil
	case MessageListenerResponse:
		return &ListenerResponsePacket{}, nil
	case MessageListenerBindRequest:
		return &ListenerBindPacket{}, nil
	case MessageListenerBindResponse:
		return &ListenerBindReponse{}, nil
	case MessageListenerSockRequest:
		return &ListenerSockRequestPacket{}, nil
	case MessageListenerSockResponse:
		return &ListenerSockResponsePacket{}, nil
	case MessageListenerCloseRequest:
		return &ListenerCloseRequestPacket{}, nil
	case MessageListenerCloseResponse:
		return &ListenerCloseResponsePacket{}, nil
	case MessageAgentKillRequest:
		return &AgentKillRequestPacket{}, nil
	case MessageListenerSocketConnectionReady:
		return &ListenerSocketConnectionReady{}, nil
	default:
		return nil, fmt.Errorf("decode called for unknown payload type: %d", payloadType)
	}
}

func unmarshalMsgpack(data []byte, v interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("msgpack decoder panic: %v", r)
		}
	}()
	return msgpack.Unmarshal(data, v)
}

type msgpackPreflightReader struct {
	reader    io.Reader
	buffer    bytes.Buffer
	remaining int
	limit     int
}

func readMsgpackObject(reader io.Reader, limit int) ([]byte, error) {
	p := msgpackPreflightReader{reader: reader, remaining: limit, limit: limit}
	if err := p.readValue(0); err != nil {
		return nil, err
	}
	return p.buffer.Bytes(), nil
}

func (p *msgpackPreflightReader) readByte() (byte, error) {
	var b [1]byte
	if err := p.readFull(b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

func (p *msgpackPreflightReader) readFull(buf []byte) error {
	if len(buf) > p.remaining {
		return fmt.Errorf("msgpack object exceeds %d byte limit", p.limit)
	}
	if _, err := io.ReadFull(p.reader, buf); err != nil {
		return err
	}
	p.remaining -= len(buf)
	_, _ = p.buffer.Write(buf)
	return nil
}

func (p *msgpackPreflightReader) readUint(size int) (uint64, error) {
	buf := make([]byte, size)
	if err := p.readFull(buf); err != nil {
		return 0, err
	}
	switch size {
	case 1:
		return uint64(buf[0]), nil
	case 2:
		return uint64(binary.BigEndian.Uint16(buf)), nil
	case 4:
		return uint64(binary.BigEndian.Uint32(buf)), nil
	default:
		return 0, fmt.Errorf("invalid msgpack integer size: %d", size)
	}
}

func (p *msgpackPreflightReader) readBytes(length uint64) error {
	if length > uint64(p.remaining) {
		return fmt.Errorf("msgpack declared length %d exceeds %d byte limit", length, p.limit)
	}
	buf := make([]byte, int(length))
	return p.readFull(buf)
}

func (p *msgpackPreflightReader) readContainerValues(count uint64, depth int) error {
	if count > uint64(p.remaining) {
		return fmt.Errorf("msgpack container with %d values exceeds %d byte limit", count, p.limit)
	}
	for i := uint64(0); i < count; i++ {
		if err := p.readValue(depth + 1); err != nil {
			return err
		}
	}
	return nil
}

func (p *msgpackPreflightReader) readValue(depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("msgpack object exceeds nesting depth limit")
	}

	marker, err := p.readByte()
	if err != nil {
		return err
	}

	switch {
	case marker <= 0x7f || marker >= 0xe0:
		return nil
	case marker >= 0x80 && marker <= 0x8f:
		return p.readContainerValues(uint64(marker&0x0f)*2, depth)
	case marker >= 0x90 && marker <= 0x9f:
		return p.readContainerValues(uint64(marker&0x0f), depth)
	case marker >= 0xa0 && marker <= 0xbf:
		return p.readBytes(uint64(marker & 0x1f))
	}

	switch marker {
	case 0xc0, 0xc2, 0xc3:
		return nil
	case 0xc1:
		return fmt.Errorf("invalid msgpack marker 0xc1")
	case 0xc4, 0xd9:
		length, err := p.readUint(1)
		if err != nil {
			return err
		}
		return p.readBytes(length)
	case 0xc5, 0xda:
		length, err := p.readUint(2)
		if err != nil {
			return err
		}
		return p.readBytes(length)
	case 0xc6, 0xdb:
		length, err := p.readUint(4)
		if err != nil {
			return err
		}
		return p.readBytes(length)
	case 0xc7:
		length, err := p.readUint(1)
		if err != nil {
			return err
		}
		return p.readBytes(length + 1)
	case 0xc8:
		length, err := p.readUint(2)
		if err != nil {
			return err
		}
		return p.readBytes(length + 1)
	case 0xc9:
		length, err := p.readUint(4)
		if err != nil {
			return err
		}
		return p.readBytes(length + 1)
	case 0xca:
		return p.readBytes(4)
	case 0xcb:
		return p.readBytes(8)
	case 0xcc, 0xd0:
		return p.readBytes(1)
	case 0xcd, 0xd1:
		return p.readBytes(2)
	case 0xce, 0xd2:
		return p.readBytes(4)
	case 0xcf, 0xd3:
		return p.readBytes(8)
	case 0xd4:
		return p.readBytes(2)
	case 0xd5:
		return p.readBytes(3)
	case 0xd6:
		return p.readBytes(5)
	case 0xd7:
		return p.readBytes(9)
	case 0xd8:
		return p.readBytes(17)
	case 0xdc:
		count, err := p.readUint(2)
		if err != nil {
			return err
		}
		return p.readContainerValues(count, depth)
	case 0xdd:
		count, err := p.readUint(4)
		if err != nil {
			return err
		}
		return p.readContainerValues(count, depth)
	case 0xde:
		count, err := p.readUint(2)
		if err != nil {
			return err
		}
		return p.readContainerValues(count*2, depth)
	case 0xdf:
		count, err := p.readUint(4)
		if err != nil {
			return err
		}
		return p.readContainerValues(count*2, depth)
	default:
		return fmt.Errorf("unsupported msgpack marker 0x%x", marker)
	}
}

func PayloadAs[T any](payload interface{}) (*T, error) {
	typed, ok := payload.(*T)
	if !ok {
		var expected *T
		return nil, fmt.Errorf("unexpected payload type %T, expected %T", payload, expected)
	}
	return typed, nil
}

// Decode read content from the reader and fill the Envelope
func (d *LigoloDecoder) Decode() error {
	payloadTypeData, err := readMsgpackObject(d.reader, MaxPacketSize)
	if err != nil {
		return fmt.Errorf("decoder: unable to read payload type: %v", err)
	}

	var payloadType uint8
	if err := unmarshalMsgpack(payloadTypeData, &payloadType); err != nil {
		return fmt.Errorf("decoder: unable to decode payload type: %v", err)
	}
	p, err := interfaceFromPayloadType(payloadType)
	if err != nil {
		return fmt.Errorf("decoder: unable to get interface from payload: %v", err)
	}

	payloadData, err := readMsgpackObject(d.reader, MaxPacketSize)
	if err != nil {
		return fmt.Errorf("decoder: unable to read payload: %v", err)
	}

	if err := unmarshalMsgpack(payloadData, p); err != nil {
		return fmt.Errorf("decoder: unable to decode payload: %v", err)
	}
	d.Payload = p

	return nil
}
