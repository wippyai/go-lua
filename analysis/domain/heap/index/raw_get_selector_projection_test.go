package index

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestRawGetDynamicKeyTraversalUsesSelectorProjection proves the reducer-side
// dynamic selector path delegates to keymatch's canonical relation
// projection.  Value.Top includes one Summary table atom per allocation plus
// opaque table alternatives; RawGet must observe Kinds(Table) exactly once.
func TestRawGetDynamicKeyTraversalUsesSelectorProjection(t *testing.T) {
	heapSchema, valueSchema, callSchema, packSchema, topology, access := rawGetDynamicSelectorFixture(t)
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawGetKey(220), rawGetKey(221), valueSchema)
	calls, callsOK := callowner.Declare(composition, rawGetKey(222), callSchema)
	heap, heapOK := heapowner.Declare(composition, rawGetKey(223), heapSchema)
	packs, packsOK := packowner.Declare(composition, rawGetKey(224), packSchema)
	rule, ruleOK := DeclareRawGet(composition, rawGetKey(225), rawGetKey(226), rawGetKey(227), topology, values, calls, heap, packs)
	if !valuesOK || !callsOK || !heapOK || !packsOK || !ruleOK || rule == nil || rule.selectors == nil {
		t.Fatal("raw-get dynamic declarations")
	}

	var got []heapdomain.KeySelector
	view := rawGetView{
		keyCount: 1,
		key:      rawSelected[valuedomain.Value]{value: valueSchema.Top(), present: true, found: true, valid: true},
	}
	if !rule.visitKeySelectors(access, view, func(selector heapdomain.KeySelector) bool {
		got = append(got, selector)
		return true
	}) {
		t.Fatal("raw-get dynamic selector traversal")
	}

	var want []heapdomain.KeySelector
	if !rule.selectors.Visit(valueSchema.Top(), func(selector heapdomain.KeySelector) bool {
		want = append(want, selector)
		return true
	}) || !sameRawGetSelectorSequence(got, want) {
		t.Fatal("raw-get dynamic traversal did not preserve canonical keymatch selectors")
	}
	if rawGetSelectorOccurrences(got, heapdomain.KeySelectorKinds, runtimekind.Bit(runtimekind.Table)) != 1 {
		t.Fatal("raw-get dynamic traversal repeated Kinds(Table)")
	}
}

func rawGetDynamicSelectorFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, *pack.Schema, *Topology, Access) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "raw_get_dynamic_selector.lua", Text: []byte(`
local first = {}
local second = {}
local key = "field"
return first[key], second[key]
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	heapSchema, heapOK := heapdomain.Seal(linked)
	valueSchema, valuesOK := valuedomain.Seal(linked, heapSchema)
	callSchema, callsOK := calldomain.New(linked)
	types, typesOK := typeauthority.Seal(linked)
	statics, _, staticErr := static.Seal(linked, types)
	packSchema, packsOK := pack.Seal(linked, statics)
	topology, topologyOK := Seal(heapSchema, valueSchema, callSchema)
	if !heapOK || !valuesOK || !callsOK || !typesOK || staticErr != nil || !packsOK || !topologyOK {
		t.Fatal("raw-get dynamic schemas")
	}
	for index := 0; index < heapSchema.IndexAccessCount(); index++ {
		candidate, ok := heapSchema.IndexAccessAt(index)
		if !ok {
			t.Fatal("raw-get dynamic index access")
		}
		access, found := topology.Access(candidate)
		if _, dynamic := access.DynamicKey(); found && access.Read() && dynamic {
			return heapSchema, valueSchema, callSchema, packSchema, topology, access
		}
	}
	t.Fatal("raw-get dynamic access")
	return heapdomain.Schema{}, nil, nil, nil, nil, Access{}
}

func sameRawGetSelectorSequence(left, right []heapdomain.KeySelector) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !sameRawGetSelector(left[index], right[index]) {
			return false
		}
	}
	return true
}

func sameRawGetSelector(left, right heapdomain.KeySelector) bool {
	if left.Kind() != right.Kind() || left.RuntimeKinds() != right.RuntimeKinds() || left.ExactCount() != right.ExactCount() || left.ReferenceCount() != right.ReferenceCount() {
		return false
	}
	for index := 0; index < left.ExactCount(); index++ {
		leftKey, leftOK := left.ExactAt(index)
		rightKey, rightOK := right.ExactAt(index)
		if !leftOK || !rightOK || leftKey != rightKey {
			return false
		}
	}
	for index := 0; index < left.ReferenceCount(); index++ {
		leftReference, leftOK := left.ReferenceAt(index)
		rightReference, rightOK := right.ReferenceAt(index)
		if !leftOK || !rightOK || leftReference != rightReference {
			return false
		}
	}
	return true
}

func rawGetSelectorOccurrences(selectors []heapdomain.KeySelector, kind heapdomain.KeySelectorKind, kinds runtimekind.Set) int {
	count := 0
	for _, selector := range selectors {
		if selector.Kind() == kind && selector.RuntimeKinds() == kinds {
			count++
		}
	}
	return count
}
