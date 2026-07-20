package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typecovariant "github.com/wippyai/go-lua/analysis/type/covariant"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPlanCovariantExposureTransactionOwnsExactN6Order(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(43)
	firstPath := pathdom.NewPath(symbol.ID(443), "first").Field("nested")
	secondPath := pathdom.NewPath(symbol.ID(444), "second")
	firstValue := typevalue.FromType(reg, typ.Any)
	secondValue := typevalue.FromType(reg, typ.String)
	facts := factflow.NewFacts(factflow.FactsInput{CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
		point: {
			factflow.NewCovariantExposure(firstPath, firstValue, factflow.CovariantExposureRecord),
			factflow.NewCovariantExposure(secondPath, secondValue, factflow.CovariantExposureArray),
		},
	}})

	transaction := PlanCovariantExposureTransaction(facts, point)
	if transaction.Point() != point || transaction.Len() != 2 || !transaction.HasStateSteps() || !transaction.Valid(reg) {
		t.Fatalf("transaction point/len/state/valid = %d/%d/%t/%t", transaction.Point(), transaction.Len(), transaction.HasStateSteps(), transaction.Valid(reg))
	}
	first, ok := transaction.Step(0)
	if !ok || first.Exposure().Kind() != factflow.CovariantExposureRecord || !first.Exposure().SourcePath().Equal(firstPath) {
		t.Fatal("first transaction member is not the first record exposure")
	}
	second, ok := transaction.Step(1)
	if !ok || second.Exposure().Kind() != factflow.CovariantExposureArray || !second.Exposure().SourcePath().Equal(secondPath) {
		t.Fatal("second transaction member is not the second array exposure")
	}
	if _, ok := transaction.Step(2); ok {
		t.Fatal("transaction exposed an out-of-range step")
	}

	mutated := first.Exposure().SourcePath()
	mutated.Segments[0].Name = "mutated"
	again, _ := transaction.Step(0)
	if !again.Exposure().SourcePath().Equal(firstPath) {
		t.Fatal("covariant-exposure transaction exposed mutable path storage")
	}
}

func TestConcreteCovariantExposureTransactionPreservesExactN6Authority(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(47)
	exposed := symbol.ID(447)
	sourcePath := pathdom.NewPath(exposed, "exposed")
	narrowType := typetable.NewRecord().Field("x", typ.Number).Build()
	wideType := typetable.NewRecord().Field("x", typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})).Build()
	narrowValue := typevalue.WithWitness(reg, typevalue.FromType(reg, narrowType), narrowType)
	wideValue := typevalue.WithWitness(reg, typevalue.FromType(reg, wideType), wideType)
	builder := visibility.NewBuilder()
	builder.Define(point, exposed, "exposed")
	resolver := visibility.NewResolver(builder.Build())
	fieldKey, ok := resolver.StateKeyAt(point, sourcePath.Field("x"))
	if !ok {
		t.Fatal("field path did not resolve through stable visibility authority")
	}
	facts := factflow.NewFacts(factflow.FactsInput{CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
		point: {factflow.NewCovariantExposure(sourcePath, wideValue, factflow.CovariantExposureRecord)},
	}})
	transaction := PlanCovariantExposureTransaction(facts, point)
	input := state.Reachable(state.State{})
	output := input.
		WriteValue(reg, key.SymbolValue(exposed), narrowValue).
		WritePathKey(reg, resolver.KeySpace(), fieldKey.PathKey(), typevalue.LiteralNumber(reg, 1))
	ctx := transfer.NodeContext{Registry: reg, Point: point}

	result := ApplyConcreteCovariantExposureTransaction(ConcreteCovariantExposureRequest{
		Context: ctx, Resolver: resolver, CovariantWiden: typecovariant.WidenRecord,
		Transaction: transaction, Input: input, Output: output,
	})
	if result.Canceled {
		t.Fatal("N6 transaction unexpectedly canceled")
	}
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	throughAuthority, err := authority.ApplyCovariantExposure(context.Background(), reg, transaction, input, output)
	if err != nil || !state.Domain(reg).Equal(throughAuthority, result.Output) {
		t.Fatalf("callback-free N6 authority differs from concrete transaction: err=%v", err)
	}
	gotType, ok := typevalue.TypeOf(reg, result.Output.ReadValue(reg, key.SymbolValue(exposed)))
	if !ok || !typ.TypeEquals(gotType, wideType) {
		t.Fatalf("widened root type = %v/%v, want %v", gotType, ok, wideType)
	}
	if got := result.Output.ReadPathKey(reg, resolver.KeySpace(), fieldKey.PathKey()); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("N6 transaction did not invalidate stale field evidence through visibility authority")
	}

	canceledContext, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	rolledBack, err := authority.ApplyCovariantExposure(canceledContext, reg, transaction, input, output)
	if err == nil {
		t.Fatal("pre-canceled N6 authority did not report cancellation")
	}
	if !state.Domain(reg).Equal(rolledBack, input) {
		t.Fatal("canceled N6 authority did not roll back to immutable point input")
	}
}

func TestConcreteCovariantExposureTransactionCancellationRollsBackPointInput(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(53)
	exposed := symbol.ID(453)
	narrowType := typetable.NewRecord().Field("x", typ.Number).Build()
	wideType := typetable.NewRecord().Field("x", typ.Any).Build()
	narrowValue := typevalue.WithWitness(reg, typevalue.FromType(reg, narrowType), narrowType)
	wideValue := typevalue.WithWitness(reg, typevalue.FromType(reg, wideType), wideType)
	transaction := PlanCovariantExposureTransaction(factflow.NewFacts(factflow.FactsInput{
		CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
			point: {factflow.NewCovariantExposure(pathdom.NewPath(exposed, "exposed"), wideValue, factflow.CovariantExposureRecord)},
		},
	}), point)
	ctx, session := cancellation.Attach(context.Background())
	input := state.Reachable(state.State{})
	output := input.WriteValue(reg, key.SymbolValue(exposed), narrowValue)
	called := false

	result := ApplyConcreteCovariantExposureTransaction(ConcreteCovariantExposureRequest{
		Context: transfer.NodeContext{Context: ctx, Session: session, Registry: reg, Point: point},
		CovariantWiden: func(sourceWitness, contract typ.Type, segments []segment.Segment) (typ.Type, [][]segment.Segment, bool) {
			called = true
			session.Token().Cancel(context.Canceled)
			return testCovariantRecordWiden(sourceWitness, contract, segments)
		},
		Transaction: transaction, Input: input, Output: output,
	})
	if !called || !result.Canceled {
		t.Fatalf("widener called/canceled = %t/%t, want true/true", called, result.Canceled)
	}
	if !state.Domain(reg).Equal(result.Output, input) {
		t.Fatal("canceled N6 transaction published a finalizer prefix instead of point Input")
	}
}
