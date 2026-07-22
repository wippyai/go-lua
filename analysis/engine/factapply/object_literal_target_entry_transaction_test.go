package factapply

import (
	"errors"
	"fmt"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestObjectLiteralTargetEntryTransactionMatchesConcreteOrderedPublication(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	point := cfg.Point(911)
	targetSymbol := symbol.ID(911)
	target := pathdom.NewPath(targetSymbol, "container").Field("target")
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9110, HasExpr: true}
	nestedSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9111, HasExpr: true}
	rawSources := make([]factflow.ValueSource, 11)
	values := make(map[factflow.ValueSource]product.Value, len(rawSources)+1)
	for index := range rawSources {
		rawSources[index] = factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(9120 + index), HasExpr: true}
		if index != 7 { // Pin unavailable-entry omission as part of the old law.
			if index&1 == 0 {
				values[rawSources[index]] = presentValue(reg)
			} else {
				values[rawSources[index]] = absentValue(reg)
			}
		}
	}
	rootID, nestedID := testTableLiteralID(rootSource.ExprRef), testTableLiteralID(nestedSource.ExprRef)
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	nestedValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(nestedID))
	values[rootSource], values[nestedSource] = rootValue, nestedValue
	indexSuffix := func(index int) pathdom.Path {
		return pathdom.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexInt, Index: index}}}
	}
	rootEntries := []factflow.ObjectEntry{
		factflow.NewObjectEntryWithMetadata(indexSuffix(1), rawSources[0], factflow.SourceSpan{}, ""),
		factflow.NewObjectEntryWithMetadata(indexSuffix(2), rawSources[1], factflow.SourceSpan{}, ""),
	}
	for index := 2; index < 9; index++ {
		rootEntries = append(rootEntries, factflow.NewObjectEntryWithMetadata(fieldSuffix(fmt.Sprintf("member%d", index)), rawSources[index], factflow.SourceSpan{}, ""))
	}
	rootEntries = append(rootEntries,
		factflow.NewObjectEntryWithMetadata(fieldSuffix("child"), nestedSource, factflow.SourceSpan{}, ""),
		factflow.NewObjectEntryWithMetadata(fieldSuffix("child_alias"), nestedSource, factflow.SourceSpan{}, ""),
		factflow.NewObjectEntryWithMetadata(fieldSuffix("late"), rawSources[9], factflow.SourceSpan{}, ""),
	)
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, targetSymbol, target, rootSource),
		},
		ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
			rootSource.ExprRef: factflow.NewObjectLiteral(rootEntries).WithIdentity(rootID).WithStaticStringKeysComplete(),
			nestedSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(fieldSuffix("left"), rawSources[9], factflow.SourceSpan{}, ""),
				factflow.NewObjectEntryWithMetadata(fieldSuffix("right"), rawSources[10], factflow.SourceSpan{}, ""),
			}).WithIdentity(nestedID).WithStaticStringKeysComplete(),
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, targetSymbol, "container")
	resolver := visibility.NewResolver(builder.Build())

	plan, err := PrepareObjectLiteralTargetEntryPlan(reg, typeValues, resolver, facts, point, target, rootSource)
	if err != nil {
		t.Fatal(err)
	}
	rootTransaction, ok := PlanRootAssignmentTransaction(facts, point)
	if !ok || !plan.MatchesRootAssignmentSourceInventory(rootTransaction) {
		t.Fatal("object target-entry sources diverged from the frozen root-assignment inventory")
	}
	lateIndex, ok := plan.ValueSourceIndex(rawSources[9])
	if !ok || lateIndex <= 8 {
		t.Fatalf("late source index = %d/%t, want an uncapped ordinal > 8", lateIndex, ok)
	}
	row := make([]luasourcevalue.ObjectLiteralPlanValue, plan.ValueSourceCount())
	for index := range row {
		source, sourceOK := plan.ValueSourceAt(index)
		if !sourceOK {
			t.Fatalf("source %d missing", index)
		}
		row[index].Value, row[index].Available = values[source]
	}
	transaction, err := ResolveObjectLiteralTargetEntryTransaction(reg, plan, row)
	if err != nil {
		t.Fatal(err)
	}
	rootObject := plan.objects[plan.rootObject]
	rootRow := make([]luasourcevalue.ObjectLiteralPlanValue, len(rootObject.localSources))
	for local, global := range rootObject.localSources {
		rootRow[local] = row[global]
	}
	wantRoot, composed := luasourcevalue.ComposeObjectLiteralPlanCached(reg, typeValues, rootObject.literal, rootRow)
	gotRoot, rootPresent := transaction.RootSourceValue()
	if !composed || !rootPresent || !product.Equal(reg, gotRoot, wantRoot) {
		t.Fatal("transaction lexical root diverged from canonical concrete object composition")
	}
	if len(plan.objects) != 2 {
		t.Fatalf("recursive plan objects = %d, want root plus one deduplicated nested object", len(plan.objects))
	}
	reorderedEntries := append([]factflow.ObjectEntry(nil), rootEntries...)
	for left, right := 0, len(reorderedEntries)-1; left < right; left, right = left+1, right-1 {
		reorderedEntries[left], reorderedEntries[right] = reorderedEntries[right], reorderedEntries[left]
	}
	reorderedFacts := factflow.NewFacts(factflow.FactsInput{ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
		rootSource.ExprRef: factflow.NewObjectLiteral(reorderedEntries).WithIdentity(rootID).WithStaticStringKeysComplete(),
		nestedSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
			factflow.NewObjectEntryWithMetadata(fieldSuffix("left"), rawSources[9], factflow.SourceSpan{}, ""),
			factflow.NewObjectEntryWithMetadata(fieldSuffix("right"), rawSources[10], factflow.SourceSpan{}, ""),
		}).WithIdentity(nestedID).WithStaticStringKeysComplete(),
	}})
	reorderedPlan, err := PrepareObjectLiteralTargetEntryPlan(reg, typeValues, resolver, reorderedFacts, point, target, rootSource)
	if err != nil {
		t.Fatal(err)
	}
	if len(reorderedPlan.objects) != 2 || reorderedPlan.rootObject < 0 || reorderedPlan.rootObject >= len(reorderedPlan.objects) {
		t.Fatal("reordered recursive plan lost its explicit lexical-root ordinal or nested deduplication")
	}
	reorderedRow := make([]luasourcevalue.ObjectLiteralPlanValue, reorderedPlan.ValueSourceCount())
	for index := range reorderedRow {
		source, sourceOK := reorderedPlan.ValueSourceAt(index)
		if !sourceOK {
			t.Fatalf("reordered source %d missing", index)
		}
		reorderedRow[index].Value, reorderedRow[index].Available = values[source]
	}
	reorderedTransaction, err := ResolveObjectLiteralTargetEntryTransaction(reg, reorderedPlan, reorderedRow)
	if err != nil {
		t.Fatal(err)
	}
	reorderedRoot, reorderedRootOK := reorderedTransaction.RootSourceValue()
	if !reorderedRootOK || !product.Equal(reg, reorderedRoot, gotRoot) {
		t.Fatal("lexical root composition changed when child visitation order changed")
	}
	if transaction.EntryCount() != len(rootEntries)-1 { // rawSources[7] is unavailable.
		t.Fatalf("ordered writes = %d, want %d", transaction.EntryCount(), len(rootEntries)-1)
	}
	wantTargets := make([]pathdom.Path, 0, len(rootEntries)-1)
	for index, entry := range rootEntries {
		if index == 7 { // rootEntries[7] is rawSources[7].
			continue
		}
		wantTargets = append(wantTargets, target.AppendSegments(entry.Suffix().Segments))
	}
	for index, want := range wantTargets {
		write, writeOK := transaction.EntryAt(index)
		if !writeOK || !write.Target.Equal(want) {
			t.Fatalf("write %d target = %s/%t, want %s", index, write.Target.String(), writeOK, want.String())
		}
	}

}

func TestGuardedObjectConstructorImportsCanonicalPlanIntoInvocationKeySpace(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(926)
	targetSymbol := symbol.ID(926)
	target := pathdom.NewPath(targetSymbol, "target")
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9260, HasExpr: true}
	memberSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9261, HasExpr: true}
	rootID := testTableLiteralID(rootSource.ExprRef)
	facts := factflow.NewFacts(factflow.FactsInput{ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
		rootSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
			factflow.NewObjectEntryWithMetadata(fieldSuffix("member"), memberSource, factflow.SourceSpan{}, ""),
		}).WithIdentity(rootID).WithStaticStringKeysComplete(),
	}})
	lexicalBuilder := visibility.NewBuilder()
	lexicalBuilder.Define(point, targetSymbol, "target")
	lexical := visibility.NewResolver(lexicalBuilder.Build())
	invocationBuilder := visibility.NewBuilder()
	invocationBuilder.Define(point, targetSymbol, "target")
	invocation := visibility.NewResolver(invocationBuilder.Build())
	if lexical.KeySpace() == invocation.KeySpace() {
		t.Fatal("test requires pointer-distinct lexical and invocation keyspaces")
	}
	plan, err := PrepareObjectLiteralTargetEntryPlan(reg, typevalue.NewCache(), lexical, facts, point, target, rootSource)
	if err != nil {
		t.Fatal(err)
	}
	member := presentValue(reg)
	values := make([]product.Value, plan.ValueSourceCount())
	for index := range values {
		source, ok := plan.ValueSourceAt(index)
		if !ok {
			t.Fatalf("source %d missing", index)
		}
		values[index] = member
		if source == rootSource {
			values[index] = product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
		}
	}
	domain := state.RegisteredProductDomain(reg)
	prepared, err := plan.PrepareGuardedObjectConstructor(domain, invocation.KeySpace(), values)
	if err != nil {
		t.Fatal(err)
	}
	constructor, rows, preparedOK := prepared.ObjectConstructor()
	root, rootOK := prepared.RootSourceValue()
	if !preparedOK || !rootOK {
		t.Fatal("prepared constructor omitted its canonical plan or lexical root")
	}
	declared, err := plan.PrepareObjectConstructorPlan(domain, invocation.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	declaredWrites, err := domain.ObjectConstructorCoordinateWrites(declared)
	if err != nil {
		t.Fatal(err)
	}
	executedWrites, err := domain.ObjectConstructorCoordinateWrites(constructor)
	if err != nil {
		t.Fatal(err)
	}
	declaredInventory, err := domain.SealCoordinateFactorInventory(invocation.KeySpace(), declaredWrites)
	if err != nil {
		t.Fatal(err)
	}
	executedInventory, err := domain.SealCoordinateFactorInventory(invocation.KeySpace(), executedWrites)
	if err != nil {
		t.Fatal(err)
	}
	if declaredInventory.Len() != executedInventory.Len() {
		t.Fatalf("constructor coordinate declaration width = %d, execution width = %d", declaredInventory.Len(), executedInventory.Len())
	}
	for index, declaredSlot := range declaredInventory.Slots() {
		equal, equalErr := domain.CoordinateSlotEqual(declaredSlot, executedInventory.Slots()[index])
		if equalErr != nil || !equal {
			t.Fatalf("constructor coordinate declaration %d differs from execution: %v", index, equalErr)
		}
	}
	heapRoots, err := domain.HeapObjectRootSlotsFromCoordinateInventory(declaredInventory)
	if err != nil || len(heapRoots) == 0 {
		t.Fatalf("constructor declaration has no heap-object root coordinate: %d/%v", len(heapRoots), err)
	}
	for index, root := range heapRoots {
		if !root.IdentityTerm().Valid() {
			t.Fatalf("constructor heap-object root %d has no identity-owned coordinate", index)
		}
	}
	resolved, err := ResolveGuardedObjectLiteralTargetEntryTransaction(reg, plan, values)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, wantRootOK := resolved.RootSourceValue()
	if !wantRootOK || !product.Equal(reg, root, wantRoot) {
		t.Fatal("prepared root diverged from concrete canonical composition")
	}
	got, err := domain.ApplyObjectConstructor(constructor, rows, state.Reachable(state.State{}))
	if err != nil {
		t.Fatal(err)
	}
	assertHeapStaticMember(t, reg, invocation.KeySpace(), got, rootSource.ExprRef, ".member", member)
}

func TestPrepareObjectLiteralTargetEntryPlanRejectsUnconstructableGraph(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(931)
	targetSymbol := symbol.ID(931)
	target := pathdom.NewPath(targetSymbol, "target")
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9310, HasExpr: true}
	nestedSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9311, HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9312, HasExpr: true}
	rootID := testTableLiteralID(rootSource.ExprRef)

	tests := []struct {
		name    string
		objects map[factflow.ExprRef]factflow.ObjectLiteral
	}{
		{
			name: "cycle",
			objects: map[factflow.ExprRef]factflow.ObjectLiteral{
				rootSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntryWithMetadata(fieldSuffix("self"), rootSource, factflow.SourceSpan{}, ""),
				}).WithIdentity(rootID),
			},
		},
		{
			name: "missing nested identity",
			objects: map[factflow.ExprRef]factflow.ObjectLiteral{
				rootSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntryWithMetadata(fieldSuffix("child"), nestedSource, factflow.SourceSpan{}, ""),
					factflow.NewObjectEntryWithMetadata(fieldSuffix("still_ordered"), entrySource, factflow.SourceSpan{}, ""),
				}).WithIdentity(rootID),
				nestedSource.ExprRef: factflow.NewObjectLiteral(nil),
			},
		},
	}

	builder := visibility.NewBuilder()
	builder.Define(point, targetSymbol, "target")
	resolver := visibility.NewResolver(builder.Build())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := factflow.NewFacts(factflow.FactsInput{ObjectLiterals: test.objects})
			plan, err := PrepareObjectLiteralTargetEntryPlan(
				reg, typevalue.NewCache(), resolver, facts, point, target, rootSource,
			)
			if !errors.Is(err, errObjectLiteralTargetGraphUnconstructable) {
				t.Fatalf("PrepareObjectLiteralTargetEntryPlan error = %v, want %v", err, errObjectLiteralTargetGraphUnconstructable)
			}
			if plan.Valid() || plan.ValueSourceCount() != 0 {
				t.Fatal("unconstructable graph retained a publishable object/member plan")
			}
		})
	}
}
