package specimen_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// harness issues the mounted ABI vocabulary one specimen law needs: one owner,
// one relation, one denominator and one invocation scope. It is deliberately
// the smallest authenticated world in which a binding can be invoked at all.
type harness struct {
	owner       model.OwnerID
	relation    model.RelationID
	key         model.KeyID
	denominator model.DenominatorRef
	schemaID    model.SchemaID
	fence       binding.Fence
	issuer      binding.Issuer
	scope       binding.ScopeToken
	refusal     model.RefusalID
	rows        []model.RowID
	witness     binding.DenominatorWitness
}

func content(t testing.TB, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relbindgen-specimen-law", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func newHarness(t testing.TB, rowLabels ...string) harness {
	t.Helper()
	keys := make([]identity.ContentID, 0, len(rowLabels))
	for _, label := range rowLabels {
		keys = append(keys, content(t, label))
	}
	return newHarnessKeyed(t, keys)
}

// newHarnessKeyed mounts a denominator whose rows carry owner-issued content
// the caller already holds, so a law can address the exact rows a real owner
// names.
func newHarnessKeyed(t testing.TB, rowKeys []identity.ContentID) harness {
	t.Helper()
	owner, ok := model.IssueOwnerID(content(t, "owner"))
	if !ok {
		t.Fatal("issue owner")
	}
	relation, ok := model.IssueRelationID(owner, content(t, "relation"))
	if !ok {
		t.Fatal("issue relation")
	}
	key, ok := model.IssueKeyID(relation, content(t, "key"))
	if !ok {
		t.Fatal("issue key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("issue denominator")
	}
	schemaID, ok := model.IssueSchemaID(owner, content(t, "schema"))
	if !ok {
		t.Fatal("issue schema")
	}
	scopeID, ok := model.IssueScopeID(owner, content(t, "scope"))
	if !ok {
		t.Fatal("issue scope")
	}
	refusal, ok := model.IssueRefusalID(owner, content(t, "refusal/binding"))
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
	scope, ok := issuer.IssueScope(scopeID, content(t, "scope/witness"))
	if !ok {
		t.Fatal("issue scope token")
	}
	value := harness{
		owner: owner, relation: relation, key: key, denominator: denominator,
		schemaID: schemaID, fence: fence, issuer: issuer, scope: scope, refusal: refusal,
	}
	for index, key := range rowKeys {
		row, rowOK := model.IssueRowID(relation, key)
		if !rowOK {
			t.Fatalf("issue row %d", index)
		}
		value.rows = append(value.rows, row)
	}
	membership, ok := binding.NewMembershipView(relation, value.rows)
	if !ok {
		t.Fatal("membership view")
	}
	value.witness, ok = issuer.IssueDenominator(denominator, membership, content(t, "denominator/witness"))
	if !ok {
		t.Fatal("denominator witness")
	}
	return value
}

func (value harness) column(t testing.TB, label string) model.ColumnID {
	t.Helper()
	column, ok := model.IssueColumnID(value.relation, content(t, label))
	if !ok {
		t.Fatalf("issue column %q", label)
	}
	return column
}

func (value harness) typeID(t testing.TB, label string) model.TypeID {
	t.Helper()
	typeID, ok := model.IssueTypeID(value.owner, content(t, label))
	if !ok {
		t.Fatalf("issue type %q", label)
	}
	return typeID
}

func (value harness) operationID(t testing.TB, label string) model.OperationID {
	t.Helper()
	operation, ok := model.IssueOperationID(value.owner, content(t, label))
	if !ok {
		t.Fatalf("issue operation %q", label)
	}
	return operation
}

func scalarInput(t testing.TB, relation model.RelationID, column model.ColumnID, typeID model.TypeID, denominator model.DenominatorRef) signature.Input {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	return signature.Input{Relation: relation, Column: column, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator}
}

func completeInput(t testing.TB, relation model.RelationID, column model.ColumnID, typeID model.TypeID, denominator model.DenominatorRef) signature.Input {
	t.Helper()
	delivery, ok := signature.NewCompleteSpanDelivery(denominator.Key())
	if !ok {
		t.Fatal("complete delivery")
	}
	return signature.Input{Relation: relation, Column: column, Type: typeID, Presence: signature.AllowMissing, Delivery: delivery, Denominator: denominator}
}

func (value harness) seal(t testing.TB, label string, inputs []signature.Input, outputs []signature.Output, cardinality model.Cardinality, codes ...outcome.Code) signature.Signature {
	t.Helper()
	set, ok := outcome.NewSet(codes...)
	if !ok {
		t.Fatal("outcome set")
	}
	sealed, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: value.operationID(t, label), Version: 1},
		Fence:       signature.Fence{Owner: value.owner, Schema: value.schemaID},
		Inputs:      inputs,
		Outputs:     outputs,
		Authority:   signature.OutputAuthority{Denominator: value.denominator},
		Cardinality: cardinality,
		Outcomes:    set,
	})
	if !ok {
		t.Fatalf("seal %q", label)
	}
	return sealed
}

func (value harness) cell(t testing.TB, column model.ColumnID, row model.RowID, typeID model.TypeID, token binding.ValueToken) binding.Cell {
	t.Helper()
	address, ok := value.issuer.IssueCell(value.witness, value.scope, column, row)
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

func (value harness) absentCell(t testing.TB, column model.ColumnID, row model.RowID, typeID model.TypeID) binding.Cell {
	t.Helper()
	address, ok := value.issuer.IssueCell(value.witness, value.scope, column, row)
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

func scalarSlot(t testing.TB, cell binding.Cell) binding.Slot {
	t.Helper()
	slot, ok := binding.NewScalarSlot(cell)
	if !ok {
		t.Fatal("scalar slot")
	}
	return slot
}

func spanSlot(t testing.TB, cells []binding.Cell) binding.Slot {
	t.Helper()
	slot, ok := binding.NewSpanSlot(cells)
	if !ok {
		t.Fatal("span slot")
	}
	return slot
}

func (value harness) frame(t testing.TB, slots ...binding.Slot) binding.Frame {
	t.Helper()
	frame, ok := binding.NewFrame(value.scope, slots...)
	if !ok {
		t.Fatal("construct frame")
	}
	return frame
}

func (value harness) buffer(t testing.TB, operation signature.Signature) *binding.ProposalBuffer {
	t.Helper()
	buffer, ok := binding.NewProposalBuffer(operation, value.fence, value.witness, value.scope)
	if !ok {
		t.Fatal("proposal buffer")
	}
	return &buffer
}

func (value harness) worker(t testing.TB, factory binding.Factory, operation signature.Signature) binding.Worker {
	t.Helper()
	admitted, ok := binding.Admit(factory, operation)
	if !ok {
		t.Fatal("admit binding")
	}
	worker, ok := admitted.NewWorker(value.fence)
	if !ok {
		t.Fatal("solve-local worker")
	}
	return worker
}
