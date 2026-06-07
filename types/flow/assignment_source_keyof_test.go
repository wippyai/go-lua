package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestTruthyAssignmentValueKeyOfConstraintsUsesAssignmentSourceEvidence(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(10), "rows")
	key := constraint.NewPath(cfg.SymbolID(11), "id")
	value := constraint.NewPath(cfg.SymbolID(12), "row")

	got := TruthyAssignmentValueKeyOfConstraints(TruthyAssignmentValueKeyOfQuery{
		ValuePath: value,
		Assignments: []UnifiedAssignment{{
			TargetPath: value,
			Source: AssignmentSource{
				Kind:      AssignmentSourceMapElement,
				MapPath:   table,
				KeySymbol: key.Symbol,
				KeyVar:    key.Root,
			},
		}},
	})
	if len(got) != 1 {
		t.Fatalf("KeyOf constraints = %#v, want one", got)
	}
	keyOf, ok := got[0].(constraint.KeyOf)
	if !ok || !keyOf.Table.Equal(table) || !keyOf.Key.Equal(key) {
		t.Fatalf("KeyOf constraint = %#v, want table/key", got[0])
	}
}

func TestTruthyAssignmentValueKeyOfConstraintsVersionsTargetTableAndKey(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(20), "rows")
	key := constraint.NewPath(cfg.SymbolID(21), "id")
	value := constraint.NewPath(cfg.SymbolID(22), "row")
	version := func(path constraint.Path) constraint.Path {
		path.Version = 7
		return path
	}
	versionedValue := version(value)

	got := TruthyAssignmentValueKeyOfConstraints(TruthyAssignmentValueKeyOfQuery{
		ValuePath:   versionedValue,
		VersionPath: version,
		Assignments: []UnifiedAssignment{{
			TargetPath: value,
			Source: AssignmentSource{
				Kind:      AssignmentSourceMapElement,
				MapPath:   table,
				KeySymbol: key.Symbol,
				KeyVar:    key.Root,
			},
		}},
	})
	if len(got) != 1 {
		t.Fatalf("KeyOf constraints = %#v, want one", got)
	}
	keyOf, ok := got[0].(constraint.KeyOf)
	if !ok {
		t.Fatalf("constraint = %#v, want KeyOf", got[0])
	}
	if keyOf.Table.Version != 7 || keyOf.Key.Version != 7 {
		t.Fatalf("KeyOf versions = table %v key %v, want 7/7", keyOf.Table.Version, keyOf.Key.Version)
	}
}

func TestTruthyAssignmentValueKeyOfConstraintsIgnoresNonMatchingSource(t *testing.T) {
	value := constraint.NewPath(cfg.SymbolID(30), "row")
	got := TruthyAssignmentValueKeyOfConstraints(TruthyAssignmentValueKeyOfQuery{
		ValuePath: value,
		Assignments: []UnifiedAssignment{{
			TargetPath: value,
			Source:     AssignmentSource{Kind: AssignmentSourcePath, Path: constraint.NewPath(cfg.SymbolID(31), "other")},
		}},
	})
	if len(got) != 0 {
		t.Fatalf("KeyOf constraints = %#v, want none", got)
	}
}
