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
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// StableShapeFact is the checker-facing proof that a receiver has a final,
// closed record shape at Point. It is deliberately absent when the analysis can
// prove only the current field value but not the table's future structural
// stability.
type StableShapeFact struct {
	Point    cfg.Point
	Identity identity.ID
	Shape    typ.Type
	Fields   []StableShapeField
}

type StableShapeField struct {
	Name string
	Type typ.Type
}

// StableShapeForStaticMemberRead returns the final-shape proof for a static
// field read's receiver, when the proof is strong enough for fixed-offset reads.
func (r *Result) StableShapeForStaticMemberRead(occ StaticMemberReadOccurrence) (StableShapeFact, bool) {
	if occ.HasReceiverValueBeforeBoundary {
		receiver := pathdom.Path{}
		if occ.HasReceiverPath {
			receiver = occ.ReceiverPath
		}
		return r.stableShapeForValue(occ.Point, occ.ReceiverValueBeforeBoundary, receiver, false)
	}
	if occ.HasReceiverValueAtBoundary {
		receiver := pathdom.Path{}
		if occ.HasReceiverPath {
			receiver = occ.ReceiverPath
		}
		return r.stableShapeForValue(occ.Point, occ.ReceiverValueAtBoundary, receiver, true)
	}
	return StableShapeFact{}, false
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
	_, stable := r.stableShapeForValue(point, value, p, false)
	return stable
}

// ValueHasStableShapeBeforeBoundary is the value-only companion to
// SourceHasStableShapeBeforeBoundary.
func (r *Result) ValueHasStableShapeBeforeBoundary(point cfg.Point, value product.Value) bool {
	_, stable := r.stableShapeForValue(point, value, pathdom.Path{}, false)
	return stable
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
	if !r.stableShapeOriginProven(st, id, object) {
		return StableShapeFact{}, false
	}
	shape, fields, ok := r.closedStableShapeFromObject(object)
	if !ok {
		return StableShapeFact{}, false
	}
	if r.priorStructuralWriteNotDominating(point, id, receiver) {
		return StableShapeFact{}, false
	}
	if r.futureStructuralMutationReachable(point, id, receiver) {
		return StableShapeFact{}, false
	}
	return StableShapeFact{
		Point:    point,
		Identity: id,
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
		return true
	case placement.OwnedHeap:
		return object.StableShape() || st.IsTableFrozen(id)
	default:
		return object.StableShape() && st.IsTableFrozen(id)
	}
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
	ks := r.KeySpace()
	if ks == nil {
		return nil, nil, false
	}
	fieldTypes := make(map[string]typ.Type)
	intTypes := make(map[int]typ.Type)
	for key, value := range object.StaticMembers() {
		if product.Equal(r.registry, value, product.Bottom(r.registry)) {
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
	return next
}

func (r *Result) priorStructuralWriteNotDominating(point cfg.Point, id identity.ID, receiver pathdom.Path) bool {
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
		if !r.PointDominates(candidate, point) {
			return true
		}
	}
	return false
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
	if receiver.IsEmpty() {
		return false
	}
	return r.CallMayInvalidateTrackedPath(point, receiver)
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
