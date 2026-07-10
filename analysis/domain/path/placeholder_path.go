package path

import "strconv"

// NewPlaceholder creates a placeholder path for function refinement parameters.
// Index 0 creates $0, index 1 creates $1, etc.
//
// Example:
//
//	p0 := NewPlaceholder(0) // $0 (first parameter)
//	p1 := NewPlaceholder(1) // $1 (second parameter)
func NewPlaceholder(index int) Path {
	if index < 0 {
		return Path{}
	}
	return Path{Root: "$" + strconv.Itoa(index)}
}

// IsPlaceholder returns true if this path is a placeholder (used in function refinements).
// Placeholders have Symbol == 0 and Root matching $0, $1, etc.
func (p Path) IsPlaceholder() bool {
	return p.Symbol == 0 && p.PlaceholderIndex() >= 0
}

// ValidateSymbol checks the symbol-first identity invariant.
// For resolved paths (non-empty, non-placeholder), Symbol must be non-zero.
// Returns empty string if valid, error message if invalid.
// Used for debug-mode assertions.
func (p Path) ValidateSymbol() string {
	if p.IsEmpty() {
		return "" // empty paths are valid
	}
	if p.IsPlaceholder() {
		return "" // placeholders are valid with Symbol=0
	}
	if p.Symbol == 0 && p.Root != "" {
		return "resolved path missing Symbol: " + p.Root
	}
	return ""
}

// PlaceholderIndex returns the parameter index if this path's root is a placeholder ($0, $1, etc).
// Returns -1 if not a placeholder.
func (p Path) PlaceholderIndex() int {
	return PlaceholderIndexFromString(p.Root)
}

// Substitute replaces placeholder roots with actual argument paths.
// Only paths with Symbol == 0 and Root matching $0, $1, etc. are substituted.
// Returns (result, true) on success, (empty, false) if placeholder out of range or arg path empty.
func (p Path) Substitute(args []Path) (Path, bool) {
	if p.IsEmpty() {
		return Path{}, false
	}

	if !p.IsPlaceholder() {
		return p, true
	}

	idx := p.PlaceholderIndex()

	if idx >= len(args) {
		return Path{}, false
	}

	argPath := args[idx]
	if argPath.IsEmpty() {
		return Path{}, false
	}

	result := Path{Root: argPath.Root, Symbol: argPath.Symbol, Version: argPath.Version}
	result.Segments = append(result.Segments, argPath.Segments...)
	result.Segments = append(result.Segments, p.Segments...)

	return result, true
}
