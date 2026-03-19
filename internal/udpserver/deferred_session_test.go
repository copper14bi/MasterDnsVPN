// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package udpserver

import (
	"testing"
	"time"

	"masterdnsvpn-go/internal/config"
	DnsParser "masterdnsvpn-go/internal/dnsparser"
	Enums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

func TestDeferredSessionProcessorLeastLoadedAndLaneAffinity(t *testing.T) {
	processor := newDeferredSessionProcessor(2, 4, nil)
	if processor == nil {
		t.Fatal("expected deferred session processor")
	}

	laneA := deferredSessionLane{sessionID: 1, streamID: 10}
	laneB := deferredSessionLane{sessionID: 1, streamID: 11}
	laneC := deferredSessionLane{sessionID: 2, streamID: 20}
	noOp := func() {}

	if !processor.Enqueue(laneA, noOp) {
		t.Fatal("expected first lane enqueue to succeed")
	}
	if !processor.Enqueue(laneB, noOp) {
		t.Fatal("expected second lane enqueue to succeed")
	}
	if !processor.Enqueue(laneC, noOp) {
		t.Fatal("expected third lane enqueue to succeed")
	}
	if !processor.Enqueue(laneA, noOp) {
		t.Fatal("expected repeat lane enqueue to succeed")
	}

	processor.mu.Lock()
	workerA := processor.laneWorker[laneA]
	workerB := processor.laneWorker[laneB]
	workerC := processor.laneWorker[laneC]
	processor.mu.Unlock()

	if workerA != 0 {
		t.Fatalf("unexpected worker for laneA: got=%d want=0", workerA)
	}
	if workerB != 1 {
		t.Fatalf("unexpected worker for laneB: got=%d want=1", workerB)
	}
	if workerC != 0 {
		t.Fatalf("unexpected worker for laneC: got=%d want=0", workerC)
	}

	if got := processor.workers[workerA].pending.Load(); got != 3 {
		t.Fatalf("unexpected pending count for workerA: got=%d want=3", got)
	}
	if got := processor.workers[workerB].pending.Load(); got != 1 {
		t.Fatalf("unexpected pending count for workerB: got=%d want=1", got)
	}
}

func TestHandlePingRequestServesQueuedOutboundPacket(t *testing.T) {
	srv := New(config.ServerConfig{
		MaxPacketSize:     65535,
		Domain:            []string{"a.com"},
		MinVPNLabelLength: 3,
	}, nil, nil)

	record := &sessionRuntimeView{
		ID:             7,
		Cookie:         9,
		ResponseMode:   mtuProbeModeRaw,
		ResponseBase64: false,
	}

	if !srv.queueSessionPacket(record.ID, VpnProto.Packet{
		PacketType:  Enums.PACKET_STREAM_FIN_ACK,
		StreamID:    55,
		SequenceNum: 91,
	}) {
		t.Fatal("expected queued packet")
	}

	ok := srv.handlePingRequest(VpnProto.Packet{SessionID: record.ID}, record)
	if !ok {
		t.Fatal("expected ping handler to accept session packet")
	}
	response := srv.serveQueuedOrPong(
		buildServerTestQuery(0x3003, "vpn.a.com", Enums.DNS_RECORD_TYPE_TXT),
		"vpn.a.com",
		record,
		time.Now(),
	)

	packet, err := DnsParser.ExtractVPNResponse(response, false)
	if err != nil {
		t.Fatalf("ExtractVPNResponse returned error: %v", err)
	}
	if packet.PacketType != Enums.PACKET_STREAM_FIN_ACK {
		t.Fatalf("unexpected packet type: got=%d want=%d", packet.PacketType, Enums.PACKET_STREAM_FIN_ACK)
	}
	if packet.StreamID != 55 || packet.SequenceNum != 91 {
		t.Fatalf("unexpected packet routing: stream=%d seq=%d", packet.StreamID, packet.SequenceNum)
	}
}
