package outputowners

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestGeneratedEvidenceIsCurrent(t *testing.T) {
	if err := Generate(filepath.Join(packageDir(t), "evidence_gen.go"), true); err != nil {
		t.Fatal(err)
	}
	current, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Rows) == 0 {
		t.Fatal("generated Program output-owner evidence is empty")
	}
}

func TestCanonicalIsDetachedAndAgreesWithCurrentGeneratedEvidence(t *testing.T) {
	current, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	want := Generated.Canonical()
	if len(want) == 0 || !bytes.Equal(current.Canonical(), want) {
		t.Fatal("Current and Generated Program output-owner evidence disagree")
	}
	got := current.Canonical()
	got[0] ^= 0xff
	if !bytes.Equal(current.Canonical(), want) {
		t.Fatal("Canonical returned bytes that are not detached")
	}
}

func TestEvidenceRejectsMissingDuplicateUnknownAndReorderedRows(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	entries := denominator.GeneratedRelationEntries()
	cases := []struct {
		name   string
		mutate func(*Evidence)
	}{
		{"missing", func(e *Evidence) { e.Rows = e.Rows[1:] }},
		{"duplicate", func(e *Evidence) { e.Rows = append(e.Rows, e.Rows[0]) }},
		{"unknown", func(e *Evidence) {
			e.Rows = append(e.Rows, Row{Relation: schema.EntryID{}, Owner: denominator.RelationOwnerProgramFlow})
		}},
		{"reordered", func(e *Evidence) { e.Rows[0], e.Rows[1] = e.Rows[1], e.Rows[0] }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := clone(evidence)
			test.mutate(&changed)
			changed.Digest = digest(changed)
			if err := changed.Validate(entries); err == nil {
				t.Fatal("invalid Program output-owner evidence was accepted")
			}
		})
	}
}

func TestEveryProgramRelationHasItsDeclaredOwner(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[schema.EntryID]*denominator.RelationEntry)
	for _, entry := range denominator.GeneratedRelationEntries() {
		byID[entry.ID()] = entry
	}
	for _, row := range evidence.Rows {
		entry := byID[row.Relation]
		if entry == nil || entry.Owner() != row.Owner {
			t.Fatalf("output owner row %#v is not sourced from denominator", row)
		}
	}
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate output-owner evidence test")
	}
	return filepath.Dir(file)
}
