package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestExistingRootQuotientAndFreshAllocationRoots(t *testing.T) {
	fixture := typeValueRootQuotientFixture(t)
	authority := fixture.authority

	primitiveSeeds := make([]Seed, 0, 2)
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
		if name == "string" {
			primitiveSeeds = append(primitiveSeeds, seed)
		}
	}
	if len(primitiveSeeds) != 2 {
		t.Fatalf("primitive string seeds = %d, want two source occurrences", len(primitiveSeeds))
	}
	primitiveRepresentative, ok := authority.SeedRoot(primitiveSeeds[0])
	if !ok {
		t.Fatal("primitive representative root")
	}
	for index, seed := range primitiveSeeds {
		root, rootOK := authority.SeedRoot(seed)
		seedID, seedIDOK := authority.SeedID(seed)
		mapped, mappedOK := authority.RootForValueIdentity(seedID)
		if !rootOK || !seedIDOK || !mappedOK || mapped != root {
			t.Fatalf("primitive seed %d lost its canonical source-root image", index)
		}
		if root != primitiveRepresentative {
			t.Fatalf("primitive seed %d did not reuse the first canonical Root", index)
		}
	}

	allocationIDs := make(map[keyspace.ContentID]struct{})
	for index := 0; index < fixture.heaps.KeyCount(); index++ {
		allocation, ok := fixture.heaps.KeyAt(index)
		if !ok || allocation.Kind() != heap.RootAllocation {
			continue
		}
		id, idOK := fixture.heaps.KeyID(allocation)
		if !idOK {
			t.Fatalf("allocation root %d lost its receipt identity", index)
		}
		allocationIDs[id] = struct{}{}
	}
	for id := range allocationIDs {
		found := false
		for index := 0; index < authority.RootCount(); index++ {
			root, ok := authority.RootAt(index)
			if !ok {
				t.Fatalf("RootAt(%d)", index)
			}
			if rootID, ok := authority.FreshRootID(root); ok && rootID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("allocation root lost its canonical TypeValue coordinate")
		}
	}
}

type rootQuotientFixture struct {
	source    *link.Link
	heaps     heap.Schema
	authority *Authority
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
		},
	})
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
	statics, heaps := sealTypeValueAuthorities(t, source, contract)
	authority, ok := New(statics, heaps)
	if !ok {
		t.Fatal("TypeValue New rejected quotient fixture")
	}
	return &rootQuotientFixture{source: source, heaps: heaps, authority: authority}
}
