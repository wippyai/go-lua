package index_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestMountedCallDispatchRetainsItsModuleScopedLoader(t *testing.T) {
	linked := twoModuleRequireLink(t)
	_, _, calls, _ := indexSchemas(t, linked)
	if calls.MountedCallCount() == 0 {
		t.Fatal("require fixture has no mounted calls")
	}
	for index := 0; index < calls.MountedCallCount(); index++ {
		mounted, mountedOK := calls.MountedCallAtHandle(index)
		application, wantContext, wantModule, _, wantLoader, identityOK := calls.MountedCallIdentity(mounted)
		lookup, lookupOK := calls.MountedCallForApplication(application)
		gotApplication, gotContext, gotModule, _, loaderID, dispatchOK := calls.MountedCallIdentity(lookup)
		if !mountedOK || !identityOK || !lookupOK || !dispatchOK || lookup != mounted || gotApplication != application || gotModule != wantModule || gotContext != wantContext || loaderID != wantLoader || !loaderID.Available() {
			t.Fatal("mounted dispatch lost its application/module context")
		}
		matched := false
		for mountIndex := 0; mountIndex < linked.Project().Mounts().Count(); mountIndex++ {
			shard, shardOK := linked.Project().Mounts().At(mountIndex)
			module, moduleOK := linked.Project().ModuleKey(shard)
			if !shardOK || !moduleOK || module != gotModule {
				continue
			}
			seed, seedOK := linked.Boundary().Seeds().ScopedLoader(shard)
			wantLoader, wantLoaderOK := linked.Boundary().Seeds().ID(seed)
			if !seedOK || !wantLoaderOK || wantLoader != loaderID {
				t.Fatal("mounted dispatch crossed its module-scoped loader")
			}
			matched = true
		}
		if !matched {
			t.Fatal("mounted dispatch module has no Link mount")
		}
	}
}

func twoModuleRequireLink(t testing.TB) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "heap_index_dispatch.lua", Text: []byte(`return require("x")`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}, {Name: "second", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	return linked
}
