package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProjectObservedPathShapeOverlaysFiniteChildFact(t *testing.T) {
	const point cfg.Point = 11
	root := constraint.NewPath(7, "node")
	base := typ.NewRecord().
		Field("meta", typ.NewRecord().Build()).
		Field("name", typ.String).
		Build()
	child := typ.NewRecord().Field("id", typ.String).Build()

	got := ProjectObservedPathShape(point, root, base, PathShapeProjection{
		Children: pathShapeProjectionSurface{
			childTypes: func(p cfg.Point, path constraint.Path) []PathFact {
				if p == point && path.Equal(root) {
					return []PathFact{{Path: root.Field("meta"), Type: child}}
				}
				return nil
			},
		},
	})

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("ProjectObservedPathShape = %T, want record", got)
	}
	if meta := rec.GetField("meta"); meta == nil || !typ.TypeEquals(meta.Type, child) {
		t.Fatalf("meta = %v, want %v", meta, child)
	}
	if name := rec.GetField("name"); name == nil || !typ.TypeEquals(name.Type, typ.String) {
		t.Fatalf("name = %v, want string", name)
	}
}

type pathShapeProjectionSurface struct {
	pathType   func(cfg.Point, constraint.Path) typ.Type
	childTypes func(cfg.Point, constraint.Path) []PathFact
}

func (p pathShapeProjectionSurface) ObservePath(q PathObservationQuery) PathObservation {
	if p.pathType == nil {
		return PathObservation{}
	}
	t := p.pathType(q.Point, q.Path)
	if typ.IsAbsentOrUnknown(t) {
		return PathObservation{}
	}
	return PathObservation{
		Type:   t,
		State:  StateResolved,
		Source: PathObservationFactProjection,
	}
}

func (p pathShapeProjectionSurface) ObserveChildPaths(q PathChildQuery) []PathFact {
	if p.childTypes == nil {
		return nil
	}
	return p.childTypes(q.Point, q.Path)
}
