package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

const surfaceLocalLawDomain = "analysis/engine/equation/surface-local-law"

func surfaceLocalLawKey(t *testing.T, surface Surface) composition.Key {
	t.Helper()
	key, ok := identityKey(surfaceLocalLawDomain, func(writer *canonical.DigestWriter) bool {
		return writeSurface(writer, surface)
	})
	if !ok {
		t.Fatalf("surface preimage rejected: %#v", surface)
	}
	return key
}

// TestSummaryContentLocalParticipatesAtFullWidth proves the summary surface
// coordinate reaches identity at full width: two content coordinates that
// agree on their first eight bytes remain two distinct surfaces.
func TestSummaryContentLocalParticipatesAtFullWidth(t *testing.T) {
	normalizer := boundaryKey(212)
	first := Surface{Factor: boundaryKey(211), Form: SurfaceReadSummary, Semantic: normalizer, Normalizer: normalizer}
	second := first
	for index := 0; index < 8; index++ {
		first.Content[index] = 0x5a
		second.Content[index] = 0x5a
	}
	first.Content[8], second.Content[8] = 1, 2
	if !first.Available() || !second.Available() {
		t.Fatal("content-space summary surface is unavailable")
	}
	if surfaceLocalLawKey(t, first) == surfaceLocalLawKey(t, second) {
		t.Fatal("content coordinates agreeing on eight bytes produced one surface identity")
	}
}

// TestOrdinalAndContentLocalsNeverAlias proves the two coordinate spaces are
// disjoint. A content coordinate whose low bytes spell an ordinal is still a
// different surface, and a surface populating both spaces or neither carries
// no coordinate at all.
func TestOrdinalAndContentLocalsNeverAlias(t *testing.T) {
	factor := boundaryKey(213)
	ordinal := Surface{Factor: factor, Form: SurfaceReadSelect, Local: 1, Semantic: factor}
	content := Surface{Factor: factor, Form: SurfaceReadSelect, Semantic: factor}
	content.Content[0] = 1
	if !ordinal.Available() || !content.Available() {
		t.Fatal("selected surface is unavailable in one of the two coordinate spaces")
	}
	if surfaceLocalLawKey(t, ordinal) == surfaceLocalLawKey(t, content) {
		t.Fatal("an ordinal coordinate and a content coordinate share one surface identity")
	}
	both := ordinal
	both.Content = content.Content
	neither := Surface{Factor: factor, Form: SurfaceReadSelect, Semantic: factor}
	if both.LocalAvailable() || both.Available() || neither.LocalAvailable() || neither.Available() {
		t.Fatal("a surface admitted a coordinate in both spaces or in neither")
	}
	if _, ok := identityKey(surfaceLocalLawDomain, func(writer *canonical.DigestWriter) bool {
		return writeSurface(writer, both)
	}); ok {
		t.Fatal("a surface populating both coordinate spaces produced a preimage")
	}
}

// TestDeclaredCoordinateFormsRejectContentLocals proves the forms whose
// coordinate is a declared Factor ordinal admit only the ordinal space.
func TestDeclaredCoordinateFormsRejectContentLocals(t *testing.T) {
	factor := boundaryKey(214)
	rows := []Surface{
		{Factor: factor, Form: SurfaceReadExact},
		{Factor: factor, Form: SurfaceWriteExact, Mode: TargetModeStrong},
	}
	for _, row := range rows {
		ordinal := row
		ordinal.Local = 4
		content := row
		content.Content[31] = 9
		if !ordinal.Available() {
			t.Fatalf("declared-coordinate form rejected its ordinal: %#v", ordinal)
		}
		if content.Available() {
			t.Fatalf("declared-coordinate form admitted a content coordinate: %#v", content)
		}
	}
}
