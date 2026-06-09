package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallEntryArgProjectionCachesNestedProductResult(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "make"}}
	arg := &ast.CastExpr{Expr: call}
	refPath := constraint.NewPlaceholder(0).Field("handler")
	table := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0}
	key := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 1}

	calls := 0
	projection := callEntryArgProjection{
		evidence: callEntryArgEvidence{
			nestedCall: func(*ast.FuncCallExpr) transfer.ProductCallContext {
				return transfer.ProductCallContext{}
			},
		},
	}
	projection.evidence.nestedResult = cachedNestedCallResult(
		func(*ast.FuncCallExpr, transfer.ProductCallContext) transfer.ProductCallResult {
			calls++
			result := transfer.EmptyProductCallResult()
			result.ReturnValues = []product.AbstractValue{product.FromType(typ.Any)}
			result.HasReturnValues = true
			result.Boundary.ReturnRefs = flow.ReturnRefsOfSlots([]flow.ReturnRefSlot{
				flow.ReturnRefSlotOf(
					flow.WithFunctionRefPath(nil, refPath, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 44})),
					flow.ClosureRefsDomain.Bottom(),
				),
			})
			result.Boundary.BoundaryFacts = flow.BoundaryFactsFromParts(flow.BoundaryFactParts{
				KeyPresence: []flow.BoundaryKeyPresenceFact{{
					Table: table,
					Key:   key,
				}},
			})
			return result
		},
		projection.evidence.nestedCall,
	)

	refs, ok := projection.argRefTrees(arg)
	if !ok {
		t.Fatalf("argRefTrees missing: %#v", refs)
	}
	if set, ok := flow.FunctionRefAtPath(refs.FunctionRefs(), refPath); !ok || set.IsTop() || len(set.Refs()) != 1 {
		t.Fatalf("argRefTrees function refs = %#v/%v, want singleton", set, ok)
	}
	if facts, ok := projection.argBoundaryFacts(arg); !ok || !facts.HasProof() {
		t.Fatalf("argBoundaryFacts = %#v/%v, want finite boundary facts", facts, ok)
	}
	if calls != 1 {
		t.Fatalf("nested ProductCallResult selections = %d, want 1", calls)
	}
}
