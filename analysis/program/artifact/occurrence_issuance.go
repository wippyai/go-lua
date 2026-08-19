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

// IssuanceRequirement is the declared operand shape one subscription consumes.
// Ordinals match structure.CategoryIssuanceRequirement: unrestricted,
// call-plain-unary.
//
// The requirement is the placement half of one denominator. A rule's owner
// seals an operand for the rows carrying the shape it interprets; the same
// shape is stated here, so the compiler places exactly those rows and no
// construction pass has to discover the difference.
type IssuanceRequirement uint8

const (
	IssuanceRequirementInvalid IssuanceRequirement = iota
	// IssuanceRequirementUnrestricted consumes every row of the occurrence
	// family the subscription names.
	IssuanceRequirementUnrestricted
	// IssuanceRequirementCallPlainUnary consumes an authored call of the strict
	// unary plain shape: plain form, exactly one positional argument, no
	// receiver, and no tail expansion.
	IssuanceRequirementCallPlainUnary
)

func (requirement IssuanceRequirement) valid() bool {
	return requirement >= IssuanceRequirementUnrestricted && requirement <= IssuanceRequirementCallPlainUnary
}

// IssuancePlacement is one sealed rule.Issues row as the compiler places it.
// Key is the declaration identity.
type IssuancePlacement struct {
	Occurrence  OccurrenceKind
	Form        IssuanceForm
	Input       RuleInputKind
	Stage       RuleStage
	Requirement IssuanceRequirement
	Code        uint64
	HasCode     bool
	Key         schema.Key
	Writes      schema.Key
	Transport   bool
}

func (placement IssuancePlacement) Available() bool {
	return placement.Occurrence.valid() && placement.Form.valid() &&
		placement.Input.valid() && placement.Stage.valid() &&
		placement.Requirement.valid() &&
		placement.Key.Available() && placement.Writes.Available()
}

// IssuanceDirectory is the sealed catalog the compiler compiles from: every
// subscription the mounted rules declare, and the digest framing of every
// staged execution cut those placements raise. It is built at the composition
// root from rule.Issues and the sealed structure table.
//
// The framings are content-address preimages, so they are declared beside the
// member that names the cut rather than authored here: a cut inside the local
// stage is named by its placement form, and the three native call cuts by their
// own issuance stage.
type IssuanceDirectory struct {
	placements []IssuancePlacement
	forms      [issuanceFormLimit]string
	stages     [ruleStageLimit]string
}

const (
	issuanceFormLimit = IssuanceFormCallStage + 1
	ruleStageLimit    = RuleStageCallEffect + 1
)

// NewIssuanceDirectory admits one sealed catalog. A framing names exactly one
// cut, so two members declaring one framing are refused rather than sealed into
// a catalog that stages two cuts onto one point.
func NewIssuanceDirectory(placements []IssuancePlacement, formFraming map[IssuanceForm]string, stageFraming map[RuleStage]string) (IssuanceDirectory, bool) {
	directory := IssuanceDirectory{placements: append([]IssuancePlacement(nil), placements...)}
	for _, placement := range directory.placements {
		if !placement.Available() {
			return IssuanceDirectory{}, false
		}
	}
	declared := make(map[string]struct{}, len(formFraming)+len(stageFraming))
	for form, framing := range formFraming {
		if !form.valid() || framing == "" {
			return IssuanceDirectory{}, false
		}
		if _, duplicate := declared[framing]; duplicate {
			return IssuanceDirectory{}, false
		}
		declared[framing] = struct{}{}
		directory.forms[form] = framing
	}
	for stage, framing := range stageFraming {
		if !stage.valid() || framing == "" {
			return IssuanceDirectory{}, false
		}
		if _, duplicate := declared[framing]; duplicate {
			return IssuanceDirectory{}, false
		}
		declared[framing] = struct{}{}
		directory.stages[stage] = framing
	}
	return directory, true
}

// Count is the number of admitted placements.
func (directory IssuanceDirectory) Count() int { return len(directory.placements) }

// At returns one admitted placement in declaration order.
func (directory IssuanceDirectory) At(index int) (IssuancePlacement, bool) {
	if index < 0 || index >= len(directory.placements) {
		return IssuancePlacement{}, false
	}
	return directory.placements[index], true
}

// formFraming is the declared digest framing of the cut one placement form
// raises inside the local stage.
func (directory IssuanceDirectory) formFraming(form IssuanceForm) (string, bool) {
	if !form.valid() {
		return "", false
	}
	framing := directory.forms[form]
	return framing, framing != ""
}

// stageFraming is the declared digest framing of the cut one native call stage
// raises.
func (directory IssuanceDirectory) stageFraming(stage RuleStage) (string, bool) {
	if !stage.valid() {
		return "", false
	}
	framing := directory.stages[stage]
	return framing, framing != ""
}

func (directory IssuanceDirectory) writesFor(key schema.Key) (schema.Key, bool) {
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

// transportRepresentatives maps every mounted factor axis to the one declared
// rule key that names it in a factor transport. The key is a naming device:
// construction resolves it to the factor its rule writes, so every declared
// writer of an axis states the same transport and the lowest key makes the
// choice a property of the declarations rather than an authored preference.
func (directory IssuanceDirectory) transportRepresentatives() (map[schema.Key]schema.Key, bool) {
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
func (directory IssuanceDirectory) TransportKey(axis schema.Key) (schema.Key, bool) {
	representative, ok := directory.transportRepresentatives()
	if !ok || !axis.Available() {
		return "", false
	}
	key := representative[axis]
	return key, key.Available()
}

// transportKeysExcept returns one deterministic declared rule key for every
// mounted factor axis except those a strong-write stage produces itself. The
// keys remain schema vocabulary; Program never resolves them to engine slots.
func (directory IssuanceDirectory) transportKeysExcept(excluded map[schema.Key]struct{}) ([]schema.Key, bool) {
	representative, ok := directory.transportRepresentatives()
	if !ok {
		return nil, false
	}
	keys := make([]schema.Key, 0, len(representative))
	for axis, key := range representative {
		if _, omitted := excluded[axis]; omitted {
			continue
		}
		keys = append(keys, key)
	}
	ordered, orderedOK := orderedWrites(keys)
	return ordered, orderedOK
}

// transportKeysFor is the positive form: the declared transport key for each
// named mounted factor axis. An axis no mounted rule writes has no transport
// key, and the plan that asked for it is refused rather than thinned.
func (directory IssuanceDirectory) transportKeysFor(included map[schema.Key]struct{}) ([]schema.Key, bool) {
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
	ordered, orderedOK := orderedWrites(keys)
	return ordered, orderedOK
}

// stageAxes is the mounted factor axis set the rules issued at one execution
// stage produce.
func (directory IssuanceDirectory) stageAxes(stage RuleStage) (map[schema.Key]struct{}, bool) {
	if !stage.valid() {
		return nil, false
	}
	axes := make(map[schema.Key]struct{})
	for _, placement := range directory.placements {
		if !placement.Available() {
			return nil, false
		}
		if !placement.Transport || placement.Stage != stage {
			continue
		}
		axes[placement.Writes] = struct{}{}
	}
	return axes, true
}

// callStageTransport is the declared factor transport plan for one call-stage
// triple, derived from the issuance directory alone.
//
// A stage reads the pre-write state of the axis it writes. The effect stage is
// the last of the three, so the axis it produces bypasses dispatch and reaches
// the summary stage straight from the base; every other mounted axis enters the
// triple at dispatch. The axis the dispatch stage produces is then carried
// forward to both of its successors, so they read the dispatched state instead
// of the base's.
type callStageTransport struct {
	dispatchEntry   []schema.Key
	effectBypass    []schema.Key
	dispatchForward []schema.Key
}

func (directory IssuanceDirectory) callStageTransport() (callStageTransport, bool) {
	effectAxes, effectOK := directory.stageAxes(RuleStageCallEffect)
	dispatchAxes, dispatchOK := directory.stageAxes(RuleStageCallDispatch)
	if !effectOK || !dispatchOK || len(effectAxes) == 0 || len(dispatchAxes) == 0 {
		return callStageTransport{}, false
	}
	entry, entryOK := directory.transportKeysExcept(effectAxes)
	bypass, bypassOK := directory.transportKeysFor(effectAxes)
	forward, forwardOK := directory.transportKeysFor(dispatchAxes)
	if !entryOK || !bypassOK || !forwardOK || len(entry) == 0 || len(bypass) == 0 || len(forward) == 0 {
		return callStageTransport{}, false
	}
	return callStageTransport{dispatchEntry: entry, effectBypass: bypass, dispatchForward: forward}, true
}

// matching selects the subscriptions one compiled occurrence row issues: the
// family it belongs to, the payload code it carries, and the operand shape the
// row itself has. The requirement is decided here rather than after placement,
// so a row an owner cannot seal an operand for is never placed.
func (compiler *compiler) matching(row OccurrenceRow) ([]IssuancePlacement, bool) {
	if !row.kind.valid() {
		return nil, false
	}
	var matched []IssuancePlacement
	for _, placement := range compiler.issuance.placements {
		if !placement.Available() || placement.Occurrence != row.kind {
			continue
		}
		if placement.HasCode && placement.Code != row.code {
			continue
		}
		admissible, decided := compiler.requirementAdmits(placement.Requirement, row)
		if !decided {
			return nil, false
		}
		if !admissible {
			continue
		}
		matched = append(matched, placement)
	}
	return matched, true
}

// requirementAdmits decides one declared operand shape against one compiled
// row. The second result is whether the shape could be decided at all: a
// requirement naming a geometry the row's family does not carry is a
// declaration the artifact cannot honor, and it refuses the compile rather
// than placing the row on an unstated reading.
func (compiler *compiler) requirementAdmits(requirement IssuanceRequirement, row OccurrenceRow) (bool, bool) {
	switch requirement {
	case IssuanceRequirementUnrestricted:
		return true, true
	case IssuanceRequirementCallPlainUnary:
		call, found := compiler.callForID(row.id)
		if !found {
			return false, false
		}
		_, hasReceiver := call.ReceiverID()
		_, hasTail := call.TailID()
		return call.Form() == CallFormPlain && call.ArgumentCount() == 1 && !hasReceiver && !hasTail, true
	default:
		return false, false
	}
}

// callForID resolves one authored call row by the parent-issued identity an
// occurrence row carries. The inverse is built once for the whole occurrence
// walk, so deciding a requirement stays constant-time per row.
func (compiler *compiler) callForID(id identity.ContentID) (CallRow, bool) {
	if !id.Available() {
		return CallRow{}, false
	}
	if compiler.callsByID == nil {
		compiler.callsByID = make(map[identity.ContentID]CallRow, len(compiler.calls))
		for _, row := range compiler.calls {
			if row.Available() {
				compiler.callsByID[row.ID()] = row
			}
		}
	}
	row, found := compiler.callsByID[id]
	return row, found
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
	stage, stageOK := compiler.predecessorStage(predecessor.to)
	placement := RuleOccurrence{key: issued.Key, occurrence: ordinal, point: stage, input: predecessor.to, stage: RuleStageLocal, inputKind: RuleInputPredecessor, route: geometry.route}
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
