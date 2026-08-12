// Package transfer declares Value's exact fixed-storage copy Rule.
package transfer

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// Rule owns the sole fixed scalar storage copy declaration. The raw engine
// Rule remains private so callers cannot pair a StorageTransfer with arbitrary
// source or destination coordinates.
type Rule struct {
	rule  *engine.Rule[value.Value, value.StorageTransfer]
	read  engine.Read[engine.OrderedCells[value.Value]]
	write engine.Write[value.Value]
	owner *valueowner.Owner
}

// Declare records one one-input identity transfer over Link's exact directed
// fixed Read/Bind/Write relation. Value retains the correlated fact algebra;
// Link retains storage occurrence and direction.
func Declare(
	composition *engine.Composition,
	ruleSemantic engine.SemanticKey,
	operandFamily engine.SemanticKey,
	evidenceSemantic engine.SemanticKey,
	owner *valueowner.Owner,
) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Schema() == nil ||
		!ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}

	var read engine.Read[engine.OrderedCells[value.Value]]
	var write engine.Write[value.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, value.StorageTransfer]{
		Semantic:       ruleSemantic,
		OperandFamily:  operandFamily,
		OperandContent: storageTransferContent,
		Output:         owner.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidenceSemantic, transferChecker(owner, ruleSemantic, &read)),
		Transfer: func(access engine.Access[value.Value, value.StorageTransfer]) bool {
			operand, ok := engine.Operand(access)
			if !ok {
				return false
			}
			if _, _, ok := transferEndpoints(owner, operand); !ok {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, ok := engine.ReadValue(access, row, read)
				if !ok || cells.Count() != 1 {
					return false
				}
				fact, present, available := cells.At(0)
				if !available {
					return false
				}
				if !present {
					return engine.NoCandidate(access, row)
				}
				// The correlated Value travels unchanged: this Rule must never
				// project/rebuild presence, identity, capability, or runtime kind.
				return engine.StageValue(access, row, fact)
			})
		},
	}, func(rule *engine.Rule[value.Value, value.StorageTransfer]) bool {
		input, ok := rule.InputAt(0)
		if !ok {
			return false
		}
		read, ok = engine.ReadFrom(rule, input, owner.ExactRead())
		if !ok || !engine.CarryFrom(rule, input, owner.Carry()) {
			return false
		}
		write, ok = engine.WriteTo(rule, owner.ExactWrite())
		return ok
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, read: read, write: write, owner: owner}, true
}

// Instance derives the sole source read and destination write from one
// canonical directed StorageTransfer. No raw Value coordinate crosses this
// API boundary.
func (rule *Rule) Instance(transfer value.StorageTransfer) (*engine.RuleInstance[value.Value, value.StorageTransfer], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil {
		return nil, false
	}
	from, to, ok := transferEndpoints(rule.owner, transfer)
	if !ok {
		return nil, false
	}
	fromRef, fromOK := rule.owner.Locate(from)
	toRef, toOK := rule.owner.Locate(to)
	if !fromOK || !toOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, transfer, func(binding *engine.RuleBinding[value.Value, value.StorageTransfer]) bool {
		return engine.InstanceRead(binding, rule.read, fromRef) && engine.InstanceWrite(binding, rule.write, toRef)
	})
}

func storageTransferContent(transfer value.StorageTransfer) (value.StorageTransfer, [32]byte, bool) {
	id, ok := transfer.ID()
	return transfer, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

// transferEndpoints centralizes the owner fence shared by construction,
// execution, and evidence. A same-content foreign Schema cannot supply an
// input/output pair to this Rule.
func transferEndpoints(owner *valueowner.Owner, transfer value.StorageTransfer) (value.Coordinate, value.Coordinate, bool) {
	if owner == nil || owner.Schema() == nil {
		return value.Coordinate{}, value.Coordinate{}, false
	}
	from, to, ok := transfer.Endpoints()
	if !ok {
		return value.Coordinate{}, value.Coordinate{}, false
	}
	if _, ok := owner.Schema().CoordinateIndex(from); !ok {
		return value.Coordinate{}, value.Coordinate{}, false
	}
	if _, ok := owner.Schema().CoordinateIndex(to); !ok {
		return value.Coordinate{}, value.Coordinate{}, false
	}
	return from, to, true
}

func transferChecker(owner *valueowner.Owner, ruleSemantic engine.SemanticKey, read *engine.Read[engine.OrderedCells[value.Value]]) engine.RuleDerivationChecker[value.Value, value.StorageTransfer] {
	return func(derivation engine.RuleDerivation[value.Value, value.StorageTransfer]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || read == nil || derivation.Rule() != ruleSemantic ||
			derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		transfer, ok := derivation.Operand()
		if !ok {
			return engine.RuleEvidence{}, false
		}
		id, idOK := transfer.ID()
		if !idOK || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		from, to, endpoints := transferEndpoints(owner, transfer)
		fromRef, fromOK := owner.Locate(from)
		toRef, toOK := owner.Locate(to)
		if !endpoints || !fromOK || !toOK || !engine.DerivationReadMatchesRef(derivation, *read, fromRef) {
			return engine.RuleEvidence{}, false
		}
		input, inputOK := derivation.InputAt(0)
		if !inputOK || input.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		for index := 0; index < derivation.DispositionCount(); index++ {
			disposition, dispositionOK := derivation.DispositionAt(index)
			if !dispositionOK || disposition.Guard().Empty() {
				return engine.RuleEvidence{}, false
			}
			cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, *read)
			if !cellsOK || cells.Count() != 1 {
				return engine.RuleEvidence{}, false
			}
			fact, present, available := cells.At(0)
			if !available {
				return engine.RuleEvidence{}, false
			}
			if !present {
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
				return engine.RuleEvidence{}, false
			}
			target, targetOK := disposition.TargetAt(0)
			actual, actualOK := disposition.Value()
			if !targetOK || !actualOK || !engine.TargetMatchesRef(target, toRef) || !owner.Schema().Equal(actual, fact) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}
