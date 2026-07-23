package heapidentity

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestObjectDomainEqualJoinReusesOperandRepresentation(t *testing.T) {
	reg := standard.Registry()
	members := map[keyspace.Key]product.Value{
		fieldSuffixKey(t, keyspace.New(), "value"): presentValue(reg),
	}
	left := NewTableObject(TableObjectConfig{Root: presentValue(reg), StaticMembers: members})
	right := CloneObject(left)
	domain := ObjectDomain(reg)
	if !domain.Equal(left, right) || domain.Same(left, right) {
		t.Fatal("test requires equal objects with distinct persistent map representations")
	}
	if joined := domain.Join(left, right); !domain.Same(joined, left) {
		t.Fatal("equal heap-object join did not reuse its left operand representation")
	}
}
