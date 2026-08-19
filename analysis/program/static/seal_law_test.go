package static

import (
	"testing"

	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"

	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"

	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"

	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"

	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"

	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static/operators"
)

func TestStaticColdRetainsOnlyContentIDSnapshot(t *testing.T) {
	component := staticContentComponent(t, Input{
		Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyTypePrimitive: 1},
		Types:  statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}}},
	})
	cold := component.Cold()
	want := cold.ContentID()
	component.contentID = identity.ContentID{}
	if got := cold.ContentID(); got != want {
		t.Fatalf("Cold snapshot changed after Component mutation: %x != %x", got, want)
	}
}

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
	input.Types.Primitive = make([]statictypes.Primitive, 27)
	for index := range input.Types.Primitive {
		input.Types.Primitive[index] = statictypes.Primitive{Kind: statictypes.PrimitiveAny}
	}
	input.Types.Optional = []statictypes.Optional{{Inner: primitive(8)}}
	input.Types.Union = []statictypes.Union{{Members: []keyspace.Term{primitive(9), primitive(10)}}}
	input.Types.Intersection = []statictypes.Intersection{{Members: []keyspace.Term{primitive(11), primitive(12)}}}
	input.Types.Generic = []statictypes.Generic{{Base: term(keyspace.FamilyTypeRef, 3), Args: []keyspace.Term{primitive(13)}}}
	input.Types.Array = []statictypes.Array{{Element: primitive(14)}}
	input.Types.Map = []statictypes.Map{{Key: primitive(15), Value: primitive(16)}}
	input.Types.Field = []statictypes.Field{{Key: 4, Type: primitive(3), Optional: true}, {Key: 5, Type: primitive(17)}}
	input.Types.Record = []statictypes.Record{{Fields: []keyspace.Term{term(keyspace.FamilyTypeField, 2)}}}
	input.References.TypeRef = append(input.References.TypeRef,
		staticrefs.TypeRef{Resolution: staticrefs.Declaration, Target: term(keyspace.FamilyTypeAlias, 1), Source: []keyspace.Key{6}},
		staticrefs.TypeRef{Resolution: staticrefs.Declaration, Target: term(keyspace.FamilyTypeInterface, 1), Source: []keyspace.Key{7}},
	)
	input.Signatures.TypeFunction[0] = staticsig.TypeFunction{
		Scope:      term(keyspace.FamilyTypeInterface, 1),
		Parameters: []staticsig.Parameter{{Name: 20, NameCoordinate: coordinate, Type: primitive(4)}},
		Variadic:   primitive(5), VariadicCoordinate: coordinate,
		ReturnsKnown: true, Returns: []keyspace.Term{term(keyspace.FamilyTypeAsserts, 1), primitive(6)},
	}
	input.Counts[keyspace.FamilyTypeAsserts] = 1
	input.Signatures.TypeAsserts = []staticsig.TypeAsserts{{Name: 20, ParamCoordinate: coordinate, Bound: true, Param: 0, Narrow: primitive(7)}}
	input.Contracts = staticcontracts.Input{
		Function: []staticcontracts.FunctionContract{{ReturnsKnown: true, Returns: []keyspace.Term{primitive(18)}}},
		Call:     []staticcontracts.CallContract{{TypeArguments: []keyspace.Term{primitive(19)}}},
	}
	input.Operators = operators.Input{
		TypeOf:      []operators.TypeOf{{Scope: term(keyspace.FamilyCell, 1), Operand: term(keyspace.FamilyRead, 1)}},
		KeyOf:       []operators.KeyOf{{Inner: term(keyspace.FamilyTypeOf, 1)}},
		IndexAccess: []operators.IndexAccess{{Object: primitive(20), Index: primitive(21)}},
		Conditional: []operators.Conditional{{Check: primitive(22), Extends: primitive(23), Then: primitive(24), Else: primitive(25)}},
	}
	input.Operands = staticoperands.Input{
		Claim:     []staticoperands.ClaimTarget{{Claim: term(keyspace.FamilyValueClaim, 1), Target: primitive(26)}},
		TypeValue: []staticoperands.TypeValueTarget{{Target: primitive(27)}},
	}
	input.Publications = staticpubs.Input{Type: []staticpubs.Publication{{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyTypeRef, 2)}}}
	return input
}

func TestStaticLocalContainmentCompositeEmitterRows(t *testing.T) {
	input := compositeLocalContainmentInput(t)
	component, staticView, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	wantID := component.ContentID()
	if !wantID.Available() {
		t.Fatal("composite fixture produced unavailable authored identity")
	}
	proof := staticView.LocalContainment()
	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	term := func(family keyspace.Family, ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(family, ordinal)
	}
	parents := make(map[keyspace.Term]keyspace.Term)
	for family := keyspace.FamilyTypeAlias; family <= keyspace.FamilyTypeConditional; family++ {
		if !staticquery.StaticTypeFamily(family) {
			continue
		}
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
	coldID := component.ContentID()
	if coldID != wantID {
		t.Fatalf("repeated Build identity = %x, want %x", coldID, wantID)
	}
	if got := component.Cold().ContentID(); got != coldID {
		t.Fatalf("Cold identity = %x, before proof = %x", got, coldID)
	}
}

func TestStaticLocalContainmentRowsAndBounds(t *testing.T) {
	_, staticView, err := Build(declarationFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	proof := staticView.LocalContainment()
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
}

// The closed authored-static family set and its immutable owner stores must
// move together. This is intentionally a local query law: it prevents a new
// type form from being accepted by Build yet becoming unqueryable as an
// annotation anchor after compaction.
func TestAnnotationAnchorCoversEveryStaticTypeFamily(t *testing.T) {
	var census [keyspace.FamilyCount]uint32
	anchor := func(term keyspace.Term) bool {
		return staticrole.AnnotationTarget(census, term)
	}
	for family := keyspace.FamilyTypePrimitive; family <= keyspace.FamilyTypeConditional; family++ {
		if !staticrole.NodeFamily(family) {
			continue
		}
		census[family] = 1
		if !anchor(keyspace.MakeTerm(family, 1)) {
			t.Fatalf("annotation anchor rejected static family %v", family)
		}
		if anchor(keyspace.MakeTerm(family, 2)) {
			t.Fatalf("annotation anchor accepted %v past its census", family)
		}
	}
	for family := keyspace.FamilyTypeAlias; family <= keyspace.FamilyTypeParam; family++ {
		census[family] = 1
		if anchor(keyspace.MakeTerm(family, 1)) {
			t.Fatalf("annotation anchor accepted declaration root %v", family)
		}
	}
}

// This chain is deliberately large enough that the former walk-from-every-
// child algorithm would repeatedly rediscover every suffix. The law asserts
// structure, not a machine-dependent duration: acceptance of the chain and
// rejection after one back-edge exercise the same linear containment seal.
func TestTypesContainmentLongChainAndCycle(t *testing.T) {
	const length = 8192
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeOptional] = length
	rows := make([]statictypes.Optional, length)
	rows[0] = statictypes.Optional{Inner: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}
	for index := 1; index < len(rows); index++ {
		rows[index] = statictypes.Optional{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, uint32(index))}
	}
	input := Input{Counts: counts, Types: statictypes.Input{
		Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}},
		Optional:  rows,
	}}
	if _, _, err := Build(input); err != nil {
		t.Fatalf("Build() rejected acyclic containment chain: %v", err)
	}
	input.Types.Optional[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypeOptional, length)
	if _, _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a containment cycle through a long chain")
	}
}

func TestContainmentTracksTypedParentsFieldsAndDirectReturns(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyTypePrimitive: 1,
		keyspace.FamilyTypeOptional:  1,
		keyspace.FamilyTypeField:     1,
		keyspace.FamilyTypeAsserts:   1,
	}
	check := newContainment(counts, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	optional := keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTypeField, 1)
	opaqueOwner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if !check.attach(opaqueOwner, primitive) || !check.attach(primitive, optional) {
		t.Fatal("containment rejected a valid typed parent chain")
	}
	if check.attach(opaqueOwner, primitive) {
		t.Fatal("containment accepted a duplicate concrete child")
	}
	if !check.claimField(opaqueOwner, field) {
		t.Fatal("containment rejected the first Field owner")
	}
	if check.claimField(opaqueOwner, field) {
		t.Fatal("containment accepted duplicate Field ownership")
	}
	assertion := keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
	if !check.markDirectReturn(opaqueOwner, assertion) || check.markDirectReturn(opaqueOwner, assertion) {
		t.Fatal("direct assertion-return evidence did not enforce one claim")
	}
	if check.parentOf(optional) != primitive || check.parentOf(field) != opaqueOwner {
		t.Fatal("containment parent lookup lost typed ownership")
	}
	if !check.valid() {
		t.Fatal("valid typed containment chain was rejected")
	}

	cycle := newContainment(counts, 1)
	if !cycle.attach(primitive, optional) || !cycle.attach(optional, primitive) {
		t.Fatal("cycle fixture could not be constructed")
	}
	if cycle.valid() {
		t.Fatal("containment accepted a concrete cycle")
	}
}

// TestStaticCensusRejectsIncompleteRelationCoverage proves the one cardinality
// column is total: a dense family's authored relation length must equal its
// census entry, in every vertical. This is a shell law because the census is
// the shell's authority, not any vertical's.
func TestStaticCensusRejectsIncompleteRelationCoverage(t *testing.T) {
	for _, test := range []struct {
		name  string
		input func(*testing.T) Input
		edit  func(*Input)
	}{
		{"missing function contract", contractsFixture, func(in *Input) { in.Contracts.Function = nil }},
		{"extra call contract", contractsFixture, func(in *Input) {
			in.Contracts.Call = append(in.Contracts.Call, staticcontracts.CallContract{})
		}},
		{"missing signature row", signatureFixture, func(in *Input) { in.Signatures.TypeFunction = nil }},
		{"missing assertion row", signatureFixture, func(in *Input) { in.Signatures.TypeAsserts = nil }},
		{"missing typeof row", func(*testing.T) Input { return operatorFixture() }, func(in *Input) {
			in.Operators.TypeOf = in.Operators.TypeOf[:1]
		}},
		{"missing keyof row", func(*testing.T) Input { return operatorFixture() }, func(in *Input) {
			in.Operators.KeyOf = nil
		}},
		{"missing indexed access row", func(*testing.T) Input { return operatorFixture() }, func(in *Input) {
			in.Operators.IndexAccess = nil
		}},
		{"missing conditional row", func(*testing.T) Input { return operatorFixture() }, func(in *Input) {
			in.Operators.Conditional = nil
		}},
		{"extra typeof count", func(*testing.T) Input { return operatorFixture() }, func(in *Input) {
			in.Counts[keyspace.FamilyTypeOf] = 1
		}},
		{"missing type value row", operandsFixture, func(in *Input) { in.Operands.TypeValue = nil }},
		{"missing annotation row", operandsFixture, func(in *Input) {
			in.Operands.Annotation = in.Operands.Annotation[:1]
		}},
		{"missing publication count", publicationFixture, func(in *Input) {
			in.Counts[keyspace.FamilyTypePublication] = 0
		}},
		{"extra publication count", publicationFixture, func(in *Input) {
			in.Counts[keyspace.FamilyTypePublication] = 2
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := test.input(t)
			test.edit(&input)
			if _, _, err := Build(input); err == nil {
				t.Fatal("Build() accepted an incomplete authored relation denominator")
			}
		})
	}
}

// TestStaticTypeParamOwnershipIsExactlyOnce proves the single joint law over
// the three claimant columns Declarations, Signatures, and Contracts publish:
// every authored TypeParam is claimed exactly once, by the owner its row names.
func TestStaticTypeParamOwnershipIsExactlyOnce(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, *Input)
	}{
		{"unclaimed type parameter", func(_ *testing.T, in *Input) { in.Contracts.Function[0].TypeParams = nil }},
		{"claimed twice by one owner", func(_ *testing.T, in *Input) {
			in.Counts[keyspace.FamilyTypeParam] = 2
			in.Declarations.TypeParam = append(in.Declarations.TypeParam,
				staticdecl.TypeParam{Owner: keyspace.MakeTerm(keyspace.FamilyFunction, 1), Name: 4})
			in.Contracts.Function[0].TypeParams = []keyspace.Term{
				keyspace.MakeTerm(keyspace.FamilyTypeParam, 1), keyspace.MakeTerm(keyspace.FamilyTypeParam, 1),
			}
		}},
		{"claimant disagrees with the row owner", func(_ *testing.T, in *Input) {
			in.Declarations.TypeParam[0].Owner = keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
			in.Counts[keyspace.FamilyTypeAlias] = 1
		}},
		{"claimed by an alias and a function", func(t *testing.T, in *Input) {
			in.Counts[keyspace.FamilyTypeAlias] = 1
			in.Counts[keyspace.FamilyBody] = 1
			in.Declarations.Alias = []staticdecl.TypeAlias{{
				Owner:  keyspace.MakeTerm(keyspace.FamilyBody, 1),
				Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
				Name:   11, NameCoordinate: signatureCoordinate(t),
				Params: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
			}}
		}},
		{"alias claims a non-parameter", func(t *testing.T, in *Input) {
			in.Counts[keyspace.FamilyTypeAlias] = 1
			in.Counts[keyspace.FamilyBody] = 1
			in.Declarations.Alias = []staticdecl.TypeAlias{{
				Owner:  keyspace.MakeTerm(keyspace.FamilyBody, 1),
				Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
				Name:   11, NameCoordinate: signatureCoordinate(t),
				Params: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)},
			}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := contractsFixture(t)
			test.edit(t, &input)
			if _, _, err := Build(input); err == nil {
				t.Fatal("Build() accepted a type parameter that is not claimed exactly once")
			}
		})
	}
}

// TestStaticInterfaceMethodScopeMustBeItsInterface proves the joint law
// Declarations defers and Signatures completes: a method's TypeFunction is
// scoped to the interface that declares it.
func TestStaticInterfaceMethodScopeMustBeItsInterface(t *testing.T) {
	input := declarationFixture(t)
	input.Signatures.TypeFunction[0].Scope = keyspace.MakeTerm(keyspace.FamilyCell, 1)
	input.Counts[keyspace.FamilyCell] = 1
	if _, _, err := Build(input); err == nil {
		t.Fatal("Build() accepted an interface method scoped outside its interface")
	}
}

// TestStaticCombinedForestRejectsSharingAndCycles proves the one combined
// containment seal: every concrete authored static type has exactly one
// parent, every Field exactly one owner, and the relation is acyclic across
// vertical boundaries. No vertical gets a private exception.
func TestStaticCombinedForestRejectsSharingAndCycles(t *testing.T) {
	for _, test := range []struct {
		name  string
		input func(*testing.T) Input
		edit  func(*testing.T, *Input)
	}{
		{"declaration shares a concrete child", declarationFixture, func(_ *testing.T, in *Input) {
			in.Declarations.TypeParam[0].Constraint = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"orphan interface field", declarationFixture, func(_ *testing.T, in *Input) {
			in.Declarations.Interface[0].Members = in.Declarations.Interface[0].Members[1:]
		}},
		{"operator shares a concrete child", func(*testing.T) Input { return operatorFixture() }, func(_ *testing.T, in *Input) {
			in.Operators.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"indexed access repeats one child", func(*testing.T) Input { return operatorFixture() }, func(_ *testing.T, in *Input) {
			in.Operators.IndexAccess[0].Index = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"keyof cycle", func(*testing.T) Input { return operatorFixture() }, func(_ *testing.T, in *Input) {
			in.Operators.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)
		}},
		{"indexed access cycle", func(*testing.T) Input { return operatorFixture() }, func(_ *testing.T, in *Input) {
			in.Operators.IndexAccess[0].Object = keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)
		}},
		{"conditional cycle", func(*testing.T) Input { return operatorFixture() }, func(_ *testing.T, in *Input) {
			in.Operators.Conditional[0].Check = keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)
		}},
		{"publications share one target", publicationFixture, func(_ *testing.T, in *Input) {
			in.Counts[keyspace.FamilyAssign] = 2
			in.Counts[keyspace.FamilyTypePublication] = 2
			in.Publications.Type = append(in.Publications.Type, staticpubs.Publication{
				Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 2), Pair: 0,
				Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
			})
		}},
		{"publication and type value share one target", publicationFixture, func(_ *testing.T, in *Input) {
			in.Counts[keyspace.FamilyTypeValue] = 1
			in.Operands.TypeValue = []staticoperands.TypeValueTarget{
				{Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)},
			}
		}},
		{"claim and type value share one target", operandsFixture, func(_ *testing.T, in *Input) {
			in.Operands.TypeValue[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"call argument shares a function return", contractsFixture, func(_ *testing.T, in *Input) {
			in.Contracts.Call[0].TypeArguments[0] = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := test.input(t)
			test.edit(t, &input)
			if _, _, err := Build(input); err == nil {
				t.Fatal("Build() accepted a shared or cyclic concrete child")
			}
		})
	}

	// A cycle wholly inside one vertical is the same law.
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeOptional] = 1
	if _, _, err := Build(Input{Counts: counts, Types: statictypes.Input{
		Optional: []statictypes.Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)}},
	}}); err == nil {
		t.Fatal("Build() accepted a cyclic static type forest")
	}

	// A cycle that crosses two verticals is also the same law: neither the
	// Types edge nor the operator edge is a private forest.
	counts = [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeOptional] = 1
	counts[keyspace.FamilyTypeKeyOf] = 1
	if _, _, err := Build(Input{Counts: counts,
		Types:     statictypes.Input{Optional: []statictypes.Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)}}},
		Operators: staticoperators.Input{KeyOf: []staticoperators.KeyOf{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)}}},
	}); err == nil {
		t.Fatal("Build() accepted a cross-vertical static cycle")
	}

	// Record membership is not a generic type-child edge, but it still closes
	// the concrete containment walk: a field cannot point back to its record.
	input := declarationFixture(t)
	input.Counts[keyspace.FamilyTypeRecord] = 1
	input.Types.Record = []statictypes.Record{{Fields: []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyTypeField, 1),
	}}}
	input.Types.Field[0].Type = keyspace.MakeTerm(keyspace.FamilyTypeRecord, 1)
	input.Declarations.Interface[0].Members = input.Declarations.Interface[0].Members[1:]
	if _, _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a field type cycle through its record owner")
	}
}

// TestStaticBoundAssertionRequiresADirectReturn proves the joint bound-
// assertion law: only a direct return position may bind a formal, so an
// assertion nested inside another type cannot.
func TestStaticBoundAssertionRequiresADirectReturn(t *testing.T) {
	input := contractsFixture(t)
	input.Counts[keyspace.FamilyTypeGeneric] = 1
	input.Counts[keyspace.FamilyTypeRef] = 1
	input.Types.Generic = []statictypes.Generic{{
		Base: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
		Args: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)},
	}}
	input.References.TypeRef = []staticrefs.TypeRef{{Resolution: staticrefs.Unresolved, Source: []keyspace.Key{1}}}
	input.Contracts.Function[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeGeneric, 1)}
	if _, _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a bound assertion nested in a function return")
	}
}

// TestStaticOperandTargetsComeFromSealedSiblingTables proves the constructor
// hands Operands the sealed Types and References tables, so the runtime type
// target admission is decided by what those owners published.
func TestStaticOperandTargetsComeFromSealedSiblingTables(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Input)
	}{
		{"static-only primitive", func(in *Input) {
			in.Operands.TypeValue[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3)
		}},
		{"unresolved reference", func(in *Input) {
			in.References.TypeRef[0].Resolution = staticrefs.Unresolved
			in.References.TypeRef[0].Target = 0
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := operandsFixture(t)
			test.edit(&input)
			if _, _, err := Build(input); err == nil {
				t.Fatal("Build() admitted a runtime type target no sibling published")
			}
		})
	}
}
