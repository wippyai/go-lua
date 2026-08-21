// Package issuance owns the declaration-derived placement directory consumed
// by the artifact compiler. It has no dependency on compiler or artifact.
package issuance

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// Form is the placement form one occurrence subscription takes. Ordinals
// match structure.CategoryIssuanceForm.
type Form uint8

const (
	FormInvalid Form = iota
	FormBase
	FormLocal
	FormComputation
	FormLocalPredecessor
	FormCallStage
	// FormLocalSuccessor places a consumer after the ordinary Local cut. It is
	// the declarative seam for rules that must observe sibling Local producers;
	// rules at one cut continue to read one immutable incoming state.
	FormLocalSuccessor
)

func (form Form) valid() bool { return form >= FormBase && form <= FormLocalSuccessor }

// Requirement is the declared operand shape one subscription consumes.
// Ordinals match structure.CategoryIssuanceRequirement.
type Requirement uint8

const (
	RequirementInvalid Requirement = iota
	RequirementUnrestricted
	RequirementCallPlainUnary
	RequirementClosureCapture
	// RequirementCallResultSlot admits a strict plain unary Call only when
	// Program publishes finite ordinal zero with a consumer-backed ValueID.
	RequirementCallResultSlot
)

func (requirement Requirement) valid() bool {
	return requirement >= RequirementUnrestricted && requirement <= RequirementCallResultSlot
}

// Placement is one sealed rule.Issues row. Key is the declaration identity.
type Placement struct {
	Occurrence  programschema.OccurrenceKind
	Form        Form
	Input       programschema.RuleInputKind
	Stage       programschema.RuleStage
	Requirement Requirement
	Code        uint64
	HasCode     bool
	Key         schema.Key
	Writes      schema.Key
	Transport   bool
}

func (placement Placement) Available() bool {
	return placement.Occurrence.Valid() && placement.Form.valid() &&
		placement.Input.Valid() && placement.Stage.Valid() &&
		placement.Requirement.valid() && placement.Key.Available() && placement.Writes.Available()
}

// Directory is the sealed catalog of declaration-owned placements and staged
// execution-cut framings. It copies every caller-provided slice or map.
type Directory struct {
	placements []Placement
	forms      [FormLocalSuccessor + 1]string
	stages     [programschema.RuleStageCallEffect + 1]string
}

// NewDirectory admits one sealed placement catalog. A framing may name only
// one cut, so duplicate framings are refused.
func NewDirectory(placements []Placement, formFraming map[Form]string, stageFraming map[programschema.RuleStage]string) (Directory, bool) {
	directory := Directory{placements: append([]Placement(nil), placements...)}
	for _, placement := range directory.placements {
		if !placement.Available() {
			return Directory{}, false
		}
	}
	declared := make(map[string]struct{}, len(formFraming)+len(stageFraming))
	for form, framing := range formFraming {
		if !form.valid() || framing == "" {
			return Directory{}, false
		}
		if _, duplicate := declared[framing]; duplicate {
			return Directory{}, false
		}
		declared[framing] = struct{}{}
		directory.forms[form] = framing
	}
	for stage, framing := range stageFraming {
		if !stage.Valid() || framing == "" {
			return Directory{}, false
		}
		if _, duplicate := declared[framing]; duplicate {
			return Directory{}, false
		}
		declared[framing] = struct{}{}
		directory.stages[stage] = framing
	}
	return directory, true
}

// Count is the number of admitted placements.
func (directory Directory) Count() int { return len(directory.placements) }

// At returns one admitted placement in declaration order.
func (directory Directory) At(index int) (Placement, bool) {
	if index < 0 || index >= len(directory.placements) {
		return Placement{}, false
	}
	return directory.placements[index], true
}

// FormFraming returns the declaration-owned digest framing of one local cut.
func (directory Directory) FormFraming(form Form) (string, bool) {
	if !form.valid() {
		return "", false
	}
	framing := directory.forms[form]
	return framing, framing != ""
}

// StageFraming returns the declaration-owned digest framing of one call cut.
func (directory Directory) StageFraming(stage programschema.RuleStage) (string, bool) {
	if !stage.Valid() {
		return "", false
	}
	framing := directory.stages[stage]
	return framing, framing != ""
}

// WritesFor resolves the one output axis each declaration key consistently
// names. A key with inconsistent declared outputs is refused.
func (directory Directory) WritesFor(key schema.Key) (schema.Key, bool) {
	if !key.Available() {
		return "", false
	}
	var writes schema.Key
	for _, placement := range directory.placements {
		if !placement.Available() {
			return "", false
		}
		if placement.Key != key {
			continue
		}
		if writes.Available() && writes != placement.Writes {
			return "", false
		}
		writes = placement.Writes
	}
	return writes, writes.Available()
}

func (directory Directory) transportRepresentatives() (map[schema.Key]schema.Key, bool) {
	representative := make(map[schema.Key]schema.Key)
	for _, placement := range directory.placements {
		if !placement.Available() {
			return nil, false
		}
		if !placement.Transport {
			continue
		}
		prior := representative[placement.Writes]
		if !prior.Available() || placement.Key < prior {
			representative[placement.Writes] = placement.Key
		}
	}
	return representative, true
}

// TransportKey is the declared rule key naming one mounted factor axis in a
// factor transport.
func (directory Directory) TransportKey(axis schema.Key) (schema.Key, bool) {
	representative, ok := directory.transportRepresentatives()
	if !ok || !axis.Available() {
		return "", false
	}
	key := representative[axis]
	return key, key.Available()
}

// OrderedKeys returns a copy of keys in canonical ascending order. It refuses
// unavailable or duplicate keys so identity-bearing transports have one
// representation for each declared key set.
func OrderedKeys(keys []schema.Key) ([]schema.Key, bool) {
	if len(keys) == 0 {
		return nil, true
	}
	ordered := append([]schema.Key(nil), keys...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	for index, key := range ordered {
		if !key.Available() || index != 0 && ordered[index-1] >= key {
			return nil, false
		}
	}
	return ordered, true
}

// TransportKeysExcept returns one deterministic transport key for every
// mounted axis except the supplied strong-write axes.
func (directory Directory) TransportKeysExcept(excluded map[schema.Key]struct{}) ([]schema.Key, bool) {
	representative, ok := directory.transportRepresentatives()
	if !ok {
		return nil, false
	}
	keys := make([]schema.Key, 0, len(representative))
	for axis, key := range representative {
		if _, omitted := excluded[axis]; !omitted {
			keys = append(keys, key)
		}
	}
	return OrderedKeys(keys)
}

func (directory Directory) transportKeysFor(included map[schema.Key]struct{}) ([]schema.Key, bool) {
	representative, ok := directory.transportRepresentatives()
	if !ok {
		return nil, false
	}
	keys := make([]schema.Key, 0, len(included))
	for axis := range included {
		key := representative[axis]
		if !key.Available() {
			return nil, false
		}
		keys = append(keys, key)
	}
	return OrderedKeys(keys)
}

func (directory Directory) stageAxes(stage programschema.RuleStage) (map[schema.Key]struct{}, bool) {
	if !stage.Valid() {
		return nil, false
	}
	axes := make(map[schema.Key]struct{})
	for _, placement := range directory.placements {
		if !placement.Available() {
			return nil, false
		}
		// Every rule at the cut contributes a strong-write axis that its
		// successor must observe. Transport marks the declaration that may
		// represent that axis on an edge; it does not make writes by other
		// declarations disappear. In particular, call activation writes the
		// Call axis at Summary while the dispatch declaration supplies the
		// canonical transport key for that same axis.
		if placement.Stage == stage {
			axes[placement.Writes] = struct{}{}
		}
	}
	return axes, true
}

// CallStageTransport derives the dispatch-entry, effect-bypass,
// dispatch-forward, and summary-forward plans from declarations. Summary
// writes must reach the effect cut: it is the declared successor of summary
// and the continuation departs effect, so omitting that edge loses call-result
// values before an ordinary local bind can observe them. Returned slices are
// copies.
func (directory Directory) CallStageTransport() (dispatchEntry, effectEntry, effectBypass, dispatchForward, summaryForward []schema.Key, ok bool) {
	effectAxes, effectOK := directory.stageAxes(programschema.RuleStageCallEffect)
	dispatchAxes, dispatchOK := directory.stageAxes(programschema.RuleStageCallDispatch)
	summaryAxes, summaryOK := directory.stageAxes(programschema.RuleStageCallSummary)
	if !effectOK || !dispatchOK || !summaryOK || len(effectAxes) == 0 || len(dispatchAxes) == 0 || len(summaryAxes) == 0 {
		return nil, nil, nil, nil, nil, false
	}
	// Dispatch is the strong writer at the first call cut. Its own axes must
	// not carry stale base facts into dispatch; effect writers are later and
	// may still be required as dispatch/summary inputs (Value is the canonical
	// example for a post-summary slot-to-Cell transfer).
	entry, entryOK := directory.TransportKeysExcept(dispatchAxes)
	effectEntryPlan, effectEntryOK := directory.TransportKeysExcept(effectAxes)
	bypass, bypassOK := directory.transportKeysFor(effectAxes)
	forward, forwardOK := directory.transportKeysFor(dispatchAxes)
	summaryEffectAxes := make(map[schema.Key]struct{}, len(summaryAxes)+len(effectAxes))
	for axis := range summaryAxes {
		summaryEffectAxes[axis] = struct{}{}
	}
	for axis := range effectAxes {
		summaryEffectAxes[axis] = struct{}{}
	}
	summary, summaryForwardOK := directory.transportKeysFor(summaryEffectAxes)
	if !entryOK || !effectEntryOK || !bypassOK || !forwardOK || !summaryForwardOK || len(entry) == 0 || len(effectEntryPlan) == 0 || len(bypass) == 0 || len(forward) == 0 || len(summary) == 0 {
		return nil, nil, nil, nil, nil, false
	}
	return append([]schema.Key(nil), entry...), append([]schema.Key(nil), effectEntryPlan...), append([]schema.Key(nil), bypass...), append([]schema.Key(nil), forward...), append([]schema.Key(nil), summary...), true
}
