package moduleload

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type HotRule struct {
	implementation *valueowner.RuleImplementation[valuedomain.ModuleLoadCall]
	callRead       engine.Read[engine.OrderedCells[call.Value]]
	valueRead      engine.Read[engine.OrderedCells[valuedomain.Value]]
	values         *valueowner.HotOwner
	calls          *callowner.HotOwner
	semantic       identity.SemanticKey
}

func BindHot(fragment *SchemaFragment, values *valueowner.HotOwner, calls *callowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || values == nil || calls == nil || values.Schema() == nil || calls.Algebra() == nil || !fragment.semantic.Available() {
		return nil, false
	}
	hot := &HotRule{values: values, calls: calls, semantic: fragment.semantic}
	var callRead engine.Read[engine.OrderedCells[call.Value]]
	var valueRead engine.Read[engine.OrderedCells[valuedomain.Value]]
	var implementation *valueowner.RuleImplementation[valuedomain.ModuleLoadCall]
	bindSpec := engine.HotRuleSpec[valuedomain.Value, valuedomain.ModuleLoadCall]{
		OperandContent: func(row valuedomain.ModuleLoadCall) (valuedomain.ModuleLoadCall, [32]byte, bool) {
			return hotContent(values.Schema(), row)
		},
		OperandResolver: hot.resolveOperand,
		Fold: func(frame engine.Frame[valuedomain.Value, valuedomain.ModuleLoadCall]) engine.RuleResult[valuedomain.Value] {
			operand, operandOK := engine.Operand(frame)
			if !operandOK {
				return engine.RuleResult[valuedomain.Value]{}
			}
			callCells, callOK := engine.ReadValue(frame, callRead)
			valueCells, valueOK := engine.ReadValue(frame, valueRead)
			if !callOK || !valueOK || callCells.Count() != 1 || valueCells.Count() != 1 {
				return engine.RuleResult[valuedomain.Value]{}
			}
			callFact, callPresent, callAvailable := callCells.At(0)
			argumentFact, argumentPresent, argumentAvailable := valueCells.At(0)
			if !callAvailable || !argumentAvailable {
				return engine.RuleResult[valuedomain.Value]{}
			}
			if !callPresent || !argumentPresent {
				return engine.NoCandidate(frame)
			}
			projected, decision, ok := classify(values.Schema(), callFact, argumentFact, operand)
			if !ok {
				return engine.RuleResult[valuedomain.Value]{}
			}
			switch decision {
			case decisionNoCandidate:
				return engine.NoCandidate(frame)
			case decisionStage:
				return engine.Staged(frame, projected)
			default:
				// decisionInvalid and decisionRefused both leave the rule
				// without a result to publish.
				return engine.RuleResult[valuedomain.Value]{}
			}
		},
	}
	implementation, bound := valueowner.BindSelectedRuleDirect(values, fragment.slot, fragment.carry, fragment.write, values.FactorRef(), bindSpec, engine.HotCarrySpec[valuedomain.Value, valuedomain.ModuleLoadCall]{}, func(row valuedomain.ModuleLoadCall) (uint64, bool) {
		return row.Endpoint(valuedomain.EndpointWrite)
	})
	if !bound {
		return nil, false
	}
	callRead, callOK := valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.callRead, calls.FactorRef(), func(row valuedomain.ModuleLoadCall) (uint64, bool) {
		return projectCall(values.Schema(), row)
	})
	valueRead, valueOK := valueowner.AddSelectedRuleDirectExactRead(implementation, fragment.valueRead, values.FactorRef(), func(row valuedomain.ModuleLoadCall) (uint64, bool) {
		return row.Endpoint(valuedomain.EndpointLeft)
	})
	if !callOK || !valueOK {
		return nil, false
	}
	hot.callRead, hot.valueRead, hot.implementation = callRead, valueRead, implementation
	return hot, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (valuedomain.ModuleLoadCall, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return valuedomain.ModuleLoadCall{}, false
	}
	row, ok := rule.values.Schema().ModuleLoadCall(coords.Mount, coords.Occurrence)
	return row, ok && rule.values.Schema().OwnsModuleLoadCall(row)
}

func (rule *HotRule) OperandForOccurrence(mount, occurrence identity.ContentID) (valuedomain.ModuleLoadCall, bool) {
	if rule == nil || rule.values == nil || rule.values.Schema() == nil || !mount.Available() || !occurrence.Available() {
		return valuedomain.ModuleLoadCall{}, false
	}
	row, ok := rule.values.Schema().ModuleLoadCall(mount, occurrence)
	return row, ok && rule.values.Schema().OwnsModuleLoadCall(row)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[valuedomain.ModuleLoadCall], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

func hotContent(schema *valuedomain.Schema, row valuedomain.ModuleLoadCall) (valuedomain.ModuleLoadCall, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsModuleLoadCall(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return valuedomain.ModuleLoadCall{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}

// projectCall reads the mounted-call coordinate the operand row publishes.
// Call issued that coordinate while Value sealed the row, so the bind-time
// projection is a field read on an owner-fenced operand.
func projectCall(schema *valuedomain.Schema, row valuedomain.ModuleLoadCall) (uint64, bool) {
	if schema == nil || !schema.OwnsModuleLoadCall(row) {
		return 0, false
	}
	coordinate, coordinateOK := row.Call()
	index, indexOK := coordinate.CoordinateIndex()
	return index, coordinateOK && indexOK
}

type decision uint8

const (
	decisionInvalid decision = iota
	// decisionRefused is the named refusal for an operand that does not carry
	// the required operation this rule exists to interpret. A rule cannot
	// widen its way out of a missing declaration: Top would assert that every
	// value is a possible module root, which is a claim about the program
	// rather than an admission that the operand is malformed.
	decisionRefused
	decisionNoCandidate
	decisionStage
)

func classify(schema *valuedomain.Schema, callFact call.Value, argument valuedomain.Value, operand valuedomain.ModuleLoadCall) (valuedomain.Value, decision, bool) {
	if schema == nil || !callValueValid(callFact) || !schema.OwnsModuleLoadCall(operand) {
		return valuedomain.Value{}, decisionInvalid, false
	}
	if argument.IsBottom() || callFact.IsEmpty() {
		return schema.Bottom(), decisionNoCandidate, true
	}
	if callFact.HasOpaqueAlternative() {
		return schema.Top(), decisionStage, true
	}
	if callFact.KnownTargetCount() == 0 {
		return schema.Bottom(), decisionNoCandidate, true
	}
	require, requireOK := operand.RequireOperation()
	if !requireOK {
		return valuedomain.Value{}, decisionRefused, false
	}
	hasScopedLoader := false
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK {
			return valuedomain.Value{}, decisionInvalid, false
		}
		// Other selected targets are owned by their own result consumers. This
		// rule contributes only the scoped-loader alternative; widening here
		// would erase otherwise precise results for every ordinary unary call.
		if !target.IsScopedLoader() {
			continue
		}
		op, opOK := target.Operation()
		if !opOK || op != require {
			return valuedomain.Value{}, decisionInvalid, false
		}
		hasScopedLoader = true
	}
	if !hasScopedLoader {
		return schema.Bottom(), decisionNoCandidate, true
	}
	expected, expectedOK := operand.ExpectedArgument()
	fact, factOK := operand.ResultFact()
	if !expectedOK || !factOK || argument.IsTop() || !schema.Equal(argument, expected) {
		return schema.Top(), decisionStage, true
	}
	return fact, decisionStage, true
}

func callValueValid(fact call.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}
