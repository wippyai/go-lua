package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// IssuanceForm is the placement form one occurrence subscription takes.
// Ordinals match structure.CategoryIssuanceForm: base, local, computation,
// local-predecessor, call-stage.
type IssuanceForm uint8

const (
	IssuanceFormInvalid IssuanceForm = iota
	IssuanceFormBase
	IssuanceFormLocal
	IssuanceFormComputation
	IssuanceFormLocalPredecessor
	IssuanceFormCallStage
)

func (form IssuanceForm) valid() bool {
	return form >= IssuanceFormBase && form <= IssuanceFormCallStage
}

// IssuancePlacement is one sealed rule.Issues row as the compiler places it.
// Key is the declaration identity.
type IssuancePlacement struct {
	Occurrence OccurrenceKind
	Form       IssuanceForm
	Input      RuleInputKind
	Stage      RuleStage
	Code       uint64
	HasCode    bool
	Key        schema.Key
}

func (placement IssuancePlacement) Available() bool {
	return placement.Occurrence.valid() && placement.Form.valid() &&
		placement.Input.valid() && placement.Stage.valid() &&
		placement.Key.Available()
}

// IssuanceDirectory is the sealed subscription catalog the compiler places
// from. It is built at the composition root from rule.Issues.
type IssuanceDirectory []IssuancePlacement

func (directory IssuanceDirectory) matching(kind OccurrenceKind, code uint64) []IssuancePlacement {
	if !kind.valid() {
		return nil
	}
	var matched []IssuancePlacement
	for _, placement := range directory {
		if !placement.Available() || placement.Occurrence != kind {
			continue
		}
		if placement.HasCode && placement.Code != code {
			continue
		}
		matched = append(matched, placement)
	}
	return matched
}

func (compiler *compiler) applyIssuance(row OccurrenceRow, ordinal uint32, geometry occurrenceSpanGeometry, finish []identity.ContentID, placement IssuancePlacement) bool {
	if !placement.Available() {
		return false
	}
	if row.kind == OccurrenceValues && len(row.points) == 0 {
		return true
	}
	switch placement.Form {
	case IssuanceFormBase:
		return compiler.appendBaseIssuance(row, ordinal, finish, placement)
	case IssuanceFormLocal:
		return compiler.appendLocalIssuance(ordinal, geometry, finish, placement)
	case IssuanceFormComputation:
		return compiler.appendComputationIssuance(row, ordinal, finish, placement)
	case IssuanceFormLocalPredecessor:
		return compiler.appendLocalPredecessorIssuance(ordinal, geometry, finish, placement)
	case IssuanceFormCallStage:
		return compiler.appendCallStageIssuance(ordinal, finish, placement)
	default:
		return false
	}
}

func (compiler *compiler) appendBaseIssuance(row OccurrenceRow, ordinal uint32, finish []identity.ContentID, issued IssuancePlacement) bool {
	if len(finish) == 0 || issued.Input != RuleInputNone {
		return false
	}
	for _, point := range finish {
		placement := RuleOccurrence{key: issued.Key, occurrence: ordinal, point: point, stage: RuleStageBase, inputKind: issued.Input}
		if !placement.Available() {
			return false
		}
		compiler.ruleOccurrences = append(compiler.ruleOccurrences, placement)
	}
	return true
}

func (compiler *compiler) appendLocalIssuance(ordinal uint32, geometry occurrenceSpanGeometry, finish []identity.ContentID, issued IssuancePlacement) bool {
	if len(finish) == 0 || issued.Input == RuleInputNone || issued.Input == RuleInputPredecessor || issued.Input == RuleInputEntry && len(geometry.entry) != 1 {
		return false
	}
	for _, base := range finish {
		stage, stageOK := compiler.localStage(base)
		if !stageOK {
			return false
		}
		input := base
		if issued.Input == RuleInputEntry {
			input = geometry.entry[0]
		}
		placement := RuleOccurrence{key: issued.Key, occurrence: ordinal, point: stage, input: input, stage: RuleStageLocal, inputKind: issued.Input}
		if !placement.Available() {
			return false
		}
		compiler.ruleOccurrences = append(compiler.ruleOccurrences, placement)
	}
	return true
}

func (compiler *compiler) appendComputationIssuance(row OccurrenceRow, ordinal uint32, finish []identity.ContentID, issued IssuancePlacement) bool {
	if len(finish) == 0 || len(row.inputs) < 2 {
		return false
	}
	for _, base := range finish {
		stage, stageOK := compiler.localComputationStage(base, issued.Key, row.id, row.inputs[0], row.inputs[1])
		placement := RuleOccurrence{key: issued.Key, occurrence: ordinal, point: stage, input: base, stage: RuleStageLocal, inputKind: RuleInputFinish}
		if !stageOK || !placement.Available() {
			return false
		}
		compiler.ruleOccurrences = append(compiler.ruleOccurrences, placement)
	}
	return true
}

func (compiler *compiler) appendLocalPredecessorIssuance(ordinal uint32, geometry occurrenceSpanGeometry, finish []identity.ContentID, issued IssuancePlacement) bool {
	if !geometry.route.Available() {
		return false
	}
	if _, duplicate := compiler.environmentRouteDuplicates[geometry.route]; duplicate {
		return false
	}
	predecessor, found := compiler.environmentByRoute[geometry.route]
	if !found || !predecessor.Available() {
		return false
	}
	finishMember := false
	for _, point := range finish {
		if point == predecessor.to {
			finishMember = true
			break
		}
	}
	stage, stageOK := compiler.localStage(predecessor.to)
	placement := RuleOccurrence{key: issued.Key, occurrence: ordinal, point: stage, input: predecessor.from, stage: RuleStageLocal, inputKind: RuleInputPredecessor, route: geometry.route}
	if !finishMember || !stageOK || !placement.Available() {
		return false
	}
	compiler.ruleOccurrences = append(compiler.ruleOccurrences, placement)
	return true
}

func (compiler *compiler) appendCallStageIssuance(ordinal uint32, finish []identity.ContentID, issued IssuancePlacement) bool {
	if len(finish) == 0 || issued.Stage < RuleStageCallDispatch || issued.Stage > RuleStageCallEffect {
		return false
	}
	for _, base := range finish {
		stages, stagesOK := compiler.callStage(base)
		if !stagesOK {
			return false
		}
		point, input := stages.dispatch, base
		switch issued.Stage {
		case RuleStageCallSummary:
			point, input = stages.summary, stages.dispatch
		case RuleStageCallEffect:
			point, input = stages.effect, stages.summary
		}
		placement := RuleOccurrence{key: issued.Key, occurrence: ordinal, point: point, input: input, stage: issued.Stage, inputKind: RuleInputFinish}
		if !placement.Available() {
			return false
		}
		compiler.ruleOccurrences = append(compiler.ruleOccurrences, placement)
	}
	return true
}
