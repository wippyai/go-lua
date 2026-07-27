package factapply

import (
	"context"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestResolvedNestedDynamicIndexMutationPublishesOnlySuffixedTarget(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9191)
	tableSymbol := symbol.ID(9191)
	tablePath := pathdom.NewPath(tableSymbol, "items")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9192, HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9193, HasExpr: true}
	suffix := []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}
	target := factflow.NewDynamicIndexTarget(tablePath, keySource, suffix)
	write := factflow.NewDynamicIndexWrite(target, valueSource, dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackKeyAndValue)
	invalidation := factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(target)
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexWrites:          map[cfg.Point]factflow.DynamicIndexWrite{point: write},
		PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{point: invalidation},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, tableSymbol, "items")
	resolver := visibility.NewResolver(builder.Build())
	keyValue := typevalue.LiteralString(reg, "active")
	valueValue := typevalue.LiteralString(reg, "timer")
	result, err := runResolvedIndexMutation(ResolvedIndexMutationFreezeRequest{
		Context: transfer.NodeContext{Registry: reg, Point: point}, Resolver: resolver, Facts: facts,
		Input: state.State{}, Output: state.State{}, Invalidation: invalidation, Write: write,
		KeyValue: keyValue, Value: valueValue, HasKeyValue: true, HasValue: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	precise := tablePath.IndexStr("active").Field("value")
	preciseKey, keyOK := visibility.AddressAt(resolver, point, precise).VisibleKeyspaceKey()
	got, ok := result.Output.ReadLocalPathStaticMember(preciseKey)
	if !keyOK || !ok || !product.Equal(reg, got, valueValue) {
		t.Fatalf("nested dynamic write target = %#v/%t, want exact suffixed value", got, ok)
	}
	if got := result.Output.DynamicIndexFactsSnapshot(); got.Top || len(got.Facts) != 0 {
		t.Fatalf("nested dynamic write published false direct-table facts: %#v", got)
	}
}

func TestResolvedIndexMutationMatchesAuthoritativeN3N4OnEveryLane(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9201)
	tableSymbol := symbol.ID(9201)
	tablePath := pathdom.NewPath(tableSymbol, "items")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9202, HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9203, HasExpr: true}
	keyValue, valueValue := presentValue(reg), absentValue(reg)
	write := factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(tablePath, keySource, nil), valueSource,
		dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackKeyAndValue)
	invalidation := factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(factflow.NewDynamicIndexTarget(tablePath, keySource, nil))
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexWrites:          map[cfg.Point]factflow.DynamicIndexWrite{point: write},
		PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{point: invalidation},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, tableSymbol, "items")
	resolver := visibility.NewResolver(builder.Build())
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	in := state.State{}
	legacySources := &recordingSourceValues{values: map[factflow.ValueSource]product.Value{
		keySource: keyValue, valueSource: valueValue,
	}}
	read := func(cfg.Point) state.State { return in }
	invalidated := applyPathDescendantInvalidation(ctx, resolver, facts, legacySources, read, in, in, invalidation, false)
	want := applyDynamicIndexWrite(ctx, resolver, facts, legacySources, read, in, invalidated, write)
	closed, ok := freezeResolvedDynamicIndexWrite(ctx, resolver, facts, legacySources, read, in, invalidated, write)
	if !ok {
		t.Fatal("failed to freeze closed dynamic-index write")
	}
	internsBefore := resolver.KeySpace().InternSize()
	closedOut, ok := ApplyResolvedDynamicIndexWrite(reg, resolver.KeySpace(), invalidated, closed)
	if !ok {
		t.Fatal("closed dynamic-index write rejected")
	}
	if internsAfter := resolver.KeySpace().InternSize(); internsAfter != internsBefore {
		t.Fatalf("closed dynamic-index Apply grew keyspace: %d -> %d", internsBefore, internsAfter)
	}
	// Freeze owns all caller-variable storage and resolved products. Mutating
	// the source provider and syntax path after Freeze cannot alter Apply.
	legacySources.values[keySource] = absentValue(reg)
	legacySources.values[valueSource] = presentValue(reg)
	tablePath.Root = "mutated-after-freeze"
	closedAfterMutation, ok := ApplyResolvedDynamicIndexWrite(reg, resolver.KeySpace(), invalidated, closed)
	if !ok {
		t.Fatal("closed dynamic-index write rejected after producer mutation")
	}

	got, err := runResolvedIndexMutation(ResolvedIndexMutationFreezeRequest{
		Context: ctx, Resolver: resolver, Facts: facts, Input: in, Output: in,
		Invalidation: invalidation, Write: write,
		KeyValue: keyValue, Value: valueValue, HasKeyValue: true, HasValue: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Applied || got.Canceled {
		t.Fatalf("resolved transaction = applied:%v canceled:%v", got.Applied, got.Canceled)
	}
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	if len(lanes) != 17 {
		t.Fatalf("lane inventory width = %d, want 17", len(lanes))
	}
	productDomain := state.RegisteredProductDomain(reg)
	beforeFactors, err := productDomain.Decompose(in)
	if err != nil {
		t.Fatal(err)
	}
	afterFactors, err := productDomain.Decompose(got.Output)
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeFactors) != len(lanes) || len(afterFactors) != len(lanes) {
		t.Fatalf("factor inventory = %d/%d, want %d", len(beforeFactors), len(afterFactors), len(lanes))
	}
	for i, lane := range lanes {
		domain, err := state.TryDomainWithLanes(reg, []state.LaneID{lane})
		if err != nil {
			t.Fatal(err)
		}
		if !domain.Equal(want, got.Output) {
			t.Fatalf("resolved transaction differs from N3+N4 on lane %q", lane)
		}
		if !domain.Equal(want, closedOut) {
			t.Fatalf("closed dynamic-index write differs from authoritative path on lane %q", lane)
		}
		if !domain.Equal(closedOut, closedAfterMutation) {
			t.Fatalf("producer mutation changed frozen dynamic-index write on lane %q", lane)
		}
		equal, err := productDomain.LaneEqual(beforeFactors[i], afterFactors[i])
		if err != nil {
			t.Fatal(err)
		}
		wantChanged := !domain.Equal(in, want)
		if beforeFactors[i].Lane().ID() != lane || afterFactors[i].Lane().ID() != lane || !equal != wantChanged {
			t.Fatalf("lane factor %d = %q/%q changed=%v, want %q changed=%v", i, beforeFactors[i].Lane().ID(), afterFactors[i].Lane().ID(), !equal, lane, wantChanged)
		}
	}
}

func TestResolvedIndexMutationCancellationAndMissingResolutionPublishNothing(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9210)
	tableSymbol := symbol.ID(9210)
	tablePath := pathdom.NewPath(tableSymbol, "items")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9211, HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9212, HasExpr: true}
	write := factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(tablePath, keySource, nil), valueSource,
		dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackKeyAndValue)
	invalidation := factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(write.TargetRef())
	builder := visibility.NewBuilder()
	builder.Define(point, tableSymbol, "items")
	resolver := visibility.NewResolver(builder.Build())
	before := state.State{}.WriteValue(reg, key.SymbolValue(tableSymbol), presentValue(reg))
	ctx, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	request := ResolvedIndexMutationFreezeRequest{
		Context:  transfer.NodeContext{Context: ctx, Session: session, Registry: reg, Point: point},
		Resolver: resolver, Input: before, Output: before,
		Invalidation: invalidation, Write: write,
		KeyValue: presentValue(reg), Value: presentValue(reg), HasKeyValue: true, HasValue: true,
	}
	got, err := runResolvedIndexMutation(request)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Canceled || got.Applied || !state.Domain(reg).Equal(got.Output, before) {
		t.Fatalf("canceled transaction published: %#v", got)
	}
	request.Context.Session = nil
	request.Context.Context = nil
	request.HasValue = false
	got, err = runResolvedIndexMutation(request)
	if err == nil || got.Applied || !state.Domain(reg).Equal(got.Output, before) {
		t.Fatalf("partial resolution published: result=%#v err=%v", got, err)
	}
}

func TestResolvedIndexMutationRetainsCanonicalAppendAndPlacementProofGates(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9220)
	tableSymbol := symbol.ID(9220)
	tablePath := pathdom.NewPath(tableSymbol, "items")
	lenExpr, keyExpr := factflow.ExprRef(9221), factflow.ExprRef(9222)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("invalid source shape")
	}
	tableSource, _ := factflow.NewPathValueSource(tablePath.Key(), -1, -1, 0, shape)
	lenSource, _ := factflow.NewExpressionValueSource(lenExpr, -1, -1, 0, shape)
	oneSource, _ := factflow.NewIntegerLiteralValueSource(1, -1, -1, 0, shape)
	keySource, _ := factflow.NewExpressionValueSource(keyExpr, -1, -1, 0, shape)
	valueSource, _ := factflow.NewExpressionValueSource(9223, -1, -1, 0, shape)
	lenOp, _ := factflow.NewUnaryExpressionOperation("#", tableSource)
	keyOp, _ := factflow.NewBinaryExpressionOperation("+", lenSource, oneSource)
	write := factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(tablePath, keySource, nil), valueSource,
		dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackValue)
	invalidation := factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(write.TargetRef())
	facts := factflow.NewFacts(factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{
		lenExpr: lenOp, keyExpr: keyOp,
	}})
	builder := visibility.NewBuilder()
	builder.Define(point, tableSymbol, "items")
	resolver := visibility.NewResolver(builder.Build())
	tableKey, ok := resolver.StateKeyAt(point, tablePath)
	if !ok {
		t.Fatal("missing table state key")
	}
	tableID := identity.ID{Kind: "table", Site: "resolved-mutation", Index: 1}
	valueID := identity.ID{Kind: "table", Site: "resolved-mutation", Index: 2}
	tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
	valueValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(valueID))
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(tableSymbol), tableValue).
		WritePlacement(tableID, placement.SharedHeap).
		WritePlacement(valueID, placement.Stack).
		WriteLenFloor(resolver.KeySpace(), tableKey, 2)
	request := ResolvedIndexMutationFreezeRequest{
		Context: transfer.NodeContext{Registry: reg, Point: point}, Resolver: resolver,
		Facts: facts, Input: in, Output: in, Invalidation: invalidation, Write: write,
		Value: valueValue, HasValue: true,
	}
	proved, err := runResolvedIndexMutation(request)
	if err != nil {
		t.Fatal(err)
	}
	if floor, ok := proved.Output.ReadLenFloor(resolver.KeySpace(), tableKey); !ok || floor != 3 {
		t.Fatalf("proved append len floor = %d/%v, want 3", floor, ok)
	}
	if got := proved.Output.ReadPlacement(valueID); got != placement.SharedHeap {
		t.Fatalf("stored reachable placement = %v, want shared heap", got)
	}

	// Removing the structural append proof and heap-owner placement proof must
	// not be replaceable by a symbolic request flag: the concrete gates remain
	// authoritative.
	unprovedIn := in.WritePlacement(tableID, placement.Stack)
	request.Facts = factflow.NewFacts(factflow.FactsInput{})
	request.Input, request.Output = unprovedIn, unprovedIn
	unproved, err := runResolvedIndexMutation(request)
	if err != nil {
		t.Fatal(err)
	}
	if floor, ok := unproved.Output.ReadLenFloor(resolver.KeySpace(), tableKey); ok {
		t.Fatalf("unproved append retained invalidated len floor = %d", floor)
	}
	if got := unproved.Output.ReadPlacement(valueID); got != placement.Stack {
		t.Fatalf("stack-owned table escaped stored value: %v", got)
	}
}

func runResolvedIndexMutation(request ResolvedIndexMutationFreezeRequest) (ResolvedIndexMutationResult, error) {
	artifact, err := FreezeResolvedIndexMutation(request)
	if err != nil {
		return ResolvedIndexMutationResult{Output: request.Output}, err
	}
	if artifact.data == nil {
		return ResolvedIndexMutationResult{Output: request.Output}, fmt.Errorf("freeze produced no closed program")
	}
	var token *cancellation.Token
	if request.Context.Session != nil {
		token = request.Context.Session.Token()
	}
	result, err := ApplyResolvedIndexMutation(artifact, token, request.Input, request.Output)
	return result, err
}
