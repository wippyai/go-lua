package captured

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// PathProjection is the captured environment view of parent flow state. It is
// intentionally expressed in producer-neutral fact surfaces, not in a concrete
// solver carrier: path reads use the normalized observation law, and child reads
// enumerate only finite facts already materialized below a path.
type PathProjection struct {
	Paths    flow.PathObservationFacts
	Children flow.PathChildFacts
}

// FromParentFactsAtPoint computes captured types for a nested graph from parent
// facts at the point where the closure environment is observed.
func FromParentFactsAtPoint(
	parentFacts flow.TypeFacts,
	childGraph *cfg.Graph,
	point cfg.Point,
	bindingsOverride *bind.BindingTable,
	projection PathProjection,
) map[cfg.SymbolID]typ.Type {
	if parentFacts == nil || childGraph == nil || point == 0 {
		return nil
	}
	bindings := bindingsOverride
	if bindings == nil {
		bindings = childGraph.Bindings()
	}
	if bindings == nil {
		return nil
	}
	fn := childGraph.Func()
	if fn == nil {
		return nil
	}
	capturedSyms := bindings.CapturedSymbols(fn)
	if len(capturedSyms) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]typ.Type, len(capturedSyms))
	for _, sym := range capturedSyms {
		if sym == 0 {
			continue
		}
		if t := capturedTypeAtPoint(parentFacts, point, sym, projection); !typ.IsAbsentOrUnknown(t) {
			out[sym] = t
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func capturedTypeAtPoint(
	parentFacts flow.TypeFacts,
	point cfg.Point,
	sym cfg.SymbolID,
	projection PathProjection,
) typ.Type {
	if parentFacts == nil || point == 0 || sym == 0 {
		return nil
	}
	tv := parentFacts.EffectiveTypeAt(point, sym)
	if tv.State != flow.StateResolved || typ.IsAbsentOrUnknown(tv.Type) {
		return nil
	}
	root := constraint.Path{Symbol: sym}
	return projectPathFacts(point, root, tv.Type, projection, nil, true)
}

type projectionKey struct {
	t    typ.Type
	path constraint.PathKey
}

func projectPathFacts(
	point cfg.Point,
	path constraint.Path,
	base typ.Type,
	projection PathProjection,
	memo map[projectionKey]typ.Type,
	allowDirect bool,
) typ.Type {
	if typ.IsAbsentOrUnknown(base) || path.IsEmpty() {
		return base
	}
	if allowDirect {
		if direct := projectionPathType(point, path, projection); !typ.IsAbsentOrUnknown(direct) {
			if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(direct, base); ok && reconciled != nil {
				base = reconciled
			} else {
				base = direct
			}
		}
	}
	if !canProjectChildren(base) {
		return base
	}
	key := projectionKey{t: base, path: path.Key()}
	if memo != nil {
		if cached, ok := memo[key]; ok {
			return cached
		}
	} else {
		memo = make(map[projectionKey]typ.Type, 4)
	}
	memo[key] = base

	var out typ.Type
	switch t := base.(type) {
	case *typ.Alias:
		target := projectPathFacts(point, path, t.Target, projection, memo, false)
		if target == nil || typ.TypeEquals(target, t.Target) {
			out = base
			break
		}
		out = typ.NewAlias(t.Name, target)
	case *typ.Optional:
		inner := projectPathFacts(point, path, t.Inner, projection, memo, false)
		if inner == nil || typ.TypeEquals(inner, t.Inner) {
			out = base
			break
		}
		out = typ.NewOptional(inner)
	case *typ.Union:
		members := make([]typ.Type, 0, len(t.Members))
		changed := false
		for _, member := range t.Members {
			projected := projectPathFacts(point, path, member, projection, memo, false)
			if projected == nil {
				projected = member
			}
			if !typ.TypeEquals(projected, member) {
				changed = true
			}
			members = append(members, projected)
		}
		if !changed {
			out = base
			break
		}
		out = join.Types(members...)
	case *typ.Record:
		out = projectRecordPathFacts(point, path, t, projection, memo)
	default:
		out = base
	}
	if out == nil {
		out = base
	}
	memo[key] = out
	return out
}

func canProjectChildren(t typ.Type) bool {
	switch t.(type) {
	case *typ.Alias, *typ.Optional, *typ.Union, *typ.Record:
		return true
	default:
		return false
	}
}

func projectRecordPathFacts(
	point cfg.Point,
	path constraint.Path,
	rec *typ.Record,
	projection PathProjection,
	memo map[projectionKey]typ.Type,
) typ.Type {
	if rec == nil {
		return rec
	}
	fields := make([]typ.Field, len(rec.Fields))
	copy(fields, rec.Fields)
	changed := false
	fieldIndex := make(map[string]int, len(fields))
	for i := range fields {
		fieldIndex[fields[i].Name] = i
	}
	for _, child := range projectionChildFacts(point, path, projection) {
		seg, ok := childDirectSegment(path, child.Path)
		if !ok || seg.Kind != constraint.SegmentField || typ.IsAbsentOrUnknown(child.Type) {
			continue
		}
		projected := projectPathFacts(point, child.Path, child.Type, projection, memo, false)
		if projected == nil {
			continue
		}
		if i, ok := fieldIndex[seg.Name]; ok {
			if !typ.TypeEquals(projected, fields[i].Type) {
				fields[i].Type = projected
				changed = true
			}
			continue
		}
		fields = append(fields, typ.Field{Name: seg.Name, Type: projected})
		fieldIndex[seg.Name] = len(fields) - 1
		changed = true
	}
	if len(fields) == 0 {
		return rec
	}
	if !changed {
		return rec
	}
	return rebuildRecordWithFields(rec, fields)
}

func projectionChildFacts(point cfg.Point, path constraint.Path, projection PathProjection) []flow.PathFact {
	if projection.Children == nil {
		return nil
	}
	return projection.Children.ObserveChildPaths(flow.PathChildQuery{
		Point: point,
		Path:  path,
		View:  flow.PathReadCurrent,
	})
}

func projectionPathType(point cfg.Point, path constraint.Path, projection PathProjection) typ.Type {
	if projection.Paths == nil {
		return nil
	}
	obs := projection.Paths.ObservePath(flow.PathObservationQuery{
		Point:               point,
		Path:                path,
		View:                flow.PathReadCurrent,
		AllowConditionProof: true,
		PreserveProof:       true,
	})
	if !obs.Resolved() {
		return nil
	}
	return obs.Type
}

func childDirectSegment(parent, child constraint.Path) (constraint.Segment, bool) {
	if parent.Symbol != child.Symbol {
		return constraint.Segment{}, false
	}
	if parent.Version != 0 && child.Version != 0 && parent.Version != child.Version {
		return constraint.Segment{}, false
	}
	if parent.Symbol == 0 && parent.Root != child.Root {
		return constraint.Segment{}, false
	}
	if len(child.Segments) != len(parent.Segments)+1 {
		return constraint.Segment{}, false
	}
	for i := range parent.Segments {
		if parent.Segments[i] != child.Segments[i] {
			return constraint.Segment{}, false
		}
	}
	return child.Segments[len(parent.Segments)], true
}

func rebuildRecordWithFields(rec *typ.Record, fields []typ.Field) typ.Type {
	if rec == nil {
		return rec
	}
	builder := typ.NewRecord().SetOpen(rec.Open)
	for _, field := range fields {
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
			builder.Field(field.Name, field.Type)
		}
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	return builder.Build()
}

// MergeCapturedTypes merges captured types into declared types as hints.
func MergeCapturedTypes(declared flow.DeclaredTypes, captured map[cfg.SymbolID]typ.Type) flow.DeclaredTypes {
	if len(captured) == 0 {
		return declared
	}
	if declared == nil {
		declared = make(flow.DeclaredTypes, len(captured))
	}
	for _, sym := range cfg.SortedSymbolIDs(captured) {
		t := captured[sym]
		if sym == 0 || t == nil {
			continue
		}
		if prev := declared[sym]; prev != nil {
			declared[sym] = value.JoinPrecise(prev, t)
		} else {
			declared[sym] = t
		}
	}
	return declared
}
