package engine

import (
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

type ruleInstanceKind uint8

const (
	ruleInstanceDirect ruleInstanceKind = iota + 1
	ruleInstanceActivation
)

// RuleInstance is one domain-owned, cold, one-shot binding of a typed Rule
// operand to its canonical source identity and complete declared surface.
// The instance is consumed atomically by assembly: neither O nor a partial
// read/write vector is published when its declaration fails.
type RuleInstance[V, O any] struct {
	rule    *Rule[V, O]
	operand O
	content [32]byte
	declare func(*RuleBinding[V, O]) bool
	kind    ruleInstanceKind
	state   *ruleInstanceState
}

type ruleInstanceState struct {
	used    atomic.Bool
	admitMu sync.Mutex
	// Admission is a cold single-writer transaction. An overlapping claimant
	// poisons the transaction so scheduling can never choose one Batch as the
	// semantic winner.
	admitBusy    bool
	admitClosed  bool
	admitOverlap bool
	admit        *ruleInstanceAdmission
	// activationPlan/activationRow are the one irreversible attachment claim
	// for a shared-template payload.  The same admitted typed instance cannot
	// be replayed into another plan or another variant before Assembly binds.
	activationPlan *activationVariantPlan
	activationRow  composition.Key
	// activationPrototype is the exact equation row obtained by executing this
	// instance's typed declaration once after its Batch has sealed. Attachment
	// must match this row byte-for-byte; a source compiler cannot admit one
	// typed instance and later substitute a different read/write surface.
	activationPrototype       *equation.RuleInstance
	activationPrototypePorts  []equation.Port
	activationPrototypeClosed bool
}

type ruleInstanceAdmission struct {
	batch      *equation.Batch
	occurrence equation.Occurrence
	operand    equation.Operand
}

// RuleBinding is the short-lived capability passed only to the owning
// domain's RuleInstance declaration. It is invalid before and after that one
// callback, so retained callbacks cannot mutate an accepted instance.
type RuleBinding[V, O any] struct {
	assembly  *Assembly
	member    *assemblyMember
	rule      *Rule[V, O]
	gate      *coldGate
	prototype bool
}

// NewRuleInstance records the complete source-owned premise for one Rule
// occurrence. The Rule-owned OperandContent law supplies the only retained O
// and its canonical content identity; callers cannot pair them independently.
func NewRuleInstance[V, O any](rule *Rule[V, O], operand O, declare func(*RuleBinding[V, O]) bool) (*RuleInstance[V, O], bool) {
	if rule == nil || rule.schema == nil || !rule.available() || rule.operandContent == nil || declare == nil {
		return nil, false
	}
	frozen, content, ok := rule.operandContent(operand)
	if !ok || content == [32]byte{} {
		return nil, false
	}
	return &RuleInstance[V, O]{rule: rule, operand: frozen, content: content, declare: declare, kind: ruleInstanceDirect, state: &ruleInstanceState{}}, true
}

// NewActivationPrototypeInstance is the engine-descendant source compiler's
// friend constructor for one shared variant Template row. The caller must
// admit this exact returned instance through SourceAssembly.PrepareInstance
// before placing that issued occurrence/operand in equation.Template, then attach this same
// instance to the sealed ActivationPlan.  It is deliberately not a domain
// registration path and never creates a second payload at attachment time.
func NewActivationPrototypeInstance[V, O any](rule *Rule[V, O], operand O, declare func(*RuleBinding[V, O]) bool) (*RuleInstance[V, O], bool) {
	if rule == nil || rule.schema == nil || !rule.available() || rule.operandContent == nil || declare == nil {
		return nil, false
	}
	frozen, content, contentOK := rule.operandContent(operand)
	if !contentOK || content == [32]byte{} {
		return nil, false
	}
	return &RuleInstance[V, O]{rule: rule, operand: frozen, content: content, declare: declare, kind: ruleInstanceActivation, state: &ruleInstanceState{}}, true
}

// StructuralInstance is the one-shot surface binding for a Support or
// activation-trigger Rule. Both are output-free; a trigger owns one shared
// target-indexed variant attachment.
type StructuralInstance struct {
	rule    *ruleSchema
	variant *activationVariantAttachment
	declare func(*StructuralBinding) bool
	state   *structuralInstanceState
}

type activationVariantAttachment struct {
	application SemanticKey
	plan        *activationVariantPlan
	ports       []activationPortBinding
}

// activationPortBinding is an engine-issued attachment endpoint.  It retains
// the Assembly point capability until Assembly commits the one topology
// transaction, where that exact point is lowered to equation's PointRef.
type activationPortBinding struct {
	role  composition.Key
	base  ActivationBase
	reads []equation.PortRead
}

type activationPortRead interface {
	activationPortRead() (SemanticKey, ActivationBase, SemanticKey, equation.Surface, bool)
}

type typedActivationPortRead[K ~uint32 | ~uint64, V any] struct {
	role   SemanticKey
	base   ActivationBase
	slot   SemanticKey
	factor *Factor[K, V]
	ref    Ref[K]
}

func (value typedActivationPortRead[K, V]) activationPortRead() (SemanticKey, ActivationBase, SemanticKey, equation.Surface, bool) {
	if !value.role.Available() || !value.base.available() || !value.slot.Available() || value.factor == nil || value.factor.schema == nil {
		return SemanticKey{}, ActivationBase{}, SemanticKey{}, equation.Surface{}, false
	}
	surface, ok := value.ref.exactWaveESurface(value.factor.schema, equation.SurfaceReadExact, equation.TargetModeNone)
	return value.role, value.base, value.slot, surface, ok
}

// activationPortReadOf records one typed caller Ref for the role/base it must
// share with every slot in its attachment.  It has no raw-surface form.
func activationPortReadOf[K ~uint32 | ~uint64, V any](role SemanticKey, base ActivationBase, slot SemanticKey, factor *Factor[K, V], ref Ref[K]) activationPortRead {
	return typedActivationPortRead[K, V]{role: role, base: base, slot: slot, factor: factor, ref: ref}
}

// activationPortBindingOf is the sole collector for a caller endpoint.  It
// accepts heterogeneous Factor refs, canonicalizes slots by semantic key, and
// rejects a slot that names a different role/base or whose typed Ref does not
// belong to its claimed Factor.  Zero slots is the explicit control-only port.
func activationPortBindingOf(role SemanticKey, base ActivationBase, reads ...activationPortRead) (activationPortBinding, bool) {
	if !role.Available() || !base.available() {
		return activationPortBinding{}, false
	}
	bound := activationPortBinding{role: role.compositionKey(), base: base, reads: make([]equation.PortRead, len(reads))}
	for index, read := range reads {
		if read == nil {
			return activationPortBinding{}, false
		}
		readRole, readBase, slot, surface, ok := read.activationPortRead()
		if !ok || readRole != role || readBase != base || !slot.Available() {
			return activationPortBinding{}, false
		}
		bound.reads[index] = equation.PortRead{Role: slot.compositionKey(), Surface: surface}
	}
	for index := range bound.reads {
		for next := index + 1; next < len(bound.reads); next++ {
			if compareSemanticKey(semanticKeyFromComposition(bound.reads[next].Role), semanticKeyFromComposition(bound.reads[index].Role)) < 0 {
				bound.reads[index], bound.reads[next] = bound.reads[next], bound.reads[index]
			}
		}
		if index != 0 && bound.reads[index-1].Role == bound.reads[index].Role {
			return activationPortBinding{}, false
		}
	}
	return bound, true
}

func activationEquationPortBindings(assembly *Assembly, pointAt map[*assemblyPoint]int, values []activationPortBinding) ([]equation.PortBinding, bool) {
	if !validAssembly(assembly) {
		return nil, false
	}
	result := make([]equation.PortBinding, len(values))
	for index, value := range values {
		if !value.base.belongsTo(assembly) {
			return nil, false
		}
		point, present := pointAt[value.base.point]
		if !present {
			return nil, false
		}
		result[index] = equation.PortBinding{Role: value.role, Base: equation.PointAt(point), Reads: append([]equation.PortRead(nil), value.reads...)}
	}
	return result, true
}

type structuralInstanceState struct {
	used    atomic.Bool
	admitMu sync.Mutex
	// Structural admission follows the same fail-closed single-writer law as
	// typed RuleInstance admission. An overlapping claimant cannot make one
	// SourceAssembly win nondeterministically.
	admitBusy    bool
	admitClosed  bool
	admitOverlap bool
	admit        *structuralInstanceAdmission
}

type structuralInstanceAdmission struct {
	batch      *equation.Batch
	occurrence equation.Occurrence
	operand    equation.Operand
}

type StructuralBinding struct {
	assembly *Assembly
	member   *assemblyMember
	gate     *coldGate
}

func NewSupportInstance(rule *SupportRule, declare func(*StructuralBinding) bool) (*StructuralInstance, bool) {
	if rule == nil || rule.schema == nil || !rule.available() || rule.schema.support == nil || declare == nil {
		return nil, false
	}
	return &StructuralInstance{rule: rule.schema, declare: declare, state: &structuralInstanceState{}}, true
}

// newVariantActivationRuleInstance is the shared-plan trigger attachment.
// It has no selected Target/Endpoint and no callback: the trigger supplies
// only its existing Application, while equation selects a presealed variant
// after an exact Member has been accepted.
func newVariantActivationRuleInstance(rule *ActivationRule, application SemanticKey, plan *activationVariantPlan, ports []activationPortBinding, declare func(*StructuralBinding) bool) (*StructuralInstance, bool) {
	if rule == nil || rule.schema == nil || !rule.available() || rule.schema.activation == nil || rule.schema.activation.family == nil || !application.Available() || plan == nil || plan.plan == (equation.VariantPlan{}) || declare == nil {
		return nil, false
	}
	return &StructuralInstance{rule: rule.schema, variant: &activationVariantAttachment{application: application, plan: plan, ports: append([]activationPortBinding(nil), ports...)}, declare: declare, state: &structuralInstanceState{}}, true
}

func StructuralRead[S any, K ~uint32 | ~uint64](binding *StructuralBinding, read Read[S], ref Ref[K]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validStructuralBinding(binding) || read.rule != binding.member.rule || read.index != binding.member.readAt || read.resolve == nil {
		failStructuralBinding(binding)
		return false
	}
	return instanceExactRead(binding.assembly, binding.member, ref)
}

func StructuralSelectorRead[RV, S any, Tag selectionTag](binding *StructuralBinding, read Read[Selection[Tag, S]], form ReadForm[RV, S]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validStructuralBinding(binding) {
		failStructuralBinding(binding)
		return false
	}
	return instanceStagedRead(binding.assembly, binding.member, read, form)
}

func StructuralSummaryRead[S, V any, K ~uint32 | ~uint64](binding *StructuralBinding, read Read[S], form ReadForm[V, S], refs *ClosedRefs[K]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validStructuralBinding(binding) || read.rule != binding.member.rule || read.index != binding.member.readAt || read.resolve == nil {
		failStructuralBinding(binding)
		return false
	}
	summary := admitSummary(binding.assembly, form, refs)
	return summary != nil && instanceSummaryRead(binding.assembly, binding.member, summary)
}

func validStructuralBinding(binding *StructuralBinding) bool {
	return binding != nil && binding.gate != nil && binding.assembly != nil && binding.member != nil &&
		binding.member.assembly == binding.assembly && validAssembly(binding.assembly)
}

func failStructuralBinding(binding *StructuralBinding) {
	if binding != nil {
		failAssembly(binding.assembly)
	}
}

// InstanceRead supplies the next declared exact read together with its typed
// declaration token. A different Rule, position, form, or Ref fails the whole
// enclosing assembly.
func InstanceRead[V, O, S any, K ~uint32 | ~uint64](binding *RuleBinding[V, O], read Read[S], ref Ref[K]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validRuleBinding(binding) || read.rule != binding.member.rule || read.index != binding.member.readAt ||
		read.index < 0 || read.index >= len(binding.member.rule.reads) || read.resolve == nil {
		failRuleBinding(binding)
		return false
	}
	return instanceExactRead(binding.assembly, binding.member, ref)
}

// InstancePortRead binds the next exact Rule read to one target-static import
// slot instead of an application-specific Factor coordinate. It is valid only
// while ActivationPrototype executes the typed declaration. The engine mints
// an out-of-range prototype surface that must be substituted by the matching
// activation port before runtime Factor binding; ordinary Assembly can never
// create or observe that placeholder.
func InstancePortRead[V, O, S any](binding *RuleBinding[V, O], read Read[S], role, slot SemanticKey) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !binding.prototype || !role.Available() || !slot.Available() || !validRuleBinding(binding) || read.rule != binding.member.rule ||
		read.index != binding.member.readAt || read.index < 0 || read.index >= len(binding.member.rule.reads) || read.resolve == nil {
		failRuleBinding(binding)
		return false
	}
	member, assembly := binding.member, binding.assembly
	declared := member.rule.reads[member.readAt]
	if declared.form == nil || declared.form.factor == nil || declared.form.readKind != exactReadForm {
		failRuleBinding(binding)
		return false
	}
	if assembly.prototypeReadLocal == nil {
		assembly.prototypeReadLocal = make(map[*factorSchema]uint64)
	}
	ordinal := assembly.prototypeReadLocal[declared.form.factor] + 1
	local := declared.form.factor.keyEnd + ordinal
	if ordinal == 0 || local <= declared.form.factor.keyEnd {
		failRuleBinding(binding)
		return false
	}
	assembly.prototypeReadLocal[declared.form.factor] = ordinal
	surface := equation.Surface{Factor: declared.form.factor.semantic.compositionKey(), Form: equation.SurfaceReadExact, Local: local}
	if !surface.Available() || !appendPrototypePortRead(assembly, role, slot, surface) {
		failRuleBinding(binding)
		return false
	}
	member.reads = append(member.reads, equation.ResolvedRead{Index: uint64(member.readAt), Surface: surface})
	member.readAt++
	return true
}

func appendPrototypePortRead(assembly *Assembly, role, slot SemanticKey, surface equation.Surface) bool {
	if assembly == nil || !role.Available() || !slot.Available() || !surface.Available() {
		return false
	}
	roleKey, slotKey := role.compositionKey(), slot.compositionKey()
	for index := range assembly.prototypePorts {
		port := &assembly.prototypePorts[index]
		if port.Role != roleKey {
			continue
		}
		for _, present := range port.Reads {
			if present.Role == slotKey || present.Surface == surface {
				return false
			}
		}
		port.Reads = append(port.Reads, equation.PortRead{Role: slotKey, Surface: surface})
		return true
	}
	assembly.prototypePorts = append(assembly.prototypePorts, equation.Port{Role: roleKey, Mode: equation.PortImport, Reads: []equation.PortRead{{Role: slotKey, Surface: surface}}})
	return true
}

// InstanceSelectorRead supplies one staged exact-read node. Its target Factor
// is sealed here, while exact Ref routes are emitted only from the completed
// Product row at runtime.
func InstanceSelectorRead[V, O, RV, S any, Tag selectionTag](binding *RuleBinding[V, O], read Read[Selection[Tag, S]], form ReadForm[RV, S]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validRuleBinding(binding) {
		failRuleBinding(binding)
		return false
	}
	return instanceStagedRead(binding.assembly, binding.member, read, form)
}

// InstanceSummaryRead supplies the next declared summary read from its
// already-sealed typed Ref vector. Reusing the exact form and ClosedRefs
// pointer within one Assembly reuses its private topology surface; fresh
// vectors remain distinct even when their contents agree.
func InstanceSummaryRead[V, O, S, RV any, K ~uint32 | ~uint64](binding *RuleBinding[V, O], read Read[S], form ReadForm[RV, S], refs *ClosedRefs[K]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validRuleBinding(binding) || read.rule != binding.member.rule || read.index != binding.member.readAt ||
		read.index < 0 || read.index >= len(binding.member.rule.reads) || read.resolve == nil {
		failRuleBinding(binding)
		return false
	}
	summary := admitSummary(binding.assembly, form, refs)
	return summary != nil && instanceSummaryRead(binding.assembly, binding.member, summary)
}

// InstanceWrite supplies the next declared exact write together with its
// typed declaration token.
func InstanceWrite[V, O any, K ~uint32 | ~uint64](binding *RuleBinding[V, O], write Write[V], ref Ref[K]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validRuleBinding(binding) || write.rule != binding.member.rule || write.index != binding.member.writeAt ||
		write.index < 0 || write.index >= len(binding.member.rule.writes) {
		failRuleBinding(binding)
		return false
	}
	return instanceExactWrite(binding.assembly, binding.member, ref)
}

// InstanceSelectorWrite supplies the next declared selector write and its
// complete positional target relation.
func InstanceSelectorWrite[V, O any](binding *RuleBinding[V, O], write Write[V], form WriteForm[V], targets []SelectorTarget, relations []CandidateRelation[V]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validRuleBinding(binding) || write.rule != binding.member.rule || write.index != binding.member.writeAt ||
		write.index < 0 || write.index >= len(binding.member.rule.writes) {
		failRuleBinding(binding)
		return false
	}
	return instanceSelectorWrite(binding.assembly, binding.member, form, targets, relations)
}

// InstanceRouteWrite binds a route write to the exact preceding staged read
// named by its cold declaration. It accepts no target capability: the only
// legal target is the presealed exact Ref carried by each Selection route at
// runtime.
func InstanceRouteWrite[V, O any, Tag selectionTag, S any](binding *RuleBinding[V, O], write Write[V], selection Read[Selection[Tag, S]]) bool {
	if binding == nil || !binding.gate.begin() {
		return false
	}
	defer binding.gate.end()
	if !validRuleBinding(binding) || write.rule != binding.member.rule || write.index != binding.member.writeAt || write.index < 0 || write.index >= len(binding.member.rule.writes) ||
		selection.rule != binding.member.rule || selection.index < 0 || selection.index >= binding.member.readAt || selection.index >= len(binding.member.rule.reads) || selection.resolve == nil {
		failRuleBinding(binding)
		return false
	}
	return instanceRouteWrite(binding.assembly, binding.member, selection.index)
}

func validRuleBinding[V, O any](binding *RuleBinding[V, O]) bool {
	return binding != nil && binding.gate != nil && binding.assembly != nil && binding.member != nil && binding.rule != nil &&
		binding.member.assembly == binding.assembly && binding.member.rule == binding.rule.schema && validAssembly(binding.assembly)
}

func failRuleBinding[V, O any](binding *RuleBinding[V, O]) {
	if binding != nil {
		failAssembly(binding.assembly)
	}
}
