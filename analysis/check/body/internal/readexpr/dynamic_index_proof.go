package readexpr

import (
	"math"

	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func dynamicIndexKeyMembershipProvesRead(config Config, point cfg.Point, dyn factflow.DynamicIndexExpression, in state.State) bool {
	if config.Visibility == nil {
		return false
	}
	source := dyn.KeySource()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	keyPath, ok := config.Facts.ExpressionPathRef(source.ExprRef)
	if !ok || keyPath.IsEmpty() || keyPath.Symbol == 0 {
		return false
	}
	found := false
	forEachDynamicIndexPathStateKey(config, point, keyPath, func(keyStateKey pathaddr.StateKey) bool {
		forEachDynamicIndexPathStateKey(config, point, dyn.TablePathRef(), func(tableKey pathaddr.StateKey) bool {
			if in.HasPathKeyMembership(keyStateKey, tableKey) {
				found = true
				return false
			}
			return true
		})
		return !found
	})
	return found
}

func dynamicIndexInBoundsProvesRead(config Config, point cfg.Point, dyn factflow.DynamicIndexExpression, in state.State) bool {
	if config.Visibility == nil || !dynamicIndexContainerCanDropMissNil(config, point, dyn, in) {
		return false
	}
	proofState := dynamicIndexProofState(config, point, in)
	proofVisibility := dynamicIndexProofVisibility(config)
	if dynamicIndexKeyIsArrayLength(config, dyn.KeySource(), dyn.TablePathRef()) {
		arrayKey, ok := visibility.AddressAt(proofVisibility, point, dyn.TablePathRef()).VisibleStateKey()
		if !ok {
			return false
		}
		floor, ok := proofState.ReadLenFloor(proofVisibility.KeySpace(), arrayKey)
		return ok && floor >= 1
	}
	if dynamicIndexModuloLengthProvesRead(config, point, dyn.KeySource(), dyn.TablePathRef(), proofState, proofVisibility) {
		return true
	}
	if index, ok := dynamicIndexIntegerConstant(config, dyn.KeySource()); ok && index >= 1 {
		if dynamicIndexLengthFloorAtLeast(proofVisibility, point, proofState, dyn.TablePathRef(), index) {
			return true
		}
		if dynamicIndexStaticSequenceLengthAtLeast(config, point, dyn.TablePathRef(), in, index) {
			return true
		}
	}
	term, ok := dynamicIndexIntegerTerm(config, dyn.KeySource())
	if !ok || term.Coeff <= 0 || term.Path.IsEmpty() {
		return false
	}
	if floor, ok := dynamicIndexTermFloor(proofVisibility, point, proofState, term); !ok || term.Coeff*floor+term.Offset < 1 {
		return false
	}
	if dynamicIndexDiffProvesLELength(config, proofVisibility, point, proofState, term, dyn.TablePathRef()) {
		return true
	}
	if ceil, ok := dynamicIndexTermCeil(proofVisibility, point, proofState, term); ok {
		if upper, ok := checkedAffineInt64(term.Coeff, ceil, term.Offset); ok && upper >= 1 {
			if dynamicIndexLengthFloorAtLeast(proofVisibility, point, proofState, dyn.TablePathRef(), upper) {
				return true
			}
			if dynamicIndexStaticSequenceLengthAtLeast(config, point, dyn.TablePathRef(), in, upper) {
				return true
			}
		}
	}
	if term.Coeff != 1 || term.Offset != 0 {
		return false
	}
	indexKey, indexOK := visibility.AddressAt(proofVisibility, point, term.Path).VisibleStateKey()
	arrayKey, arrayOK := visibility.AddressAt(proofVisibility, point, dyn.TablePathRef()).VisibleStateKey()
	return indexOK && arrayOK && proofState.HasIndexInRangeProofForStateKeys(proofVisibility.KeySpace(), indexKey, arrayKey)
}

func dynamicIndexLengthFloorAtLeast(resolver *visibility.Resolver, point cfg.Point, in state.State, arrayPath pathdom.Path, floor int64) bool {
	if floor <= 0 {
		return true
	}
	if resolver == nil || arrayPath.IsEmpty() || arrayPath.Symbol == 0 {
		return false
	}
	arrayKey, ok := visibility.AddressAt(resolver, point, arrayPath).VisibleStateKey()
	if !ok {
		return false
	}
	got, ok := in.ReadLenFloor(resolver.KeySpace(), arrayKey)
	return ok && got >= floor
}

func dynamicIndexProofState(config Config, point cfg.Point, fallback state.State) state.State {
	if config.ProofState == nil {
		return fallback
	}
	proofState, ok := config.ProofState(point)
	if !ok {
		return fallback
	}
	return proofState
}

func dynamicIndexProofVisibility(config Config) *visibility.Resolver {
	if config.ProofVisibility != nil {
		return config.ProofVisibility
	}
	return config.Visibility
}

func dynamicIndexTermFloor(resolver *visibility.Resolver, point cfg.Point, in state.State, term dynamicIndexTerm) (int64, bool) {
	if resolver == nil || term.Path.IsEmpty() {
		return 0, false
	}
	stateKey, ok := visibility.AddressAt(resolver, point, term.Path).RootOrVisibleStateKey()
	if !ok {
		return 0, false
	}
	floor, ok := in.ReadNumFloor(resolver.KeySpace(), stateKey)
	if !ok {
		return 0, false
	}
	return floor, true
}

func dynamicIndexTermCeil(resolver *visibility.Resolver, point cfg.Point, in state.State, term dynamicIndexTerm) (int64, bool) {
	if resolver == nil || term.Path.IsEmpty() {
		return 0, false
	}
	stateKey, ok := visibility.AddressAt(resolver, point, term.Path).RootOrVisibleStateKey()
	if !ok {
		return 0, false
	}
	ceil, ok := in.ReadNumCeil(resolver.KeySpace(), stateKey)
	if !ok {
		return 0, false
	}
	return ceil, true
}

func dynamicIndexModuloLengthProvesRead(
	config Config,
	point cfg.Point,
	source factflow.ValueSource,
	arrayPath pathdom.Path,
	in state.State,
	resolver *visibility.Resolver,
) bool {
	modSource, ok := dynamicIndexPlusOneModuloSource(config, source)
	if !ok {
		return false
	}
	op, ok := config.Facts.ExpressionOperation(modSource.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != "%" {
		return false
	}
	if !dynamicIndexKeyIsArrayLength(config, op.Right(), arrayPath) {
		return false
	}
	if !dynamicIndexArrayLengthKnownAtLeastOne(config, point, arrayPath, in, resolver) {
		return false
	}
	return dynamicIndexModuloDividendHasIntegerSource(config, point, op.Left(), in)
}

func dynamicIndexArrayLengthKnownAtLeastOne(config Config, point cfg.Point, arrayPath pathdom.Path, in state.State, resolver *visibility.Resolver) bool {
	arrayKey, ok := visibility.AddressAt(resolver, point, arrayPath).VisibleStateKey()
	if ok {
		if floor, ok := in.ReadLenFloor(resolver.KeySpace(), arrayKey); ok && floor >= 1 {
			return true
		}
	}
	tableValue, ok := Project(config, point, arrayPath, in)
	if !ok {
		return false
	}
	tableType, ok := config.TypeValues.TypeOf(config.Registry, tableValue)
	return ok && staticallyNonEmptySequenceType(tableType, 0)
}

func dynamicIndexStaticSequenceLengthAtLeast(config Config, point cfg.Point, arrayPath pathdom.Path, in state.State, floor int64) bool {
	if floor <= 0 || config.TypeValues == nil {
		return floor <= 0
	}
	tableValue, ok := Project(config, point, arrayPath, in)
	if !ok {
		return false
	}
	tableType, ok := config.TypeValues.TypeOf(config.Registry, tableValue)
	return ok && staticallyKnownSequenceLengthAtLeast(tableType, floor, 0)
}

func dynamicIndexModuloDividendHasIntegerSource(config Config, point cfg.Point, source factflow.ValueSource, in state.State) bool {
	term, ok := dynamicIndexIntegerTerm(config, source)
	if ok && !term.Path.IsEmpty() {
		value, valueOK := Project(config, point, term.Path, in)
		return valueOK && typevalue.HasIntegerType(config.Registry, value)
	}
	return dynamicIndexSourceHasIntegerType(config, point, source, in)
}

func dynamicIndexSourceHasIntegerType(config Config, point cfg.Point, source factflow.ValueSource, in state.State) bool {
	if value, ok := dynamicIndexExpressionKeyValue(config, point, source, in); ok {
		return typevalue.HasIntegerType(config.Registry, value)
	}
	term, ok := dynamicIndexIntegerTerm(config, source)
	if !ok || term.Path.IsEmpty() {
		return false
	}
	value, ok := Project(config, point, term.Path, in)
	return ok && typevalue.HasIntegerType(config.Registry, value)
}

func staticallyNonEmptySequenceType(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Tuple:
		return len(tt.Elements) > 0
	case *typ.Record:
		member := tt.GetStaticIntIndex(1)
		return member != nil && !member.Optional
	case *typ.Optional:
		return staticallyNonEmptySequenceType(tt.Inner, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !staticallyNonEmptySequenceType(member, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func staticallyKnownSequenceLengthAtLeast(t typ.Type, floor int64, depth int) bool {
	if floor <= 0 {
		return true
	}
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Tuple:
		return int64(len(tt.Elements)) >= floor
	case *typ.Record:
		for i := int64(1); i <= floor; i++ {
			member := tt.GetStaticIntIndex(i)
			if member == nil || member.Optional {
				return false
			}
		}
		return true
	case *typ.Optional:
		return staticallyKnownSequenceLengthAtLeast(tt.Inner, floor, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !staticallyKnownSequenceLengthAtLeast(member, floor, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func checkedAffineInt64(coeff, value, offset int64) (int64, bool) {
	if coeff != 0 && (value > math.MaxInt64/coeff || value < math.MinInt64/coeff) {
		return 0, false
	}
	product := coeff * value
	if (offset > 0 && product > math.MaxInt64-offset) || (offset < 0 && product < math.MinInt64-offset) {
		return 0, false
	}
	return product + offset, true
}

func dynamicIndexPlusOneModuloSource(config Config, source factflow.ValueSource) (factflow.ValueSource, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return factflow.ValueSource{}, false
	}
	op, ok := config.Facts.ExpressionOperation(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != "+" {
		return factflow.ValueSource{}, false
	}
	if c, ok := dynamicIndexIntegerConstant(config, op.Right()); ok && c == 1 && dynamicIndexSourceIsModulo(config, op.Left()) {
		return op.Left(), true
	}
	if c, ok := dynamicIndexIntegerConstant(config, op.Left()); ok && c == 1 && dynamicIndexSourceIsModulo(config, op.Right()) {
		return op.Right(), true
	}
	return factflow.ValueSource{}, false
}

func dynamicIndexSourceIsModulo(config Config, source factflow.ValueSource) bool {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	op, ok := config.Facts.ExpressionOperation(source.ExprRef)
	return ok && op.Kind() == factflow.ExpressionOperationBinary && op.Op() == "%"
}

func dynamicIndexContainerCanDropMissNil(config Config, point cfg.Point, dyn factflow.DynamicIndexExpression, in state.State) bool {
	if config.TypeValues == nil {
		return false
	}
	tableValue, ok := Project(config, point, dyn.TablePathRef(), in)
	if !ok {
		return false
	}
	tableType, ok := config.TypeValues.TypeOf(config.Registry, tableValue)
	return ok && inRangeDynamicIndexContainerNonNil(tableType, 0)
}

type dynamicIndexTerm struct {
	Path   pathdom.Path
	Coeff  int64
	Offset int64
}

func dynamicIndexIntegerTerm(config Config, source factflow.ValueSource) (dynamicIndexTerm, bool) {
	if p, ok := dynamicIndexSourcePath(config, source); ok {
		return dynamicIndexTerm{Path: p, Coeff: 1}, true
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return dynamicIndexTerm{}, false
	}
	if p, ok := config.Facts.ExpressionPathRef(source.ExprRef); ok {
		return dynamicIndexTerm{Path: p, Coeff: 1}, true
	}
	op, ok := config.Facts.ExpressionOperation(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary {
		return dynamicIndexTerm{}, false
	}
	switch op.Op() {
	case "+":
		if term, ok := dynamicIndexTermPlusConstant(config, op.Left(), op.Right(), 1); ok {
			return term, true
		}
		return dynamicIndexTermPlusConstant(config, op.Right(), op.Left(), 1)
	case "-":
		return dynamicIndexTermPlusConstant(config, op.Left(), op.Right(), -1)
	case "*":
		if term, ok := dynamicIndexTermScaled(config, op.Left(), op.Right()); ok {
			return term, true
		}
		return dynamicIndexTermScaled(config, op.Right(), op.Left())
	default:
		return dynamicIndexTerm{}, false
	}
}

func dynamicIndexSourcePath(config Config, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" {
		return pathdom.Path{}, false
	}
	if p, ok := pathaddr.LocalPathFromKey(source.PathKey); ok {
		return p, true
	}
	if sym, _, suffix, ok := pathaddr.ParseResolverPath(source.PathKey); ok && suffix == "" {
		return pathdom.Path{Symbol: sym}, true
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

func dynamicIndexTermScaled(config Config, constSource, termSource factflow.ValueSource) (dynamicIndexTerm, bool) {
	c, ok := dynamicIndexIntegerConstant(config, constSource)
	if !ok || c <= 0 {
		return dynamicIndexTerm{}, false
	}
	term, ok := dynamicIndexIntegerTerm(config, termSource)
	if !ok {
		return dynamicIndexTerm{}, false
	}
	term.Coeff *= c
	term.Offset *= c
	return term, true
}

func dynamicIndexTermPlusConstant(config Config, termSource, constSource factflow.ValueSource, sign int64) (dynamicIndexTerm, bool) {
	term, ok := dynamicIndexIntegerTerm(config, termSource)
	if !ok {
		return dynamicIndexTerm{}, false
	}
	c, ok := dynamicIndexIntegerConstant(config, constSource)
	if !ok {
		return dynamicIndexTerm{}, false
	}
	term.Offset += sign * c
	return term, true
}

func dynamicIndexIntegerConstant(config Config, source factflow.ValueSource) (int64, bool) {
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
}

func dynamicIndexKeyIsArrayLength(config Config, source factflow.ValueSource, arrayPath pathdom.Path) bool {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	op, ok := config.Facts.ExpressionOperation(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationUnary || op.Op() != "#" {
		return false
	}
	left := op.Left()
	if left.Kind != factflow.ValueSourceExpression || !left.HasExpr {
		return dynamicIndexSourcePathEqualsArray(left, arrayPath)
	}
	p, ok := config.Facts.ExpressionPathRef(left.ExprRef)
	return ok && p.Equal(arrayPath)
}

func dynamicIndexSourcePathEqualsArray(source factflow.ValueSource, arrayPath pathdom.Path) bool {
	if arrayPath.IsEmpty() || source.Kind != factflow.ValueSourcePath || source.PathKey == "" {
		return false
	}
	if source.PathKey == arrayPath.Key() {
		return true
	}
	if p, ok := pathaddr.LocalPathFromKey(source.PathKey); ok {
		return p.Equal(arrayPath)
	}
	sym, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey)
	if !ok {
		return false
	}
	p := pathdom.Path{Symbol: sym, Segments: segments}
	return p.Equal(arrayPath)
}

func dynamicIndexDiffProvesLELength(config Config, resolver *visibility.Resolver, point cfg.Point, in state.State, term dynamicIndexTerm, arrayPath pathdom.Path) bool {
	indexKey, indexOK := relationOperandAt(resolver, point, term.Path, false)
	arrayLenKey, arrayOK := relationOperandAt(resolver, point, arrayPath, true)
	if !indexOK || !arrayOK {
		return false
	}
	snap := in.RelConstraints()
	if snap.Bottom || len(snap.Constraints) == 0 {
		return false
	}
	asserted := make([]numeric.NumericConstraint, 0, len(snap.Constraints))
	floorKeys := make(map[pathaddr.StateKey]struct{})
	valueKeys := make([]pathaddr.StateKey, 0, 3)
	for _, c := range snap.Constraints {
		asserted = append(asserted, c.NumericConstraint())
		valueKeys = c.AppendValueStateKeys(valueKeys[:0])
		for _, key := range valueKeys {
			floorKeys[key] = struct{}{}
		}
	}
	for key := range floorKeys {
		if lo, ok := in.ReadNumFloor(resolver.KeySpace(), key); ok {
			asserted = append(asserted, numeric.GeConst{X: key.PathKey(), C: lo})
		}
	}
	goal := numeric.NewScaledLe(term.Coeff, indexKey.NumericKey(), 0, "", arrayLenKey.NumericKey(), -term.Offset)
	return solver.DefaultPortfolio().Entails(asserted, goal) == decision.Valid
}

func relationOperandAt(resolver *visibility.Resolver, point cfg.Point, p pathdom.Path, length bool) (state.RelOperand, bool) {
	stateKey, ok := visibility.AddressAt(resolver, point, p).RootOrVisibleStateKey()
	if !ok {
		return state.RelOperand{}, false
	}
	if length {
		return state.RelLengthOperand(stateKey), true
	}
	return state.RelValueOperand(stateKey), true
}

// dropInBoundsIndexNil removes the soundly-optional nil from an array element
// read when a proven length floor establishes the literal integer index is in
// range: index k >= 1 with len(array) >= k. The decision consults the
// point-local length-floor lane keyed by the array path's visible state key.
// Out-of-floor indices keep their optional nil.
func dropInBoundsIndexNil(config Config, point cfg.Point, p pathdom.Path, in state.State, value product.Value) product.Value {
	reg := config.Registry
	if config.Visibility == nil || len(p.Segments) == 0 {
		return value
	}
	last := p.Segments[len(p.Segments)-1]
	if last.Kind != segment.SegmentIndexInt || last.Index < 1 {
		return value
	}
	arrayKey, keyOK := visibility.AddressAt(config.Visibility, point, p.ParentView()).VisibleStateKey()
	if !keyOK {
		return value
	}
	floor, ok := in.ReadLenFloor(config.Visibility.KeySpace(), arrayKey)
	if !ok || floor < int64(last.Index) {
		return value
	}
	if !parentHasInBoundsIndexWitness(config, point, p.ParentView(), int64(last.Index), floor, in) {
		return value
	}
	return sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
}

func parentHasInBoundsIndexWitness(config Config, point cfg.Point, parent pathdom.Path, index int64, floor int64, in state.State) bool {
	parentValue, ok := project(config, point, parent, in, true)
	if !ok {
		return false
	}
	parentType, ok := config.TypeValues.TypeOf(config.Registry, parentValue)
	return ok && definitelyInBoundsIndexContainerTypeAtFloor(parentType, index, floor, 0)
}

func definitelyInBoundsIndexContainerTypeAtFloor(t typ.Type, index int64, floor int64, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Optional:
		return definitelyInBoundsIndexContainerTypeAtFloor(tt.Inner, index, floor, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		reachable := false
		for _, member := range tt.Members {
			if !typeCanHaveLengthAtLeast(member, floor, depth+1) {
				continue
			}
			reachable = true
			if !definitelyInBoundsIndexContainerTypeAtFloor(member, index, floor, depth+1) {
				return false
			}
		}
		return reachable
	default:
		return definitelyInBoundsIndexContainerType(t, index, depth)
	}
}

func typeCanHaveLengthAtLeast(t typ.Type, floor int64, depth int) bool {
	if floor <= 0 {
		return true
	}
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array, *typ.Map, *typ.ReadonlyMap:
		return true
	case *typ.Tuple:
		return int64(len(tt.Elements)) >= floor
	case *typ.Record:
		if tt.Open || tt.Metatable != nil || tt.HasMapComponent() {
			return true
		}
		for _, member := range tt.StaticMembers {
			if member.Kind == typ.StaticMemberIntIndex && member.Index >= floor && !member.Optional {
				return true
			}
		}
		return false
	case *typ.Optional:
		return typeCanHaveLengthAtLeast(tt.Inner, floor, depth+1)
	case *typ.Union:
		for _, member := range tt.Members {
			if typeCanHaveLengthAtLeast(member, floor, depth+1) {
				return true
			}
		}
		return false
	case *typ.Literal:
		if tt.Base != kind.String {
			return false
		}
		value, ok := tt.Value.(string)
		return ok && int64(len(value)) >= floor
	default:
		switch unwrap.Alias(t).Kind() {
		case kind.String, kind.Any, kind.Unknown:
			return true
		case kind.Never:
			return false
		default:
			return false
		}
	}
}

func definitelyInBoundsIndexContainerType(t typ.Type, index int64, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Tuple:
		return index >= 1 && index <= int64(len(tt.Elements))
	case *typ.Record:
		member := tt.GetStaticIntIndex(index)
		return member != nil && !member.Optional
	case *typ.Optional:
		return definitelyInBoundsIndexContainerType(tt.Inner, index, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !definitelyInBoundsIndexContainerType(member, index, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func inRangeDynamicIndexContainerNonNil(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		elem := tt.Element
		if elem == nil {
			elem = typ.Unknown
		}
		return !typevalue.TypeIncludesNil(elem)
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return false
		}
		for _, elem := range tt.Elements {
			if elem == nil {
				elem = typ.Unknown
			}
			if typevalue.TypeIncludesNil(elem) {
				return false
			}
		}
		return true
	case *typ.Record:
		var found bool
		for i := int64(1); ; i++ {
			member := tt.GetStaticIntIndex(i)
			if member == nil || member.Optional {
				return found
			}
			found = true
			if typevalue.TypeIncludesNil(member.Type) {
				return false
			}
		}
	case *typ.Optional:
		return inRangeDynamicIndexContainerNonNil(tt.Inner, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		reachable := false
		for _, member := range tt.Members {
			if !typeCanHaveLengthAtLeast(member, 1, depth+1) {
				continue
			}
			reachable = true
			if !inRangeDynamicIndexContainerNonNil(member, depth+1) {
				return false
			}
		}
		return reachable
	default:
		return false
	}
}
