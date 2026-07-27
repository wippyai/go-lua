package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func exactBoundaryPathBinding(ctx planCompileContext, p pathdom.Path) (BoundaryPathBinding, error) {
	if p.Symbol == 0 || p.Version != 0 {
		return BoundaryPathBinding{}, fmt.Errorf("path is not a canonical lexical root")
	}
	arena := ctx.builder.Arena()
	owner, ok := ctx.locals[p.Symbol]
	if !ok || owner == 0 {
		owner, ok = arena.environmentValue(p.Symbol)
	}
	if !ok || owner == 0 {
		return BoundaryPathBinding{}, fmt.Errorf("lexical root %d has no exact current value", p.Symbol)
	}
	// Structural lowering owns the body's current lexical slots. Keep those
	// paths in the sealed environment namespace even when the symbol was
	// originally a parameter: its current value may have been reassigned, and
	// a call-frame root would address the entry value instead.
	if ctx.structuralEnvironment {
		base := arena.EnvironmentPath(p.Symbol)
		if base == 0 {
			return BoundaryPathBinding{}, fmt.Errorf("symbol %d is not a sealed lexical environment root", p.Symbol)
		}
		return BoundaryPathBinding{Symbol: p.Symbol, Base: base, Owner: owner, Point: ctx.point}, nil
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
		base := arena.EnvironmentPath(p.Symbol)
		if base == 0 {
			return BoundaryPathBinding{}, fmt.Errorf("symbol %d is not a sealed lexical environment root", p.Symbol)
		}
		return BoundaryPathBinding{Symbol: p.Symbol, Base: base, Owner: owner, Point: ctx.point}, nil
	}
	base := arena.Path(root)
	return BoundaryPathBinding{Symbol: p.Symbol, Root: root, Base: base, Owner: owner, Point: ctx.point}, nil
}

func exactCompilerDynamicReadTerm(ctx planCompileContext, read factflow.DynamicIndexExpression, active map[factflow.ExprRef]bool) (ValueTerm, error) {
	keySource := read.KeySource()
	keyPath := PathTerm(0)
	if sourcePath, exact := predicateSourcePath(ctx.facts, keySource); exact && sourcePath.Symbol != 0 && sourcePath.Version == 0 {
		var pathErr error
		keyPath, pathErr = boundaryMemberPathTerm(ctx, sourcePath)
		if pathErr != nil {
			return 0, fmt.Errorf("key path: %w", pathErr)
		}
	}
	shape := indexform.IndexShape{}
	rangePath := PathTerm(0)
	integerProof := ValueTerm(0)
	normalized, normalizedOK := ctx.facts.NormalizeDynamicReadIndexForm(read, func(source factflow.ValueSource) (int64, bool) {
		if source.Kind == factflow.ValueSourceLiteral && source.LiteralKind == factflow.ValueSourceLiteralInteger {
			return source.Int, true
		}
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return 0, false
		}
		value, ok := ctx.facts.ExpressionValue(source.ExprRef)
		if !ok {
			return 0, false
		}
		return typevalue.IntegerLiteralValue(ctx.builder.Arena().reg, value)
	})
	if normalizedOK {
		form, formOK := normalized.Form()
		if formOK {
			shape, normalizedOK = form.Shape()
			if affine, affineOK := form.Affine(); affineOK {
				affinePath, pathOK := affine.Path()
				if !pathOK {
					normalizedOK = false
				} else {
					var rangeErr error
					rangePath, rangeErr = boundaryMemberPathTerm(ctx, affinePath)
					normalizedOK = rangeErr == nil && rangePath != 0
				}
			}
			if form.Kind() == indexform.IndexFormModuloLength {
				dividend, dividendOK := normalized.IntegerCertificateSource()
				if dividendOK {
					integerProof, _ = exactCompilerSourceTermActive(ctx, dividend, active)
				}
				normalizedOK = dividendOK && integerProof != 0
			}
		}
	}
	if !normalizedOK {
		shape, rangePath, integerProof = indexform.IndexShape{}, 0, 0
	}
	tablePath := read.TablePathRef()
	// TablePathRef is an optional producer convenience, not a second source of
	// semantic truth. Recover the same canonical provenance from the table
	// ValueSource that key lowering uses, so named tables never degrade into an
	// unaddressable direct read merely because the duplicate field was omitted.
	if tablePath.IsEmpty() {
		if tableSource, present := read.TableSource(); present {
			if sourcePath, exact := predicateSourcePath(ctx.facts, tableSource); exact && sourcePath.Symbol != 0 && sourcePath.Version == 0 {
				tablePath = sourcePath
			}
		}
	}
	if tablePath.IsEmpty() {
		tableSource, ok := read.TableSource()
		if !ok {
			return 0, fmt.Errorf("dynamic read has neither a table path nor an exact table source")
		}
		table, err := exactCompilerSourceTermActive(ctx, tableSource, active)
		if err != nil {
			return 0, fmt.Errorf("table: %w", err)
		}
		key, err := exactCompilerSourceTermActive(ctx, keySource, active)
		if err != nil {
			return 0, fmt.Errorf("key: %w", err)
		}
		term := ctx.builder.Arena().dynamicReadTableValueAtPaths(ctx.point, table, 0, key, keyPath, shape, rangePath, integerProof)
		if term == 0 {
			return 0, fmt.Errorf("direct dynamic table read term construction failed")
		}
		return term, nil
	}
	binding, err := exactBoundaryPathBinding(ctx, tablePath)
	if err != nil {
		return 0, fmt.Errorf("table: %w", err)
	}
	key, err := exactCompilerSourceTermActive(ctx, keySource, active)
	if err != nil {
		return 0, fmt.Errorf("key: %w", err)
	}
	// A root-only lexical path and its sealed owner denote the same table
	// coordinate. Lower it as a direct-table read so execution consults exact
	// heap/type semantics without demanding a second path-state spelling for a
	// value-only loop or returned-result binding. Descendant paths still need
	// ordinary path projection to reach their table value.
	if len(tablePath.Segments) == 0 {
		term := ctx.builder.Arena().dynamicReadTableValueAtPaths(ctx.point, binding.Owner, binding.Base, key, keyPath, shape, rangePath, integerProof)
		if term == 0 {
			return 0, fmt.Errorf("root dynamic table read term construction failed")
		}
		return term, nil
	}
	pathTerm := ctx.builder.Arena().AppendPath(binding.Base, tablePath.Segments...)
	term := ctx.builder.Arena().dynamicReadValueAtPaths(ctx.point, binding.Owner, pathTerm, key, keyPath, shape, rangePath, integerProof)
	if term == 0 || pathTerm == 0 {
		return 0, fmt.Errorf("dynamic index: boundary read construction failed")
	}
	return term, nil
}
