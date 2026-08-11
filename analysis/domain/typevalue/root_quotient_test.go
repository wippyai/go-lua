package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestExistingRootQuotientAndFreshAllocationRoots(t *testing.T) {
	fixture := typeValueRootQuotientFixture(t)
	authority := typeValueAuthorityWithHeap(t, fixture.source, fixture.heaps)

	primitiveSeeds := make([]Seed, 0, 2)
	namedSeeds := make(map[keyspace.Term][]Seed)
	for index := 0; index < authority.SeedCount(); index++ {
		seed, ok := authority.SeedAt(index)
		if !ok {
			t.Fatalf("SeedAt(%d) absent", index)
		}
		descriptor, ok := authority.SeedDescriptor(seed)
		name, disposition, named := authority.DescriptorName(descriptor)
		if !ok || !named || disposition != NameExact {
			t.Fatalf("seed %d descriptor name = %q/%v/%v", index, name, disposition, named)
		}
		switch name {
		case "string":
			primitiveSeeds = append(primitiveSeeds, seed)
		case "Token":
			activation := typeValueSeedActivation(t, fixture.source, seed)
			namedSeeds[activation] = append(namedSeeds[activation], seed)
		}
	}
	if len(primitiveSeeds) != 2 {
		t.Fatalf("primitive string seeds = %d, want two source occurrences", len(primitiveSeeds))
	}
	if len(namedSeeds) != 3 {
		t.Fatalf("Token activations = %d, want chunk plus two functions", len(namedSeeds))
	}

	primitiveRepresentative := requireSeedRoot(t, authority, primitiveSeeds[0])
	firstPrimitiveSourceValue := requireSeedSourceValue(t, primitiveSeeds[0])
	if root, ok := authority.RootForValue(firstPrimitiveSourceValue); !ok || root != primitiveRepresentative {
		t.Fatal("first primitive seed did not retain its canonical existing Root representative")
	}
	for index, seed := range primitiveSeeds[1:] {
		if root := requireSeedRoot(t, authority, seed); root != primitiveRepresentative {
			t.Fatalf("primitive seed %d did not reuse the first canonical Root", index+1)
		}
		sourceValue := requireSeedSourceValue(t, seed)
		if root, ok := authority.RootForValue(sourceValue); !ok || root != primitiveRepresentative {
			t.Fatalf("primitive seed %d retained a pre-quotient TypeValue coordinate", index+1)
		}
	}

	activationRoots := make(map[uint32]keyspace.Term, len(namedSeeds))
	for activation, seeds := range namedSeeds {
		if len(seeds) == 0 {
			t.Fatalf("Token activation %v has no seeds", activation)
		}
		representative := requireSeedRoot(t, authority, seeds[0])
		ordinal, ok := authority.RootIndex(representative)
		if !ok {
			t.Fatal("named representative lacks Root ordinal")
		}
		if prior, duplicate := activationRoots[ordinal]; duplicate && prior != activation {
			t.Fatal("same-name Token seeds from different function activations were collapsed")
		}
		activationRoots[ordinal] = activation
		for index, seed := range seeds {
			if root := requireSeedRoot(t, authority, seed); root != representative {
				t.Fatalf("Token seed %d in activation %v did not reuse its representative", index, activation)
			}
			if index == 0 {
				continue
			}
			sourceValue := requireSeedSourceValue(t, seed)
			if root, ok := authority.RootForValue(sourceValue); !ok || root != representative {
				t.Fatal("repeated Token source occurrence retained a pre-quotient TypeValue coordinate")
			}
		}
	}
	if len(activationRoots) != len(namedSeeds) {
		t.Fatal("distinct named activations did not retain distinct representatives")
	}
	usedRoots := make(map[uint32]struct{})
	for index := 0; index < fixture.source.Boundary().Values().Count(); index++ {
		value, ok := fixture.source.Boundary().Values().At(index)
		root, found := authority.RootForValue(value)
		ordinal, indexed := authority.RootIndex(root)
		if !ok || !found || !indexed {
			t.Fatalf("Link Value %d lost its quotient image", index)
		}
		usedRoots[ordinal] = struct{}{}
	}
	for index := 0; index < fixture.heaps.KeyCount(); index++ {
		allocationRoot, ok := fixture.heaps.KeyAt(index)
		if !ok || allocationRoot.Kind() != heap.RootAllocation {
			continue
		}
		root, found := authority.RootForHeapKey(allocationRoot)
		ordinal, indexed := authority.RootIndex(root)
		if !ok || !found || !indexed {
			t.Fatalf("allocation root %d lost its quotient image", index)
		}
		usedRoots[ordinal] = struct{}{}
	}
	if authority.RootCount() != len(usedRoots) {
		t.Fatalf("Root authority retained %d unreachable pre-quotient cells", authority.RootCount()-len(usedRoots))
	}

}

func typeValueSeedActivation(t testing.TB, source *link.Link, seed Seed) keyspace.Term {
	t.Helper()
	value := requireSeedSourceValue(t, seed)
	shard, term, ok := source.Boundary().Values().Origin(value)
	if !ok {
		t.Fatal("seed Value lacks origin")
	}
	p, ok := source.Project().Mounts().Program(shard)
	if !ok {
		t.Fatal("seed origin lacks Program")
	}
	body, _, _, ok := p.Source().Index().Position(term)
	if !ok {
		t.Fatal("seed origin lacks SourceIndex")
	}
	activation, ok := p.Flow().Activation().For(body)
	if !ok {
		t.Fatal("seed body lacks activation")
	}
	return activation
}

func requireSeedRoot(t testing.TB, authority *Authority, seed Seed) Root {
	t.Helper()
	root, ok := authority.SeedRoot(seed)
	if !ok {
		t.Fatal("seed lacks TypeValue Root")
	}
	return root
}

func requireSeedSourceValue(t testing.TB, seed Seed) linkboundary.Value {
	t.Helper()
	value, ok := typeValueSeedSource(t, seed)
	if !ok {
		t.Fatal("seed lacks Link Value source")
	}
	return value
}

func typeValueSeedSource(t testing.TB, seed Seed) (linkboundary.Value, bool) {
	t.Helper()
	if seed.owner == nil {
		return linkboundary.Value{}, false
	}
	return seed.owner.SeedSource(seed)
}

type rootQuotientFixture struct {
	source *link.Link
	heaps  heap.Schema
}

func typeValueRootQuotientFixture(t testing.TB) *rootQuotientFixture {
	t.Helper()
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{{
			Root:       "GlobalEnvRoot",
			Key:        keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"},
			Value:      target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			Mutability: target.InitialMutable,
		}, {
			Root:       "GlobalEnvRoot",
			Key:        keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__typevalue_absent"},
			Value:      target.InitialValueSpec{Kind: target.InitialValueAbsent},
			Mutability: target.InitialMutable,
		}},
		InitialBindings: []target.InitialBindingSpec{{
			Name: "_G",
			Root: "GlobalEnvRoot",
			Key:  keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"},
		}},
		Operations: []target.OperationSpec{
			{
				Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"op"}}},
				Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
				Outcomes: []target.OutcomeSpec{{
					Kind:   kind.OutcomeNormal,
					Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
					FreshResults: []target.FreshResultSpec{{
						Result: 0, Kind: target.FreshFunction,
					}},
					Produced: []target.ProducedSpec{{
						Result: 0, Operation: target.SpecRef(2),
						Captures: []target.CaptureSpec{{Kind: target.CaptureTypeValueFormal, Ordinal: 0}},
					}},
				}},
				Effects: target.RowSpec{Tail: target.RowClosed},
			},
			{
				Input:    target.ValuesSpec{Tail: target.ValuesClosed},
				Outcomes: []target.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
				Effects:  target.RowSpec{Tail: target.RowClosed},
			},
		}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := programlower.Lower(programlower.Source{
		Name: "typevalue_root_quotient",
		Text: []byte(`
type Token = { value: string }
string("primitive first")
string("primitive second")
Token({})
Token({})
local function first()
	Token({})
	Token({})
end
local function second()
	Token({})
end
first()
second()
op("capture")
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "typevalue_root_quotient", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(source)
	if !heapsOK {
		t.Fatal("Heap seal")
	}
	return &rootQuotientFixture{source: source, heaps: heaps}
}
