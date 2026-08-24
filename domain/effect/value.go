package effect

import "github.com/wippyai/go-lua/domain/effect/internal/valuecore"

// Value is effect's Fact type: Bottom, an immutable sparse atom set, or Top.
// Every other axis defines its Value/Fact type directly at the axis root;
// effect's data layout is defined in valuecore so the axis root can name it
// without importing the factor algebra (which reaches domain/static) or the
// owner package (which holds DenseCoordinate). The algebra that constructs
// and reduces over Value stays in domain/effect/factor.
type Value = valuecore.Value

// Atom is one opaque, algebra-local effect-template identity. It rides the
// same valuecore split as Value, for the same reason.
type Atom = valuecore.Atom
