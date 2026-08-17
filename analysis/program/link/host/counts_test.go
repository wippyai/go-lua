package host

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestHostCountRowsMatchNativeProjectionCounts(t *testing.T) {
	project, boundary, module := hostFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	rows := component.CountRows()
	if !rows.Available() || rows.Count() != 5 {
		t.Fatalf("Host CountRows = %d/%t, want 5/true", rows.Count(), rows.Available())
	}
	ids := denominator.GeneratedLinkHostIDs()
	want := []struct {
		id    schema.EntryID
		count int
	}{
		{ids.LinkHost, boundary.Endpoints().Count()},
		{ids.LinkHostExposure, component.Exposures().Count()},
		{ids.LinkHostBoot, component.Globals().Count()},
		{ids.LinkHostMember, component.Members().Count()},
		{ids.LinkHostEndpointTarget, boundary.Endpoints().Count()},
	}
	for _, row := range want {
		if got, ok := rows.Value(row.id); !ok || got != uint64(row.count) {
			t.Fatalf("Host denominator %v = %d/%t, want %d/true", row.id, got, ok, row.count)
		}
	}
}

func globalInverseFixture(t testing.TB) (*linkproject.Component, *linkboundary.Component, *linkmodule.Component, *program.Program, linkproject.Shard, keyspace.Term) {
	t.Helper()
	closed := target.OperationSpec{
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}
	contract, err := target.Seal(&target.Spec{
		Semantics: domaincontract.NewSemantics(),
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}},
			Input:    closed.Input, Outcomes: closed.Outcomes, Effects: closed.Effects,
		}},
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{
				Kind: target.InitialValueRoot, Root: "GlobalEnvRoot",
			}},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := lower.Lower(lower.Source{Name: "global-inverse", Text: []byte("return require")})
	if err != nil {
		t.Fatal(err)
	}
	other, err := lower.Lower(lower.Source{Name: "global-inverse-other", Text: []byte("return 2")})
	if err != nil {
		t.Fatal(err)
	}
	pd, err := linkproject.Build(linkproject.Input{Target: contract, Modules: []linkproject.Module{
		{Name: "main", Program: p}, {Name: "other", Program: other},
	}})
	if err != nil {
		t.Fatal(err)
	}
	project, err := pd.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	bd, err := linkboundary.Build(linkboundary.Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := bd.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	md, err := linkmodule.Build(linkmodule.Input{Project: project, Boundary: boundary})
	if err != nil {
		t.Fatal(err)
	}
	module, err := md.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	var shard linkproject.Shard
	var shardOK bool
	for index := 0; index < project.Mounts().Count(); index++ {
		candidate, ok := project.Mounts().At(index)
		mounted, mountedOK := project.Mounts().Program(candidate)
		if ok && mountedOK && mounted == p {
			shard, shardOK = candidate, true
			break
		}
	}
	if !shardOK {
		t.Fatal("missing fixture Program shard")
	}
	var cell keyspace.Term
	cells := p.Flow().Authored().Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		candidate, ok := cells.At(index)
		kind, body, key, mapped := cells.Get(candidate)
		if ok && mapped && kind == flow.CellGlobal && body == 0 && key != 0 {
			cell = candidate
			break
		}
	}
	if cell == 0 {
		t.Fatal("fixture has no global Cell")
	}
	return project, boundary, module, p, shard, cell
}

func TestGlobalsForProgramCellExactInverseAndOwnerFence(t *testing.T) {
	project, boundary, module, p, shard, cell := globalInverseFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	host, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	globals := host.Globals()
	if globals.Count() == 0 {
		t.Fatal("fixture Host has no globals")
	}
	first, ok := globals.ForProgramCell(shard, p, cell)
	if !ok {
		t.Fatal("first global inverse lookup failed")
	}
	last, ok := globals.ForProgramCell(shard, p, cell)
	if !ok || last != first {
		t.Fatal("repeat global inverse lookup changed its owner-local handle")
	}
	if _, ok := globals.ForProgramCell(shard, nil, cell); ok {
		t.Fatal("nil Program owner admitted")
	}
	if _, ok := globals.ForProgramCell(shard, p, 0); ok {
		t.Fatal("zero Cell admitted")
	}
	if _, ok := globals.ForProgramCell(shard, p, cell+1); ok {
		t.Fatal("absent Cell admitted")
	}
	equivalent, err := lower.Lower(lower.Source{Name: "global-inverse-equivalent", Text: []byte("return require")})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := globals.ForProgramCell(shard, equivalent, cell); ok {
		t.Fatal("equivalent foreign Program admitted")
	}
	var wrongShard linkproject.Shard
	var wrongShardOK bool
	for index := 0; index < project.Mounts().Count(); index++ {
		candidate, candidateOK := project.Mounts().At(index)
		mounted, mountedOK := project.Mounts().Program(candidate)
		if candidateOK && mountedOK && mounted != p {
			wrongShard, wrongShardOK = candidate, true
			break
		}
	}
	if !wrongShardOK {
		t.Fatal("missing wrong-shard fixture")
	}
	if _, ok := globals.ForProgramCell(wrongShard, p, cell); ok {
		t.Fatal("wrong mounted Shard admitted")
	}
	foreignProjectDraft, err := linkproject.Build(linkproject.Input{Target: mustTarget(boundary), Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	foreignProject, err := foreignProjectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	foreignShard, ok := foreignProject.Mounts().At(0)
	if !ok {
		t.Fatal("missing foreign shard")
	}
	if _, ok := globals.ForProgramCell(foreignShard, p, cell); ok {
		t.Fatal("foreign same-ordinal Project Shard admitted")
	}
	otherDraft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	other, err := otherDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	otherBinding, ok := other.Globals().ForProgramCell(shard, p, cell)
	if !ok {
		t.Fatal("equivalent Host inverse lookup failed")
	}
	if _, _, _, _, _, _, ok := globals.Mapping(otherBinding); ok {
		t.Fatal("equivalent foreign Host binding crossed owner fence")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_, _ = globals.ForProgramCell(shard, p, cell)
	}); allocations != 0 {
		t.Fatalf("ForProgramCell allocated: %g", allocations)
	}
}

func TestGlobalsForProgramCellRejectsDuplicateCollisionAtSeal(t *testing.T) {
	project, boundary, _, _, _, _ := globalInverseFixture(t)
	md, err := linkmodule.Build(linkmodule.Input{
		Project: project, Boundary: boundary,
		Spec: linkmodule.Spec{
			Actors: []linkmodule.ActorSpec{{Name: "actor"}},
			ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{{
				Actor: "actor", Instances: []string{"instance-main"}, Representative: "instance-main",
			}, {
				Actor: "actor", Instances: []string{"instance-other"}, Representative: "instance-other",
			}},
			AnalysisRoots: []linkmodule.AnalysisRootSpec{
				{Name: "first", Module: "main", Actor: "actor", Instance: "instance-main"},
				{Name: "other", Module: "other", Actor: "actor", Instance: "instance-other"},
				{Name: "second", Module: "main", Actor: "actor", Instance: "instance-main"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	duplicateModule, err := md.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Input{Project: project, Boundary: boundary, Module: duplicateModule}); err == nil {
		t.Fatal("duplicate (Shard, Cell) global collision was admitted")
	}
}
