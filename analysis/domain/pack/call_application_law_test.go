package pack

import (
	"testing"

	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestSealRetainsZeroOffsetClosedValues is the first call-boundary law: a Pack
// schema must be sealable before a caller requests any nonzero tail offset.
// A closed Values expression still needs offset zero for its first formal.
//
// This is deliberately an end-to-end semantic law, not a test of Pack's file
// layout or implementation choices.
func TestSealRetainsZeroOffsetClosedValues(t *testing.T) {
	_, linked, statics := sealCallLaw(t, `
local function sink(_) end
sink({})
`)
	schema, ok := Seal(linked, statics)
	if !ok || schema == nil {
		t.Fatal("seal Pack schema for a closed call boundary")
	}
	if schema.RootCount() == 0 {
		t.Fatal("closed call boundary has no Pack root")
	}
}

func TestSourceAnchorOwnsCallAndValuesCausalOccurrences(t *testing.T) {
	_, linked, statics := sealCallLaw(t, `
local receiver = { method = function() end }
receiver:method()
`)
	schema, ok := Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}

	application := firstMethodApplication(t, linked)
	callRoot, ok := schema.CallRoot(application)
	if !ok {
		t.Fatal("Pack call root")
	}
	callSource, ok := schema.Source(callRoot)
	if !ok {
		t.Fatal("Pack call source")
	}
	foreignSchema, ok := Seal(linked, statics)
	if !ok {
		t.Fatal("foreign Pack schema")
	}
	foreignRoot, ok := foreignSchema.CallRoot(application)
	if !ok {
		t.Fatal("foreign Pack call root")
	}
	if _, ok := schema.Source(foreignRoot); ok {
		t.Fatal("foreign Pack source crossed the owner fence before anchor projection")
	}
	gotShard, gotTerm, anchorOK := callSource.Anchor()
	wantShard, wantTerm, wantOK := linked.Project().Applications().Call(application)
	if !anchorOK || !wantOK || gotShard != wantShard || gotTerm != wantTerm {
		t.Fatal("call source did not expose its Call occurrence rather than private actual Values")
	}

	p, ok := linked.Project().Mounts().Program(wantShard)
	if !ok || p == nil {
		t.Fatal("call Program")
	}
	values := p.Flow().Authored().Values()
	if values.Count() == 0 {
		t.Fatal("fixture Values")
	}
	valueTerm, ok := values.At(0)
	if !ok {
		t.Fatal("first Values term")
	}
	_, valueRoot, ok := schema.Values(wantShard, valueTerm)
	if !ok {
		t.Fatal("Pack Values root")
	}
	valueSource, ok := schema.Source(valueRoot)
	if !ok {
		t.Fatal("Pack Values source")
	}
	gotShard, gotTerm, anchorOK = valueSource.Anchor()
	if !anchorOK || gotShard != wantShard || gotTerm != valueTerm {
		t.Fatal("Values source did not expose its Values occurrence")
	}
	var zero Source
	if _, _, ok := zero.Anchor(); ok {
		t.Fatal("zero Pack source acquired an anchor")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _, _ = callSource.Anchor()
	}); allocations != 0 {
		t.Fatalf("Source.Anchor allocations = %g, want 0", allocations)
	}
}

// TestSealRejectsSameContentForeignStaticAuthority proves that Pack's cold
// replay identity does not authenticate a live Static owner. Independently
// sealed equal Links are portable-equal but remain distinct authority handles;
// only the exact Link used to seal Static may enter Pack.
func TestSealRejectsSameContentForeignStaticAuthority(t *testing.T) {
	const source = `
local function sink(_) end
sink({})
`
	_, localLink, localStatic := sealCallLaw(t, source)
	_, foreignLink, foreignStatic := sealCallLaw(t, source)
	if localLink == foreignLink {
		t.Fatal("independent Link seals reused the live handle")
	}
	if localLink.ContentID() != foreignLink.ContentID() {
		t.Fatal("fixture did not produce equal replay identity")
	}
	if localStatic.Link() != localLink || foreignStatic.Link() != foreignLink {
		t.Fatal("Static authority lost its exact live Link owner")
	}
	if _, ok := Seal(localLink, foreignStatic); ok {
		t.Fatal("same-content foreign Static authority crossed Pack seal fence")
	}
	if _, ok := Seal(foreignLink, localStatic); ok {
		t.Fatal("same-content foreign Static authority crossed reverse Pack seal fence")
	}
	if _, ok := Seal(localLink, localStatic); !ok {
		t.Fatal("exact local Static authority was rejected")
	}
	if _, ok := Seal(foreignLink, foreignStatic); !ok {
		t.Fatal("exact foreign-local Static authority was rejected")
	}
}

// TestSealPredeclaresIndexedWriteTailOffsets proves the Pack offset
// denominator is drawn from executable Program consumers as well as Target
// formals.  The third indexed write takes the third member of an open RHS
// tail, so offset two must be sealed before solving.  No later selection may
// manufacture offset three.
func TestSealPredeclaresIndexedWriteTailOffsets(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "pack_index_tail_offset_law.lua", Text: []byte(`
local function many(...) return ... end
local record = {}
record.first, record.second, record.third = many()
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_index_tail_offset_law", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}

	seenTailTwo := false
	flowView := p.Flow()
	geometry := flowView.AccessGeometry()
	if !geometry.Available() {
		t.Fatal("AccessGeometry unavailable")
	}
	writes := geometry.IndexAccesses().Writes()
	for index := 0; index < writes.Count(); index++ {
		write, ok := writes.At(index)
		if !ok {
			t.Fatal("IndexSet candidate")
		}
		_, _, values, sourceIndex, _, ok := writes.Get(write)
		if !ok {
			t.Fatal("IndexSet candidate operands")
		}
		position, ok := flowView.Authored().Values().Position(values, sourceIndex)
		if !ok {
			t.Fatal("IndexSet Values position")
		}
		seenTailTwo = seenTailTwo || (position.Tail != 0 && position.TailOffset == 2)
	}
	if !seenTailTwo {
		t.Fatal("fixture did not expose executable IndexSet RHS tail offset two")
	}

	schema, ok := Seal(linked, statics)
	if !ok || schema == nil {
		t.Fatal("Pack schema")
	}
	if _, ok := schema.TableIndex(2); !ok {
		t.Fatal("sealed indexed-write tail offset two")
	}
	if _, ok := schema.TableIndex(3); ok {
		t.Fatal("unsealed indexed-write tail offset three was accepted")
	}
}

// TestTargetFormalOffsetsAreZeroBased proves Target's fixed value-formal
// count contributes the last zero-based selector, not an exclusive count.
// The expected algebra identity is checked directly so a surplus offset is
// observable even if callers never request it.
func TestTargetFormalOffsetsAreZeroBased(t *testing.T) {
	const formalCount = 3
	p, err := lower.Lower(lower.Source{Name: "pack_target_formal_offset_law.lua", Text: []byte(`
local function sink(_) end
sink({})
`)})
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"fixed"}}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{binding},
		Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any, typ.Any}, Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_target_formal_offset_law", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}
	if _, ok := schema.TableIndex(formalCount - 1); !ok {
		t.Fatalf("last fixed formal offset %d was not sealed", formalCount-1)
	}
	if _, ok := schema.TableIndex(formalCount); ok {
		t.Fatalf("exclusive fixed formal offset %d was incorrectly sealed", formalCount)
	}
	if got := len(schema.state.owner.offsets); got != formalCount {
		t.Fatalf("sealed offset denominator = %d, want %d", got, formalCount)
	}
	expected, ok := newAlgebraWithOffsets(statics.Classes(), nil, []nat{natFromUint64(0), natFromUint64(1), natFromUint64(2)})
	if !ok || expected.id != schema.state.owner.id {
		t.Fatal("surplus formal offset changed the Pack algebra identity")
	}
}

// TestSealUsesCandidateWriteDenominatorWithoutAuthoredCellGaps proves that
// only the one executable candidate lens Write contributes a Pack offset.
// The later Cell Writes share its open Values tail but are not IndexSet
// candidates and must not widen the frozen denominator.
func TestSealUsesCandidateWriteDenominatorWithoutAuthoredCellGaps(t *testing.T) {
	_, linked, statics := sealCallLaw(t, `
local function many(...) return ... end
local record = {}
local first
local second
local third
record.first, first, second, third = many()
`)
	mounts := linked.Project().Mounts()
	shard, ok := mounts.At(0)
	if !ok {
		t.Fatal("Pack gap fixture mount")
	}
	p, ok := mounts.Program(shard)
	if !ok || p == nil {
		t.Fatal("Pack gap fixture Program")
	}
	flowView := p.Flow()
	geometry := flowView.AccessGeometry()
	if !geometry.Available() {
		t.Fatal("AccessGeometry unavailable")
	}
	candidates := geometry.IndexAccesses().Writes()
	if candidates.Count() != 1 {
		t.Fatalf("candidate IndexSet Writes = %d, want 1", candidates.Count())
	}
	candidate, ok := candidates.At(0)
	if !ok {
		t.Fatal("candidate IndexSet Write")
	}
	_, candidateKey, values, candidatePosition, lens, ok := candidates.Get(candidate)
	if !ok || candidatePosition != 0 || keyspace.TermFamily(lens) != keyspace.FamilyLensExact {
		t.Fatalf("candidate Write = key %v values %v position %d lens %v ok %v", candidateKey, values, candidatePosition, lens, ok)
	}
	assign, _, writeOK := flowView.Authored().Storage().Writes().Get(candidate)
	assigns := flowView.Authored().Storage().Assigns()
	assignOwner, assignValues, assignOK := assigns.Get(assign)
	if !writeOK || !assignOK || assignOwner == 0 || assignValues != values {
		t.Fatalf("candidate Assign = %v/%v values %v/%v", assign, writeOK, assignValues, assignOK)
	}
	writeCount, ok := assigns.WriteCount(assign)
	if !ok || writeCount != 4 {
		t.Fatalf("authored Write count = %d/%v, want 4", writeCount, ok)
	}
	writes := flowView.Authored().Storage().Writes()
	valuesView := flowView.Authored().Values()
	for position := 0; position < writeCount; position++ {
		write, ok := assigns.WriteAt(assign, position)
		parent, target, writeOK := writes.Get(write)
		if !ok || !writeOK || parent != assign {
			t.Fatalf("authored Write[%d] = %v/%v parent %v/%v", position, write, ok, parent, writeOK)
		}
		valuePosition, positionOK := valuesView.Position(values, position)
		if !positionOK || valuePosition.Tail == 0 || valuePosition.TailOffset != position {
			t.Fatalf("authored Values position %d = %#v/%v", position, valuePosition, positionOK)
		}
		if position == 0 {
			if write != candidate || keyspace.TermFamily(target) != keyspace.FamilyLensExact {
				t.Fatalf("candidate Write moved from position zero: %v target %v", write, target)
			}
			continue
		}
		if candidates.Contains(write) || keyspace.TermFamily(target) != keyspace.FamilyCell {
			t.Fatalf("noncandidate Cell Write[%d] entered IndexSet geometry: %v target %v", position, write, target)
		}
	}
	schema, ok := Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}
	if _, ok := schema.TableIndex(0); !ok {
		t.Fatal("candidate offset zero was not sealed")
	}
	for index := 1; index < writeCount; index++ {
		if _, ok := schema.TableIndex(int64(index)); ok {
			t.Fatalf("noncandidate authored Write widened Pack offset denominator at %d", index)
		}
	}
	if got := len(schema.state.owner.offsets); got != 1 {
		t.Fatalf("Pack offset denominator = %d, want only zero", got)
	}
}

// TestSealAcceptsValidZeroCandidateAccessGeometry proves a valid assembled
// Program can publish AccessGeometry with no candidate indexed rows, and Pack
// accepts that available zero-row projection while retaining mandatory offset 0.
func TestSealAcceptsValidZeroCandidateAccessGeometry(t *testing.T) {
	_, linked, statics := sealCallLaw(t, `
local value = 1
`)
	mounts := linked.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			t.Fatal("zero-candidate mount")
		}
		p, ok := mounts.Program(shard)
		if !ok || p == nil {
			t.Fatal("zero-candidate Program")
		}
		geometry := p.Flow().AccessGeometry()
		if !geometry.Available() {
			t.Fatal("valid zero-candidate AccessGeometry unavailable")
		}
		if geometry.IndexAccesses().Reads().Count() != 0 || geometry.IndexAccesses().Writes().Count() != 0 {
			t.Fatal("zero-candidate fixture unexpectedly retained indexed candidates")
		}
	}
	schema, ok := Seal(linked, statics)
	if !ok || schema == nil {
		t.Fatal("Pack rejected valid zero-candidate AccessGeometry")
	}
	if _, ok := schema.TableIndex(0); !ok {
		t.Fatal("zero-candidate Pack omitted mandatory offset zero")
	}
}

// TestSelectionOffsetsRejectsUnavailableAccessGeometry proves Pack keeps the
// Flow projection-availability fence. The remaining authored Values view is
// from a real lower->Program fixture; only the typed AccessGeometry input is
// the unavailable zero value. It must not be accepted as an empty candidate
// denominator.
func TestSelectionOffsetsRejectsUnavailableAccessGeometry(t *testing.T) {
	_, linked, _ := sealCallLaw(t, `
local value = 1
`)
	mounts := linked.Project().Mounts()
	shard, ok := mounts.At(0)
	if !ok {
		t.Fatal("unavailable-geometry fixture mount")
	}
	mounted, ok := mounts.Program(shard)
	if !ok || mounted == nil {
		t.Fatal("unavailable-geometry fixture Program")
	}
	flowView := mounted.Flow()
	if !flowView.ContentID().Available() || !flowView.AccessGeometry().Available() {
		t.Fatal("real fixture Flow or AccessGeometry unavailable")
	}
	if _, ok := selectionOffsetsForProgram(0, flow.AccessGeometry{}, flowView.Authored().Values()); ok {
		t.Fatal("selection-offset helper accepted unavailable AccessGeometry")
	}
}

// TestInputSelectorFreezesScalarOffsetBeforeCallApplication proves the input
// selector is a cold Pack template, not a per-derivation Target/offset
// lookup.  Link authenticates the direct call occurrence at Pack seal. The
// scalar index itself is already an owner-issued Pack offset.
func TestInputSelectorFreezesScalarOffsetBeforeCallApplication(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "pack_input_selector_law.lua", Text: []byte(`
local function many(...) return ... end
local object = {}
object:send(many())
`)})
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"selected"}}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings:   []target.BindingSpec{binding},
		ValuesVars: 1,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any, typ.Any}, Tail: target.ValuesVariable, Var: 0},
		Outcomes:   []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:    target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	selected, ok := contract.Lookup(binding)
	if !ok {
		t.Fatal("selected operation")
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_input_selector_law", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}

	selector, ok := schema.InputSelector(selected, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 2})
	if !ok || !selector.valid() || !selector.table.valid() || selector.table.value != 2 {
		t.Fatal("sealed formal-two scalar selector")
	}
	if _, ok := schema.InputSelector(selected, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 3}); ok {
		t.Fatal("out-of-range formal acquired a selector")
	}
	application := firstMethodApplication(t, linked)
	root, ok := schema.CallRoot(application)
	if !ok {
		t.Fatal("Pack call root")
	}
	foreignSchema, ok := Seal(linked, statics)
	if !ok {
		t.Fatal("foreign Pack schema")
	}
	foreignRoot, ok := foreignSchema.CallRoot(application)
	if !ok {
		t.Fatal("foreign Pack call root")
	}
	source, ok := schema.Source(root)
	if !ok {
		t.Fatal("Pack source")
	}
	fact, ok := lawSourceFact(schema, source)
	if !ok {
		t.Fatal("Pack source fact")
	}
	if _, ok := schema.ObserveInput(foreignRoot, fact, selector); ok {
		t.Fatal("selector crossed Pack schema fence")
	}
	if _, ok := schema.ObserveInput(root, fact, InputSelector{}); ok {
		t.Fatal("invalid selector observed Pack call root")
	}
	observation, ok := schema.ObserveInput(root, fact, selector)
	if !ok || observation.ScalarCount() != 1 {
		t.Fatal("prebuilt selector did not project exact scalar")
	}
	selectedScalar, ok := observation.ScalarAt(0)
	if !ok || selectedScalar.Kind() != ScalarHead {
		t.Fatal("formal-two projection did not retain the symbolic open-tail head")
	}
}

// TestCallTailSelectionRetainsSymbolicHead proves the precision needed
// when a method receiver is prepended to an open actuals Pack.  The second
// input is the head of the original tail, not an arbitrary value of the tail's
// runtime class.  Losing that symbolic head would make a later Escape
// projection unable to reach the exact source endpoint.
func TestCallTailSelectionRetainsSymbolicHead(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	receiver, ok := endpointScalar(fixture.firstEnd)
	if !ok {
		t.Fatal("receiver endpoint scalar")
	}
	tail, ok := freeTail(fixture.secondPort)
	if !ok {
		t.Fatal("actuals free tail")
	}
	zero, ok := zeroOffset(fixture.owner)
	if !ok {
		t.Fatal("zero tail offset")
	}
	rest, ok := tailRest(tail, zero)
	if !ok {
		t.Fatal("tail rest")
	}
	input, ok := openTerm(fixture.owner, []Scalar{receiver}, rest, nil)
	if !ok {
		t.Fatal("receiver-prepended input Pack")
	}
	first, ok := fixture.schema.TableIndex(0)
	if !ok {
		t.Fatal("sealed table index zero")
	}
	selected, ok := projectTermTableIndex(input, first)
	if !ok || selected.Kind() != ScalarEndpoint {
		t.Fatal("input formal zero must remain the receiver endpoint")
	}
	second, ok := fixture.schema.TableIndex(1)
	if !ok {
		t.Fatal("sealed table index one")
	}
	selected, ok = projectTermTableIndex(input, second)
	if !ok || selected.Kind() != ScalarHead {
		t.Fatal("input formal one must retain the exact head of the actuals tail")
	}
	third, ok := fixture.schema.TableIndex(2)
	if !ok {
		t.Fatal("sealed table index two")
	}
	advanced, ok := projectTermTableIndex(input, third)
	if !ok || advanced.Kind() != ScalarHead || equalScalar(selected, advanced) || compareOffset(selected.offset, advanced.offset) >= 0 {
		t.Fatal("successive open-tail selections must retain distinct advancing head offsets")
	}
}

// TestSealCarriesCallAndVarargTailProducersAndOffsets proves the occurrence
// denominator Pack freezes from Program and Target together.  Call and
// vararg tails are distinct producers, not fake Values sources; formal one
// and formal two select different offsets from the same open tail and produce
// a concrete non-bottom Pack fact before a later producer Rule fills it.
func TestSealCarriesCallAndVarargTailProducersAndOffsets(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "pack_tail_producer_law.lua", Text: []byte(`
local function many(...) return ... end
local function forward(...) local object = {}; object:send(...) end
local object = {}
object:send(many(1))
forward(2, 3)
`)})
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"selected"}}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings:   []target.BindingSpec{binding},
		ValuesVars: 1,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any, typ.Any}, Tail: target.ValuesVariable, Var: 0},
		Outcomes:   []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:    target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	selected, ok := contract.Lookup(binding)
	if !ok {
		t.Fatal("selected operation")
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_tail_producer_law", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}

	var calls, varargs int
	for index := 0; index < schema.RootCount(); index++ {
		root, ok := schema.RootAt(index)
		if !ok {
			t.Fatal("Pack root")
		}
		producer, exists := schema.TailProducer(root)
		if !exists {
			continue
		}
		if _, ok := schema.Source(root); ok {
			t.Fatal("Call/Vararg tail acquired an authored zero-read source")
		}
		if _, ok := producer.ContentID(); !ok {
			t.Fatal("tail producer content identity")
		}
		switch producer.Kind() {
		case TailProducerCall:
			calls++
		case TailProducerVararg:
			varargs++
		default:
			t.Fatal("unclassified Pack tail producer")
		}
	}
	if calls == 0 || varargs == 0 {
		t.Fatalf("tail producer kinds: calls=%d varargs=%d", calls, varargs)
	}

	application := firstMethodApplication(t, linked)
	root, ok := schema.CallRoot(application)
	if !ok {
		t.Fatal("Pack call root")
	}
	source, ok := schema.Source(root)
	if !ok {
		t.Fatal("Pack call source")
	}
	fact, ok := lawSourceFact(schema, source)
	if !ok || fact.IsBottom() {
		t.Fatal("authored call source became Bottom")
	}
	firstSelector, ok := schema.InputSelector(selected, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1})
	if !ok {
		t.Fatal("first tail formal selector")
	}
	secondSelector, ok := schema.InputSelector(selected, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 2})
	if !ok {
		t.Fatal("second tail formal selector")
	}
	firstInput, ok := schema.ObserveInput(root, fact, firstSelector)
	if !ok || firstInput.ScalarCount() != 1 {
		t.Fatal("first tail formal observation")
	}
	secondInput, ok := schema.ObserveInput(root, fact, secondSelector)
	if !ok || secondInput.ScalarCount() != 1 {
		t.Fatal("second tail formal observation")
	}
	firstScalar, _ := firstInput.ScalarAt(0)
	secondScalar, _ := secondInput.ScalarAt(0)
	if firstScalar.Kind() != ScalarHead || secondScalar.Kind() != ScalarHead || equalScalar(firstScalar, secondScalar) || compareOffset(firstScalar.offset, secondScalar.offset) >= 0 {
		t.Fatal("call tail offsets collapsed or failed to advance")
	}
}

func lawSourceFact(schema *Schema, source Source) (Value, bool) {
	root, ok := source.Root()
	if !ok || source.Count() != 1 {
		return Value{}, false
	}
	item, ok := source.At(0)
	if !ok {
		return Value{}, false
	}
	builder, ok := schema.Builder(root)
	if !ok {
		return Value{}, false
	}
	fixed := make([]Scalar, item.FixedCount())
	for index := range fixed {
		endpoint, ok := item.FixedAt(index)
		if !ok {
			return Value{}, false
		}
		fixed[index], ok = builder.Endpoint(endpoint)
		if !ok {
			return Value{}, false
		}
	}
	tail, offset, open := item.Tail()
	var term Term
	if open {
		free, ok := builder.FreeTail(tail)
		if !ok {
			return Value{}, false
		}
		rest, ok := builder.Tail(free, offset)
		if !ok {
			return Value{}, false
		}
		term, ok = builder.Open(fixed, rest, nil)
	} else {
		term, ok = builder.Closed(fixed...)
	}
	if !ok {
		return Value{}, false
	}
	port, ok := item.Port()
	if !ok {
		return Value{}, false
	}
	equation, ok := builder.Pack(port, term)
	if !ok {
		return Value{}, false
	}
	caseValue, ok := builder.Case(equation)
	if !ok {
		return Value{}, false
	}
	fact, ok := builder.Value(caseValue)
	return fact, ok && schema.Admit(root, fact)
}

func firstMethodApplication(t testing.TB, linked *link.Link) linkproject.Application {
	t.Helper()
	applications := linked.Project().Applications()
	calls := applications.Calls()
	for index := 0; index < calls.Count(); index++ {
		application, ok := calls.At(index)
		if !ok {
			t.Fatal("CallApplicationAt")
		}
		shard, callTerm, callOK := applications.Call(application)
		p, programOK := linked.Project().Mounts().Program(shard)
		if !callOK || !programOK || p == nil {
			continue
		}
		_, _, receiver, _, callOK := p.Flow().Authored().Calls().Get(callTerm)
		if callOK && receiver != 0 {
			return application
		}
	}
	t.Fatal("method call application")
	return linkproject.Application{}
}

func sealCallLaw(t testing.TB, text string) (*target.Contract, *link.Link, *staticdomain.Authority) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "pack_call_input_law.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_call_input_law", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("seal type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	return contract, linked, statics
}
