package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

type contentDelta struct {
	name   string
	build  func(*testing.T) *Component
	mutate func(*Component)
}

func runContentDeltaLedger(t *testing.T, cases []contentDelta) {
	t.Helper()
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			component := test.build(t)
			before := component.Cold().ContentID()
			test.mutate(component)
			if after := contentID(component); after == before {
				t.Fatal("retained authored field did not change ContentID")
			}
		})
	}
}

func TestStaticContentIDNonTypeFieldLedger(t *testing.T) {
	cases := []contentDelta{
		{"reference.resolution", contentReferencesComponent, func(c *Component) { c.references.rows[0].resolution = TypeRefUnresolved }},
		{"reference.target", contentReferencesComponent, func(c *Component) { c.references.rows[0].target = 0 }},
		{"reference.root", contentReferencesComponent, func(c *Component) { c.references.rows[1].root = 0 }},
		{"reference.source-key", contentReferencesComponent, func(c *Component) { c.references.source[0] = 77 }},
		{"reference.source-arity", contentReferencesComponent, func(c *Component) { c.references.rows[1].source.End = c.references.rows[1].source.Start }},
		{"reference.source-order", contentReferencesComponent, swapReferenceSource},
		{"reference.canonical-key", contentReferencesComponent, func(c *Component) { c.references.canonical[0] = 77 }},
		{"reference.canonical-arity", contentReferencesComponent, func(c *Component) { c.references.rows[1].canonical.End = c.references.rows[1].canonical.Start }},
		{"reference.canonical-order", contentReferencesComponent, swapReferenceCanonical},

		{"alias.owner", contentDeclarationsComponent, func(c *Component) { c.declarations.aliases[0].owner = 0 }},
		{"alias.target", contentDeclarationsComponent, func(c *Component) { c.declarations.aliases[0].target = 0 }},
		{"alias.name", contentDeclarationsComponent, func(c *Component) { c.declarations.aliases[0].name = 77 }},
		{"alias.coordinate", contentDeclarationsComponent, func(c *Component) { c.declarations.aliases[0].coordinate = sourceZeroCoordinate }},
		{"alias.params", contentDeclarationsComponent, func(c *Component) { c.declarations.aliases[0].params.End = c.declarations.aliases[0].params.Start }},
		{"typeparam.owner", contentDeclarationsComponent, func(c *Component) { c.declarations.params[0].Owner = 0 }},
		{"typeparam.name", contentDeclarationsComponent, func(c *Component) { c.declarations.params[0].Name = 77 }},
		{"typeparam.constraint", contentDeclarationsComponent, func(c *Component) { c.declarations.params[0].Constraint = 0 }},
		{"interface.owner", contentDeclarationsComponent, func(c *Component) { c.declarations.interfaces[0].owner = 0 }},
		{"interface.name", contentDeclarationsComponent, func(c *Component) { c.declarations.interfaces[0].name = 77 }},
		{"interface.coordinate", contentDeclarationsComponent, func(c *Component) { c.declarations.interfaces[0].coordinate = sourceZeroCoordinate }},
		{"interface.extends-member", contentDeclarationsComponent, func(c *Component) { c.declarations.interfaceRefs[0] = 0 }},
		{"interface.extends-arity", contentDeclarationsComponent, func(c *Component) {
			c.declarations.interfaces[0].extends.End = c.declarations.interfaces[0].extends.Start
		}},
		{"interface.member-kind", contentDeclarationsComponent, func(c *Component) { c.declarations.members[0].kind = InterfaceMethod }},
		{"interface.member-field", contentDeclarationsComponent, func(c *Component) { c.declarations.members[0].field = 0 }},
		{"interface.member-name", contentDeclarationsComponent, func(c *Component) { c.declarations.members[1].name = 77 }},
		{"interface.member-coordinate", contentDeclarationsComponent, func(c *Component) { c.declarations.members[1].coordinate = sourceZeroCoordinate }},
		{"interface.member-signature", contentDeclarationsComponent, func(c *Component) { c.declarations.members[1].signature = 0 }},
		{"interface.members-order", contentDeclarationsComponent, swapInterfaceMembers},
		{"interface.members-arity", contentDeclarationsComponent, func(c *Component) {
			c.declarations.interfaces[0].members.End = c.declarations.interfaces[0].members.Start
		}},
		{"declared-type.cell", contentDeclaredTypeComponent, func(c *Component) { c.declarations.declaredTypes[0].cell = 0 }},
		{"declared-type.target", contentDeclaredTypeComponent, func(c *Component) { c.declarations.declaredTypes[0].target = 0 }},

		{"signature.scope", contentSignaturesComponent, func(c *Component) { c.signatures.functions[0].scope = 0 }},
		{"signature.type-param", contentSignaturesComponent, func(c *Component) { c.signatures.params[0] = 0 }},
		{"signature.type-param-arity", contentSignaturesComponent, func(c *Component) {
			c.signatures.functions[0].typeParams.End = c.signatures.functions[0].typeParams.Start
		}},
		{"signature.parameter.name", contentSignaturesComponent, func(c *Component) { c.signatures.fixed[0].name = 0 }},
		{"signature.parameter.coordinate", contentSignaturesComponent, func(c *Component) { c.signatures.fixed[0].coordinate = sourceZeroCoordinate }},
		{"signature.parameter.type", contentSignaturesComponent, func(c *Component) { c.signatures.fixed[0].typ = 0 }},
		{"signature.parameter-arity", contentSignaturesComponent, func(c *Component) {
			c.signatures.functions[0].parameters.End = c.signatures.functions[0].parameters.Start
		}},
		{"signature.variadic", contentSignaturesComponent, func(c *Component) { c.signatures.functions[0].variadic = 0 }},
		{"signature.variadic-coordinate", contentSignaturesComponent, func(c *Component) { c.signatures.functions[0].variadicCoord = sourceZeroCoordinate }},
		{"signature.returns-known", contentSignaturesComponent, func(c *Component) { c.signatures.functions[0].returnsKnown = false }},
		{"signature.return", contentSignaturesComponent, func(c *Component) { c.signatures.returns[0] = 0 }},
		{"signature.return-arity", contentSignaturesComponent, func(c *Component) { c.signatures.functions[0].returns.End = c.signatures.functions[0].returns.Start }},
		{"assertion.name", contentSignaturesComponent, func(c *Component) { c.signatures.assertions[0].Name = 0 }},
		{"assertion.coordinate", contentSignaturesComponent, func(c *Component) { c.signatures.assertions[0].ParamCoordinate = sourceZeroCoordinate }},
		{"assertion.bound", contentSignaturesComponent, func(c *Component) { c.signatures.assertions[0].Bound = false }},
		{"assertion.param", contentSignaturesComponent, func(c *Component) { c.signatures.assertions[0].Param = 1 }},
		{"assertion.narrow", contentSignaturesComponent, func(c *Component) { c.signatures.assertions[0].Narrow = 0 }},

		{"function-contract.type-param", contentContractsComponent, func(c *Component) { c.contracts.terms[c.contracts.functions[0].typeParams.Start] = 0 }},
		{"function-contract.type-param-arity", contentContractsComponent, func(c *Component) {
			c.contracts.functions[0].typeParams.End = c.contracts.functions[0].typeParams.Start
		}},
		{"function-contract.returns-known", contentContractsComponent, func(c *Component) { c.contracts.functions[0].returnsKnown = false }},
		{"function-contract.return", contentContractsComponent, func(c *Component) { c.contracts.terms[c.contracts.functions[0].returns.Start] = 0 }},
		{"function-contract.return-arity", contentContractsComponent, func(c *Component) { c.contracts.functions[0].returns.End = c.contracts.functions[0].returns.Start }},
		{"call-contract.argument", contentContractsComponent, func(c *Component) { c.contracts.terms[c.contracts.calls[0].Start] = 0 }},
		{"call-contract.argument-arity", contentContractsComponent, func(c *Component) { c.contracts.calls[0].End = c.contracts.calls[0].Start }},
		{"call-contract.argument-order", contentContractsComponent, swapCallTypeArguments},

		{"typeof.scope", contentOperatorsComponent, func(c *Component) { c.operators.typeOf[0].Scope = 0 }},
		{"typeof.operand", contentOperatorsComponent, func(c *Component) { c.operators.typeOf[0].Operand = 0 }},
		{"keyof.inner", contentOperatorsComponent, func(c *Component) { c.operators.keyOf[0].Inner = 0 }},
		{"index-access.object", contentOperatorsComponent, func(c *Component) { c.operators.indexAccess[0].Object = 0 }},
		{"index-access.index", contentOperatorsComponent, func(c *Component) { c.operators.indexAccess[0].Index = 0 }},
		{"conditional.check", contentOperatorsComponent, func(c *Component) { c.operators.conditional[0].Check = 0 }},
		{"conditional.extends", contentOperatorsComponent, func(c *Component) { c.operators.conditional[0].Extends = 0 }},
		{"conditional.then", contentOperatorsComponent, func(c *Component) { c.operators.conditional[0].Then = 0 }},
		{"conditional.else", contentOperatorsComponent, func(c *Component) { c.operators.conditional[0].Else = 0 }},

		{"claim.claim", contentOperandsComponent, func(c *Component) { c.operands.claims[0].claim = 0 }},
		{"claim.target", contentOperandsComponent, func(c *Component) { c.operands.claims[0].target = 0 }},
		{"type-value.target", contentOperandsComponent, func(c *Component) { c.operands.typeValues[0] = 0 }},
		{"annotation.scope", contentOperandsComponent, func(c *Component) { c.operands.annotations[0].Scope = 0 }},
		{"annotation.target", contentOperandsComponent, func(c *Component) { c.operands.annotations[0].Target = 0 }},
		{"annotation.name", contentOperandsComponent, func(c *Component) { c.operands.annotations[0].Name = 0 }},
		{"annotation.values", contentOperandsComponent, func(c *Component) { c.operands.annotations[0].Values = 0 }},
		{"annotation.order", contentOperandsComponent, swapAnnotations},

		{"publication.assign", contentPublicationsComponent, func(c *Component) { c.publications[0].assign = 0 }},
		{"publication.pair", contentPublicationsComponent, func(c *Component) { c.publications[0].pair = 1 }},
		{"publication.target", contentPublicationsComponent, func(c *Component) { c.publications[0].target = 0 }},
	}
	runContentDeltaLedger(t, cases)
}

// These close the remaining ordered-relation cases not exercised by the
// field deltas above. Sparse ClaimTarget is different: builder input order is
// not authored order, because its canonical relation is Flow Claim ordinal.
func TestStaticContentIDResidualOrderAndSparseClaimCanonicalization(t *testing.T) {
	t.Run("primitive rows", func(t *testing.T) {
		component := contentTypesComponent(t)
		before := contentID(component)
		component.types.primitive[0], component.types.primitive[1] = component.types.primitive[1], component.types.primitive[0]
		if after := contentID(component); after == before {
			t.Fatal("primitive row order omitted from ContentID")
		}
	})
	t.Run("reference rows", func(t *testing.T) {
		component := contentReferencesComponent(t)
		before := contentID(component)
		component.references.rows[0], component.references.rows[1] = component.references.rows[1], component.references.rows[0]
		if after := contentID(component); after == before {
			t.Fatal("TypeRef row order omitted from ContentID")
		}
	})
	t.Run("typeof rows", func(t *testing.T) {
		component := contentOperatorsComponent(t)
		component.operators.typeOf[1].Scope = 0 // make the two authored rows distinct before swapping them.
		before := contentID(component)
		component.operators.typeOf[0], component.operators.typeOf[1] = component.operators.typeOf[1], component.operators.typeOf[0]
		if after := contentID(component); after == before {
			t.Fatal("TypeOf row order omitted from ContentID")
		}
	})
	t.Run("publication rows", func(t *testing.T) {
		input := publicationFixture(t)
		input.Counts[keyspace.FamilyTypePublication] = 2
		input.Publications.Type = append(input.Publications.Type, Publication{
			Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: 1, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2),
		})
		first := staticContentComponent(t, input).Cold().ContentID()
		input.Publications.Type[0], input.Publications.Type[1] = input.Publications.Type[1], input.Publications.Type[0]
		if second := staticContentComponent(t, input).Cold().ContentID(); second == first {
			t.Fatal("TypePublication row order omitted from ContentID")
		}
	})
	t.Run("sparse claim input permutation", func(t *testing.T) {
		input := operandsFixture(t)
		input.Counts[keyspace.FamilyTypePrimitive] = 4
		input.Types.Primitive = append(input.Types.Primitive, Primitive{Kind: PrimitiveBoolean})
		input.Operands.Claim = append(input.Operands.Claim, ClaimTarget{
			Claim: keyspace.MakeTerm(keyspace.FamilyValueClaim, 2), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4),
		})
		first := staticContentComponent(t, input).Cold().ContentID()
		input.Operands.Claim[0], input.Operands.Claim[1] = input.Operands.Claim[1], input.Operands.Claim[0]
		if second := staticContentComponent(t, input).Cold().ContentID(); second != first {
			t.Fatalf("sparse ClaimTarget input order changed canonical ContentID: %x != %x", second, first)
		}
	})
}

var sourceZeroCoordinate = source.Coordinate{}

func contentReferencesComponent(t *testing.T) *Component {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeRef], counts[keyspace.FamilyTypeAlias], counts[keyspace.FamilyCell] = 3, 1, 1
	return staticContentComponent(t, referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{
		{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
		{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{7, 8}},
		{Resolution: TypeRefUnresolved, Source: []keyspace.Key{4, 5}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1)},
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

func swapInterfaceMembers(c *Component) {
	c.declarations.members[0], c.declarations.members[1] = c.declarations.members[1], c.declarations.members[0]
}
func swapReferenceSource(c *Component) {
	c.references.source[1], c.references.source[2] = c.references.source[2], c.references.source[1]
}
func swapReferenceCanonical(c *Component) {
	c.references.canonical[0], c.references.canonical[1] = c.references.canonical[1], c.references.canonical[0]
}
func swapCallTypeArguments(c *Component) {
	range_ := c.contracts.calls[0]
	c.contracts.terms[range_.Start], c.contracts.terms[range_.Start+1] = c.contracts.terms[range_.Start+1], c.contracts.terms[range_.Start]
}
func swapAnnotations(c *Component) {
	c.operands.annotations[0], c.operands.annotations[1] = c.operands.annotations[1], c.operands.annotations[0]
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
				return Input{Counts: counts, Types: TypesInput{Primitive: []Primitive{{Kind: PrimitiveNumber}}}}
			},
			mutate: func(_ *testing.T, input *Input) { input.Types.Primitive[0].Kind = PrimitiveString },
		},
		{
			name: "references",
			input: func(*testing.T) Input {
				counts := [keyspace.FamilyCount]uint32{}
				counts[keyspace.FamilyTypeRef] = 1
				return Input{Counts: counts, References: ReferencesInput{TypeRef: []TypeRef{{
					Resolution: TypeRefUnresolved, Source: []keyspace.Key{1},
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
			first := staticContentComponent(t, input).Cold().ContentID()
			if !first.Available() {
				t.Fatal("content identity unavailable")
			}
			if second := staticContentComponent(t, test.input(t)).Cold().ContentID(); second != first {
				t.Fatalf("equivalent rebuild ContentID = %x, want %x", second, first)
			}
			test.mutate(t, &input)
			if changed := staticContentComponent(t, input).Cold().ContentID(); changed == first {
				t.Fatal("authored vertical mutation left ContentID unchanged")
			}
		})
	}
}

func TestStaticContentIDExcludesDerivativesAndExternalClaimCardinality(t *testing.T) {
	input := operandsFixture(t)
	first := staticContentComponent(t, input)
	baseline := first.Cold().ContentID()

	// A Flow-only ValueClaim with no Static ClaimTarget must not change Static
	// identity just because it grows the retained dense query lookup.
	withoutTarget := input
	withoutTarget.Counts[keyspace.FamilyValueClaim]++
	if got := staticContentComponent(t, withoutTarget).Cold().ContentID(); got != baseline {
		t.Fatalf("external claim cardinality changed Static ContentID: %x != %x", got, baseline)
	}

	// These are deliberately internal mutations of derived query state. A
	// fresh hash must ignore them, proving the cached identity has no second
	// semantic authority behind it.
	first.operands.claimTargets[0] = 0
	first.operands.annotationTargets[0] = 0
	first.declarations.declaredByCell = append(first.declarations.declaredByCell, 0)
	if got := contentID(first); got != baseline {
		t.Fatalf("derived index changed Static ContentID: %x != %x", got, baseline)
	}
}

func TestStaticContentIDIsImmutableAndAllocationFree(t *testing.T) {
	input := publicationFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	// Build has copied every semantic slice before content identity becomes
	// observable. Mutating caller storage cannot change the Component hash.
	input.References.TypeRef[0].Source[0] = 99
	input.Publications.Type[0].Pair = 19
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := component.Cold().ContentID(), staticContentComponent(t, publicationFixture(t)).Cold().ContentID(); got != want {
		t.Fatalf("caller mutation after Build changed ContentID: %x != %x", got, want)
	}
	if allocations := testing.AllocsPerRun(1000, func() { _ = component.Cold().ContentID() }); allocations != 0 {
		t.Fatalf("Cold().ContentID allocations = %f, want 0", allocations)
	}
	if got := (Cold{}).ContentID(); got.Available() {
		t.Fatalf("zero Cold exposed identity: %x", got)
	}
}

func staticContentComponent(t *testing.T, input Input) *Component {
	t.Helper()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatal(err)
	}
	return component
}

func TestStaticContentIDTypesFieldLedger(t *testing.T) {
	cases := []contentDelta{
		{"primitive.kind", contentTypesComponent, func(c *Component) { c.types.primitive[0].Kind = PrimitiveString }},
		{"literal.kind", contentTypesComponent, func(c *Component) { c.types.literal[0].Kind = keyspace.LiteralInteger }},
		{"literal.exact", contentTypesComponent, func(c *Component) { c.types.literal[0].Exact = 77 }},
		{"literal.float-bits", contentTypesComponent, func(c *Component) { c.types.literal[0].FloatBits = 3 }},
		{"optional.inner", contentTypesComponent, func(c *Component) { c.types.optional[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) }},
		{"union.member", contentTypesComponent, func(c *Component) {
			c.types.terms[c.types.union[0].Start] = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3)
		}},
		{"union.arity", contentTypesComponent, func(c *Component) { c.types.union[0].End = c.types.union[0].Start }},
		{"union.order", contentTypesComponent, swapUnionMembers},
		{"intersection.member", contentTypesComponent, func(c *Component) {
			c.types.terms[c.types.intersection[0].Start] = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"intersection.arity", contentTypesComponent, func(c *Component) { c.types.intersection[0].End = c.types.intersection[0].Start }},
		{"intersection.order", contentTypesComponent, swapIntersectionMembers},
		{"generic.base", contentTypesComponent, func(c *Component) { c.types.generic[0].base = keyspace.MakeTerm(keyspace.FamilyTypeRef, 2) }},
		{"generic.arg", contentTypesComponent, func(c *Component) {
			c.types.terms[c.types.generic[0].args.Start] = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"generic.arity", contentTypesComponent, func(c *Component) { c.types.generic[0].args.End = c.types.generic[0].args.Start }},
		{"array.element", contentTypesComponent, func(c *Component) { c.types.array[0].Element = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) }},
		{"array.read-only", contentTypesComponent, func(c *Component) { c.types.array[0].ReadOnly = !c.types.array[0].ReadOnly }},
		{"map.key", contentTypesComponent, func(c *Component) { c.types.mapType[0].Key = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) }},
		{"map.value", contentTypesComponent, func(c *Component) { c.types.mapType[0].Value = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) }},
		{"map.read-only", contentTypesComponent, func(c *Component) { c.types.mapType[0].ReadOnly = !c.types.mapType[0].ReadOnly }},
		{"record.field", contentTypesComponent, func(c *Component) {
			c.types.fields[c.types.record[0].fields.Start] = keyspace.MakeTerm(keyspace.FamilyTypeField, 2)
		}},
		{"record.arity", contentTypesComponent, func(c *Component) { c.types.record[0].fields.End = c.types.record[0].fields.Start }},
		{"record.read-only", contentTypesComponent, func(c *Component) { c.types.record[0].readOnly = !c.types.record[0].readOnly }},
		{"field.key", contentTypesComponent, func(c *Component) { c.types.field[0].Key = 88 }},
		{"field.type", contentTypesComponent, func(c *Component) { c.types.field[0].Type = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) }},
		{"field.optional", contentTypesComponent, func(c *Component) { c.types.field[0].Optional = !c.types.field[0].Optional }},
	}
	runContentDeltaLedger(t, cases)
}

func contentTypesComponent(t *testing.T) *Component {
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
		Types: TypesInput{
			Primitive:    []Primitive{{Kind: PrimitiveNil}, {Kind: PrimitiveNumber}, {Kind: PrimitiveString}, {Kind: PrimitiveBoolean}, {Kind: PrimitiveNever}},
			Literal:      []Literal{{Kind: keyspace.LiteralString, Exact: 7}},
			Optional:     []Optional{{Inner: primitive(1)}},
			Union:        []Union{{Members: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1), primitive(2)}}},
			Intersection: []Intersection{{Members: []keyspace.Term{primitive(3), primitive(4)}}},
			Generic:      []Generic{{Base: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1), Args: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeUnion, 1)}}},
			Array:        []Array{{Element: keyspace.MakeTerm(keyspace.FamilyTypeGeneric, 1), ReadOnly: true}},
			Map:          []Map{{Key: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2), Value: keyspace.MakeTerm(keyspace.FamilyTypeArray, 1)}},
			Field:        []Field{{Key: 9, Type: keyspace.MakeTerm(keyspace.FamilyTypeMap, 1), Optional: true}},
			Record:       []Record{{Fields: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeField, 1)}, ReadOnly: true}},
		},
		References: ReferencesInput{TypeRef: []TypeRef{
			{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
			{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{4}},
		}},
		Declarations: DeclarationsInput{Alias: []TypeAlias{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: primitive(5), Name: 10, NameCoordinate: coordinate}}},
	}
	return staticContentComponent(t, input)
}

func swapUnionMembers(component *Component) {
	range_ := component.types.union[0]
	component.types.terms[range_.Start], component.types.terms[range_.Start+1] = component.types.terms[range_.Start+1], component.types.terms[range_.Start]
}

func swapIntersectionMembers(component *Component) {
	range_ := component.types.intersection[0]
	component.types.terms[range_.Start], component.types.terms[range_.Start+1] = component.types.terms[range_.Start+1], component.types.terms[range_.Start]
}
