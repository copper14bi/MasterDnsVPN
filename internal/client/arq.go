// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	Enums "masterdnsvpn-go/internal/enums"
)

// StreamState mirrors Python's Stream_State enum
type StreamState int

const (
	StateOpen StreamState = iota
	StateHalfClosedLocal
	StateHalfClosedRemote
	StateClosing
	StateReset
	StateClosed
	StateDraining
)

type arqItem struct {
	Data       []byte
	CreatedAt  time.Time
	LastSentAt time.Time
	Retries    int
	CurrentRTO time.Duration
}

type arqControlItem struct {
	PacketType uint8
	Payload    []byte
	Priority   int
	CreatedAt  time.Time
	LastSentAt time.Time
	Retries    int
	CurrentRTO time.Duration
}

type ARQ struct {
	mu sync.Mutex

	streamID uint16
	stream   *Stream_client
	conn     net.Conn

	// Sequence and buffers
	sndNxt uint16
	rcvNxt uint16
	sndBuf map[uint16]*arqItem
	rcvBuf map[uint16][]byte

	// Control reliability buffer
	controlSndBuf map[uint32]*arqControlItem // key: ptype << 16 | sn

	// Stream lifecycle and flags
	state       StreamState
	closed      bool
	closeReason string

	finSent        bool
	finReceived    bool
	finAcked       bool
	finSeqSent     *uint16
	finSeqReceived *uint16

	rstReceived    bool
	rstSent        bool
	rstAcked       bool
	rstSeqSent     *uint16
	rstSeqReceived *uint16

	localWriteClosed  bool
	remoteWriteClosed bool
	stopLocalRead     bool

	// Configuration (Mirrors Python defaults)
	windowSize      int
	mtu             int
	rto             time.Duration
	maxRTO          time.Duration
	dataPacketTTL   time.Duration
	maxDataRetries  int
	finDrainTimeout time.Duration

	// Concurrency
	ctx    context.Context
	cancel context.CancelFunc
}

// NewARQ initializes a new ARQ instance exactly like Python's __init__.
func NewARQ(streamID uint16, stream *Stream_client, conn net.Conn, mtu int) *ARQ {
	a := &ARQ{
		streamID:        streamID,
		stream:          stream,
		conn:            conn,
		sndBuf:          make(map[uint16]*arqItem),
		rcvBuf:          make(map[uint16][]byte),
		controlSndBuf:   make(map[uint32]*arqControlItem),
		state:           StateOpen,
		windowSize:      600,
		mtu:             mtu,
		rto:             800 * time.Millisecond,
		maxRTO:          1500 * time.Millisecond,
		dataPacketTTL:   600 * time.Second,
		maxDataRetries:  400,
		finDrainTimeout: 300 * time.Second,
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	return a
}

// Start launches the core loops (ioLoop and retransmitLoop).
func (a *ARQ) Start() {
	go a.ioLoop()
	go a.retransmitLoop()
}

// ---------------------------------------------------------------------
// Core Loops
// ---------------------------------------------------------------------

// ioLoop reads from local socket data and enqueues reliable outbound packets.
func (a *ARQ) ioLoop() {
	buf := make([]byte, a.mtu)
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
			// 1. Backpressure check (window_not_full)
			a.mu.Lock()
			if len(a.sndBuf) >= int(float64(a.windowSize)*0.8) {
				a.mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				continue
			}
			a.mu.Unlock()

			// 2. Read from local connection
			n, err := a.conn.Read(buf)
			if err != nil {
				if err == io.EOF {
					a.initiateGracefulClose("Local App Closed Connection (EOF)")
				} else {
					a.Abort("Read Error: " + err.Error())
				}
				return
			}

			if n > 0 {
				a.sendDataPacket(buf[:n])
			}
		}
	}
}

// retransmitLoop mirrors Python's _retransmit_loop.
func (a *ARQ) retransmitLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.checkRetransmits()
		}
	}
}

// ---------------------------------------------------------------------
// Data Plane
// ---------------------------------------------------------------------

func (a *ARQ) sendDataPacket(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed || a.localWriteClosed {
		return
	}

	sn := a.sndNxt
	a.sndNxt++

	payload := append([]byte(nil), data...)
	now := time.Now()

	a.sndBuf[sn] = &arqItem{
		Data:       payload,
		CreatedAt:  now,
		LastSentAt: now,
		Retries:    0,
		CurrentRTO: a.rto,
	}

	// Initial Push to stream priority queue (Priority 3 - Normal)
	a.stream.PushTXPacket(3, Enums.PACKET_STREAM_DATA, sn, payload)
}

// ReceiveData handles inbound STREAM_DATA and emit STREAM_DATA_ACK.
func (a *ARQ) ReceiveData(sn uint16, data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed || a.state == StateReset {
		return
	}

	// 1. Immediate ACK
	a.stream.PushTXPacket(1, Enums.PACKET_STREAM_DATA_ACK, sn, nil)

	// 2. Sequence check
	diff := sn - a.rcvNxt
	if diff >= 32768 {
		return // Old packet or duplicate
	}

	if sn == a.rcvNxt {
		_, _ = a.conn.Write(data)
		a.rcvNxt++

		// Flush buffered out-of-order packets
		for {
			nextData, ok := a.rcvBuf[a.rcvNxt]
			if !ok {
				break
			}
			_, _ = a.conn.Write(nextData)
			delete(a.rcvBuf, a.rcvNxt)
			a.rcvNxt++
		}
	} else {
		// Out of order: Buffer it
		if len(a.rcvBuf) < 1024 {
			a.rcvBuf[sn] = append([]byte(nil), data...)
		}
	}
}

// AcknowledgeData handles inbound STREAM_DATA_ACK.
func (a *ARQ) AcknowledgeData(sn uint16) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sndBuf, sn)
}

// ---------------------------------------------------------------------
// Control Plane
// ---------------------------------------------------------------------

// SendControlPacket enqueues a reliable control packet (SYN, FIN, RST).
func (a *ARQ) SendControlPacket(ptype uint8, payload []byte, priority int, track bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	sn := a.sndNxt
	a.sndNxt++

	if track {
		now := time.Now()
		key := uint32(ptype)<<16 | uint32(sn)
		a.controlSndBuf[key] = &arqControlItem{
			PacketType: ptype,
			Payload:    append([]byte(nil), payload...),
			Priority:   priority,
			CreatedAt:  now,
			LastSentAt: now,
			Retries:    0,
			CurrentRTO: a.rto,
		}
	}

	a.stream.PushTXPacket(priority, ptype, sn, payload)
}

// ReceiveControlAck handles ACKs for control packets.
func (a *ARQ) ReceiveControlAck(ackPtype uint8, sn uint16) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Map ACK type back to request type (Python logic)
	// Example: STREAM_SYN_ACK corresponds to STREAM_SYN
	reqPtype := a.getReverseControlType(ackPtype)
	key := uint32(reqPtype)<<16 | uint32(sn)
	delete(a.controlSndBuf, key)

	// Lifecycle hooks
	if reqPtype == Enums.PACKET_STREAM_FIN {
		a.finAcked = true
	}
}

// ---------------------------------------------------------------------
// Lifecycle & State
// ---------------------------------------------------------------------

func (a *ARQ) initiateGracefulClose(reason string) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closeReason = reason
	a.state = StateDraining
	a.mu.Unlock()

	// Drain send buffer then close with FIN
	go func() {
		deadline := time.Now().Add(a.finDrainTimeout)
		for time.Now().Before(deadline) {
			a.mu.Lock()
			if len(a.sndBuf) == 0 {
				a.mu.Unlock()
				break
			}
			a.mu.Unlock()
			time.Sleep(50 * time.Millisecond)
		}
		a.Close("Graceful close complete", true)
	}()
}

func (a *ARQ) Close(reason string, sendFin bool) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	a.closeReason = reason
	a.cancel()
	a.mu.Unlock()

	if sendFin {
		a.SendControlPacket(Enums.PACKET_STREAM_FIN, nil, 0, true)
	}

	if a.conn != nil {
		_ = a.conn.Close()
	}
}

func (a *ARQ) Abort(reason string) {
	a.SendControlPacket(Enums.PACKET_STREAM_RST, nil, 0, false)
	a.Close(reason, false)
}

// ---------------------------------------------------------------------
// Retransmission Logic
// ---------------------------------------------------------------------

func (a *ARQ) checkRetransmits() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

	// 1. Data retransmits
	for sn, item := range a.sndBuf {
		if now.Sub(item.LastSentAt) >= item.CurrentRTO {
			item.Retries++
			if item.Retries > a.maxDataRetries {
				go a.Abort("Max data retries exceeded")
				return
			}
			// Binary exponential backoff
			item.CurrentRTO = time.Duration(float64(item.CurrentRTO) * 1.5)
			if item.CurrentRTO > a.maxRTO {
				item.CurrentRTO = a.maxRTO
			}
			item.LastSentAt = now
			a.stream.PushTXPacket(3, Enums.PACKET_STREAM_RESEND, sn, item.Data)
		}
	}

	// 2. Control retransmits
	for key, item := range a.controlSndBuf {
		if now.Sub(item.LastSentAt) >= item.CurrentRTO {
			item.Retries++
			if item.Retries > 15 { // Max control retries
				delete(a.controlSndBuf, key)
				continue
			}
			item.CurrentRTO = time.Duration(float64(item.CurrentRTO) * 1.5)
			item.LastSentAt = now
			a.stream.PushTXPacket(item.Priority, item.PacketType, uint16(key&0xFFFF), item.Payload)
		}
	}
}

// Helpers
func (a *ARQ) getReverseControlType(ackPtype uint8) uint8 {
	switch ackPtype {
	case Enums.PACKET_STREAM_SYN_ACK:
		return Enums.PACKET_STREAM_SYN
	case Enums.PACKET_STREAM_FIN_ACK:
		return Enums.PACKET_STREAM_FIN
	case Enums.PACKET_STREAM_RST_ACK:
		return Enums.PACKET_STREAM_RST
	case Enums.PACKET_SOCKS5_SYN_ACK:
		return Enums.PACKET_SOCKS5_SYN
	}
	return 0
}
