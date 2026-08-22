package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

// TestCallResultRequirementTreatsAbsentSlotsAsNonAdmission exercises the
// sealed call-result plan with the two authored call shapes that share the
// same plain-call row. A discarded call has no CallResultSlot and therefore
// must be a normal non-admission; the owner-issued value slot admits the
// otherwise identical call and emits its summary request. The law crosses
// OpRead's typed optional absence and OpRequirePresent's false-proof path
// through Rows.Seal and Evaluate rather than invoking either opcode directly.
func TestCallResultRequirementTreatsAbsentSlotsAsNonAdmission(t *testing.T) {
	table := scheduleTable(t)
	plan, planOK := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{{
		Family:      "occurrence/call",
		Requirement: programissuance.RequirementCallResult,
		Form:        programissuance.FormCallSummary,
		Rule:        "law/rule/call-result",
		Writes:      "law/axis/call-placement",
	}})
	if !planOK {
		t.Fatal("call-result execution plan refused sealed declarations")
	}

	for _, test := range []struct {
		name          string
		withValueSlot bool
		wantRequests  int
	}{
		{name: "discarded-call", wantRequests: 0},
		{name: "valued-call", withValueSlot: true, wantRequests: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			occurrenceID := lawID(20)
			// The canonical occurrence/call relation is keyed by the authored
			// call identity; a call occurrence owns that same identity.
			callID := occurrenceID
			occurrence, occurrenceOK := programschema.NewOccurrence(
				programschema.OccurrenceCall, occurrenceID, identity.ContentID{}, 0,
				0, 0, 0, 0, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
			)
			if !occurrenceOK {
				t.Fatal("call occurrence unavailable")
			}
			call, callOK := programschema.NewCall(
				callID, lawID(22), lawID(23), lawID(24), lawID(25), lawID(26), lawID(27),
				lawID(28), lawID(29), identity.ContentID{}, identity.ContentID{}, identity.ContentID{},
				programschema.CallFormPlain, 0, 0, 0, 1, 0, 0, false, false,
			)
			if !callOK {
				t.Fatal("plain call unavailable")
			}
			publication := &programpublication.Publication{
				Occurrences: []programschema.Occurrence{occurrence},
				Calls:       []programschema.Call{call},
			}
			if test.withValueSlot {
				slot, slotOK := programschema.NewDerivedCallResultSlot(
					callID, 0, programschema.CallResultSlotSourceCallValue,
					programschema.CallResultSlotConsumerStructural, lawID(30), 0, lawID(31),
				)
				if !slotOK {
					t.Fatal("owner-issued call result slot unavailable")
				}
				publication.CallResultSlots = []programschema.CallResultSlot{slot}
			}
			builder := programissuance.NewBuilder()
			if !builder.AddGeometry(programschema.OccurrenceCall, occurrenceID, nil, []identity.ContentID{lawID(32)}) {
				t.Fatal("call occurrence geometry refused")
			}
			rows, rowsOK := builder.Seal(table, publication)
			if !rowsOK {
				t.Fatal("sealed call occurrence rows refused")
			}
			requests, evaluated := Evaluate(plan, rows)
			if !evaluated {
				t.Fatal("call-result requirement evaluation refused normal non-admission")
			}
			if len(requests) != test.wantRequests {
				t.Fatalf("call-result requests = %d, want %d", len(requests), test.wantRequests)
			}
		})
	}
}

// TestModuleLoadRequirementUsesProgramImportWitness states the module-load
// placement boundary in owner terms. Plain unary result geometry is not a
// module load; only the same call joined to Program's authenticated
// ModuleImport row may issue the rule.
func TestModuleLoadRequirementUsesProgramImportWitness(t *testing.T) {
	table := scheduleTable(t)
	plan, planOK := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{{
		Family:      "occurrence/call",
		Requirement: programissuance.RequirementModuleLoadCall,
		Form:        programissuance.FormCallSummary,
		Rule:        "law/rule/module-load",
		Writes:      "law/axis/module-load",
	}})
	if !planOK {
		t.Fatal("module-load execution plan refused sealed declarations")
	}

	for _, test := range []struct {
		name       string
		withImport bool
		want       int
	}{
		{name: "ordinary-unary-result"},
		{name: "authenticated-module-import", withImport: true, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			callID := lawID(40)
			occurrence, occurrenceOK := programschema.NewOccurrence(
				programschema.OccurrenceCall, callID, identity.ContentID{}, 0,
				0, 0, 0, 0, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
			)
			call, callOK := programschema.NewCall(
				callID, lawID(41), lawID(42), lawID(43), lawID(44), lawID(45), lawID(46),
				lawID(47), lawID(48), identity.ContentID{}, identity.ContentID{}, identity.ContentID{},
				programschema.CallFormPlain, 0, 0, 0, 1, 0, 0, false, false,
			)
			slot, slotOK := programschema.NewDerivedCallResultSlot(
				callID, 0, programschema.CallResultSlotSourceCallValue,
				programschema.CallResultSlotConsumerStructural, lawID(49), 0, lawID(50),
			)
			if !occurrenceOK || !callOK || !slotOK {
				t.Fatal("canonical call rows unavailable")
			}
			publication := &programpublication.Publication{
				Occurrences:     []programschema.Occurrence{occurrence},
				Calls:           []programschema.Call{call},
				CallResultSlots: []programschema.CallResultSlot{slot},
			}
			if test.withImport {
				imported, importOK := programschema.NewModuleImport(lawID(51), callID, identity.ContentID{}, 0, 1, false)
				if !importOK {
					t.Fatal("canonical module import unavailable")
				}
				publication.ModuleImports = []programschema.ModuleImport{imported}
			}
			builder := programissuance.NewBuilder()
			if !builder.AddGeometry(programschema.OccurrenceCall, callID, nil, []identity.ContentID{lawID(52)}) {
				t.Fatal("call occurrence geometry refused")
			}
			rows, rowsOK := builder.Seal(table, publication)
			if !rowsOK {
				t.Fatal("sealed module-load rows refused")
			}
			requests, evaluated := Evaluate(plan, rows)
			if !evaluated {
				t.Fatal("module-load requirement evaluation refused")
			}
			if len(requests) != test.want {
				t.Fatalf("module-load requests = %d, want %d", len(requests), test.want)
			}
		})
	}
}

// TestTailTransferRequirementUsesOwnerIssuedResultRelation pins the only
// admissible tail-transfer row to the canonical CallResultSlot relation. A
// fixed Values member with the same value/cell coordinates is not a tail
// receipt, so it is a normal non-admission; the exact owner-issued tail slot
// admits one request.
func TestTailTransferRequirementUsesOwnerIssuedResultRelation(t *testing.T) {
	table := scheduleTable(t)
	relation, relationOK := table.Entry(programissuance.RelationTailTransferResult, schemaissuance.KindRelation)
	if !relationOK {
		t.Fatal("tail-transfer result relation unavailable")
	}
	point := lawID(1)
	occurrenceID := lawID(2)
	valueID := lawID(3)
	cellID := lawID(4)
	callID := lawID(5)

	for name, sourceKind := range map[string]programschema.CallResultSlotSourceKind{
		"fixed-values-member": programschema.CallResultSlotSourceValue,
		"owner-issued-tail":   programschema.CallResultSlotSourceValuesTail,
	} {
		t.Run(name, func(t *testing.T) {
			occurrence, occurrenceOK := programschema.NewOccurrence(
				programschema.OccurrenceStorageBindTransfer, occurrenceID, identity.ContentID{}, 0,
				0, 1, 0, 3, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
			)
			if !occurrenceOK {
				t.Fatal("storage-bind-transfer occurrence unavailable")
			}
			pointRow, pointOK := programschema.NewOccurrencePoint(point)
			inputs := make([]programschema.OccurrenceInput, 0, 3)
			for _, input := range []identity.ContentID{lawID(6), valueID, cellID} {
				row, rowOK := programschema.NewOccurrenceInput(input)
				if !rowOK {
					t.Fatal("occurrence input unavailable")
				}
				inputs = append(inputs, row)
			}
			slot, slotOK := programschema.NewDerivedCallResultSlot(
				callID, 0, sourceKind, programschema.CallResultSlotConsumerCell, cellID, 0, valueID,
			)
			if !occurrenceOK || !pointOK || !slotOK {
				t.Fatal("canonical owner rows unavailable")
			}
			publication := &programpublication.Publication{
				Occurrences:      []programschema.Occurrence{occurrence},
				OccurrencePoints: []programschema.OccurrencePoint{pointRow},
				OccurrenceInputs: inputs,
				CallResultSlots:  []programschema.CallResultSlot{slot},
			}
			builder := programissuance.NewBuilder()
			if !builder.AddGeometry(programschema.OccurrenceStorageBindTransfer, occurrenceID, nil, []identity.ContentID{point}) {
				t.Fatal("owner-issued occurrence geometry refused")
			}
			rows, rowsOK := builder.Seal(table, publication)
			if !rowsOK {
				t.Fatal("canonical Program rows refused sealing")
			}
			targets, followOK := rows.Follow(rowsAt(rows, programissuance.RowOccurrence, 0, t), relation)
			if !followOK {
				t.Fatal("canonical tail-transfer relation refused follow")
			}
			if len(targets) != 1 {
				t.Fatalf("owner-issued tail relation join targets = %d, want one canonical slot", len(targets))
			}
			plan, planOK := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{{
				Family:      programissuanceFamilyStorageBindTransfer,
				Requirement: programissuance.RequirementTailTransfer,
				Form:        programissuance.FormBaseNoneAllowEmpty,
				Rule:        "law/rule/tail-transfer",
				Writes:      "law/axis/placement",
			}})
			if !planOK {
				t.Fatal("tail-transfer subscription refused sealing")
			}
			requests, evaluated := Evaluate(plan, rows)
			if !evaluated {
				t.Fatal("tail-transfer requirement evaluation refused a non-admission")
			}
			wantRequests := 0
			if sourceKind == programschema.CallResultSlotSourceValuesTail {
				wantRequests = 1
			}
			if len(requests) != wantRequests {
				t.Fatalf("tail-transfer requests = %d, want %d", len(requests), wantRequests)
			}
		})
	}
}

const programissuanceFamilyStorageBindTransfer schema.Key = "occurrence/storage-bind-transfer"

func rowsAt(rows programissuance.Rows, space schema.Key, index int, t *testing.T) programissuance.Row {
	t.Helper()
	row, ok := rows.At(space, index)
	if !ok {
		t.Fatalf("row %s[%d] unavailable", space, index)
	}
	return row
}

func lawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0], id[1] = 0xa7, value
	return id
}
