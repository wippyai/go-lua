package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

func subjectLivenessArtifactLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func subjectLivenessArtifactLawRow(t *testing.T, subject identity.ContentID, kind lifecycle.SubjectLivenessKind, lo, hi uint32, state lifecycle.SubjectLivenessState) lifecycle.SubjectLivenessSpan {
	t.Helper()
	id, idOK := lifecycle.SubjectLivenessSpanIdentity(kind, subject, lo, hi)
	if !idOK {
		t.Fatal("subject-liveness span identity")
	}
	row, rowOK := lifecycle.NewSubjectLivenessSpan(id, subject, kind, lo, hi, state)
	if !rowOK {
		t.Fatal("subject-liveness span row")
	}
	return row
}

func subjectLivenessArtifactLawArtifact(t *testing.T, rows []lifecycle.SubjectLivenessSpan) *Artifact {
	t.Helper()
	frozen, catalog := coldLawPublication(t, programpublication.Publication{
		Lifecycle: lifecycle.Publication{SubjectSpans: rows},
	})
	return &Artifact{frozen: frozen, coldCatalog: catalog}
}

func subjectLivenessArtifactLawBoundary(t *testing.T, call, route, from, to identity.ContentID, ordinal uint32) lifecycle.SubjectYieldBoundary {
	t.Helper()
	id, idOK := lifecycle.SubjectYieldBoundaryIdentity(call, route)
	if !idOK {
		t.Fatal("subject-yield-boundary identity")
	}
	row, rowOK := lifecycle.NewSubjectYieldBoundary(id, call, route, from, to, ordinal)
	if !rowOK {
		t.Fatal("subject-yield-boundary row")
	}
	return row
}

func subjectLivenessArtifactLawBoundaryArtifact(t *testing.T, rows []lifecycle.SubjectYieldBoundary) *Artifact {
	t.Helper()
	frozen, catalog := coldLawPublication(t, programpublication.Publication{
		Lifecycle: lifecycle.Publication{SubjectBoundaries: rows},
	})
	return &Artifact{frozen: frozen, coldCatalog: catalog}
}

// TestArtifactIDSealsEverySubjectLivenessSpanField keeps the whole span plane
// inside the artifact identity: two programs that disagree about any column of
// a live range are two artifacts, including the range itself and the answer.
func TestArtifactIDSealsEverySubjectLivenessSpanField(t *testing.T) {
	subject := subjectLivenessArtifactLawID(97)
	baseRow := subjectLivenessArtifactLawRow(t, subject, lifecycle.SubjectLivenessCell, 2, 5, lifecycle.SubjectLivenessLive)
	base := artifactID(subjectLivenessArtifactLawArtifact(t, []lifecycle.SubjectLivenessSpan{baseRow}))
	if !base.Available() {
		t.Fatal("base artifact identity unavailable")
	}

	variants := []struct {
		name string
		row  lifecycle.SubjectLivenessSpan
	}{
		{
			name: "subject kind",
			row:  subjectLivenessArtifactLawRow(t, subject, lifecycle.SubjectLivenessValue, 2, 5, lifecycle.SubjectLivenessLive),
		},
		{
			name: "subject id",
			row:  subjectLivenessArtifactLawRow(t, subjectLivenessArtifactLawID(98), lifecycle.SubjectLivenessCell, 2, 5, lifecycle.SubjectLivenessLive),
		},
		{
			name: "range lo",
			row:  subjectLivenessArtifactLawRow(t, subject, lifecycle.SubjectLivenessCell, 3, 5, lifecycle.SubjectLivenessLive),
		},
		{
			name: "range hi",
			row:  subjectLivenessArtifactLawRow(t, subject, lifecycle.SubjectLivenessCell, 2, 6, lifecycle.SubjectLivenessLive),
		},
		{
			name: "state",
			row:  subjectLivenessArtifactLawRow(t, subject, lifecycle.SubjectLivenessCell, 2, 5, lifecycle.SubjectLivenessDiesBefore),
		},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			got := artifactID(subjectLivenessArtifactLawArtifact(t, []lifecycle.SubjectLivenessSpan{variant.row}))
			if !got.Available() || got == base {
				t.Fatalf("artifact identity = %v, base = %v", got, base)
			}
		})
	}
}

// The boundary sequence is half the plane: the spans are ranges over it, so a
// program that numbers its boundaries differently is a different artifact even
// when every span row is byte-identical.
func TestArtifactIDSealsEverySubjectYieldBoundaryField(t *testing.T) {
	route := subjectLivenessArtifactLawID(1)
	call := subjectLivenessArtifactLawID(137)
	from := subjectLivenessArtifactLawID(33)
	to := subjectLivenessArtifactLawID(65)
	baseRow := subjectLivenessArtifactLawBoundary(t, call, route, from, to, 0)
	base := artifactID(subjectLivenessArtifactLawBoundaryArtifact(t, []lifecycle.SubjectYieldBoundary{baseRow}))
	if !base.Available() {
		t.Fatal("base artifact identity unavailable")
	}

	variants := []struct {
		name string
		row  lifecycle.SubjectYieldBoundary
	}{
		{
			name: "call occurrence and row id",
			row:  subjectLivenessArtifactLawBoundary(t, subjectLivenessArtifactLawID(138), route, from, to, 0),
		},
		{
			name: "yield route and row id",
			row:  subjectLivenessArtifactLawBoundary(t, call, subjectLivenessArtifactLawID(2), from, to, 0),
		},
		{
			name: "yield from path",
			row:  subjectLivenessArtifactLawBoundary(t, call, route, subjectLivenessArtifactLawID(34), to, 0),
		},
		{
			name: "yield to path",
			row:  subjectLivenessArtifactLawBoundary(t, call, route, from, subjectLivenessArtifactLawID(66), 0),
		},
		{
			name: "ordinal",
			row:  subjectLivenessArtifactLawBoundary(t, call, route, from, to, 1),
		},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			got := artifactID(subjectLivenessArtifactLawBoundaryArtifact(t, []lifecycle.SubjectYieldBoundary{variant.row}))
			if !got.Available() || got == base {
				t.Fatalf("artifact identity = %v, base = %v", got, base)
			}
		})
	}
}
