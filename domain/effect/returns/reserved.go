package returns

import (
	"fmt"

	"github.com/wippyai/go-lua/domain/constraint/expr"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
)

// ReturnLength is reserved return metadata. It is audited by capability
// descriptors, but not actively lowered into return semantics.
type ReturnLength struct {
	ReturnIndex int
	Length      expr.Expr
}

func (ReturnLength) CapabilityID() string { return capability.ReturnsReturnLength }
func (r ReturnLength) String() string {
	return fmt.Sprintf("ret[%d].len = %s", r.ReturnIndex, r.Length)
}
func (r ReturnLength) Equals(other effect.Label) bool {
	if o, ok := effect.NormalizeLabel(other).(ReturnLength); ok {
		return r.ReturnIndex == o.ReturnIndex && expr.ExprEquals(r.Length, o.Length)
	}
	return false
}

// CorrelatedReturn is reserved high-risk return metadata. Stdlib signatures
// must not declare it while lowering ignores it.
type CorrelatedReturn struct {
	Indices []int
}

func (CorrelatedReturn) CapabilityID() string { return capability.ReturnsCorrelatedReturn }
func (c CorrelatedReturn) String() string {
	return fmt.Sprintf("correlated_return(%v)", c.Indices)
}
func (c CorrelatedReturn) Equals(other effect.Label) bool {
	o, ok := effect.NormalizeLabel(other).(CorrelatedReturn)
	if !ok || len(c.Indices) != len(o.Indices) {
		return false
	}
	for i := range c.Indices {
		if c.Indices[i] != o.Indices[i] {
			return false
		}
	}
	return true
}
