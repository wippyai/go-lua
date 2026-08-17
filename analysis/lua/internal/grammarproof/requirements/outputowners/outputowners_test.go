package outputowners

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/relations"
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

func TestEvidenceRejectsMissingDuplicateAndUnknownRows(t *testing.T) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*Evidence)
	}{
		{name: "missing", mutate: func(e *Evidence) { e.Rows = append([]Row(nil), e.Rows[1:]...) }},
		{name: "duplicate", mutate: func(e *Evidence) { e.Rows = append(e.Rows, e.Rows[0]) }},
		{name: "unknown", mutate: func(e *Evidence) {
			e.Rows = append(e.Rows, Row{Output: "TargetContract@-", Owner: relations.OwnerProgramFlow})
		}},
		{name: "reordered", mutate: func(e *Evidence) { e.Rows[0], e.Rows[1] = e.Rows[1], e.Rows[0] }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := Evidence{SchemaDigest: evidence.SchemaDigest, Rows: append([]Row(nil), evidence.Rows...)}
			test.mutate(&changed)
			changed.Digest = digest(changed)
			if err := changed.Validate(schema); err == nil {
				t.Fatal("invalid Program output-owner evidence was accepted")
			}
		})
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
