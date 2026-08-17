package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestAssemblyGlobalOriginUsesCollectorFilename(t *testing.T) {
	span, err := globalOriginSpan("globals.lua", ast.Position{Line: 2, Column: 3, EndLine: 2, EndColumn: 7})
	if err != nil || span.File != "globals.lua" || span.StartLine != 2 || span.EndCol != 7 {
		t.Fatalf("globalOriginSpan = %#v, %v", span, err)
	}
	if _, err := globalOriginSpan("globals.lua", ast.Position{File: "other.lua"}); err == nil {
		t.Fatal("globalOriginSpan accepted a foreign source file")
	}
}
