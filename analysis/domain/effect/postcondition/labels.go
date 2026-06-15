// Package postcondition defines signature-owned facts that hold when a call
// returns normally.
package postcondition

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/internal/hash"
)

const (
	NormalReturnRefinementKind = "postcondition.normalReturnRefinement"
	PresentKind                = "present"
	AbsentKind                 = "absent"
)

var _ effect.Label = NormalReturnRefinement{}

// Refinement is one argument refinement that can be declared by a function
// signature postcondition.
type Refinement interface {
	refinement()
	Kind() string
	String() string
	Equals(other Refinement) bool
	Hash() uint64
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

func (NormalReturnRefinement) EffectLabel() {}
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
	h := hash.FnvString("postcondition.NormalReturnRefinement")
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
