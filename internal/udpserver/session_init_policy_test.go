package udpserver

import (
	"encoding/binary"
	"testing"
)

func TestSessionInitPolicyMTULimitsAreAppliedToServerSession(t *testing.T) {
	store := newSessionStore(16, 32)
	payload := make([]byte, sessionInitDataSize)
	payload[0] = mtuProbeModeRaw
	binary.BigEndian.PutUint16(payload[2:4], 999)
	binary.BigEndian.PutUint16(payload[4:6], 9999)
	copy(payload[6:10], []byte{1, 2, 3, 4})

	record, reused, err := store.findOrCreate(payload, 0, 0, 10, 150, 2048)
	if err != nil {
		t.Fatalf("findOrCreate returned error: %v", err)
	}
	if reused {
		t.Fatal("expected first session init not to be reused")
	}
	if record.UploadMTU != 150 {
		t.Fatalf("unexpected upload mtu clamp: got=%d want=%d", record.UploadMTU, 150)
	}
	if record.DownloadMTU != 2048 {
		t.Fatalf("unexpected download mtu clamp: got=%d want=%d", record.DownloadMTU, 2048)
	}
}
