package propagate

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// TestKill_PerConstraintKind_PerAssignmentShape verifies, for every
// Constraint kind crossed with every assignment shape (root / subfield /
// static-index), that KillRedefinedConditions kills exactly the literals
// whose SemanticAffectedPaths intersect (via prefix in either direction)
// the assignment path.
//
// DOMAIN_DESIGN.md §8.2 + §10.3.
func TestKill_PerConstraintKind_PerAssignmentShape(t *testing.T) {
	const symX cfg.SymbolID = 1
	const symY cfg.SymbolID = 2

	x := constraint.Path{Root: "x", Symbol: symX}
	y := constraint.Path{Root: "y", Symbol: symY}

	strLit := typ.LiteralString("v")
	strKey := typ.LiteralString("k")
	intKey := typ.LiteralInt(7)
	stringTypeKey := narrow.BuiltinTypeKey("string")

	fieldSeg := constraint.Segment{Kind: constraint.SegmentField, Name: "kind"}
	otherFieldSeg := constraint.Segment{Kind: constraint.SegmentField, Name: "other"}
	idxStrSeg := constraint.Segment{Kind: constraint.SegmentIndexString, Name: "k"}
	idxIntSeg := constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 7}

	// Each test case: a constraint and the set of assignment shapes that
	// MUST kill it. Every other assignment shape (over the canonical set
	// below) must NOT kill it.
	type killCase struct {
		name    string
		c       constraint.Constraint
		kills   []Assignment
		nokills []Assignment
	}

	// Canonical assignment shapes used across cases.
	rootX := Assignment{Point: 0, TargetSym: symX}
	rootY := Assignment{Point: 0, TargetSym: symY}
	xKind := Assignment{Point: 0, TargetSym: symX, TargetSegs: []constraint.Segment{fieldSeg}}
	xOther := Assignment{Point: 0, TargetSym: symX, TargetSegs: []constraint.Segment{otherFieldSeg}}
	xIdxK := Assignment{Point: 0, TargetSym: symX, TargetSegs: []constraint.Segment{idxStrSeg}}
	xIdx7 := Assignment{Point: 0, TargetSym: symX, TargetSegs: []constraint.Segment{idxIntSeg}}

	// Precise + sound kill rule: assignment to w kills literal L iff w is at
	// or above some SemanticAffectedPath(L) (w's segs is a prefix of the
	// read path's segs). Subpath writes that do NOT shadow the read path —
	// e.g., x.value = … when L reads x.kind — do NOT kill.
	cases := []killCase{
		{
			name:    "Truthy(x)",
			c:       constraint.Truthy{Path: x},
			kills:   []Assignment{rootX},
			nokills: []Assignment{rootY, xKind, xOther, xIdxK, xIdx7},
		},
		{
			name:    "Falsy(x)",
			c:       constraint.Falsy{Path: x},
			kills:   []Assignment{rootX},
			nokills: []Assignment{rootY, xKind, xOther, xIdxK, xIdx7},
		},
		{
			name:    "IsNil(x)",
			c:       constraint.IsNil{Path: x},
			kills:   []Assignment{rootX},
			nokills: []Assignment{rootY, xKind, xOther, xIdxK, xIdx7},
		},
		{
			name:    "NotNil(x)",
			c:       constraint.NotNil{Path: x},
			kills:   []Assignment{rootX},
			nokills: []Assignment{rootY, xKind, xOther, xIdxK, xIdx7},
		},
		{
			name:    "HasType(x,string)",
			c:       constraint.HasType{Path: x, Type: stringTypeKey},
			kills:   []Assignment{rootX},
			nokills: []Assignment{rootY, xKind, xOther, xIdxK, xIdx7},
		},
		{
			name:    "NotHasType(x,string)",
			c:       constraint.NotHasType{Path: x, Type: stringTypeKey},
			kills:   []Assignment{rootX},
			nokills: []Assignment{rootY, xKind, xOther, xIdxK, xIdx7},
		},
		{
			name:    "HasField(x,kind)",
			c:       constraint.HasField{Path: x, Field: "kind"},
			kills:   []Assignment{rootX, xKind},
			nokills: []Assignment{rootY, xOther, xIdxK, xIdx7},
		},
		{
			name:    "FieldEquals(x.kind, lit)",
			c:       constraint.FieldEquals{Target: x, Field: "kind", Value: strLit},
			kills:   []Assignment{rootX, xKind},
			nokills: []Assignment{rootY, xOther, xIdxK, xIdx7},
		},
		{
			name:    "FieldNotEquals(x.kind, lit)",
			c:       constraint.FieldNotEquals{Target: x, Field: "kind", Value: strLit},
			kills:   []Assignment{rootX, xKind},
			nokills: []Assignment{rootY, xOther, xIdxK, xIdx7},
		},
		{
			name:    "FieldEqualsPath(x.kind, y)",
			c:       constraint.FieldEqualsPath{Target: x, Field: "kind", Value: y},
			kills:   []Assignment{rootX, xKind, rootY},
			nokills: []Assignment{xOther, xIdxK, xIdx7},
		},
		{
			name:    "FieldNotEqualsPath(x.kind, y)",
			c:       constraint.FieldNotEqualsPath{Target: x, Field: "kind", Value: y},
			kills:   []Assignment{rootX, xKind, rootY},
			nokills: []Assignment{xOther, xIdxK, xIdx7},
		},
		{
			name:    "IndexEquals(x[\"k\"], lit)",
			c:       constraint.IndexEquals{Target: x, Key: strKey, Value: strLit},
			kills:   []Assignment{rootX, xIdxK},
			nokills: []Assignment{rootY, xKind, xOther, xIdx7},
		},
		{
			name:    "IndexNotEquals(x[\"k\"], lit)",
			c:       constraint.IndexNotEquals{Target: x, Key: strKey, Value: strLit},
			kills:   []Assignment{rootX, xIdxK},
			nokills: []Assignment{rootY, xKind, xOther, xIdx7},
		},
		{
			name:    "IndexEquals(x[7], lit)",
			c:       constraint.IndexEquals{Target: x, Key: intKey, Value: strLit},
			kills:   []Assignment{rootX, xIdx7},
			nokills: []Assignment{rootY, xKind, xOther, xIdxK},
		},
		{
			name:    "IndexNotEquals(x[7], lit)",
			c:       constraint.IndexNotEquals{Target: x, Key: intKey, Value: strLit},
			kills:   []Assignment{rootX, xIdx7},
			nokills: []Assignment{rootY, xKind, xOther, xIdxK},
		},
		{
			name:    "IndexEqualsPath(x[\"k\"], y)",
			c:       constraint.IndexEqualsPath{Target: x, Key: strKey, Value: y},
			kills:   []Assignment{rootX, xIdxK, rootY},
			nokills: []Assignment{xKind, xOther, xIdx7},
		},
		{
			name:    "IndexNotEqualsPath(x[\"k\"], y)",
			c:       constraint.IndexNotEqualsPath{Target: x, Key: strKey, Value: y},
			kills:   []Assignment{rootX, xIdxK, rootY},
			nokills: []Assignment{xKind, xOther, xIdx7},
		},
		{
			name:    "EqPath(x, y)",
			c:       constraint.NewEqPath(x, y),
			kills:   []Assignment{rootX, rootY},
			nokills: []Assignment{xKind, xOther, xIdxK, xIdx7},
		},
		{
			name:    "NotEqPath(x, y)",
			c:       constraint.NewNotEqPath(x, y),
			kills:   []Assignment{rootX, rootY},
			nokills: []Assignment{xKind, xOther, xIdxK, xIdx7},
		},
		{
			name:    "KeyOf(x, y)",
			c:       constraint.KeyOf{Table: x, Key: y},
			kills:   []Assignment{rootX, rootY},
			nokills: []Assignment{xKind, xOther, xIdxK, xIdx7},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cond := constraint.FromConstraints(tc.c)

			for _, a := range tc.kills {
				out := KillRedefinedConditions(cond, 0, []Assignment{a})
				if !out.IsTrue() {
					t.Errorf("assignment %s should kill %s, got cond %v", formatAssign(a), tc.name, out)
				}
			}
			for _, a := range tc.nokills {
				out := KillRedefinedConditions(cond, 0, []Assignment{a})
				if out.IsTrue() {
					t.Errorf("assignment %s should NOT kill %s, but condition collapsed to ⊤", formatAssign(a), tc.name)
				}
				if !out.Equals(cond) {
					t.Errorf("assignment %s should NOT kill %s, but condition changed: in=%v out=%v",
						formatAssign(a), tc.name, cond, out)
				}
			}
		})
	}
}

func formatAssign(a Assignment) string {
	s := fmt.Sprintf("sym=%d", a.TargetSym)
	for _, seg := range a.TargetSegs {
		switch seg.Kind {
		case constraint.SegmentField:
			s += "." + seg.Name
		case constraint.SegmentIndexString:
			s += "[\"" + seg.Name + "\"]"
		case constraint.SegmentIndexInt:
			s += fmt.Sprintf("[%d]", seg.Index)
		}
	}
	return s
}

// TestKill_SubpathWrites is the DOMAIN_DESIGN.md §8.2 / §10.3 regression
// lock: `x.kind = …` MUST kill `FieldEquals{x, "kind", lit}`. Prior to this
// pass, VisitPaths exposed only the root path of FieldEquals, so the kill
// step missed the subpath write — an existing unsoundness. The current
// implementation uses SemanticAffectedPaths and must always kill.
func TestKill_SubpathWrites(t *testing.T) {
	x := constraint.Path{Root: "x", Symbol: 1}
	strLit := typ.LiteralString("v")
	fieldSeg := constraint.Segment{Kind: constraint.SegmentField, Name: "kind"}

	cond := constraint.FromConstraints(constraint.FieldEquals{Target: x, Field: "kind", Value: strLit})
	out := KillRedefinedConditions(cond, 0, []Assignment{{Point: 0, TargetSym: 1, TargetSegs: []constraint.Segment{fieldSeg}}})

	if !out.IsTrue() {
		t.Fatalf("`x.kind = …` did not kill FieldEquals{x, kind, lit}; got %v", out)
	}
}

// TestKill_PostUpdateConditionContainsNoKilledLiterals goes beyond a single
// literal: it builds a multi-literal condition mixing some literals that
// MUST be killed and some that must be retained, then asserts the
// post-update condition mentions zero killed literals (per Codex Q10.3).
func TestKill_PostUpdateConditionContainsNoKilledLiterals(t *testing.T) {
	x := constraint.Path{Root: "x", Symbol: 1}
	y := constraint.Path{Root: "y", Symbol: 2}
	strLit := typ.LiteralString("v")
	fieldSeg := constraint.Segment{Kind: constraint.SegmentField, Name: "kind"}

	killed := constraint.FieldEquals{Target: x, Field: "kind", Value: strLit}
	retained := constraint.NotNil{Path: y}

	cond := constraint.FromConjunction([]constraint.Constraint{killed, retained})
	out := KillRedefinedConditions(cond, 0, []Assignment{{Point: 0, TargetSym: 1, TargetSegs: []constraint.Segment{fieldSeg}}})

	if !out.HasConstraints() {
		t.Fatalf("expected NotNil(y) to remain after killing FieldEquals(x.kind, …), got %v", out)
	}

	for _, d := range out.Disjuncts {
		for _, lit := range d {
			if lit.Equals(killed) {
				t.Fatalf("killed literal still present in post-update condition: %v", out)
			}
		}
	}
}

// TestKill_DescendantWriteDoesNotKillRoot pins down the precision-preserving
// half of the kill rule: a write to a deeper SUBPATH of a constraint's read
// path does NOT kill the constraint. Writing x.deep does not change x
// itself, so Truthy(x) is preserved.
//
// This refines DOMAIN_DESIGN.md §8.2 (which states "ancestor or descendant"
// — too aggressive in the descendant direction; would lose precision the
// downstream type narrowing relies on). The implementation only kills when
// the assignment is at or above (a prefix of) the literal's read path; that
// alone fixes Codex's original subpath-write soundness gap because
// SemanticAffectedPaths now exposes the literal's full read path.
func TestKill_DescendantWriteDoesNotKillRoot(t *testing.T) {
	x := constraint.Path{Root: "x", Symbol: 1}
	deepSeg := constraint.Segment{Kind: constraint.SegmentField, Name: "deep"}

	cond := constraint.FromConstraints(constraint.Truthy{Path: x})
	out := KillRedefinedConditions(cond, 0, []Assignment{{Point: 0, TargetSym: 1, TargetSegs: []constraint.Segment{deepSeg}}})
	if out.IsTrue() {
		t.Fatalf("descendant write x.deep should NOT kill Truthy(x); got ⊤")
	}
	if !out.Equals(cond) {
		t.Fatalf("descendant write x.deep should leave Truthy(x) unchanged; got %v", out)
	}
}
