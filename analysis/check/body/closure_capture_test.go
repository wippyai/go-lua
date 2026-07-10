package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func TestClosureCaptureFactsExportSingleAssignmentRecordFacts(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Buf = { n: number }
local buf: Buf = { n = 0 }
local function push(v: number)
    buf.n = buf.n + v
end
return push
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := CheckBoundChunk(stmts, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundChunk: %v", err)
	}
	fact := requireClosureCaptureFact(t, result, "buf")
	if fact.SchemaVersion != ClosureCaptureFactSchemaVersion {
		t.Fatalf("schema version = %d, want %d", fact.SchemaVersion, ClosureCaptureFactSchemaVersion)
	}
	if fact.Policy != ClosureCapturePolicyFull {
		t.Fatalf("policy = %s, want full", fact.Policy)
	}
	if !fact.HasType {
		t.Fatalf("capture type = %v/%v, want table-like Buf", fact.Type, fact.HasType)
	}
	if !fact.HasShape || fact.Shape == nil {
		t.Fatalf("capture shape = %v/%v, want exported record shape", fact.Shape, fact.HasShape)
	}
	if _, ok := unwrap.Annotated(fact.Shape).(*typ.Record); !ok {
		t.Fatalf("capture shape = %T %v, want record", fact.Shape, fact.Shape)
	}
	if !fact.NilabilityKnown || fact.Nilable {
		t.Fatalf("nilability = %v/%v, want known non-nil", fact.Nilable, fact.NilabilityKnown)
	}
	if !fact.HasPlacement || fact.Placement != placement.Stack {
		t.Fatalf("placement = %s/%v, want stack", fact.Placement, fact.HasPlacement)
	}
}

func TestClosureCaptureFactsDegradeWrittenCaptureToInvariantType(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local x: string? = "ready"
local get = function(): string?
    return x
end
x = nil
return get
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := CheckBoundChunk(stmts, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundChunk: %v", err)
	}
	fact := requireClosureCaptureFact(t, result, "x")
	if fact.Policy != ClosureCapturePolicyWriteInvariant {
		t.Fatalf("policy = %s, want write-invariant", fact.Policy)
	}
	if !fact.HasType || !typevalue.TypeIncludesNil(fact.Type) {
		t.Fatalf("capture type = %v/%v, want nilable invariant type", fact.Type, fact.HasType)
	}
	if !fact.NilabilityKnown || !fact.Nilable {
		t.Fatalf("nilability = %v/%v, want known nilable", fact.Nilable, fact.NilabilityKnown)
	}
	if fact.HasPlacement {
		t.Fatalf("placement = %s/%v, want no placement for write-invariant scalar", fact.Placement, fact.HasPlacement)
	}
}

func requireClosureCaptureFact(t *testing.T, result *Result, name string) ClosureCaptureFact {
	t.Helper()
	var found ClosureCaptureFact
	var ok bool
	result.ForEachClosureCaptureFact(func(fact ClosureCaptureFact) bool {
		if fact.Name == name {
			found = fact
			ok = true
			return false
		}
		return true
	})
	if !ok {
		t.Fatalf("closure capture %q not found", name)
	}
	return found
}
