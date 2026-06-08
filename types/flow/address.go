package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// PathRoot is the semantic root identity of a path. It deliberately separates
// symbol identity from textual roots so deterministic key strings cannot
// accidentally make a variable symbol and a placeholder/root name equivalent.
type PathRoot struct {
	kind   pathRootKind
	symbol cfg.SymbolID
	name   string
}

type pathRootKind uint8

const (
	pathRootInvalid pathRootKind = iota
	pathRootSymbol
	pathRootName
)

// SymbolPathRoot builds a symbol-rooted path identity.
func SymbolPathRoot(sym cfg.SymbolID) (PathRoot, bool) {
	if sym == 0 {
		return PathRoot{}, false
	}
	return PathRoot{kind: pathRootSymbol, symbol: sym}, true
}

// NamedPathRoot builds a non-symbol root identity for placeholders, globals, or
// boundary-relative roots that cannot be represented by CFG symbols.
func NamedPathRoot(name string) (PathRoot, bool) {
	if name == "" {
		return PathRoot{}, false
	}
	return PathRoot{kind: pathRootName, name: name}, true
}

// PathRootOfPath extracts the stable root identity from a source path.
func PathRootOfPath(path constraint.Path) (PathRoot, bool) {
	if path.Symbol != 0 {
		return SymbolPathRoot(path.Symbol)
	}
	return NamedPathRoot(path.Root)
}

// Symbol returns the CFG symbol for symbol-rooted identities.
func (r PathRoot) Symbol() (cfg.SymbolID, bool) {
	return r.symbol, r.kind == pathRootSymbol && r.symbol != 0
}

// Name returns the textual root for non-symbol identities.
func (r PathRoot) Name() (string, bool) {
	return r.name, r.kind == pathRootName && r.name != ""
}

// Equal reports semantic root identity equality.
func (r PathRoot) Equal(other PathRoot) bool {
	return r.kind == other.kind && r.symbol == other.symbol && r.name == other.name
}

func (r PathRoot) isValid() bool {
	switch r.kind {
	case pathRootSymbol:
		return r.symbol != 0
	case pathRootName:
		return r.name != ""
	default:
		return false
	}
}

// PathSuffix is the structured member/index suffix of an address. It is a
// value object around constraint.Segment so prefix/overlap logic stays
// structural instead of falling back to string-prefix tests.
type PathSuffix struct {
	segments []constraint.Segment
}

// PathSuffixOfSegments builds a defensive structured suffix value.
func PathSuffixOfSegments(segments []constraint.Segment) PathSuffix {
	return pathSuffixOfOwnedSegments(cloneAddressSegments(segments))
}

// pathSuffixOfOwnedSegments wraps a segment slice already owned by this package.
// PathSuffix has no mutating operations; callers must not pass shared slices.
func pathSuffixOfOwnedSegments(segments []constraint.Segment) PathSuffix {
	if len(segments) == 0 {
		return PathSuffix{}
	}
	return PathSuffix{segments: segments}
}

// Segments returns a defensive copy of the suffix segments.
func (s PathSuffix) Segments() []constraint.Segment {
	return cloneAddressSegments(s.segments)
}

// Len returns the number of member/index steps in the suffix.
func (s PathSuffix) Len() int { return len(s.segments) }

// Equal reports structural suffix equality.
func (s PathSuffix) Equal(other PathSuffix) bool {
	if len(s.segments) != len(other.segments) {
		return false
	}
	for i := range s.segments {
		if s.segments[i] != other.segments[i] {
			return false
		}
	}
	return true
}

// HasPrefix reports whether prefix is this suffix or one of its ancestors.
func (s PathSuffix) HasPrefix(prefix PathSuffix) bool {
	if len(prefix.segments) > len(s.segments) {
		return false
	}
	for i := range prefix.segments {
		if s.segments[i] != prefix.segments[i] {
			return false
		}
	}
	return true
}

// RemainderAfterPrefix returns the suffix below prefix when prefix is this
// suffix or one of its ancestors.
func (s PathSuffix) RemainderAfterPrefix(prefix PathSuffix) ([]constraint.Segment, bool) {
	if !s.HasPrefix(prefix) {
		return nil, false
	}
	return cloneAddressSegments(s.segments[len(prefix.segments):]), true
}

// Overlaps reports whether one suffix is a prefix of the other.
func (s PathSuffix) Overlaps(other PathSuffix) bool {
	return s.HasPrefix(other) || other.HasPrefix(s)
}

// KeySuffix returns the deterministic private suffix encoding used by existing
// map carriers.
func (s PathSuffix) KeySuffix() string {
	return constraint.FormatSegments(s.segments)
}

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

// Key returns the deterministic point-local map key for this address. Unlike
// StableAddress.Key, this preserves the SSA version carried by the source path.
func (a LocalAddress) Key() constraint.PathKey {
	if a.path.IsEmpty() {
		return ""
	}
	return a.path.Key()
}

// Stable returns the version-insensitive address for must-fact domains.
func (a LocalAddress) Stable() (StableAddress, bool) {
	return StableAddressOfPath(a.path)
}

// SameVersion reports exact point-local identity.
func (a LocalAddress) SameVersion(b LocalAddress) bool {
	return a.path.Equal(b.path)
}

// StableAddress is the version-insensitive path identity used by finite must
// fact domains. Its string key is only an internal deterministic map key; all
// semantic relation checks go through this structured value.
type StableAddress struct {
	root   PathRoot
	suffix PathSuffix
}

// StableAddressOfPath lowers a source path to the stable identity used by
// point-local must facts. Symbol identity wins; otherwise Root identifies
// placeholders and boundary-relative paths.
func StableAddressOfPath(path constraint.Path) (StableAddress, bool) {
	root, ok := PathRootOfPath(path)
	if !ok {
		return StableAddress{}, false
	}
	return stableAddressOfRootAndSuffix(root, pathSuffixOfOwnedSegments(cloneAddressSegments(path.Segments)))
}

// SameStablePath reports whether two source paths name the same runtime path in
// the stable must-fact identity space. SSA versions are ignored.
func SameStablePath(a, b constraint.Path) bool {
	left, leftOK := StableAddressOfPath(a)
	right, rightOK := StableAddressOfPath(b)
	return leftOK && rightOK && left.Equal(right)
}

// StableAddressOfSymbol builds a stable symbol-rooted address.
func StableAddressOfSymbol(sym cfg.SymbolID, segments []constraint.Segment) (StableAddress, bool) {
	root, ok := SymbolPathRoot(sym)
	if !ok {
		return StableAddress{}, false
	}
	return stableAddressOfRootAndSuffix(root, PathSuffixOfSegments(segments))
}

func stableAddressOfSymbolOwnedSegments(sym cfg.SymbolID, segments []constraint.Segment) (StableAddress, bool) {
	root, ok := SymbolPathRoot(sym)
	if !ok {
		return StableAddress{}, false
	}
	return stableAddressOfRootAndSuffix(root, pathSuffixOfOwnedSegments(segments))
}

// StableAddressOfRoot builds a stable root-name address for placeholders and
// boundary paths that are not represented by CFG symbols.
func StableAddressOfRoot(root string, segments []constraint.Segment) (StableAddress, bool) {
	pathRoot, ok := NamedPathRoot(root)
	if !ok {
		return StableAddress{}, false
	}
	return stableAddressOfRootAndSuffix(pathRoot, PathSuffixOfSegments(segments))
}

func stableAddressOfRootOwnedSegments(root string, segments []constraint.Segment) (StableAddress, bool) {
	pathRoot, ok := NamedPathRoot(root)
	if !ok {
		return StableAddress{}, false
	}
	return stableAddressOfRootAndSuffix(pathRoot, pathSuffixOfOwnedSegments(segments))
}

// StableAddressOfRootAndSuffix builds a stable address from normalized root and
// suffix vocabulary.
func StableAddressOfRootAndSuffix(root PathRoot, suffix PathSuffix) (StableAddress, bool) {
	return stableAddressOfRootAndSuffix(root, suffix)
}

func stableAddressOfRootAndSuffix(root PathRoot, suffix PathSuffix) (StableAddress, bool) {
	if !root.isValid() {
		return StableAddress{}, false
	}
	return StableAddress{root: root, suffix: suffix}, true
}

// StableAddressFromCanonicalKey parses a stored fact key back into structured
// identity. It accepts only keys produced by StableAddress.Key.
func StableAddressFromCanonicalKey(key constraint.PathKey) (StableAddress, bool) {
	if key == "" {
		return StableAddress{}, false
	}
	if sym, segments, ok := ParseSymbolPathKey(key); ok {
		return stableAddressOfSymbolOwnedSegments(sym, segments)
	}
	if _, _, ok := parseLegacyConstraintSymbolPathKey(key); ok {
		return StableAddress{}, false
	}
	root, suffix, ok := splitStableRootKey(string(key))
	if !ok {
		return StableAddress{}, false
	}
	segments, ok := parseSymbolPathSegments(suffix)
	if !ok {
		return StableAddress{}, false
	}
	return stableAddressOfRootOwnedSegments(root, segments)
}

func parseLegacyConstraintSymbolPathKey(key constraint.PathKey) (cfg.SymbolID, []constraint.Segment, bool) {
	s := string(key)
	if len(s) < 4 || s[0] != 's' || s[1] != 'y' || s[2] != 'm' {
		return 0, nil, false
	}
	i := 3
	var n uint64
	for i < len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + uint64(c-'0')
		i++
	}
	if n == 0 {
		return 0, nil, false
	}
	if i < len(s) && s[i] == '@' {
		i++
		for i < len(s) {
			c := s[i]
			if c < '0' || c > '9' {
				break
			}
			i++
		}
	}
	segments, ok := parseSymbolPathSegments(s[i:])
	if !ok {
		return 0, nil, false
	}
	return cfg.SymbolID(n), segments, true
}

// StablePathKey returns the deterministic key for path's stable address.
func StablePathKey(path constraint.Path) constraint.PathKey {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return ""
	}
	return addr.Key()
}

// PathIdentityKey returns the canonical identity key for proof and queue
// lookups. Resolved paths use stable address identity; placeholders and other
// non-addressable paths fall back to their structural path key.
func PathIdentityKey(path constraint.Path) constraint.PathKey {
	if key := StablePathKey(path); key != "" {
		return key
	}
	return path.Key()
}

// StableAddressKeyHasPrefix reports whether key denotes prefix or a descendant
// of prefix in the stable-address key space. It is the key-domain form of
// StableAddress.HasPrefix and avoids projecting segment slices for callers that
// only need subtree membership.
func StableAddressKeyHasPrefix(key constraint.PathKey, prefix StableAddress) bool {
	if key == "" || !prefix.root.isValid() {
		return false
	}
	root, suffix, ok := stableAddressKeyRootAndSuffix(key)
	if !ok || !root.Equal(prefix.root) {
		return false
	}
	return stableSuffixStringHasPrefix(suffix, prefix.suffix.KeySuffix())
}

func stableAddressKeyRootAndSuffix(key constraint.PathKey) (PathRoot, string, bool) {
	s := string(key)
	if sym, suffix, ok := splitSymbolPathKey(s); ok {
		root, rootOK := SymbolPathRoot(sym)
		return root, suffix, rootOK
	}
	if _, _, ok := parseLegacyConstraintSymbolPathKey(key); ok {
		return PathRoot{}, "", false
	}
	root, suffix, ok := splitStableRootKey(s)
	if !ok {
		return PathRoot{}, "", false
	}
	pathRoot, ok := NamedPathRoot(root)
	return pathRoot, suffix, ok
}

func splitSymbolPathKey(s string) (cfg.SymbolID, string, bool) {
	if len(s) < 2 || s[0] != 's' {
		return 0, "", false
	}
	i := 1
	var n uint64
	for i < len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + uint64(c-'0')
		i++
	}
	if n == 0 || n > uint64(^cfg.SymbolID(0)) {
		return 0, "", false
	}
	if i < len(s) && s[i] != '.' && s[i] != '[' {
		return 0, "", false
	}
	return cfg.SymbolID(n), s[i:], true
}

func stableSuffixStringHasPrefix(suffix, prefix string) bool {
	if prefix == "" {
		return true
	}
	if suffix == prefix {
		return true
	}
	if len(suffix) <= len(prefix) || suffix[:len(prefix)] != prefix {
		return false
	}
	switch suffix[len(prefix)] {
	case '.', '[':
		return true
	default:
		return false
	}
}

// Key returns the internal deterministic key for map/set carriers.
func (a StableAddress) Key() constraint.PathKey {
	if !a.root.isValid() {
		return ""
	}
	if sym, ok := a.root.Symbol(); ok {
		return SymbolPathKey(sym, a.suffix.segments)
	}
	root, _ := a.root.Name()
	return constraint.Path{Root: root, Segments: a.suffix.segments}.Key()
}

// Path returns a path view for consumers that still operate on constraint.Path.
func (a StableAddress) Path() (constraint.Path, bool) {
	if !a.root.isValid() {
		return constraint.Path{}, false
	}
	segments := a.suffix.Segments()
	if sym, ok := a.root.Symbol(); ok {
		return constraint.Path{Symbol: sym, Segments: segments}, true
	}
	root, _ := a.root.Name()
	return constraint.Path{Root: root, Segments: segments}, true
}

// RootIdentity returns the structured semantic root.
func (a StableAddress) RootIdentity() PathRoot {
	return a.root
}

// Symbol returns the root symbol when this is a symbol address.
func (a StableAddress) Symbol() (cfg.SymbolID, bool) {
	return a.root.Symbol()
}

// Root returns the root name when this is a non-symbol root address.
func (a StableAddress) Root() (string, bool) {
	return a.root.Name()
}

// Segments returns a defensive copy of the address suffix.
func (a StableAddress) Segments() []constraint.Segment {
	return a.suffix.Segments()
}

// Suffix returns the structured path suffix.
func (a StableAddress) Suffix() PathSuffix {
	return PathSuffixOfSegments(a.suffix.segments)
}

// Append returns the descendant address reached by appending structured
// member/index segments to this address.
func (a StableAddress) Append(segments []constraint.Segment) (StableAddress, bool) {
	if !a.root.isValid() {
		return StableAddress{}, false
	}
	if len(segments) == 0 {
		return stableAddressOfRootAndSuffix(a.root, a.suffix)
	}
	next := make([]constraint.Segment, len(a.suffix.segments)+len(segments))
	copy(next, a.suffix.segments)
	copy(next[len(a.suffix.segments):], segments)
	return stableAddressOfRootAndSuffix(a.root, pathSuffixOfOwnedSegments(next))
}

// Equal reports stable identity equality.
func (a StableAddress) Equal(b StableAddress) bool {
	return a.root.Equal(b.root) && a.suffix.Equal(b.suffix)
}

// HasPrefix reports whether prefix is this address or an ancestor of it.
func (a StableAddress) HasPrefix(prefix StableAddress) bool {
	return a.sameRoot(prefix) && a.suffix.HasPrefix(prefix.suffix)
}

// RemainderAfterPrefix returns the member/index suffix below prefix when prefix
// is this address or one of its ancestors.
func (a StableAddress) RemainderAfterPrefix(prefix StableAddress) ([]constraint.Segment, bool) {
	if !a.sameRoot(prefix) {
		return nil, false
	}
	return a.suffix.RemainderAfterPrefix(prefix.suffix)
}

// Overlaps reports whether two addresses share a root and one path is a prefix
// of the other.
func (a StableAddress) Overlaps(b StableAddress) bool {
	return a.HasPrefix(b) || b.HasPrefix(a)
}

func (a StableAddress) sameRoot(b StableAddress) bool {
	return a.root.isValid() && a.root.Equal(b.root)
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
