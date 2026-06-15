package path

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Path identifies a runtime value through its syntax-facing access path.
//
// Paths are the syntax-facing access identity used by Lua extraction,
// refinement placeholders, and domain projections. Point-local abstract state
// keys must be produced by a state/key resolver with an explicit nonzero SSA
// version; finite must-fact addresses deliberately ignore versions.
//
// Symbol provides lexical binding identity; Root is optional for display when Symbol is set.
// When Symbol is non-zero, it is the primary identity for the path root.
// Placeholder paths ($0, $1, etc.) use Root only with Symbol=0 and are
// substituted with concrete paths when applying function refinements at call sites.
//
// Examples:
//   - {Root: "x", Symbol: 5}: Variable x with symbol ID 5
//   - {Root: "x", Symbol: 5, Segments: [{Kind: segment.SegmentField, Name: "name"}]}: x.name
//   - {Root: "$0", Symbol: 0}: Placeholder for first function parameter
type Path struct {
	Root     string            // Display/root identity (optional for symbol paths, required for placeholders)
	Symbol   symbol.ID         // Symbol identity (0 if unresolved/placeholder)
	Segments []segment.Segment // Field/index access suffix
	Version  int               // Optional syntax/projection SSA version; point-state keys require an explicit resolver version.
}

// PathKey is a stable string key for map usage.
type PathKey string
