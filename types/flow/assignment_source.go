package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	typevalue "github.com/wippyai/go-lua/types/domain/value"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// AssignmentSourceReadFacts is the solved-flow surface needed by the
// assignment-source algebra. Indexed reads consume the same proof surface as
// path observations, so key-presence and length facts have one reducer.
type AssignmentSourceReadFacts interface {
	NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type
	PreStateTypeAt(p cfg.Point, path constraint.Path) typ.Type
	IndexReadObservationProofs
}

// AssignmentSourceQuery describes one source-owned RHS evaluation.
type AssignmentSourceQuery struct {
	Point  cfg.Point
	Target constraint.Path
	Static typ.Type
	Source AssignmentSource
	Flow   AssignmentSourceReadFacts
	Ctx    *db.QueryContext
	Types  querycore.TypeOps
}

// AssignmentSourceValue evaluates q.Source against q.Flow and returns the
// source-owned value type before target annotation reconciliation.
func AssignmentSourceValue(q AssignmentSourceQuery) typ.Type {
	if q.Flow == nil {
		return nil
	}
	var sourceType typ.Type
	switch q.Source.Kind {
	case AssignmentSourcePath:
		sourceType = assignmentPathType(q)
	case AssignmentSourceIterator:
		sourceType = assignmentIteratorType(q)
	case AssignmentSourceContainerElement:
		sourceType = assignmentContainerElementType(q)
	case AssignmentSourceMapElement:
		sourceType = assignmentMapElementType(q)
	case AssignmentSourceLengthIndex:
		sourceType = assignmentLengthIndexType(q)
	case AssignmentSourceCallReturn:
		sourceType = assignmentCallReturnType(q)
	case AssignmentSourceOperator:
		sourceType = assignmentOperatorType(q)
	}
	return assignmentSourceProjection(sourceType, q.Source)
}

func assignmentSourceProjection(sourceType typ.Type, source AssignmentSource) typ.Type {
	if source.ProjectionKind == AssignmentSourceProjectionNone {
		return sourceType
	}
	return typevalue.SelectSourceProjection(sourceType, source.ProjectedType)
}

func assignmentPathType(q AssignmentSourceQuery) typ.Type {
	if !q.Source.Path.HasSymbol() {
		return nil
	}
	if q.Target.Symbol != 0 && pathRelated(q.Target, q.Source.Path) {
		if pre := q.Flow.PreStateTypeAt(q.Point, q.Source.Path); !typ.IsAbsentOrUnknown(pre) {
			return pre
		}
	}
	return q.Flow.NarrowedTypeAt(q.Point, q.Source.Path)
}

func assignmentIteratorType(q AssignmentSourceQuery) typ.Type {
	if q.Source.Path.IsEmpty() {
		return nil
	}
	if q.Source.IteratorKind == IterateIndexed && q.Source.VarIndex == 0 {
		return typ.Integer
	}
	source := q.Flow.NarrowedTypeAt(q.Point, q.Source.Path)
	if typ.IsAbsentOrUnknown(source) {
		source = q.Flow.PreStateTypeAt(q.Point, q.Source.Path)
	}
	if typ.IsAbsentOrUnknown(source) {
		return nil
	}
	projection, ok := ProjectIteratorVarTypes(q.Source.IteratorKind, q.Source.VarIndex+1, source)
	if !ok || projection.Empty || q.Source.VarIndex < 0 || q.Source.VarIndex >= len(projection.Types) {
		return nil
	}
	return projection.Types[q.Source.VarIndex]
}

func assignmentContainerElementType(q AssignmentSourceQuery) typ.Type {
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

func assignmentMapElementType(q AssignmentSourceQuery) typ.Type {
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
	keyType := assignmentDynamicKeyType(q)
	if valueType, ok := assignmentIndex(q, mapType, keyType); ok {
		return assignmentRefineIndexRead(q, mapType, valueType, PathObservationIndexRead{
			Container: mapType,
			TablePath: q.Source.MapPath,
			KeyPath:   assignmentSourceKeyPath(q.Source),
			KeyType:   keyType,
		})
	}
	if valueType := querycore.ValueType(mapType); valueType != nil {
		return assignmentRefineIndexRead(q, mapType, valueType, PathObservationIndexRead{
			Container: mapType,
			TablePath: q.Source.MapPath,
			KeyPath:   assignmentSourceKeyPath(q.Source),
			KeyType:   keyType,
		})
	}
	if mapType.Kind().IsPlaceholder() {
		return typ.Any
	}
	return typ.Nil
}

func assignmentSourceKeyPath(source AssignmentSource) constraint.Path {
	if source.KeySymbol == 0 {
		return constraint.Path{}
	}
	return constraint.Path{Root: source.KeyVar, Symbol: source.KeySymbol}
}

func assignmentDynamicKeyType(q AssignmentSourceQuery) typ.Type {
	if q.Source.KeySymbol != 0 {
		if keyType := q.Flow.NarrowedTypeAt(q.Point, assignmentSourceKeyPath(q.Source)); !typ.IsAbsentOrUnknown(keyType) {
			return NormalizeDynamicKeyType(keyType)
		}
	}
	return typ.Any
}

func assignmentLengthIndexType(q AssignmentSourceQuery) typ.Type {
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
		if resolved, ok := assignmentIndex(q, container, typ.Integer); ok {
			read = resolved
		} else {
			read = querycore.ValueType(container)
		}
	}
	if typ.IsAbsentOrUnknown(read) {
		return nil
	}
	refined := assignmentRefineIndexRead(q, container, read, PathObservationIndexRead{
		Container:       container,
		TablePath:       q.Source.ContainerPath,
		LengthPath:      q.Source.ContainerPath,
		LengthOffset:    q.Source.Offset,
		HasLength:       true,
		LiteralIndex:    q.Source.Offset,
		HasLiteralIndex: q.Source.Offset >= 1,
	})
	if !typ.IsAbsentOrUnknown(refined) {
		return refined
	}
	return nil
}

func assignmentRefineIndexRead(
	q AssignmentSourceQuery,
	container typ.Type,
	result typ.Type,
	index PathObservationIndexRead,
) typ.Type {
	if result == nil {
		return nil
	}
	index.Container = container
	if refined, ok := RefineIndexReadObservation(IndexReadObservationQuery{
		Point:  q.Point,
		View:   PathReadCurrent,
		Result: result,
		Index:  index,
		Proofs: q.Flow,
	}); ok {
		return refined
	}
	return result
}

func assignmentCallReturnType(q AssignmentSourceQuery) typ.Type {
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
		if method, ok := assignmentField(q, receiver, q.Source.Method); ok {
			callee = method
		}
	} else if !q.Source.CalleePath.IsEmpty() {
		callee = q.Flow.NarrowedTypeAt(q.Point, q.Source.CalleePath)
	}
	fn := unwrap.Function(callee)
	if fn == nil || q.Source.ReturnIndex >= len(fn.Returns) {
		return nil
	}
	return assignmentCallReturnSlotType(fn, q.Source.ReturnIndex, receiver)
}

func assignmentCallReturnSlotType(fn *typ.Function, returnIndex int, receiver typ.Type) typ.Type {
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

func assignmentOperatorType(q AssignmentSourceQuery) typ.Type {
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
		return assignmentUnaryOp(q, q.Source.Operator, operands[0])
	case 2:
		return assignmentBinaryOp(q, operands[0], q.Source.Operator, operands[1])
	default:
		return nil
	}
}

func assignmentField(q AssignmentSourceQuery, t typ.Type, name string) (typ.Type, bool) {
	if q.Types != nil {
		return q.Types.Field(q.Ctx, t, name)
	}
	return querycore.Field(t, name)
}

func assignmentIndex(q AssignmentSourceQuery, t typ.Type, key typ.Type) (typ.Type, bool) {
	if q.Types != nil {
		return q.Types.Index(q.Ctx, t, key)
	}
	return querycore.Index(t, key)
}

func assignmentBinaryOp(q AssignmentSourceQuery, left typ.Type, op string, right typ.Type) typ.Type {
	if q.Types != nil {
		return q.Types.BinaryOp(q.Ctx, left, op, right)
	}
	return querycore.BinaryOp(left, op, right)
}

func assignmentUnaryOp(q AssignmentSourceQuery, op string, operand typ.Type) typ.Type {
	if q.Types != nil {
		return q.Types.UnaryOp(q.Ctx, op, operand)
	}
	return querycore.UnaryOp(op, operand)
}

func pathRelated(a, b constraint.Path) bool {
	aAddr, aOK := StableAddressOfPath(a)
	bAddr, bOK := StableAddressOfPath(b)
	return aOK && bOK && aAddr.Overlaps(bAddr)
}
