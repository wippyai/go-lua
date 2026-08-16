package readexpr

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/domain/type/inspect"
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func dynamicIndexExpressionKeyValue(config Config, point cfg.Point, source factflow.ValueSource, in state.State) (product.Value, bool) {
	return dynamicIndexExpressionKeyValueActive(config, point, source, in, nil)
}

func dynamicIndexProofVisibility(config Config) *visibility.Resolver {
	if config.ProofVisibility != nil {
		return config.ProofVisibility
	}
	return config.Visibility
}

func dynamicIndexModuloDividendHasIntegerSource(config Config, point cfg.Point, source factflow.ValueSource, in state.State) bool {
	value, ok := dynamicIndexExpressionKeyValue(config, point, source, in)
	return ok && typevalue.HasIntegerType(config.Registry, value)
}

func dynamicIndexSourcePath(config Config, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" {
		return pathdom.Path{}, false
	}
	if p, ok := pathaddr.LocalPathFromKey(source.PathKey); ok {
		return p, true
	}
	if sym, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey); ok {
		segments, segmentsOK := segment.ParseFormattedSegments(suffix)
		if !segmentsOK {
			return pathdom.Path{}, false
		}
		return pathdom.Path{Symbol: sym, Version: version, Segments: segments}, true
	}
	if stable, ok := pathaddr.StableFromKey(source.PathKey); ok {
		return stable.Path()
	}
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey); ok {
		return pathdom.Path{Symbol: sym, Segments: segments}, true
	}
	if config.Visibility != nil && config.Visibility.KeySpace() != nil {
		key, ok := config.Visibility.KeySpace().FromStateKey(source.PathKey)
		if ok && key.Sym != 0 {
			return pathdom.Path{Symbol: key.Sym, Segments: config.Visibility.KeySpace().Segments(key)}, true
		}
	}
	return pathdom.Path{}, false
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
		if refinement, ok := config.Facts.ExpressionRefinement(source.ExprRef); ok {
			if active[source.ExprRef] {
				return product.Value{}, false
			}
			if active == nil {
				active = make(map[factflow.ExprRef]bool, 1)
			}
			active[source.ExprRef] = true
			value, valueOK := dynamicIndexExpressionKeyValueActive(config, point, refinement.Source(), in, active)
			delete(active, source.ExprRef)
			if !valueOK {
				if refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation {
					return product.Value{}, false
				}
				value = product.Bottom(config.Registry)
			}
			return sourcevalue.ApplyExpressionRefinement(config.Registry, value, refinement), true
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

// staticIntegerIndexValue lowers a statically spelled integer path segment to
// the same normalized dynamic-read request used by computed indices. Static
// and dynamic Lua indexing therefore share path, heap, type and range laws;
// no post-projection nil refiner is needed.
func staticIntegerIndexValue(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	if config.Registry == nil || config.Visibility == nil || len(p.Segments) == 0 {
		return product.Value{}, false
	}
	last := p.Segments[len(p.Segments)-1]
	if last.Kind != segment.SegmentIndexInt {
		return product.Value{}, false
	}
	parent := p.ParentView()
	tableValue, ok := project(config, point, parent, in, false)
	if !ok {
		return product.Value{}, false
	}
	proofInput := in
	hasProofInput := false
	if config.ProofState != nil {
		if proof, proofOK := config.ProofState(point); proofOK {
			proofInput, hasProofInput = proof, true
		}
	}
	return sourcevalue.ReadBoundDynamicValue(sourcevalue.BoundDynamicRead{
		Registry: config.Registry, TypeValues: config.TypeValues, KeySpace: config.Visibility.KeySpace(),
		Visibility: config.Visibility, ProofVisibility: dynamicIndexProofVisibility(config), Point: point,
		TablePath: parent, TableValue: tableValue, KeyValue: typevalue.LiteralInt(config.Registry, int64(last.Index)),
		ValueInput: in, ProofInput: proofInput, HasProofInput: hasProofInput,
		IndexForm: indexform.NewConstantIndex(int64(last.Index)),
	})
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
	tableValue, tableValueOK := Project(config, point, dyn.TablePathRef(), in)
	if !tableValueOK {
		if tableSource, ok := dyn.TableSource(); ok {
			tableValue, tableValueOK = dynamicIndexExpressionKeyValueActive(config, point, tableSource, in, active)
		}
	}
	if !tableValueOK {
		if dyn.TablePathRef().IsEmpty() {
			return product.Value{}, false
		}
		// A structurally addressed operand may execute without contributing a
		// scalar-axis payload. Preserve that distinction for the canonical
		// sparse binder; its path/heap factors remain the semantic authority.
		tableValue, tableValueOK = product.Bottom(reg), true
	}
	var keyPath pathdom.Path
	keySource := dyn.KeySource()
	if keySource.Kind == factflow.ValueSourceExpression && keySource.HasExpr {
		keyPath, _ = config.Facts.ExpressionPathRef(keySource.ExprRef)
	}
	keyValue, keyValueOK := dynamicIndexExpressionKeyValueActive(config, point, dyn.KeySource(), in, active)
	if !keyValueOK {
		if keyPath.IsEmpty() {
			return product.Value{}, false
		}
		keyValue = product.Bottom(reg)
	}
	keys := keyspace.New()
	var resolver *visibility.Resolver
	if config.Visibility != nil {
		keys, resolver = config.Visibility.KeySpace(), config.Visibility
	}
	normalized, normalizedOK := config.Facts.NormalizeDynamicReadIndexForm(dyn, func(source factflow.ValueSource) (int64, bool) {
		if source.Kind == factflow.ValueSourceLiteral && source.LiteralKind == factflow.ValueSourceLiteralInteger {
			return source.Int, true
		}
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return 0, false
		}
		value, ok := config.Facts.ExpressionValue(source.ExprRef)
		if !ok {
			return 0, false
		}
		return typevalue.IntegerLiteralValue(config.Registry, value)
	})
	form := indexform.IndexForm{}
	moduloInteger := false
	proofInput := in
	hasProofInput := false
	if config.ProofState != nil {
		if proof, proofOK := config.ProofState(point); proofOK {
			proofInput, hasProofInput = proof, true
		}
	}
	if normalizedOK {
		form, normalizedOK = normalized.Form()
		if normalizedOK && form.Kind() == indexform.IndexFormModuloLength {
			if dividend, dividendOK := normalized.IntegerCertificateSource(); dividendOK {
				moduloInteger = dynamicIndexModuloDividendHasIntegerSource(config, point, dividend, proofInput)
			}
		}
	}
	value, ok := sourcevalue.ReadBoundDynamicValue(sourcevalue.BoundDynamicRead{
		Registry: reg, TypeValues: config.TypeValues, KeySpace: keys,
		Visibility: resolver, ProofVisibility: dynamicIndexProofVisibility(config), Point: point,
		TablePath: dyn.TablePathRef(), KeyPath: keyPath, TableValue: tableValue, KeyValue: keyValue,
		ValueInput: in, ProofInput: proofInput, HasProofInput: hasProofInput,
		IndexForm: form, ModuloInteger: moduloInteger,
	})
	if !ok {
		return product.Value{}, false
	}
	return value, true
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
	poll := cancellation.NewPoller(config.Cancel, cancellation.EveryExpensive)
	forEachDynamicIndexPathStateKey(config, point, parent, func(tableStateKey pathaddr.StateKey) bool {
		if poll.Poll() {
			aborted = true
			return false
		}
		tableKey, ok := config.Visibility.KeySpace().InternStateKey(tableStateKey)
		if !ok {
			return true
		}
		if in.ForEachDynamicIndexFact(func(key dynamicindex.Key, fact dynamicindex.Fact) bool {
			if poll.Poll() {
				aborted = true
				return false
			}
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
	return dynamicIndexKeyDefinitelyMatchesSegment(keyType, seg)
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

func dynamicIndexKeyDefinitelyMatchesSegment(t typ.Type, seg segment.Segment) bool {
	if !inspect.LeastBoolFixedPoint(t, dynamicIndexKeyProductivityEquation) {
		return false
	}
	return inspect.GreatestBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch tt := unwrap.Alias(current).(type) {
		case *typ.Literal:
			return inspect.Constant(literalDynamicIndexKeyMatchesSegment(tt, seg))
		case *typ.Optional:
			return inspect.Constant(false)
		case *typ.Union:
			if len(tt.Members) == 0 {
				return inspect.Constant(false)
			}
			return inspect.All(tt.Members...)
		case *typ.Intersection:
			if len(tt.Members) == 0 {
				return inspect.Constant(false)
			}
			return inspect.Any(tt.Members...)
		case *typ.Recursive:
			return inspect.All(tt.Body)
		default:
			return inspect.Constant(false)
		}
	})
}

func dynamicIndexKeyProductivityEquation(current typ.Type) inspect.BoolEquation {
	switch tt := unwrap.Alias(current).(type) {
	case nil:
		return inspect.Constant(false)
	case *typ.Union, *typ.Intersection:
		var members []typ.Type
		switch value := tt.(type) {
		case *typ.Union:
			members = value.Members
		case *typ.Intersection:
			members = value.Members
		}
		if len(members) == 0 {
			return inspect.Constant(false)
		}
		return inspect.Any(members...)
	case *typ.Recursive:
		return inspect.Any(tt.Body)
	default:
		return inspect.Constant(true)
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
