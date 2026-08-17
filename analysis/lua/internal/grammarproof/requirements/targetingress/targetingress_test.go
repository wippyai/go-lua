package targetingress

import (
	"bytes"
	"errors"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
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
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	entries := denominator.GeneratedRelationEntries()
	cases := []struct {
		name   string
		want   error
		mutate func(*Evidence)
	}{
		{"missing", ErrMissingRow, func(e *Evidence) { e.Rows = e.Rows[1:] }},
		{"duplicate", ErrDuplicateRow, func(e *Evidence) { e.Rows[len(e.Rows)-1] = e.Rows[0] }},
		{"unknown", ErrUnknownRow, func(e *Evidence) { e.Rows[len(e.Rows)-1].Relation = schema.EntryID{} }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := clone(evidence)
			test.mutate(&changed)
			changed.Digest = digest(changed)
			if err := changed.Validate(entries); !errors.Is(err, test.want) {
				t.Fatalf("Validate() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDeriveRejectsMissingDuplicateAndUnknownIngressRows(t *testing.T) {
	entries := denominator.GeneratedRelationEntries()
	duplicate := append([]*denominator.RelationEntry(nil), entries...)
	for _, entry := range entries {
		if entry.Owner() == denominator.RelationOwnerTarget {
			duplicate = append(duplicate, entry)
			break
		}
	}
	if _, err := derive(duplicate); !errors.Is(err, ErrDuplicateRow) {
		t.Fatalf("duplicate derive() error = %v, want %v", err, ErrDuplicateRow)
	}

	// The one-catalog design makes a missing or foreign row fail at Evidence
	// validation, where the expected set is the catalog's closed Target set.
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	evidence.Rows = evidence.Rows[:len(evidence.Rows)-1]
	evidence.Digest = digest(evidence)
	if err := evidence.Validate(entries); !errors.Is(err, ErrMissingRow) {
		t.Fatalf("missing Validate() error = %v, want %v", err, ErrMissingRow)
	}
}

func TestTargetRowsPreserveExactDenominatorParents(t *testing.T) {
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
		if entry == nil || entry.Owner() != denominator.RelationOwnerTarget || entry.Form() != row.Form || entry.Owner() != row.Owner {
			t.Fatalf("row does not preserve denominator owner/form: %#v", row)
		}
		parents := entry.Parents()
		sort.Slice(parents, func(left, right int) bool { return bytes.Compare(parents[left][:], parents[right][:]) < 0 })
		if len(parents) != len(row.Ingress) {
			t.Fatalf("row %x ingress length = %d, want %d", row.Relation, len(row.Ingress), len(parents))
		}
		for index := range parents {
			if parents[index] != row.Ingress[index] {
				t.Fatalf("row %x ingress[%d] changed", row.Relation, index)
			}
		}
	}
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate Target ingress evidence test")
	}
	return filepath.Dir(file)
}
