package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

type fakeAssignmentSourceFlow struct {
	narrowed map[constraint.PathKey]typ.Type
	pre      map[constraint.PathKey]typ.Type
	lengths  map[constraint.PathKey]int64
	keyOf    map[string]bool
}

func (f fakeAssignmentSourceFlow) NarrowedTypeAt(_ cfg.Point, path constraint.Path) typ.Type {
	return f.narrowed[path.Key()]
}

func (f fakeAssignmentSourceFlow) PreStateTypeAt(_ cfg.Point, path constraint.Path) typ.Type {
	return f.pre[path.Key()]
}

func (f fakeAssignmentSourceFlow) NumericBoundsAt(cfg.Point, cfg.SymbolID) (int64, int64, bool) {
	return 0, 0, false
}

func (f fakeAssignmentSourceFlow) ArrayLenRefPathAt(cfg.Point, cfg.SymbolID) (constraint.Path, int64, bool) {
	return constraint.Path{}, 0, false
}

func (f fakeAssignmentSourceFlow) LengthBoundsAt(_ cfg.Point, path constraint.Path) (int64, int64, bool) {
	lower, ok := f.lengths[path.Key()]
	return lower, lower, ok
}

func (f fakeAssignmentSourceFlow) HasKeyOf(_ cfg.Point, tablePath, keyPath constraint.Path) bool {
	return f.keyOf[string(tablePath.Key())+"->"+string(keyPath.Key())]
}

func (f fakeAssignmentSourceFlow) IndexReadback(IndexWriteReadQuery) (typ.Type, bool) {
	return nil, false
}

func TestAssignmentSourceValuePathSourceUsesPreStateForSelfRelatedAssignment(t *testing.T) {
	target := constraint.NewPath(1, "x")
	source := target.Field("kind")
	got := AssignmentSourceValue(AssignmentSourceQuery{
		Point:  10,
		Target: target,
		Source: AssignmentSource{
			Kind: AssignmentSourcePath,
			Path: source,
		},
		Flow: fakeAssignmentSourceFlow{
			narrowed: map[constraint.PathKey]typ.Type{source.Key(): typ.Number},
			pre:      map[constraint.PathKey]typ.Type{source.Key(): typ.String},
		},
	})
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("AssignmentSourceValue(self-related path) = %v, want pre-state string", got)
	}
}

func TestAssignmentSourceValueMapElementUsesIndexReadPresenceProof(t *testing.T) {
	table := constraint.NewPath(1, "suites")
	key := constraint.NewPath(2, "name")
	got := AssignmentSourceValue(AssignmentSourceQuery{
		Point:  10,
		Target: constraint.NewPath(3, "suite"),
		Source: AssignmentSource{
			Kind:      AssignmentSourceMapElement,
			MapPath:   table,
			KeySymbol: key.Symbol,
			KeyVar:    key.Root,
		},
		Flow: fakeAssignmentSourceFlow{
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
		t.Fatalf("AssignmentSourceValue(map element with key presence) = %v, want number", got)
	}
}

func TestAssignmentSourceValueMapElementNormalizesOptionalKeyBeforePresenceProof(t *testing.T) {
	table := constraint.NewPath(1, "suites")
	key := constraint.NewPath(2, "name")
	got := AssignmentSourceValue(AssignmentSourceQuery{
		Point:  10,
		Target: constraint.NewPath(3, "suite"),
		Source: AssignmentSource{
			Kind:      AssignmentSourceMapElement,
			MapPath:   table,
			KeySymbol: key.Symbol,
			KeyVar:    key.Root,
		},
		Flow: fakeAssignmentSourceFlow{
			narrowed: map[constraint.PathKey]typ.Type{
				table.Key(): typ.NewMap(typ.String, typ.Number),
				key.Key():   typ.NewOptional(typ.LiteralString("stable")),
			},
			keyOf: map[string]bool{
				string(table.Key()) + "->" + string(key.Key()): true,
			},
		},
	})
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("AssignmentSourceValue(map element with optional key presence) = %v, want number", got)
	}
}

func TestAssignmentSourceValueLengthIndexUsesIndexReadLengthProof(t *testing.T) {
	items := constraint.NewPath(1, "items")
	got := AssignmentSourceValue(AssignmentSourceQuery{
		Point:  10,
		Target: constraint.NewPath(2, "last"),
		Static: typ.Nil,
		Source: AssignmentSource{
			Kind:          AssignmentSourceLengthIndex,
			ContainerPath: items,
		},
		Flow: fakeAssignmentSourceFlow{
			narrowed: map[constraint.PathKey]typ.Type{
				items.Key(): typ.NewArray(typ.String),
			},
			lengths: map[constraint.PathKey]int64{
				items.Key(): 1,
			},
		},
	})
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("AssignmentSourceValue(length index) = %v, want string", got)
	}
}
