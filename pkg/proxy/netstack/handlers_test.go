// Ligolo-ng
// Copyright (C) 2025 Nicolas Chatelain (nicocha30)

package netstack

import (
	"bytes"
	"testing"

	"github.com/nicocha30/gvisor-ligolo/pkg/tcpip"
	"github.com/nicocha30/gvisor-ligolo/pkg/tcpip/header"
	"github.com/nicocha30/gvisor-ligolo/pkg/tcpip/link/channel"
	"github.com/nicocha30/gvisor-ligolo/pkg/tcpip/network/ipv4"
	"github.com/nicocha30/gvisor-ligolo/pkg/tcpip/stack"
	"github.com/nicocha30/gvisor-ligolo/pkg/tcpip/transport/udp"
)

func TestSendUDPPortUnreachable(t *testing.T) {
	ns := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{udp.NewProtocol},
	})
	defer ns.Close()

	linkEP := channel.New(1, 1500, "")
	defer linkEP.Close()
	if err := ns.CreateNIC(1, linkEP); err != nil {
		t.Fatalf("CreateNIC: %v", err)
	}

	targetAddress := tcpip.AddrFrom4([4]byte{192, 0, 2, 10})
	clientAddress := tcpip.AddrFrom4([4]byte{198, 51, 100, 20})
	ns.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}})
	if err := ns.SetPromiscuousMode(1, true); err != nil {
		t.Fatalf("SetPromiscuousMode: %v", err)
	}
	if err := ns.SetSpoofing(1, true); err != nil {
		t.Fatalf("SetSpoofing: %v", err)
	}

	wantPayload := []byte("udp-probe")
	endpointID := stack.TransportEndpointID{
		LocalPort:     53,
		LocalAddress:  targetAddress,
		RemotePort:    42000,
		RemoteAddress: clientAddress,
	}
	if err := sendUDPPortUnreachable(ns, endpointID, wantPayload); err != nil {
		t.Fatalf("sendUDPPortUnreachable: %v", err)
	}

	pkt := linkEP.Read()
	if pkt.IsNil() {
		t.Fatal("no ICMP packet was emitted")
	}
	defer pkt.DecRef()

	ipHeader := header.IPv4(pkt.NetworkHeader().Slice())
	if got := ipHeader.SourceAddress(); got != targetAddress {
		t.Fatalf("ICMP source = %s, want %s", got, targetAddress)
	}
	if got := ipHeader.DestinationAddress(); got != clientAddress {
		t.Fatalf("ICMP destination = %s, want %s", got, clientAddress)
	}

	icmpHeader := header.ICMPv4(pkt.TransportHeader().Slice())
	if got := icmpHeader.Type(); got != header.ICMPv4DstUnreachable {
		t.Fatalf("ICMP type = %d, want %d", got, header.ICMPv4DstUnreachable)
	}
	if got := icmpHeader.Code(); got != header.ICMPv4PortUnreachable {
		t.Fatalf("ICMP code = %d, want %d", got, header.ICMPv4PortUnreachable)
	}

	quoted, ok := pkt.Data().PullUp(header.IPv4MinimumSize + header.UDPMinimumSize + len(wantPayload))
	if !ok {
		t.Fatal("ICMP packet does not contain the offending UDP datagram")
	}
	quotedIP := header.IPv4(quoted)
	quotedUDP := header.UDP(quoted[header.IPv4MinimumSize:])
	if got := quotedIP.SourceAddress(); got != clientAddress {
		t.Fatalf("quoted IP source = %s, want %s", got, clientAddress)
	}
	if got := quotedIP.DestinationAddress(); got != targetAddress {
		t.Fatalf("quoted IP destination = %s, want %s", got, targetAddress)
	}
	if got := quotedUDP.SourcePort(); got != endpointID.RemotePort {
		t.Fatalf("quoted UDP source port = %d, want %d", got, endpointID.RemotePort)
	}
	if got := quotedUDP.DestinationPort(); got != endpointID.LocalPort {
		t.Fatalf("quoted UDP destination port = %d, want %d", got, endpointID.LocalPort)
	}
	if got := quoted[header.IPv4MinimumSize+header.UDPMinimumSize:]; !bytes.Equal(got, wantPayload) {
		t.Fatalf("quoted UDP payload = %q, want %q", got, wantPayload)
	}
}
