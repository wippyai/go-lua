package operationplan

import "github.com/wippyai/go-lua/analysis/engine/factflow"

// AttachMetatableOperation is the immutable semantic identity of one
// canonical setmetatable(table, metatable) operation. The constructor is
// deliberately owned by body preparation: consumers receive typed operands
// and never reconstruct authority from a callee spelling.
type AttachMetatableOperation struct {
	table     factflow.ValueSource
	metatable factflow.ValueSource
}

// NewAttachMetatableOperation seals the exact two sources of one binding-
// resolved canonical setmetatable call.
func NewAttachMetatableOperation(table, metatable factflow.ValueSource) (AttachMetatableOperation, bool) {
	if !table.Valid() || !metatable.Valid() || table.Kind == factflow.ValueSourceUnknown || metatable.Kind == factflow.ValueSourceUnknown {
		return AttachMetatableOperation{}, false
	}
	return AttachMetatableOperation{table: table, metatable: metatable}, true
}

func (o AttachMetatableOperation) Table() factflow.ValueSource     { return o.table }
func (o AttachMetatableOperation) Metatable() factflow.ValueSource { return o.metatable }

func (o AttachMetatableOperation) valid() bool {
	return o.table.Valid() && o.metatable.Valid() &&
		o.table.Kind != factflow.ValueSourceUnknown && o.metatable.Kind != factflow.ValueSourceUnknown
}

func (o AttachMetatableOperation) equal(other AttachMetatableOperation) bool {
	return o.valid() && other.valid() &&
		factflow.ValueSourceEqual(o.table, other.table) &&
		factflow.ValueSourceEqual(o.metatable, other.metatable)
}
