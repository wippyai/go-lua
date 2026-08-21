package link_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	contractvalue "github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func targetStringKey(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}

func contract(t *testing.T, bindings ...vocabulary.BindingSpec) *contractvalue.Contract {
	t.Helper()
	hasRequire := false
	for _, binding := range bindings {
		if binding.Namespace == vocabulary.BindingBuiltin && len(binding.Owner) == 0 && len(binding.Member) == 1 && binding.Member[0] == "require" {
			hasRequire = true
			break
		}
	}
	if !hasRequire {
		bindings = append(bindings, vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}})
	}
	operations := make([]vocabulary.OperationSpec, len(bindings))
	for index, binding := range bindings {
		operations[index] = vocabulary.OperationSpec{
			Bindings: []vocabulary.BindingSpec{binding},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
	}
	return testBootContract(t, operations)
}

func testBootRoots() []vocabulary.InitialRootSpec {
	return []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}}
}

func testBootContract(t testing.TB, operations []vocabulary.OperationSpec, programs ...*program.Program) *contractvalue.Contract {
	return testBootContractWithProtocols(t, operations, nil, programs...)
}

func testBootContractWithProtocols(t testing.TB, operations []vocabulary.OperationSpec, protocols []vocabulary.ProtocolSpec, programs ...*program.Program) *contractvalue.Contract {
	t.Helper()
	globals := make(map[string]struct{})
	for _, p := range programs {
		if p == nil {
			continue
		}
		cells := p.Flow().Authored().Storage().Cells()
		for index := 0; index < cells.Count(); index++ {
			cell, ok := cells.At(index)
			if !ok {
				t.Fatal("missing Program global")
			}
			cellKind, body, key, ok := cells.Get(cell)
			literal, literalOK := p.Source().Keys().Exact(key)
			if !ok || cellKind != authored.CellGlobal || body != 0 || key == 0 || !literalOK || literal.Kind != keyspace.LiteralString {
				continue
			}
			name := literal.String
			if name == "" {
				t.Fatal("malformed Program global")
			}
			globals[name] = struct{}{}
		}
	}
	admitted := make(map[string]vocabulary.BindingSpec)
	structural := make(map[string]struct{})
	for _, operation := range operations {
		for _, binding := range operation.Bindings {
			if binding.Namespace == vocabulary.BindingBuiltin && len(binding.Owner) == 0 && len(binding.Member) == 1 {
				admitted[binding.Member[0]] = binding
			}
			if binding.Namespace == vocabulary.BindingBuiltin && len(binding.Owner) == 0 && len(binding.Member) != 0 {
				structural[binding.Member[0]] = struct{}{}
			}
			if binding.Namespace == vocabulary.BindingModule && len(binding.Owner) != 0 {
				structural[binding.Owner[0]] = struct{}{}
			}
		}
	}
	for name := range admitted {
		globals[name] = struct{}{}
	}
	for name := range structural {
		globals[name] = struct{}{}
	}
	entries := []vocabulary.InitialEntrySpec{
		{Root: "GlobalEnvRoot", Key: targetStringKey("_G"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("__link_absent"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
	}
	bindings := []vocabulary.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: targetStringKey("_G")}, {Name: "__link_absent", Root: "GlobalEnvRoot", Key: targetStringKey("__link_absent")}}
	for name := range globals {
		binding, admittedBinding := admitted[name]
		if !admittedBinding {
			if _, isStructural := structural[name]; isStructural {
				entries = append(entries, vocabulary.InitialEntrySpec{Root: "GlobalEnvRoot", Key: targetStringKey(name), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable})
				bindings = append(bindings, vocabulary.InitialBindingSpec{Name: name, Root: "GlobalEnvRoot", Key: targetStringKey(name)})
			}
			continue
		}
		entries = append(entries, vocabulary.InitialEntrySpec{Root: "GlobalEnvRoot", Key: targetStringKey(name), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: binding}, Mutability: vocabulary.InitialMutable})
		bindings = append(bindings, vocabulary.InitialBindingSpec{Name: name, Root: "GlobalEnvRoot", Key: targetStringKey(name)})
	}
	sealed, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: operations, Protocols: protocols, InitialRoots: testBootRoots(), InitialEntries: entries, InitialBindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func source(t testing.TB, text string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "test", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func linked(t testing.TB, sealed *contractvalue.Contract, modules ...linkproject.Module) *link.Link {
	t.Helper()
	l, err := link.Seal(&link.Spec{Target: sealed, Modules: modules})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func onlyShard(t *testing.T, l *link.Link, p *program.Program) linkproject.Shard {
	return onlyShardFor(t, l, p)
}

func onlyShardFor(t testing.TB, l *link.Link, p *program.Program) linkproject.Shard {
	t.Helper()
	shard, ok := shardForProgram(l, p)
	if !ok {
		t.Fatal("missing Program shard")
	}
	return shard
}

func shardForProgram(l *link.Link, p *program.Program) (linkproject.Shard, bool) {
	if l == nil || p == nil {
		return linkproject.Shard{}, false
	}
	for index := 0; index < l.Project().Mounts().Count(); index++ {
		shard, _ := l.Project().Mounts().At(index)
		owner, _ := l.Project().Mounts().Program(shard)
		if owner == p {
			return shard, true
		}
	}
	return linkproject.Shard{}, false
}

func call(t *testing.T, p *program.Program, index int) keyspace.Term {
	t.Helper()
	term, ok := p.Flow().Authored().Calls().At(index)
	if !ok {
		t.Fatalf("missing call %d", index)
	}
	return term
}

func exactStringKey(t *testing.T, p *program.Program, want string) keyspace.Key {
	t.Helper()
	keys := p.Source().Keys()
	for index := 0; index < keys.ExactCount(); index++ {
		key, value, ok := keys.ExactAt(index)
		if ok && value.Kind == keyspace.LiteralString && value.String == want {
			return key
		}
	}
	t.Fatalf("missing exact string key %q", want)
	return 0
}

func moduleCacheDeployment(t *testing.T, p, dependency *program.Program) (actors []linkmodule.ActorSpec, aliases []linkmodule.ModuleCacheAliasClassSpec, roots []linkmodule.AnalysisRootSpec, entries []linkmodule.ModuleCacheEntrySpec) {
	t.Helper()
	actors = []linkmodule.ActorSpec{{Name: "actor"}}
	// Canonical instance identity encodes the string length before bytes;
	// cache-main is therefore the representative of this two-member class.
	representative := "cache-main"
	aliases = []linkmodule.ModuleCacheAliasClassSpec{{Actor: "actor", Instances: []string{"cache-main", "cache-dependency"}, Representative: representative}}
	roots = []linkmodule.AnalysisRootSpec{{Name: "main-root", Module: "main", Actor: "actor", Instance: "cache-main"}, {Name: "dependency-root", Module: "dependency", Actor: "actor", Instance: "cache-dependency"}}
	importTerm, ok := p.Flow().Authored().Imports().At(0)
	if !ok || dependency == nil {
		t.Fatal("missing source Import")
	}
	entries = []linkmodule.ModuleCacheEntrySpec{{Module: "main", Import: importTerm, FromRoot: "main-root", ToRoot: "dependency-root"}}
	return actors, aliases, roots, entries
}

func hostGlobalRead(t *testing.T, p *program.Program, name string) keyspace.Term {
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
		cellKind, body, key, ok := p.Flow().Authored().Storage().Cells().Get(source)
		literal, literalOK := p.Source().Keys().Exact(key)
		if ok && cellKind == authored.CellGlobal && body == 0 && literalOK && literal.Kind == keyspace.LiteralString && literal.String == name {
			return read
		}
	}
	t.Fatalf("missing global Read %q", name)
	return 0
}

func artifactMemberRead(t testing.TB, p *program.Program) keyspace.Term {
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
		_, _, _, kind, ok := p.Flow().Authored().Access().Exact().Get(source)
		if ok && kind != flowkind.FieldList && kind != flowkind.FieldKey {
			return read
		}
	}
	t.Fatal("missing member Read")
	return 0
}

func actorBootOperation(parts ...string) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: parts}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func actorBootContract(t *testing.T, operations []vocabulary.OperationSpec, entries []vocabulary.InitialEntrySpec, bindings []vocabulary.InitialBindingSpec) *contractvalue.Contract {
	t.Helper()
	entries = append([]vocabulary.InitialEntrySpec{
		{Root: "GlobalEnvRoot", Key: targetStringKey("_G"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("__actor_boot_absent"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
	}, entries...)
	bindings = append([]vocabulary.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: targetStringKey("_G")}}, bindings...)
	sealed, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: operations, InitialRoots: testBootRoots(), InitialEntries: entries, InitialBindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func capabilityFixture(t *testing.T, permuted bool) (*link.Link, *contractvalue.Contract, *program.Program, vocabulary.Operation, keyspace.Term, keyspace.Term) {
	t.Helper()
	p := source(t, `actor.send(1)`)
	binding := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	sealed := testBootContract(t, []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{binding},
		Input:    vocabulary.ValuesSpec{Fixed: portableTypes(t, []typ.Type{typ.Any}), Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: portableTypes(t, []typ.Type{typ.Any}), Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}, p)
	op, ok := sealed.Operations.Lookup(binding)
	if !ok {
		t.Fatal("missing provider operation")
	}
	actor := hostGlobalRead(t, p, "actor")
	member := artifactMemberRead(t, p)
	spec := link.Spec{
		Target:           sealed,
		Modules:          []linkproject.Module{{Name: "main", Program: p}},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "actor.send", Binding: binding}},
		Host: linkhost.Spec{ProviderCapabilities: []linkhost.ProviderCapabilitySpec{
			{Identity: "actor"}, {Identity: "boot"}, {Identity: "input"}, {Identity: "result"},
		},
			ProviderCapabilitySeeds: []linkhost.ProviderCapabilitySeedSpec{
				{Capability: "actor", Source: linkhost.ProviderCapabilitySourceExposure, Module: "main", Access: actor},
				{Capability: "boot", Source: linkhost.ProviderCapabilitySourceInitialRoot, InitialRoot: "GlobalEnvRoot"},
				{Capability: "input", Source: linkhost.ProviderCapabilitySourceABIInput, Binding: binding, Formal: 0},
				{Capability: "result", Source: linkhost.ProviderCapabilitySourceResult, Binding: binding, Outcome: 0, Result: 0},
			},
			Members: []linkhost.HostMemberSpec{{Module: "main", Access: member, Capability: "actor", Endpoint: "actor.send", Dispatch: linkhost.HostDispatchLookup}}},
	}
	if permuted {
		reverseCapabilities(spec.Host.ProviderCapabilities)
		reverseCapabilitySeeds(spec.Host.ProviderCapabilitySeeds)
	}
	l, err := link.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	return l, sealed, p, op, actor, member
}

func portableTypes(t testing.TB, values []typ.Type) []schematype.Type {
	t.Helper()
	if len(values) == 0 {
		return nil
	}
	out := make([]schematype.Type, len(values))
	for index, value := range values {
		encoded, err := domaincontract.EncodeStorage(context.Background(), value, nil)
		if err != nil {
			t.Fatal(err)
		}
		out[index] = encoded
	}
	return out
}

func reverseCapabilities(items []linkhost.ProviderCapabilitySpec) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reverseCapabilitySeeds(items []linkhost.ProviderCapabilitySeedSpec) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}
