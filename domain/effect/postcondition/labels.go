// Package postcondition defines signature-owned facts that hold when a call
// returns normally.
package postcondition

import (
	"fmt"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
	"github.com/wippyai/go-lua/internal/hash"
)

const (
	NormalReturnRefinementKind = "postcondition.normalReturnRefinement"
	PresentKind                = "present"
	AbsentKind                 = "absent"
)

var _ effect.Label = NormalReturnRefinement{}

// RefinementCount is the size of the closed refinement vocabulary.
const RefinementCount = 2

// Refinements is the refinement catalog. It is the one enumeration of the
// members this package owns, so a consumer that visits, serializes, or declares
// every refinement projects it instead of restating the member list. The
// catalog is returned by value and costs no allocation to range over.
func Refinements() [RefinementCount]Refinement {
	return [RefinementCount]Refinement{Present{}, Absent{}}
}

// RefinementForKind returns the catalog member that declares kind. The kind
// spellings come from the members' own Kind methods, so the name a refinement
// answers to and the name it is found by are one statement and cannot drift: a
// refinement the catalog holds is readable by construction.
func RefinementForKind(kind string) (Refinement, bool) {
	if kind == "" {
		return nil, false
	}
	for _, refinement := range Refinements() {
		if refinement.Kind() == kind {
			return refinement, true
		}
	}
	return nil, false
}

// Refinement is one argument refinement that can be declared by a function
// signature postcondition.
type Refinement interface {
	refinement()
	Kind() string
	String() string
	Equals(other Refinement) bool
	Hash() uint64
}

// NormalizeRefinement returns the canonical value form for a refinement. It is
// the ownership boundary for pointer/value refinement spellings used by
// manifests and effect lowering; callers should not duplicate type switches for
// the concrete refinement variants.
func NormalizeRefinement(refinement Refinement) (Refinement, bool) {
	switch r := refinement.(type) {
	case Present:
		return r, true
	case *Present:
		if r != nil {
			return Present{}, true
		}
	case Absent:
		return r, true
	case *Absent:
		if r != nil {
			return Absent{}, true
		}
	}
	return nil, false
}

// RefinementIsNil reports whether refinement is nil, including supported typed
// nil pointer spellings. Unsupported non-nil refinements return false so callers
// can keep distinct "missing" and "unsupported" diagnostics.
func RefinementIsNil(refinement Refinement) bool {
	if refinement == nil {
		return true
	}
	switch r := refinement.(type) {
	case *Present:
		return r == nil
	case *Absent:
		return r == nil
	default:
		return false
	}
}

// Present refines the target argument to be present after normal return.
type Present struct{}

func (Present) refinement()  {}
func (Present) Kind() string { return PresentKind }
func (Present) String() string {
	return PresentKind
}
func (Present) Equals(other Refinement) bool {
	switch o := other.(type) {
	case Present:
		return true
	case *Present:
		return o != nil
	default:
		return false
	}
}
func (Present) Hash() uint64 {
	return hash.FnvString("postcondition.Present")
}

// Absent refines the target argument to be nil/absent after normal return.
type Absent struct{}

func (Absent) refinement()  {}
func (Absent) Kind() string { return AbsentKind }
func (Absent) String() string {
	return AbsentKind
}
func (Absent) Equals(other Refinement) bool {
	switch o := other.(type) {
	case Absent:
		return true
	case *Absent:
		return o != nil
	default:
		return false
	}
}
func (Absent) Hash() uint64 {
	return hash.FnvString("postcondition.Absent")
}

// NormalReturnRefinement declares that Target is refined if the call returns
// normally.
type NormalReturnRefinement struct {
	Target     effect.ParamRef
	Refinement Refinement
}

func (NormalReturnRefinement) CapabilityID() string {
	return capability.PostconditionNormalReturnRefinement
}
func (NormalReturnRefinement) Kind() string { return NormalReturnRefinementKind }
func (n NormalReturnRefinement) String() string {
	return fmt.Sprintf("normal_return_refine(%s, %s)", n.Target, refinementString(n.Refinement))
}
func (n NormalReturnRefinement) Equals(other effect.Label) bool {
	o, ok := effect.NormalizeLabel(other).(NormalReturnRefinement)
	if !ok {
		return false
	}
	return n.Target.Index == o.Target.Index && refinementEquals(n.Refinement, o.Refinement)
}
func (n NormalReturnRefinement) Hash() uint64 {
	h := hash.FnvString(capability.PostconditionNormalReturnRefinement)
	h = hash.MixHash(h, hash.FnvString(fmt.Sprintf("target:%d", n.Target.Index)))
	if n.Refinement != nil {
		h = hash.MixHash(h, n.Refinement.Hash())
	}
	return h
}

func refinementEquals(a, b Refinement) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equals(b)
}

func refinementString(refinement Refinement) string {
	if refinement == nil {
		return "<nil>"
	}
	return refinement.String()
}
