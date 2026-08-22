package freshresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the Value-owned Link-lane transfer from one authenticated Call
// application to its fixed Target fresh-result Value coordinate.
type HotRule struct {
	implementation *valueowner.RuleImplementation[valuedomain.FreshResultCall]
	callRead       engine.Read[engine.OrderedCells[calldomain.Value]]
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
}

// BindHot binds the exact Call read and transformed Value carry/write lane.
// The Call owner supplies only the dispatch fact; the Value schema supplies
// the admitted fresh-result operand, write coordinate, Recent fact, and Age
// transition.
func BindHot(fragment *SchemaFragment, values *valueowner.HotOwner, calls *callowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || values == nil || calls == nil || values.Schema() == nil || !values.Schema().Valid() || calls.Algebra() == nil || !calls.Algebra().Valid() || !fragment.semantic.Available() || !fragment.transform.Available() {
		return nil, false
	}
	if !values.Schema().LinkOwner().Matches(calls.Algebra().LinkOwner()) {
		return nil, false
	}
	rule := &HotRule{values: values, calls: calls}
	schema := values.Schema()
	implementation, bound := valueowner.BindCarryRule(values, fragment.slot, fragment.carry, fragment.write, engine.HotRuleSpec[valuedomain.Value, valuedomain.FreshResultCall]{
		OperandContent: func(row valuedomain.FreshResultCall) (valuedomain.FreshResultCall, [32]byte, bool) {
			if !schema.OwnsFreshResultCall(row) {
				return valuedomain.FreshResultCall{}, [32]byte{}, false
			}
			id, idOK := row.KeyID()
			if !idOK || !id.Available() {
				return valuedomain.FreshResultCall{}, [32]byte{}, false
			}
			return row, [32]byte(id), true
		},
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[valuedomain.Value, valuedomain.FreshResultCall]{
		Apply: func(row valuedomain.FreshResultCall, prior valuedomain.Value) (valuedomain.Value, bool) {
			key, keyOK := row.Key()
			if !keyOK {
				return valuedomain.Value{}, false
			}
			return schema.Age(prior, key)
		},
	}, func(row valuedomain.FreshResultCall) (uint64, bool) {
		coordinate, coordinateOK := row.Coordinate()
		index, indexOK := schema.CoordinateIndex(coordinate)
		return uint64(index), coordinateOK && indexOK
	})
	if !bound || implementation == nil {
		return nil, false
	}
	callRead, callOK := valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(row valuedomain.FreshResultCall) (uint64, bool) {
		application, applicationOK := row.ApplicationID()
		if !applicationOK {
			return 0, false
		}
		key, keyOK := calls.Algebra().KeyForApplicationID(application)
		index, indexOK := calls.Algebra().KeyIndex(key)
		return uint64(index), keyOK && indexOK && key.IsApplication()
	})
	if !callOK {
		return nil, false
	}
	rule.implementation, rule.callRead = implementation, callRead
	return rule, true
}

func (rule *HotRule) fold(frame engine.Frame[valuedomain.Value, valuedomain.FreshResultCall]) engine.RuleResult[valuedomain.Value] {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil {
		return engine.RuleResult[valuedomain.Value]{}
	}
	operand, operandOK := engine.Operand(frame)
	if !operandOK || !rule.values.Schema().OwnsFreshResultCall(operand) {
		return engine.RuleResult[valuedomain.Value]{}
	}
	cells, readOK := engine.ReadValue(frame, rule.callRead)
	if !readOK || cells.Count() != 1 {
		return engine.RuleResult[valuedomain.Value]{}
	}
	callFact, present, available := cells.At(0)
	if !available {
		return engine.RuleResult[valuedomain.Value]{}
	}
	if !present {
		return engine.NoCandidate(frame)
	}
	if !validCallFact(callFact) {
		return engine.RuleResult[valuedomain.Value]{}
	}
	if callFact.HasOpaqueAlternative() {
		return engine.Staged(frame, rule.values.Schema().Top())
	}
	if callFact.KnownTargetCount() == 0 {
		return engine.NoCandidate(frame)
	}
	operation, operationOK := operand.Operation()
	if !operationOK {
		return engine.RuleResult[valuedomain.Value]{}
	}
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK {
			return engine.RuleResult[valuedomain.Value]{}
		}
		targetOperation, hasOperation := target.Operation()
		// Function-body and other non-operation targets are authenticated Call
		// alternatives, but cannot select a fresh-result operation row.
		if hasOperation && targetOperation == operation {
			key, keyOK := operand.Key()
			fresh := valuedomain.Value{}
			var freshOK bool
			if keyOK {
				fresh, freshOK = rule.values.Schema().FreshResultFact(key, materialization.Recent)
			}
			if !freshOK {
				return engine.RuleResult[valuedomain.Value]{}
			}
			return engine.Staged(frame, fresh)
		}
	}
	return engine.NoCandidate(frame)
}

func validCallFact(fact calldomain.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}

// resolveOperand redeems the Link occurrence ID as a Heap key, then asks the
// Value schema for the already-admitted fixed fresh-result operand.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (valuedomain.FreshResultCall, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || !coords.Occurrence.Available() {
		return valuedomain.FreshResultCall{}, false
	}
	key, keyOK := rule.values.Schema().Heap().KeyForID(coords.Occurrence)
	if !keyOK || key.Kind() != heapdomain.RootAllocation {
		return valuedomain.FreshResultCall{}, false
	}
	return rule.values.Schema().FreshResultCallFor(key)
}

// Count implements rule.OccurrenceCatalog by filtering Heap's canonical FreshAt
// order to the fixed CallResultValue rows admitted by Value.
func (rule *HotRule) Count() int {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || !rule.values.Schema().Valid() {
		return 0
	}
	return rule.values.Schema().FreshResultCallCount()
}

// IDAt returns the Heap KeyID of the index-th admitted fixed fresh result.
// It deliberately does not mint a second occurrence identity.
func (rule *HotRule) IDAt(index int) (identity.ContentID, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || !rule.values.Schema().Valid() || index < 0 {
		return identity.ContentID{}, false
	}
	operand, operandOK := rule.values.Schema().FreshResultCallAt(index)
	if !operandOK {
		return identity.ContentID{}, false
	}
	return operand.KeyID()
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[valuedomain.FreshResultCall], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}
