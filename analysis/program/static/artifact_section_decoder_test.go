package static

import (
	"testing"

	"github.com/wippyai/go-lua/internal/framing"
)

func decodeStaticArtifactInputForTest(t *testing.T, input Input) Input {
	t.Helper()
	component := staticContentComponent(t, input)
	reader := newStaticArtifactReader(t, encodeStaticArtifactComponent(t, component, true))
	if got, err := reader.Record(); err != nil || got != staticArtifactTestRoot {
		t.Fatalf("artifact root record = %d/%v, want %d", got, err, staticArtifactTestRoot)
	}
	decoded, err := ReadArtifactSection(reader)
	if err != nil {
		t.Fatalf("ReadArtifactSection() error = %v", err)
	}
	if got, err := reader.Record(); err != nil || got != staticArtifactTestSentinel {
		t.Fatalf("artifact suffix = %d/%v, want %d", got, err, staticArtifactTestSentinel)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	return decoded
}

func TestArtifactDecoderPreflightsMissingSectionPayload(t *testing.T) {
	data := encodeStaticMalformedSection(t, func(writer *framing.Writer) {
		if err := writer.Record(staticArtifactRecordTypes); err != nil {
			t.Fatal(err)
		}
	})
	reader := newStaticArtifactReader(t, data)
	if got, err := reader.Record(); err != nil || got != staticArtifactTestRoot {
		t.Fatalf("artifact root record = %d/%v, want %d", got, err, staticArtifactTestRoot)
	}
	decoded, err := ReadArtifactSection(reader)
	if err == nil {
		t.Fatal("decoder accepted a section with no Types row counts")
	}
	if !staticArtifactInputEmpty(decoded) {
		t.Fatal("decoder returned partial input after preflight failure")
	}
}
