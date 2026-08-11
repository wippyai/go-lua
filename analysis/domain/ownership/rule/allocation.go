// Package rule declares Ownership's exact allocation-origin source judgment.
// It owns no reachability, heap image, residence, or release decision.
package rule

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/ownership"
	ownershipowner "github.com/wippyai/go-lua/analysis/domain/ownership/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
)

// AllocationOperand is one exact allocation-origin ownership coordinate. The
// sealed coordinate already contains its AnalysisRoot; the Link allocation
// origin supplies the sole allocation alternative. No Rule caller selects a
// root, role, or analysis root independently.
type AllocationOperand struct {
	source     *link.Link
	schema     ownership.Schema
	coordinate ownership.Coordinate
	root       ownership.Root
	origin     ownership.Origin
	role       ownership.Role
	content    keyspace.ContentID
}

// NewAllocationOperand derives the initial Owner or Lifetime duty for a
// concrete allocation alternative. It intentionally excludes BootRoot:
// ownership.Value ranges over allocation alternatives only, while boot-root
// retention/residence remains its own boundary rule once those premises exist.
func NewAllocationOperand(source *link.Link, schema ownership.Schema, coordinate ownership.Coordinate) (AllocationOperand, bool) {
	if source == nil || !schema.Valid() || schema.Link() != source {
		return AllocationOperand{}, false
	}
	linkID, linked := schema.LinkContentID()
	if !linked || linkID != source.ContentID() {
		return AllocationOperand{}, false
	}
	origin, originOK := schema.Origin(coordinate)
	role, roleOK := schema.Role(coordinate)
	if !originOK || !roleOK || origin.Kind() != ownership.OriginAllocationRoot || (role != ownership.Owner && role != ownership.Lifetime) {
		return AllocationOperand{}, false
	}
	allocation, allocationOK := schema.OriginHeapKey(origin)
	root, rootOK := schema.RootFor(allocation)
	if !allocationOK || !rootOK {
		return AllocationOperand{}, false
	}
	content, contentOK := allocationOperandID(source, schema, coordinate, root, origin, role)
	if !contentOK {
		return AllocationOperand{}, false
	}
	return AllocationOperand{source: source, schema: schema, coordinate: coordinate, root: root, origin: origin, role: role, content: content}, true
}

// ContentID is the cold identity of this exact source/root/duty tuple.
func (operand AllocationOperand) ContentID() (keyspace.ContentID, bool) {
	if !operand.valid() {
		return keyspace.ContentID{}, false
	}
	return operand.content, true
}

func (operand AllocationOperand) valid() bool {
	if operand.source == nil || !operand.schema.Valid() || !operand.coordinate.Valid() || !operand.root.Valid() || operand.origin.Kind() != ownership.OriginAllocationRoot ||
		(operand.role != ownership.Owner && operand.role != ownership.Lifetime) || !operand.content.Available() {
		return false
	}
	if operand.schema.Link() != operand.source {
		return false
	}
	linkID, linked := operand.schema.LinkContentID()
	if !linked || linkID != operand.source.ContentID() {
		return false
	}
	expected, ok := NewAllocationOperand(operand.source, operand.schema, operand.coordinate)
	return ok && expected.schema == operand.schema && expected.coordinate == operand.coordinate && expected.root == operand.root && expected.origin == operand.origin && expected.role == operand.role && expected.content == operand.content
}

func allocationOperandID(source *link.Link, schema ownership.Schema, coordinate ownership.Coordinate, root ownership.Root, origin ownership.Origin, role ownership.Role) (keyspace.ContentID, bool) {
	if source == nil || !schema.Valid() || schema.Link() != source || !coordinate.Valid() || !root.Valid() || !role.Valid() {
		return keyspace.ContentID{}, false
	}
	analysis, analysisOK := schema.AnalysisRoot(coordinate)
	originID, originOK := schema.OriginID(origin)
	analysisID, analysisOK := source.Module().Roots().ID(analysis)
	allocation, allocationOK := schema.HeapKey(root)
	allocationID, allocationIDOK := allocation.ContentID()
	if !originOK || !analysisOK || !allocationOK || !allocationIDOK {
		return keyspace.ContentID{}, false
	}
	var image [32 + 32 + 32 + 32 + 8]byte
	linkID := source.ContentID()
	copy(image[:32], linkID[:])
	copy(image[32:64], originID[:])
	copy(image[64:96], analysisID[:])
	copy(image[96:128], allocationID[:])
	binary.BigEndian.PutUint64(image[128:], uint64(role))
	return keyspace.ContentID(sha256.Sum256(image[:])), true
}

// AllocationRule is the zero-read source of an exact initial ownership duty.
// Body assembly later supplies its Program guard; this declaration cannot mint
// an ownership obligation for an arbitrary coordinate or allocation root.
type AllocationRule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[ownership.Value, AllocationOperand]
	owner    *ownershipowner.Owner
	write    engine.Write[ownership.Value]
}

func DeclareAllocation(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *ownershipowner.Owner) (*AllocationRule, bool) {
	if composition == nil || owner == nil || !owner.Schema().Valid() || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	declaration := &AllocationRule{semantic: semantic, owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[ownership.Value, AllocationOperand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: allocationOperandContent, Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[ownership.Value, AllocationOperand]) bool {
		write, written := engine.WriteTo(rule, owner.Write())
		if written {
			declaration.rule, declaration.write = rule, write
		}
		return written
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

// Instance binds the source rule to its one derived Ownership coordinate only
// after composition sealing.
func (rule *AllocationRule) Instance(operand AllocationOperand) (*engine.RuleInstance[ownership.Value, AllocationOperand], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || !validAllocationOperand(rule.owner, operand) {
		return nil, false
	}
	ref, ok := rule.owner.Locate(operand.coordinate)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[ownership.Value, AllocationOperand]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func allocationOperandContent(operand AllocationOperand) (AllocationOperand, [32]byte, bool) {
	if !operand.valid() {
		return AllocationOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.content), true
}

func validAllocationOperand(owner *ownershipowner.Owner, operand AllocationOperand) bool {
	if owner == nil || !owner.Schema().Valid() || !operand.valid() {
		return false
	}
	if owner.Schema().Link() != operand.source {
		return false
	}
	schemaID, schemaOK := owner.Schema().LinkContentID()
	if !schemaOK || operand.source.ContentID() != schemaID {
		return false
	}
	expected, ok := NewAllocationOperand(operand.source, owner.Schema(), operand.coordinate)
	return ok && expected.schema == operand.schema && expected.coordinate == operand.coordinate && expected.root == operand.root && expected.origin == operand.origin && expected.role == operand.role && expected.content == operand.content
}

func allocationResult(owner *ownershipowner.Owner, operand AllocationOperand) (ownership.Value, bool) {
	if !validAllocationOperand(owner, operand) {
		return ownership.Value{}, false
	}
	value, ok := owner.Schema().Of(operand.root, materialization.Recent, ownership.One, ownership.One)
	return value, ok && owner.Schema().Admit(operand.coordinate, value)
}

func (rule *AllocationRule) transfer(access engine.Access[ownership.Value, AllocationOperand]) bool {
	operand, ok := engine.Operand(access)
	if !ok {
		return false
	}
	value, ok := allocationResult(rule.owner, operand)
	if !ok {
		return false
	}
	rows := 0
	return engine.Product(access, func(row engine.Row) bool {
		rows++
		return rows == 1 && engine.StageValue(access, row, value)
	}) && rows == 1
}

func (rule *AllocationRule) check(derivation engine.RuleDerivation[ownership.Value, AllocationOperand]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, ok := derivation.Operand()
	if !ok || !validAllocationOperand(rule.owner, operand) || !derivation.OperandContentMatches([32]byte(operand.content)) {
		return engine.RuleEvidence{}, false
	}
	ref, refOK := rule.owner.Locate(operand.coordinate)
	expected, valueOK := allocationResult(rule.owner, operand)
	disposition, dispositionOK := derivation.DispositionAt(0)
	actual, actualOK := disposition.Value()
	target, targetOK := disposition.TargetAt(0)
	if !refOK || !valueOK || !dispositionOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() ||
		!actualOK || !targetOK || !engine.TargetMatchesRef(target, ref) || !rule.owner.Schema().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
