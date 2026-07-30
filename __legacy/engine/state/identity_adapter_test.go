package state

import (
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
)

func TestWrappedMustSetDomainPreservesEqualOperandIdentity(t *testing.T) {
	relation := StoreRelation{
		Source: pathaddr.StateKey("sym1@1.source"),
		Into:   pathaddr.StateKey("sym2@1.destination"),
	}
	left := storeRelationLane{mustSetLane[StoreRelation]{
		values: map[StoreRelation]struct{}{relation: {}},
	}}
	right := storeRelationLane{mustSetLane[StoreRelation]{
		values: map[StoreRelation]struct{}{relation: {}},
	}}
	domain := storeRelationDomain()
	if domain.Same == nil {
		t.Fatal("wrapped domain dropped the persistent Same hook")
	}
	if domain.Same(left, right) {
		t.Fatal("independently allocated equal lanes reported the same representation")
	}
	if joined := domain.Join(left, right); !domain.Same(joined, left) {
		t.Fatal("equal must-set join did not reuse its left wrapper operand")
	}
}
