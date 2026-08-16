package table

import (
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// MetatableUnconstrained marks a record metatable axis that is unconstrained
// by a source-level annotation. Subtype treats it as the top of the metatable
// axis (anything narrows to it), and method lookup ignores it (it carries no
// methods of its own). This is distinct from Unknown, which would otherwise
// leak through specialAccessType-driven field/method queries.
var MetatableUnconstrained typ.Type = metatableUnconstrainedType{}

// IsMetatableUnconstrained reports whether t is the sentinel used by source-level
// record annotations that omit a metatable constraint.
func IsMetatableUnconstrained(t typ.Type) bool {
	_, ok := t.(metatableUnconstrainedType)
	return ok
}

// metatableUnconstrainedType reports the sentinel kind as Unknown so existing
// generic dispatch (visitors, kind-based switches) treats it as unresolved,
// while the value identity itself is unique so subtype/method lookup can
// special-case it via IsMetatableUnconstrained.
type metatableUnconstrainedType struct{}

func (metatableUnconstrainedType) Kind() kind.Kind { return kind.Unknown }
func (metatableUnconstrainedType) String() string  { return "<metatable-unconstrained>" }
func (metatableUnconstrainedType) Hash() uint64    { return uint64(kind.Unknown) ^ 0x9E3779B97F4A7C15 }
func (metatableUnconstrainedType) Equals(o typ.Type) bool {
	_, ok := o.(metatableUnconstrainedType)
	return ok
}
