package lower_test

import (
	"os"
	"testing"

	programlower "github.com/wippyai/go-lua/program/lower"
)

// Content identity is a semantic artifact boundary, not a cached lowerer
// detail.  Exercise the frozen corpus twice so every currently reachable
// Program relation must be safe to encode and deterministic on reconstruction.
func TestFrozenFixtureCorpusHasStableProgramContentID(t *testing.T) {
	for _, fixture := range frozenFixturePaths(t) {
		source, err := os.ReadFile(fixture.absolute)
		if err != nil {
			t.Fatalf("read %s: %v", fixture.relative, err)
		}
		first, err := programlower.Lower(programlower.Source{Name: fixture.relative, Text: source})
		if err != nil {
			t.Fatalf("first lower %s: %v", fixture.relative, err)
		}
		second, err := programlower.Lower(programlower.Source{Name: fixture.relative, Text: source})
		if err != nil {
			t.Fatalf("second lower %s: %v", fixture.relative, err)
		}
		left, right := first.ContentID(), second.ContentID()
		if !left.Available() || left != right {
			t.Fatalf("ContentID(%s) = %v / %v; want equal available semantic IDs", fixture.relative, left, right)
		}
	}
}

func TestProgramContentIDTracksSourceSemanticsAndCoordinates(t *testing.T) {
	base, err := programlower.Lower(programlower.Source{Name: "identity.lua", Text: []byte("local x = 1\nreturn x")})
	if err != nil {
		t.Fatal(err)
	}
	value, err := programlower.Lower(programlower.Source{Name: "identity.lua", Text: []byte("local x = 2\nreturn x")})
	if err != nil {
		t.Fatal(err)
	}
	span, err := programlower.Lower(programlower.Source{Name: "identity.lua", Text: []byte("\nlocal x = 1\nreturn x")})
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := programlower.Lower(programlower.Source{Name: "other.lua", Text: []byte("local x = 1\nreturn x")})
	if err != nil {
		t.Fatal(err)
	}
	for label, got := range map[string][32]byte{
		"base": base.ContentID(), "value": value.ContentID(), "span": span.ContentID(), "name": renamed.ContentID(),
	} {
		if got == ([32]byte{}) {
			t.Fatalf("%s ContentID unavailable", label)
		}
	}
	if base.ContentID() == value.ContentID() || base.ContentID() == span.ContentID() || base.ContentID() == renamed.ContentID() {
		t.Fatal("semantic payload, source coordinates, and source identity must each affect ContentID")
	}
}
