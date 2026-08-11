package static

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func compositeLocalContainmentInput(t *testing.T) Input {
	t.Helper()
	input := declarationFixture(t)
	coordinate := input.Declarations.Alias[0].NameCoordinate
	term := func(family keyspace.Family, ordinal uint32) keyspace.Term { return keyspace.MakeTerm(family, ordinal) }
	primitive := func(ordinal uint32) keyspace.Term { return term(keyspace.FamilyTypePrimitive, ordinal) }
	input.Counts[keyspace.FamilyCell] = 1
	input.Counts[keyspace.FamilyRead] = 1
	input.Counts[keyspace.FamilyAssign] = 1
	input.Counts[keyspace.FamilyValueClaim] = 1
	input.Counts[keyspace.FamilyTypeValue] = 1
	input.Counts[keyspace.FamilyFunction] = 1
	input.Counts[keyspace.FamilyCall] = 1
	input.Counts[keyspace.FamilyTypePublication] = 1
	input.Counts[keyspace.FamilyTypePrimitive] = 27
	input.Counts[keyspace.FamilyTypeOptional] = 1
	input.Counts[keyspace.FamilyTypeUnion] = 1
	input.Counts[keyspace.FamilyTypeIntersection] = 1
	input.Counts[keyspace.FamilyTypeRef] = 3
	input.Counts[keyspace.FamilyTypeGeneric] = 1
	input.Counts[keyspace.FamilyTypeArray] = 1
	input.Counts[keyspace.FamilyTypeMap] = 1
	input.Counts[keyspace.FamilyTypeRecord] = 1
	input.Counts[keyspace.FamilyTypeField] = 2
	input.Counts[keyspace.FamilyTypeAsserts] = 1
	input.Counts[keyspace.FamilyTypeOf] = 1
	input.Counts[keyspace.FamilyTypeKeyOf] = 1
	input.Counts[keyspace.FamilyTypeIndexAccess] = 1
	input.Counts[keyspace.FamilyTypeConditional] = 1
	input.Types.Primitive = make([]Primitive, 27)
	for index := range input.Types.Primitive {
		input.Types.Primitive[index] = Primitive{Kind: PrimitiveAny}
	}
	input.Types.Optional = []Optional{{Inner: primitive(8)}}
	input.Types.Union = []Union{{Members: []keyspace.Term{primitive(9), primitive(10)}}}
	input.Types.Intersection = []Intersection{{Members: []keyspace.Term{primitive(11), primitive(12)}}}
	input.Types.Generic = []Generic{{Base: term(keyspace.FamilyTypeRef, 3), Args: []keyspace.Term{primitive(13)}}}
	input.Types.Array = []Array{{Element: primitive(14)}}
	input.Types.Map = []Map{{Key: primitive(15), Value: primitive(16)}}
	input.Types.Field = []Field{{Key: 4, Type: primitive(3), Optional: true}, {Key: 5, Type: primitive(17)}}
	input.Types.Record = []Record{{Fields: []keyspace.Term{term(keyspace.FamilyTypeField, 2)}}}
	input.References.TypeRef = append(input.References.TypeRef,
		TypeRef{Resolution: TypeRefDeclaration, Target: term(keyspace.FamilyTypeAlias, 1), Source: []keyspace.Key{6}},
		TypeRef{Resolution: TypeRefDeclaration, Target: term(keyspace.FamilyTypeInterface, 1), Source: []keyspace.Key{7}},
	)
	input.Signatures.TypeFunction[0] = TypeFunction{
		Scope:      term(keyspace.FamilyTypeInterface, 1),
		Parameters: []Parameter{{Name: 20, NameCoordinate: coordinate, Type: primitive(4)}},
		Variadic:   primitive(5), VariadicCoordinate: coordinate,
		ReturnsKnown: true, Returns: []keyspace.Term{term(keyspace.FamilyTypeAsserts, 1), primitive(6)},
	}
	input.Counts[keyspace.FamilyTypeAsserts] = 1
	input.Signatures.TypeAsserts = []TypeAsserts{{Name: 20, ParamCoordinate: coordinate, Bound: true, Param: 0, Narrow: primitive(7)}}
	input.Contracts = ContractsInput{
		Function: []FunctionContract{{ReturnsKnown: true, Returns: []keyspace.Term{primitive(18)}}},
		Call:     []CallContract{{TypeArguments: []keyspace.Term{primitive(19)}}},
	}
	input.Operators = OperatorsInput{
		TypeOf:      []TypeOf{{Scope: term(keyspace.FamilyCell, 1), Operand: term(keyspace.FamilyRead, 1)}},
		KeyOf:       []KeyOf{{Inner: term(keyspace.FamilyTypeOf, 1)}},
		IndexAccess: []IndexAccess{{Object: primitive(20), Index: primitive(21)}},
		Conditional: []Conditional{{Check: primitive(22), Extends: primitive(23), Then: primitive(24), Else: primitive(25)}},
	}
	input.Operands = OperandsInput{
		Claim:     []ClaimTarget{{Claim: term(keyspace.FamilyValueClaim, 1), Target: primitive(26)}},
		TypeValue: []TypeValueTarget{{Target: primitive(27)}},
	}
	input.Publications = PublicationsInput{Type: []Publication{{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyTypeRef, 2)}}}
	return input
}

func TestStaticLocalContainmentCompositeEmitterRows(t *testing.T) {
	input := compositeLocalContainmentInput(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	wantID := draft.state.component.ContentID()
	if !wantID.Available() {
		t.Fatal("composite fixture produced unavailable authored identity")
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	term := func(family keyspace.Family, ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(family, ordinal)
	}
	parents := make(map[keyspace.Term]keyspace.Term)
	for _, family := range staticTypeFamilies {
		for ordinal := uint32(1); ordinal <= input.Counts[family]; ordinal++ {
			parents[term(family, ordinal)] = 0
		}
	}
	setParent := func(child, parent keyspace.Term) { parents[child] = parent }
	setParent(primitive(1), term(keyspace.FamilyTypeAlias, 1))
	setParent(primitive(2), term(keyspace.FamilyTypeParam, 1))
	setParent(primitive(3), term(keyspace.FamilyTypeField, 1))
	setParent(primitive(4), term(keyspace.FamilyTypeFunction, 1))
	setParent(primitive(5), term(keyspace.FamilyTypeFunction, 1))
	setParent(term(keyspace.FamilyTypeAsserts, 1), term(keyspace.FamilyTypeFunction, 1))
	setParent(primitive(6), term(keyspace.FamilyTypeFunction, 1))
	setParent(primitive(7), term(keyspace.FamilyTypeAsserts, 1))
	setParent(primitive(8), term(keyspace.FamilyTypeOptional, 1))
	setParent(primitive(9), term(keyspace.FamilyTypeUnion, 1))
	setParent(primitive(10), term(keyspace.FamilyTypeUnion, 1))
	setParent(primitive(11), term(keyspace.FamilyTypeIntersection, 1))
	setParent(primitive(12), term(keyspace.FamilyTypeIntersection, 1))
	setParent(term(keyspace.FamilyTypeRef, 3), term(keyspace.FamilyTypeGeneric, 1))
	setParent(primitive(13), term(keyspace.FamilyTypeGeneric, 1))
	setParent(primitive(14), term(keyspace.FamilyTypeArray, 1))
	setParent(primitive(15), term(keyspace.FamilyTypeMap, 1))
	setParent(primitive(16), term(keyspace.FamilyTypeMap, 1))
	setParent(primitive(17), term(keyspace.FamilyTypeField, 2))
	setParent(term(keyspace.FamilyTypeRef, 1), term(keyspace.FamilyTypeInterface, 1))
	setParent(term(keyspace.FamilyTypeFunction, 1), term(keyspace.FamilyTypeInterface, 1))
	setParent(primitive(18), term(keyspace.FamilyFunction, 1))
	setParent(primitive(19), term(keyspace.FamilyCall, 1))
	setParent(term(keyspace.FamilyTypeRef, 2), term(keyspace.FamilyTypePublication, 1))
	setParent(term(keyspace.FamilyTypeOf, 1), term(keyspace.FamilyTypeKeyOf, 1))
	setParent(primitive(20), term(keyspace.FamilyTypeIndexAccess, 1))
	setParent(primitive(21), term(keyspace.FamilyTypeIndexAccess, 1))
	setParent(primitive(22), term(keyspace.FamilyTypeConditional, 1))
	setParent(primitive(23), term(keyspace.FamilyTypeConditional, 1))
	setParent(primitive(24), term(keyspace.FamilyTypeConditional, 1))
	setParent(primitive(25), term(keyspace.FamilyTypeConditional, 1))
	setParent(primitive(26), term(keyspace.FamilyValueClaim, 1))
	setParent(primitive(27), term(keyspace.FamilyTypeValue, 1))
	fieldOwners := map[keyspace.Term]keyspace.Term{
		term(keyspace.FamilyTypeField, 1): term(keyspace.FamilyTypeInterface, 1),
		term(keyspace.FamilyTypeField, 2): term(keyspace.FamilyTypeRecord, 1),
	}
	for child, wantParent := range parents {
		gotParent, ok := proof.Parent(child)
		if wantParent == 0 {
			if ok || gotParent != 0 {
				t.Fatalf("root Parent(%v) = %v/%v, want 0/false", child, gotParent, ok)
			}
			continue
		}
		if !ok || gotParent != wantParent {
			t.Fatalf("Parent(%v) = %v/%v, want %v/true", child, gotParent, ok, wantParent)
		}
	}
	for field, wantOwner := range fieldOwners {
		if gotOwner, ok := proof.FieldOwner(field); !ok || gotOwner != wantOwner {
			t.Fatalf("FieldOwner(%v) = %v/%v, want %v/true", field, gotOwner, ok, wantOwner)
		}
	}
	seen := make(map[keyspace.Term]struct{}, proof.Count())
	for index := 0; index < proof.Count(); index++ {
		at, ok := proof.At(index)
		if !ok {
			t.Fatalf("At(%d) failed within Count=%d", index, proof.Count())
		}
		if _, duplicate := seen[at]; duplicate {
			t.Fatalf("At(%d) duplicated %v", index, at)
		}
		seen[at] = struct{}{}
		if keyspace.TermFamily(at) == keyspace.FamilyTypeField {
			t.Fatalf("At(%d) exposed Field %v outside FieldOwner", index, at)
		}
	}
	if len(seen) != len(parents) {
		t.Fatalf("At set size = %d, closed parent denominator = %d", len(seen), len(parents))
	}
	for at := range parents {
		if _, ok := seen[at]; !ok {
			t.Fatalf("At enumeration omitted parent-domain term %v", at)
		}
	}
	if _, ok := proof.At(-1); ok {
		t.Fatal("At accepted negative index")
	}
	if _, ok := proof.At(proof.Count()); ok {
		t.Fatal("At accepted Count index")
	}
	if _, ok := proof.Parent(term(keyspace.FamilyRead, 1)); ok {
		t.Fatal("Parent accepted foreign family")
	}
	if _, ok := proof.Parent(keyspace.Term(uint32(keyspace.FamilyTypePrimitive))); ok {
		t.Fatal("Parent accepted ordinal-zero term")
	}
	if _, ok := proof.Parent(primitive(input.Counts[keyspace.FamilyTypePrimitive] + 1)); ok {
		t.Fatal("Parent accepted out-of-range ordinal")
	}
	coldID := draft.state.component.ContentID()
	if coldID != wantID {
		t.Fatalf("repeated Build identity = %x, want %x", coldID, wantID)
	}
	component, err := finalizer.Commit(commitInputForDraft(draft))
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if got := component.ContentID(); got != coldID {
		t.Fatalf("published identity = %x, before proof = %x", got, coldID)
	}
	if got := component.Cold().ContentID(); got != coldID {
		t.Fatalf("Cold identity = %x, before proof = %x", got, coldID)
	}
}

func TestStaticLocalContainmentRowsAndBounds(t *testing.T) {
	draft, err := Build(declarationFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	param := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	iface := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	ref := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTypeField, 1)
	if got, ok := proof.Parent(primitive(1)); !ok || got != alias {
		t.Fatalf("alias target parent = %v/%v, want %v/true", got, ok, alias)
	}
	if got, ok := proof.Parent(primitive(2)); !ok || got != param {
		t.Fatalf("constraint parent = %v/%v, want %v/true", got, ok, param)
	}
	if got, ok := proof.Parent(ref); !ok || got != iface {
		t.Fatalf("interface extension parent = %v/%v, want %v/true", got, ok, iface)
	}
	if got, ok := proof.Parent(function); !ok || got != iface {
		t.Fatalf("interface method parent = %v/%v, want %v/true", got, ok, iface)
	}
	if got, ok := proof.Parent(primitive(3)); !ok || got != field {
		t.Fatalf("field value parent = %v/%v, want %v/true", got, ok, field)
	}
	if got, ok := proof.Parent(alias); ok || got != 0 {
		t.Fatalf("root alias parent = %v/%v, want 0/false", got, ok)
	}
	if got, ok := proof.FieldOwner(field); !ok || got != iface {
		t.Fatalf("field owner = %v/%v, want %v/true", got, ok, iface)
	}
	if _, ok := proof.Parent(keyspace.MakeTerm(keyspace.FamilyRead, 1)); ok {
		t.Fatal("Parent accepted foreign family")
	}
	if _, ok := proof.Parent(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4)); ok {
		t.Fatal("Parent accepted out-of-range ordinal")
	}
	if _, ok := proof.Parent(0); ok {
		t.Fatal("Parent accepted zero term")
	}
	if _, ok := proof.FieldOwner(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)); ok {
		t.Fatal("FieldOwner accepted non-field family")
	}
	if _, ok := proof.FieldOwner(keyspace.MakeTerm(keyspace.FamilyTypeField, 2)); ok {
		t.Fatal("FieldOwner accepted out-of-range field")
	}
	if proof.Count() == 0 {
		t.Fatal("LocalContainment omitted closed static denominator")
	}
	if _, ok := proof.At(-1); ok {
		t.Fatal("LocalContainment.At accepted negative index")
	}
	if _, ok := proof.At(proof.Count()); ok {
		t.Fatal("LocalContainment.At accepted out-of-range index")
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
}

func TestStaticLocalContainmentExpiresCopiesAndPreservesIdentity(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	copied := proof
	if _, ok := proof.At(0); !ok {
		t.Fatal("claimed LocalContainment unavailable")
	}
	want := finalizer.View().Types().Primitives().Count()
	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if got := component.View().Types().Primitives().Count(); got != want {
		t.Fatalf("published Component count = %d, want %d", got, want)
	}
	if _, ok := proof.At(0); ok {
		t.Fatal("LocalContainment survived Commit")
	}
	if _, ok := copied.At(0); ok {
		t.Fatal("copied LocalContainment survived Commit")
	}
	if draft.state.localContainment != nil {
		t.Fatal("Draft retained local proof after Commit")
	}
	cold := component.Cold()
	if got := cold.ContentID(); got != component.ContentID() {
		t.Fatalf("Cold identity = %x, Component identity = %x", got, component.ContentID())
	}
	componentProof := component.View().LocalContainment()
	if componentProof.Count() != 0 {
		t.Fatalf("published Component View exposed LocalContainment count %d", componentProof.Count())
	}
	if _, ok := componentProof.At(0); ok {
		t.Fatal("published Component View exposed LocalContainment rows")
	}

	abortDraft := primitiveDraft(t)
	abortFinalizer, err := abortDraft.Finalizer()
	if err != nil {
		t.Fatalf("Abort Finalizer() error = %v", err)
	}
	abortProof := abortFinalizer.View().LocalContainment()
	if err := abortFinalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if _, ok := abortProof.At(0); ok {
		t.Fatal("LocalContainment survived Abort")
	}
	if abortDraft.state.localContainment != nil {
		t.Fatal("Draft retained local proof after Abort")
	}
}

func TestStaticLocalContainmentReadsRaceTerminal(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		<-start
		for range 1000 {
			proof.Parent(primitive)
			proof.FieldOwner(keyspace.MakeTerm(keyspace.FamilyTypeField, 1))
			proof.Count()
			proof.At(0)
		}
	}()
	go func() {
		defer group.Done()
		<-start
		_, _ = finalizer.Commit(CommitInput{})
	}()
	close(start)
	group.Wait()
}

func TestStaticLocalContainmentQueriesDoNotAllocate(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if allocations := testing.AllocsPerRun(100, func() {
		proof.Parent(primitive)
		proof.FieldOwner(keyspace.MakeTerm(keyspace.FamilyTypeField, 1))
		proof.Count()
		proof.At(0)
	}); allocations != 0 {
		t.Fatalf("LocalContainment queries allocated %.2f times", allocations)
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
}
