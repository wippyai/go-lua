// Package readexpr adapts Lua expression paths to check-body state reads.
package readexpr

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/check/internal/staticmemberwitness"
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type Config struct {
	Registry        *axis.Registry
	Facts           factflow.Facts
	Visibility      *visibility.Resolver
	TypeValues      *typevalue.Cache
	Context         *Context
	ProofState      func(cfg.Point) (state.State, bool)
	ProofVisibility *visibility.Resolver

	active *projectionActive
	memo   *projectionMemo
}

// Context owns reusable projection scratch for one stable read model. Callers
// must not share it across states that can answer the same point/path
// differently.
type Context struct {
	active projectionActive
	memo   projectionMemo
}

func (c *Context) bind(config Config) Config {
	if c == nil {
		return config
	}
	config.active = &c.active
	config.memo = &c.memo
	return config
}

type projectionFrame struct {
	point cfg.Point
	path  projectionPathIdentity
}

type projectionFrameMapKey struct {
	point cfg.Point
	path  projectionPathIdentity
}

type projectionMemoKey struct {
	point       cfg.Point
	path        projectionPathIdentity
	overlayRoot bool
}

type projectionMemoMapKey struct {
	point       cfg.Point
	path        projectionPathIdentity
	overlayRoot bool
}

type projectionResult struct {
	value product.Value
	ok    bool
}

type projectionPathIdentity = keyspace.PathIdentity

func newProjectionPathIdentity(config Config, p pathdom.Path) (projectionPathIdentity, bool) {
	var ks *keyspace.KeySpace
	if config.Visibility != nil {
		ks = config.Visibility.KeySpace()
	}
	return keyspace.PathIdentityFromPath(ks, p)
}

const (
	projectionScratchInline       = 8
	projectionScratchRetainMapMax = 64
)

type projectionActive struct {
	small    [projectionScratchInline]projectionFrame
	smallLen int
	entries  map[projectionFrameMapKey]struct{}
}

func (a *projectionActive) contains(frame projectionFrame) bool {
	if a == nil {
		return false
	}
	for i := 0; i < a.smallLen; i++ {
		if a.small[i].equal(frame) {
			return true
		}
	}
	if a.entries == nil {
		return false
	}
	_, ok := a.entries[frame.mapKey()]
	return ok
}

func (a *projectionActive) push(frame projectionFrame) bool {
	if a == nil || a.contains(frame) {
		return false
	}
	if a.entries == nil && a.smallLen < len(a.small) {
		a.small[a.smallLen] = frame
		a.smallLen++
		return true
	}
	if a.entries == nil {
		a.entries = make(map[projectionFrameMapKey]struct{}, len(a.small)+1)
		for i := 0; i < a.smallLen; i++ {
			a.entries[a.small[i].mapKey()] = struct{}{}
			a.small[i] = projectionFrame{}
		}
		a.smallLen = 0
	}
	a.entries[frame.mapKey()] = struct{}{}
	return true
}

func (a *projectionActive) pop(frame projectionFrame) {
	if a == nil {
		return
	}
	for i := 0; i < a.smallLen; i++ {
		if a.small[i].equal(frame) {
			last := a.smallLen - 1
			a.small[i] = a.small[last]
			a.small[last] = projectionFrame{}
			a.smallLen--
			return
		}
	}
	delete(a.entries, frame.mapKey())
}

func (f projectionFrame) equal(other projectionFrame) bool {
	return f.point == other.point && f.path == other.path
}

func (f projectionFrame) mapKey() projectionFrameMapKey {
	return projectionFrameMapKey(f)
}

type projectionMemoEntry struct {
	key    projectionMemoKey
	result projectionResult
}

type projectionMemo struct {
	small    [projectionScratchInline]projectionMemoEntry
	smallLen int
	entries  map[projectionMemoMapKey]projectionResult
}

type projectionScratch struct {
	active projectionActive
	memo   projectionMemo
}

var projectionScratchPool = sync.Pool{
	New: func() any { return new(projectionScratch) },
}

func getProjectionScratch() *projectionScratch {
	return projectionScratchPool.Get().(*projectionScratch)
}

func putProjectionScratch(scratch *projectionScratch) {
	if scratch == nil {
		return
	}
	scratch.active.reset()
	scratch.memo.reset()
	projectionScratchPool.Put(scratch)
}

func (a *projectionActive) reset() {
	if a == nil {
		return
	}
	for i := 0; i < a.smallLen; i++ {
		a.small[i] = projectionFrame{}
	}
	a.smallLen = 0
	if len(a.entries) > projectionScratchRetainMapMax {
		a.entries = nil
		return
	}
	clear(a.entries)
}

func (m *projectionMemo) lookup(key projectionMemoKey) (projectionResult, bool) {
	if m == nil {
		return projectionResult{}, false
	}
	for i := 0; i < m.smallLen; i++ {
		entry := m.small[i]
		if entry.key.equal(key) {
			return entry.result, true
		}
	}
	if m.entries == nil {
		return projectionResult{}, false
	}
	result, ok := m.entries[key.mapKey()]
	return result, ok
}

func (m *projectionMemo) remember(key projectionMemoKey, result projectionResult) {
	if m == nil {
		return
	}
	for i := 0; i < m.smallLen; i++ {
		if m.small[i].key.equal(key) {
			m.small[i].result = result
			return
		}
	}
	if m.entries == nil && m.smallLen < len(m.small) {
		m.small[m.smallLen] = projectionMemoEntry{key: key, result: result}
		m.smallLen++
		return
	}
	if m.entries == nil {
		m.entries = make(map[projectionMemoMapKey]projectionResult, len(m.small)+1)
		for i := 0; i < m.smallLen; i++ {
			entry := m.small[i]
			m.entries[entry.key.mapKey()] = entry.result
			m.small[i] = projectionMemoEntry{}
		}
		m.smallLen = 0
	}
	m.entries[key.mapKey()] = result
}

func (k projectionMemoKey) equal(other projectionMemoKey) bool {
	return k.point == other.point && k.overlayRoot == other.overlayRoot && k.path == other.path
}

func (k projectionMemoKey) mapKey() projectionMemoMapKey {
	return projectionMemoMapKey(k)
}

func (m *projectionMemo) reset() {
	if m == nil {
		return
	}
	for i := 0; i < m.smallLen; i++ {
		m.small[i] = projectionMemoEntry{}
	}
	m.smallLen = 0
	if len(m.entries) > projectionScratchRetainMapMax {
		m.entries = nil
		return
	}
	clear(m.entries)
}

func Provider(config Config) sourcevalue.ExpressionValueProvider {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	return func(point cfg.Point, expr factflow.ExprRef, _ factflow.ValueSource, in state.State) (product.Value, bool) {
		p, ok := config.Facts.ExpressionPathRef(expr)
		if ok {
			return Project(config, point, p, in)
		}
		dyn, ok := config.Facts.DynamicIndexExpression(expr)
		if !ok {
			return product.Value{}, false
		}
		p, ok = dynamicIndexExpressionPath(config, point, dyn, in)
		if ok {
			value, ok := Project(config, point, p, in)
			if ok {
				if dynamicIndexKeyMembershipProvesRead(config, point, dyn, in) ||
					dynamicIndexInBoundsProvesRead(config, point, dyn, in) {
					value = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
				}
				return value, true
			}
		}
		return dynamicIndexExpressionValue(config, point, dyn, in)
	}
}

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

// DynamicIndexReadProvenPresent reports whether the solved facts prove that a
// dynamic-index expression reads an existing non-nil slot at point. It exposes
// the same proof used by expression projection so diagnostic readmodels do not
// rebuild in-range or key-membership logic independently.
func DynamicIndexReadProvenPresent(config Config, point cfg.Point, expr factflow.ExprRef, in state.State) bool {
	dyn, ok := config.Facts.DynamicIndexExpression(expr)
	if !ok {
		return false
	}
	if dynamicIndexKeyMembershipProvesRead(config, point, dyn, in) ||
		dynamicIndexInBoundsProvesRead(config, point, dyn, in) {
		return true
	}
	if _, ok := dynamicIndexExpressionProvenMemberValue(config, point, dyn, in); ok {
		return true
	}
	return false
}

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
	if term.Coeff != 1 || term.Offset != 0 {
		return false
	}
	indexKey, indexOK := visibility.AddressAt(proofVisibility, point, term.Path).VisibleStateKey()
	arrayKey, arrayOK := visibility.AddressAt(proofVisibility, point, dyn.TablePathRef()).VisibleStateKey()
	return indexOK && arrayOK && proofState.HasIndexInRangeProofForStateKeys(proofVisibility.KeySpace(), indexKey, arrayKey)
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

func Project(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	config = config.Context.bind(config)
	var scratch *projectionScratch
	if config.active == nil || config.memo == nil {
		scratch = getProjectionScratch()
		defer putProjectionScratch(scratch)
		if config.active == nil {
			config.active = &scratch.active
		}
		if config.memo == nil {
			config.memo = &scratch.memo
		}
	}
	value, ok := project(config, point, p, in, true)
	if !ok {
		return product.Value{}, false
	}
	return applyPathPresenceProof(config, point, p, in, value), true
}

// ProjectWithoutRootStaticMemberOverlay projects a path without synthesizing a
// root type witness from current static-member facts. It is for callers that can
// first answer from an existing root witness and only need the expensive overlay
// as a fallback.
func ProjectWithoutRootStaticMemberOverlay(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	config = config.Context.bind(config)
	var scratch *projectionScratch
	if config.active == nil || config.memo == nil {
		scratch = getProjectionScratch()
		defer putProjectionScratch(scratch)
		if config.active == nil {
			config.active = &scratch.active
		}
		if config.memo == nil {
			config.memo = &scratch.memo
		}
	}
	value, ok := project(config, point, p, in, false)
	if !ok {
		return product.Value{}, false
	}
	return applyPathPresenceProof(config, point, p, in, value), true
}

func applyPathPresenceProof(config Config, point cfg.Point, p pathdom.Path, in state.State, value product.Value) product.Value {
	proven, ok := pathPresenceProof(config, point, p, in)
	if !ok {
		return value
	}
	value = product.WithPresence(config.Registry, value, proven)
	if presence.Equal(proven, presence.Present()) {
		value = sourcevalue.WithoutNilRuntimeKind(config.Registry, value)
	}
	return value
}

func pathPresenceProof(config Config, point cfg.Point, p pathdom.Path, in state.State) (presence.Value, bool) {
	if config.Visibility == nil || p.IsEmpty() {
		return presence.Bottom(), false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return presence.Bottom(), false
	}
	snapshot := in.BranchProofsSnapshot(ks)
	if snapshot.Bottom || snapshot.Top || len(snapshot.Proofs) == 0 {
		return presence.Bottom(), false
	}
	address := visibility.AddressAt(config.Visibility, point, p)
	var out presence.Value
	found := false
	address.ForEachStateKey(func(stateKey pathaddr.StateKey) bool {
		key, ok := ks.InternStateKey(stateKey)
		if !ok {
			return true
		}
		for _, proof := range snapshot.Proofs {
			if proof.Kind != pathevidence.BranchProofPathPresence || proof.Path != key {
				continue
			}
			if found && !presence.Equal(out, proof.Presence) {
				found = false
				return false
			}
			out = proof.Presence
			found = true
		}
		return true
	}, visibility.StateKeyVisible, visibility.StateKeyRootOrVisible, visibility.StateKeyStructural)
	if !found {
		return presence.Bottom(), false
	}
	return out, true
}

// ProjectStaticMember reads one proven static member for owner without
// synthesizing a whole root static-member witness.
func ProjectStaticMember(config Config, point cfg.Point, owner pathdom.Path, member segment.Segment, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	config = config.Context.bind(config)
	var scratch *projectionScratch
	if config.active == nil {
		scratch = getProjectionScratch()
		defer putProjectionScratch(scratch)
		config.active = &scratch.active
	}
	if config.Visibility == nil || owner.IsEmpty() {
		return product.Value{}, false
	}
	if memberKey, ok := staticMemberLocalKey(config, point, owner, member); ok {
		value, ok := readStaticMemberLocalValue(config, memberKey, in)
		if !ok {
			return product.Value{}, false
		}
		if current, ok := currentLocalPathKeyValue(config, memberKey, in); ok {
			value = mergeCurrentStaticMemberValue(reg, config.TypeValues, value, current, true)
		}
		return value, true
	}
	memberPath := owner.AppendSegments([]segment.Segment{member})
	memberKey, ok := visibility.AddressAt(config.Visibility, point, memberPath).VisiblePathKey()
	if !ok {
		return product.Value{}, false
	}
	value, ok := readStaticMemberValue(config, memberKey, in)
	if !ok {
		return product.Value{}, false
	}
	if current, ok := currentPathKeyValue(config, point, memberPath, in); ok {
		value = mergeCurrentStaticMemberValue(reg, config.TypeValues, value, current, true)
	}
	return value, true
}

func staticMemberLocalKey(config Config, point cfg.Point, owner pathdom.Path, member segment.Segment) (keyspace.Key, bool) {
	if config.Visibility == nil || owner.IsEmpty() {
		return keyspace.Key{}, false
	}
	ownerKey, ok := visibility.AddressAt(config.Visibility, point, owner).VisibleLocalKeyspaceKey()
	if !ok {
		return keyspace.Key{}, false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return keyspace.Key{}, false
	}
	return ks.AppendSegment(ownerKey, member)
}

func readStaticMemberValue(config Config, pathKey pathdom.PathKey, in state.State) (product.Value, bool) {
	ks := config.Visibility.KeySpace()
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return product.Value{}, false
	}
	return readStaticMemberLocalValue(config, localKey, in)
}

func readStaticMemberLocalValue(config Config, localKey keyspace.Key, in state.State) (product.Value, bool) {
	if config.Visibility == nil {
		return product.Value{}, false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return product.Value{}, false
	}
	value, ok := in.ReadLocalPathStaticMember(localKey)
	if ok {
		return value, true
	}
	canonical, ok := ks.FieldCanonical(localKey)
	if ok {
		if value, ok := in.ReadLocalPathStaticMember(canonical); ok {
			return value, true
		}
	}
	if stable, ok := stableStaticMemberKey(ks, localKey); ok {
		if value, ok := in.ReadLocalPathStaticMember(stable); ok {
			return value, true
		}
		if canonical, ok := ks.FieldCanonical(stable); ok {
			return in.ReadLocalPathStaticMember(canonical)
		}
	}
	return product.Value{}, false
}

func stableStaticMemberKey(ks *keyspace.KeySpace, localKey keyspace.Key) (keyspace.Key, bool) {
	if ks == nil || localKey.Kind != keyspace.KindResolverSym || localKey.Sym == 0 {
		return keyspace.Key{}, false
	}
	segments, ok := ks.SegmentsView(localKey)
	if !ok {
		return keyspace.Key{}, false
	}
	return ks.FromStableSymbol(localKey.Sym, segments)
}

func mergeCurrentStaticMemberValue(reg *axis.Registry, typeValues *typevalue.Cache, fallback, current product.Value, fromPathKey bool) product.Value {
	if currentStaticMemberValueReplacesFallback(reg, typeValues, fallback, current) {
		return current
	}
	if fromPathKey && currentValueHasType(reg, typeValues, current) {
		return current
	}
	if merged := valuerefine.MeetConstraint(reg, fallback, current); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	if fromPathKey {
		return current
	}
	return fallback
}

func project(config Config, point cfg.Point, p pathdom.Path, in state.State, overlayRoot bool) (product.Value, bool) {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	if p.IsEmpty() {
		return product.Value{}, false
	}
	pathID, ok := newProjectionPathIdentity(config, p)
	if !ok {
		return product.Value{}, false
	}
	memoKey := projectionMemoKey{point: point, path: pathID, overlayRoot: overlayRoot}
	if result, cached := config.memo.lookup(memoKey); cached {
		return result.value, result.ok
	}
	frame := projectionFrame{point: point, path: pathID}
	if config.active.contains(frame) {
		return product.Value{}, false
	}
	config.active.push(frame)
	defer config.active.pop(frame)

	if len(p.Segments) == 0 {
		value, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, p, in)
		if !ok {
			return rememberProjection(config, memoKey, product.Value{}, false)
		}
		if !overlayRoot {
			return rememberProjection(config, memoKey, value, true)
		}
		return rememberProjection(config, memoKey, overlayStaticMemberWitness(config, point, p, in, value), true)
	}

	exactPresent := product.Value{}
	hasExactPresent := false
	if exact, ok := exactPathValue(config, point, p, in); ok {
		switch gotPresence := product.PresenceOf(exact); {
		case presence.Equal(gotPresence, presence.Present()):
			exactPresent = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, exact, presence.Present()))
			hasExactPresent = true
			originProjected, hasOriginProjected := projectCurrentVariantOrigin(config, point, p, in)
			if identityvalue.HasExact(reg, exactPresent) {
				if hasOriginProjected {
					return rememberProjection(config, memoKey, mergeOriginProjectedWithExact(reg, originProjected, exactPresent, true), true)
				}
				if projected, ok, _ := projectFromStructuralEvidence(config, point, p, in); ok {
					if merged := product.Meet(reg, projected, exactPresent); !product.Equal(reg, merged, product.Bottom(reg)) {
						return rememberProjection(config, memoKey, merged, true)
					}
				}
				if parentValue, hasParent := project(config, point, p.ParentView(), in, false); hasParent {
					exactPresent = inheritParentTopOriginForExact(reg, exactPresent, parentValue)
				}
				return rememberProjection(config, memoKey, exactPresent, true)
			}
		case presence.Equal(gotPresence, presence.Absent()):
			if projected, ok := projectDynamicOrHeapMember(config, point, p, in, product.Value{}, false); ok {
				return rememberProjection(config, memoKey, projected, true)
			}
			return rememberProjection(config, memoKey, product.Absent(reg), true)
		}
	}
	exactPresentOnlyPresence := hasExactPresent && exactValueOnlyProvesPresence(reg, exactPresent)
	if !hasExactPresent || exactPresentOnlyPresence {
		if projected, ok := projectDynamicOrHeapMember(config, point, p, in, exactPresent, hasExactPresent); ok {
			return projected, true
		}
	}

	originProjected := product.Value{}
	hasOriginProjected := false
	if projected, ok := projectCurrentVariantOrigin(config, point, p, in); ok {
		originProjected = projected
		hasOriginProjected = true
	}

	if hasExactPresent {
		if parentValue, hasParent := project(config, point, p.ParentView(), in, false); hasParent {
			exactPresent = inheritParentTopOriginForExact(reg, exactPresent, parentValue)
		}
	}

	if projected, ok := projectFinalStaticMember(config, point, p, in); ok {
		if hasOriginProjected {
			projected = mergeStructuralAndOriginProjection(reg, projected, originProjected)
		}
		if hasExactPresent {
			if hasOriginProjected {
				return rememberProjection(config, memoKey, mergeOriginProjectedWithExact(reg, projected, exactPresent, true), true)
			}
			return rememberProjection(config, memoKey, mergeProjectedWithExact(reg, projected, exactPresent, true), true)
		}
		return rememberProjection(config, memoKey, dropInBoundsIndexNil(config, point, p, in, projected), true)
	}

	if projected, ok, blocked := projectFromStructuralEvidence(config, point, p, in); ok {
		projected = overlayStaticMemberWitness(config, point, p, in, projected)
		if hasOriginProjected {
			projected = mergeStructuralAndOriginProjection(reg, projected, originProjected)
		}
		if hasExactPresent {
			if hasOriginProjected {
				return rememberProjection(config, memoKey, mergeOriginProjectedWithExact(reg, projected, exactPresent, true), true)
			}
			return rememberProjection(config, memoKey, mergeProjectedWithExact(reg, projected, exactPresent, true), true)
		}
		return rememberProjection(config, memoKey, dropInBoundsIndexNil(config, point, p, in, projected), true)
	} else if blocked && !hasExactPresent {
		return rememberProjection(config, memoKey, product.Value{}, false)
	}

	if hasExactPresent {
		return rememberProjection(config, memoKey, exactPresent, true)
	}
	if hasOriginProjected {
		return rememberProjection(config, memoKey, mergeOriginProjectedWithExact(reg, originProjected, exactPresent, hasExactPresent), true)
	}

	value, hasUnknownIndexValue := unknownIndexReadValue(config, p.Segments[len(p.Segments)-1])
	if !hasUnknownIndexValue {
		return rememberProjection(config, memoKey, product.Value{}, false)
	}
	if parentValue, hasParent := project(config, point, p.ParentView(), in, false); hasParent {
		value = sourcevalue.InheritTopOriginEvidence(reg, value, parentValue)
	}
	return rememberProjection(config, memoKey, dropInBoundsIndexNil(config, point, p, in, value), true)
}

func projectCurrentVariantOrigin(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	projected, ok := projectFromVariantOrigin(config, point, p, in)
	if !ok {
		return product.Value{}, false
	}
	return refineProjectionWithCurrentRootType(config, point, p, in, projected), true
}

func exactPathValue(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	if exact, ok := sourcevalue.ExactPathValue(config.Registry, config.Visibility, point, p, in); ok {
		return exact, true
	}
	if config.ProofVisibility == nil || config.ProofVisibility == config.Visibility {
		return product.Value{}, false
	}
	proofState := in
	if config.ProofState != nil {
		if st, ok := config.ProofState(point); ok {
			proofState = st
		}
	}
	return sourcevalue.ExactPathValue(config.Registry, config.ProofVisibility, point, p, proofState)
}

func inheritParentTopOriginForExact(reg *axis.Registry, exact, parent product.Value) product.Value {
	if exactHasConcreteNonTopProof(reg, exact) {
		return exact
	}
	return sourcevalue.InheritTopOriginEvidence(reg, exact, parent)
}

func exactHasConcreteNonTopProof(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t)
}

func currentValueHasType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	var t typ.Type
	var ok bool
	if typeValues == nil {
		t, ok = typevalue.TypeOf(reg, value)
	} else {
		t, ok = typeValues.TypeOf(reg, value)
	}
	return ok && !weakCallablePlaceholderType(t)
}

func weakCallablePlaceholderType(t typ.Type) bool {
	fn, ok := unwrap.Alias(t).(*typ.Function)
	if !ok || fn == nil {
		return false
	}
	if len(fn.TypeParams) != 0 || len(fn.Params) != 0 || len(fn.Returns) != 1 {
		return false
	}
	return typ.IsAny(fn.Variadic) && typ.IsAny(fn.Returns[0])
}

func rememberProjection(config Config, key projectionMemoKey, value product.Value, ok bool) (product.Value, bool) {
	if config.memo != nil {
		config.memo.remember(key, projectionResult{value: value, ok: ok})
	}
	return value, ok
}

func projectDynamicOrHeapMember(
	config Config,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
	exact product.Value,
	hasExact bool,
) (product.Value, bool) {
	reg := config.Registry
	if dynamicProjected, ok := projectFromDynamicIndexFacts(config, point, p, in); ok {
		dynamicProjected = refineProjectionWithCurrentRootType(config, point, p, in, dynamicProjected)
		dynamicProjected = dropInBoundsIndexNil(config, point, p, in, dynamicProjected)
		if value, ok := strongProjectedValueOrFallback(reg, dynamicProjected, exact, hasExact); ok {
			return value, true
		}
	}
	if heapProjected, ok := projectFromHeapIdentity(config, point, p, in); ok {
		heapProjected = refineProjectionWithCurrentRootType(config, point, p, in, heapProjected)
		heapProjected = dropInBoundsIndexNil(config, point, p, in, heapProjected)
		if value, ok := strongProjectedValueOrFallback(reg, heapProjected, exact, hasExact); ok {
			return value, true
		}
	}
	return product.Value{}, false
}

func mergeStructuralAndOriginProjection(reg *axis.Registry, structural, origin product.Value) product.Value {
	switch {
	case product.LessOrEq(reg, origin, structural):
		return origin
	case product.LessOrEq(reg, structural, origin):
		return structural
	}
	if merged := valuerefine.MeetConstraint(reg, structural, origin); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	return structural
}

func strongProjectedValueOrFallback(reg *axis.Registry, projected, exact product.Value, hasExact bool) (product.Value, bool) {
	value := mergeProjectedWithExact(reg, projected, exact, hasExact)
	if !projectedValueCarriesContent(reg, value) {
		return product.Value{}, false
	}
	return value, true
}

func mergeProjectedWithExact(reg *axis.Registry, projected, exact product.Value, hasExact bool) product.Value {
	if !hasExact {
		return projected
	}
	if merged := valuerefine.MeetConstraint(reg, exact, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	return exact
}

func mergeOriginProjectedWithExact(reg *axis.Registry, projected, exact product.Value, hasExact bool) product.Value {
	if !hasExact {
		return projected
	}
	if merged := valuerefine.MeetConstraint(reg, exact, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	return projected
}

func exactValueOnlyProvesPresence(reg *axis.Registry, value product.Value) bool {
	if reg == nil || !presence.Equal(product.PresenceOf(value), presence.Present()) {
		return false
	}
	if _, ok := reg.LookupErased(evidence.Key.ID()); ok {
		ev := product.Get(reg, value, evidence.Key)
		if ev.IsExplicitTop() || ev.IsGradualTop() {
			return false
		}
	}
	if _, ok := reg.LookupErased(identity.Key.ID()); ok {
		if identityvalue.HasExact(reg, value) {
			return false
		}
	}
	if _, ok := reg.LookupErased(runtimekind.Key.ID()); ok {
		if kindValue := product.Get(reg, value, runtimekind.Key); !kindValue.IsTop() && !runtimekind.Equal(kindValue, runtimekind.Singleton(runtimekind.Table)) {
			return false
		}
	}
	if _, ok := reg.LookupErased(typewitness.Key.ID()); ok {
		if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
			return false
		}
	}
	if _, ok := reg.LookupErased(variantorigin.Key.ID()); ok {
		if origin := product.Get(reg, value, variantorigin.Key); !origin.IsTop() {
			return false
		}
	}
	return true
}

func projectedValueCarriesContent(reg *axis.Registry, value product.Value) bool {
	if reg == nil ||
		product.Equal(reg, value, product.Top()) ||
		product.Equal(reg, value, product.Bottom(reg)) ||
		exactValueOnlyProvesPresence(reg, value) {
		return false
	}
	return true
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

func overlayStaticMemberWitness(config Config, point cfg.Point, root pathdom.Path, in state.State, value product.Value) product.Value {
	reg := config.Registry
	if config.Visibility == nil || !sourcevalue.RuntimeMayBeTable(reg, value, true) {
		return value
	}
	rootKey, ok := visibility.AddressAt(config.Visibility, point, root).VisibleKeyspaceKey()
	if !ok {
		return value
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return value
	}
	selfIndexMember := false
	builder := staticmemberwitness.NewBuilder()
	in.ForEachPathStaticMember(func(memberKey keyspace.Key, memberValue product.Value) bool {
		memberSuffix, ok := ks.ExactRemainderAfterPrefix(memberKey, rootKey)
		if !ok || len(memberSuffix) == 0 {
			return true
		}
		if selfIndexStaticMemberSuffix(memberSuffix) && sameExactIdentity(reg, value, memberValue) {
			selfIndexMember = true
			return false
		}
		if product.Equal(reg, memberValue, product.Bottom(reg)) {
			return true
		}
		memberValue = currentStaticMemberValue(config, point, root, memberSuffix, memberKey, in, memberValue)
		memberType, ok := config.TypeValues.TypeOf(reg, memberValue)
		if !ok || memberType == nil {
			return true
		}
		builder.Add(memberSuffix, memberType)
		return true
	})
	if selfIndexMember {
		return value
	}
	staticType, ok := builder.Build()
	if !ok {
		return value
	}
	if existing, ok := config.TypeValues.TypeOf(reg, value); ok && existing != nil && !typ.IsAny(existing) && !typ.IsUnknown(existing) && !typ.IsNever(existing) {
		if _, isMap := unwrap.Alias(existing).(*typ.Map); isMap {
			// A map's declared type is invariant under a conforming key write (a
			// non-conforming write is a separate assignment error), so the root
			// witness stays the declared map rather than intersecting with the
			// per-key static-member record. Individual key reads remain precise
			// through the static-member facts; preserving the map witness keeps a
			// covariant mutable-map alias from being admitted unsoundly.
			return value
		}
		if merged, ok := mergeStaticMemberWitness(existing, staticType); ok {
			staticType = merged
		} else {
			return value
		}
		if typ.SameNodeOrAcyclicEqual(existing, staticType) {
			return value
		}
	}
	return typevalue.WithWitness(reg, value, staticType)
}

func currentStaticMemberValue(
	config Config,
	point cfg.Point,
	root pathdom.Path,
	suffix []segment.Segment,
	memberKey keyspace.Key,
	in state.State,
	fallback product.Value,
) product.Value {
	if len(suffix) == 0 {
		return fallback
	}
	current, ok := currentLocalPathKeyValue(config, memberKey, in)
	fromPathKey := ok
	if !ok {
		currentPath := root.AppendSegments(suffix)
		current, ok = project(config, point, currentPath, in, false)
		if !ok || !projectedValueCarriesContent(config.Registry, current) {
			return fallback
		}
	}
	if currentStaticMemberValueReplacesFallback(config.Registry, config.TypeValues, fallback, current) {
		return current
	}
	return mergeCurrentStaticMemberValue(config.Registry, config.TypeValues, fallback, current, fromPathKey)
}

func currentStaticMemberValueReplacesFallback(reg *axis.Registry, typeValues *typevalue.Cache, fallback, current product.Value) bool {
	fallbackType, fallbackOK := typeValues.TypeOf(reg, fallback)
	currentType, currentOK := typeValues.TypeOf(reg, current)
	if !fallbackOK || !currentOK {
		return false
	}
	return emptyRecordType(fallbackType) && declaredContainerType(currentType)
}

func emptyRecordType(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	return len(rec.Fields) == 0 &&
		len(rec.StaticMembers) == 0 &&
		rec.Metatable == nil &&
		rec.MapKey == nil &&
		rec.MapValue == nil &&
		!rec.Open
}

func declaredContainerType(t typ.Type) bool {
	switch unwrap.Alias(t).(type) {
	case *typ.Array, *typ.Tuple, *typ.Map, *typ.ReadonlyMap:
		return true
	default:
		return false
	}
}

func currentPathKeyValue(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	if config.Visibility == nil {
		return product.Value{}, false
	}
	pathKey, ok := visibility.AddressAt(config.Visibility, point, p).VisiblePathKey()
	if !ok {
		return product.Value{}, false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return product.Value{}, false
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return product.Value{}, false
	}
	return currentLocalPathKeyValue(config, localKey, in)
}

func currentLocalPathKeyValue(config Config, localKey keyspace.Key, in state.State) (product.Value, bool) {
	if config.Visibility == nil {
		return product.Value{}, false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return product.Value{}, false
	}
	value := in.ReadLocalPathKey(config.Registry, localKey)
	if !projectedValueCarriesContent(config.Registry, value) {
		canonical, ok := ks.FieldCanonical(localKey)
		if !ok {
			return product.Value{}, false
		}
		value = in.ReadLocalPathKey(config.Registry, canonical)
		if !projectedValueCarriesContent(config.Registry, value) {
			return product.Value{}, false
		}
	}
	return value, true
}

func selfIndexStaticMemberSuffix(suffix []segment.Segment) bool {
	if len(suffix) == 0 {
		return false
	}
	last := suffix[len(suffix)-1]
	return (last.Kind == segment.SegmentField || last.Kind == segment.SegmentIndexString) && last.Name == "__index"
}

func sameExactIdentity(reg *axis.Registry, left product.Value, right product.Value) bool {
	leftID, leftOK := product.Get(reg, left, identity.Key).ID()
	rightID, rightOK := product.Get(reg, right, identity.Key).ID()
	return leftOK && rightOK && leftID == rightID
}

func mergeStaticMemberWitness(existing typ.Type, static typ.Type) (typ.Type, bool) {
	return typetable.OverlayRecordMembers(existing, static)
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

func unknownIndexReadValue(config Config, seg segment.Segment) (product.Value, bool) {
	reg := config.Registry
	keyType, ok := luatypeprojection.SegmentKeyType(seg)
	if !ok {
		return product.Value{}, false
	}
	projected, ok := access.RuntimeIndex(typetable.NewMap(typ.Any, typ.Unknown), keyType)
	if !ok {
		return product.Value{}, false
	}
	if typ.IsUnknown(projected) {
		return product.Top(), true
	}
	return config.TypeValues.FromType(reg, projected), true
}

func projectFromHeapIdentity(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	last := p.Segments[len(p.Segments)-1]
	parent := p.ParentView()
	parentProjected := product.Value{}
	hasParentProjected := false
	parentValue, _ := project(config, point, parent, in, false)
	if projected, ok := sourcevalue.HeapMemberFromValue(reg, config.Visibility.KeySpace(), in, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		parentProjected = projected
		hasParentProjected = true
	} else if projected, ok := projectHeapMemberFromRootWitness(config, in, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		parentProjected = projected
		hasParentProjected = true
	} else if projected, ok := projectHeapDynamicMember(config, parentValue, last, in); ok {
		parentProjected = projected
		hasParentProjected = true
	}

	root := p.RootOnly()
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := sourcevalue.HeapMemberFromValue(reg, config.Visibility.KeySpace(), in, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		} else if projected, ok := projectHeapMemberFromRootWitness(config, in, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		} else if projected, ok := projectHeapDynamicDescendant(config, rootValue, p.Segments, in); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}
	if hasParentProjected {
		return parentProjected, true
	}
	if hasRootProjected {
		return rootProjected, true
	}
	return product.Value{}, false
}

func projectHeapMemberFromRootWitness(config Config, in state.State, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || len(suffix) == 0 {
		return product.Value{}, false
	}
	id, ok := identityvalue.ExactID(reg, value)
	if !ok {
		return product.Value{}, false
	}
	object := in.ReadHeapTableObject(reg, id)
	root := object.Root()
	rootID, ok := identityvalue.ExactID(reg, root)
	if !ok || rootID != id {
		return product.Value{}, false
	}
	if product.Equal(reg, product.Meet(reg, root, value), product.Bottom(reg)) {
		return product.Value{}, false
	}
	projected, ok := projectFromValueEvidence(config, root, suffix)
	if !ok {
		return product.Value{}, false
	}
	if ownerPresence := product.PresenceOf(value); !presence.Equal(ownerPresence, presence.Present()) {
		projected = product.WithPresence(reg, projected, presence.Join(product.PresenceOf(projected), ownerPresence))
	}
	return projected, true
}

func projectHeapDynamicDescendant(config Config, root product.Value, suffix []segment.Segment, in state.State) (product.Value, bool) {
	if len(suffix) == 0 {
		return product.Value{}, false
	}
	parent, ok := sourcevalue.HeapMemberFromValue(config.Registry, config.Visibility.KeySpace(), in, root, suffix[:len(suffix)-1])
	if !ok {
		return product.Value{}, false
	}
	return projectHeapDynamicMember(config, parent, suffix[len(suffix)-1], in)
}

func projectHeapDynamicMember(config Config, parent product.Value, last segment.Segment, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || config.Visibility == nil || config.TypeValues == nil {
		return product.Value{}, false
	}
	id, ok := identityvalue.ExactID(reg, parent)
	if !ok {
		return product.Value{}, false
	}
	object := in.ReadHeapTableObject(reg, id)
	domain := product.Domain(reg)
	joined := domain.Bottom()
	found := false
	maybeMissing := false
	for _, fact := range object.DynamicIndexFacts() {
		if fact.Admission == dynamicindex.AdmissionRejected || domain.Equal(fact.Value, domain.Bottom()) {
			continue
		}
		if dynamicIndexFactDefinitelyMatchesSegment(reg, config.TypeValues, fact, last) {
			if !found {
				joined = fact.Value
				found = true
			} else {
				joined = domain.Join(joined, fact.Value)
			}
			continue
		}
		if dynamicIndexFactMayMatchSegment(reg, config.TypeValues, fact, last) {
			maybeMissing = true
			if !found {
				joined = fact.Value
				found = true
			} else {
				joined = domain.Join(joined, fact.Value)
			}
		}
	}
	if !found {
		return product.Value{}, false
	}
	if maybeMissing {
		joined = product.Join(reg, joined, product.Absent(reg))
	}
	return joined, true
}

func projectFromVariantOrigin(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || len(p.Segments) == 0 {
		return product.Value{}, false
	}
	rootValue, ok := project(config, point, p.RootOnly(), in, false)
	if !ok {
		return product.Value{}, false
	}
	origin := product.Get(reg, rootValue, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return product.Value{}, false
	}
	family, cases, ok := variant.ProjectOrigin(origin.Family(), origin.CasesRef(), p.Segments)
	projectedOrigin := variantorigin.Value{}
	hasProjectedOrigin := ok
	if hasProjectedOrigin {
		projectedOrigin = variantorigin.Of(family, cases)
	}
	if rootType, ok := config.TypeValues.TypeFromVariantOrigin(origin.Family(), origin.CasesRef()); ok {
		if value, ok := projectTypeThroughPath(config, p, rootValue, rootType, projectedOrigin, hasProjectedOrigin); ok {
			return refineProjectionWithRootType(config, p, rootValue, value), true
		}
	}
	if rootType, ok := typevalue.StructuralTypeOf(reg, config.TypeValues, rootValue, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	}); ok {
		if value, ok := projectTypeThroughPath(config, p, rootValue, rootType, projectedOrigin, hasProjectedOrigin); ok {
			return refineProjectionWithRootType(config, p, rootValue, value), true
		}
	}
	if !hasProjectedOrigin {
		return product.Value{}, false
	}
	if t, ok := config.TypeValues.TypeFromVariantOrigin(family, cases); ok {
		value := projectedPathValue(reg, config.TypeValues, t)
		value = product.Set(reg, value, variantorigin.Key, projectedOrigin)
		value = inheritProjectedParentPresence(reg, value, rootValue)
		if projectedValueCarriesContent(reg, value) {
			return refineProjectionWithRootType(config, p, rootValue, value), true
		}
	}
	value := product.Set(reg, product.Top(), variantorigin.Key, projectedOrigin)
	return refineProjectionWithRootType(config, p, rootValue, inheritProjectedParentPresence(reg, value, rootValue)), true
}

func projectTypeThroughPath(
	config Config,
	p pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	projectedOrigin variantorigin.Value,
	hasProjectedOrigin bool,
) (product.Value, bool) {
	projectedType, ok := luatypeprojection.ApplySegments(rootType, p.Segments)
	if !ok {
		return product.Value{}, false
	}
	value := projectedPathValue(config.Registry, config.TypeValues, projectedType)
	if hasProjectedOrigin {
		value = product.Set(config.Registry, value, variantorigin.Key, projectedOrigin)
	}
	value = inheritProjectedParentPresence(config.Registry, value, rootValue)
	if !projectedValueCarriesContent(config.Registry, value) {
		return product.Value{}, false
	}
	return value, true
}

func refineProjectionWithCurrentRootType(
	config Config,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
	projected product.Value,
) product.Value {
	if len(p.Segments) == 0 {
		return projected
	}
	rootValue, ok := project(config, point, p.RootOnly(), in, false)
	if !ok {
		return projected
	}
	return refineProjectionWithRootType(config, p, rootValue, projected)
}

func projectFinalStaticMember(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	if len(p.Segments) == 0 {
		return product.Value{}, false
	}
	parent := p.ParentView()
	parentValue, hasParent := project(config, point, parent, in, false)
	if !sourcevalue.RuntimeMayBeTable(config.Registry, parentValue, hasParent) {
		return product.Value{}, false
	}
	member := p.Segments[len(p.Segments)-1]
	value, ok := ProjectStaticMember(config, point, parent, member, in)
	if !ok {
		return product.Value{}, false
	}
	if hasParent {
		value = inheritProjectedParentPresence(config.Registry, value, parentValue)
		value = mergeFinalStaticMemberWithCurrentParent(config, p, parentValue, value)
	}
	return value, true
}

func mergeFinalStaticMemberWithCurrentParent(config Config, p pathdom.Path, parentValue product.Value, value product.Value) product.Value {
	if len(p.Segments) == 0 {
		return value
	}
	current, ok := projectFromValueEvidence(config, parentValue, p.Segments[len(p.Segments)-1:])
	if !ok {
		return value
	}
	if product.LessOrEq(config.Registry, current, value) {
		return current
	}
	if product.LessOrEq(config.Registry, value, current) {
		return value
	}
	if merged := valuerefine.MeetConstraint(config.Registry, value, current); !product.Equal(config.Registry, merged, product.Bottom(config.Registry)) {
		return merged
	}
	return current
}

func refineProjectionWithRootType(config Config, p pathdom.Path, rootValue, projected product.Value) product.Value {
	rootType, ok := config.TypeValues.TypeOf(config.Registry, rootValue)
	if !ok || rootType == nil {
		return projected
	}
	rootProjected, ok := projectTypeThroughPath(config, p, rootValue, rootType, variantorigin.Value{}, false)
	if !ok {
		return projected
	}
	refined := valuerefine.MeetConstraint(config.Registry, projected, rootProjected)
	if !product.Equal(config.Registry, refined, product.Bottom(config.Registry)) {
		return refined
	}
	return projected
}

func projectFromStructuralEvidence(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool, bool) {
	reg := config.Registry
	root := p.RootOnly()
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := projectFromValueEvidence(config, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}

	parent := p.ParentView()
	parentValue, hasParent := project(config, point, parent, in, false)
	if !sourcevalue.RuntimeMayBeTable(reg, parentValue, hasParent) {
		return product.Value{}, false, true
	}
	if projected, ok := projectFromValueEvidence(config, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		// The parent-relative projection observes per-segment narrowing recorded
		// on the intermediate path (e.g. a truthy guard that removed nil from an
		// optional field), so it is at least as precise as a single root-relative
		// projection across the full suffix. Meeting them keeps a narrowed
		// non-optional result instead of re-widening it with the root's optional.
		if hasRootProjected {
			if merged := valuerefine.MeetConstraint(reg, rootProjected, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true, false
			}
		}
		return projected, true, false
	}
	if nilValue, ok := projectMissingFinalSegmentAsNil(config, in, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		return nilValue, true, false
	}

	if hasRootProjected {
		return rootProjected, true, false
	}

	return product.Value{}, false, false
}

func projectFromValueEvidence(config Config, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	reg := config.Registry
	if len(suffix) == 0 {
		return product.Value{}, false
	}
	parentType, ok := typevalue.StructuralTypeOf(reg, config.TypeValues, value, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok {
		return product.Value{}, false
	}
	projected, ok := luatypeprojection.ApplySegments(parentType, suffix)
	if !ok {
		return product.Value{}, false
	}
	projectedValue := projectedPathValue(reg, config.TypeValues, projected)
	return inheritProjectedParentPresence(reg, projectedValue, value), true
}

func inheritProjectedParentPresence(reg *axis.Registry, projected, parent product.Value) product.Value {
	parentPresence := product.PresenceOf(parent)
	if presence.Equal(parentPresence, presence.Present()) {
		return projected
	}
	return product.WithPresence(reg, projected, presence.Join(product.PresenceOf(projected), parentPresence))
}

func projectedPathValue(reg *axis.Registry, typeValues *typevalue.Cache, t typ.Type) product.Value {
	value := typeValues.FromTypeWithWitness(reg, t)
	if t != nil && !typevalue.ProjectionHasNil(t) {
		value = product.WithPresence(reg, value, presence.Present())
	}
	return value
}

func projectMissingFinalSegmentAsNil(config Config, in state.State, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	if len(suffix) != 1 {
		return product.Value{}, false
	}
	id, ok := identityvalue.ExactID(config.Registry, value)
	if !ok || !localExclusivePlacementProvesMissingSlotNil(in.ReadPlacement(id)) {
		return product.Value{}, false
	}
	parentType, ok := typevalue.StructuralTypeOf(config.Registry, config.TypeValues, value, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok || parentType == nil || typ.IsAny(parentType) || typ.IsUnknown(parentType) || typ.IsNever(parentType) {
		return product.Value{}, false
	}
	_, ok = luatypeprojection.ApplySegments(parentType, suffix)
	if ok || !access.MissingFieldReadsNil(parentType) {
		return product.Value{}, false
	}
	return inheritProjectedParentPresence(config.Registry, typevalue.Nil(config.Registry), value), true
}

func localExclusivePlacementProvesMissingSlotNil(place placement.Value) bool {
	return place == placement.Stack || place == placement.OwnedHeap
}
