package keyspace

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func field(name string) segment.Segment {
	return segment.Segment{Kind: segment.SegmentField, Name: name}
}

func indexStr(key string) segment.Segment {
	return segment.Segment{Kind: segment.SegmentIndexString, Name: key}
}

func indexInt(i int) segment.Segment {
	return segment.Segment{Kind: segment.SegmentIndexInt, Index: i}
}

// segmentLists covers empty, deep/nested, field, string-index needing escaping,
// and integer indexes (including negative).
func segmentLists() [][]segment.Segment {
	return [][]segment.Segment{
		nil,
		{field("name")},
		{field("a"), field("b"), field("c")},
		{indexStr("k")},
		{indexStr("needs\"quote")},
		{indexStr("back\\slash")},
		{indexStr("dot.in.key")},
		{indexStr("bracket]key")},
		{indexInt(0)},
		{indexInt(7)},
		{indexInt(-1)},
		{indexInt(-42)},
		{field("a"), indexInt(3), field("b")},
		{field("a"), indexStr("k"), indexInt(-2)},
		{indexStr("x"), field("x")},
		{field("x"), indexStr("x")},
	}
}

// corpus builds every root flavor crossed with every segment list. Each entry is
// produced through the SAME public KeySpace constructor that later stages use, so
// the test exercises the real build path, then asserts Format equivalence against
// the canonical old spelling oracle for that flavor.
type corpusEntry struct {
	key    Key
	oracle pathdom.PathKey
}

func buildCorpus(t *testing.T, ks *KeySpace) []corpusEntry {
	t.Helper()
	var out []corpusEntry

	add := func(k Key, oracle pathdom.PathKey) {
		out = append(out, corpusEntry{key: k, oracle: oracle})
	}

	for _, segs := range segmentLists() {
		// Versioned and unversioned resolver symbols, several symbol ids/versions.
		for _, sym := range []symbol.ID{1, 5, 42, 1000} {
			for _, ver := range []int{0, 1, 3, 99} {
				p := pathdom.Path{Symbol: sym, Version: ver, Segments: segs}
				add(ks.FromPath(p), p.Key())
			}
			// Resolver-key builder (matches Resolver.KeyForVersion spelling).
			for _, ver := range []int{1, 3, 99} {
				k, ok := ks.FromResolverKey(sym, ver, segs)
				if !ok {
					t.Fatalf("FromResolverKey(%d,%d) failed", sym, ver)
				}
				local, lok := pathaddr.LocalKeyForVersion(sym, ver, segs)
				if !lok {
					t.Fatalf("LocalKeyForVersion(%d,%d) failed", sym, ver)
				}
				add(k, local.PathKey())
			}
			// Unversioned resolver key.
			k, ok := ks.FromResolverKey(sym, 0, segs)
			if !ok {
				t.Fatalf("FromResolverKey(%d,0) failed", sym)
			}
			add(k, pathdom.Path{Symbol: sym, Segments: segs}.Key())

			// Stable compact symbol key.
			sk, ok := ks.FromStableSymbol(sym, segs)
			if !ok {
				t.Fatalf("FromStableSymbol(%d) failed", sym)
			}
			add(sk, pathaddr.SymbolPathKey(sym, segs))
		}

		// Placeholder roots.
		for _, idx := range []int{0, 1, 7, 100} {
			ph := pathdom.NewPlaceholder(idx)
			ph.Segments = segs
			add(ks.FromPath(ph), ph.Key())
		}

		// Return-slot roots.
		for _, idx := range []int{0, 1, 2, 11} {
			ret := pathdom.Path{Root: retSlotRoot(idx), Segments: segs}
			add(ks.FromPath(ret), ret.Key())
		}

		// Arbitrary named (global) roots.
		for _, name := range []string{"x", "globalTable", "_G", "config"} {
			named := pathdom.Path{Root: name, Segments: segs}
			add(ks.FromPath(named), named.Key())
		}

		// Rootless static-member heap suffix (only for non-empty segments).
		if rk, ok := ks.FromRootlessSuffix(segs); ok {
			oracle, ook := pathaddr.RelativeStaticMemberSuffixKey(segs)
			if !ook {
				t.Fatalf("RelativeStaticMemberSuffixKey failed for %v", segs)
			}
			add(rk, oracle.PathKey())
		}
	}
	return out
}

// buildPrefixLegs builds every root flavor over a small set of prefix-relevant
// segment lists, for use as the from/to legs of the Rebase proof.
func buildPrefixLegs(t *testing.T, ks *KeySpace) []corpusEntry {
	t.Helper()
	legSegs := [][]segment.Segment{
		nil,
		{field("a")},
		{field("a"), field("b")},
		{indexInt(3)},
		{indexInt(-1)},
		{indexStr("k")},
		{indexStr("dot.in.key")},
	}
	var out []corpusEntry
	add := func(k Key, oracle pathdom.PathKey) {
		out = append(out, corpusEntry{key: k, oracle: oracle})
	}
	for _, segs := range legSegs {
		for _, sym := range []symbol.ID{5, 42} {
			pv := pathdom.Path{Symbol: sym, Version: 2, Segments: segs}
			add(ks.FromPath(pv), pv.Key())
			pu := pathdom.Path{Symbol: sym, Segments: segs}
			add(ks.FromPath(pu), pu.Key())
			sk, _ := ks.FromStableSymbol(sym, segs)
			add(sk, pathaddr.SymbolPathKey(sym, segs))
		}
		ph := pathdom.NewPlaceholder(1)
		ph.Segments = segs
		add(ks.FromPath(ph), ph.Key())
		ret := pathdom.Path{Root: retSlotRoot(2), Segments: segs}
		add(ks.FromPath(ret), ret.Key())
		for _, name := range []string{"x", "globalTable"} {
			named := pathdom.Path{Root: name, Segments: segs}
			add(ks.FromPath(named), named.Key())
		}
		if rk, ok := ks.FromRootlessSuffix(segs); ok {
			oracle, _ := pathaddr.RelativeStaticMemberSuffixKey(segs)
			add(rk, oracle.PathKey())
		}
	}
	return out
}

func retSlotRoot(idx int) string {
	return "ret[" + itoa(idx) + "]"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func TestFormatMatchesOldSpelling(t *testing.T) {
	ks := New()
	corpus := buildCorpus(t, ks)
	if len(corpus) < 500 {
		t.Fatalf("corpus too small: %d", len(corpus))
	}
	for _, e := range corpus {
		if got := ks.Format(e.key); got != e.oracle {
			t.Fatalf("Format(%+v) = %q, want %q", e.key, got, e.oracle)
		}
	}
	t.Logf("Format equivalence verified over %d corpus keys", len(corpus))
}

func TestCanonicalNamedRootsMatchAddressEncoding(t *testing.T) {
	ks := New()
	roots := []string{
		"a.b",
		"a[0]",
		"$x",
		"$0x",
		"ret[]",
		"ret[x]",
		"ret[1",
		"s42",
		"s00042",
		"sym7",
		"sym00042",
		"sym42@003",
		"n04:sym7",
	}
	segments := [][]segment.Segment{
		nil,
		{field("value")},
		{indexStr("k")},
		{field("a"), indexInt(2)},
	}
	for _, root := range roots {
		for _, segs := range segments {
			p := pathdom.Path{Root: root, Segments: segs}
			stable, ok := pathaddr.StableOfPath(p)
			if !ok {
				t.Fatalf("StableOfPath(%q) failed", p.Key())
			}
			key := ks.FromPath(p)
			key.Canon = key.isStableNamed()
			got := ks.Format(key)
			if got != stable.Key() {
				t.Fatalf("canonical Format(%q) = %q, want address stable key %q", p.Key(), got, stable.Key())
			}
			if got == p.Key() {
				t.Fatalf("canonical Format(%q) was not encoded", p.Key())
			}
		}
	}
}

func TestInternStateKeyMatchesRawStateKeyParsing(t *testing.T) {
	ks := New()
	valid := []pathdom.PathKey{
		"sym42",
		"sym42@3",
		"sym42@3.field",
		`$0["item"]`,
		"ret[2].value",
		"global.value",
	}
	for _, raw := range valid {
		stateKey, ok := pathaddr.StateKeyFromPathKey(raw)
		if !ok {
			t.Fatalf("StateKeyFromPathKey(%q) failed", raw)
		}
		got, gotOK := ks.InternStateKey(stateKey)
		want, wantOK := ks.FromStateKey(raw)
		if gotOK != wantOK {
			t.Fatalf("InternStateKey(%q) ok = %v, want %v", raw, gotOK, wantOK)
		}
		if gotOK && got != want {
			t.Fatalf("InternStateKey(%q) = %+v (%q), want %+v (%q)", raw, got, ks.Format(got), want, ks.Format(want))
		}
	}

	named, ok := ks.InternStateKey(pathaddr.StateKey("s42.field"))
	if !ok || ks.Format(named) != "s42.field" {
		t.Fatalf("InternStateKey(named root s42.field) = %+v/%v format %q, want named root round-trip", named, ok, ks.Format(named))
	}
	if got, ok := ks.InternStateKey(pathaddr.StateKey(".field")); ok || got != (Key{}) {
		t.Fatalf("InternStateKey(invalid syntax) = %+v/%v, want rejected", got, ok)
	}
	if got, ok := ks.InternStateKey(""); ok || got != (Key{}) {
		t.Fatalf("InternStateKey(empty) = %+v/%v, want rejected", got, ok)
	}
}

func TestHasPrefixMatchesAddress(t *testing.T) {
	ks := New()
	corpus := buildCorpus(t, ks)
	for _, a := range corpus {
		for _, b := range corpus {
			want := pathaddr.PathKeyHasPrefix(a.oracle, b.oracle)
			got := ks.HasPrefix(a.key, b.key)
			if got != want {
				t.Fatalf("HasPrefix(%q,%q) = %v, want %v", a.oracle, b.oracle, got, want)
			}
			wantStrict := pathaddr.PathKeyHasStrictPrefix(a.oracle, b.oracle)
			gotStrict := ks.HasStrictPrefix(a.key, b.key)
			if gotStrict != wantStrict {
				t.Fatalf("HasStrictPrefix(%q,%q) = %v, want %v", a.oracle, b.oracle, gotStrict, wantStrict)
			}
		}
	}
}

func TestRebaseMatchesAddress(t *testing.T) {
	ks := New()
	corpus := buildCorpus(t, ks)
	// from/to legs span every flavor crossed with the prefix-relevant segment
	// lists; k stays the full corpus. Rebase only fires when from is a prefix of
	// k, so confining the prefix legs to short suffixes keeps the proof
	// exhaustive over the relations that can actually rebase.
	legs := buildPrefixLegs(t, ks)
	checked := 0
	for _, k := range corpus {
		for _, from := range legs {
			for _, to := range legs {
				wantKey, wantOK := pathaddr.RebasePathKey(k.oracle, from.oracle, to.oracle)
				gotKey, gotOK := ks.Rebase(k.key, from.key, to.key)
				if gotOK != wantOK {
					t.Fatalf("Rebase(%q,%q,%q) ok = %v, want %v",
						k.oracle, from.oracle, to.oracle, gotOK, wantOK)
				}
				if wantOK && ks.Format(gotKey) != wantKey {
					t.Fatalf("Rebase(%q,%q,%q) = %q, want %q",
						k.oracle, from.oracle, to.oracle, ks.Format(gotKey), wantKey)
				}
				checked++
			}
		}
	}
	t.Logf("Rebase equivalence verified over %d triples", checked)
}

func TestFieldCanonicalMatchesAddress(t *testing.T) {
	ks := New()
	corpus := buildCorpus(t, ks)
	for _, k := range corpus {
		wantKey, wantOK := pathaddr.FieldCanonicalPathKey(k.oracle)
		gotKey, gotOK := ks.FieldCanonical(k.key)
		if gotOK != wantOK {
			t.Fatalf("FieldCanonical(%q) ok = %v, want %v", k.oracle, gotOK, wantOK)
		}
		if wantOK && ks.Format(gotKey) != wantKey {
			t.Fatalf("FieldCanonical(%q) = %q, want %q", k.oracle, ks.Format(gotKey), wantKey)
		}
	}
}

func TestSuffixSegmentsMatchesAddress(t *testing.T) {
	ks := New()
	corpus := buildCorpus(t, ks)
	for _, k := range corpus {
		suffixKey, suffixOK := pathaddr.SuffixKeyFromPathKey(k.oracle)
		wantSegs, wantOK := pathaddr.RelativeStaticMemberSuffixSegments(suffixKey)
		wantOK = suffixOK && wantOK
		gotSegs, gotOK := ks.SuffixSegments(k.key)
		if gotOK != wantOK {
			t.Fatalf("SuffixSegments(%q) ok = %v, want %v", k.oracle, gotOK, wantOK)
		}
		if wantOK && !segmentsEqual(gotSegs, wantSegs) {
			t.Fatalf("SuffixSegments(%q) = %v, want %v", k.oracle, gotSegs, wantSegs)
		}
	}
}

func TestLessMatchesStringOrder(t *testing.T) {
	ks := New()
	corpus := buildCorpus(t, ks)
	for _, a := range corpus {
		for _, b := range corpus {
			want := a.oracle < b.oracle
			got := ks.Less(a.key, b.key)
			if got != want {
				t.Fatalf("Less(%q,%q) = %v, want %v", a.oracle, b.oracle, got, want)
			}
		}
	}
}

func TestKeyIsComparableMapKey(t *testing.T) {
	ks := New()
	m := make(map[Key]int)
	a := ks.FromPath(pathdom.Path{Symbol: 5, Version: 2, Segments: []segment.Segment{field("name")}})
	b := ks.FromPath(pathdom.Path{Symbol: 5, Version: 2, Segments: []segment.Segment{field("name")}})
	m[a] = 1
	if m[b] != 1 {
		t.Fatalf("structurally equal keys are not the same map key")
	}
}

func segmentsEqual(a, b []segment.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
