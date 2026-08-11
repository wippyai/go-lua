package capability

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	programflow "github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestExposureRuleBindsOnlyLinkExposureSeeds(t *testing.T) {
	schema, _ := exposureFixture(t)
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, exposureKey(1), exposureKey(900_001), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	rule, ok := DeclareExposure(composition, exposureKey(2), exposureKey(3), exposureKey(4), owner)
	if !ok || rule == nil {
		t.Fatal("exposure Rule")
	}
	if !declareExposureQuery(composition, owner) || !composition.Seal() {
		t.Fatal("exposure composition seal")
	}
	report, ok := composition.SemanticReport()
	if !ok || len(report.Incidences) != 1 || report.Incidences[0] != (engine.FactorIncidence{Read: exposureKey(1), Write: exposureKey(1)}) {
		t.Fatal("exposure Rule did not retain Value's exact self dependency")
	}

	exposed, nonExposure := 0, 0
	for index := 0; index < schema.CapabilitySeedCount(); index++ {
		seed, ok := schema.CapabilitySeedAt(index)
		if !ok {
			t.Fatalf("CapabilitySeedAt(%d)", index)
		}
		_, exposure := seed.Exposure()
		instance, bound := rule.Instance(seed)
		if exposure {
			exposed++
			frozen, digest, frozenOK := func() (value.CapabilitySeed, [32]byte, bool) {
				id, ok := seed.ID()
				return seed, [32]byte(id), ok && [32]byte(id) != [32]byte{}
			}()
			_, replay, replayOK := func() (value.CapabilitySeed, [32]byte, bool) {
				id, ok := frozen.ID()
				return frozen, [32]byte(id), ok && [32]byte(id) != [32]byte{}
			}()
			if !bound || instance == nil || !frozenOK || !replayOK || digest == [32]byte{} || digest != replay {
				t.Fatalf("exposure seed %d did not bind its exact canonical instance", index)
			}
			continue
		}
		nonExposure++
		if bound || instance != nil {
			t.Fatalf("non-exposure seed %d acquired a Value coordinate", index)
		}
	}
	if exposed != 1 || nonExposure == 0 {
		t.Fatalf("capability seed denominator exposure=%d non-exposure=%d", exposed, nonExposure)
	}

	foreignSchema, _ := exposureFixture(t)
	foreign, ok := foreignSchema.CapabilitySeedAt(0)
	if !ok {
		t.Fatal("foreign capability seed")
	}
	if instance, ok := rule.Instance(foreign); ok || instance != nil {
		t.Fatal("foreign same-content capability seed crossed Value owner fence")
	}
	if evidence, accepted := exposureChecker(owner, exposureKey(2), &rule.read)(engine.RuleDerivation[value.Value, value.CapabilitySeed]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged derivation minted capability exposure evidence")
	}
}

func declareExposureQuery(composition *engine.Composition, owner *valueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: exposureKey(5),
		Project:  func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: exposureKey(6), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		_, declared := engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	return ok && query != nil
}

func exposureFixture(t testing.TB) (*value.Schema, *link.Link) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "capability_exposure_rule.lua", Text: []byte("actor.send(1)")})
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}},
		}},
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{binding}, Input: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: exposureLiteral("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: exposureLiteral("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: exposureLiteral("_G")},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: exposureLiteral("__link_absent")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	actorRead := exposureGlobalRead(t, p, "actor")
	linked, err := link.Seal(&link.Spec{
		Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "actor.send", Binding: binding}},
		Host: linkhost.Spec{
			ProviderCapabilities: []linkhost.ProviderCapabilitySpec{{Identity: "actor"}, {Identity: "boot"}},
			ProviderCapabilitySeeds: []linkhost.ProviderCapabilitySeedSpec{
				{Capability: "actor", Source: linkhost.ProviderCapabilitySourceExposure, Module: "main", Access: actorRead},
				{Capability: "boot", Source: linkhost.ProviderCapabilitySourceInitialRoot, InitialRoot: "GlobalEnvRoot"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := value.Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("Value schema")
	}
	return schema, linked
}

func exposureGlobalRead(t testing.TB, p *program.Program, name string) keyspace.Term {
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

func exposureLiteral(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}

func exposureKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("capability exposure test key")
	}
	return key
}
