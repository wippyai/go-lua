package targetingress

import (
	"bytes"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/semanticsource"
	"github.com/wippyai/go-lua/analysis/schema/relations"
)

func TestGeneratedEvidenceIsCurrent(t *testing.T) {
	if err := Generate(filepath.Join(packageDir(t), "evidence_gen.go"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := Current(); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalIsDetachedAndAgreesWithCurrentGeneratedEvidence(t *testing.T) {
	current, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	want := Generated.Canonical()
	if len(want) == 0 || !bytes.Equal(current.Canonical(), want) {
		t.Fatal("Current and Generated Target ingress evidence disagree")
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
		want   error
		mutate func(*Evidence)
	}{
		{name: "missing", want: ErrMissingRow, mutate: func(e *Evidence) { e.Rows = append([]Row(nil), e.Rows[1:]...) }},
		{name: "duplicate", want: ErrDuplicateRow, mutate: func(e *Evidence) { e.Rows[len(e.Rows)-1] = e.Rows[0] }},
		{name: "unknown", want: ErrUnknownRow, mutate: func(e *Evidence) {
			e.Rows[len(e.Rows)-1] = Row{Relation: Reference{}, Owner: relations.OwnerTarget, Form: relations.FormAuthored}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := Evidence{SchemaDigest: evidence.SchemaDigest, Rows: append([]Row(nil), evidence.Rows...)}
			test.mutate(&changed)
			changed.Digest = digest(changed)
			if err := changed.Validate(schema); !errors.Is(err, test.want) {
				t.Fatalf("Validate() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDeriveRejectsMissingDuplicateAndUnknownIngressRows(t *testing.T) {
	rows, digest, schema := canonicalRows(t)
	missing := append([]relations.Row(nil), rows...)
	for index, row := range missing {
		if row.Owner == relations.OwnerTarget {
			missing = append(missing[:index], missing[index+1:]...)
			break
		}
	}
	if _, err := derive(missing, digest, schema); !errors.Is(err, ErrMissingRow) {
		t.Fatalf("missing derive() error = %v, want %v", err, ErrMissingRow)
	}

	duplicate := append([]relations.Row(nil), rows...)
	for _, row := range rows {
		if row.Owner == relations.OwnerTarget {
			duplicate = append(duplicate, row)
			break
		}
	}
	if _, err := derive(duplicate, digest, schema); !errors.Is(err, ErrDuplicateRow) {
		t.Fatalf("duplicate derive() error = %v, want %v", err, ErrDuplicateRow)
	}

	unknown := append([]relations.Row(nil), rows...)
	for index := range unknown {
		if unknown[index].Owner != relations.OwnerTarget {
			unknown[index].Owner = relations.OwnerTarget
			break
		}
	}
	if _, err := derive(unknown, digest, schema); !errors.Is(err, ErrUnknownRow) {
		t.Fatalf("unknown derive() error = %v, want %v", err, ErrUnknownRow)
	}
}

func canonicalRows(t *testing.T) ([]relations.Row, [32]byte, semanticsource.ProgramSchema) {
	t.Helper()
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	return schema.Rows(), schema.Digest(), schema
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate Target ingress evidence test")
	}
	return filepath.Dir(file)
}
