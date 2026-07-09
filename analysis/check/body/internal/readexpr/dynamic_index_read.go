package readexpr

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func dynamicIndexExpressionPath(config Config, point cfg.Point, dyn factflow.DynamicIndexExpression, in state.State) (pathdom.Path, bool) {
	keyValue, ok := dynamicIndexExpressionKeyValue(config, point, dyn.KeySource(), in)
	if !ok {
		return pathdom.Path{}, false
	}
	seg, ok := staticScalarKeySegment(config.Registry, config.TypeValues, keyValue)
	if !ok {
		return pathdom.Path{}, false
	}
	return dyn.TablePathRef().Append(seg), true
}

func staticScalarKeySegment(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (segment.Segment, bool) {
	t, ok := typeValues.TypeOf(reg, value)
	if !ok {
		return segment.Segment{}, false
	}
	lit, ok := unwrap.Alias(t).(*typ.Literal)
	if !ok {
		return segment.Segment{}, false
	}
	switch lit.Base {
	case kind.String:
		name, ok := lit.Value.(string)
		if !ok {
			return segment.Segment{}, false
		}
		return segment.Segment{Kind: segment.SegmentIndexString, Name: name}, true
	case kind.Integer:
		index, ok := lit.Value.(int64)
		if !ok || int64(int(index)) != index {
			return segment.Segment{}, false
		}
		return segment.Segment{Kind: segment.SegmentIndexInt, Index: int(index)}, true
	default:
		return segment.Segment{}, false
	}
}

func dynamicIndexExpressionKeyValue(config Config, point cfg.Point, source factflow.ValueSource, in state.State) (product.Value, bool) {
	return dynamicIndexExpressionKeyValueActive(config, point, source, in, nil)
}

func dynamicIndexExpressionKeyValueActive(
	config Config,
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return product.Value{}, false
		}
		if p, ok := config.Facts.ExpressionPathRef(source.ExprRef); ok {
			if value, ok := Project(config, point, p, in); ok {
				return value, true
			}
			value, ok := config.Facts.ExpressionValue(source.ExprRef)
			return value, ok
		}
		if value, ok := dynamicIndexExpressionOperationValue(config, point, source.ExprRef, in, active); ok {
			return value, true
		}
		if dyn, ok := config.Facts.DynamicIndexExpression(source.ExprRef); ok {
			return dynamicIndexExpressionValueActive(config, point, source.ExprRef, dyn, in, active)
		}
		value, ok := config.Facts.ExpressionValue(source.ExprRef)
		return value, ok
	case factflow.ValueSourcePath:
		p, ok := dynamicIndexSourcePath(config, source)
		if !ok {
			return product.Value{}, false
		}
		return Project(config, point, p, in)
	case factflow.ValueSourceNil:
		return typevalue.Nil(config.Registry), true
	case factflow.ValueSourceLiteral:
		switch source.LiteralKind {
		case factflow.ValueSourceLiteralBool:
			return typevalue.LiteralBool(config.Registry, source.Bool), true
		case factflow.ValueSourceLiteralInteger:
			return typevalue.LiteralInt(config.Registry, source.Int), true
		case factflow.ValueSourceLiteralNumber:
			return typevalue.LiteralNumber(config.Registry, source.Float), true
		case factflow.ValueSourceLiteralString:
			return typevalue.LiteralString(config.Registry, source.String), true
		default:
			return product.Value{}, false
		}
	default:
		return product.Value{}, false
	}
}

func dynamicIndexExpressionOperationValue(
	config Config,
	point cfg.Point,
	expr factflow.ExprRef,
	in state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if expr == 0 || config.TypeValues == nil {
		return product.Value{}, false
	}
	op, ok := config.Facts.ExpressionOperation(expr)
	if !ok {
		return product.Value{}, false
	}
	if active[expr] {
		return product.Value{}, false
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[expr] = true
	left, ok := dynamicIndexExpressionKeyValueActive(config, point, op.Left(), in, active)
	if !ok {
		delete(active, expr)
		return product.Value{}, false
	}
	var right product.Value
	if op.Kind() == factflow.ExpressionOperationBinary {
		right, ok = dynamicIndexExpressionKeyValueActive(config, point, op.Right(), in, active)
		if !ok {
			delete(active, expr)
			return product.Value{}, false
		}
	}
	delete(active, expr)
	return luasourcevalue.ExpressionOperationValue(config.Registry, config.TypeValues, op, left, right)
}

func dynamicIndexExpressionValue(config Config, point cfg.Point, dyn factflow.DynamicIndexExpression, in state.State) (product.Value, bool) {
	return dynamicIndexExpressionValueActive(config, point, 0, dyn, in, nil)
}

func dynamicIndexExpressionValueActive(
	config Config,
	point cfg.Point,
	expr factflow.ExprRef,
	dyn factflow.DynamicIndexExpression,
	in state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || config.TypeValues == nil {
		return product.Value{}, false
	}
	if expr != 0 {
		if active[expr] {
			return product.Value{}, false
		}
		if active == nil {
			active = make(map[factflow.ExprRef]bool, 1)
		}
		active[expr] = true
		defer delete(active, expr)
	}
	if value, ok := dynamicIndexExpressionProvenMemberValue(config, point, dyn, in); ok {
		return value, true
	}
	tableValue, tableValueOK := Project(config, point, dyn.TablePathRef(), in)
	if !tableValueOK {
		if tableSource, ok := dyn.TableSource(); ok {
			tableValue, tableValueOK = dynamicIndexExpressionKeyValueActive(config, point, tableSource, in, active)
		}
	}
	if tableValueOK {
		keyValue, keyValueOK := dynamicIndexExpressionKeyValueActive(config, point, dyn.KeySource(), in, active)
		if keyValueOK {
			if config.Visibility != nil {
				if seg, ok := staticScalarKeySegment(reg, config.TypeValues, keyValue); ok {
					if value, ok := sourcevalue.HeapMemberFromValue(reg, config.Visibility.KeySpace(), in, tableValue, []segment.Segment{seg}); ok {
						return value, true
					}
				}
			}
			if value, ok := config.TypeValues.RuntimeIndex(reg, tableValue, keyValue); ok {
				value = sourcevalue.InheritTopOriginEvidence(reg, value, tableValue)
				if dynamicIndexKeyMembershipProvesRead(config, point, dyn, in) ||
					dynamicIndexInBoundsProvesRead(config, point, dyn, in) {
					value = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
				}
				return value, true
			}
			if typevalue.HasIntegerType(reg, keyValue) {
				keyValue := config.TypeValues.FromTypeWithWitness(reg, typ.Integer)
				if value, ok := config.TypeValues.RuntimeIndex(reg, tableValue, keyValue); ok {
					value = sourcevalue.InheritTopOriginEvidence(reg, value, tableValue)
					if dynamicIndexKeyMembershipProvesRead(config, point, dyn, in) ||
						dynamicIndexInBoundsProvesRead(config, point, dyn, in) {
						value = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
					}
					return value, true
				}
			}
		}
		if dynamicIndexInBoundsProvesRead(config, point, dyn, in) {
			keyValue := config.TypeValues.FromTypeWithWitness(reg, typ.Integer)
			if value, ok := config.TypeValues.RuntimeIndex(reg, tableValue, keyValue); ok {
				value = sourcevalue.InheritTopOriginEvidence(reg, value, tableValue)
				value = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
				return value, true
			}
		}
	}
	return product.Value{}, false
}

func dynamicIndexExpressionProvenMemberValue(config Config, point cfg.Point, dyn factflow.DynamicIndexExpression, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || config.Visibility == nil {
		return product.Value{}, false
	}
	pathMembershipProof := dynamicIndexKeyMembershipProvesRead(config, point, dyn, in)
	readKeyValue, hasReadKeyValue := dynamicIndexExpressionKeyValue(config, point, dyn.KeySource(), in)
	domain := product.Domain(reg)
	joined := domain.Bottom()
	found := false
	aborted := false
	forEachDynamicIndexPathStateKey(config, point, dyn.TablePathRef(), func(tableStateKey pathaddr.StateKey) bool {
		tableKey, ok := config.Visibility.KeySpace().InternStateKey(tableStateKey)
		if !ok {
			return true
		}
		if in.ForEachDynamicIndexFact(func(key dynamicindex.Key, fact dynamicindex.Fact) bool {
			if key.Table != tableKey || fact.Admission == dynamicindex.AdmissionRejected {
				return true
			}
			if domain.Equal(fact.Value, domain.Bottom()) {
				return true
			}
			if !pathMembershipProof {
				if !hasReadKeyValue || !dynamicIndexFactHasExactReadKey(config, fact, readKeyValue) || !dynamicIndexFactDefinitelyPresent(reg, fact) {
					return true
				}
			}
			if !found {
				joined = fact.Value
				found = true
				return true
			}
			joined = domain.Join(joined, fact.Value)
			return true
		}) {
			aborted = true
			return false
		}
		return true
	})
	if aborted {
		return product.Value{}, false
	}
	if !found {
		return product.Value{}, false
	}
	return sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, joined, presence.Present())), true
}

func dynamicIndexFactDefinitelyPresent(reg *axis.Registry, fact dynamicindex.Fact) bool {
	if reg == nil || product.Equal(reg, fact.Value, product.Bottom(reg)) {
		return false
	}
	if typevalue.HasOnlyNilType(reg, fact.Value) {
		return false
	}
	return presence.Equal(product.PresenceOf(fact.Value), presence.Present())
}

func dynamicIndexFactHasExactReadKey(config Config, fact dynamicindex.Fact, readKeyValue product.Value) bool {
	readSeg, ok := staticScalarKeySegment(config.Registry, config.TypeValues, readKeyValue)
	if !ok {
		return false
	}
	factSeg, ok := staticScalarKeySegment(config.Registry, config.TypeValues, fact.KeyValue)
	return ok && factSeg == readSeg
}

func forEachDynamicIndexPathStateKey(config Config, point cfg.Point, p pathdom.Path, fn func(pathaddr.StateKey) bool) bool {
	if config.Visibility == nil || p.IsEmpty() || p.Symbol == 0 {
		return true
	}
	return visibility.AddressAt(config.Visibility, point, p).ForEachStateKey(fn,
		visibility.StateKeyVisible,
		visibility.StateKeyRootOrVisible,
	)
}

func projectFromDynamicIndexFacts(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || config.Visibility == nil || len(p.Segments) == 0 {
		return product.Value{}, false
	}
	parent := p.ParentView()
	last := p.Segments[len(p.Segments)-1]
	domain := product.Domain(reg)
	joined := product.Bottom(reg)
	found := false
	aborted := false
	forEachDynamicIndexPathStateKey(config, point, parent, func(tableStateKey pathaddr.StateKey) bool {
		tableKey, ok := config.Visibility.KeySpace().InternStateKey(tableStateKey)
		if !ok {
			return true
		}
		if in.ForEachDynamicIndexFact(func(key dynamicindex.Key, fact dynamicindex.Fact) bool {
			if key.Table != tableKey || fact.Admission == dynamicindex.AdmissionRejected {
				return true
			}
			if !dynamicIndexFactDefinitelyMatchesSegment(reg, config.TypeValues, fact, last) {
				return true
			}
			if domain.Equal(fact.Value, domain.Bottom()) {
				return true
			}
			if !found {
				joined = fact.Value
				found = true
				return true
			}
			joined = domain.Join(joined, fact.Value)
			return true
		}) {
			aborted = true
			return false
		}
		return true
	})
	if aborted {
		return product.Value{}, false
	}
	if !found {
		return product.Value{}, false
	}
	return joined, true
}

func dynamicIndexFactDefinitelyMatchesSegment(reg *axis.Registry, typeValues *typevalue.Cache, fact dynamicindex.Fact, seg segment.Segment) bool {
	keyType, ok := typeValues.TypeOf(reg, fact.KeyValue)
	if !ok {
		return false
	}
	return dynamicIndexKeyDefinitelyMatchesSegment(keyType, seg, 0)
}

func dynamicIndexFactMayMatchSegment(reg *axis.Registry, typeValues *typevalue.Cache, fact dynamicindex.Fact, seg segment.Segment) bool {
	keyType, ok := typeValues.TypeOf(reg, fact.KeyValue)
	if !ok {
		return true
	}
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return typetable.MapComponentKeyMayContainString(keyType, seg.Name)
	case segment.SegmentIndexInt:
		return typetable.MapComponentKeyMayContainInt(keyType, int64(seg.Index))
	default:
		return true
	}
}

func dynamicIndexKeyDefinitelyMatchesSegment(t typ.Type, seg segment.Segment, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case nil:
		return false
	case *typ.Literal:
		return literalDynamicIndexKeyMatchesSegment(tt, seg)
	case *typ.Optional:
		return false
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !dynamicIndexKeyDefinitelyMatchesSegment(member, seg, depth+1) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		for _, member := range tt.Members {
			if dynamicIndexKeyDefinitelyMatchesSegment(member, seg, depth+1) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func literalDynamicIndexKeyMatchesSegment(lit *typ.Literal, seg segment.Segment) bool {
	if lit == nil {
		return false
	}
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if lit.Base != kind.String {
			return false
		}
		name, ok := lit.Value.(string)
		return ok && name == seg.Name
	case segment.SegmentIndexInt:
		switch lit.Base {
		case kind.Integer:
			index, ok := lit.Value.(int64)
			return ok && index == int64(seg.Index)
		case kind.Number:
			number, ok := lit.Value.(float64)
			return ok && number == float64(seg.Index)
		default:
			return false
		}
	default:
		return false
	}
}
