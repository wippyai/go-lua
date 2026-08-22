// Package population owns the closed execution populations shared by schema
// declaration and the engine.  It deliberately contains no authored role
// keys, query-family names, or domain callbacks: those are resolved by the
// declaration surface before this token crosses a boundary.
package population

// Kind is the resolved population of one query family.
//
// The zero value is intentionally invalid.  A family must carry one of the
// two admitted populations; callers must not infer a population from a
// family key or from the presence of a row.
type Kind uint8

const (
	Invalid Kind = iota
	SelectedPoint
	Observation
)

// Available reports whether kind is one of the closed populations.
func (kind Kind) Available() bool {
	return kind == SelectedPoint || kind == Observation
}
