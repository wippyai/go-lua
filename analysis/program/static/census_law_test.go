package static

import (
	"testing"

	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"

	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func censusLawInputs(t *testing.T) map[string]Input {
	t.Helper()
	return map[string]Input{
		"fixture":     staticFixture(t),
		"denominator": staticTypeDenominatorInput(t),
	}
}

// TestCensusColumnIsTheAuthoredRelationLength proves the sealed column is the
// authored input length for every dense family and the owning component's
// external denominator for every sparse one. This is the assignment Build
// performs once; nothing downstream may recompute it.
func TestCensusColumnIsTheAuthoredRelationLength(t *testing.T) {
	for name, input := range censusLawInputs(t) {
		component := staticContentComponent(t, input)
		for _, family := range staticFamilyInventory[:staticDenseFamilyCount] {
			length, ok := staticFamilyInputCount(input, family)
			if !ok {
				t.Fatalf("%s: dense family %v has no authored relation", name, family)
			}
			if uint64(component.census[family]) != uint64(length) {
				t.Fatalf("%s: census[%v] = %d, want authored relation length %d",
					name, family, component.census[family], length)
			}
		}
		for _, family := range staticFamilyInventory[staticDenseFamilyCount:] {
			length, ok := staticFamilyInputCount(input, family)
			if !ok {
				t.Fatalf("%s: sparse family %v has no authored relation", name, family)
			}
			if component.census[family] != input.Counts[family] {
				t.Fatalf("%s: sparse census[%v] = %d, want external denominator %d",
					name, family, component.census[family], input.Counts[family])
			}
			if uint64(length) > uint64(component.census[family]) {
				t.Fatalf("%s: sparse %v has %d rows beyond census %d", name, family, length, component.census[family])
			}
		}
	}
}

// TestStaticTypeForestReadsTheCensusColumn proves the forest enumeration is the
// census column restricted to the inventory's forest window, with no second
// prefix table to keep in step.
func TestStaticTypeForestReadsTheCensusColumn(t *testing.T) {
	for name, input := range censusLawInputs(t) {
		component := staticContentComponent(t, input)
		want := 0
		for _, family := range staticFamilyInventory[:staticTypeFamilyCount] {
			want += int(component.census[family])
		}
		if got := component.StaticTypeTermCount(); got != want {
			t.Fatalf("%s: StaticTypeTermCount = %d, want census forest sum %d", name, got, want)
		}
		index := 0
		for _, family := range staticFamilyInventory[:staticTypeFamilyCount] {
			for ordinal := uint32(1); ordinal <= component.census[family]; ordinal++ {
				term, ok := component.StaticTypeTermAt(index)
				if !ok || term != keyspace.MakeTerm(family, ordinal) {
					t.Fatalf("%s: StaticTypeTermAt(%d) = (%d, %v), want %v#%d",
						name, index, term, ok, family, ordinal)
				}
				if !component.StaticTypeTerm(term) {
					t.Fatalf("%s: enumerated term %v#%d is not a forest member", name, family, ordinal)
				}
				index++
			}
		}
		if _, ok := component.StaticTypeTermAt(want); ok {
			t.Fatalf("%s: StaticTypeTermAt(%d) admitted a term past the sealed forest", name, want)
		}
		for _, family := range staticFamilyInventory[:staticTypeFamilyCount] {
			past := keyspace.MakeTerm(family, component.census[family]+1)
			if component.StaticTypeTerm(past) {
				t.Fatalf("%s: forest admitted %v ordinal %d past census %d",
					name, family, component.census[family]+1, component.census[family])
			}
		}
	}
}

func TestStaticCountRowsEnumeratesOwnGeneratedRelations(t *testing.T) {
	component := staticContentComponent(t, staticFixture(t))
	rows, err := CountRows(component.View())
	if err != nil {
		t.Fatalf("CountRows() error = %v", err)
	}
	if rows.Count() != 10 {
		t.Fatalf("Static CountRows count = %d, want 10 generated relations", rows.Count())
	}
	ids := denominator.GeneratedProgramStaticIDs()
	view := component.View()
	declarations, signatures, contracts, operators, operands := view.Declarations(), view.Signatures(), view.Contracts(), view.Operators(), view.Operands()
	primary := declarations.Aliases().Count() + declarations.Interfaces().Count() + declarations.TypeParams().Count() +
		view.Types().Primitives().Count() + view.Types().Literals().Count() + view.Types().Optionals().Count() +
		view.Types().Unions().Count() + view.Types().Intersections().Count() + view.Types().Generics().Count() +
		view.Types().Arrays().Count() + view.Types().Maps().Count() + view.Types().Records().Count() +
		view.References().Count() + signatures.TypeFunctions().Count() + signatures.Assertions().Count() +
		operators.TypeOfs().Count() + operators.KeyOfs().Count() + operators.IndexAccesses().Count() + operators.Conditionals().Count()
	callArguments := 0
	for index := 0; index < contracts.Calls().Count(); index++ {
		term, ok := contracts.Calls().At(index)
		if !ok {
			t.Fatalf("staticcontracts.Calls.At(%d) failed", index)
		}
		count, ok := contracts.Calls().TypeArgumentCount(term)
		if !ok {
			t.Fatalf("TypeArgumentCount(%v) failed", term)
		}
		callArguments += count
	}
	want := []struct {
		name  string
		id    schema.EntryID
		value uint64
	}{
		{name: "program static", id: ids.ProgramStatic, value: uint64(primary)},
		{name: "function contract", id: ids.ProgramStaticFunctionContract, value: uint64(contracts.Functions().Count())},
		{name: "call arguments", id: ids.ProgramStaticCallTypeArguments, value: uint64(callArguments)},
		{name: "declared type", id: ids.ProgramStaticCellDeclaredType, value: uint64(declarations.DeclaredTypes().Count())},
		{name: "claim target", id: ids.ProgramStaticClaimTarget, value: uint64(operands.Claims().Count())},
		{name: "type value", id: ids.ProgramStaticTypeValueTarget, value: uint64(operands.TypeValues().Count())},
		{name: "typeof", id: ids.ProgramStaticTypeof, value: uint64(operators.TypeOfs().Count())},
		{name: "annotation", id: ids.ProgramStaticAnnotation, value: uint64(operands.Annotations().Count())},
		{name: "publication", id: ids.ProgramStaticPublication, value: uint64(view.Publications().Count())},
		{name: "type reference", id: ids.ProgramStaticTypeRef, value: uint64(view.References().Count())},
	}
	for _, id := range want {
		if !id.id.Available() {
			t.Fatalf("%s identity unavailable", id.name)
		}
		if got, ok := rows.Value(id.id); !ok || got != id.value {
			t.Fatalf("%s count = %d/%v, want %d", id.name, got, ok, id.value)
		}
	}
	if _, err := CountRows(View{}); err == nil {
		t.Fatal("CountRows accepted an unavailable View")
	}
}

// The call type-argument row is the one denominator that is neither a census
// entry nor a store length: it is the sealed width of the call segment inside
// the contracts term pool, which the function type-parameter and return
// segments precede. This proves the sealed width is that segment and not the
// whole pool.
func TestStaticCallTypeArgumentRowIsTheSealedColumnWidth(t *testing.T) {
	component := staticContentComponent(t, contractsFixture(t))
	contracts := component.View().Contracts()
	walked := 0
	for index := 0; index < contracts.Calls().Count(); index++ {
		term, ok := contracts.Calls().At(index)
		if !ok {
			t.Fatalf("staticcontracts.Calls.At(%d) failed", index)
		}
		count, ok := contracts.Calls().TypeArgumentCount(term)
		if !ok {
			t.Fatalf("TypeArgumentCount(%v) failed", term)
		}
		walked += count
	}
	if walked == 0 {
		t.Fatal("fixture authors no call type arguments, so the sealed width proves nothing")
	}
	if uint64(uint32(component.contracts.CallTypeArgumentWidth())) != uint64(walked) {
		t.Fatalf("sealed call type-argument width = %d, want walked total %d",
			uint32(component.contracts.CallTypeArgumentWidth()), walked)
	}
	// The call segment shares its column with the function contracts that
	// precede it. Unless the fixture authors function-side terms too, an
	// offset bug would be invisible here.
	functionTerms := 0
	functions := component.View().Contracts().Functions()
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			t.Fatalf("Functions().At(%d) failed", index)
		}
		params, paramsOK := functions.TypeParamCount(function)
		returns, returnsOK := functions.ReturnCount(function)
		if !paramsOK || !returnsOK {
			t.Fatalf("function contract %v column counts failed", function)
		}
		functionTerms += params + returns
	}
	if functionTerms == 0 {
		t.Fatal("fixture shares no term column with function contracts, so the segment offset proves nothing")
	}
	rows, err := CountRows(component.View())
	if err != nil {
		t.Fatalf("CountRows() error = %v", err)
	}
	got, ok := rows.Value(denominator.GeneratedProgramStaticIDs().ProgramStaticCallTypeArguments)
	if !ok || got != uint64(walked) {
		t.Fatalf("call type-argument row = %d/%v, want %d", got, ok, walked)
	}
}

// This law is deliberately about semantic coverage, never Go file layout.
// The single inventory is partitioned into dense authored columns and sparse
// cross-owner anchors; Build seals both through the one Component census.
func TestStaticInputRelationDenominatorIsCompleteAndUnique(t *testing.T) {
	if len(staticFamilyInventory) != 27 || staticDenseFamilyCount != 26 || staticTypeFamilyCount != 19 || staticNodeFamilyOffset != 3 {
		t.Fatalf("static inventory segments = %d total / %d dense / %d forest / %d node offset, want 27 / 26 / 19 / 3",
			len(staticFamilyInventory), staticDenseFamilyCount, staticTypeFamilyCount, staticNodeFamilyOffset)
	}
	var empty Input
	seen := map[keyspace.Family]bool{}
	for _, family := range staticFamilyInventory[:staticDenseFamilyCount] {
		if seen[family] {
			t.Fatalf("static relation denominator duplicates %v", family)
		}
		seen[family] = true
		empty.Counts[family] = 1
		if _, ok := staticCensus(empty); ok {
			t.Fatalf("staticCensus accepted a nonzero dense %v without its typed input row", family)
		}
		empty.Counts[family] = 0
	}
	for _, family := range staticFamilyInventory[staticDenseFamilyCount:] {
		if seen[family] {
			t.Fatalf("static relation denominator overlaps dense family %v", family)
		}
		seen[family] = true
		// A sparse relation may be absent under a nonzero external census.
		empty.Counts[family] = 1
		census, ok := staticCensus(empty)
		if !ok {
			t.Fatalf("staticCensus rejected an empty sparse %v relation under external count one", family)
		}
		if census[family] != 1 {
			t.Fatalf("sparse %v census = %d, want the owning component's external count 1", family, census[family])
		}
		empty.Operands.Claim = []staticoperands.ClaimTarget{
			{Claim: keyspace.MakeTerm(family, 1)},
			{Claim: keyspace.MakeTerm(family, 2)},
		}
		if _, ok := staticCensus(empty); ok {
			t.Fatalf("staticCensus accepted sparse %v rows beyond external count", family)
		}
		empty.Operands.Claim = nil
		empty.Counts[family] = 0
	}
	if len(seen) != 27 {
		t.Fatalf("Static relation inventory unique count = %d, want 27", len(seen))
	}
}

// TestStaticNodeWindowMatchesRoleVocabulary proves the inventory's node window
// and role.NodeFamily are the same closed vocabulary, so Static keeps one
// authored static-type node authority rather than two that can drift.
func TestStaticNodeWindowMatchesRoleVocabulary(t *testing.T) {
	window := map[keyspace.Family]bool{}
	for _, family := range staticFamilyInventory[staticNodeFamilyOffset:staticTypeFamilyCount] {
		if !staticrole.NodeFamily(family) {
			t.Fatalf("inventory node window admits %v, which role.NodeFamily rejects", family)
		}
		window[family] = true
	}
	for family := keyspace.Family(0); family < keyspace.FamilyCount; family++ {
		if staticrole.NodeFamily(family) && !window[family] {
			t.Fatalf("role.NodeFamily admits %v, which the inventory node window omits", family)
		}
	}
	if len(window) != staticTypeFamilyCount-staticNodeFamilyOffset {
		t.Fatalf("node window size = %d, want %d", len(window), staticTypeFamilyCount-staticNodeFamilyOffset)
	}
	for _, family := range staticFamilyInventory[:staticNodeFamilyOffset] {
		if staticrole.NodeFamily(family) {
			t.Fatalf("declaration-owned type root %v must not be a static type occurrence", family)
		}
		if !staticrole.TypeReferenceTargetFamily(family) {
			t.Fatalf("type root %v is not a declaration target family", family)
		}
	}
}

func TestStaticTypeEnumerationIsCompleteAndOrdered(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	wantFamilies := [...]keyspace.Family{
		keyspace.FamilyTypeAlias,
		keyspace.FamilyTypeInterface,
		keyspace.FamilyTypeParam,
		keyspace.FamilyTypePrimitive,
		keyspace.FamilyTypeLiteral,
		keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion,
		keyspace.FamilyTypeIntersection,
		keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric,
		keyspace.FamilyTypeArray,
		keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord,
		keyspace.FamilyTypeFunction,
		keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf,
		keyspace.FamilyTypeKeyOf,
		keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional,
	}
	want := make([]keyspace.Term, 0, len(wantFamilies)+19)
	for _, family := range wantFamilies {
		count := 1
		if family == keyspace.FamilyTypePrimitive {
			count = 20
		}
		for ordinal := 1; ordinal <= count; ordinal++ {
			want = append(want, keyspace.MakeTerm(family, uint32(ordinal)))
		}
	}
	if got := component.StaticTypeTermCount(); got != len(want) {
		t.Fatalf("StaticTypeTermCount = %d, want %d", got, len(want))
	}
	seen := make(map[keyspace.Family]int, len(wantFamilies))
	for index, expected := range want {
		term, ok := component.StaticTypeTermAt(index)
		if !ok || term != expected {
			t.Fatalf("StaticTypeTermAt(%d) = %v/%v, want %v", index, term, ok, expected)
		}
		if !component.StaticTypeTerm(term) {
			t.Fatalf("enumerated term %v is not a static type", term)
		}
		seen[keyspace.TermFamily(term)]++
	}
	for _, family := range wantFamilies {
		if seen[family] == 0 {
			t.Fatalf("static type family %d has no enumerated row", family)
		}
	}
	if _, ok := component.StaticTypeTermAt(-1); ok {
		t.Fatal("StaticTypeTermAt accepted a negative index")
	}
	if _, ok := component.StaticTypeTermAt(len(want)); ok {
		t.Fatal("StaticTypeTermAt accepted an out-of-range index")
	}
	if component.StaticTypeTerm(keyspace.MakeTerm(keyspace.FamilyRead, 1)) {
		t.Fatal("non-static Flow term entered the static authority")
	}

}

func TestStaticTypeQueriesDoNotAllocate(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if allocations := testing.AllocsPerRun(1000, func() {
		component.StaticTypeTermCount()
		component.StaticTypeTermAt(0)
		component.StaticTypeTerm(term)
	}); allocations != 0 {
		t.Fatalf("static type queries allocated %.2f times", allocations)
	}
}

func TestStaticTypeIndexIsExcludedFromAuthoredContentID(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	want := component.ContentID()
	component.census[keyspace.FamilyTypeAlias]++
	if got := contentID(component); got != want {
		t.Fatalf("derived static type prefix changed authored ContentID: %x != %x", got, want)
	}
}

func TestStaticTypesViewUsesPublishedForestOrder(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	types := component.View().StaticTypes()

	wantFamilies := [...]keyspace.Family{
		keyspace.FamilyTypeAlias,
		keyspace.FamilyTypeInterface,
		keyspace.FamilyTypeParam,
		keyspace.FamilyTypePrimitive,
		keyspace.FamilyTypeLiteral,
		keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion,
		keyspace.FamilyTypeIntersection,
		keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric,
		keyspace.FamilyTypeArray,
		keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord,
		keyspace.FamilyTypeFunction,
		keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf,
		keyspace.FamilyTypeKeyOf,
		keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional,
	}
	want := make([]keyspace.Term, 0, component.StaticTypeTermCount())
	for _, family := range wantFamilies {
		count := 1
		if family == keyspace.FamilyTypePrimitive {
			count = 20
		}
		for ordinal := 1; ordinal <= count; ordinal++ {
			want = append(want, keyspace.MakeTerm(family, uint32(ordinal)))
		}
	}
	if got := types.Count(); got != len(want) {
		t.Fatalf("StaticTypes.Count() = %d, want %d", got, len(want))
	}
	for index, expected := range want {
		ref, ok := types.At(index)
		if !ok || ref.Term() != expected {
			t.Fatalf("StaticTypes.At(%d) = %v/%v, want %v", index, ref.Term(), ok, expected)
		}
		bound, ok := types.Ref(expected)
		if !ok || bound.Term() != expected {
			t.Fatalf("StaticTypes.Ref(%v) = %v/%v", expected, bound.Term(), ok)
		}
	}
}

func TestStaticTypesViewRejectsNilAndMalformedTerms(t *testing.T) {
	var nilComponent *Component
	zero := nilComponent.View().StaticTypes()
	if zero.Count() != 0 {
		t.Fatal("nil Component StaticTypes view exposed rows")
	}
	if _, ok := zero.At(0); ok {
		t.Fatal("nil Component StaticTypes.At succeeded")
	}
	if _, ok := zero.Ref(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)); ok {
		t.Fatal("nil Component StaticTypes.Ref succeeded")
	}

	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	types := component.View().StaticTypes()
	bad := []keyspace.Term{
		0,
		keyspace.MakeTerm(keyspace.FamilyRead, 1),
		keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 0),
		keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 21),
	}
	for _, term := range bad {
		if ref, ok := types.Ref(term); ok || ref.Term() != 0 {
			t.Fatalf("StaticTypes.Ref(%v) = %v/%v, want zero/false", term, ref.Term(), ok)
		}
	}
}

func TestStaticTypesRawTermsRebindLocally(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	foreign := staticContentComponent(t, Input{
		Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyTypePrimitive: 1},
		Types:  statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}}},
	})

	foreignRef, ok := foreign.View().StaticTypes().At(0)
	if !ok {
		t.Fatal("foreign StaticTypes.At(0) failed")
	}
	raw := foreignRef.Term()
	if raw != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) {
		t.Fatalf("foreign StaticTypeRef.Term() = %v, want primitive/1", raw)
	}

	bound, ok := component.View().StaticTypes().Ref(raw)
	if !ok || bound.Term() != raw {
		t.Fatalf("raw term failed to rebind locally: %v/%v", bound.Term(), ok)
	}
	if foreignRef.component != foreign {
		t.Fatal("foreign ref lost its owner component")
	}
	if bound.component != component {
		t.Fatal("rebound ref did not carry the receiving owner component")
	}
	if bound.component == foreignRef.component {
		t.Fatal("rebound ref retained the foreign owner component")
	}
}

func TestStaticTypesConstructionViewCannotLeakReferences(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	construction := finalizer.View().StaticTypes()
	if construction.Count() != 0 {
		t.Fatal("claimed construction View exposed post-commit StaticTypes")
	}
	if _, ok := construction.At(0); ok {
		t.Fatal("claimed construction View minted a StaticTypeRef")
	}

	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if construction.Count() != 0 {
		t.Fatal("expired construction StaticTypes view regained rows")
	}
	if _, ok := construction.Ref(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)); ok {
		t.Fatal("expired construction StaticTypes view minted a ref")
	}
	ref, ok := component.View().StaticTypes().Ref(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))
	if !ok || ref.Term() == 0 {
		t.Fatal("published Component StaticTypes failed to mint a ref")
	}
}

func TestStaticTypesHotQueriesDoNotAllocate(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	types := component.View().StaticTypes()
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	ref, ok := types.Ref(term)
	if !ok {
		t.Fatal("StaticTypes.Ref failed for allocation fixture")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_ = types.Count()
		_, _ = types.At(0)
		_, _ = types.Ref(term)
		_ = ref.Term()
	}); allocations != 0 {
		t.Fatalf("StaticTypes hot queries allocated %.2f times", allocations)
	}
}
