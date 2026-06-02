// Package assignsource evaluates AST-free assignment RHS evidence against a
// solved flow projection.
package assignsource

import (
	"github.com/wippyai/go-lua/compiler/check/domain/iteration"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	typevalue "github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Flow is the solved-flow proof surface needed to evaluate assignment sources.
type Flow interface {
	NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type
	PreStateTypeAt(p cfg.Point, path constraint.Path) typ.Type
	LengthBoundsAt(p cfg.Point, path constraint.Path) (lower, upper int64, ok bool)
	HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool
}

// Query describes one assignment-source evaluation.
type Query struct {
	Point  cfg.Point
	Target constraint.Path
	Static typ.Type
	Source flow.AssignmentSource
	Flow   Flow
	Ctx    *db.QueryContext
	Types  querycore.TypeOps
}

// Value evaluates q.Source against q.Flow and returns the source-owned value
// type, before any target annotation reconciliation.
func Value(q Query) typ.Type {
	if q.Flow == nil {
		return nil
	}
	var sourceType typ.Type
	switch q.Source.Kind {
	case flow.AssignmentSourcePath:
		sourceType = pathType(q)
	case flow.AssignmentSourceIterator:
		sourceType = iteratorType(q)
	case flow.AssignmentSourceContainerElement:
		sourceType = containerElementType(q)
	case flow.AssignmentSourceMapElement:
		sourceType = mapElementType(q)
	case flow.AssignmentSourceLengthIndex:
		sourceType = lengthIndexType(q)
	case flow.AssignmentSourceCallReturn:
		sourceType = callReturnType(q)
	case flow.AssignmentSourceOperator:
		sourceType = operatorType(q)
	}
	return sourceProjection(sourceType, q.Source)
}

func sourceProjection(sourceType typ.Type, source flow.AssignmentSource) typ.Type {
	if source.ProjectionKind == flow.AssignmentSourceProjectionNone {
		return sourceType
	}
	return typevalue.SelectSourceProjection(sourceType, source.ProjectedType)
}

func pathType(q Query) typ.Type {
	if !q.Source.Path.HasSymbol() {
		return nil
	}
	if q.Target.Symbol != 0 && pathkey.PathRelated(q.Target, q.Source.Path) {
		if pre := q.Flow.PreStateTypeAt(q.Point, q.Source.Path); !typ.IsAbsentOrUnknown(pre) {
			return pre
		}
	}
	return q.Flow.NarrowedTypeAt(q.Point, q.Source.Path)
}

func iteratorType(q Query) typ.Type {
	if q.Source.Path.IsEmpty() {
		return nil
	}
	if q.Source.IteratorKind == flow.IterateIndexed && q.Source.VarIndex == 0 {
		return typ.Integer
	}
	source := q.Flow.NarrowedTypeAt(q.Point, q.Source.Path)
	if typ.IsAbsentOrUnknown(source) {
		source = q.Flow.PreStateTypeAt(q.Point, q.Source.Path)
	}
	if typ.IsAbsentOrUnknown(source) {
		return nil
	}
	projection, ok := iteration.ProjectVarTypes(iteratorKind(q.Source.IteratorKind), q.Source.VarIndex+1, source)
	if !ok || projection.Empty || q.Source.VarIndex < 0 || q.Source.VarIndex >= len(projection.Types) {
		return nil
	}
	return projection.Types[q.Source.VarIndex]
}

func iteratorKind(kind flow.IteratorKind) effect.IteratorKind {
	switch kind {
	case flow.IterateIndexed:
		return effect.IterateIndexed
	case flow.IterateKeyed:
		return effect.IterateKeyed
	default:
		return effect.IterateIndexed
	}
}

func containerElementType(q Query) typ.Type {
	if q.Source.ContainerPath.IsEmpty() {
		return nil
	}
	container := q.Flow.NarrowedTypeAt(q.Point, q.Source.ContainerPath)
	if typ.IsAbsentOrUnknown(container) {
		container = q.Flow.PreStateTypeAt(q.Point, q.Source.ContainerPath)
	}
	if typ.IsAbsentOrUnknown(container) {
		return nil
	}
	if inst, ok := container.(*typ.Instantiated); ok && len(inst.TypeArgs) > 0 {
		return inst.TypeArgs[0]
	}
	return querycore.ElementType(container)
}

func mapElementType(q Query) typ.Type {
	if q.Source.MapPath.IsEmpty() {
		return nil
	}
	mapType := q.Flow.NarrowedTypeAt(q.Point, q.Source.MapPath)
	if typ.IsAbsentOrUnknown(mapType) || mapType.Kind().IsPlaceholder() {
		if pre := q.Flow.PreStateTypeAt(q.Point, q.Source.MapPath); !typ.IsAbsentOrUnknown(pre) {
			mapType = pre
		}
	}
	if typ.IsAbsentOrUnknown(mapType) {
		return nil
	}
	keyType := dynamicKeyType(q)
	if valueType, ok := index(q, mapType, keyType); ok {
		return refineKeyPresence(q, valueType)
	}
	if valueType := querycore.ValueType(mapType); valueType != nil {
		return refineKeyPresence(q, valueType)
	}
	if mapType.Kind().IsPlaceholder() {
		return typ.Any
	}
	return typ.Nil
}

func dynamicKeyType(q Query) typ.Type {
	if q.Source.KeySymbol != 0 {
		keyPath := constraint.Path{Root: q.Source.KeyVar, Symbol: q.Source.KeySymbol}
		if keyType := q.Flow.NarrowedTypeAt(q.Point, keyPath); !typ.IsAbsentOrUnknown(keyType) {
			return normalizeDynamicKeyType(keyType)
		}
	}
	return typ.Any
}

func normalizeDynamicKeyType(keyType typ.Type) typ.Type {
	keyType = narrow.ToTruthy(keyType)
	if typ.IsAbsentOrUnknown(keyType) {
		return typ.Unknown
	}
	if typ.UnwrapAnnotated(keyType).Kind() == kind.Literal {
		return keyType
	}
	return subtype.Widen(keyType)
}

func refineKeyPresence(q Query, valueType typ.Type) typ.Type {
	if valueType == nil || q.Source.MapPath.IsEmpty() || q.Source.KeySymbol == 0 {
		return valueType
	}
	keyPath := constraint.Path{Root: q.Source.KeyVar, Symbol: q.Source.KeySymbol}
	if keyPath.IsEmpty() || !q.Flow.HasKeyOf(q.Point, q.Source.MapPath, keyPath) {
		return valueType
	}
	if !narrow.NilPresenceIsOnlyFlowUncertainty(valueType) {
		return valueType
	}
	refined := narrow.RemoveNil(valueType)
	if refined == nil || typ.IsNever(refined) {
		return valueType
	}
	return refined
}

func lengthIndexType(q Query) typ.Type {
	if q.Source.ContainerPath.IsEmpty() {
		return nil
	}
	container := q.Flow.NarrowedTypeAt(q.Point, q.Source.ContainerPath)
	if typ.IsAbsentOrUnknown(container) || container.Kind().IsPlaceholder() {
		if pre := q.Flow.PreStateTypeAt(q.Point, q.Source.ContainerPath); !typ.IsAbsentOrUnknown(pre) {
			container = pre
		}
	}
	if typ.IsAbsentOrUnknown(container) {
		return nil
	}
	read := q.Static
	if typ.IsAbsentOrUnknown(read) || unwrap.IsNilType(read) {
		if resolved, ok := index(q, container, typ.Integer); ok {
			read = resolved
		} else {
			read = querycore.ValueType(container)
		}
	}
	if typ.IsAbsentOrUnknown(read) {
		return nil
	}
	lower, _, ok := q.Flow.LengthBoundsAt(q.Point, q.Source.ContainerPath)
	if !ok {
		return nil
	}
	refined := narrow.RefineLengthIndex(container, read, lower, q.Source.Offset)
	if refined != nil {
		return refined
	}
	index := lower + q.Source.Offset
	if index >= 1 && q.Source.Offset == 0 {
		return read
	}
	return nil
}

func callReturnType(q Query) typ.Type {
	if q.Source.ReturnIndex < 0 {
		return nil
	}
	var callee typ.Type
	var receiver typ.Type
	if !q.Source.ReceiverPath.IsEmpty() && q.Source.Method != "" {
		receiver = q.Flow.NarrowedTypeAt(q.Point, q.Source.ReceiverPath)
		if typ.IsAbsentOrUnknown(receiver) || unwrap.IsOptionalLike(receiver) {
			return nil
		}
		if method, ok := field(q, receiver, q.Source.Method); ok {
			callee = method
		}
	} else if !q.Source.CalleePath.IsEmpty() {
		callee = q.Flow.NarrowedTypeAt(q.Point, q.Source.CalleePath)
	}
	fn := unwrap.Function(callee)
	if fn == nil || q.Source.ReturnIndex >= len(fn.Returns) {
		return nil
	}
	return callReturnSlotType(fn, q.Source.ReturnIndex, receiver)
}

func callReturnSlotType(fn *typ.Function, returnIndex int, receiver typ.Type) typ.Type {
	if fn == nil || returnIndex < 0 || returnIndex >= len(fn.Returns) {
		return nil
	}
	base := fn.Returns[returnIndex]
	if receiver != nil {
		base = subst.SelfValue(base, receiver)
	}
	if er := contract.ErrorReturnForValue(fn, returnIndex); er != nil && !unwrap.IsOptionalLike(base) {
		return typ.NewOptional(base)
	}
	return base
}

func operatorType(q Query) typ.Type {
	if len(q.Source.Operands) == 0 {
		return nil
	}
	operands := make([]typ.Type, len(q.Source.Operands))
	for i, operand := range q.Source.Operands {
		operands[i] = operand.Static
		if operand.Path.HasSymbol() {
			if t := q.Flow.NarrowedTypeAt(q.Point, operand.Path); !typ.IsAbsentOrUnknown(t) {
				operands[i] = t
			}
		}
		if typ.IsAbsentOrUnknown(operands[i]) {
			return nil
		}
	}
	switch len(operands) {
	case 1:
		return unaryOp(q, q.Source.Operator, operands[0])
	case 2:
		return binaryOp(q, operands[0], q.Source.Operator, operands[1])
	default:
		return nil
	}
}

func field(q Query, t typ.Type, name string) (typ.Type, bool) {
	if q.Types != nil {
		return q.Types.Field(q.Ctx, t, name)
	}
	return querycore.Field(t, name)
}

func index(q Query, t typ.Type, key typ.Type) (typ.Type, bool) {
	if q.Types != nil {
		return q.Types.Index(q.Ctx, t, key)
	}
	return querycore.Index(t, key)
}

func binaryOp(q Query, left typ.Type, op string, right typ.Type) typ.Type {
	if q.Types != nil {
		return q.Types.BinaryOp(q.Ctx, left, op, right)
	}
	return querycore.BinaryOp(left, op, right)
}

func unaryOp(q Query, op string, operand typ.Type) typ.Type {
	if q.Types != nil {
		return q.Types.UnaryOp(q.Ctx, op, operand)
	}
	return querycore.UnaryOp(op, operand)
}
