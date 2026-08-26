package operand

import "github.com/wippyai/go-lua/analysis/engine/internal/facts/support"

// SelectedCell is one delivered member of a Selection. Value has already been
// through the read's sealed substitutions, so Present is false only under a
// contract that genuinely reads evidence provenance. Region is the member's own
// authenticated support row, which is what a routed write stages against.
type SelectedCell[V any] struct {
	Value   V
	Present bool
	Tag     uint64
	Region  support.Mask
}
