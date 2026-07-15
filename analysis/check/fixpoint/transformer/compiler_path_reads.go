package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func exactBoundaryPathBinding(ctx planCompileContext, p pathdom.Path) (BoundaryPathBinding, error) {
	if p.Symbol == 0 || p.Version != 0 {
		return BoundaryPathBinding{}, fmt.Errorf("path is not a canonical lexical root")
	}
	var root Root
	if index, ok := ctx.plan.BoundaryParamIndex(p.Symbol); ok {
		root = Root{Kind: RootParam, Index: uint32(index)}
	} else if index, ok := ctx.plan.BoundaryCaptureIndex(p.Symbol); ok {
		if !ctx.allowLexicalBoundaryRoots {
			return BoundaryPathBinding{}, fmt.Errorf("symbol %d is not a boundary parameter", p.Symbol)
		}
		root = Root{Kind: RootCapture, Index: uint32(index)}
	} else if index, ok := ctx.plan.BoundaryGlobalIndex(p.Symbol); ok {
		if !ctx.allowLexicalBoundaryRoots {
			return BoundaryPathBinding{}, fmt.Errorf("symbol %d is not a boundary parameter", p.Symbol)
		}
		root = Root{Kind: RootGlobal, Index: uint32(index)}
	} else {
		return BoundaryPathBinding{}, fmt.Errorf("symbol %d is not a lexical boundary root", p.Symbol)
	}
	owner, ok := ctx.locals[p.Symbol]
	if !ok || owner == 0 {
		return BoundaryPathBinding{}, fmt.Errorf("boundary root %d has no exact current value", p.Symbol)
	}
	return BoundaryPathBinding{Symbol: p.Symbol, Root: root, Owner: owner}, nil
}

func exactCompilerDynamicReadTerm(ctx planCompileContext, read factflow.DynamicIndexExpression, active map[factflow.ExprRef]bool) (ValueTerm, error) {
	tablePath := read.TablePathRef()
	if tablePath.IsEmpty() {
		// A direct table source may be exact as a value, but without a canonical
		// path it cannot preserve flow-sensitive read evidence at the caller.
		return 0, fmt.Errorf("table source has no canonical boundary path")
	}
	binding, err := exactBoundaryPathBinding(ctx, tablePath)
	if err != nil {
		return 0, fmt.Errorf("table: %w", err)
	}
	key, err := exactCompilerSourceTermActive(ctx, read.KeySource(), active)
	if err != nil {
		return 0, fmt.Errorf("key: %w", err)
	}
	term, _, err := ctx.builder.Arena().LowerBoundaryDynamicReadValue(tablePath, binding, key)
	if err != nil {
		return 0, err
	}
	return term, nil
}
