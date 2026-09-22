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
	"io"
	"strings"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	var buffer bytes.Buffer

	baseEnvelope := InfoReplyPacket{Name: "hello"}
	enc := NewEncoder(&buffer)
	if err := enc.Encode(baseEnvelope); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Envelope created: %+v\n", buffer)

	dec := NewDecoder(&buffer)
	if err := dec.Decode(); err != nil {
		if err != io.EOF {
			t.Fatal(err)
		}
	}

	fmt.Printf("Envelope: %+v\n", dec.Payload)

	if dec.Payload.(*InfoReplyPacket).Name != "hello" {
		t.Fatal("invalid packet decoded")
	}

}

func TestConnectPacketsPreserveFramedUDPNegotiation(t *testing.T) {
	tests := []struct {
		name   string
		packet interface{}
		framed func(interface{}) bool
	}{
		{
			name:   "request",
			packet: ConnectRequestPacket{Transport: TransportUDP, FramedUDP: true},
			framed: func(payload interface{}) bool {
				return payload.(*ConnectRequestPacket).FramedUDP
			},
		},
		{
			name:   "response",
			packet: ConnectResponsePacket{Established: true, FramedUDP: true},
			framed: func(payload interface{}) bool {
				return payload.(*ConnectResponsePacket).FramedUDP
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buffer bytes.Buffer
			encoder := NewEncoder(&buffer)
			if err := encoder.Encode(tt.packet); err != nil {
				t.Fatalf("Encode: %v", err)
			}

			decoder := NewDecoder(&buffer)
			if err := decoder.Decode(); err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !tt.framed(decoder.Payload) {
				t.Fatal("FramedUDP negotiation flag was lost")
			}
		})
	}
}

func TestDecodeRejectsHugeDeclaredContainer(t *testing.T) {
	var buffer bytes.Buffer
	buffer.WriteByte(MessageInfoReply)
	buffer.Write([]byte{
		0x83,
		0xa4, 'N', 'a', 'm', 'e',
		0xa5, 'a', 'g', 'e', 'n', 't',
		0xaa, 'I', 'n', 't', 'e', 'r', 'f', 'a', 'c', 'e', 's',
		0xdd,
	})
	if err := binary.Write(&buffer, binary.BigEndian, uint32(MaxPacketSize+1)); err != nil {
		t.Fatal(err)
	}

	decoder := NewDecoder(&buffer)
	err := decoder.Decode()
	if err == nil {
		t.Fatal("expected an error for an oversized declared container")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected limit error, got %v", err)
	}
	if decoder.Payload != nil {
		t.Fatal("payload should not be set after a rejected packet")
	}
}

func TestDecodeRecoversFromMsgpackPanic(t *testing.T) {
	packet := []byte{
		MessageInfoReply,
		0x83,
		0xa4, 'N', 'a', 'm', 'e',
		0xa5, 'a', 'g', 'e', 'n', 't',
		0xaa, 'I', 'n', 't', 'e', 'r', 'f', 'a', 'c', 'e', 's',
		0xa3, 'b', 'a', 'd',
		0xa9, 'S', 'e', 's', 's', 'i', 'o', 'n', 'I', 'D',
		0xa0,
	}

	decoder := NewDecoder(bytes.NewReader(packet))
	err := decoder.Decode()
	if err == nil {
		t.Fatal("expected malformed slice field to return an error")
	}
	if !strings.Contains(err.Error(), "msgpack decoder panic") {
		t.Fatalf("expected recovered panic error, got %v", err)
	}
}

func TestPayloadAsRejectsUnexpectedPacketType(t *testing.T) {
	if _, err := PayloadAs[InfoReplyPacket](&ConnectRequestPacket{}); err == nil {
		t.Fatal("expected unexpected payload type error")
	}
}

func BenchmarkEncodeDecode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var buffer bytes.Buffer
		baseEnvelope := InfoReplyPacket{Name: "hello"}
		enc := NewEncoder(&buffer)
		if err := enc.Encode(baseEnvelope); err != nil {
			b.Fatal(err)
		}

		dec := NewDecoder(&buffer)
		if err := dec.Decode(); err != nil {
			if err != io.EOF {
				b.Fatal(err)
			}
		}

		if dec.Payload.(*InfoReplyPacket).Name != "hello" {
			b.Fatal("invalid packet decoded")
		}
	}
}
