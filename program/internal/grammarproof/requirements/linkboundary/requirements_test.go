package linkboundary

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestBuildDerivesCompleteCanonicalBoundary(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 1 {
		t.Fatalf("boundary rows = %d, want one", len(evidence.Rows))
	}
	row := evidence.Rows[0]
	if row.Boundary.Origin != semanticsource.OriginLinkBoundary || row.Boundary.Facet != 0 || row.Owner != relations.OwnerLinkBoundary || row.Form != relations.FormVirtualPredicate {
		t.Fatalf("boundary row = %#v", row)
	}
	want := boundaryParentSpecs()
	if len(row.Parents) != len(want) {
		t.Fatalf("boundary parents = %#v", row.Parents)
	}
	for index, parent := range row.Parents {
		if parent.Origin != want[index].origin || parent.Facet != want[index].facet {
			t.Fatalf("boundary parent %d = %#v, want %#v", index, parent, want[index])
		}
	}
}

func TestCanonicalIsDeterministicAndOwnsDigest(t *testing.T) {
	first, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, secondBytes := first.Canonical(), second.Canonical()
	if len(firstBytes) == 0 || !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("canonical bytes are not stable: first=%x second=%x", firstBytes, secondBytes)
	}
	sum := sha256.Sum256(firstBytes)
	if got := hex.EncodeToString(sum[:]); got != first.Digest {
		t.Fatalf("canonical digest = %s, evidence digest = %s", got, first.Digest)
	}
}

func TestCanonicalChangesWithValidatedEvidence(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	baseline := evidence.Canonical()
	changed := evidence
	changed.SchemaDigest = "0000000000000000000000000000000000000000000000000000000000000000"
	changed.Digest = digest(changed)
	if err := changed.Validate(); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(baseline, changed.Canonical()) {
		t.Fatal("canonical bytes did not change with schema identity")
	}
}

func TestCanonicalRejectsMalformedEvidence(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	malformed := evidence
	malformed.Rows = append([]Row(nil), evidence.Rows...)
	malformed.Rows[0].Parents = append([]Reference(nil), evidence.Rows[0].Parents...)
	malformed.Rows[0].Parents[0].Facet = semanticsource.FacetTargetABI
	malformed.Digest = digest(malformed)
	if got := malformed.Canonical(); got != nil {
		t.Fatalf("malformed evidence published canonical bytes: %x", got)
	}

	malformed = evidence
	malformed.SchemaDigest = "not-a-sha256"
	malformed.Digest = digest(malformed)
	if got := malformed.Canonical(); got != nil {
		t.Fatalf("malformed schema identity published canonical bytes: %x", got)
	}
}

func TestCanonicalReturnsDetachedBytes(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	first := evidence.Canonical()
	want := append([]byte(nil), first...)
	first[0] ^= 0xff
	if got := evidence.Canonical(); !bytes.Equal(got, want) {
		t.Fatalf("caller mutation changed canonical evidence: got=%x want=%x", got, want)
	}
}

func TestDeriveRejectsMissingBoundaryRow(t *testing.T) {
	rows, digest := canonicalRows(t)
	filtered := rows[:0]
	for _, row := range rows {
		if row.Owner != relations.OwnerLinkBoundary {
			filtered = append(filtered, row)
		}
	}
	_, err := derive(filtered, digest)
	if !errors.Is(err, ErrMissingBoundary) {
		t.Fatalf("derive() error = %v, want %v", err, ErrMissingBoundary)
	}
}

func TestDeriveRejectsDuplicateBoundaryRow(t *testing.T) {
	rows, digest := canonicalRows(t)
	for _, row := range rows {
		if row.Owner == relations.OwnerLinkBoundary {
			rows = append(rows, row)
			break
		}
	}
	_, err := derive(rows, digest)
	if !errors.Is(err, ErrDuplicateBoundary) {
		t.Fatalf("derive() error = %v, want %v", err, ErrDuplicateBoundary)
	}
}

func TestDeriveRejectsUnknownBoundaryRow(t *testing.T) {
	rows, digest := canonicalRows(t)
	var other relations.Row
	for _, row := range rows {
		if row.Owner != relations.OwnerLinkBoundary {
			other = row
			break
		}
	}
	for index := range rows {
		if rows[index].Owner == relations.OwnerLinkBoundary {
			rows[index].Definition = other.Definition
			break
		}
	}
	_, err := derive(rows, digest)
	if !errors.Is(err, ErrUnknownBoundary) {
		t.Fatalf("derive() error = %v, want %v", err, ErrUnknownBoundary)
	}
}

func canonicalRows(t *testing.T) ([]relations.Row, [32]byte) {
	t.Helper()
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	return schema.Rows(), schema.Digest()
}
