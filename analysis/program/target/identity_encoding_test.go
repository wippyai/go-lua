package target

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/internal/framing"
)

func TestIdentityEncodingFramesDistinctInputCoordinates(t *testing.T) {
	encode := func(input InputSource) []byte {
		hash := sha256.New()
		var writer framing.Writer
		if err := writer.Reset(hash, "target/identity-test", 1); err != nil {
			t.Fatal(err)
		}
		if err := encodeInput(&writer, input); err != nil {
			t.Fatal(err)
		}
		if err := writer.Finish(); err != nil {
			t.Fatal(err)
		}
		return hash.Sum(nil)
	}
	formal := encode(InputSource{Kind: InputSourceValueFormal, Ordinal: 0})
	values := encode(InputSource{Kind: InputSourceValuesVar, Ordinal: 0})
	if bytes.Equal(formal, values) {
		t.Fatal("identity framing collapsed distinct input-source kinds")
	}
}
