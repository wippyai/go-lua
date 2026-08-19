package lower_test

import (
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
)

// TestLowerPublishesAParsedChunk is the one root-level contract for the
// public lowering facade. Detailed source, flow, static, and corpus laws live
// under the acceptance owner.
func TestLowerPublishesAParsedChunk(t *testing.T) {
	program, err := programlower.Lower(programlower.Source{Name: "source.lua", Text: []byte("return 1\n")})
	if err != nil || program == nil {
		t.Fatalf("Lower returned program=%v, err=%v", program, err)
	}
}
