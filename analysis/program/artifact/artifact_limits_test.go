package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/internal/framing"
)

func TestArtifactBufferPublishesOnlyWithinItsLimit(t *testing.T) {
	buffer := newArtifactBuffer(3)
	if written, err := buffer.WriteString("ab"); written != 2 || err != nil {
		t.Fatalf("first write = %d/%v", written, err)
	}
	if written, err := buffer.Write([]byte("xy")); written != 0 || err != ErrLimit || string(buffer.Bytes()) != "ab" {
		t.Fatalf("overflow write = %d/%v bytes=%q", written, err, buffer.Bytes())
	}
	if artifactMeasureAllowed(framing.StreamMeasure{Events: artifactMaxEvents + 1}) {
		t.Fatal("event limit was not enforced")
	}
}
