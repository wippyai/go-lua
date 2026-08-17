package refinement

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// HotRule executes one artifact-issued storage nilability refinement. It sees
// no Branch, Program, Flow, or diagnostic vocabulary: those proofs were
// consumed when ProgramArtifact placed the rule on the exact guarded arm.
type HotRule struct {
	implementation *valueowner.RuleImplementation[value.PresenceRefinement]
	read           engine.Read[engine.OrderedCells[value.Value]]
	owner          *valueowner.HotOwner
}

func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		!fragment.semantic.Available() || !fragment.evidence.Available() || fragment.semantic == fragment.evidence {
		return nil, false
	}
	var runtimeRead engine.Read[engine.OrderedCells[value.Value]]
	implementation, read, ok := valueowner.BindExactReadAndCarryRule(owner, fragment.slot, fragment.read, fragment.carry, fragment.write, engine.HotRuleSpec[value.Value, value.PresenceRefinement]{
		OperandContent: func(refinement value.PresenceRefinement) (value.PresenceRefinement, [32]byte, bool) {
			return hotContent(owner.Schema(), refinement)
		},
		Admission: engine.AdmitRuleByDerivation(fragment.evidence, hotChecker(owner, fragment.semantic, &runtimeRead)),
		Transfer: func(access engine.Access[value.Value, value.PresenceRefinement]) bool {
			refinement, operandOK := engine.Operand(access)
			_, present, targetOK := hotTarget(owner.Schema(), refinement)
			if !operandOK || !targetOK {
				return false
			}
			return engine.Product(access, func(row engine.Row) bool {
				cells, readOK := engine.ReadValue(access, row, runtimeRead)
				if !readOK || cells.Count() != 1 {
					return false
				}
				fact, factPresent, available := cells.At(0)
				if !available {
					return false
				}
				if !factPresent {
					return engine.NoCandidate(access, row)
				}
				result, resultOK := owner.Schema().FilterPresence(fact, present)
				return resultOK && engine.StageValue(access, row, result)
			})
		},
	}, engine.HotCarrySpec[value.Value, value.PresenceRefinement]{})
	if !ok || implementation == nil {
		return nil, false
	}
	runtimeRead = read
	return &HotRule{implementation: implementation, read: read, owner: owner}, true
}

func (rule *HotRule) ReceiptForOccurrence(mount, id identity.ContentID) (value.PresenceRefinement, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return value.PresenceRefinement{}, false
	}
	row, ok := rule.owner.Schema().PresenceRefinement(mount, id)
	return row, ok && rule.owner.Schema().OwnsPresenceRefinement(row)
}

func (rule *HotRule) AttachMountedRule(assembly *engine.ReceiptAssembly, mountID, pointID, occurrenceID identity.ContentID) (engine.BindingRuleRowRef, bool) {
	if rule == nil || rule.owner == nil || assembly == nil {
		return engine.BindingRuleRowRef{}, false
	}
	operand, operandOK := rule.ReceiptForOccurrence(mountID, occurrenceID)
	implementation, implementationOK := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	capability := mountedCapability(rule.implementation)
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(capability, mountID, pointID, occurrenceID)
	target, _, targetOK := hotTarget(rule.owner.Schema(), operand)
	targetRef, refOK := rule.owner.Ref(target)
	if !operandOK || !implementationOK || !occurrenceOK || !targetOK || !refOK {
		return engine.BindingRuleRowRef{}, false
	}
	transaction, transactionOK := engine.BeginMountedRuleAdmission(assembly, implementation, occurrence, operand)
	if !transactionOK || !engine.AddExactRead(transaction, targetRef) || !transaction.AddCarry() || !engine.AddExactWrite(transaction, targetRef) {
		return engine.BindingRuleRowRef{}, false
	}
	queued := assembly.QueueMountedRuleFinalizer(capability, func() bool {
		source, sourceOK := transaction.Seal()
		draft, draftOK := implementation.BeginReceiptRuleRow(source)
		readPart, readOK := implementation.ReceiptReadPart(source, 0)
		carryPart, carryOK := implementation.ReceiptCarryPart(source, 0)
		writePart, writeOK := implementation.ReceiptWritePart(source, 0)
		if !sourceOK || !draftOK || !readOK || !carryOK || !writeOK ||
			!draft.AddRead(readPart) || !draft.AddCarry(carryPart) || !draft.AddWrite(writePart) {
			return false
		}
		_, added := assembly.AddRuleFromDraft(occurrence, draft)
		return added
	})
	return engine.BindingRuleRowRef{}, queued
}

func (rule *HotRule) BeginReceiptCompilation(graph *engine.ReceiptGraph) (*engine.ReceiptCompilation, bool) {
	if rule == nil || rule.owner == nil {
		return nil, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.BeginReceiptCompilation(implementation, graph)
}

func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, pointID, occurrenceID identity.ContentID) (*engine.ReceiptMember, bool) {
	if rule == nil || graph == nil {
		return nil, false
	}
	member, memberOK := graph.MountedRuleMember(mountedCapability(rule.implementation), mountID, pointID, occurrenceID)
	operand, operandOK := rule.ReceiptForOccurrence(mountID, occurrenceID)
	implementation, implementationOK := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !memberOK || !operandOK || !implementationOK {
		return nil, false
	}
	return engine.AttachReceiptRuleMember(compilation, implementation, member, operand)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.PresenceRefinement], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

func mountedCapability(issuer interface {
	MountedCapability() (engine.RuleSlotCapability, bool)
}) engine.RuleSlotCapability {
	capability, _ := issuer.MountedCapability()
	return capability
}

func hotContent(schema *value.Schema, row value.PresenceRefinement) (value.PresenceRefinement, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsPresenceRefinement(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return value.PresenceRefinement{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}

func hotTarget(schema *value.Schema, row value.PresenceRefinement) (value.Coordinate, bool, bool) {
	if schema == nil || !schema.OwnsPresenceRefinement(row) {
		return value.Coordinate{}, false, false
	}
	target, present, ok := row.Target()
	if !ok {
		return value.Coordinate{}, false, false
	}
	if _, coordinateOK := schema.CoordinateIndex(target); !coordinateOK {
		return value.Coordinate{}, false, false
	}
	return target, present, true
}

func hotChecker(owner *valueowner.HotOwner, semantic identity.SemanticKey, read *engine.Read[engine.OrderedCells[value.Value]]) engine.RuleDerivationChecker[value.Value, value.PresenceRefinement] {
	return func(derivation engine.RuleDerivation[value.Value, value.PresenceRefinement]) (engine.RuleEvidence, bool) {
		if owner == nil || owner.Schema() == nil || read == nil || derivation.Rule() != semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() == 0 {
			return engine.RuleEvidence{}, false
		}
		operand, operandOK := derivation.Operand()
		canonical, digest, contentOK := hotContent(owner.Schema(), operand)
		target, present, targetOK := hotTarget(owner.Schema(), canonical)
		input, inputOK := derivation.InputAt(0)
		if !operandOK || !contentOK || !targetOK || !derivation.OperandContentMatches(digest) || !inputOK || input.Guard().Empty() ||
			!valueowner.ReadMatches(owner, derivation, *read, target) {
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
			fact, factPresent, available := cells.At(0)
			if !available {
				return engine.RuleEvidence{}, false
			}
			if !factPresent {
				if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
					return engine.RuleEvidence{}, false
				}
				continue
			}
			expected, expectedOK := owner.Schema().FilterPresence(fact, present)
			if !expectedOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 {
				return engine.RuleEvidence{}, false
			}
			actualTarget, targetOK := disposition.TargetAt(0)
			actual, valueOK := disposition.Value()
			if !targetOK || !valueOK || !owner.TargetMatches(actualTarget, target) || !owner.Schema().Equal(actual, expected) {
				return engine.RuleEvidence{}, false
			}
		}
		return derivation.Accept()
	}
}
