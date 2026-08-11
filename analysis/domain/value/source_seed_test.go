package value

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	programflow "github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestSourceSeedDirectlyEnumeratesCanonicalValues(t *testing.T) {
	schema, linked := correlatedFixture(t, `
type Box = { value: number }
local box = Box("payload")
local table = { value = 1 }
return nil, false, true, 1, 2.5, "text", box, table
`, false)
	seen := make(map[keyspace.Family]int)
	runtimeSeeds := 0
	values := linked.Boundary().Values()
	for index := 0; index < values.Count(); index++ {
		value, ok := values.At(index)
		if !ok {
			t.Fatalf("ValueAt(%d)", index)
		}
		family, _, relationOK := schema.sourceLiteral(value)
		literal := relationOK
		runtime := schema.typeRefs[value] != 0
		want := literal || runtime
		seed, admitted := schema.SourceSeed(value)
		at, atOK := schema.SourceSeedAt(index)
		if admitted != want || atOK != want || admitted && at != seed {
			t.Fatalf("value %d literal=%t runtime=%t seed=%t at=%t", index, literal, runtime, admitted, atOK)
		}
		if !want {
			continue
		}
		coordinate, fact, factOK := seed.Result()
		direct, directOK := schema.SourceValue(value)
		id, idOK := seed.ID()
		valueID, valueIDOK := values.ID(value)
		shard, term, originOK := seed.Origin()
		wantShard, wantTerm, wantOrigin := values.Origin(value)
		if !factOK || !directOK || !schema.Same(fact, direct) || coordinate == (Coordinate{}) ||
			!idOK || !valueIDOK || id != valueID || !originOK || !wantOrigin || shard != wantShard || term != wantTerm || schema.Equal(fact, schema.Bottom()) {
			t.Fatalf("value %d failed direct source reconstruction", index)
		}
		if literal {
			seen[family]++
		}
		if runtime {
			runtimeSeeds++
		}
	}
	for _, family := range []keyspace.Family{
		keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString,
	} {
		if seen[family] == 0 {
			t.Fatalf("literal family %d has no source seed", family)
		}
	}
	if runtimeSeeds == 0 {
		t.Fatal("fixture lacks a direct runtime-TypeValue seed")
	}
	for _, key := range allocationKeys(t, schema) {
		shard, term, _, programAllocation := key.ProgramAllocation()
		if !programAllocation {
			continue
		}
		value, ok := values.Of(shard, term)
		if !ok {
			t.Fatal("Program allocation lost its Link Value")
		}
		if _, ok := schema.SourceSeed(value); ok {
			t.Fatal("allocation Value acquired an unconditional source seed")
		}
	}
}

func TestSchemaDirectlyTraversesContextualOwnerRelations(t *testing.T) {
	schema, linked := contextualSourceSeedFixture(t)
	if len(schema.bootRefs) != linked.Host().BootRoots().Count() || len(schema.endpointRefs) != linked.Boundary().Endpoints().Count() ||
		schema.CapabilitySeedCount() != linked.Host().CapabilitySeeds().Count() || schema.HostMemberCount() != linked.Host().Members().Count() {
		t.Fatal("Value did not retain every direct Link-owned contextual relation")
	}
	endpoints := linked.Boundary().Endpoints()
	if endpoints.Count() != 2 {
		t.Fatalf("endpoint count = %d, want 2", endpoints.Count())
	}
	left, leftOK := endpoints.At(0)
	right, rightOK := endpoints.At(1)
	leftOperation, leftOperationOK := endpoints.Operation(left)
	rightOperation, rightOperationOK := endpoints.Operation(right)
	if !leftOK || !rightOK || !leftOperationOK || !rightOperationOK || leftOperation != rightOperation || left == right {
		t.Fatal("same-operation endpoints lost their nominal distinction")
	}
	leftAtom, leftAtomOK := schema.Endpoint(left)
	rightAtom, rightAtomOK := schema.Endpoint(right)
	if !leftAtomOK || !rightAtomOK || leftAtom == rightAtom {
		t.Fatal("Value collapsed distinct nominal endpoints")
	}
	leftReference, _, leftReferenceOK := leftAtom.Reference()
	rightReference, _, rightReferenceOK := rightAtom.Reference()
	projectedLeft, projectedLeftOK := leftReference.Endpoint()
	projectedRight, projectedRightOK := rightReference.Endpoint()
	if !leftReferenceOK || !rightReferenceOK || !projectedLeftOK || !projectedRightOK || projectedLeft != left || projectedRight != right {
		t.Fatal("endpoint reference did not preserve Boundary endpoint identity")
	}
	for index := 0; index < schema.HostMemberCount(); index++ {
		member, ok := schema.HostMemberAt(index)
		if !ok {
			t.Fatalf("HostMemberAt(%d)", index)
		}
		output, ok := member.Output()
		if !ok {
			t.Fatal("host member lost its output Value")
		}
		if _, ok := schema.SourceSeed(output); ok {
			t.Fatal("host member output became an unconditional source")
		}
		endpoint, endpointOK := member.Endpoint()
		if !endpointOK {
			t.Fatal("host member lost its nominal Boundary endpoint")
		}
		if operation, operationOK := endpoints.Operation(endpoint); !operationOK || operation != leftOperation {
			t.Fatal("host member endpoint did not revalidate through Boundary")
		}
	}
	for index := 0; index < schema.CapabilitySeedCount(); index++ {
		seed, ok := schema.CapabilitySeedAt(index)
		if !ok {
			t.Fatalf("CapabilitySeedAt(%d)", index)
		}
		if coordinate, exposed := seed.Exposure(); exposed {
			value, ok := schema.CoordinateAt(int(coordinate.index - 1))
			if !ok {
				t.Fatal("capability exposure lost its coordinate")
			}
			_ = value
		}
	}
}

func TestSchemaRejectsForeignBoundaryEndpoints(t *testing.T) {
	schema, linked := contextualSourceSeedFixture(t)
	foreignSchema, foreign := contextualSourceSeedFixture(t)
	endpoint, endpointOK := linked.Boundary().Endpoints().At(0)
	foreignEndpoint, foreignEndpointOK := foreign.Boundary().Endpoints().At(0)
	if !endpointOK || !foreignEndpointOK {
		t.Fatal("endpoint unavailable")
	}
	if _, ok := schema.Endpoint(foreignEndpoint); ok {
		t.Fatal("foreign equivalent Boundary endpoint crossed Value's owner fence")
	}
	if _, ok := foreignSchema.Endpoint(endpoint); ok {
		t.Fatal("local endpoint crossed into an equivalent foreign Value schema")
	}
}

func TestSourceSeedFailsClosedForZeroAndForeignValues(t *testing.T) {
	schema, linked := correlatedFixture(t, "return 1", false)
	foreignSchema, foreignLink := correlatedFixture(t, "return 1", false)
	var zero SourceSeed
	if _, ok := zero.ID(); ok {
		t.Fatal("zero SourceSeed acquired an identity")
	}
	if _, _, ok := zero.Result(); ok {
		t.Fatal("zero SourceSeed acquired a result")
	}
	if _, _, ok := zero.Origin(); ok {
		t.Fatal("zero SourceSeed acquired an authored origin")
	}
	if _, ok := schema.SourceSeed(linkboundary.Value{}); ok {
		t.Fatal("zero Link Value acquired a source seed")
	}
	if _, ok := (*Schema)(nil).SourceSeedAt(0); ok {
		t.Fatal("nil Schema issued a source seed")
	}
	foreign, ok := foreignLink.Boundary().Values().At(0)
	if !ok {
		t.Fatal("foreign Value")
	}
	if _, ok := schema.SourceSeed(foreign); ok {
		t.Fatal("foreign Value crossed the Schema owner fence")
	}
	local, ok := linked.Boundary().Values().At(0)
	if !ok {
		t.Fatal("local Value")
	}
	if _, ok := schema.SourceSeed(local); !ok {
		t.Fatal("local literal Value did not acquire a source seed")
	}
	if _, ok := foreignSchema.SourceSeed(local); ok {
		t.Fatal("local Value crossed the foreign Schema owner fence")
	}
	seed, ok := schema.SourceSeed(local)
	if !ok {
		t.Fatal("local source seed")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _, _ = seed.Origin()
	}); allocations != 0 {
		t.Fatalf("SourceSeed.Origin allocations = %g, want 0", allocations)
	}
}

func TestSourceSeedFloatFallbackKeepsOnlyValueOwnedObservations(t *testing.T) {
	schema, _ := correlatedFixture(t, "return 2.5", false)
	primitive, primitiveOK := schema.sourceFloatAtom(2.5)
	positiveZero, positiveZeroOK := schema.sourceFloatAtom(0)
	nanA, nanAOK := schema.sourceFloatAtom(math.Float64frombits(0x7ff8000000000001))
	nanB, nanBOK := schema.sourceFloatAtom(math.Float64frombits(0x7ff80000000000ff))
	negativeZero, negativeZeroOK := schema.sourceFloatAtom(math.Copysign(0, -1))
	if !primitiveOK || !positiveZeroOK || !nanAOK || !nanBOK || !negativeZeroOK {
		t.Fatal("float atom classification")
	}
	if primitive != positiveZero || primitive != negativeZero || nanA != nanB || primitive == nanA {
		t.Fatal("float source precision classes collapsed or leaked payload identity")
	}
	if schema.atoms[primitive-1].kind != atomPrimitive || schema.atoms[nanA-1].kind != atomNaN {
		t.Fatal("float source selected the wrong existing Number atom")
	}
	for _, atomID := range []uint32{nanA, nanB, negativeZero} {
		fact, ok := schema.Singleton(Atom{schema: schema, id: atomID})
		if !ok || schema.Presence(fact) != PresencePresent || !schema.RuntimeKinds(fact).Contains(runtimekind.Number) || schema.Truthiness(fact) != TruthTrue {
			t.Fatalf("opaque float projection = presence:%b kinds:%b truth:%b", schema.Presence(fact), schema.RuntimeKinds(fact), schema.Truthiness(fact))
		}
	}
}

func sourceSeedProgram(t testing.TB, name, text string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func contextualSourceSeedFixture(t testing.TB) (*Schema, *link.Link) {
	t.Helper()
	p := sourceSeedProgram(t, "source_seed_contextual.lua", "actor.send(1)")
	binding := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		Operations:   []target.OperationSpec{{Bindings: []target.BindingSpec{binding}, Input: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	actorRead := sourceSeedGlobalRead(t, p, "actor")
	memberRead := sourceSeedMemberRead(t, p)
	linked, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "main", Program: p}},
		EndpointRequests: []linkboundary.EndpointRequest{
			{Identity: "actor.send", Binding: binding},
			{Identity: "actor.send.again", Binding: binding},
		},
		Host: linkhost.Spec{
			ProviderCapabilities: []linkhost.ProviderCapabilitySpec{{Identity: "actor"}, {Identity: "boot"}},
			ProviderCapabilitySeeds: []linkhost.ProviderCapabilitySeedSpec{
				{Capability: "actor", Source: linkhost.ProviderCapabilitySourceExposure, Module: "main", Access: actorRead},
				{Capability: "boot", Source: linkhost.ProviderCapabilitySourceInitialRoot, InitialRoot: "GlobalEnvRoot"},
			},
			Members: []linkhost.HostMemberSpec{{Module: "main", Access: memberRead, Capability: "actor", Endpoint: "actor.send", Dispatch: linkhost.HostDispatchLookup}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("contextual Value schema")
	}
	return schema, linked
}

func sourceSeedGlobalRead(t testing.TB, p *program.Program, name string) keyspace.Term {
	t.Helper()
	reads := p.Flow().Authored().Storage().Reads()
	cells := p.Flow().Authored().Storage().Cells()
	for index := 0; index < reads.Count(); index++ {
		read, ok := reads.At(index)
		if !ok {
			continue
		}
		_, source, _, ok := reads.Get(read)
		if !ok {
			continue
		}
		kind, _, key, ok := cells.Get(source)
		literal, literalOK := p.Source().Keys().Exact(key)
		if ok && kind == programflow.CellGlobal && literalOK && literal.Kind == keyspace.LiteralString && literal.String == name {
			return read
		}
	}
	t.Fatalf("missing global Read %q", name)
	return 0
}

func sourceSeedMemberRead(t testing.TB, p *program.Program) keyspace.Term {
	t.Helper()
	reads := p.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.Count(); index++ {
		read, ok := reads.At(index)
		if !ok {
			continue
		}
		_, source, _, ok := reads.Get(read)
		if !ok {
			continue
		}
		_, _, keySource, fieldKind, exactOK := p.Flow().Authored().Access().Exact().Get(source)
		if exactOK && (fieldKind == flowkind.FieldName || fieldKind == flowkind.FieldExact) && programExactKey(p, keySource) {
			return read
		}
	}
	t.Fatal("missing member Read")
	return 0
}

func programExactKey(p *program.Program, term keyspace.Term) bool {
	source := p.Source()
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyKey:
		_, _, key, ok := source.Keys().Name(term)
		if ok {
			return key != 0
		}
		_, _, key, ok = source.Keys().List(term)
		return ok && key != 0
	case keyspace.FamilyNil:
		return false
	case keyspace.FamilyBool:
		actual, _, value, ok := source.Literals().Bools().At(int(keyspace.TermOrdinal(term)) - 1)
		if !ok || actual != term {
			return false
		}
		_, ok = source.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value})
		return ok
	case keyspace.FamilyInteger:
		actual, _, value, ok := source.Literals().Integers().At(int(keyspace.TermOrdinal(term)) - 1)
		if !ok || actual != term {
			return false
		}
		_, ok = source.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value})
		return ok
	case keyspace.FamilyFloat:
		actual, _, bits, ok := source.Literals().Floats().At(int(keyspace.TermOrdinal(term)) - 1)
		if !ok || actual != term {
			return false
		}
		_, ok = source.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: bits})
		return ok
	case keyspace.FamilyString:
		actual, _, value, ok := source.Literals().Strings().At(int(keyspace.TermOrdinal(term)) - 1)
		if !ok || actual != term {
			return false
		}
		_, ok = source.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value})
		return ok
	default:
		return false
	}
}
