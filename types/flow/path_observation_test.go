package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSelectPathObservationResult_StrictPreFallsBackToDeclared(t *testing.T) {
	declared := typ.String
	got := SelectPathObservationResult(PathObservationSelection{
		Query: PathObservationQuery{
			Point:      4,
			Path:       constraint.NewPath(10, "x"),
			View:       PathReadPre,
			StrictView: true,
		},
		Declared: declared,
	})

	if !got.Resolved() || got.Source != PathObservationDeclared || !typ.TypeEquals(got.Type, declared) {
		t.Fatalf("strict-pre fallback = %#v, want declared string", got)
	}
}

func TestPathObservationIndexReadBuildsReadbackQuery(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(41), "rows")
	key := constraint.NewPath(cfg.SymbolID(42), "id")
	index := PathObservationIndexRead{
		TablePath: table,
		KeyPath:   key,
		KeyType:   typ.String,
	}

	got, ok := index.ReadbackQuery(cfg.Point(7), PathReadPost)
	if !ok {
		t.Fatal("ReadbackQuery returned false")
	}
	if got.Point != cfg.Point(7) || got.View != PathReadPost {
		t.Fatalf("query point/view = %v/%v, want 7/post", got.Point, got.View)
	}
	target, ok := StableAddressOfPath(table)
	if !ok {
		t.Fatal("table address")
	}
	keyAddr, ok := StableAddressOfPath(key)
	if !ok {
		t.Fatal("key address")
	}
	if !got.Admission.Target.Equal(target) || !got.Admission.HasKeyPath || !got.Admission.KeyPath.Equal(keyAddr) {
		t.Fatalf("query admission = %#v, want table/key addresses", got.Admission)
	}
}

func TestSelectPathObservationResult_ProofOnlyReportsConditionProof(t *testing.T) {
	got := SelectPathObservationResult(PathObservationSelection{
		Query: PathObservationQuery{
			Point:               7,
			Path:                constraint.NewPath(11, "x"),
			AllowConditionProof: true,
		},
		Declared: typ.Any,
		Proof:    typ.String,
	})

	if !got.Resolved() || got.Source != PathObservationConditionProof || !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("proof-only observation = %#v, want condition-proof string", got)
	}
}

func TestSelectPathObservationResult_DirectTakesPrecedence(t *testing.T) {
	got := SelectPathObservationResult(PathObservationSelection{
		Query: PathObservationQuery{
			Point:               7,
			Path:                constraint.NewPath(11, "x"),
			AllowConditionProof: true,
		},
		Declared: typ.Any,
		Direct: PathObservationCandidate{
			Type:   typ.Number,
			Source: PathObservationDirectPath,
			OK:     true,
		},
		Solved: PathObservationCandidate{
			Type:   typ.String,
			Source: PathObservationSolvedFlow,
			OK:     true,
		},
		Proof: typ.Boolean,
	})

	if !got.Resolved() || got.Source != PathObservationDirectPath || !typ.TypeEquals(got.Type, typ.Number) {
		t.Fatalf("direct-precedence observation = %#v, want direct number", got)
	}
}

func TestSelectPathObservationResult_ProofRefinesDirectOptionalPath(t *testing.T) {
	got := SelectPathObservationResult(PathObservationSelection{
		Query: PathObservationQuery{
			Point:               7,
			Path:                constraint.NewPath(11, "event").Field("code"),
			AllowConditionProof: true,
		},
		Declared: typ.NewOptional(typ.Number),
		Direct: PathObservationCandidate{
			Type:   typ.NewOptional(typ.Number),
			Source: PathObservationDirectPath,
			OK:     true,
		},
		Solved: PathObservationCandidate{
			Type:   typ.NewOptional(typ.Number),
			Source: PathObservationFactProjection,
			OK:     true,
		},
		Proof: typ.Number,
	})

	if !got.Resolved() || got.Source != PathObservationConditionProof || !typ.TypeEquals(got.Type, typ.Number) {
		t.Fatalf("proof-refined direct observation = %#v, want condition-proof number", got)
	}
}

func TestSelectPathObservationResult_NormalizesProofSelectedSource(t *testing.T) {
	got := SelectPathObservationResult(PathObservationSelection{
		Query: PathObservationQuery{
			Point:               7,
			Path:                constraint.NewPath(11, "x"),
			AllowConditionProof: true,
		},
		Proof: typ.String,
	})

	if !got.Resolved() || got.Source != PathObservationConditionProof || !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("proof-selected source = %#v, want condition-proof string", got)
	}
}

func TestSelectPathObservationResult_NeverSolvedIsAuthoritative(t *testing.T) {
	got := SelectPathObservationResult(PathObservationSelection{
		Query: PathObservationQuery{
			Point: 8,
			Path:  constraint.NewPath(12, "x"),
			View:  PathReadCurrent,
		},
		Declared: typ.String,
		Solved: PathObservationCandidate{
			Type:   typ.Never,
			Source: PathObservationSolvedFlow,
			OK:     true,
		},
	})

	if !got.Resolved() || got.Source != PathObservationSolvedFlow || !typ.IsNever(got.Type) {
		t.Fatalf("never solved observation = %#v, want solved-flow never", got)
	}
}

func TestSelectPathObservationResult_KeepsNilRefinementOfOptionalDeclaredRead(t *testing.T) {
	declared := typ.NewOptional(typ.String)
	got := SelectPathObservationResult(PathObservationSelection{
		Query: PathObservationQuery{
			Point: 9,
			Path:  constraint.NewPath(13, "x"),
		},
		Declared: declared,
		Solved: PathObservationCandidate{
			Type:   typ.Nil,
			Source: PathObservationFactProjection,
			OK:     true,
		},
		AdmitSelected: true,
	})

	if !got.Resolved() || got.Source != PathObservationFactProjection || !typ.TypeEquals(got.Type, typ.Nil) {
		t.Fatalf("nil optional observation = %#v, want nil", got)
	}
}

func TestSelectPathObservationResult_AdmitsSelectedWhenRequested(t *testing.T) {
	got := SelectPathObservationResult(PathObservationSelection{
		Query: PathObservationQuery{
			Point: 10,
			Path:  constraint.NewPath(14, "x"),
		},
		Solved: PathObservationCandidate{
			Type:   typ.LiteralString("ready"),
			Source: PathObservationFactProjection,
			OK:     true,
		},
		AdmitSelected: true,
	})

	if !got.Resolved() || !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("admitted observation = %#v, want string", got)
	}
}

func TestSelectAssignmentSourceObservation_PathRefinesStoredAny(t *testing.T) {
	point := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	got := SelectAssignmentSourceObservation(AssignmentSourceObservationSelection{
		Stored: typ.Any,
		Path:   point,
	})

	if !typ.TypeEquals(got, point) {
		t.Fatalf("assignment source selection = %v, want precise path observation", got)
	}
}

func TestSelectAssignmentSourceObservation_KeepsStoredWhenPathIsBroad(t *testing.T) {
	point := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	got := SelectAssignmentSourceObservation(AssignmentSourceObservationSelection{
		Stored: point,
		Path:   typ.Any,
	})

	if !typ.TypeEquals(got, point) {
		t.Fatalf("assignment source selection = %v, want stored source", got)
	}
}
