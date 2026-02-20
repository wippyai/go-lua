// Package pathkey provides canonical path key generation for flow analysis.
//
// In SSA form, each variable has multiple versions at different program points.
// This package maps constraint paths (symbol + field segments) to canonical
// versioned keys that uniquely identify the variable incarnation.
//
// Key format: sym<SymbolID>@<VersionID><segments>
// Example: sym42@3.field[0] refers to symbol 42, version 3, field "field", index 0.
package pathkey

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// VersionedGraph provides SSA version lookup for symbols at CFG points.
//
// In SSA form, each variable has multiple versions corresponding to different
// assignments. VisibleVersion returns the version of a symbol that is "live"
// at a particular program point - the most recent assignment that dominates
// the point.
type VersionedGraph interface {
	// VisibleVersion returns the SSA version of sym that is visible at point p.
	// Returns a zero Version if no version is visible (symbol not in scope).
	VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version
}

// Resolver provides canonical path key generation from constraint paths.
//
// All path-to-key conversions in the flow solver must go through the resolver
// to ensure consistent SSA versioning. The resolver encapsulates the logic for:
//
//   - Querying the CFG for visible SSA versions
//   - Building canonical key strings in the "sym<ID>@<Ver><segments>" format
//   - Handling placeholder paths (used in function effects)
//   - Validating that paths have resolvable versions
//
// Using the resolver ensures that the same path at the same point always
// produces the same key, enabling correct value lookup and constraint matching.
type Resolver struct {
	graph VersionedGraph
}

// NewResolver creates a resolver bound to a versioned graph.
//
// The graph must provide SSA version lookup for all symbols that will be
// resolved. Typically this is a cfg.VersionedGraph from CFG construction.
func NewResolver(g VersionedGraph) *Resolver {
	return &Resolver{graph: g}
}

// KeyAt returns the canonical key for a path at a CFG point.
// This is the ONLY method that should be used for path→key conversion.
//
// Canonical key format: sym<SymbolID>@<VersionID><segments>
// Example: sym42@3.field[0]
//
// Rules:
//   - Symbol=0 paths (placeholders) use Root as-is
//   - Requires valid SSA version for non-placeholder paths
//   - Returns empty string if path is empty or version unavailable
func (r *Resolver) KeyAt(p cfg.Point, path constraint.Path) constraint.PathKey {
	if path.IsEmpty() {
		return ""
	}

	// Placeholders ($0, $1) use Root directly
	if path.IsPlaceholder() {
		return buildPlaceholderKey(path.Root, path.Segments)
	}

	// Non-placeholder paths require Symbol
	if path.Symbol == 0 {
		return ""
	}

	ver := r.versionAt(p, path)
	if ver.IsZero() {
		return ""
	}

	return r.buildKey(path.Symbol, ver.ID, path.Segments)
}

// KeyAtVersion returns the canonical key using an explicit version ID.
// Use when version is already known (e.g., from phi operands).
func (r *Resolver) KeyAtVersion(sym cfg.SymbolID, versionID int, segments []constraint.Segment) constraint.PathKey {
	if sym == 0 {
		return ""
	}
	return r.buildKey(sym, versionID, segments)
}

// buildKey constructs the canonical key string from components.
//
// The format is: sym<SymbolID>@<VersionID><segments>
// Example: sym42@3.field[0]
//
// This format ensures keys are:
//   - Unique per (symbol, version, path suffix) tuple
//   - Parseable back to components via ParseKey
//   - Sortable in a meaningful order (by symbol, then version)
func (r *Resolver) buildKey(sym cfg.SymbolID, versionID int, segments []constraint.Segment) constraint.PathKey {
	var b strings.Builder
	b.WriteString("sym")
	writeUint(&b, uint64(sym))
	b.WriteByte('@')
	writeInt(&b, versionID)
	appendSegments(&b, segments)
	return constraint.PathKey(b.String())
}

func buildPlaceholderKey(root string, segments []constraint.Segment) constraint.PathKey {
	if len(segments) == 0 {
		return constraint.PathKey(root)
	}
	var b strings.Builder
	b.WriteString(root)
	appendSegments(&b, segments)
	return constraint.PathKey(b.String())
}

func appendSegments(b *strings.Builder, segments []constraint.Segment) {
	for _, seg := range segments {
		switch seg.Kind {
		case constraint.SegmentField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case constraint.SegmentIndexString:
			writeQuotedSegmentIndex(b, seg.Name)
		case constraint.SegmentIndexInt:
			b.WriteByte('[')
			writeInt(b, seg.Index)
			b.WriteByte(']')
		}
	}
}

func writeQuotedSegmentIndex(b *strings.Builder, key string) {
	b.WriteString("[\"")
	for i := 0; i < len(key); i++ {
		switch key[i] {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(key[i])
		default:
			b.WriteByte(key[i])
		}
	}
	b.WriteString("\"]")
}

func writeUint(b *strings.Builder, value uint64) {
	var scratch [20]byte
	out := strconv.AppendUint(scratch[:0], value, 10)
	_, _ = b.Write(out)
}

func writeInt(b *strings.Builder, value int) {
	var scratch [20]byte
	out := strconv.AppendInt(scratch[:0], int64(value), 10)
	_, _ = b.Write(out)
}

// ParseKey extracts components from a canonical key string.
//
// This is the inverse of buildKey. Given "sym42@3.field", returns:
//   - SymbolID: 42
//   - VersionID: 3
//   - Suffix: ".field"
//   - OK: true
//
// For keys without a version (missing @), versionID is 0.
// For keys that don't start with "sym", returns (0, 0, "", false).
//
// The suffix includes any field or index path after the version number.
func ParseKey(key constraint.PathKey) (cfg.SymbolID, int, string, bool) {
	s := string(key)
	if !strings.HasPrefix(s, "sym") {
		return 0, 0, "", false
	}
	s = s[3:] // skip "sym"

	// Find symbol end (@ or . or [ or end)
	symEnd := 0
	for symEnd < len(s) && s[symEnd] != '@' && s[symEnd] != '.' && s[symEnd] != '[' {
		symEnd++
	}
	if symEnd == 0 {
		return 0, 0, "", false
	}
	sym, err := strconv.ParseUint(s[:symEnd], 10, 64)
	if err != nil {
		return 0, 0, "", false
	}

	rest := s[symEnd:]
	versionID := 0
	suffix := ""

	if len(rest) > 0 && rest[0] == '@' {
		rest = rest[1:]
		verEnd := 0
		for verEnd < len(rest) && rest[verEnd] != '.' && rest[verEnd] != '[' {
			verEnd++
		}
		if verEnd == 0 {
			return 0, 0, "", false
		}
		v, err := strconv.Atoi(rest[:verEnd])
		if err != nil || v < 0 {
			return 0, 0, "", false
		}
		versionID = v
		suffix = rest[verEnd:]
	} else {
		suffix = rest
	}

	if suffix != "" && ParseSuffix(suffix) == nil {
		return 0, 0, "", false
	}

	return cfg.SymbolID(sym), versionID, suffix, true
}

// KeySymbol extracts the symbol ID from a canonical key.
//
// This is a convenience wrapper around ParseKey for when only the symbol
// is needed. Returns 0 if the key is not a valid canonical key.
func KeySymbol(key constraint.PathKey) cfg.SymbolID {
	sym, _, _, ok := ParseKey(key)
	if !ok {
		return 0
	}
	return sym
}

// KeysShareSymbol returns true if both keys reference the same symbol.
//
// This is used to check if two keys are versions of the same variable,
// regardless of version number or path suffix. Useful for phi node handling
// and constraint propagation across versions.
func KeysShareSymbol(a, b constraint.PathKey) bool {
	symA, _, _, okA := ParseKey(a)
	symB, _, _, okB := ParseKey(b)
	return okA && okB && symA == symB
}

// versionAt returns the visible SSA version for a path at point p.
//
// Queries the versioned graph to find which version of the path's symbol
// is live at the given point. Returns a zero version if the graph is nil
// or the symbol is not in scope at the point.
func (r *Resolver) versionAt(p cfg.Point, path constraint.Path) cfg.Version {
	if path.Symbol == 0 {
		return cfg.Version{}
	}
	if path.Version != 0 {
		return cfg.Version{Root: path.Root, Symbol: path.Symbol, ID: path.Version}
	}
	if r.graph == nil {
		return cfg.Version{}
	}
	return r.graph.VisibleVersion(p, path.Symbol)
}

// VersionAtSym returns the visible SSA version for a symbol at point p.
//
// This is a convenience method when you have a SymbolID rather than a
// full Path. Equivalent to calling VersionAt with Path{Symbol: sym}.
func (r *Resolver) VersionAtSym(p cfg.Point, sym cfg.SymbolID) cfg.Version {
	if r.graph == nil || sym == 0 {
		return cfg.Version{}
	}
	return r.graph.VisibleVersion(p, sym)
}

// HasVersion returns true if a valid SSA version exists for the path at point p.
//
// A path has a version if:
//   - It is not empty
//   - It is not a placeholder ($0, $1, etc.)
//   - It has a Symbol
//   - The symbol has a non-zero version at point p
//
// This is used to check if a path can be resolved before attempting resolution.
func (r *Resolver) HasVersion(p cfg.Point, path constraint.Path) bool {
	if path.IsEmpty() || path.IsPlaceholder() {
		return false
	}
	if path.Symbol == 0 {
		return false
	}
	return !r.versionAt(p, path).IsZero()
}
