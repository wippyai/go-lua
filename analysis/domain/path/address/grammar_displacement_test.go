package address

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// A key's spelling is a property of the path grammar, never of the semantic
// address wrapper carrying it. Stable addressing drops a version and local
// addressing requires one, but both format the same symbol and named roots.
func TestSemanticWrappersShareCanonicalPathKeyGrammar(t *testing.T) {
	field := []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}

	stable, ok := StableOfPath(pathdom.Path{Symbol: 7, Version: 9, Segments: field})
	if !ok || stable.Key() != pathdom.PathKey("sym7.field") {
		t.Fatalf("stable key = %q/%v, want sym7.field/true", stable.Key(), ok)
	}
	local, ok := LocalOfPath(pathdom.Path{Symbol: 7, Version: 9, Segments: field})
	if !ok || local.Key() != pathdom.PathKey("sym7@9.field") {
		t.Fatalf("local key = %q/%v, want sym7@9.field/true", local.Key(), ok)
	}

	for _, tc := range []struct {
		name string
		path pathdom.Path
		want pathdom.PathKey
	}{
		{name: "named", path: pathdom.Path{Root: "a.b"}, want: "n3:a.b"},
		{name: "placeholder", path: pathdom.NewPlaceholder(0), want: "n2:$0"},
		{name: "return", path: pathdom.Path{Root: "ret[1]"}, want: "n6:ret[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stable, ok := StableOfPath(tc.path)
			if !ok || stable.Key() != tc.want {
				t.Fatalf("stable key = %q/%v, want %q/true", stable.Key(), ok, tc.want)
			}
			local, ok := LocalOfPath(tc.path)
			if !ok || local.Key() != tc.want {
				t.Fatalf("local key = %q/%v, want %q/true", local.Key(), ok, tc.want)
			}
		})
	}

	suffix, ok := RelativeStaticMemberSuffixKey(field)
	if !ok || suffix.PathKey() != pathdom.PathKey("n0:.field") {
		t.Fatalf("suffix key = %q/%v, want n0:.field/true", suffix.PathKey(), ok)
	}
}
