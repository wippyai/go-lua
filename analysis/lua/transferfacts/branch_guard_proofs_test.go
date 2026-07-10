package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func TestBranchPathEvidenceForDirectCheckLaws(t *testing.T) {
	xPath := path.NewPath(1, "x")
	yPath := path.NewPath(2, "y")
	xsPath := path.NewPath(3, "xs")

	tests := []struct {
		name  string
		check branchcond.Check
		want  []branchEvidenceWant
	}{
		{
			name:  "truthy publishes presence and direct opposite falsy",
			check: branchcond.Check{Kind: branchcond.CheckTruthy, Path: xPath},
			want: []branchEvidenceWant{
				{kind: factflow.BranchPathEvidencePresence, target: xPath, presence: presence.Present(), hasPresence: true, edge: true},
				{kind: factflow.BranchPathEvidenceTruthy, target: xPath, edge: true, oppositeFalsy: true},
			},
		},
		{
			name:  "type equal non-nil publishes presence",
			check: branchcond.Check{Kind: branchcond.CheckTypeEqual, Path: xPath, TypeName: "string"},
			want: []branchEvidenceWant{
				{kind: factflow.BranchPathEvidencePresence, target: xPath, presence: presence.Present(), hasPresence: true, edge: true},
			},
		},
		{
			name:  "path inequality publishes not-equal on true edge and equal on false edge",
			check: branchcond.Check{Kind: branchcond.CheckPathNot, Path: xPath, OtherPath: yPath},
			want: []branchEvidenceWant{
				{kind: factflow.BranchPathEvidenceNotEqual, target: xPath, other: yPath, hasOther: true, edge: true},
				{kind: factflow.BranchPathEvidenceEqual, target: xPath, other: yPath, hasOther: true, edge: false},
			},
		},
		{
			name:  "negated index range publishes only on false edge",
			check: branchcond.Check{Kind: branchcond.CheckIndexInRange, Path: xPath, OtherPath: xsPath, Negated: true},
			want: []branchEvidenceWant{
				{kind: factflow.BranchPathEvidenceIndexInRange, target: xPath, other: xsPath, hasOther: true, edge: false},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := branchPathEvidenceForCheck(tc.check)
			for _, want := range tc.want {
				if !branchEvidenceContains(got, want) {
					t.Fatalf("missing evidence %#v in %#v", want, got)
				}
			}
		})
	}
}

type branchEvidenceWant struct {
	kind          factflow.BranchPathEvidenceKind
	target        path.Path
	other         path.Path
	hasOther      bool
	presence      presence.Value
	hasPresence   bool
	edge          bool
	oppositeFalsy bool
}

func branchEvidenceContains(got []factflow.BranchPathEvidence, want branchEvidenceWant) bool {
	for _, proof := range got {
		if proof.Kind() != want.kind || !proof.Path().Equal(want.target) || !proof.ActiveOnEdge(want.edge) {
			continue
		}
		if want.hasOther {
			other, ok := proof.OtherPath()
			if !ok || !other.Equal(want.other) {
				continue
			}
		}
		if want.hasPresence {
			gotPresence, ok := proof.Presence()
			if !ok || gotPresence != want.presence {
				continue
			}
		}
		if proof.OppositeEdgeImpliesFalsy() != want.oppositeFalsy {
			continue
		}
		return true
	}
	return false
}
