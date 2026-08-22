package composite

import "github.com/wippyai/go-lua/analysis/schema/axis"

// AdmissionStage names the pass of the mounted admission walk that refused.
// It is the walk's own vocabulary: which rule refused is the DiagnosticRule
// beside it, and why that rule refused is the rule's own erased evidence.
type AdmissionStage uint8

const (
	AdmissionStageNone AdmissionStage = iota
	// AdmissionStagePlacement is the sealed ingress placement walk: a mount,
	// occurrence, or point row the sealed Program does not publish. No rule has
	// been reached, so the verdict is the walk's rather than a rule's.
	AdmissionStagePlacement
	// AdmissionStageActivation is the activation lane refusing one occurrence.
	// The refusing rule supplies its own evidence, recovered by
	// AdmissionRejection at that rule's type.
	AdmissionStageActivation
	// AdmissionStageCapability is an ordinary mounted rule whose sealed
	// capability is absent, not mounted, or an activation capability on the
	// ordinary lane.
	AdmissionStageCapability
	// AdmissionStageConstruction is the engine's own program constructor
	// refusing a row this walk already admitted. The rule is recovered from the
	// engine's opaque role, and the walk holds no evidence of its own because
	// it raised no refusal.
	AdmissionStageConstruction
)

func (stage AdmissionStage) String() string {
	switch stage {
	case AdmissionStagePlacement:
		return "placement"
	case AdmissionStageActivation:
		return "activation"
	case AdmissionStageCapability:
		return "capability"
	case AdmissionStageConstruction:
		return "construction"
	default:
		return "none"
	}
}

// AdmissionFailure is the closed verdict of one refused mounted admission
// walk. It names the pass, the rule that pass reached, and carries that rule's
// own refusal evidence erased. The composition never reads the evidence, so no
// rule's refusal vocabulary enters this record.
type AdmissionFailure struct {
	Stage AdmissionStage
	Rule  DiagnosticRule

	reason axis.Cell
}

// RefusedAdmission is the verdict one refusing rule produces: the pass, the
// rule, and that rule's own evidence exactly as the rule erased it.
func RefusedAdmission(stage AdmissionStage, rule DiagnosticRule, reason axis.Cell) AdmissionFailure {
	if stage == AdmissionStageNone {
		return AdmissionFailure{}
	}
	return AdmissionFailure{Stage: stage, Rule: rule, reason: reason}
}

// RefusedAdmissionRule is the verdict of a pass that reached one rule and
// carries no rule-owned evidence.
func RefusedAdmissionRule(stage AdmissionStage, rule DiagnosticRule) AdmissionFailure {
	return RefusedAdmission(stage, rule, axis.Cell{})
}

func (failure AdmissionFailure) Available() bool { return failure.Stage != AdmissionStageNone }

func (failure AdmissionFailure) String() string {
	if !failure.Available() {
		return "none"
	}
	return failure.Stage.String() + "/" + failure.Rule.String()
}

// AdmissionRejection recovers a refusing rule's own evidence at that rule's
// type. A caller that names the wrong type, or one that asks a verdict which
// carries no rule evidence, receives the absent value rather than a guess.
func AdmissionRejection[R any](failure AdmissionFailure) (R, bool) {
	var absent R
	if !failure.Available() || !failure.reason.Available() {
		return absent, false
	}
	return axis.Payload[R](failure.reason)
}
