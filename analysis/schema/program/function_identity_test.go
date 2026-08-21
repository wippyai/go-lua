package programschema

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestCallFormalIdentityCommitsTheSealedBodyContext(t *testing.T) {
	context := identity.ContentID(sha256.Sum256([]byte("body-context")))
	got, ok := CallFormalIdentity(context)
	if !ok {
		t.Fatal("available Body context did not issue a Call formal identity")
	}
	var payload [8 + 8 + sha256.Size]byte
	copy(payload[:8], "pcallbod")
	binary.BigEndian.PutUint64(payload[8:16], 1)
	copy(payload[16:], context[:])
	if want := identity.ContentID(sha256.Sum256(payload[:])); got != want {
		t.Fatalf("Call formal identity = %x, want %x", got, want)
	}
	if zero, zeroOK := CallFormalIdentity(identity.ContentID{}); zeroOK || zero.Available() {
		t.Fatal("zero Body context issued a Call formal identity")
	}
}
