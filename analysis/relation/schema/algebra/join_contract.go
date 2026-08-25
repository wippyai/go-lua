package algebra

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// JoinContract declares one oriented equijoin over logical column vectors.
// Both vectors must be non-empty and have equal arity; the checker owns those
// validity rules. Parent membership, address identity, occurrence, and
// correspondence are ordinary relation columns and therefore use this same
// contract. Key IDs and physical index choice belong to mount planning, not
// logical algebra.
type JoinContract struct {
	leftColumns  []model.ColumnID
	rightColumns []model.ColumnID
}

// NewJoinContract constructs an oriented equijoin declaration. It copies the
// authored vectors and deliberately performs no typing or authority checks.
func NewJoinContract(leftColumns, rightColumns []model.ColumnID) JoinContract {
	return JoinContract{leftColumns: cloneColumns(leftColumns), rightColumns: cloneColumns(rightColumns)}
}

// LeftColumns returns the ordered left vector.
func (contract JoinContract) LeftColumns() []model.ColumnID {
	return cloneColumns(contract.leftColumns)
}

// RightColumns returns the ordered right vector.
func (contract JoinContract) RightColumns() []model.ColumnID {
	return cloneColumns(contract.rightColumns)
}

func (contract JoinContract) digestBytes() []byte {
	parts := appendColumns(nil, contract.leftColumns)
	return appendColumns(parts, contract.rightColumns)
}
