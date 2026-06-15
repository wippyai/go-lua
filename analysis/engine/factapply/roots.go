package factapply

import (
	"math"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func applyRootAssignmentFact(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.RootAssignment,
) (state.State, bool) {
	declared, hasDeclared := fact.DeclaredValue()
	out, targetPath, applied := applyRootAssignment(ctx, resolver, sources, read, in, out, fact.TargetSymbol(), fact.TargetPath(), fact.Source(), declared, hasDeclared, fact.DeclaredValueContracts())
	if applied {
		out = applyRootAssignmentPathProof(ctx, resolver, facts, out, targetPath, fact.Source())
		out = applyRootAssignmentNumFloor(ctx, resolver, facts, in, out, targetPath, fact.Source())
		out = applyObjectLiteralEntries(ctx, resolver, facts, sources, read, in, out, targetPath, fact.Source())
	}
	return out, applied
}

func applyRootAssignment(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	target symbol.ID,
	targetPath pathdom.Path,
	source factflow.ValueSource,
	declared product.Value,
	hasDeclared bool,
	declaredContracts bool,
) (state.State, pathdom.Path, bool) {
	root, ok := rootAssignmentTarget(target, targetPath)
	if !ok {
		return out, pathdom.Path{}, false
	}
	var value product.Value
	if hasDeclared && declaredContracts {
		value = declared
	} else if sourceValue, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out)); ok {
		value = sourceValue
	} else {
		if !hasDeclared {
			return out, pathdom.Path{}, false
		}
		value = declared
	}
	targetPath = rootAssignmentPath(root, targetPath)
	return writeRootSymbol(ctx, resolver, out, root, targetPath, value), targetPath, true
}

func rootAssignmentTarget(target symbol.ID, targetPath pathdom.Path) (symbol.ID, bool) {
	if len(targetPath.Segments) != 0 {
		return 0, false
	}
	if target != 0 {
		return target, true
	}
	if targetPath.Symbol != 0 {
		return targetPath.Symbol, true
	}
	return 0, false
}

func rootAssignmentPath(target symbol.ID, targetPath pathdom.Path) pathdom.Path {
	out := copyPath(targetPath)
	if out.Symbol == 0 {
		out.Symbol = target
	}
	return out
}

func writeRootSymbol(ctx transfer.NodeContext, resolver *visibility.Resolver, out state.State, target symbol.ID, targetPath pathdom.Path, value product.Value) state.State {
	if target == 0 {
		return out
	}
	if resolver != nil {
		if invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, targetPath); ok {
			out = invalidated
		}
	}
	return out.WriteValue(ctx.Registry, key.SymbolValue(target), value)
}

func applyRootAssignmentPathProof(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || !source.HasExpr || targetPath.Symbol == 0 {
		return out
	}
	sourcePath, ok := facts.ExpressionPath(source.ExprRef)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return out
	}
	targetKey := resolver.KeyAt(ctx.Point, targetPath)
	sourceKey := resolver.KeyAt(ctx.Point, sourcePath)
	if targetKey == "" || sourceKey == "" || targetKey == sourceKey {
		return out
	}
	return out.AddBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  targetKey,
		Other: sourceKey,
	})
}

func applyRootAssignmentNumFloor(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 || len(targetPath.Segments) != 0 {
		return out
	}
	targetKey := numFloorKeyAt(resolver, ctx.Point, targetPath)
	if targetKey == "" {
		return out
	}
	if floor, ok := numFloorForSource(ctx, resolver, facts, in, source, nil); ok {
		return out.WriteNumFloor(targetKey, floor)
	}
	return out.ClearNumFloor(targetKey)
}

func numFloorForSource(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
	active map[factflow.ExprRef]bool,
) (int64, bool) {
	if resolver == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return 0, false
	}
	if value, ok := facts.ExpressionValue(source.ExprRef); ok {
		if floor, ok := exactIntegerValue(ctx.Registry, value); ok {
			return floor, true
		}
	}
	if p, ok := facts.ExpressionPath(source.ExprRef); ok {
		pathKey := numFloorKeyAt(resolver, ctx.Point, p)
		if pathKey != "" {
			if floor, ok := in.ReadNumFloor(pathKey); ok {
				return floor, true
			}
		}
	}
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok {
		return 0, false
	}
	if active[source.ExprRef] {
		return 0, false
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[source.ExprRef] = true
	defer delete(active, source.ExprRef)
	return numFloorForOperation(ctx, resolver, facts, in, op, active)
}

func numFloorKeyAt(resolver *visibility.Resolver, point cfg.Point, p pathdom.Path) pathdom.PathKey {
	if p.Symbol == 0 {
		return ""
	}
	if len(p.Segments) == 0 {
		return p.Key()
	}
	if resolver == nil {
		return ""
	}
	return resolver.KeyAt(point, p)
}

func numFloorForOperation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	op factflow.ExpressionOperation,
	active map[factflow.ExprRef]bool,
) (int64, bool) {
	if op.Kind() != factflow.ExpressionOperationBinary {
		return 0, false
	}
	switch op.Op() {
	case "+":
		if floor, ok := numFloorPlusConstant(ctx, resolver, facts, in, op.Left(), op.Right(), active); ok {
			return floor, true
		}
		return numFloorPlusConstant(ctx, resolver, facts, in, op.Right(), op.Left(), active)
	case "-":
		c, ok := exactIntegerSource(ctx, facts, op.Right())
		if !ok {
			return 0, false
		}
		left, ok := numFloorForSource(ctx, resolver, facts, in, op.Left(), active)
		if !ok {
			return 0, false
		}
		return checkedAddInt64(left, -c)
	default:
		return 0, false
	}
}

func numFloorPlusConstant(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	source factflow.ValueSource,
	constant factflow.ValueSource,
	active map[factflow.ExprRef]bool,
) (int64, bool) {
	c, ok := exactIntegerSource(ctx, facts, constant)
	if !ok {
		return 0, false
	}
	floor, ok := numFloorForSource(ctx, resolver, facts, in, source, active)
	if !ok {
		return 0, false
	}
	return checkedAddInt64(floor, c)
}

func exactIntegerSource(ctx transfer.NodeContext, facts factflow.Facts, source factflow.ValueSource) (int64, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return 0, false
	}
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		return 0, false
	}
	return exactIntegerValue(ctx.Registry, value)
}

func exactIntegerValue(reg *axis.Registry, value product.Value) (int64, bool) {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return 0, false
	}
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		return 0, false
	}
	v, ok := lit.Value.(int64)
	return v, ok
}

func checkedAddInt64(a, b int64) (int64, bool) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, false
	}
	return a + b, true
}
