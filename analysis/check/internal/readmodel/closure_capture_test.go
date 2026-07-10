package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestForEachClosureCaptureProjectsSolvedCaptureFacts(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Box = { value: string }
local box: Box = { value = "ok" }
local function get(): string
    return box.value
end
return get
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var captures []ClosureCapture
	New(result).ForEachClosureCapture(func(capture ClosureCapture) bool {
		captures = append(captures, capture)
		return true
	})
	if len(captures) != 1 {
		t.Fatalf("captures = %#v, want one", captures)
	}
	capture := captures[0]
	if capture.SchemaVersion != readapi.ClosureCaptureSchemaVersion {
		t.Fatalf("schema version = %d, want %d", capture.SchemaVersion, readapi.ClosureCaptureSchemaVersion)
	}
	if capture.Name != "box" || capture.Policy != "full" {
		t.Fatalf("capture = %#v, want full box capture", capture)
	}
	if !capture.HasType || !capture.HasShape || !capture.NilabilityKnown || capture.Nilable {
		t.Fatalf("capture facts = type:%v shape:%v nil:%v/%v, want type+shape+known non-nil",
			capture.HasType, capture.HasShape, capture.Nilable, capture.NilabilityKnown)
	}
}
