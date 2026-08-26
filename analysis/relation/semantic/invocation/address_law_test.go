package invocation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

type invocationLawFixture struct {
	fence        binding.Fence
	foreignFence binding.Fence
	address      InvocationAddress
	changed      InvocationAddress
	row          model.RowID
}

func newInvocationLawFixture(t *testing.T) invocationLawFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(identity.ContentID{1})
	if !ok {
		t.Fatal("owner")
	}
	schema, ok := model.IssueSchemaID(owner, identity.ContentID{2})
	if !ok {
		t.Fatal("schema")
	}
	fence, ok := binding.NewFence(schema, identity.MountID{3}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	foreignFence, ok := binding.NewFence(schema, identity.MountID{4}, identity.Generation(1))
	if !ok {
		t.Fatal("foreign fence")
	}
	relation, ok := model.IssueRelationID(owner, identity.ContentID{5})
	if !ok {
		t.Fatal("relation")
	}
	row, ok := model.IssueRowID(relation, identity.ContentID{6})
	if !ok {
		t.Fatal("row")
	}
	changedRow, ok := model.IssueRowID(relation, identity.ContentID{7})
	if !ok {
		t.Fatal("changed row")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issuer")
	}
	scope, ok := issuer.IssueScope(identity.ContentID{8})
	if !ok {
		t.Fatal("scope")
	}
	address := lawAddress(t, scope, row)
	changed := lawAddress(t, scope, changedRow)
	return invocationLawFixture{fence: fence, foreignFence: foreignFence, address: address, changed: changed, row: row}
}

func lawAddress(t *testing.T, scope binding.ScopeToken, row model.RowID) InvocationAddress {
	t.Helper()
	tuple, ok := NewTupleSources([]model.RowID{row})
	if !ok {
		t.Fatal("tuple")
	}
	child, ok := NewSourceVector([]TupleSources{tuple})
	if !ok {
		t.Fatal("child")
	}
	address, ok := New(scope, []SourceVector{child})
	if !ok {
		t.Fatal("address")
	}
	return address
}

func TestInvocationAddressIsStructuralDefensiveAndOrdered(t *testing.T) {
	fixture := newInvocationLawFixture(t)
	if !fixture.address.Same(fixture.address) || fixture.address.Compare(fixture.address) != 0 {
		t.Fatal("address was not reflexive")
	}
	if fixture.address.Same(fixture.changed) || fixture.address.Compare(fixture.changed) == 0 {
		t.Fatal("source-row structure was not retained in comparison")
	}
}

func TestInvocationAddressCopiesInputStorage(t *testing.T) {
	fixture := newInvocationLawFixture(t)
	sourceRows := []model.RowID{fixture.row}
	tupleCopy, ok := NewTupleSources(sourceRows)
	if !ok {
		t.Fatal("source tuple")
	}
	childCopy, ok := NewSourceVector([]TupleSources{tupleCopy})
	if !ok {
		t.Fatal("source child")
	}
	copyAddress, ok := New(fixture.address.Scope(), []SourceVector{childCopy})
	if !ok {
		t.Fatal("source address")
	}
	sourceRows[0] = model.RowID{}
	if !copyAddress.Same(fixture.address) {
		t.Fatal("constructor retained caller row storage")
	}
	children := fixture.address.Children()
	if len(children) != 1 {
		t.Fatal("children")
	}
	tuple, ok := children[0].At(0)
	if !ok {
		t.Fatal("tuple")
	}
	rows := tuple.Rows()
	if len(rows) != 1 {
		t.Fatal("rows")
	}
	rows[0] = model.RowID{}
	if !fixture.address.Same(fixture.address) {
		t.Fatal("accessor exposed address storage")
	}
	original, ok := fixture.address.ChildAt(0)
	if !ok {
		t.Fatal("original child")
	}
	originalTuple, ok := original.At(0)
	if !ok {
		t.Fatal("original tuple")
	}
	originalRow, ok := originalTuple.At(0)
	if !ok || originalRow != fixture.row {
		t.Fatal("accessor exposed address storage")
	}
}
