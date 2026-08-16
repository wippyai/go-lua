package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

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
