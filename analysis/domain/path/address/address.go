package address

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Stable is the version-insensitive path identity used by finite state facts.
type Stable struct {
	root   Root
	suffix Suffix
}

// StableOfPath lowers a path to stable identity. Symbol identity wins over Root.
func StableOfPath(path pathdom.Path) (Stable, bool) {
	root, ok := RootOfPath(path)
	if !ok {
		return Stable{}, false
	}
	return stableOfRootAndSuffix(root, suffixOfOwnedSegments(cloneSegments(path.Segments)))
}

func stableOfSymbolOwnedSegments(sym symbol.ID, segments []segment.Segment) (Stable, bool) {
	root, ok := SymbolRoot(sym)
	if !ok {
		return Stable{}, false
	}
	return stableOfRootAndSuffix(root, suffixOfOwnedSegments(segments))
}

func stableOfRootOwnedSegments(root string, segments []segment.Segment) (Stable, bool) {
	pathRoot, ok := NamedRoot(root)
	if !ok {
		return Stable{}, false
	}
	return stableOfRootAndSuffix(pathRoot, suffixOfOwnedSegments(segments))
}

func stableOfRootAndSuffix(root Root, suffix Suffix) (Stable, bool) {
	if !root.isValid() {
		return Stable{}, false
	}
	return Stable{root: root, suffix: suffix}, true
}

// StableFromKey parses a key produced by Stable.Key.
func StableFromKey(key pathdom.PathKey) (Stable, bool) {
	if key == "" {
		return Stable{}, false
	}
	if sym, segments, ok := ParseSymbolPathKey(key); ok {
		return stableOfSymbolOwnedSegments(sym, segments)
	}
	if _, _, _, ok := ParseResolverPath(key); ok {
		return Stable{}, false
	}
	if root, segments, ok := parseEncodedNamedRootKey(string(key)); ok {
		return stableOfRootOwnedSegments(root, segments)
	}
	parsed, ok := parsePlainNamedRootSuffix(key)
	if !ok {
		return Stable{}, false
	}
	return stableOfRootOwnedSegments(parsed.root, parsed.segments)
}

// Key returns the deterministic key for map/set carriers.
func (a Stable) Key() pathdom.PathKey {
	return a.StableKey().PathKey()
}

// StableKey returns the deterministic stable-address key.
func (a Stable) StableKey() StableKey {
	if !a.root.isValid() {
		return ""
	}
	if sym, ok := a.root.Symbol(); ok {
		return SymbolStableKey(sym, a.suffix.segments)
	}
	root, _ := a.root.Name()
	return StableKey(namedRootKey(root, a.suffix.segments))
}

// Path returns a path view of the address.
func (a Stable) Path() (pathdom.Path, bool) {
	if !a.root.isValid() {
		return pathdom.Path{}, false
	}
	segments := a.suffix.Segments()
	if sym, ok := a.root.Symbol(); ok {
		return pathdom.Path{Symbol: sym, Segments: segments}, true
	}
	root, _ := a.root.Name()
	return pathdom.Path{Root: root, Segments: segments}, true
}

// RootIdentity returns the structured root identity.
func (a Stable) RootIdentity() Root {
	return a.root
}

// Symbol returns the root symbol when this is a symbol address.
func (a Stable) Symbol() (symbol.ID, bool) {
	return a.root.Symbol()
}

// Root returns the root name when this is a named-root address.
func (a Stable) Root() (string, bool) {
	return a.root.Name()
}

// Segments returns a defensive copy of the address suffix.
func (a Stable) Segments() []segment.Segment {
	return a.suffix.Segments()
}

// Suffix returns the structured path suffix.
func (a Stable) Suffix() Suffix {
	return SuffixOfSegments(a.suffix.segments)
}

// Append returns the descendant address reached by appending segments.
func (a Stable) Append(segments []segment.Segment) (Stable, bool) {
	if !a.root.isValid() {
		return Stable{}, false
	}
	if len(segments) == 0 {
		return stableOfRootAndSuffix(a.root, a.suffix)
	}
	next := make([]segment.Segment, len(a.suffix.segments)+len(segments))
	copy(next, a.suffix.segments)
	copy(next[len(a.suffix.segments):], segments)
	return stableOfRootAndSuffix(a.root, suffixOfOwnedSegments(next))
}

// Parent returns the address without its last suffix segment.
func (a Stable) Parent() (Stable, bool) {
	if !a.root.isValid() || len(a.suffix.segments) == 0 {
		return Stable{}, false
	}
	return stableOfRootAndSuffix(a.root, suffixOfOwnedSegments(cloneSegments(a.suffix.segments[:len(a.suffix.segments)-1])))
}

// Equal reports stable identity equality.
func (a Stable) Equal(b Stable) bool {
	return a.root.Equal(b.root) && a.suffix.Equal(b.suffix)
}

// HasPrefix reports whether prefix is this address or one of its ancestors.
func (a Stable) HasPrefix(prefix Stable) bool {
	return a.sameRoot(prefix) && a.suffix.HasPrefix(prefix.suffix)
}

// RemainderAfterPrefix returns the member/index suffix below prefix.
func (a Stable) RemainderAfterPrefix(prefix Stable) ([]segment.Segment, bool) {
	if !a.sameRoot(prefix) {
		return nil, false
	}
	return a.suffix.RemainderAfterPrefix(prefix.suffix)
}

// Overlaps reports whether two addresses share a root and a prefix relation.
func (a Stable) Overlaps(b Stable) bool {
	return a.HasPrefix(b) || b.HasPrefix(a)
}

func (a Stable) sameRoot(b Stable) bool {
	return a.root.isValid() && a.root.Equal(b.root)
}
