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

func subjectLivenessArtifactLawRow(t *testing.T, route, from, to, subject identity.ContentID, kind lifecycle.SubjectLivenessKind, state lifecycle.SubjectLivenessState) lifecycle.SubjectLiveness {
	t.Helper()
	id, idOK := lifecycle.SubjectLivenessIdentity(route, kind, subject)
	if !idOK {
		t.Fatal("subject-liveness identity")
	}
	row, rowOK := lifecycle.NewSubjectLiveness(id, route, from, to, subject, kind, state)
	if !rowOK {
		t.Fatal("subject-liveness row")
	}
	return row
}

func subjectLivenessArtifactLawArtifact(t *testing.T, rows []lifecycle.SubjectLiveness) *Artifact {
	t.Helper()
	frozen, catalog := coldLawPublication(t, programpublication.Publication{
		Lifecycle: lifecycle.Publication{SubjectLifetimes: rows},
	})
	return &Artifact{frozen: frozen, coldCatalog: catalog}
}

func TestArtifactIDSealsEverySubjectLivenessField(t *testing.T) {
	route := subjectLivenessArtifactLawID(1)
	from := subjectLivenessArtifactLawID(33)
	to := subjectLivenessArtifactLawID(65)
	subject := subjectLivenessArtifactLawID(97)
	baseRow := subjectLivenessArtifactLawRow(t, route, from, to, subject, lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessLive)
	base := artifactID(subjectLivenessArtifactLawArtifact(t, []lifecycle.SubjectLiveness{baseRow}))
	if !base.Available() {
		t.Fatal("base artifact identity unavailable")
	}

	variants := []struct {
		name string
		row  lifecycle.SubjectLiveness
	}{
		{
			name: "yield route and row id",
			row:  subjectLivenessArtifactLawRow(t, subjectLivenessArtifactLawID(2), from, to, subject, lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessLive),
		},
		{
			name: "yield from path",
			row:  subjectLivenessArtifactLawRow(t, route, subjectLivenessArtifactLawID(34), to, subject, lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessLive),
		},
		{
			name: "yield to path",
			row:  subjectLivenessArtifactLawRow(t, route, from, subjectLivenessArtifactLawID(66), subject, lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessLive),
		},
		{
			name: "subject kind",
			row:  subjectLivenessArtifactLawRow(t, route, from, to, subject, lifecycle.SubjectLivenessValue, lifecycle.SubjectLivenessLive),
		},
		{
			name: "subject id",
			row:  subjectLivenessArtifactLawRow(t, route, from, to, subjectLivenessArtifactLawID(98), lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessLive),
		},
		{
			name: "state",
			row:  subjectLivenessArtifactLawRow(t, route, from, to, subject, lifecycle.SubjectLivenessCell, lifecycle.SubjectLivenessDiesBefore),
		},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			got := artifactID(subjectLivenessArtifactLawArtifact(t, []lifecycle.SubjectLiveness{variant.row}))
			if !got.Available() || got == base {
				t.Fatalf("artifact identity = %v, base = %v", got, base)
			}
		})
	}
}
