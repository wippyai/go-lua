package keymatch_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	keymatch "github.com/wippyai/go-lua/domain/heap/keymatch"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestProjectPreservesOneAtomAlternativeWithoutInventingIdentity(t *testing.T) {
	heap, values, linked := fixture(t, "keymatch_exact", `
local object = {}
return nil, 1, 1.0, object
`)
	nilAtom := sourceAtom(t, values, linked, func(atom valuedomain.Atom) bool {
		return atom.RuntimeKinds() == runtimekind.Bit(runtimekind.Nil)
	})
	if validity := nilAtom.TableKeyValidity(); validity.MayBeValid() || !validity.MayBeInvalid() {
		t.Fatal("nil did not retain its invalid-key branch")
	}
	if _, ok := keymatch.Project(heap, values, nilAtom); ok {
		t.Fatal("nil manufactured a valid Heap selector")
	}

	var numeric []valuedomain.Atom
	boundaryValues := linked.Boundary().Values()
	for index := 0; index < boundaryValues.Count(); index++ {
		value, valueOK := boundaryValues.At(index)
		if !valueOK {
			t.Fatal("Value")
		}
		valueID, valueIDOK := boundaryValues.ID(value)
		fact, factOK := values.SourceValueID(valueID)
		var atom valuedomain.Atom
		if factOK {
			values.VisitSupport(fact, func(candidate valuedomain.Atom) { atom = candidate })
		}
		if !valueIDOK || !factOK || !values.OwnsAtom(atom) {
			continue
		}
		if literal, keyOK := atom.ExactKey(); keyOK {
			if literal.Kind == keyspace.LiteralInteger {
				numeric = append(numeric, atom)
			}
		}
	}
	if len(numeric) < 2 {
		t.Fatal("fixture omitted normalized numeric sources")
	}
	if numeric[0] != numeric[1] {
		t.Fatal("Value did not retain Link's 1 == 1.0 exact key quotient")
	}
	firstNumeric, firstNumericOK := keymatch.Project(heap, values, numeric[0])
	secondNumeric, secondNumericOK := keymatch.Project(heap, values, numeric[1])
	if !firstNumericOK || !secondNumericOK || firstNumeric.Selector().Kind() != heapdomain.KeySelectorAtom || !sameAlternative(firstNumeric, secondNumeric) {
		t.Fatal("normalized numeric source did not project one exact Link key")
	}
	if firstNumeric.Containment().Kind() != heapdomain.ContainmentNone {
		t.Fatal("literal key did not prove absent containment")
	}

	key, keyOK := firstAllocationKey(heap)
	recentAtom, atomOK := values.Allocation(key, materialization.Recent)
	if !keyOK || !atomOK || recentAtom.TableKeyValidity() != valuedomain.TableKeyValid {
		t.Fatal("recent rooted table key")
	}
	rooted, projected := keymatch.Project(heap, values, recentAtom)
	if !projected || rooted.Selector().Kind() != heapdomain.KeySelectorAtom {
		t.Fatal("recent rooted atom did not project exactly")
	}
	wantChild, childOK := heap.Reference(key, materialization.Recent)
	gotChild, hasChild := rooted.Containment().Reference()
	gotSelectorChild, selectorChildOK := rooted.Selector().ReferenceAt(0)
	if !keyOK || !childOK || rooted.Containment().Kind() != heapdomain.ContainmentExact || !hasChild || gotChild != wantChild || !selectorChildOK || gotSelectorChild != wantChild {
		t.Fatal("rooted key lost its one Heap child mapping")
	}

	summaryAtom, summaryOK := values.Allocation(key, materialization.Summary)
	summary, summaryProjected := keymatch.Project(heap, values, summaryAtom)
	if !summaryOK || !summaryProjected {
		t.Fatal("summary atom")
	}
	wantSummary, summaryChildOK := heap.Reference(key, materialization.Summary)
	gotChild, hasChild = summary.Containment().Reference()
	if summary.Selector().Kind() != heapdomain.KeySelectorKinds || summary.Containment().Kind() != heapdomain.ContainmentExact || !summaryChildOK || !hasChild || gotChild != wantSummary {
		t.Fatal("summary must retain child but lose exact key identity")
	}

	inexact, inexactOK := values.OpaqueKind(runtimekind.Number)
	if !inexactOK || inexact.TableKeyValidity() != valuedomain.TableKeyValid|valuedomain.TableKeyInvalid {
		t.Fatal("opaque number did not retain both key-validity branches")
	}
	opaque, opaqueProjected := keymatch.Project(heap, values, inexact)
	if !opaqueProjected || opaque.Selector().Kind() != heapdomain.KeySelectorKinds || opaque.Selector().RuntimeKinds() != runtimekind.Bit(runtimekind.Number) {
		t.Fatal("opaque numeric key did not become one typed Number selector")
	}
	if opaque.Containment().Kind() != heapdomain.ContainmentNone {
		t.Fatal("opaque numeric key did not prove absent containment")
	}

	opaqueReference, opaqueReferenceOK := values.OpaqueReference(valuedomain.ReferenceTable)
	if !opaqueReferenceOK {
		t.Fatal("opaque reference value")
	}
	opaqueReferenceAlternative, opaqueReferenceProjected := keymatch.Project(heap, values, opaqueReference)
	if !opaqueReferenceProjected || opaqueReferenceAlternative.Selector().Kind() != heapdomain.KeySelectorKinds || opaqueReferenceAlternative.Containment().Kind() != heapdomain.ContainmentUnknown {
		t.Fatal("opaque reference key was mistaken for known-none containment")
	}
}

func TestContainmentMapsScalarRootedOpaqueAndForeignAtoms(t *testing.T) {
	heap, values, linked := fixture(t, "keymatch_containment", `local object = {}; return 1, object`)
	scalar := sourceAtom(t, values, linked, func(atom valuedomain.Atom) bool {
		return atom.RuntimeKinds() == runtimekind.Bit(runtimekind.Number)
	})
	scalarContainment, scalarOK := keymatch.Containment(heap, values, scalar)
	if !scalarOK || scalarContainment.Kind() != heapdomain.ContainmentNone {
		t.Fatal("scalar atom did not prove known-none containment")
	}

	key, keyOK := firstAllocationKey(heap)
	rooted, rootedOK := values.Allocation(key, materialization.Recent)
	want, wantOK := heap.Reference(key, materialization.Recent)
	rootedContainment, containmentOK := keymatch.Containment(heap, values, rooted)
	got, gotOK := rootedContainment.Reference()
	if !rootedOK || !keyOK || !wantOK || !containmentOK || rootedContainment.Kind() != heapdomain.ContainmentExact || !gotOK || got != want {
		t.Fatal("tracked rooted atom lost exact containment")
	}

	opaque, opaqueOK := values.OpaqueReference(valuedomain.ReferenceTable)
	opaqueContainment, opaqueContainmentOK := keymatch.Containment(heap, values, opaque)
	if !opaqueOK || !opaqueContainmentOK || opaqueContainment.Kind() != heapdomain.ContainmentUnknown {
		t.Fatal("opaque reference atom was not preserved as unknown containment")
	}

	foreignHeap, foreignValues, _ := fixture(t, "keymatch_containment_foreign", `local object = {}; return 1, object`)
	foreignKey, foreignKeyOK := firstAllocationKey(foreignHeap)
	foreign, foreignOK := foreignValues.Allocation(foreignKey, materialization.Recent)
	if !foreignKeyOK || !foreignOK || !foreignValues.OwnsHeapSchema(foreignHeap) {
		t.Fatal("foreign containment fixture")
	}
	if _, ok := keymatch.Containment(heap, foreignValues, foreign); ok {
		t.Fatal("foreign atom crossed containment owner fence")
	}
}

func TestProjectTopSupportIsCompleteDeterministicAndSchemaFenced(t *testing.T) {
	heap, values, _ := fixture(t, "keymatch_top", `local left = {}; local right = {}; return left, right`)
	collect := func() (bool, []keymatch.Alternative, bool) {
		complete, invalid := true, false
		var alternatives []keymatch.Alternative
		if !values.VisitSupport(values.Top(), func(atom valuedomain.Atom) {
			validity := atom.TableKeyValidity()
			invalid = invalid || validity.MayBeInvalid()
			if !validity.MayBeValid() {
				return
			}
			alternative, ok := keymatch.Project(heap, values, atom)
			if !ok {
				complete = false
				return
			}
			alternatives = append(alternatives, alternative)
		}) {
			complete = false
		}
		return invalid, alternatives, complete
	}
	firstInvalid, first, firstOK := collect()
	secondInvalid, second, secondOK := collect()
	if !firstOK || !secondOK || !firstInvalid || !secondInvalid || !reflect.DeepEqual(first, second) || len(first) == 0 {
		t.Fatal("Top projection was incomplete or nondeterministic")
	}
	var kinds runtimekind.Set
	for index := range first {
		kinds |= first[index].Selector().RuntimeKinds()
	}
	wantKinds := runtimekind.All &^ runtimekind.Bit(runtimekind.Nil)
	if kinds != wantKinds {
		t.Fatalf("Top selector kinds=%b, want legal table keys=%b", kinds, wantKinds)
	}
	for index := 0; index < heap.KeyCount(); index++ {
		key, keyOK := heap.KeyAt(index)
		if !keyOK {
			t.Fatal("Heap key")
		}
		for _, role := range materialization.Roles() {
			reference, referenceOK := heap.Reference(key, role)
			if !referenceOK {
				continue
			}
			if !containsChild(first, reference) {
				t.Fatalf("Top omitted schema-issued root/role %d", role)
			}
		}
	}

	otherHeap, otherValues, _ := fixture(t, "keymatch_foreign", `local left = {}; local right = {}; return left, right`)
	key, keyOK := firstAllocationKey(otherHeap)
	atom, atomOK := otherValues.Allocation(key, materialization.Recent)
	reference, role, referenceOK := atom.Reference()
	if !keyOK || !atomOK || !referenceOK || role != materialization.Recent {
		t.Fatal("foreign rooted Value atom")
	}
	if _, ok := keymatch.Project(heap, otherValues, atom); ok {
		t.Fatal("foreign Value schema entered atom projection")
	}
	if _, ok := keymatch.Reference(heap, otherValues, reference, role); ok {
		t.Fatal("foreign Value reference entered Heap mapping")
	}
	if !otherValues.OwnsHeapSchema(otherHeap) {
		t.Fatal("foreign fixture did not retain its Link owner")
	}
}

func TestProjectBootExactReference(t *testing.T) {
	heap, values, linked := bootFixture(t, "keymatch_boot")
	root, rootOK := linked.Host().BootRoots().At(0)
	rootID, rootIDOK := linked.Host().BootRoots().ID(root)
	atom, atomOK := values.BootID(rootID)
	if !rootOK || !rootIDOK || !atomOK || atom.TableKeyValidity() != valuedomain.TableKeyValid {
		t.Fatal("boot exact Value")
	}
	alternative, projected := keymatch.Project(heap, values, atom)
	key, keyOK := heap.KeyForBootID(rootID)
	want, wantOK := heap.Reference(key, materialization.Exact)
	if !projected || alternative.Selector().Kind() != heapdomain.KeySelectorAtom || !keyOK || !wantOK || alternative.Containment().Kind() != heapdomain.ContainmentExact {
		t.Fatal("boot exact root did not retain one exact key alternative")
	}
	got, gotOK := alternative.Containment().Reference()
	selector, selectorOK := alternative.Selector().ReferenceAt(0)
	if !gotOK || !selectorOK || got != want || selector != want {
		t.Fatal("boot exact root lost its contained Heap reference")
	}
}

func sameAlternative(left, right keymatch.Alternative) bool {
	leftSelector, rightSelector := left.Selector(), right.Selector()
	if leftSelector.Kind() != rightSelector.Kind() || leftSelector.RuntimeKinds() != rightSelector.RuntimeKinds() || leftSelector.ExactCount() != rightSelector.ExactCount() || leftSelector.ReferenceCount() != rightSelector.ReferenceCount() {
		return false
	}
	for index := 0; index < leftSelector.ExactCount(); index++ {
		leftKey, leftOK := leftSelector.ExactAt(index)
		rightKey, rightOK := rightSelector.ExactAt(index)
		if !leftOK || !rightOK || leftKey != rightKey {
			return false
		}
	}
	for index := 0; index < leftSelector.ReferenceCount(); index++ {
		leftReference, leftOK := leftSelector.ReferenceAt(index)
		rightReference, rightOK := rightSelector.ReferenceAt(index)
		if !leftOK || !rightOK || leftReference != rightReference {
			return false
		}
	}
	return left.Containment() == right.Containment()
}

func containsChild(alternatives []keymatch.Alternative, want heapdomain.Reference) bool {
	for _, alternative := range alternatives {
		child, ok := alternative.Containment().Reference()
		if ok && child == want {
			return true
		}
	}
	return false
}

// firstAllocationKey selects Heap's owner-issued allocation coordinate. Tests
// intentionally never recover it through Link's retired allocation plane.
func firstAllocationKey(schema heapdomain.Schema) (heapdomain.Key, bool) {
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if ok && key.Kind() == heapdomain.RootAllocation {
			return key, true
		}
	}
	return heapdomain.Key{}, false
}

func fixture(t testing.TB, module, text string) (heapdomain.Schema, *valuedomain.Schema, *link.Link) {
	t.Helper()
	p, err := lualower.Lower(lualower.Source{Name: module + ".lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: module, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("keymatch compilation")
	}
	heap, heapFailure := heapdomain.SealWithArtifacts(linked, keymatchHeapMounts(t, linked, compilation))
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, keymatchValueMounts(t, linked, compilation), structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone {
		t.Fatal("domain schema")
	}
	return heap, values, linked
}

func bootFixture(t testing.TB, module string) (heapdomain.Schema, *valuedomain.Schema, *link.Link) {
	t.Helper()
	p, err := lualower.Lower(lualower.Source{Name: module + ".lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), InitialRoots: []vocabulary.InitialRootSpec{{
		Identity: "GlobalEnvRoot",
		Shape: vocabulary.BootShapeSpec{
			Aggregate: vocabulary.BootAggregateTable,
			Value:     vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: module, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("keymatch compilation")
	}
	heap, heapFailure := heapdomain.SealWithArtifacts(linked, keymatchHeapMounts(t, linked, compilation))
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, keymatchValueMounts(t, linked, compilation), structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone {
		t.Fatal("boot domain schema")
	}
	return heap, values, linked
}

func sourceAtom(t testing.TB, values *valuedomain.Schema, linked *link.Link, match func(valuedomain.Atom) bool) valuedomain.Atom {
	t.Helper()
	boundaryValues := linked.Boundary().Values()
	for index := 0; index < boundaryValues.Count(); index++ {
		value, valueOK := boundaryValues.At(index)
		valueID, valueIDOK := boundaryValues.ID(value)
		fact, factOK := values.SourceValueID(valueID)
		var atom valuedomain.Atom
		if factOK {
			values.VisitSupport(fact, func(candidate valuedomain.Atom) { atom = candidate })
		}
		if valueOK && valueIDOK && factOK && values.OwnsAtom(atom) && match(atom) {
			return atom
		}
	}
	t.Fatal("matching source atom")
	return valuedomain.Atom{}
}

func keymatchHeapMounts(t testing.TB, linked *link.Link, compilation composite.Compilation) []programmount.MountedArtifact {
	t.Helper()
	heapMounts, _ := keymatchMountedArtifacts(t, linked, compilation)
	return heapMounts
}

func keymatchValueMounts(t testing.TB, linked *link.Link, compilation composite.Compilation) []programmount.MountedArtifact {
	t.Helper()
	_, valueMounts := keymatchMountedArtifacts(t, linked, compilation)
	return valueMounts
}

func keymatchMountedArtifacts(t testing.TB, linked *link.Link, compilation composite.Compilation) ([]programmount.MountedArtifact, []programmount.MountedArtifact) {
	t.Helper()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilation.Available() || !executionSchemaID.Available() || !issuanceOK || linked == nil || linked.Project() == nil {
		t.Fatal("keymatch artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	heapMounts := make([]programmount.MountedArtifact, projectMounts.Count())
	valueMounts := make([]programmount.MountedArtifact, projectMounts.Count())
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		_, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("keymatch artifact mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("keymatch artifact: %v", failure)
		}
		var heapOK, valueOK bool
		heapMounts[index], heapOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		valueMounts[index], valueOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		if !heapOK || !valueOK {
			t.Fatal("keymatch artifact mount receipt")
		}
	}
	return heapMounts, valueMounts
}
