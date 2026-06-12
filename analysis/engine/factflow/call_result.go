package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CallResultValue describes a fixed product value for one call return slot.
type CallResultValue struct {
	index int
	value product.Value
}

// CallResultValueSet groups fixed call result values emitted at the same CFG
// call point.
type CallResultValueSet struct {
	values []CallResultValue
}

// NewCallResultValue creates a fixed call result value fact.
func NewCallResultValue(index int, value product.Value) CallResultValue {
	return CallResultValue{index: index, value: value}
}

// NewCallResultValueSet creates a fixed call result value set.
func NewCallResultValueSet(values ...CallResultValue) CallResultValueSet {
	return CallResultValueSet{values: copyCallResultValueSlice(values)}
}

// Index returns the result slot index.
func (v CallResultValue) Index() int { return v.index }

// Value returns the product value for the result slot.
func (v CallResultValue) Value() product.Value { return v.value }

// Values returns the fixed call result values in deterministic order.
func (s CallResultValueSet) Values() []CallResultValue {
	return copyCallResultValueSlice(s.values)
}

func (s CallResultValueSet) copy() CallResultValueSet {
	return CallResultValueSet{values: copyCallResultValueSlice(s.values)}
}

func copyCallResultValueMap(in map[cfg.Point]CallResultValueSet) map[cfg.Point]CallResultValueSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]CallResultValueSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func copyCallResultValueSlice(in []CallResultValue) []CallResultValue {
	if len(in) == 0 {
		return nil
	}
	out := make([]CallResultValue, len(in))
	copy(out, in)
	return out
}
