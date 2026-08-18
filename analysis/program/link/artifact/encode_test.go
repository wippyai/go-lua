package artifact

import (
	"errors"
	"testing"
)

// TestArtifactEncoderEnforcesBoundedOutput proves the encoder publishes no
// byte past its limit. The stream header alone crosses a one-byte bound, so
// the refusal is observable before any Link row is read, and the sink is
// checked directly for the first payload that would cross its limit.
func TestArtifactEncoderEnforcesBoundedOutput(t *testing.T) {
	if _, err := encodeBounded(nil, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("bounded artifact encoding error = %v, want ErrLimit", err)
	}
	buffer := &artifactBuffer{limit: 4}
	if _, err := buffer.WriteString("abcd"); err != nil {
		t.Fatalf("within-limit artifact write = %v", err)
	}
	if _, err := buffer.Write([]byte{0}); !errors.Is(err, ErrLimit) {
		t.Fatalf("over-limit artifact write = %v, want ErrLimit", err)
	}
	if buffer.data.Len() != 4 {
		t.Fatalf("published artifact bytes = %d, want 4", buffer.data.Len())
	}
}
