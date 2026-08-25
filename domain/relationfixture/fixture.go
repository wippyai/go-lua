// Package relationfixture seals the analysis world every axis binds its
// laws against. Every authority in it is the production one: a real Link, a
// real heap schema, a real correlated value schema, a real call algebra, a
// real pack schema and a real sealed index topology over a program that
// indexes a table.
package relationfixture

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Sealed is the sealed analysis world every axis binds its laws against.
// Every authority in it is the production one: a real Link, a real heap
// schema, a real correlated value schema, a real call algebra, and a real
// sealed index topology over a program that indexes a table.
type Sealed struct {
	Heap     heapdomain.Schema
	Values   *valuedomain.Schema
	Calls    *calldomain.Algebra
	Packs    *packdomain.Schema
	Classes  *staticdomain.ClassSet
	Effects  *effectfactor.Algebra
	Topology *indexdomain.Topology
	Receiver valuedomain.Value
	Root     heapdomain.Key
}

const fixtureSource = `local holder = {}
holder.field = nil
return holder.field`

func portableAnyTypes(count int) []schematype.Type {
	values := make([]schematype.Type, count)
	for index := range values {
		value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
		if !ok {
			panic("portable any type")
		}
		values[index] = value
	}
	return values
}

func New(t testing.TB) Sealed {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "relbindgen_binding_law.lua", Text: []byte(fixtureSource)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: portableAnyTypes(1), Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK {
		t.Fatal("binding fixture compilation")
	}
	mountCount := linked.Project().Mounts().Count()
	heapMounts := make([]programmount.MountedArtifact, 0, mountCount)
	valueMounts := make([]programmount.MountedArtifact, 0, mountCount)
	packMounts := make([]programmount.MountedArtifact, 0, mountCount)
	callMounts := make([]calldomain.MountedArtifact, 0, mountCount)
	effectMounts := make([]effectfactor.MountedArtifact, 0, mountCount)
	staticMounts := make([]staticdomain.MountedProgram, 0, mountCount)
	programs := make([]programschema.Program, 0, mountCount)
	for index := 0; index < mountCount; index++ {
		shard, shardOK := linked.Project().Mounts().At(index)
		source, sourceOK := linked.Project().Mounts().Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if !shardOK || !sourceOK || source == nil || !moduleOK {
			t.Fatal("binding fixture mount")
		}
		var artifact *programartifact.Artifact
		artifact, failure := artifactcompiler.CompileDetailed(source, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("binding fixture artifact: %v", failure)
		}
		lowered := snapshottest.MustLower(t, artifact)
		heapMount, heapOK := programmount.MountedArtifactFromSnapshot(lowered, module)
		valueMount, valueOK := programmount.MountedArtifactFromSnapshot(lowered, module)
		packMount, packOK := programmount.MountedArtifactFromSnapshot(lowered, module)
		if !heapOK || !valueOK || !packOK {
			t.Fatal("binding fixture mounted artifact")
		}
		mounted := snapshottest.MustMount(t, artifact, module)
		heapMounts = append(heapMounts, heapMount)
		valueMounts = append(valueMounts, valueMount)
		packMounts = append(packMounts, packMount)
		callMounts = append(callMounts, calldomain.MountedArtifact{Program: mounted, Snapshot: lowered})
		effectMounts = append(effectMounts, effectfactor.MountedArtifact{ModuleKey: module, Snapshot: lowered})
		staticMounts = append(staticMounts, staticdomain.MountedProgram{Program: mounted.Program, ModuleID: module, NamespaceID: module})
		programs = append(programs, artifact.Program())
	}
	heap, heapFailure := heapdomain.SealWithArtifacts(linked, heapMounts)
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if heapFailure != heapdomain.SealFailureNone || !structuralOK {
		t.Fatalf("binding fixture heap seal: %v", heapFailure)
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, calltest.MustSeal(t, linked, valueMounts), valueMounts, structural)
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, callMounts)
	if valueFailure != valuedomain.SealFailureNone || !callsOK {
		t.Fatalf("binding fixture value seal: %v calls=%t", valueFailure, callsOK)
	}
	boundary, _ := linked.Boundary().Target()
	types, typeErr := typeauthority.SealProgramRows(linked.ContentID(), programs, nil)
	if typeErr != nil || types == nil {
		t.Fatalf("binding fixture type seal: %v", typeErr)
	}
	statics, _, staticErr := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: boundary}, types, staticMounts)
	if staticErr != nil || statics == nil {
		t.Fatalf("binding fixture static seal: %v", staticErr)
	}
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, packMounts)
	if !packsOK || packs == nil {
		t.Fatal("binding fixture pack seal")
	}
	selectors, selectorsOK := keymatch.NewSelectorProjection(heap, values)
	if !selectorsOK {
		t.Fatal("binding fixture key selection")
	}
	topology, sealed := indexdomain.Seal(heap, values, calls, packs, selectors)
	if !sealed || topology == nil {
		t.Fatal("binding fixture topology seal")
	}
	effects, effectsOK := effectfactor.NewWithMountedArtifacts(linked, packs, boundary, effectMounts)
	if !effectsOK || effects == nil {
		t.Fatal("binding fixture effect seal")
	}
	fixture := Sealed{Heap: heap, Values: values, Calls: calls, Packs: packs, Classes: statics.Classes(), Effects: effects, Topology: topology}
	fixture.Root, fixture.Receiver = fixtureReceiver(t, heap, values)
	return fixture
}

// fixtureReceiver returns the table root the program allocates and a receiver
// value that observes exactly that root.
func fixtureReceiver(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema) (heapdomain.Key, valuedomain.Value) {
	t.Helper()
	for index := 0; index < heap.KeyCount(); index++ {
		candidate, ok := heap.KeyAt(index)
		_, _, _, kind, _, source := heap.AllocationOriginForKey(candidate)
		if !ok || !source || kind != heapdomain.AllocationTable {
			continue
		}
		atom, atomOK := values.Allocation(candidate, materialization.Recent)
		if !atomOK {
			continue
		}
		receiver, receiverOK := values.Singleton(atom)
		if !receiverOK {
			continue
		}
		return candidate, receiver
	}
	t.Fatal("binding fixture table root")
	return heapdomain.Key{}, valuedomain.Value{}
}
