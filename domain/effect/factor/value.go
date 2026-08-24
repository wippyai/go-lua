package factor

import "github.com/wippyai/go-lua/domain/effect"

// Value and Atom are effect's Fact types, named at the axis root
// (domain/effect) and aliased here. The algebra that constructs and reduces
// over them lives in this package; their data layout lives in
// domain/effect/internal/valuecore, which this package reaches for the
// authenticated constructors (NewValue, NewAtom).
type Value = effect.Value
type Atom = effect.Atom
