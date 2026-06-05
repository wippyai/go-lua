package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// LocalAddress is a point-local path identity. It preserves the SSA version
// carried by constraint.Path and is appropriate for facts that are bound to one
// program point before assignment kills/redefinitions are applied.
type LocalAddress struct {
	path constraint.Path
}

// LocalAddressOfPath normalizes a concrete source path into the point-local
// address mode. Empty paths have no address.
func LocalAddressOfPath(path constraint.Path) (LocalAddress, bool) {
	if path.IsEmpty() {
		return LocalAddress{}, false
	}
	return LocalAddress{path: cloneAddressPath(path)}, true
}

// Path returns a defensive copy of the underlying source path.
func (a LocalAddress) Path() constraint.Path {
	return cloneAddressPath(a.path)
}

// Stable returns the version-insensitive address for must-fact domains.
func (a LocalAddress) Stable() (StableAddress, bool) {
	return StableAddressOfPath(a.path)
}

// SameVersion reports exact point-local identity.
func (a LocalAddress) SameVersion(b LocalAddress) bool {
	return a.path.Equal(b.path)
}

type stableAddressKind uint8

const (
	stableAddressInvalid stableAddressKind = iota
	stableAddressSymbol
	stableAddressRoot
)

// StableAddress is the version-insensitive path identity used by finite must
// fact domains. Its string key is only an internal deterministic map key; all
// semantic relation checks go through this structured value.
type StableAddress struct {
	kind     stableAddressKind
	symbol   cfg.SymbolID
	root     string
	segments []constraint.Segment
}

// StableAddressOfPath lowers a source path to the stable identity used by
// point-local must facts. Symbol identity wins; otherwise Root identifies
// placeholders and boundary-relative paths.
func StableAddressOfPath(path constraint.Path) (StableAddress, bool) {
	switch {
	case path.Symbol != 0:
		return StableAddressOfSymbol(path.Symbol, path.Segments)
	case path.Root != "":
		return StableAddressOfRoot(path.Root, path.Segments)
	default:
		return StableAddress{}, false
	}
}

// StableAddressOfSymbol builds a stable symbol-rooted address.
func StableAddressOfSymbol(sym cfg.SymbolID, segments []constraint.Segment) (StableAddress, bool) {
	if sym == 0 {
		return StableAddress{}, false
	}
	return StableAddress{
		kind:     stableAddressSymbol,
		symbol:   sym,
		segments: cloneAddressSegments(segments),
	}, true
}

// StableAddressOfRoot builds a stable root-name address for placeholders and
// boundary paths that are not represented by CFG symbols.
func StableAddressOfRoot(root string, segments []constraint.Segment) (StableAddress, bool) {
	if root == "" {
		return StableAddress{}, false
	}
	return StableAddress{
		kind:     stableAddressRoot,
		root:     root,
		segments: cloneAddressSegments(segments),
	}, true
}

// StableAddressFromKey parses a deterministic fact key back into structured
// identity. It accepts keys produced by StableAddress.Key.
func StableAddressFromKey(key constraint.PathKey) (StableAddress, bool) {
	if key == "" {
		return StableAddress{}, false
	}
	if sym, segments, ok := ParseSymbolPathKey(key); ok {
		return StableAddressOfSymbol(sym, segments)
	}
	root, suffix, ok := splitStableRootKey(string(key))
	if !ok {
		return StableAddress{}, false
	}
	segments, ok := parseSymbolPathSegments(suffix)
	if !ok {
		return StableAddress{}, false
	}
	return StableAddressOfRoot(root, segments)
}

// StablePathKey returns the deterministic key for path's stable address.
func StablePathKey(path constraint.Path) constraint.PathKey {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return ""
	}
	return addr.Key()
}

// Key returns the internal deterministic key for map/set carriers.
func (a StableAddress) Key() constraint.PathKey {
	switch a.kind {
	case stableAddressSymbol:
		return SymbolPathKey(a.symbol, a.segments)
	case stableAddressRoot:
		return constraint.Path{Root: a.root, Segments: cloneAddressSegments(a.segments)}.Key()
	default:
		return ""
	}
}

// Path returns a path view for consumers that still operate on constraint.Path.
func (a StableAddress) Path() (constraint.Path, bool) {
	switch a.kind {
	case stableAddressSymbol:
		return constraint.Path{Symbol: a.symbol, Segments: cloneAddressSegments(a.segments)}, true
	case stableAddressRoot:
		return constraint.Path{Root: a.root, Segments: cloneAddressSegments(a.segments)}, true
	default:
		return constraint.Path{}, false
	}
}

// Symbol returns the root symbol when this is a symbol address.
func (a StableAddress) Symbol() (cfg.SymbolID, bool) {
	return a.symbol, a.kind == stableAddressSymbol && a.symbol != 0
}

// Root returns the root name when this is a non-symbol root address.
func (a StableAddress) Root() (string, bool) {
	return a.root, a.kind == stableAddressRoot && a.root != ""
}

// Segments returns a defensive copy of the address suffix.
func (a StableAddress) Segments() []constraint.Segment {
	return cloneAddressSegments(a.segments)
}

// Equal reports stable identity equality.
func (a StableAddress) Equal(b StableAddress) bool {
	if a.kind != b.kind || a.symbol != b.symbol || a.root != b.root || len(a.segments) != len(b.segments) {
		return false
	}
	for i := range a.segments {
		if a.segments[i] != b.segments[i] {
			return false
		}
	}
	return true
}

// HasPrefix reports whether prefix is this address or an ancestor of it.
func (a StableAddress) HasPrefix(prefix StableAddress) bool {
	if !a.sameRoot(prefix) || len(prefix.segments) > len(a.segments) {
		return false
	}
	for i := range prefix.segments {
		if a.segments[i] != prefix.segments[i] {
			return false
		}
	}
	return true
}

// Overlaps reports whether two addresses share a root and one path is a prefix
// of the other.
func (a StableAddress) Overlaps(b StableAddress) bool {
	return a.HasPrefix(b) || b.HasPrefix(a)
}

func (a StableAddress) sameRoot(b StableAddress) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case stableAddressSymbol:
		return a.symbol != 0 && a.symbol == b.symbol
	case stableAddressRoot:
		return a.root != "" && a.root == b.root
	default:
		return false
	}
}

func splitStableRootKey(key string) (string, string, bool) {
	if key == "" {
		return "", "", false
	}
	for i := 0; i < len(key); i++ {
		if key[i] == '.' || key[i] == '[' {
			if i == 0 {
				return "", "", false
			}
			return key[:i], key[i:], true
		}
	}
	return key, "", true
}

func cloneAddressPath(path constraint.Path) constraint.Path {
	path.Segments = cloneAddressSegments(path.Segments)
	return path
}

func cloneAddressSegments(segments []constraint.Segment) []constraint.Segment {
	if len(segments) == 0 {
		return nil
	}
	return append([]constraint.Segment(nil), segments...)
}
