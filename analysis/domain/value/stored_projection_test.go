package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestStoredProjectionPartitionsTheSealedValueUniverse proves the complete
// local partition consumed by stored reads. It intentionally builds one
// source of every relevant reference family: a raw allocation occurrence,
// tracked allocation roles, a boot root, host endpoint, bootstrap callable,
// runtime TypeValue, and opaque reference families.
func TestStoredProjectionPartitionsTheSealedValueUniverse(t *testing.T) {
	schema, linked := storedProjectionFixture(t, false)
	capability, capabilityOK := linked.Host().Capabilities().At(0)
	if !capabilityOK {
		t.Fatal("stored projection fixture has no capability")
	}

	none, unknown, exact := schema.Bottom(), schema.Bottom(), schema.Bottom()
	var exactAtoms []Atom
	sources := make(map[referenceSource]bool)
	for index := 0; index < schema.AtomCount(); index++ {
		atom := Atom{schema: schema, id: uint32(index + 1)}
		input := mustCorrelatedSingleton(t, schema, atom)
		input, inputOK := schema.WithCapability(input, atom, capability)
		if !inputOK {
			t.Fatalf("capability attach for atom %d", index)
		}
		if row := schema.atoms[index]; row.kind == atomReference {
			sources[schema.references[row.reference-1].source] = true
		}

		noneResult, noneOK := schema.FilterStoredNone(input)
		unknownResult, unknownOK := schema.FilterStoredUnknown(input)
		exactResult, exactOK := schema.FilterStoredExact(input, atom)
		switch {
		case schema.storedExactReference(schema.atoms[index]):
			exactAtoms = append(exactAtoms, atom)
			if !noneOK || !unknownOK || !exactOK || !schema.Equal(noneResult, schema.Bottom()) || !schema.Equal(unknownResult, schema.Bottom()) || !schema.Equal(exactResult, input) || !schema.HasCapability(exactResult, atom, capability) {
				t.Fatalf("tracked stored atom %d did not remain precise", index)
			}
			var joined bool
			exact, joined = schema.Join(exact, exactResult)
			if !joined {
				t.Fatalf("join tracked stored atom %d", index)
			}
		case schema.storedUnknownReference(uint32(index + 1)):
			if !noneOK || !unknownOK || exactOK || !schema.Equal(noneResult, schema.Bottom()) || !schema.Equal(unknownResult, input) || !schema.HasCapability(unknownResult, atom, capability) {
				t.Fatalf("unknown stored atom %d was dropped or made tracked", index)
			}
			var joined bool
			unknown, joined = schema.Join(unknown, unknownResult)
			if !joined {
				t.Fatalf("join unknown stored atom %d", index)
			}
		default:
			if !noneOK || !unknownOK || exactOK || !schema.Equal(noneResult, input) || !schema.Equal(unknownResult, schema.Bottom()) || !schema.HasCapability(noneResult, atom, capability) {
				t.Fatalf("non-reference stored atom %d gained a child edge", index)
			}
			var joined bool
			none, joined = schema.Join(none, noneResult)
			if !joined {
				t.Fatalf("join non-reference stored atom %d", index)
			}
		}
	}

	for _, source := range []referenceSource{
		referenceSourceAllocation,
		referenceSourceBoot,
		referenceSourceEndpoint,
		referenceSourceCallable,
		referenceSourceRuntimeType,
	} {
		if !sources[source] {
			t.Fatalf("fixture omitted reference source %d", source)
		}
	}
	if len(exactAtoms) == 0 || schema.Equal(none, schema.Bottom()) || schema.Equal(unknown, schema.Bottom()) {
		t.Fatal("stored class denominator is incomplete")
	}
	if storedProjectionOverlaps(schema, none, unknown) || storedProjectionOverlaps(schema, none, exact) || storedProjectionOverlaps(schema, unknown, exact) {
		t.Fatal("stored projection classes overlap")
	}
	covered, coveredOK := schema.Join(none, unknown)
	if !coveredOK {
		t.Fatal("join none/unknown")
	}
	covered, coveredOK = schema.Join(covered, exact)
	if !coveredOK || !schema.Equal(covered, schema.Top()) {
		t.Fatal("stored projection classes do not cover Top")
	}

	fromTopNone, noneTopOK := schema.FilterStoredNone(schema.Top())
	fromTopUnknown, unknownTopOK := schema.FilterStoredUnknown(schema.Top())
	if !noneTopOK || !unknownTopOK || !schema.Equal(fromTopNone, none) || !schema.Equal(fromTopUnknown, unknown) {
		t.Fatal("Top lost the stored projection partition")
	}
	for _, atom := range exactAtoms {
		got, gotOK := schema.FilterStoredExact(schema.Top(), atom)
		want, wantOK := schema.FilterStoredExact(exact, atom)
		if !gotOK || !wantOK || !schema.Equal(got, want) || !schema.HasCapability(got, atom, capability) {
			t.Fatalf("Top lost exact stored atom %d", atom.id)
		}
	}
}

func TestStoredProjectionRejectsForeignValueAndSelectorOwners(t *testing.T) {
	schema, _ := storedProjectionFixture(t, false)
	foreign, _ := storedProjectionFixture(t, false)
	localExact := storedProjectionExactAtom(t, schema)
	foreignExact := storedProjectionExactAtom(t, foreign)
	if _, ok := schema.FilterStoredNone(foreign.Top()); ok {
		t.Fatal("foreign Value entered FilterStoredNone")
	}
	if _, ok := schema.FilterStoredUnknown(foreign.Top()); ok {
		t.Fatal("foreign Value entered FilterStoredUnknown")
	}
	if _, ok := schema.FilterStoredExact(foreign.Top(), foreignExact); ok {
		t.Fatal("foreign Value and selector entered FilterStoredExact")
	}
	if _, ok := schema.FilterStoredExact(schema.Top(), foreignExact); ok {
		t.Fatal("foreign selector entered FilterStoredExact")
	}
	if _, ok := schema.FilterStoredExact(schema.Top(), Atom{}); ok {
		t.Fatal("zero selector entered FilterStoredExact")
	}
	if _, ok := schema.FilterStoredExact(schema.Top(), localExact); !ok {
		t.Fatal("local selector stopped entering FilterStoredExact")
	}
}

func TestStoredProjectionIsCanonicalUnderModulePermutation(t *testing.T) {
	left, leftLink := storedProjectionFixture(t, false)
	right, rightLink := storedProjectionFixture(t, true)
	if leftLink.ContentID() != rightLink.ContentID() || left.AtomCount() != right.AtomCount() {
		t.Fatal("module permutation changed the sealed Value vocabulary")
	}
	leftNone, leftNoneOK := left.FilterStoredNone(left.Top())
	leftUnknown, leftUnknownOK := left.FilterStoredUnknown(left.Top())
	rightNone, rightNoneOK := right.FilterStoredNone(right.Top())
	rightUnknown, rightUnknownOK := right.FilterStoredUnknown(right.Top())
	if !leftNoneOK || !leftUnknownOK || !rightNoneOK || !rightUnknownOK ||
		left.Fingerprint(leftNone) != right.Fingerprint(rightNone) ||
		left.Fingerprint(leftUnknown) != right.Fingerprint(rightUnknown) {
		t.Fatal("module permutation changed stored projection layout")
	}
	for index := 0; index < left.AtomCount(); index++ {
		leftExact := left.storedExactReference(left.atoms[index])
		rightExact := right.storedExactReference(right.atoms[index])
		leftUnknown := left.storedUnknownReference(uint32(index + 1))
		rightUnknown := right.storedUnknownReference(uint32(index + 1))
		if leftExact != rightExact || leftUnknown != rightUnknown {
			t.Fatalf("module permutation changed stored class at atom %d", index)
		}
	}
}

func TestStoredProjectionWarmReductionsDoNotAllocate(t *testing.T) {
	schema, linked := storedProjectionFixture(t, false)
	capability, capabilityOK := linked.Host().Capabilities().At(0)
	exact := storedProjectionExactAtom(t, schema)
	input, inputOK := schema.WithCapability(mustCorrelatedSingleton(t, schema, exact), exact, capability)
	if !capabilityOK || !inputOK {
		t.Fatal("stored projection warm fixture")
	}
	var sink Value
	if allocations := testing.AllocsPerRun(1_000, func() {
		var ok bool
		if sink, ok = schema.FilterStoredNone(schema.Top()); !ok {
			t.Fatal("FilterStoredNone Top")
		}
		if sink, ok = schema.FilterStoredUnknown(schema.Top()); !ok {
			t.Fatal("FilterStoredUnknown Top")
		}
		if sink, ok = schema.FilterStoredExact(schema.Top(), exact); !ok {
			t.Fatal("FilterStoredExact Top")
		}
		if sink, ok = schema.FilterStoredExact(input, exact); !ok {
			t.Fatal("FilterStoredExact finite")
		}
	}); allocations != 0 {
		t.Fatalf("stored projection warm allocations = %v, want 0", allocations)
	}
	if !schema.HasCapability(sink, exact, capability) {
		t.Fatal("warm result lost exact capability")
	}
}

func storedProjectionOverlaps(schema *Schema, left, right Value) bool {
	if schema == nil || !schema.owns(left) || !schema.owns(right) || left.top || right.top {
		return true
	}
	stride := schema.stride()
	leftAt, rightAt := 0, 0
	for leftAt < len(left.image) && rightAt < len(right.image) {
		leftID, rightID := left.image[leftAt], right.image[rightAt]
		switch {
		case leftID < rightID:
			leftAt += stride
		case leftID > rightID:
			rightAt += stride
		default:
			return true
		}
	}
	return false
}

func storedProjectionExactAtom(t testing.TB, schema *Schema) Atom {
	t.Helper()
	for index := range schema.atoms {
		if schema.storedExactReference(schema.atoms[index]) {
			return Atom{schema: schema, id: uint32(index + 1)}
		}
	}
	t.Fatal("missing tracked stored atom")
	return Atom{}
}

func storedProjectionFixture(t testing.TB, reverse bool) (*Schema, *link.Link) {
	t.Helper()
	alpha, err := programlower.Lower(programlower.Source{Name: "stored_projection_alpha.lua", Text: []byte(`
type Box = { value: number }
local typed = Box("payload")
local object = { child = {} }
actor.send(1)
return object, typed
`)})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := programlower.Lower(programlower.Source{Name: "stored_projection_beta.lua", Text: []byte(`
local other = {}
return other, "beta"
`)})
	if err != nil {
		t.Fatal(err)
	}
	actorBinding := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	admittedBinding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"admitted"}}
	operation := func(binding target.BindingSpec) target.OperationSpec {
		return target.OperationSpec{
			Bindings: []target.BindingSpec{binding},
			Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape:    target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}},
		}},
		Operations: []target.OperationSpec{operation(actorBinding), operation(admittedBinding)},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: storedProjectionLiteral("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: storedProjectionLiteral("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: storedProjectionLiteral("admitted"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: admittedBinding}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: storedProjectionLiteral("_G")},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: storedProjectionLiteral("__link_absent")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	modules := []linkproject.Module{{Name: "alpha", Program: alpha}, {Name: "beta", Program: beta}}
	if reverse {
		modules[0], modules[1] = modules[1], modules[0]
	}
	actorRead := storedProjectionGlobalRead(t, alpha, "actor")
	memberRead := storedProjectionMemberRead(t, alpha)
	linked, err := link.Seal(&link.Spec{
		Target:           contract,
		Modules:          modules,
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "actor.send", Binding: actorBinding}},
		Host: linkhost.Spec{
			ProviderCapabilities: []linkhost.ProviderCapabilitySpec{
				{Identity: "stored"},
			},
			ProviderCapabilitySeeds: []linkhost.ProviderCapabilitySeedSpec{{
				Capability: "stored", Source: linkhost.ProviderCapabilitySourceExposure, Module: "alpha", Access: actorRead,
			}},
			Members: []linkhost.HostMemberSpec{{
				Module: "alpha", Access: memberRead, Capability: "stored", Endpoint: "actor.send", Dispatch: linkhost.HostDispatchLookup,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("stored projection Value schema")
	}
	if _, ok := schema.OpaqueKind(runtimekind.Table); !ok {
		t.Fatal("stored projection fixture lacks opaque table class")
	}
	if _, ok := schema.OpaqueReference(ReferenceTable); !ok {
		t.Fatal("stored projection fixture lacks opaque reference class")
	}
	return schema, linked
}

func storedProjectionLiteral(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}

func storedProjectionGlobalRead(t testing.TB, p *program.Program, name string) keyspace.Term {
	return sourceSeedGlobalRead(t, p, name)
}

func storedProjectionMemberRead(t testing.TB, p *program.Program) keyspace.Term {
	return sourceSeedMemberRead(t, p)
}
