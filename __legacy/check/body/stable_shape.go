package body

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// StableShapeFact is the checker-facing proof that a receiver has a final,
// closed record shape at Point. It is deliberately absent when the analysis can
// prove only the current field value but not the table's future structural
// stability.
type StableShapeFact struct {
	Point    cfg.Point
	Identity identity.ID
	Tier     StableShapeTier
	Shape    typ.Type
	Fields   []StableShapeField
}

type StableShapeTier uint8

const (
	StableShapeTierUnknown StableShapeTier = iota
	StableShapeTierStable
	StableShapeTierPrefixStable
	StableShapeTierStableAfterPoint
)

func (t StableShapeTier) String() string {
	switch t {
	case StableShapeTierStable:
		return "stable"
	case StableShapeTierPrefixStable:
		return "prefix-stable"
	case StableShapeTierStableAfterPoint:
		return "stable-after-p"
	default:
		return "unknown"
	}
}

type StableShapeField struct {
	Name string
	Type typ.Type
}

// StableShapeForStaticMemberRead returns the final-shape proof for a static
// field read's receiver, when the proof is strong enough for fixed-offset reads.
func (r *Result) StableShapeForStaticMemberRead(occ StaticMemberReadOccurrence) (StableShapeFact, bool) {
	var fact StableShapeFact
	var ok bool
	if occ.HasReceiverValueBeforeBoundary {
		receiver := pathdom.Path{}
		if occ.HasReceiverPath {
			receiver = occ.ReceiverPath
		}
		fact, ok = r.stableShapeForValue(occ.Point, occ.ReceiverValueBeforeBoundary, receiver, false)
	} else if occ.HasReceiverValueAtBoundary {
		receiver := pathdom.Path{}
		if occ.HasReceiverPath {
			receiver = occ.ReceiverPath
		}
		fact, ok = r.stableShapeForValue(occ.Point, occ.ReceiverValueAtBoundary, receiver, true)
	}
	if !ok {
		fact, ok = r.stableShapeForImportedModuleRead(occ)
	}
	if !ok || !stableShapeFactContainsMember(fact, occ.MemberName) {
		return StableShapeFact{}, false
	}
	return fact, true
}

// StableShapeForPathAtBoundary resolves p and returns a final-shape proof at
// the diagnostic/call boundary.
func (r *Result) StableShapeForPathAtBoundary(point cfg.Point, p pathdom.Path) (StableShapeFact, bool) {
	value, ok := r.PathValueAtBoundary(point, p)
	if !ok {
		return StableShapeFact{}, false
	}
	return r.stableShapeForValue(point, value, p, true)
}

// StableShapeForValueAtBoundary returns a final-shape proof for value at the
// diagnostic/call boundary.
func (r *Result) StableShapeForValueAtBoundary(point cfg.Point, value product.Value) (StableShapeFact, bool) {
	return r.stableShapeForValue(point, value, pathdom.Path{}, true)
}

// SourceHasStableShapeBeforeBoundary reports whether source carries a final
// shape proof before point's transfer. Summary projection uses this variant so
// returned stack-born tables can be marked stable before return transfer moves
// them into owned-heap placement.
func (r *Result) SourceHasStableShapeBeforeBoundary(point cfg.Point, source factflow.ValueSource) bool {
	value, ok := r.SourceValueBeforeBoundary(point, source)
	if !ok {
		return false
	}
	p, _ := r.valueSourcePath(source)
	fact, stable := r.stableShapeForValue(point, value, p, false)
	return stable && fact.Tier != StableShapeTierPrefixStable
}

// ValueHasStableShapeBeforeBoundary is the value-only companion to
// SourceHasStableShapeBeforeBoundary.
func (r *Result) ValueHasStableShapeBeforeBoundary(point cfg.Point, value product.Value) bool {
	fact, stable := r.stableShapeForValue(point, value, pathdom.Path{}, false)
	return stable && fact.Tier != StableShapeTierPrefixStable
}

// ValueHasStructuralMutationAtBoundary reports whether value's table identity is
// structurally written anywhere in this body. It is broader than stable-shape
// eligibility: array/map element facts can be invariant even when they are not a
// closed record shape.
func (r *Result) ValueHasStructuralMutationAtBoundary(point cfg.Point, value product.Value, receiver pathdom.Path) bool {
	if r == nil || r.registry == nil || r.Graph() == nil {
		return false
	}
	id, ok := identityvalue.ExactID(r.registry, value)
	if !ok {
		return false
	}
	graph := r.Graph()
	entry := graph.Entry()
	for _, candidate := range graph.RPO() {
		if !r.PointCanReach(entry, candidate) {
			continue
		}
		if r.structuralMutationMayTarget(candidate, id, receiver) {
			return true
		}
	}
	return false
}

func (r *Result) stableShapeForValue(point cfg.Point, value product.Value, receiver pathdom.Path, boundary bool) (StableShapeFact, bool) {
	if r == nil || r.registry == nil || r.Graph() == nil {
		return StableShapeFact{}, false
	}
	st, ok := r.stableShapeState(point, boundary)
	if !ok {
		return StableShapeFact{}, false
	}
	id, ok := identityvalue.ExactID(r.registry, value)
	if !ok {
		return StableShapeFact{}, false
	}
	object := st.ReadHeapTableObject(r.registry, id)
	if heapidentity.ObjectDomain(r.registry).Equal(object, heapidentity.BottomObject(r.registry)) {
		return StableShapeFact{}, false
	}
	if r.stableShapeOriginProven(st, id, object) {
		shape, fields, ok := r.closedStableShapeFromObject(object)
		if ok &&
			!r.priorStructuralMutationDisqualifies(point, id, receiver) &&
			!r.futureStructuralMutationReachable(point, id, receiver) {
			return StableShapeFact{
				Point:    point,
				Identity: id,
				Tier:     r.closedStableShapeTier(st, id, object),
				Shape:    shape,
				Fields:   fields,
			}, true
		}
	}
	if !r.prefixStableShapeOriginProven(object) {
		return StableShapeFact{}, false
	}
	shape, fields, ok := r.prefixStableShapeFromObject(object)
	if !ok {
		return StableShapeFact{}, false
	}
	fieldSet := stableShapeFieldSet(fields)
	if r.priorPrefixKillingWrite(point, id, receiver, fieldSet) {
		return StableShapeFact{}, false
	}
	if r.futurePrefixKillingMutationReachable(point, id, receiver, fieldSet) {
		return StableShapeFact{}, false
	}
	return StableShapeFact{
		Point:    point,
		Identity: id,
		Tier:     StableShapeTierPrefixStable,
		Shape:    shape,
		Fields:   fields,
	}, true
}

func (r *Result) stableShapeState(point cfg.Point, boundary bool) (state.State, bool) {
	if boundary {
		return r.StateAtBoundary(point)
	}
	return r.StateAt(point)
}

func (r *Result) stableShapeOriginProven(st state.State, id identity.ID, object heapidentity.TableObject) bool {
	switch st.ReadPlacement(id) {
	case placement.Stack:
		return object.PrefixStableShape() || object.StableShape()
	case placement.OwnedHeap:
		return object.StableShape() || st.IsTableFrozen(id)
	default:
		return object.StableShape() && st.IsTableFrozen(id)
	}
}

func (r *Result) closedStableShapeTier(st state.State, id identity.ID, object heapidentity.TableObject) StableShapeTier {
	if object.StableShape() || st.IsTableFrozen(id) {
		return StableShapeTierStable
	}
	return StableShapeTierStableAfterPoint
}

func (r *Result) prefixStableShapeOriginProven(object heapidentity.TableObject) bool {
	return object.PrefixStableShape() || object.StableShape()
}

func (r *Result) stableShapeForImportedModuleRead(occ StaticMemberReadOccurrence) (StableShapeFact, bool) {
	if r == nil || !occ.HasReceiverPath || !occ.HasReceiverTypeBeforeBoundary {
		return StableShapeFact{}, false
	}
	if len(occ.ReceiverPath.Segments) != 0 || occ.ReceiverPath.Root == "" {
		return StableShapeFact{}, false
	}
	if _, ok := r.RequireAliasModulePath(occ.ReceiverPath.Root); !ok {
		return StableShapeFact{}, false
	}
	shape, fields, ok := r.closedStableShapeFromType(occ.ReceiverTypeBeforeBoundary)
	if !ok {
		return StableShapeFact{}, false
	}
	return StableShapeFact{
		Point:  occ.Point,
		Tier:   StableShapeTierStable,
		Shape:  shape,
		Fields: fields,
	}, true
}

func (r *Result) closedStableShapeFromObject(object heapidentity.TableObject) (typ.Type, []StableShapeField, bool) {
	if len(object.DynamicIndexFacts()) != 0 {
		return nil, nil, false
	}
	if object.StableShape() {
		if shape, fields, ok := r.closedStableShapeFromRootType(object.Root()); ok {
			return shape, fields, true
		}
	}
	return r.stableShapeFromStaticMembers(object)
}

func (r *Result) prefixStableShapeFromObject(object heapidentity.TableObject) (typ.Type, []StableShapeField, bool) {
	if len(object.DynamicIndexFacts()) != 0 {
		return nil, nil, false
	}
	if object.StableShape() {
		if shape, fields, ok := r.closedStableShapeFromRootType(object.Root()); ok {
			return shape, fields, true
		}
	}
	return r.stableShapeFromStaticMembers(object)
}

func (r *Result) stableShapeFromStaticMembers(object heapidentity.TableObject) (typ.Type, []StableShapeField, bool) {
	ks := r.KeySpace()
	if ks == nil {
		return nil, nil, false
	}
	fieldTypes := make(map[string]typ.Type)
	intTypes := make(map[int]typ.Type)
	for key, value := range object.StaticMembers() {
		if product.Equal(r.registry, value, product.Bottom(r.registry)) || typevalue.HasOnlyNilType(r.registry, value) {
			continue
		}
		segs, ok := ks.SuffixSegmentsView(key)
		if !ok || len(segs) != 1 {
			continue
		}
		t := r.stableShapeMemberType(value)
		switch segs[0].Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			name := segs[0].Name
			if name == "" {
				continue
			}
			fieldTypes[name] = mergeStableShapeFieldType(fieldTypes[name], t)
		case segment.SegmentIndexInt:
			intTypes[segs[0].Index] = mergeStableShapeFieldType(intTypes[segs[0].Index], t)
		}
	}
	if len(fieldTypes) == 0 && len(intTypes) == 0 {
		return nil, nil, false
	}
	builder := table.NewRecord()
	fields := make([]StableShapeField, 0, len(fieldTypes)+len(intTypes))
	names := make([]string, 0, len(fieldTypes))
	for name := range fieldTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := fieldTypes[name]
		builder.Field(name, t)
		fields = append(fields, StableShapeField{Name: name, Type: t})
	}
	ints := make([]int, 0, len(intTypes))
	for index := range intTypes {
		ints = append(ints, index)
	}
	sort.Slice(ints, func(i, j int) bool { return ints[i] < ints[j] })
	for _, index := range ints {
		t := intTypes[index]
		builder.StaticIntIndex(int64(index), t)
		fields = append(fields, StableShapeField{Name: segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentIndexInt, Index: index}}), Type: t})
	}
	return builder.Build(), fields, true
}

func (r *Result) closedStableShapeFromRootType(value product.Value) (typ.Type, []StableShapeField, bool) {
	t, ok := typevalue.TypeOf(r.registry, value)
	if !ok || t == nil {
		return nil, nil, false
	}
	return r.closedStableShapeFromType(t)
}

func (r *Result) closedStableShapeFromType(t typ.Type) (typ.Type, []StableShapeField, bool) {
	rec, ok := unwrap.Annotated(t).(*typ.Record)
	if !ok || rec.Open || rec.HasMapComponent() || rec.Metatable != nil {
		return nil, nil, false
	}
	if len(rec.Fields) == 0 && len(rec.StaticMembers) == 0 {
		return nil, nil, false
	}
	fields := make([]StableShapeField, 0, len(rec.Fields)+len(rec.StaticMembers))
	for _, field := range rec.Fields {
		if field.Optional {
			return nil, nil, false
		}
		fields = append(fields, StableShapeField{Name: field.Name, Type: field.Type})
	}
	for _, member := range rec.StaticMembers {
		if member.Optional {
			return nil, nil, false
		}
		switch member.Kind {
		case typ.StaticMemberStringIndex:
			fields = append(fields, StableShapeField{Name: member.Name, Type: member.Type})
		case typ.StaticMemberIntIndex:
			fields = append(fields, StableShapeField{
				Name: segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentIndexInt, Index: int(member.Index)}}),
				Type: member.Type,
			})
		}
	}
	return rec, fields, true
}

func (r *Result) stableShapeMemberType(value product.Value) typ.Type {
	if fn, ok := r.FunctionValueTypeForValue(value); ok && fn != nil {
		return fn
	}
	if t, ok := typevalue.TypeOf(r.registry, value); ok && t != nil {
		return t
	}
	return typ.Unknown
}

func mergeStableShapeFieldType(existing, next typ.Type) typ.Type {
	if existing == nil {
		return next
	}
	if next == nil || typ.TypeEquals(existing, next) {
		return existing
	}
	return typeexpr.Union(existing, next)
}

func stableShapeFactContainsMember(fact StableShapeFact, name string) bool {
	if name == "" {
		return true
	}
	for _, field := range fact.Fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

func stableShapeFieldSet(fields []StableShapeField) map[string]struct{} {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field.Name != "" {
			out[field.Name] = struct{}{}
		}
	}
	return out
}

func (r *Result) priorStructuralMutationDisqualifies(point cfg.Point, id identity.ID, receiver pathdom.Path) bool {
	graph := r.Graph()
	if graph == nil {
		return false
	}
	entry := graph.Entry()
	for _, candidate := range graph.RPO() {
		if candidate == point {
			continue
		}
		if !r.PointCanReach(entry, candidate) || !r.PointCanReach(candidate, point) {
			continue
		}
		if !r.structuralMutationMayTarget(candidate, id, receiver) {
			continue
		}
		if !r.structuralMutationIsStaticMemberWrite(candidate, id) {
			if r.structuralMutationIsHeapObjectBirth(candidate, id) && r.PointDominates(candidate, point) {
				continue
			}
			return true
		}
		if !r.PointDominates(candidate, point) {
			return true
		}
	}
	return false
}

func (r *Result) structuralMutationIsStaticMemberWrite(point cfg.Point, id identity.ID) bool {
	write, ok := r.facts.PathStaticMemberWrite(point)
	return ok && r.pathBeforeBoundaryHasIdentity(point, write.TargetPathRef().Parent(), id)
}

func (r *Result) structuralMutationIsHeapObjectBirth(point cfg.Point, id identity.ID) bool {
	before, beforeOK := r.StateAt(point)
	after, afterOK := r.StateAtBoundary(point)
	if !beforeOK || !afterOK {
		return false
	}
	domain := heapidentity.ObjectDomain(r.registry)
	return domain.Equal(before.ReadHeapTableObject(r.registry, id), domain.Bottom()) &&
		!domain.Equal(after.ReadHeapTableObject(r.registry, id), domain.Bottom())
}

func (r *Result) futureStructuralMutationReachable(point cfg.Point, id identity.ID, receiver pathdom.Path) bool {
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == point {
			continue
		}
		if !r.PointCanReach(point, candidate) {
			continue
		}
		if r.structuralMutationMayTarget(candidate, id, receiver) {
			return true
		}
	}
	return false
}

func (r *Result) priorPrefixKillingWrite(point cfg.Point, id identity.ID, receiver pathdom.Path, fields map[string]struct{}) bool {
	graph := r.Graph()
	if graph == nil {
		return false
	}
	entry := graph.Entry()
	for _, candidate := range graph.RPO() {
		if candidate == point {
			continue
		}
		if !r.PointCanReach(entry, candidate) || !r.PointCanReach(candidate, point) {
			continue
		}
		kind := r.prefixKillingMutationKind(candidate, id, receiver, fields)
		switch kind {
		case prefixMutationNone:
			continue
		case prefixMutationStaticPrefixWrite:
			if !r.PointDominates(candidate, point) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func (r *Result) futurePrefixKillingMutationReachable(point cfg.Point, id identity.ID, receiver pathdom.Path, fields map[string]struct{}) bool {
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == point {
			continue
		}
		if !r.PointCanReach(point, candidate) {
			continue
		}
		if r.callSiteReferencesIdentityAt(candidate, id, receiver) &&
			r.callMayStructurallyMutateIdentity(candidate, id, receiver) {
			return true
		}
		if r.prefixKillingMutationKind(candidate, id, receiver, fields) != prefixMutationNone {
			return true
		}
	}
	return false
}

type prefixMutationKind uint8

const (
	prefixMutationNone prefixMutationKind = iota
	prefixMutationStaticPrefixWrite
	prefixMutationUnknown
)

func (r *Result) prefixKillingMutationKind(point cfg.Point, id identity.ID, receiver pathdom.Path, fields map[string]struct{}) prefixMutationKind {
	if write, ok := r.facts.PathStaticMemberWrite(point); ok {
		target := write.TargetPathRef()
		if r.pathBeforeBoundaryHasIdentity(point, target.Parent(), id) {
			name, ok := staticMemberWriteName(target)
			if !ok {
				return prefixMutationUnknown
			}
			if _, inPrefix := fields[name]; inPrefix && !r.pathBeforeBoundaryHasStackLocalIdentity(point, target.Parent(), id) {
				return prefixMutationStaticPrefixWrite
			}
			return prefixMutationNone
		}
	}
	if write, ok := r.facts.DynamicIndexWrite(point); ok {
		if r.pathBeforeBoundaryHasIdentity(point, write.TablePathRef(), id) {
			return prefixMutationUnknown
		}
	}
	if invalidation, ok := r.facts.PathDescendantInvalidation(point); ok {
		if r.pathBeforeBoundaryHasIdentity(point, invalidation.ContainerPathRef(), id) {
			return prefixMutationUnknown
		}
		if tablePath, _, _, ok := invalidation.DynamicTargetRef(); ok && r.pathBeforeBoundaryHasIdentity(point, tablePath, id) {
			return prefixMutationUnknown
		}
	}
	if kind, ok := r.callPrefixMutationKind(point, id, receiver, fields); ok {
		return kind
	}
	return prefixMutationNone
}

func (r *Result) callPrefixMutationKind(point cfg.Point, id identity.ID, receiver pathdom.Path, fields map[string]struct{}) (prefixMutationKind, bool) {
	if r == nil || !r.callSiteExists(point) {
		return prefixMutationNone, false
	}
	outcome, hasOutcome := r.CallOutcomeAt(point)
	if !hasOutcome {
		if r.callMayStructurallyMutateIdentity(point, id, receiver) {
			return prefixMutationUnknown, true
		}
		return prefixMutationNone, true
	}
	if !receiver.IsEmpty() && r.callOutcomeHasCovariantExposureForTargetAt(point, outcome, receiver) {
		return prefixMutationUnknown, true
	}
	if !r.callOutcomeHasExactGuardInvalidationSummaryAt(point, outcome, true) || CallOutcomeHasGlobalGuardInvalidation(outcome) {
		if r.callMayStructurallyMutateIdentity(point, id, receiver) {
			return prefixMutationUnknown, true
		}
		return prefixMutationNone, true
	}
	invalidations, ok := r.callOutcomeGuardInvalidationPathsAt(point, outcome)
	if !ok {
		return prefixMutationUnknown, true
	}
	for _, invalidation := range invalidations {
		if invalidation.RootRebinding {
			if r.pathBeforeBoundaryHasIdentity(point, invalidation.Path, id) {
				return prefixMutationUnknown, true
			}
			continue
		}
		name, targets := r.callInvalidationStaticMemberName(point, id, invalidation.Path)
		if !targets {
			continue
		}
		if name == "" || !invalidation.PreserveStructuralWitness {
			return prefixMutationUnknown, true
		}
		if _, inPrefix := fields[name]; !inPrefix {
			continue
		}
		if r.callStaticMemberExistedBefore(point, id, invalidation.Path) {
			return prefixMutationUnknown, true
		}
		if !r.callOutcomeHasStaticMemberDeltaAt(point, outcome, invalidation.Path) &&
			!r.callOutcomeHasConcreteStaticMemberFactAt(point, outcome, invalidation.Path) {
			return prefixMutationUnknown, true
		}
	}
	return prefixMutationNone, true
}

func (r *Result) callInvalidationStaticMemberName(point cfg.Point, id identity.ID, target pathdom.Path) (string, bool) {
	if target.IsEmpty() {
		return "", false
	}
	if len(target.Segments) == 0 {
		return "", r.pathBeforeBoundaryHasIdentity(point, target, id)
	}
	if !r.pathBeforeBoundaryHasIdentity(point, target.Parent(), id) {
		return "", false
	}
	name, ok := staticMemberWriteName(target)
	if !ok {
		return "", true
	}
	return name, true
}

func (r *Result) callStaticMemberExistedBefore(point cfg.Point, id identity.ID, target pathdom.Path) bool {
	if r == nil || len(target.Segments) == 0 {
		return false
	}
	before, ok := r.StateAt(point)
	if !ok {
		return false
	}
	object := before.ReadHeapTableObject(r.registry, id)
	key, ok := heapidentity.StaticMemberSuffixKey(r.KeySpace(), target.Segments[len(target.Segments)-1:])
	if !ok {
		return false
	}
	_, ok = object.StaticMember(key)
	return ok
}

func (r *Result) callOutcomeHasStaticMemberDeltaAt(point cfg.Point, outcome callpayload.CallOutcome, target pathdom.Path) bool {
	site, ok := r.CallSiteView(point)
	if !ok {
		return false
	}
	bindings := r.callGuardCallBindings(site)
	for _, fact := range outcome.NormalReturnFacts.PathStaticMemberDeltas {
		substituted, ok := fact.Path.Substitute(bindings)
		if ok && substituted.Equal(target) {
			return true
		}
	}
	return false
}

func (r *Result) callOutcomeHasConcreteStaticMemberFactAt(point cfg.Point, outcome callpayload.CallOutcome, target pathdom.Path) bool {
	site, ok := r.CallSiteView(point)
	if !ok {
		return false
	}
	bindings := r.callGuardCallBindings(site)
	for _, fact := range outcome.NormalReturnFacts.PathStaticMembers {
		if fact.Path.IsPlaceholder() {
			continue
		}
		substituted, ok := fact.Path.Substitute(bindings)
		if ok && substituted.Equal(target) {
			return true
		}
	}
	return false
}

func staticMemberWriteName(target pathdom.Path) (string, bool) {
	if len(target.Segments) == 0 {
		return "", false
	}
	last := target.Segments[len(target.Segments)-1]
	switch last.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return last.Name, last.Name != ""
	case segment.SegmentIndexInt:
		return segment.FormatSegments([]segment.Segment{last}), true
	default:
		return "", false
	}
}

func (r *Result) structuralMutationMayTarget(point cfg.Point, id identity.ID, receiver pathdom.Path) bool {
	if write, ok := r.facts.PathStaticMemberWrite(point); ok {
		if r.pathBeforeBoundaryHasIdentity(point, write.TargetPathRef().Parent(), id) {
			return true
		}
	}
	if write, ok := r.facts.DynamicIndexWrite(point); ok {
		if r.pathBeforeBoundaryHasIdentity(point, write.TablePathRef(), id) {
			return true
		}
	}
	if invalidation, ok := r.facts.PathDescendantInvalidation(point); ok {
		if r.pathBeforeBoundaryHasIdentity(point, invalidation.ContainerPathRef(), id) {
			return true
		}
		if tablePath, _, _, ok := invalidation.DynamicTargetRef(); ok && r.pathBeforeBoundaryHasIdentity(point, tablePath, id) {
			return true
		}
	}
	if r.callMayStructurallyMutateIdentity(point, id, receiver) {
		return true
	}
	return false
}

func (r *Result) callMayStructurallyMutateIdentity(point cfg.Point, id identity.ID, receiver pathdom.Path) bool {
	if r == nil || !r.callSiteExists(point) {
		return false
	}
	before, beforeOK := r.StateAt(point)
	after, afterOK := r.StateAtBoundary(point)
	if beforeOK && afterOK {
		domain := heapidentity.ObjectDomain(r.registry)
		if !domain.Equal(before.ReadHeapTableObject(r.registry, id), after.ReadHeapTableObject(r.registry, id)) {
			return true
		}
	}
	if r.opaqueCallMaySynchronouslyInvokeCapturedClosure(point) &&
		r.callArgumentGraphCapturesIdentity(point, id) {
		return true
	}
	if receiver.IsEmpty() {
		return false
	}
	return r.CallMayInvalidateTrackedPath(point, receiver)
}

// opaqueCallMaySynchronouslyInvokeCapturedClosure reports whether the call
// lacks an authoritative direct-call effect summary. Such a callee may invoke
// a closure argument before returning; a known local function or an explicit
// signature/effect summary keeps its existing precise behavior.
func (r *Result) opaqueCallMaySynchronouslyInvokeCapturedClosure(point cfg.Point) bool {
	if r == nil || !r.callSiteExists(point) {
		return false
	}
	outcome, hasOutcome := r.CallOutcomeAt(point)
	if !hasOutcome {
		return !r.callSiteHasExactEmptyGuardInvalidationSummaryAt(point)
	}
	if r.callOutcomeHasExactGuardInvalidationSummaryAt(point, outcome, true) &&
		!CallOutcomeHasGlobalGuardInvalidation(outcome) {
		return false
	}
	return true
}

// callArgumentGraphCapturesIdentity follows solved closure captures and finite
// heap members from every call argument. It is deliberately identity-based:
// when an opaque callee receives a closure (possibly nested in a table) that
// captures target, it may invoke that closure synchronously and structurally
// mutate target or any table reachable from that capture.
func (r *Result) callArgumentGraphCapturesIdentity(point cfg.Point, target identity.ID) bool {
	if r == nil || target == (identity.ID{}) {
		return false
	}
	site, ok := r.CallSiteView(point)
	if !ok {
		return false
	}
	visited := make(map[identity.ID]struct{})
	found := false
	site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
		value, ok := r.SourceValueBeforeBoundary(point, source)
		if !ok {
			return true
		}
		if r.valueGraphHasClosureCapturingIdentity(point, value, target, visited) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (r *Result) valueGraphHasClosureCapturingIdentity(point cfg.Point, value product.Value, target identity.ID, visited map[identity.ID]struct{}) bool {
	if r == nil || r.registry == nil {
		return false
	}
	id, ok := identityvalue.ExactID(r.registry, value)
	if !ok {
		return false
	}
	if _, seen := visited[id]; seen {
		return false
	}
	visited[id] = struct{}{}
	if r.closureCapturesIdentity(point, id, target, visited) {
		return true
	}
	st, ok := r.StateAt(point)
	if !ok {
		return false
	}
	object := st.ReadHeapTableObject(r.registry, id)
	for _, member := range object.StaticMembers() {
		if r.valueGraphHasClosureCapturingIdentity(point, member, target, visited) {
			return true
		}
	}
	for _, fact := range object.DynamicIndexFacts() {
		if r.valueGraphHasClosureCapturingIdentity(point, fact.Value, target, visited) {
			return true
		}
	}
	return false
}

// valueGraphReachesIdentity follows a closure's captured environment. Once a
// closure is known to be invoked, every table reachable from its captures can
// be mutated by its body, so target itself is a successful terminal here.
func (r *Result) valueGraphReachesIdentity(point cfg.Point, value product.Value, target identity.ID, visited map[identity.ID]struct{}) bool {
	if r == nil || r.registry == nil {
		return false
	}
	id, ok := identityvalue.ExactID(r.registry, value)
	if !ok {
		return false
	}
	if id == target {
		return true
	}
	if _, seen := visited[id]; seen {
		return false
	}
	visited[id] = struct{}{}
	if r.closureCapturesIdentity(point, id, target, visited) {
		return true
	}
	st, ok := r.StateAt(point)
	if !ok {
		return false
	}
	object := st.ReadHeapTableObject(r.registry, id)
	for _, member := range object.StaticMembers() {
		if r.valueGraphReachesIdentity(point, member, target, visited) {
			return true
		}
	}
	for _, fact := range object.DynamicIndexFacts() {
		if r.valueGraphReachesIdentity(point, fact.Value, target, visited) {
			return true
		}
	}
	return false
}

func (r *Result) closureCapturesIdentity(point cfg.Point, closure identity.ID, target identity.ID, visited map[identity.ID]struct{}) bool {
	if r == nil || r.wir == nil || r.Graph() == nil {
		return false
	}
	for _, candidate := range r.Graph().RPO() {
		if candidate != point && !r.PointDominates(candidate, point) {
			continue
		}
		for _, inst := range r.wir.PointInstructions(candidate) {
			if inst.Op != wir.OpClosure || inst.Func == 0 || inst.Dst.Kind != wir.OperandPath {
				continue
			}
			closurePath := r.wir.Path(wir.PathRef(inst.Dst.Ref))
			value, ok := r.PathValueAtBoundary(candidate, closurePath)
			if !ok {
				continue
			}
			id, ok := identityvalue.ExactID(r.registry, value)
			if !ok || id != closure {
				continue
			}
			for _, capture := range r.wir.Operands(inst.List) {
				if capture.Kind != wir.OperandPath {
					continue
				}
				capturePath := r.wir.Path(wir.PathRef(capture.Ref))
				if capturePath.Symbol == 0 {
					continue
				}
				captured, ok := r.PathValueBeforeBoundary(point, pathdom.NewPath(capturePath.Symbol, r.SymbolName(capturePath.Symbol)))
				if ok && r.valueGraphReachesIdentity(point, captured, target, visited) {
					return true
				}
			}
		}
	}
	return false
}

func (r *Result) callSiteReferencesIdentityAt(point cfg.Point, id identity.ID, receiver pathdom.Path) bool {
	if r == nil || id == (identity.ID{}) {
		return false
	}
	site, ok := r.CallSiteView(point)
	if !ok {
		return false
	}
	references := func(p pathdom.Path) bool {
		if p.IsEmpty() {
			return false
		}
		if !receiver.IsEmpty() && receiver.Overlaps(p) {
			return true
		}
		return r.pathBeforeBoundaryHasIdentity(point, p, id)
	}
	if receiverPath, ok := site.ReceiverPath(); ok && references(receiverPath) {
		return true
	}
	found := false
	site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
		argPath, ok := r.valueSourcePath(source)
		if ok && references(argPath) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (r *Result) pathBeforeBoundaryHasIdentity(point cfg.Point, p pathdom.Path, id identity.ID) bool {
	if p.IsEmpty() || id == (identity.ID{}) {
		return false
	}
	value, ok := r.PathValueBeforeBoundary(point, p)
	if !ok {
		return false
	}
	got, ok := identityvalue.ExactID(r.registry, value)
	return ok && got == id
}

func (r *Result) pathBeforeBoundaryHasStackLocalIdentity(point cfg.Point, p pathdom.Path, id identity.ID) bool {
	if !r.pathBeforeBoundaryHasIdentity(point, p, id) {
		return false
	}
	value, ok := r.PathValueBeforeBoundary(point, p)
	return ok && r.ValueHasStackLocalExactIdentity(point, value)
}
