package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	Occurrence  programschema.OccurrenceKind
	Form        IssuanceForm
	Input       programschema.RuleInputKind
	Stage       programschema.RuleStage
	Requirement IssuanceRequirement
	Code        uint64
	HasCode     bool
	Key         schema.Key
	Writes      schema.Key
	Transport   bool
}

func (placement IssuancePlacement) Available() bool {
	return placement.Occurrence.Valid() && placement.Form.valid() &&
		placement.Input.Valid() && placement.Stage.Valid() &&
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
	ruleStageLimit    = programschema.RuleStageCallEffect + 1
)

// NewIssuanceDirectory admits one sealed catalog. A framing names exactly one
// cut, so two members declaring one framing are refused rather than sealed into
// a catalog that stages two cuts onto one point.
func NewIssuanceDirectory(placements []IssuancePlacement, formFraming map[IssuanceForm]string, stageFraming map[programschema.RuleStage]string) (IssuanceDirectory, bool) {
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
		if !stage.Valid() || framing == "" {
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
func (directory IssuanceDirectory) stageFraming(stage programschema.RuleStage) (string, bool) {
	if !stage.Valid() {
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
func (directory IssuanceDirectory) stageAxes(stage programschema.RuleStage) (map[schema.Key]struct{}, bool) {
	if !stage.Valid() {
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
	effectAxes, effectOK := directory.stageAxes(programschema.RuleStageCallEffect)
	dispatchAxes, dispatchOK := directory.stageAxes(programschema.RuleStageCallDispatch)
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
func (compiler *compiler) matching(row programschema.Occurrence) ([]IssuancePlacement, bool) {
	if !row.Kind().Valid() {
		return nil, false
	}
	var matched []IssuancePlacement
	for _, placement := range compiler.issuance.placements {
		if !placement.Available() || placement.Occurrence != row.Kind() {
			continue
		}
		if placement.HasCode && placement.Code != row.Code() {
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
func (compiler *compiler) requirementAdmits(requirement IssuanceRequirement, row programschema.Occurrence) (bool, bool) {
	switch requirement {
	case IssuanceRequirementUnrestricted:
		return true, true
	case IssuanceRequirementCallPlainUnary:
		call, found := compiler.callForID(row.ID())
		if !found {
			return false, false
		}
		_, hasReceiver := call.ReceiverID()
		_, hasTail := call.TailID()
		return call.Form() == programschema.CallFormPlain && call.ArgumentCount() == 1 && !hasReceiver && !hasTail, true
	default:
		return false, false
	}
}

// callForID resolves one authored call row by the parent-issued identity an
// occurrence row carries. The canonical Call column is the sole authority;
// this construction-only lookup scans it directly and retains no inverse.
func (compiler *compiler) callForID(id identity.ContentID) (programschema.Call, bool) {
	if compiler == nil || !id.Available() {
		return programschema.Call{}, false
	}
	var found programschema.Call
	for _, row := range compiler.calls {
		if !row.Available() || row.ID() != id {
			continue
		}
		if found.Available() {
			return programschema.Call{}, false
		}
		found = row
	}
	return found, found.Available()
}

func (compiler *compiler) applyIssuance(row programschema.Occurrence, ordinal uint32, geometry occurrenceSpanGeometry, finish []identity.ContentID, placement IssuancePlacement) bool {
	if !placement.Available() {
		return false
	}
	if row.Kind() == programschema.OccurrenceValues {
		_, pointCount, pointOK := row.PointSpan()
		if !pointOK {
			return false
		}
		if pointCount == 0 {
			return true
		}
	}
	switch placement.Form {
	case IssuanceFormBase:
		return compiler.appendBaseIssuance(ordinal, finish, placement)
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

func (compiler *compiler) appendBaseIssuance(ordinal uint32, finish []identity.ContentID, issued IssuancePlacement) bool {
	if len(finish) == 0 || issued.Input != programschema.RuleInputNone {
		return false
	}
	for _, point := range finish {
		if !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, point, identity.ContentID{}, programschema.RuleStageBase, issued.Input, identity.ContentID{}) {
			return false
		}
	}
	return true
}

func (compiler *compiler) appendLocalIssuance(ordinal uint32, geometry occurrenceSpanGeometry, finish []identity.ContentID, issued IssuancePlacement) bool {
	if len(finish) == 0 || issued.Input == programschema.RuleInputNone || issued.Input == programschema.RuleInputPredecessor || issued.Input == programschema.RuleInputEntry && len(geometry.entry) != 1 {
		return false
	}
	for _, base := range finish {
		stage, stageOK := compiler.localStage(base)
		if !stageOK {
			return false
		}
		input := base
		if issued.Input == programschema.RuleInputEntry {
			input = geometry.entry[0]
		}
		if !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, stage, input, programschema.RuleStageLocal, issued.Input, identity.ContentID{}) {
			return false
		}
	}
	return true
}

func (compiler *compiler) appendComputationIssuance(row programschema.Occurrence, ordinal uint32, finish []identity.ContentID, issued IssuancePlacement) bool {
	left, leftOK := occurrenceInputID(row, compiler.occurrenceInputs, 0)
	right, rightOK := occurrenceInputID(row, compiler.occurrenceInputs, 1)
	if len(finish) == 0 || !leftOK || !rightOK {
		return false
	}
	for _, base := range finish {
		stage, stageOK := compiler.localComputationStage(base, issued.Key, row.ID(), left, right)
		if !stageOK || !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, stage, base, programschema.RuleStageLocal, programschema.RuleInputFinish, identity.ContentID{}) {
			return false
		}
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
	if !finishMember || !stageOK || !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, stage, predecessor.to, programschema.RuleStageLocal, programschema.RuleInputPredecessor, geometry.route) {
		return false
	}
	return true
}

func (compiler *compiler) appendCallStageIssuance(ordinal uint32, finish []identity.ContentID, issued IssuancePlacement) bool {
	if len(finish) == 0 || issued.Stage < programschema.RuleStageCallDispatch || issued.Stage > programschema.RuleStageCallEffect {
		return false
	}
	for _, base := range finish {
		stages, stagesOK := compiler.callStage(base)
		if !stagesOK {
			return false
		}
		point, input := stages.dispatch, base
		switch issued.Stage {
		case programschema.RuleStageCallSummary:
			point, input = stages.summary, stages.dispatch
		case programschema.RuleStageCallEffect:
			point, input = stages.effect, stages.summary
		}
		if !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, point, input, issued.Stage, programschema.RuleInputFinish, identity.ContentID{}) {
			return false
		}
	}
	return true
}
