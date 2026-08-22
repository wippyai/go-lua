package index_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestModuleExportFreshTopologyKeepsExactMixedAndOpaqueArms proves the narrow
// composed-require seam. Value authenticates the exported table roots, while
// Heap accepts them only for an exact scoped-loader Call state. A known extra
// target or the Call opaque arm must retain RouteUnknown.
func TestModuleExportFreshTopologyKeepsExactMixedAndOpaqueArms(t *testing.T) {
	linked := moduleExportTopologyLink(t)
	heap, values, calls, mounts := indexSchemas(t, linked)
	topology := indexTopology(t, heap, values, calls, mounts)

	var freshKey heapdomain.Key
	var applicationID identity.ContentID
	var loaderOperation vocabulary.Operation
	for index := 0; index < heap.FreshCount(); index++ {
		_, candidate, candidateOK := heap.FreshAt(index)
		operation, operationOK := values.ModuleExportFreshOperation(candidate)
		if !candidateOK || !operationOK {
			continue
		}
		applicationID, _, _, _ = candidate.FreshResultID()
		freshKey = candidate
		loaderOperation = operation
		break
	}
	if !freshKey.Valid() || !applicationID.Available() || loaderOperation == 0 {
		t.Fatalf("no Value-authenticated module export fresh root (fresh=%d)", heap.FreshCount())
	}
	callKey, callKeyOK := calls.KeyForApplicationID(applicationID)
	if !callKeyOK || !callKey.Valid() {
		t.Fatal("module export fresh root has no Call application")
	}

	var loader, extra call.Target
	var loaderOK, extraOK bool
	for index := 0; index < calls.SupportCount(callKey); index++ {
		candidate, candidateOK := calls.SupportTargetAt(callKey, index)
		if !candidateOK {
			t.Fatal("module export support target")
		}
		operation, operationOK := candidate.Operation()
		if operationOK && operation == loaderOperation && candidate.IsScopedLoader() && !loaderOK {
			loader, loaderOK = candidate, true
			continue
		}
		if !candidate.IsScopedLoader() && !extraOK {
			extra, extraOK = candidate, true
		}
	}
	if !loaderOK || !extraOK {
		t.Fatalf("module export support loader=%t extra=%t", loaderOK, extraOK)
	}

	atoms, atomsOK := values.Allocation(freshKey, materialization.Recent)
	receiver, receiverOK := values.Singleton(atoms)
	if !atomsOK || !receiverOK {
		t.Fatal("module export fresh receiver")
	}
	collect := func(state call.Value) (roots, unknown int) {
		if !topology.VisitReceiver(receiver, func(key call.Key, tag uint64) (call.Value, bool) {
			return state, key == callKey && tag != 0
		}, func(route indexdomain.Route) bool {
			switch route.Kind() {
			case indexdomain.RouteRoot:
				roots++
			case indexdomain.RouteUnknown:
				unknown++
			default:
				t.Fatalf("module export emitted unexpected route kind %v", route.Kind())
			}
			return true
		}) {
			t.Fatal("module export receiver route failed")
		}
		return roots, unknown
	}

	exact, exactOK := calls.DispatchValue(callKey, []call.Target{loader}, false)
	if !exactOK {
		t.Fatal("exact scoped-loader Call state")
	}
	exactRoots, exactUnknown := collect(exact)
	if exactRoots == 0 || exactUnknown != 0 {
		t.Fatalf("exact scoped-loader routes roots=%d unknown=%d", exactRoots, exactUnknown)
	}

	mixed, mixedOK := calls.DispatchValue(callKey, []call.Target{loader, extra}, false)
	if !mixedOK {
		t.Fatal("mixed Call state")
	}
	mixedRoots, mixedUnknown := collect(mixed)
	if mixedRoots == 0 || mixedUnknown != 1 {
		t.Fatalf("mixed routes roots=%d unknown=%d", mixedRoots, mixedUnknown)
	}

	opaque, opaqueOK := calls.DispatchValue(callKey, []call.Target{loader}, true)
	if !opaqueOK {
		t.Fatal("opaque Call state")
	}
	opaqueRoots, opaqueUnknown := collect(opaque)
	if opaqueRoots == 0 || opaqueUnknown != 1 {
		t.Fatalf("opaque routes roots=%d unknown=%d", opaqueRoots, opaqueUnknown)
	}
}

func moduleExportTopologyLink(t testing.TB) *link.Link {
	t.Helper()
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	main, err := lower.Lower(lower.Source{Name: "module-export-main.lua", Text: []byte(`local loaded = require("x")
return loaded`)})
	if err != nil {
		t.Fatal(err)
	}
	module, err := lower.Lower(lower.Source{Name: "module-export-x.lua", Text: []byte(`local exported = {}
exported.value = 1
return exported`)})
	if err != nil {
		t.Fatal(err)
	}
	modules := []linkproject.Module{{Name: "main", Program: main}, {Name: "x", Program: module}}
	aliases := []linkmodule.ModuleCacheAliasClassSpec{
		{Actor: "module-export-actor", Instances: []string{"instance:main"}, Representative: "instance:main"},
		{Actor: "module-export-actor", Instances: []string{"instance:x"}, Representative: "instance:x"},
	}
	roots := []linkmodule.AnalysisRootSpec{
		{Name: "root:main", Module: "main", Actor: "module-export-actor", Instance: "instance:main"},
		{Name: "root:x", Module: "x", Actor: "module-export-actor", Instance: "instance:x"},
	}
	imports := main.Flow().Authored().Imports()
	item, itemOK := imports.ImportAt(0)
	if !itemOK || item.Term == 0 || item.Request == 0 {
		t.Fatal("module export import geometry")
	}
	if keyspace.TermFamily(item.Request) != keyspace.FamilyString {
		t.Fatal("module export import request family")
	}
	linked, err := link.Seal(&link.Spec{
		Target:  target,
		Modules: modules,
		Module: linkmodule.Spec{
			Actors:             []linkmodule.ActorSpec{{Name: "module-export-actor"}},
			ModuleCacheAliases: aliases,
			AnalysisRoots:      roots,
			ModuleCacheEntries: []linkmodule.ModuleCacheEntrySpec{{Module: "main", Import: item.Term, FromRoot: "root:main", ToRoot: "root:x"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return linked
}
