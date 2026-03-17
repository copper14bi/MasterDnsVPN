// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package udpserver

import (
	"encoding/binary"
	"testing"

	"masterdnsvpn-go/internal/config"
	"masterdnsvpn-go/internal/enums"
)

func TestHandlePacketDropsDNSResponses(t *testing.T) {
	srv := New(config.ServerConfig{
		MaxPacketSize:     65535,
		Domain:            []string{"a.com"},
		MinVPNLabelLength: 3,
	}, nil)

	packet := buildServerTestQuery(0x1001, "vpn.a.com", enums.DNSRecordTypeTXT)
	packet[2] |= 0x80

	if response := srv.handlePacket(packet); response != nil {
		t.Fatal("handlePacket should drop DNS response packets")
	}
}

func TestHandlePacketReturnsNoDataForUnauthorizedDomain(t *testing.T) {
	srv := New(config.ServerConfig{
		MaxPacketSize:     65535,
		Domain:            []string{"a.com"},
		MinVPNLabelLength: 3,
	}, nil)

	packet := buildServerTestQuery(0x2002, "evil.com", enums.DNSRecordTypeTXT)
	response := srv.handlePacket(packet)
	if len(response) == 0 {
		t.Fatal("handlePacket should return a DNS response for unauthorized DNS queries")
	}

	if got := binary.BigEndian.Uint16(response[0:2]); got != 0x2002 {
		t.Fatalf("unexpected response id: got=%#x want=%#x", got, 0x2002)
	}
	flags := binary.BigEndian.Uint16(response[2:4])
	if flags&0x000F != enums.DNSRCodeNoError {
		t.Fatalf("unexpected rcode: got=%d want=%d", flags&0x000F, enums.DNSRCodeNoError)
	}
}

func buildServerTestQuery(id uint16, name string, qtype uint16) []byte {
	qname := encodeServerTestName(name)
	packet := make([]byte, 12+len(qname)+4)
	binary.BigEndian.PutUint16(packet[0:2], id)
	binary.BigEndian.PutUint16(packet[2:4], 0x0100)
	binary.BigEndian.PutUint16(packet[4:6], 1)

	offset := 12
	offset += copy(packet[offset:], qname)
	binary.BigEndian.PutUint16(packet[offset:offset+2], qtype)
	binary.BigEndian.PutUint16(packet[offset+2:offset+4], enums.DNSQClassIN)
	return packet
}

func encodeServerTestName(name string) []byte {
	encoded := make([]byte, 0, len(name)+2)
	labelStart := 0
	for i := 0; i <= len(name); i++ {
		if i != len(name) && name[i] != '.' {
			continue
		}
		encoded = append(encoded, byte(i-labelStart))
		encoded = append(encoded, name[labelStart:i]...)
		labelStart = i + 1
	}
	return append(encoded, 0)
}
