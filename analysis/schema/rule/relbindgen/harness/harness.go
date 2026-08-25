// Package harness issues the mounted ABI vocabulary a binding law needs.
//
// It is deliberately the smallest authenticated world in which a binding can
// be invoked at all: one owner, one relation, one denominator, one invocation
// scope, and the rows a law addresses. It names no domain, so every axis's
// bindings are proven against the same mounted world.
package harness

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
)

// Place is one mounted world.
type Place struct {
	Owner       model.OwnerID
	Relation    model.RelationID
	Key         model.KeyID
	Denominator model.DenominatorRef
	SchemaID    model.SchemaID
	Fence       binding.Fence
	Issuer      binding.Issuer
	Scope       binding.ScopeToken
	Refusal     model.RefusalID
	Rows        []model.RowID
	Witness     binding.DenominatorWitness
}

// Content derives one owner-issued content identity from a stable label.
func Content(t testing.TB, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relbindgen-binding-law", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

// New mounts a world whose rows are named by the given labels.
func New(t testing.TB, rowLabels ...string) Place {
	t.Helper()
	keys := make([]identity.ContentID, 0, len(rowLabels))
	for _, label := range rowLabels {
		keys = append(keys, Content(t, label))
	}
	return NewKeyed(t, keys)
}

// NewKeyed mounts a denominator whose rows carry owner-issued content the
// caller already holds, so a law can address the exact rows a real owner
// names.
func NewKeyed(t testing.TB, rowKeys []identity.ContentID) Place {
	t.Helper()
	owner, ok := model.IssueOwnerID(Content(t, "owner"))
	if !ok {
		t.Fatal("issue owner")
	}
	relation, ok := model.IssueRelationID(owner, Content(t, "relation"))
	if !ok {
		t.Fatal("issue relation")
	}
	key, ok := model.IssueKeyID(relation, Content(t, "key"))
	if !ok {
		t.Fatal("issue key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("issue denominator")
	}
	schemaID, ok := model.IssueSchemaID(owner, Content(t, "schema"))
	if !ok {
		t.Fatal("issue schema")
	}
	refusal, ok := model.IssueRefusalID(owner, Content(t, "refusal/binding"))
	if !ok {
		t.Fatal("issue refusal")
	}
	fence, ok := binding.NewFence(schemaID, identity.MountID{7}, identity.Generation(1))
	if !ok {
		t.Fatal("mount fence")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("token issuer")
	}
	scope, ok := issuer.IssueScope(Content(t, "scope/witness"))
	if !ok {
		t.Fatal("issue scope token")
	}
	place := Place{
		Owner: owner, Relation: relation, Key: key, Denominator: denominator,
		SchemaID: schemaID, Fence: fence, Issuer: issuer, Scope: scope, Refusal: refusal,
	}
	for index, rowKey := range rowKeys {
		row, rowOK := model.IssueRowID(relation, rowKey)
		if !rowOK {
			t.Fatalf("issue row %d", index)
		}
		place.Rows = append(place.Rows, row)
	}
	membership, ok := binding.NewMembershipView(relation, place.Rows)
	if !ok {
		t.Fatal("membership view")
	}
	place.Witness, ok = issuer.IssueDenominator(denominator, membership, Content(t, "denominator/witness"))
	if !ok {
		t.Fatal("denominator witness")
	}
	return place
}

// Column issues one owner-issued column of the mounted relation.
func (place Place) Column(t testing.TB, label string) model.ColumnID {
	t.Helper()
	column, ok := model.IssueColumnID(place.Relation, Content(t, label))
	if !ok {
		t.Fatalf("issue column %q", label)
	}
	return column
}

// TypeID issues one owner-issued semantic type.
func (place Place) TypeID(t testing.TB, label string) model.TypeID {
	t.Helper()
	typeID, ok := model.IssueTypeID(place.Owner, Content(t, label))
	if !ok {
		t.Fatalf("issue type %q", label)
	}
	return typeID
}

// OperationID issues one owner-issued operation identity.
func (place Place) OperationID(t testing.TB, label string) model.OperationID {
	t.Helper()
	operation, ok := model.IssueOperationID(place.Owner, Content(t, label))
	if !ok {
		t.Fatalf("issue operation %q", label)
	}
	return operation
}

// ScalarInput declares one scalar input of a sealed signature.
func ScalarInput(t testing.TB, relation model.RelationID, column model.ColumnID, typeID model.TypeID, denominator model.DenominatorRef) signature.Input {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	return signature.Input{Relation: relation, Column: column, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator}
}

// CompleteInput declares one complete-span input of a sealed signature.
func CompleteInput(t testing.TB, relation model.RelationID, column model.ColumnID, typeID model.TypeID, denominator model.DenominatorRef) signature.Input {
	t.Helper()
	delivery, ok := signature.NewCompleteSpanDelivery(denominator.Key())
	if !ok {
		t.Fatal("complete delivery")
	}
	return signature.Input{Relation: relation, Column: column, Type: typeID, Presence: signature.AllowMissing, Delivery: delivery, Denominator: denominator}
}

// Seal freezes one operation contract in the mounted world.
func (place Place) Seal(t testing.TB, label string, inputs []signature.Input, outputs []signature.Output, cardinality model.Cardinality, codes ...outcome.Code) signature.Signature {
	t.Helper()
	set, ok := outcome.NewSet(codes...)
	if !ok {
		t.Fatal("outcome set")
	}
	sealed, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: place.OperationID(t, label), Version: 1},
		Fence:       signature.Fence{Owner: place.Owner, Schema: place.SchemaID},
		Inputs:      inputs,
		Outputs:     outputs,
		Authority:   signature.OutputAuthority{Denominator: place.Denominator},
		Cardinality: cardinality,
		Outcomes:    set,
	})
	if !ok {
		t.Fatalf("seal %q", label)
	}
	return sealed
}

// Cell delivers one present cell of the mounted relation.
func (place Place) Cell(t testing.TB, column model.ColumnID, row model.RowID, typeID model.TypeID, token binding.ValueToken) binding.Cell {
	t.Helper()
	address, ok := place.Issuer.IssueCell(place.Witness, place.Scope, column, row)
	if !ok {
		t.Fatal("issue cell address")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	cell, ok := binding.NewCell(address, typeID, token, presence)
	if !ok {
		t.Fatal("construct cell")
	}
	return cell
}

// AbsentCell delivers one proven-absent cell of the mounted relation.
func (place Place) AbsentCell(t testing.TB, column model.ColumnID, row model.RowID, typeID model.TypeID) binding.Cell {
	t.Helper()
	address, ok := place.Issuer.IssueCell(place.Witness, place.Scope, column, row)
	if !ok {
		t.Fatal("issue cell address")
	}
	presence, ok := model.NewPresence(model.ProvenAbsent)
	if !ok {
		t.Fatal("presence")
	}
	cell, ok := binding.NewCell(address, typeID, binding.ValueToken{}, presence)
	if !ok {
		t.Fatal("construct absent cell")
	}
	return cell
}

// ScalarSlot delivers one scalar frame slot.
func ScalarSlot(t testing.TB, cell binding.Cell) binding.Slot {
	t.Helper()
	slot, ok := binding.NewScalarSlot(cell)
	if !ok {
		t.Fatal("scalar slot")
	}
	return slot
}

// SpanSlot delivers one span frame slot.
func SpanSlot(t testing.TB, cells []binding.Cell) binding.Slot {
	t.Helper()
	slot, ok := binding.NewSpanSlot(cells)
	if !ok {
		t.Fatal("span slot")
	}
	return slot
}

// Frame assembles one invocation frame under the mounted scope.
func (place Place) Frame(t testing.TB, slots ...binding.Slot) binding.Frame {
	t.Helper()
	frame, ok := binding.NewFrame(place.Scope, slots...)
	if !ok {
		t.Fatal("construct frame")
	}
	return frame
}

// Buffer opens one proposal buffer for a sealed operation.
func (place Place) Buffer(t testing.TB, operation signature.Signature) *binding.ProposalBuffer {
	t.Helper()
	buffer, ok := binding.NewProposalBuffer(operation, place.Fence, place.Witness, place.Scope)
	if !ok {
		t.Fatal("proposal buffer")
	}
	return &buffer
}

// Worker admits one factory and opens its solve-local worker.
func (place Place) Worker(t testing.TB, factory binding.Factory, operation signature.Signature) binding.Worker {
	t.Helper()
	admitted, ok := binding.Admit(factory, operation)
	if !ok {
		t.Fatal("admit binding")
	}
	worker, ok := admitted.NewWorker(place.Fence)
	if !ok {
		t.Fatal("solve-local worker")
	}
	return worker
}

// NewColumn installs one owner column over its own solve-local store.
func NewColumn[T any](t testing.TB, typeID model.TypeID, label string, reserve int) *relbindgen.Column[T] {
	t.Helper()
	store, ok := relbindgen.NewStore[T](Content(t, label), reserve)
	if !ok {
		t.Fatalf("store %q", label)
	}
	column, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatalf("column %q", label)
	}
	return column
}
