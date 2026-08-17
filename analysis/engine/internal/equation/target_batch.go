package equation

// TargetBatch is the typed, callback-free admission vocabulary for the rows
// that sit above ordinary Sites/Occurrences/Operands.  It deliberately uses
// the same open Batch transaction: target rows are checked while the Batch is
// open and become readable only after that Batch seals.

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// BatchInput is an input boundary recipe. Its endpoint capabilities are
// checked against the owning open Batch and the immutable Input is derived at
// the single target Batch seal.
type BatchInput struct {
	Source     Site
	Target     Site
	Provenance composition.Key
	Pre        Expr
	Reindex    Reindex
	Post       Expr
}

// TargetBoundaryInput creates one target input recipe. It does not mint an
// Input before the owning Batch seals.
func TargetBoundaryInput(source, target Site, provenance composition.Key, pre Expr, reindex Reindex, post Expr) BatchInput {
	return BatchInput{Source: source, Target: target, Provenance: provenance, Pre: pre, Reindex: reindex, Post: post}
}

type BatchGroup struct {
	Members          []RuleRef
	Output           PointRef
	Inputs           []BatchInput
	EnvironmentInput *BatchInput
	Premise          Expr
}

type BatchFactorEdge struct {
	Target PointRef
	Input  BatchInput
	Factor composition.Key
}

type BatchEnvironmentEdge struct {
	Target PointRef
	Input  BatchInput
}

// targetRows are part of the same Batch-owned mutable transaction. They are
// never a parallel sealed topology or a derived activation rows.
type targetRows struct {
	points      []PointSpec
	rules       []RuleInstance
	inputs      []BatchInput
	groups      []BatchGroup
	factorEdges []BatchFactorEdge
	environment []BatchEnvironmentEdge
	summaries   []SummaryMapping
	weakTargets []WeakTargetMapping
	sealed      bool
}

func (batch *Batch) admitTargetOpen() bool {
	return batch != nil && batch.phase == batchOpen
}

// AdmitRule admits one exact target Rule row while the Batch is open.
func (batch *Batch) AdmitRule(rule RuleInstance) bool {
	if !batch.admitTargetOpen() || !rule.Schema.Available() || !rule.OperandFamily.Available() ||
		!batch.OwnsOpenOccurrence(rule.Occurrence) || !batch.OwnsOpenOperandFor(rule.Operand, rule.Occurrence) {
		if batch != nil && batch.phase == batchOpen {
			batch.rejectOpen()
		}
		return false
	}
	batch.targets.rules = append(batch.targets.rules, copyInstance(rule))
	return true
}

// AdmitPoint issues the target point ordinal for one concrete Site. Point
// refs are allocated only in this target transaction and are stable after the
// one Batch seal.
func (batch *Batch) AdmitPoint(site Site) (PointRef, bool) {
	if !batch.admitTargetOpen() || !batch.OwnsOpenSite(site) {
		if batch != nil && batch.phase == batchOpen {
			batch.rejectOpen()
		}
		return 0, false
	}
	batch.targets.points = append(batch.targets.points, PointSpec{Site: site})
	return PointAt(len(batch.targets.points) - 1), true
}

func validTargetInput(batch *Batch, input BatchInput) bool {
	return batch != nil && batch.admitTargetOpen() && batch.OwnsOpenSite(input.Source) && batch.OwnsOpenSite(input.Target) &&
		input.Provenance.Available() && input.Pre.Available() && input.Post.Available() && input.Reindex.Available()
}

// AdmitInput records one ordinary boundary input in the same open target
// transaction as Sites, Rules, and Groups.  The canonical Input capability is
// intentionally delayed until Batch.Seal, so formal template authors cannot
// accidentally retain a cross-batch or pre-seal input.
func (batch *Batch) AdmitInput(input BatchInput) bool {
	if !validTargetInput(batch, input) {
		if batch != nil && batch.phase == batchOpen {
			batch.rejectOpen()
		}
		return false
	}
	batch.targets.inputs = append(batch.targets.inputs, input)
	return true
}

func validTargetMembers(members []RuleRef) bool {
	if len(members) == 0 {
		return false
	}
	seen := make(map[RuleRef]struct{}, len(members))
	for _, member := range members {
		if uint64(member) == 0 {
			return false
		}
		if _, duplicate := seen[member]; duplicate {
			return false
		}
		seen[member] = struct{}{}
	}
	return true
}

// AdmitGroup admits a target Group recipe. Inputs remain recipes until Seal.
func (batch *Batch) AdmitGroup(group BatchGroup) bool {
	if !batch.admitTargetOpen() || group.Output == 0 || !validTargetMembers(group.Members) {
		if batch != nil && batch.phase == batchOpen {
			batch.rejectOpen()
		}
		return false
	}
	for _, input := range group.Inputs {
		if !validTargetInput(batch, input) {
			batch.rejectOpen()
			return false
		}
	}
	if group.EnvironmentInput != nil && !validTargetInput(batch, *group.EnvironmentInput) {
		batch.rejectOpen()
		return false
	}
	copyGroup := BatchGroup{Members: append([]RuleRef(nil), group.Members...), Output: group.Output, Inputs: append([]BatchInput(nil), group.Inputs...), Premise: group.Premise}
	if group.EnvironmentInput != nil {
		value := *group.EnvironmentInput
		copyGroup.EnvironmentInput = &value
	}
	batch.targets.groups = append(batch.targets.groups, copyGroup)
	return true
}

func (batch *Batch) AdmitFactorEdge(edge BatchFactorEdge) bool {
	if !batch.admitTargetOpen() || edge.Target == 0 || !edge.Factor.Available() || !validTargetInput(batch, edge.Input) {
		if batch != nil && batch.phase == batchOpen {
			batch.rejectOpen()
		}
		return false
	}
	batch.targets.factorEdges = append(batch.targets.factorEdges, edge)
	return true
}

func (batch *Batch) AdmitEnvironmentEdge(edge BatchEnvironmentEdge) bool {
	if !batch.admitTargetOpen() || edge.Target == 0 || !validTargetInput(batch, edge.Input) {
		if batch != nil && batch.phase == batchOpen {
			batch.rejectOpen()
		}
		return false
	}
	batch.targets.environment = append(batch.targets.environment, edge)
	return true
}

// AdmitSummary records one exact summary mapping in the same target Batch.
func (batch *Batch) AdmitSummary(value SummaryMapping) bool {
	if !batch.admitTargetOpen() || !validSummaryMapping(value) {
		if batch != nil && batch.phase == batchOpen {
			batch.rejectOpen()
		}
		return false
	}
	for _, existing := range batch.targets.summaries {
		if existing.Surface == value.Surface {
			return false
		}
	}
	batch.targets.summaries = append(batch.targets.summaries, SummaryMapping{Surface: value.Surface, Keys: append([]uint64(nil), value.Keys...)})
	return true
}

// AdmitWeakTarget records one exact weak-target coverage mapping in the same
// target Batch. Coverage is validated again at seal and assembly.
func (batch *Batch) AdmitWeakTarget(value WeakTargetMapping) bool {
	if !batch.admitTargetOpen() || !validWeakTargetMapping(value) {
		if batch != nil && batch.phase == batchOpen {
			batch.rejectOpen()
		}
		return false
	}
	for _, existing := range batch.targets.weakTargets {
		if existing.Surface == value.Surface {
			return false
		}
	}
	batch.targets.weakTargets = append(batch.targets.weakTargets, WeakTargetMapping{Surface: value.Surface, Candidates: append([]Surface(nil), value.Candidates...)})
	return true
}

func copyTargetInput(value BatchInput) Input {
	return BoundaryInput(value.Source, value.Target, value.Provenance, value.Pre, value.Reindex, value.Post)
}

func (batch *Batch) sealTargetRowsWithFailure() SealFailure {
	if batch == nil || !batch.Sealed() || batch.targets.sealed {
		if batch != nil && batch.targets.sealed {
			return SealFailure{}
		}
		return sealRefused(SealFailureFamilySource, "target-state")
	}
	for _, rule := range batch.targets.rules {
		if !batch.ownsOccurrence(rule.Occurrence) || !batch.ownsOperand(rule.Operand) || !rule.Operand.Occurrence().Same(rule.Occurrence) {
			return sealRefused(SealFailureFamilySource, "target-rule")
		}
	}
	for _, input := range batch.targets.inputs {
		if !copyTargetInput(input).Available() {
			return sealRefused(SealFailureFamilySource, "target-input")
		}
	}
	for _, group := range batch.targets.groups {
		if !validTargetMembers(group.Members) {
			return sealRefused(SealFailureFamilySource, "target-group")
		}
		for _, input := range group.Inputs {
			if !copyTargetInput(input).Available() {
				return sealRefused(SealFailureFamilySource, "target-group-input")
			}
		}
		if group.EnvironmentInput != nil && !copyTargetInput(*group.EnvironmentInput).Available() {
			return sealRefused(SealFailureFamilySource, "target-environment-input")
		}
	}
	for _, edge := range batch.targets.factorEdges {
		if !edge.Factor.Available() || !copyTargetInput(edge.Input).Available() {
			return sealRefused(SealFailureFamilySource, "target-factor-edge")
		}
	}
	for _, edge := range batch.targets.environment {
		if !copyTargetInput(edge.Input).Available() {
			return sealRefused(SealFailureFamilySource, "target-environment-edge")
		}
	}
	for _, value := range batch.targets.summaries {
		if !validSummaryMapping(value) {
			return sealRefused(SealFailureFamilySource, "target-summary")
		}
	}
	for _, value := range batch.targets.weakTargets {
		if !validWeakTargetMapping(value) {
			return sealRefused(SealFailureFamilySource, "target-weak")
		}
	}
	batch.targets.sealed = true
	return SealFailure{}
}

func (batch *Batch) TargetMetadataRows() ([]SummaryMapping, []WeakTargetMapping, bool) {
	if batch == nil || !batch.Sealed() || !batch.targets.sealed {
		return nil, nil, false
	}
	summaries := make([]SummaryMapping, len(batch.targets.summaries))
	for index, value := range batch.targets.summaries {
		summaries[index] = SummaryMapping{Surface: value.Surface, Keys: append([]uint64(nil), value.Keys...)}
	}
	weak := make([]WeakTargetMapping, len(batch.targets.weakTargets))
	for index, value := range batch.targets.weakTargets {
		weak[index] = WeakTargetMapping{Surface: value.Surface, Candidates: append([]Surface(nil), value.Candidates...)}
	}
	return summaries, weak, true
}

// TargetInputRows returns the immutable standalone inputs admitted to this
// Batch. It is a read of the same sealed target catalog, not a second graph.
func (batch *Batch) TargetInputRows() ([]Input, bool) {
	if batch == nil || !batch.Sealed() || !batch.targets.sealed {
		return nil, false
	}
	result := make([]Input, len(batch.targets.inputs))
	for index, value := range batch.targets.inputs {
		result[index] = copyTargetInput(value)
		if !result[index].Available() {
			return nil, false
		}
	}
	return result, true
}

// TargetRows returns immutable copies of all target rows after the owning
// Batch seals. It is intentionally a single catalog read, not a second graph.
func (batch *Batch) TargetRows() ([]PointSpec, []RuleInstance, []BatchGroup, []BatchFactorEdge, []BatchEnvironmentEdge, bool) {
	if batch == nil || !batch.Sealed() || !batch.targets.sealed {
		return nil, nil, nil, nil, nil, false
	}
	points := append([]PointSpec(nil), batch.targets.points...)
	rules := make([]RuleInstance, len(batch.targets.rules))
	for index, rule := range batch.targets.rules {
		rules[index] = copyInstance(rule)
	}
	groups := make([]BatchGroup, len(batch.targets.groups))
	for index, group := range batch.targets.groups {
		groups[index] = BatchGroup{Members: append([]RuleRef(nil), group.Members...), Output: group.Output, Inputs: append([]BatchInput(nil), group.Inputs...), Premise: group.Premise}
		if group.EnvironmentInput != nil {
			value := *group.EnvironmentInput
			groups[index].EnvironmentInput = &value
		}
	}
	return points, rules, groups, append([]BatchFactorEdge(nil), batch.targets.factorEdges...), append([]BatchEnvironmentEdge(nil), batch.targets.environment...), true
}

// MaterializeTargetBatch projects the one sealed target transaction into the
// ordinary TopologySpec consumed by the equation compiler. It does not copy
// or reconstruct a legacy fragment; every row originates in the same Batch.
func MaterializeTargetBatch(batch *Batch) (TopologySpec, bool) {
	points, rules, groups, factorEdges, environmentEdges, ok := batch.TargetRows()
	if !ok {
		return TopologySpec{}, false
	}
	summaries, weakTargets, metadataOK := batch.TargetMetadataRows()
	if !metadataOK {
		return TopologySpec{}, false
	}
	ordinaryGroups := make([]Group, len(groups))
	for index, value := range groups {
		ordinary := Group{Members: append([]RuleRef(nil), value.Members...), Output: value.Output, Inputs: make([]Input, len(value.Inputs)), premise: value.Premise}
		if !ordinary.premise.Available() {
			ordinary.premise = TrueExpr()
		}
		for inputIndex, input := range value.Inputs {
			ordinary.Inputs[inputIndex] = copyTargetInput(input)
			if !ordinary.Inputs[inputIndex].Available() {
				return TopologySpec{}, false
			}
		}
		if value.EnvironmentInput != nil {
			ordinary.EnvironmentInput = copyTargetInput(*value.EnvironmentInput)
			if !ordinary.EnvironmentInput.Available() {
				return TopologySpec{}, false
			}
		}
		ordinaryGroups[index] = ordinary
	}
	ordinaryFactorEdges := make([]FactorEdge, len(factorEdges))
	for index, value := range factorEdges {
		input := copyTargetInput(value.Input)
		if !input.Available() {
			return TopologySpec{}, false
		}
		ordinaryFactorEdges[index] = FactorEdge{Target: value.Target, Input: input, Factor: value.Factor}
	}
	ordinaryEnvironmentEdges := make([]EnvironmentEdge, len(environmentEdges))
	for index, value := range environmentEdges {
		input := copyTargetInput(value.Input)
		if !input.Available() {
			return TopologySpec{}, false
		}
		ordinaryEnvironmentEdges[index] = EnvironmentEdge{Target: value.Target, Input: input}
	}
	return TopologySpec{Batch: batch, Rules: rules, Points: points, Groups: ordinaryGroups, FactorEdges: ordinaryFactorEdges, EnvironmentEdges: ordinaryEnvironmentEdges, Summaries: summaries, WeakTargets: weakTargets}, true
}
