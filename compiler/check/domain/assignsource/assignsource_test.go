package assignsource

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type fakeFlow struct {
	narrowed map[constraint.PathKey]typ.Type
	pre      map[constraint.PathKey]typ.Type
	lengths  map[constraint.PathKey]int64
	keyOf    map[string]bool
}

func (f fakeFlow) NarrowedTypeAt(_ cfg.Point, path constraint.Path) typ.Type {
	return f.narrowed[path.Key()]
}

func (f fakeFlow) PreStateTypeAt(_ cfg.Point, path constraint.Path) typ.Type {
	return f.pre[path.Key()]
}

func (f fakeFlow) LengthBoundsAt(_ cfg.Point, path constraint.Path) (int64, int64, bool) {
	lower, ok := f.lengths[path.Key()]
	return lower, lower, ok
}

func (f fakeFlow) HasKeyOf(_ cfg.Point, tablePath, keyPath constraint.Path) bool {
	return f.keyOf[string(tablePath.Key())+"->"+string(keyPath.Key())]
}

func TestValuePathSourceUsesPreStateForSelfRelatedAssignment(t *testing.T) {
	target := constraint.NewPath(1, "x")
	source := target.Field("kind")
	got := Value(Query{
		Point:  10,
		Target: target,
		Source: flow.AssignmentSource{
			Kind: flow.AssignmentSourcePath,
			Path: source,
		},
		Flow: fakeFlow{
			narrowed: map[constraint.PathKey]typ.Type{source.Key(): typ.Number},
			pre:      map[constraint.PathKey]typ.Type{source.Key(): typ.String},
		},
	})
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("Value(self-related path) = %v, want pre-state string", got)
	}
}

func TestValueMapElementUsesKeyPresenceProof(t *testing.T) {
	table := constraint.NewPath(1, "suites")
	key := constraint.NewPath(2, "name")
	got := Value(Query{
		Point:  10,
		Target: constraint.NewPath(3, "suite"),
		Source: flow.AssignmentSource{
			Kind:      flow.AssignmentSourceMapElement,
			MapPath:   table,
			KeySymbol: key.Symbol,
			KeyVar:    key.Root,
		},
		Flow: fakeFlow{
			narrowed: map[constraint.PathKey]typ.Type{
				table.Key(): typ.NewMap(typ.String, typ.Number),
				key.Key():   typ.String,
			},
			keyOf: map[string]bool{
				string(table.Key()) + "->" + string(key.Key()): true,
			},
		},
	})
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("Value(map element with key presence) = %v, want number", got)
	}
}

func TestValueLengthIndexUsesLengthProof(t *testing.T) {
	items := constraint.NewPath(1, "items")
	got := Value(Query{
		Point:  10,
		Target: constraint.NewPath(2, "last"),
		Static: typ.Nil,
		Source: flow.AssignmentSource{
			Kind:          flow.AssignmentSourceLengthIndex,
			ContainerPath: items,
		},
		Flow: fakeFlow{
			narrowed: map[constraint.PathKey]typ.Type{
				items.Key(): typ.NewArray(typ.String),
			},
			lengths: map[constraint.PathKey]int64{
				items.Key(): 1,
			},
		},
	})
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("Value(length index) = %v, want string", got)
	}
}
