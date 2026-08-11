package link

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func contract(t *testing.T, bindings ...target.BindingSpec) *target.Contract {
	t.Helper()
	hasRequire := false
	for _, binding := range bindings {
		if binding.Namespace == target.BindingBuiltin && len(binding.Owner) == 0 && len(binding.Member) == 1 && binding.Member[0] == "require" {
			hasRequire = true
			break
		}
	}
	if !hasRequire {
		bindings = append(bindings, target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}})
	}
	operations := make([]target.OperationSpec, len(bindings))
	for index, binding := range bindings {
		operations[index] = target.OperationSpec{
			Bindings: []target.BindingSpec{binding},
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}
	}
	return testBootContract(t, operations)
}

func testBootRoots() []target.InitialRootSpec {
	return []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}}
}

func testBootContract(t testing.TB, operations []target.OperationSpec, programs ...*program.Program) *target.Contract {
	return testBootContractWithProtocols(t, operations, nil, programs...)
}

func testBootContractWithProtocols(t testing.TB, operations []target.OperationSpec, protocols []target.ProtocolSpec, programs ...*program.Program) *target.Contract {
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
			if !ok || cellKind != flow.CellGlobal || body != 0 || key == 0 || !literalOK || literal.Kind != keyspace.LiteralString {
				continue
			}
			name := literal.String
			if name == "" {
				t.Fatal("malformed Program global")
			}
			globals[name] = struct{}{}
		}
	}
	admitted := make(map[string]target.BindingSpec)
	structural := make(map[string]struct{})
	for _, operation := range operations {
		for _, binding := range operation.Bindings {
			if binding.Namespace == target.BindingBuiltin && len(binding.Owner) == 0 && len(binding.Member) == 1 {
				admitted[binding.Member[0]] = binding
			}
			if binding.Namespace == target.BindingBuiltin && len(binding.Owner) == 0 && len(binding.Member) != 0 {
				structural[binding.Member[0]] = struct{}{}
			}
			if binding.Namespace == target.BindingModule && len(binding.Owner) != 0 {
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
	entries := []target.InitialEntrySpec{
		{Root: "GlobalEnvRoot", Key: targetStringKey("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
	}
	bindings := []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: targetStringKey("_G")}, {Name: "__link_absent", Root: "GlobalEnvRoot", Key: targetStringKey("__link_absent")}}
	for name := range globals {
		binding, admittedBinding := admitted[name]
		if !admittedBinding {
			if _, isStructural := structural[name]; isStructural {
				entries = append(entries, target.InitialEntrySpec{Root: "GlobalEnvRoot", Key: targetStringKey(name), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable})
				bindings = append(bindings, target.InitialBindingSpec{Name: name, Root: "GlobalEnvRoot", Key: targetStringKey(name)})
			}
			continue
		}
		entries = append(entries, target.InitialEntrySpec{Root: "GlobalEnvRoot", Key: targetStringKey(name), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: binding}, Mutability: target.InitialMutable})
		bindings = append(bindings, target.InitialBindingSpec{Name: name, Root: "GlobalEnvRoot", Key: targetStringKey(name)})
	}
	sealed, err := target.Seal(&target.Spec{Operations: operations, Protocols: protocols, InitialRoots: testBootRoots(), InitialEntries: entries, InitialBindings: bindings})
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

func linked(t testing.TB, sealed *target.Contract, modules ...linkproject.Module) *Link {
	t.Helper()
	l, err := Seal(&Spec{Target: sealed, Modules: modules})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func onlyShard(t *testing.T, l *Link, p *program.Program) linkproject.Shard {
	return onlyShardFor(t, l, p)
}

func onlyShardFor(t testing.TB, l *Link, p *program.Program) linkproject.Shard {
	t.Helper()
	shard, ok := shardForProgram(l, p)
	if !ok {
		t.Fatal("missing Program shard")
	}
	return shard
}

func onlyProjectShardFor(t testing.TB, l *Link, p *program.Program) linkproject.Shard {
	t.Helper()
	if l == nil || l.Project() == nil || p == nil {
		t.Fatalf("missing Project or Program")
	}
	mounts := l.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		mounted, mountedOK := mounts.Program(shard)
		if ok && mountedOK && mounted == p {
			return shard
		}
	}
	t.Fatalf("Program is not mounted")
	return linkproject.Shard{}
}

func shardForProgram(l *Link, p *program.Program) (linkproject.Shard, bool) {
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

func stringTerm(t *testing.T, p *program.Program, want string) keyspace.Term {
	t.Helper()
	strings := p.Source().Literals().Strings()
	for index := 0; index < strings.Count(); index++ {
		term, _, value, ok := strings.At(index)
		if !ok {
			continue
		}
		if value == want {
			return term
		}
	}
	t.Fatalf("missing literal %q", want)
	return 0
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
	importTerm, ok := p.Module().ImportAt(0)
	if !ok || dependency == nil {
		t.Fatal("missing source Import")
	}
	entries = []linkmodule.ModuleCacheEntrySpec{{Module: "main", Import: importTerm.Term, FromRoot: "main-root", ToRoot: "dependency-root"}}
	return actors, aliases, roots, entries
}

func sealModuleCacheProject(t *testing.T) (*Link, *target.Contract, *program.Program, *program.Program) {
	t.Helper()
	sealed, _, _, _ := moduleCacheContract(t)
	main := source(t, `op(function() end); require("dependency")`)
	dependency := source(t, `return 1`)
	actors, aliases, roots, entries := moduleCacheDeployment(t, main, dependency)
	l, err := Seal(&Spec{Target: sealed, Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}}, Module: linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries}})
	if err != nil {
		t.Fatal(err)
	}
	return l, sealed, main, dependency
}

func moduleCacheContract(t *testing.T) (*target.Contract, target.Operation, target.CallbackID, target.ResumeID) {
	t.Helper()
	sealed := testBootContract(t, []target.OperationSpec{
		{
			Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"op"}}},
			Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		},
		{
			Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}},
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		},
	})
	op, _ := sealed.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"op"}})
	return sealed, op, 0, 0
}

func analysisRootForProgram(t testing.TB, l *Link, p *program.Program) linkmodule.AnalysisRoot {
	t.Helper()
	shard := onlyProjectShardFor(t, l, p)
	if count := l.Module().Roots().ForShardCount(shard); count != 1 {
		t.Fatalf("analysis roots for %q = %d, want 1", p.ContentID(), count)
	}
	root, ok := l.Module().Roots().ForShardAt(shard, 0)
	if !ok {
		t.Fatal("missing analysis root")
	}
	return root
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
		if ok && cellKind == flow.CellGlobal && body == 0 && literalOK && literal.Kind == keyspace.LiteralString && literal.String == name {
			return read
		}
	}
	t.Fatalf("missing global Read %q", name)
	return 0
}

func reverseModules(items []linkproject.Module) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reverseRoots(items []linkmodule.AnalysisRootSpec) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reverseEntries(items []linkmodule.ModuleCacheEntrySpec) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func effectRowContract(t testing.TB) *target.Contract {
	t.Helper()
	terminal := func() []target.OutcomeSpec {
		return []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}
	}
	callback := func(function target.ValueFormal, effectTarget target.SpecRef, values []target.ValueFormal, rows []target.RowVar, tail target.RowTail) target.CallbackSpec {
		return target.CallbackSpec{
			Function:  target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(function)},
			Admission: target.OrdinaryCallable,
			Arguments: target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}},
				{Kind: flowkind.OutcomeReturn, Values: target.ValuesSpec{Tail: target.ValuesClosed}},
				{Kind: flowkind.OutcomeThrow, Values: target.ValuesSpec{Tail: target.ValuesClosed}},
				{Kind: flowkind.OutcomeYield, Values: target.ValuesSpec{Tail: target.ValuesClosed}},
				{Kind: flowkind.OutcomeCancel, Values: target.ValuesSpec{Tail: target.ValuesClosed}},
			},
			Lifecycle: target.CallbackRetainedOptionalOnce,
			Effects:   target.RowSpec{Occurrences: []target.EffectSpec{{Target: effectTarget, ValueArgs: values, RowArgs: rows}}, Tail: tail},
		}
	}
	closed := target.OperationSpec{
		Bindings:  []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"closed"}}},
		Input:     target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
		Outcomes:  terminal(),
		Effects:   target.RowSpec{Occurrences: []target.EffectSpec{{Target: 1, ValueArgs: []target.ValueFormal{0}}}, Tail: target.RowClosed},
		Callbacks: []target.CallbackSpec{callback(0, 1, []target.ValueFormal{0}, nil, target.RowClosed)},
	}
	named := target.OperationSpec{
		Bindings:   []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"named"}}},
		RowFormals: 1,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: target.ValuesClosed},
		Outcomes:   terminal(),
		Effects:    target.RowSpec{Occurrences: []target.EffectSpec{{Target: 2, ValueArgs: []target.ValueFormal{0, 1}, RowArgs: []target.RowVar{0}}}, Tail: target.RowVariable, Var: 0},
		Callbacks: []target.CallbackSpec{
			callback(0, 2, []target.ValueFormal{0, 1}, []target.RowVar{0}, target.RowVariable),
			callback(1, 1, []target.ValueFormal{0}, nil, target.RowClosed),
		},
	}
	return testBootContract(t, []target.OperationSpec{closed, named})
}

func sameLinkValue(left *Link, leftValue linkboundary.Value, right *Link, rightValue linkboundary.Value) bool {
	leftID, leftOK := left.Boundary().Values().ID(leftValue)
	rightID, rightOK := right.Boundary().Values().ID(rightValue)
	return leftOK && rightOK && leftID == rightID
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

func actorBootOperation(parts ...string) target.OperationSpec {
	return target.OperationSpec{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: parts}},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}
}

func actorBootContract(t *testing.T, operations []target.OperationSpec, entries []target.InitialEntrySpec, bindings []target.InitialBindingSpec) *target.Contract {
	t.Helper()
	entries = append([]target.InitialEntrySpec{
		{Root: "GlobalEnvRoot", Key: targetStringKey("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("__actor_boot_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
	}, entries...)
	bindings = append([]target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: targetStringKey("_G")}}, bindings...)
	sealed, err := target.Seal(&target.Spec{Operations: operations, InitialRoots: testBootRoots(), InitialEntries: entries, InitialBindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func capabilityFixture(t *testing.T, permuted bool) (*Link, *target.Contract, *program.Program, target.Operation, keyspace.Term, keyspace.Term) {
	t.Helper()
	p := source(t, `actor.send(1)`)
	binding := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	sealed := testBootContract(t, []target.OperationSpec{{
		Bindings: []target.BindingSpec{binding},
		Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}, p)
	op, ok := sealed.Lookup(binding)
	if !ok {
		t.Fatal("missing provider operation")
	}
	actor := hostGlobalRead(t, p, "actor")
	member := artifactMemberRead(t, p)
	spec := Spec{
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
	l, err := Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	return l, sealed, p, op, actor, member
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
