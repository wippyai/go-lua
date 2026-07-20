package factapply

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestModuleLoadTransactionResolvesEvaluatedArgumentIntoExactN0(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(2)
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, _ := factflow.NewPathValueSource("7:module_name", 1, 0, 0, shape)
	operation, ok := operationplan.NewModuleLoadOperation(reg, argument, []operationplan.ModuleLoadExport{
		{Path: "alpha", Value: typevalue.FromType(reg, typ.Number), PostReturnAuthority: true},
		{Path: "beta", Value: typevalue.String(reg)},
	})
	if !ok {
		t.Fatal("module operation")
	}
	plan := operationplan.New(moduleLoadTransactionGraph(), factflow.FactsInput{}).
		WithModuleLoads(map[cfg.Point]operationplan.ModuleLoadOperation{point: operation})
	transaction, ok := PlanModuleLoadTransaction(reg, plan, point)
	if !ok || !transaction.Valid(reg) || !factflow.ValueSourceEqual(transaction.Argument(), argument) ||
		!transaction.OperationID().Available() || !transaction.TableID().Available() {
		t.Fatalf("module transaction = %#v/%v", transaction, ok)
	}

	resolved, ok := transaction.Resolve(reg, typevalue.LiteralString(reg, "alpha"))
	if !ok || !resolved.Valid(reg) || !resolved.Matches(transaction) || !resolved.PostReturnAuthority() {
		t.Fatalf("resolved module transaction = %#v/%v", resolved, ok)
	}
	result := resolved.ResultTransaction()
	if result.Point() != point || result.Len() != 1 || !result.HasMaterializeSteps() || result.HasPostconditionSteps() || result.HasPublicationSteps() {
		t.Fatalf("resolved N0 payload = %#v", result)
	}
	input := state.Domain(reg).Bottom()
	got, err := ApplyResolvedModuleLoadTransaction(context.Background(), reg, resolved, input)
	if err != nil {
		t.Fatal(err)
	}
	resultSlot := key.CallResult(uint32(point), 0)
	if want := typevalue.FromType(reg, typ.Number); !product.Equal(reg, got.ReadValue(reg, resultSlot), want) {
		t.Fatalf("module N0 result = %v, want %v", got.ReadValue(reg, resultSlot), want)
	}
	for name, value := range map[string]product.Value{
		"dynamic":   typevalue.String(reg),
		"missing":   typevalue.LiteralString(reg, "missing"),
		"nonstring": typevalue.FromType(reg, typ.Boolean),
	} {
		if candidate, exact := transaction.Resolve(reg, value); exact || candidate.Valid(reg) {
			t.Fatalf("%s argument minted module authority: %#v/%v", name, candidate, exact)
		}
	}
}

func TestModuleLoadTransactionFencesExportTableVersion(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(2)
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, _ := factflow.NewPathValueSource("7:module_name", 1, 0, 0, shape)
	build := func(value product.Value) ModuleLoadTransaction {
		t.Helper()
		operation, ok := operationplan.NewModuleLoadOperation(reg, argument, []operationplan.ModuleLoadExport{{
			Path: "alpha", Value: value, PostReturnAuthority: true,
		}})
		if !ok {
			t.Fatal("module operation")
		}
		plan := operationplan.New(moduleLoadTransactionGraph(), factflow.FactsInput{}).
			WithModuleLoads(map[cfg.Point]operationplan.ModuleLoadOperation{point: operation})
		transaction, ok := PlanModuleLoadTransaction(reg, plan, point)
		if !ok {
			t.Fatal("module transaction")
		}
		return transaction
	}
	first := build(typevalue.String(reg))
	second := build(typevalue.FromType(reg, typ.Number))
	if first.TableID() == second.TableID() || first.OperationID() == second.OperationID() {
		t.Fatal("changed export table reused a prior semantic version")
	}
	resolved, ok := first.Resolve(reg, typevalue.LiteralString(reg, "alpha"))
	if !ok || resolved.Matches(second) {
		t.Fatal("resolved N0 crossed export-table versions")
	}
}

func TestResolvedModuleLoadTransactionCancellationRollsBackN0(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(2)
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, _ := factflow.NewStringLiteralValueSource("alpha", 0, 0, 0, shape)
	operation, _ := operationplan.NewModuleLoadOperation(reg, argument, []operationplan.ModuleLoadExport{{
		Path: "alpha", Value: typevalue.String(reg), PostReturnAuthority: true,
	}})
	plan := operationplan.New(moduleLoadTransactionGraph(), factflow.FactsInput{}).
		WithModuleLoads(map[cfg.Point]operationplan.ModuleLoadOperation{point: operation})
	transaction, _ := PlanModuleLoadTransaction(reg, plan, point)
	resolved, _ := transaction.Resolve(reg, typevalue.LiteralString(reg, "alpha"))
	input := state.Domain(reg).Bottom().WriteValue(reg, key.CallResult(uint32(point), 0), product.Top())
	ctx, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	got, err := ApplyResolvedModuleLoadTransaction(ctx, reg, resolved, input)
	if !errors.Is(err, context.Canceled) || !state.Domain(reg).Equal(got, input) {
		t.Fatalf("canceled N0 = state:%v err:%v", got, err)
	}
}

func moduleLoadTransactionGraph() cfg.Graph {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	return graph
}
