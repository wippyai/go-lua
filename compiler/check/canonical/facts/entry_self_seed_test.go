package facts

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildPreTransferStoresTableLiteralSelfEntrySeed(t *testing.T) {
	root := parseFactsTestChunk(t, `
local obj = {
	value = 0,
	get = function(self): number
		return self.value
	end,
}
`)
	m, rootGraph := buildPrototypeFactsForSource(t, root)
	objSym := symbolNamed(t, rootGraph, "obj")
	getRef := assertFieldFunc(t, m, objSym, "get")

	assertSelfSeedField(t, m.FunctionEntrySeeds(getRef), "value", typ.Integer)
}

func TestBuildPreTransferStoresFieldAssignmentSelfEntrySeed(t *testing.T) {
	root := parseFactsTestChunk(t, `
local function make()
	local obj = { x = 1 }
	local function init()
		obj.get_x = function(self): number
			return self.x
		end
	end
	init()
	return obj
end
`)
	m, _ := buildPrototypeFactsForSource(t, root)
	var getRef ref.FuncRef
	for _, seed := range m.entrySelfSeeds {
		if seed.FuncRef != (ref.FuncRef{}) {
			getRef = seed.FuncRef
			break
		}
	}
	if getRef == (ref.FuncRef{}) {
		t.Fatalf("missing self seed rows")
	}

	assertSelfSeedField(t, m.FunctionEntrySeeds(getRef), "x", typ.Integer)
}

func assertSelfSeedField(t *testing.T, seeds []FunctionEntrySeed, field string, want typ.Type) {
	t.Helper()
	for _, seed := range seeds {
		if seed.Slot != 0 {
			continue
		}
		got, ok := querycore.Field(seed.Type, field)
		if ok && typ.TypeEquals(got, want) {
			return
		}
		t.Fatalf("seed field %q = %v/%v, want %v; seed=%v", field, got, ok, want, seed.Type)
	}
	t.Fatalf("missing slot 0 self seed in %+v", seeds)
}
