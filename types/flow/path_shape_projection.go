package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// PathShapeProjection is the producer-neutral view needed to overlay solved
// path observations onto a declared root shape. Direct path reads use the same
// observation law as diagnostics; child reads enumerate only finite facts already
// materialized below the path.
type PathShapeProjection struct {
	Paths    PathObservationFacts
	Children PathChildFacts
	View     PathReadView
}

// ProjectObservedPathShape refines declared with direct and finite child path
// observations at path. It is deliberately bounded: recursive record products are
// only opened along materialized child facts, never by deriving an unbounded
// descendant tree from the declared type itself.
func ProjectObservedPathShape(
	point cfg.Point,
	path constraint.Path,
	declared typ.Type,
	projection PathShapeProjection,
) typ.Type {
	return projectObservedPathShape(point, path, declared, projection, nil, true)
}

type pathShapeProjectionKey struct {
	t    typ.Type
	path constraint.PathKey
}

func projectObservedPathShape(
	point cfg.Point,
	path constraint.Path,
	base typ.Type,
	projection PathShapeProjection,
	memo map[pathShapeProjectionKey]typ.Type,
	allowDirect bool,
) typ.Type {
	if typ.IsAbsentOrUnknown(base) || path.IsEmpty() {
		return base
	}
	if allowDirect {
		if direct := projectionPathObservationType(point, path, projection); !typ.IsAbsentOrUnknown(direct) {
			if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(direct, base); ok && reconciled != nil {
				base = reconciled
			} else {
				base = direct
			}
		}
	}
	if !canProjectObservedChildren(base) {
		return base
	}
	key := pathShapeProjectionKey{t: base, path: path.Key()}
	if memo != nil {
		if cached, ok := memo[key]; ok {
			return cached
		}
	} else {
		memo = make(map[pathShapeProjectionKey]typ.Type, 4)
	}
	memo[key] = base

	var out typ.Type
	switch t := base.(type) {
	case *typ.Alias:
		target := projectObservedPathShape(point, path, t.Target, projection, memo, false)
		if target == nil || typ.TypeEquals(target, t.Target) {
			out = base
			break
		}
		out = typ.NewAlias(t.Name, target)
	case *typ.Optional:
		inner := projectObservedPathShape(point, path, t.Inner, projection, memo, false)
		if inner == nil || typ.TypeEquals(inner, t.Inner) {
			out = base
			break
		}
		out = typ.NewOptional(inner)
	case *typ.Union:
		members := make([]typ.Type, 0, len(t.Members))
		changed := false
		for _, member := range t.Members {
			projected := projectObservedPathShape(point, path, member, projection, memo, false)
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
		out = projectRecordPathShape(point, path, t, projection, memo)
	default:
		out = base
	}
	if out == nil {
		out = base
	}
	memo[key] = out
	return out
}

func canProjectObservedChildren(t typ.Type) bool {
	switch t.(type) {
	case *typ.Alias, *typ.Optional, *typ.Union, *typ.Record:
		return true
	default:
		return false
	}
}

func projectRecordPathShape(
	point cfg.Point,
	path constraint.Path,
	rec *typ.Record,
	projection PathShapeProjection,
	memo map[pathShapeProjectionKey]typ.Type,
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
	for _, child := range projectionChildPathFacts(point, path, projection) {
		seg, ok := directChildSegment(path, child.Path)
		if !ok || seg.Kind != constraint.SegmentField || typ.IsAbsentOrUnknown(child.Type) {
			continue
		}
		projected := projectObservedPathShape(point, child.Path, child.Type, projection, memo, false)
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
	if len(fields) == 0 || !changed {
		return rec
	}
	return rebuildRecordWithPathShapeFields(rec, fields)
}

func projectionChildPathFacts(point cfg.Point, path constraint.Path, projection PathShapeProjection) []PathFact {
	if projection.Children == nil {
		return nil
	}
	return projection.Children.ObserveChildPaths(PathChildQuery{
		Point: point,
		Path:  path,
		View:  projection.View,
	})
}

func projectionPathObservationType(point cfg.Point, path constraint.Path, projection PathShapeProjection) typ.Type {
	if projection.Paths == nil {
		return nil
	}
	obs := projection.Paths.ObservePath(PathObservationQuery{
		Point:               point,
		Path:                path,
		View:                projection.View,
		AllowConditionProof: true,
		PreserveProof:       true,
	})
	if !obs.Resolved() {
		return nil
	}
	return obs.Type
}

func directChildSegment(parent, child constraint.Path) (constraint.Segment, bool) {
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

func rebuildRecordWithPathShapeFields(rec *typ.Record, fields []typ.Field) typ.Type {
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
