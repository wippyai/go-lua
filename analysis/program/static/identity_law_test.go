package static

import (
	"testing"

	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"

	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"

	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// These close the remaining ordered-relation cases not exercised by the
// field deltas above. Sparse staticoperands.ClaimTarget is different: builder input order is
// not authored order, because its canonical relation is Flow Claim ordinal.
func TestStaticContentIDResidualOrderAndSparseClaimCanonicalization(t *testing.T) {
	t.Run("primitive rows", func(t *testing.T) {
		input := contentTypesInput(t)
		before := staticContentComponent(t, input).ContentID()
		input.Types.Primitive[0], input.Types.Primitive[1] = input.Types.Primitive[1], input.Types.Primitive[0]
		if after := staticContentComponent(t, input).ContentID(); after == before {
			t.Fatal("primitive row order omitted from ContentID")
		}
	})
	t.Run("typeof rows", func(t *testing.T) {
		input := operatorFixture()
		input.Counts[keyspace.FamilyRead] = 2
		input.Operators.TypeOf[1].Operand = keyspace.MakeTerm(keyspace.FamilyRead, 2)
		before := staticContentComponent(t, input).ContentID()
		input.Operators.TypeOf[0], input.Operators.TypeOf[1] = input.Operators.TypeOf[1], input.Operators.TypeOf[0]
		if after := staticContentComponent(t, input).ContentID(); after == before {
			t.Fatal("TypeOf row order omitted from ContentID")
		}
	})
	t.Run("publication rows", func(t *testing.T) {
		input := publicationFixture(t)
		input.Counts[keyspace.FamilyTypePublication] = 2
		input.Publications.Type = append(input.Publications.Type, staticpubs.Publication{
			Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: 1, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2),
		})
		first := staticContentComponent(t, input).ContentID()
		input.Publications.Type[0], input.Publications.Type[1] = input.Publications.Type[1], input.Publications.Type[0]
		if second := staticContentComponent(t, input).ContentID(); second == first {
			t.Fatal("TypePublication row order omitted from ContentID")
		}
	})
	t.Run("sparse claim input permutation", func(t *testing.T) {
		input := operandsFixture(t)
		input.Counts[keyspace.FamilyTypePrimitive] = 4
		input.Types.Primitive = append(input.Types.Primitive, statictypes.Primitive{Kind: statictypes.PrimitiveBoolean})
		input.Operands.Claim = append(input.Operands.Claim, staticoperands.ClaimTarget{
			Claim: keyspace.MakeTerm(keyspace.FamilyValueClaim, 2), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4),
		})
		first := staticContentComponent(t, input).ContentID()
		input.Operands.Claim[0], input.Operands.Claim[1] = input.Operands.Claim[1], input.Operands.Claim[0]
		if second := staticContentComponent(t, input).ContentID(); second != first {
			t.Fatalf("sparse staticoperands.ClaimTarget input order changed canonical ContentID: %x != %x", second, first)
		}
	})
}

var sourceZeroCoordinate = source.Coordinate{}

func contentReferencesComponent(t *testing.T) *Component {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeRef], counts[keyspace.FamilyTypeAlias], counts[keyspace.FamilyCell] = 3, 1, 1
	return staticContentComponent(t, referenceInput(counts, staticrefs.Input{TypeRef: []staticrefs.TypeRef{
		{Resolution: staticrefs.Declaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
		{Resolution: staticrefs.CanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{7, 8}},
		{Resolution: staticrefs.Unresolved, Source: []keyspace.Key{4, 5}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1)},
	}}))
}

func contentDeclarationsComponent(t *testing.T) *Component {
	return staticContentComponent(t, declarationFixture(t))
}

func contentDeclaredTypeComponent(t *testing.T) *Component {
	return staticContentComponent(t, declaredTypeFixture(t))
}

func contentSignaturesComponent(t *testing.T) *Component {
	return staticContentComponent(t, signatureFixture(t))
}

func contentContractsComponent(t *testing.T) *Component {
	return staticContentComponent(t, contractsFixture(t))
}

func contentOperatorsComponent(t *testing.T) *Component {
	return staticContentComponent(t, operatorFixture())
}

func contentOperandsComponent(t *testing.T) *Component {
	return staticContentComponent(t, operandsFixture(t))
}

func contentPublicationsComponent(t *testing.T) *Component {
	return staticContentComponent(t, publicationFixture(t))
}

// The eight cases are the closed ContentID record denominator. Each mutation
// changes one retained authored relation in its vertical while preserving the
// local Static laws; no query projection is used as semantic input.
func TestStaticContentIDCoversEveryAuthoredVertical(t *testing.T) {
	cases := []struct {
		name   string
		input  func(*testing.T) Input
		mutate func(*testing.T, *Input)
	}{
		{
			name: "types",
			input: func(*testing.T) Input {
				counts := [keyspace.FamilyCount]uint32{}
				counts[keyspace.FamilyTypePrimitive] = 1
				return Input{Counts: counts, Types: statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}}}
			},
			mutate: func(_ *testing.T, input *Input) { input.Types.Primitive[0].Kind = statictypes.PrimitiveString },
		},
		{
			name: "references",
			input: func(*testing.T) Input {
				counts := [keyspace.FamilyCount]uint32{}
				counts[keyspace.FamilyTypeRef] = 1
				return Input{Counts: counts, References: staticrefs.Input{TypeRef: []staticrefs.TypeRef{{
					Resolution: staticrefs.Unresolved, Source: []keyspace.Key{1},
				}}}}
			},
			mutate: func(_ *testing.T, input *Input) {
				input.References.TypeRef[0].Source = append([]keyspace.Key(nil), input.References.TypeRef[0].Source...)
				input.References.TypeRef[0].Source[0] = 2
			},
		},
		{
			name:   "declarations",
			input:  declarationFixture,
			mutate: func(_ *testing.T, input *Input) { input.Declarations.Alias[0].Name = 77 },
		},
		{
			name:  "signatures",
			input: signatureFixture,
			mutate: func(t *testing.T, input *Input) {
				coordinate, ok := source.CoordinateFromParts(4, 1, 4, 5)
				if !ok {
					t.Fatal("CoordinateFromParts rejected mutation")
				}
				input.Signatures.TypeFunction[0].VariadicCoordinate = coordinate
			},
		},
		{
			name:  "contracts",
			input: contractsFixture,
			mutate: func(_ *testing.T, input *Input) {
				input.Contracts.Call[0].TypeArguments = append([]keyspace.Term(nil), input.Contracts.Call[0].TypeArguments...)
				input.Contracts.Call[0].TypeArguments[0], input.Contracts.Call[0].TypeArguments[1] = input.Contracts.Call[0].TypeArguments[1], input.Contracts.Call[0].TypeArguments[0]
			},
		},
		{
			name: "operators",
			input: func(*testing.T) Input {
				input := operatorFixture()
				input.Counts[keyspace.FamilyCell] = 2
				return input
			},
			mutate: func(_ *testing.T, input *Input) {
				input.Operators.TypeOf[1].Scope = keyspace.MakeTerm(keyspace.FamilyCell, 2)
			},
		},
		{
			name:   "operands",
			input:  operandsFixture,
			mutate: func(_ *testing.T, input *Input) { input.Operands.Annotation[0].Name = 9 },
		},
		{
			name:   "publications",
			input:  publicationFixture,
			mutate: func(_ *testing.T, input *Input) { input.Publications.Type[0].Pair = 1 },
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := test.input(t)
			first := staticContentComponent(t, input).ContentID()
			if !first.Available() {
				t.Fatal("content identity unavailable")
			}
			if second := staticContentComponent(t, test.input(t)).ContentID(); second != first {
				t.Fatalf("equivalent rebuild ContentID = %x, want %x", second, first)
			}
			test.mutate(t, &input)
			if changed := staticContentComponent(t, input).ContentID(); changed == first {
				t.Fatal("authored vertical mutation left ContentID unchanged")
			}
		})
	}
}

func TestStaticContentIDExcludesDerivativesAndExternalClaimCardinality(t *testing.T) {
	input := operandsFixture(t)
	first := staticContentComponent(t, input)
	baseline := first.ContentID()

	// A Flow-only ValueClaim with no Static staticoperands.ClaimTarget must not change Static
	// identity just because it grows the retained dense query lookup.
	withoutTarget := input
	withoutTarget.Counts[keyspace.FamilyValueClaim]++
	if got := staticContentComponent(t, withoutTarget).ContentID(); got != baseline {
		t.Fatalf("external claim cardinality changed Static ContentID: %x != %x", got, baseline)
	}

	// A Cell the authored relation never declares must not change Static
	// identity either, though it widens the dense declared-type inverse the
	// Declarations vertical retains for O(1) lookup.
	withoutDeclaredCell := input
	withoutDeclaredCell.Counts[keyspace.FamilyCell]++
	if got := staticContentComponent(t, withoutDeclaredCell).ContentID(); got != baseline {
		t.Fatalf("external cell cardinality changed Static ContentID: %x != %x", got, baseline)
	}

	// The identity is a pure function of the authored relations. Each vertical
	// proves separately that its own derived indexes stay out of the section
	// stream, while the published component exposes the sealed result directly.
	if got := first.ContentID(); got != baseline {
		t.Fatalf("rehashing a published component changed Static ContentID: %x != %x", got, baseline)
	}
}

func TestStaticContentIDIsImmutableAndAllocationFree(t *testing.T) {
	input := publicationFixture(t)
	component, _, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	// Build has copied every semantic slice before content identity becomes
	// observable. Mutating caller storage cannot change the Component hash.
	input.References.TypeRef[0].Source[0] = 99
	input.Publications.Type[0].Pair = 19
	if got, want := component.ContentID(), staticContentComponent(t, publicationFixture(t)).ContentID(); got != want {
		t.Fatalf("caller mutation after Build changed ContentID: %x != %x", got, want)
	}
	if allocations := testing.AllocsPerRun(1000, func() { _ = component.ContentID() }); allocations != 0 {
		t.Fatalf("ContentID allocations = %f, want 0", allocations)
	}
}

// The per-field, per-arity, and per-order distinctions of the Types vertical
// are proven by TestAuthoredDistinctionsReachTheSection in
// analysis/program/static/types, against the same section schema this digest
// consumes. They moved with the rows: a sealed types table cannot be perturbed
// after Build, so the distinction is now stated over authored input.

func contentTypesComponent(t *testing.T) *Component {
	t.Helper()
	return staticContentComponent(t, contentTypesInput(t))
}

func contentTypesInput(t *testing.T) Input {
	t.Helper()
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("CoordinateFromParts rejected content fixture")
	}
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 5
	counts[keyspace.FamilyTypeLiteral] = 1
	counts[keyspace.FamilyTypeOptional] = 1
	counts[keyspace.FamilyTypeUnion] = 1
	counts[keyspace.FamilyTypeIntersection] = 1
	counts[keyspace.FamilyTypeRef] = 2
	counts[keyspace.FamilyTypeGeneric] = 1
	counts[keyspace.FamilyTypeArray] = 1
	counts[keyspace.FamilyTypeMap] = 1
	counts[keyspace.FamilyTypeRecord] = 1
	counts[keyspace.FamilyTypeField] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyCell] = 1
	primitive := func(ordinal uint32) keyspace.Term { return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal) }
	input := Input{Counts: counts,
		Types: statictypes.Input{
			Primitive:    []statictypes.Primitive{{Kind: statictypes.PrimitiveNil}, {Kind: statictypes.PrimitiveNumber}, {Kind: statictypes.PrimitiveString}, {Kind: statictypes.PrimitiveBoolean}, {Kind: statictypes.PrimitiveNever}},
			Literal:      []statictypes.Literal{{Kind: keyspace.LiteralString, Exact: 7}},
			Optional:     []statictypes.Optional{{Inner: primitive(1)}},
			Union:        []statictypes.Union{{Members: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1), primitive(2)}}},
			Intersection: []statictypes.Intersection{{Members: []keyspace.Term{primitive(3), primitive(4)}}},
			Generic:      []statictypes.Generic{{Base: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1), Args: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeUnion, 1)}}},
			Array:        []statictypes.Array{{Element: keyspace.MakeTerm(keyspace.FamilyTypeGeneric, 1), ReadOnly: true}},
			Map:          []statictypes.Map{{Key: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2), Value: keyspace.MakeTerm(keyspace.FamilyTypeArray, 1)}},
			Field:        []statictypes.Field{{Key: 9, Type: keyspace.MakeTerm(keyspace.FamilyTypeMap, 1), Optional: true}},
			Record:       []statictypes.Record{{Fields: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeField, 1)}, ReadOnly: true}},
		},
		References: staticrefs.Input{TypeRef: []staticrefs.TypeRef{
			{Resolution: staticrefs.Declaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
			{Resolution: staticrefs.CanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{4}},
		}},
		Declarations: staticdecl.Input{Alias: []staticdecl.TypeAlias{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: primitive(5), Name: 10, NameCoordinate: coordinate}}},
	}
	return input
}

func TestStaticIdentityCodecsAreStableAndOwnerScoped(t *testing.T) {
	owner := identity.ContentID{0: 1}
	otherOwner := identity.ContentID{0: 2}
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	otherTerm := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	ref, ok := component.View().StaticTypes().Ref(term)
	if !ok {
		t.Fatal("StaticTypes.Ref rejected the identity-law type term")
	}

	occur, occurOK := staticquery.OccurrenceID(owner, 1, term)
	if !occurOK {
		t.Fatalf("OccurrenceID = %x/%v, want available identity", occur, occurOK)
	}
	if again, againOK := staticquery.OccurrenceID(owner, 1, term); !againOK || again != occur {
		t.Fatal("OccurrenceID was not deterministic")
	}
	if changed, changedOK := staticquery.OccurrenceID(owner, 1, otherTerm); !changedOK || changed == occur {
		t.Fatal("OccurrenceID ignored its authored term")
	}
	if changed, changedOK := staticquery.OccurrenceID(otherOwner, 1, term); !changedOK || changed == occur {
		t.Fatal("OccurrenceID ignored its owner")
	}

	typeID, typeOK := staticquery.TypeReferenceID(owner, ref)
	expressionID, expressionOK := staticquery.ExpressionID(owner, ref)
	inputID, inputOK := staticquery.InputID(owner, 2, term, 3)
	scopeID, scopeOK := staticquery.ScopeID(owner, term)
	if !typeOK || !expressionOK || !inputOK || !scopeOK {
		t.Fatalf("static identity availability = type=%v expression=%v input=%v scope=%v", typeOK, expressionOK, inputOK, scopeOK)
	}
	if typeID == expressionID || typeID == inputID || typeID == scopeID || expressionID == inputID || expressionID == scopeID || inputID == scopeID {
		t.Fatal("Static identity domains collided")
	}
	if again, againOK := staticquery.TypeReferenceID(owner, ref); !againOK || again != typeID {
		t.Fatal("TypeReferenceID was not deterministic")
	}
	if again, againOK := staticquery.ExpressionID(owner, ref); !againOK || again != expressionID {
		t.Fatal("ExpressionID was not deterministic")
	}
	if changed, changedOK := staticquery.InputID(owner, 2, term, 4); !changedOK || changed == inputID {
		t.Fatal("InputID ignored its dense index")
	}
	if changed, changedOK := staticquery.ScopeID(owner, otherTerm); !changedOK || changed == scopeID {
		t.Fatal("ScopeID ignored its authored scope")
	}
}

func TestStaticIdentityCodecsRejectUnavailableInputs(t *testing.T) {
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	var zero identity.ContentID
	if _, ok := staticquery.OccurrenceID(zero, 1, term); ok {
		t.Fatal("OccurrenceID accepted an unavailable owner")
	}
	if _, ok := staticquery.OccurrenceID(identity.ContentID{0: 1}, 0, term); ok {
		t.Fatal("OccurrenceID accepted an invalid family")
	}
	if _, ok := staticquery.OccurrenceID(identity.ContentID{0: 1}, 1, 0); ok {
		t.Fatal("OccurrenceID accepted an invalid term")
	}
	if _, ok := staticquery.TypeReferenceID(identity.ContentID{0: 1}, staticquery.StaticTypeRef{}); ok {
		t.Fatal("TypeReferenceID accepted an unavailable reference")
	}
	if _, ok := staticquery.ExpressionID(identity.ContentID{0: 1}, staticquery.StaticTypeRef{}); ok {
		t.Fatal("ExpressionID accepted an unavailable reference")
	}
	if _, ok := staticquery.InputID(zero, 1, term, 0); ok {
		t.Fatal("InputID accepted an unavailable owner")
	}
	if _, ok := staticquery.InputID(identity.ContentID{0: 1}, 1, 0, 0); ok {
		t.Fatal("InputID accepted an invalid source")
	}
	if _, ok := staticquery.ScopeID(zero, term); ok {
		t.Fatal("ScopeID accepted an unavailable owner")
	}
	if _, ok := staticquery.ScopeID(identity.ContentID{0: 1}, 0); ok {
		t.Fatal("ScopeID accepted an invalid scope")
	}
}
