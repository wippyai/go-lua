package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// SourceBoundary is one opaque, source-assembly-owned control boundary. It has
// no exported equation representation and is valid only for the exact source
// assembly that issued its Site and control capabilities.
type SourceBoundary struct {
	owner      *SourceAssembly
	descriptor *sourceBoundaryDescriptor
}

// sourceBoundaryDescriptor is retained only by the owning SourceAssembly
// until Seal. The descriptor is then filled with the canonical equation.Input
// produced by equation.BoundaryInput; no second boundary registry or copied
// topology is introduced.
type sourceBoundaryDescriptor struct {
	source     equation.Site
	target     equation.Site
	provenance composition.Key
	pre        equation.Expr
	reindex    equation.Reindex
	post       equation.Expr
	input      equation.Input
}

// Available reports whether this issued boundary survived source sealing.
func (boundary SourceBoundary) Available() bool {
	state := boundary.owner.assemblyState()
	return state != nil && state.sealed.Load() && boundary.descriptor != nil &&
		state.batch.OwnsSite(boundary.descriptor.input.Source()) && state.batch.OwnsSite(boundary.descriptor.input.Target()) &&
		boundary.descriptor.input.Available()
}

// AssemblyPoint is an opaque Point capability issued by the production
// assembly facade. Its underlying engine point cannot be manufactured or
// selected by declaration order.
type AssemblyPoint struct {
	owner    *SourceAssembly
	assembly *Assembly
	value    *assemblyPoint
}

func (point AssemblyPoint) Available() bool {
	state := point.owner.assemblyState()
	return state != nil && state.sealed.Load() && point.assembly != nil && point.assembly.sourceAssembly == point.owner && point.value != nil &&
		state.batch.OwnsSite(point.value.site) && validPoint(point.assembly, point.value)
}

// AssemblyMember is an opaque Rule-instance attachment issued by the
// production assembly facade.
type AssemblyMember struct {
	owner    *SourceAssembly
	assembly *Assembly
	value    *assemblyMember
}

func (member AssemblyMember) Available() bool {
	state := member.owner.assemblyState()
	return state != nil && state.sealed.Load() && member.assembly != nil && member.assembly.sourceAssembly == member.owner && member.value != nil &&
		member.value.point != nil && state.batch.OwnsSite(member.value.point.site) &&
		state.batch.OwnsOccurrence(member.value.occurrence) && state.batch.OwnsOperand(member.value.operand) &&
		validMember(member.assembly, member.value)
}

// AssemblyGroup is an opaque simultaneous group capability issued by the
// production assembly facade.
type AssemblyGroup struct {
	owner    *SourceAssembly
	assembly *Assembly
	value    *assemblyGroup
}

func (group AssemblyGroup) Available() bool {
	state := group.owner.assemblyState()
	if state == nil || !state.sealed.Load() || group.assembly == nil || group.assembly.sourceAssembly != group.owner || group.value == nil || group.value.output == nil ||
		!state.batch.OwnsSite(group.value.output.site) || !validGroup(group.assembly, group.value) {
		return false
	}
	for _, member := range group.value.members {
		if member == nil || member.point == nil || !state.batch.OwnsSite(member.point.site) ||
			!state.batch.OwnsOccurrence(member.occurrence) || !state.batch.OwnsOperand(member.operand) ||
			!validMember(group.assembly, member) {
			return false
		}
	}
	return true
}

// AssemblyQuery is an opaque Query observation attachment issued by the
// production assembly facade.
type AssemblyQuery struct {
	owner    *SourceAssembly
	assembly *Assembly
	value    *assemblyObservation
}

func (query AssemblyQuery) Available() bool {
	state := query.owner.assemblyState()
	return state != nil && state.sealed.Load() && query.assembly != nil && query.assembly.sourceAssembly == query.owner && query.value != nil &&
		query.value.point != nil && state.batch.OwnsSite(query.value.point.site) && (validObservation(query.assembly, query.value) || validQueryReceipt(query.value.receipt))
}

// Available methods on source capabilities deliberately expose no identity
// or internal equation value. They are useful only for local fail-closed
// checks at the next stage.
func (site SourceSite) Available() bool {
	state := site.owner.assemblyState()
	return state != nil && state.sealed.Load() && state.batch.OwnsSite(site.value)
}

func (occurrence SourceOccurrence) Available() bool {
	state := occurrence.owner.assemblyState()
	return state != nil && state.sealed.Load() && state.batch.OwnsOccurrence(occurrence.value)
}

func (operand SourceOperand) Available() bool {
	state := operand.owner.assemblyState()
	return state != nil && state.sealed.Load() && state.batch.OwnsOperand(operand.value)
}

func (instance SourceInstance) Available() bool {
	if instance.owner == nil || instance.occurrence.owner != instance.owner || instance.operand.owner != instance.owner ||
		!instance.occurrence.Available() || !instance.operand.Available() || !instance.operand.value.Occurrence().Same(instance.occurrence.value) {
		return false
	}
	if instance.instance != nil {
		return instance.activationUsed == nil && instance.activationRule == nil
	}
	return instance.activationUsed != nil && instance.activationRule != nil && !instance.activationUsed.Load()
}

// InputCount reports the exact predecessor-port arity declared by the Rule
// behind one instance or late-bound activation admission owned by this
// SourceAssembly. The projection is intentionally owned by the open
// assembly: it is available only after the exact occurrence/operand admission
// and before the source Batch seal. No Rule, Input, equation coordinate, or
// admission capability crosses this boundary.
func (assembly *SourceAssembly) InputCount(instance SourceInstance) (int, bool) {
	state := assembly.assemblyState()
	if state == nil || state.sealed.Load() || state.assembled.Load() || state.batch == nil || state.batch.Sealed() ||
		state.composition == nil || !state.composition.Sealed() || instance.owner != assembly ||
		instance.occurrence.owner != assembly || instance.operand.owner != assembly {
		return 0, false
	}
	if instance.activationRule != nil {
		if instance.instance != nil || instance.activationUsed == nil || instance.activationRule.composition != state.composition ||
			instance.activationRule.schema == nil || !validColdActivationRule(state.composition, instance.activationRule.schema) ||
			instance.activationRule.schema.inputs < 0 || !state.batch.OwnsOpenOperandFor(instance.operand.value, instance.occurrence.value) {
			return 0, false
		}
		return instance.activationRule.schema.inputs, true
	}
	if instance.instance == nil {
		return 0, false
	}
	return instance.instance.sourceInputCount(assembly, instance.occurrence.value, instance.operand.value)
}

// Point issues one opaque point for an issued Site in this production
// assembly. It is a method so external callers never need the raw equation
// Site accepted by the lower-level test/compiler helper.
func (value *Assembly) Point(site SourceSite) (AssemblyPoint, bool) {
	if value == nil || value.sourceAssembly == nil || site.owner != value.sourceAssembly || !site.Available() {
		return AssemblyPoint{}, false
	}
	point := admitPoint(value, site.value)
	if point == nil {
		return AssemblyPoint{}, false
	}
	return AssemblyPoint{owner: value.sourceAssembly, assembly: value, value: point}, true
}

// ActivationBase issues the opaque base capability consumed by
// NewActivationPort. It accepts only an AssemblyPoint returned by this exact
// Assembly transaction; callers never receive the private assemblyPoint or an
// alternate endpoint authority.
func (value *Assembly) ActivationBase(point AssemblyPoint) (ActivationBase, bool) {
	if value == nil || value.sourceAssembly == nil || point.owner != value.sourceAssembly || point.assembly != value || !point.Available() {
		return ActivationBase{}, false
	}
	return ActivationBaseAt(value, point.value)
}

// sourceMemberInstance is private so external callers cannot implement a
// second member authority. Typed RuleInstance and engine StructuralInstance
// are the only admitted implementations.
type sourceMemberInstance interface {
	productionMember(*Assembly, AssemblyPoint, SourceOccurrence, SourceOperand) (AssemblyMember, bool)
	sourceInputCount(*SourceAssembly, equation.Occurrence, equation.Operand) (int, bool)
}

type sourceRuleInstance interface {
	sourceMemberInstance
	productionOperand(*SourceAssembly, SourceOccurrence) (SourceOperand, bool)
}

func (instance *RuleInstance[V, O]) productionMember(value *Assembly, point AssemblyPoint, occurrence SourceOccurrence, operand SourceOperand) (AssemblyMember, bool) {
	if instance == nil || value == nil || value.sourceAssembly == nil || point.owner != value.sourceAssembly || occurrence.owner != value.sourceAssembly || operand.owner != value.sourceAssembly ||
		!point.Available() || !occurrence.Available() || !operand.Available() {
		return AssemblyMember{}, false
	}
	member := admitInstance(value, point.value, occurrence.value, operand.value, instance)
	if member == nil {
		return AssemblyMember{}, false
	}
	return AssemblyMember{owner: value.sourceAssembly, assembly: value, value: member}, true
}

func (instance *RuleInstance[V, O]) sourceInputCount(owner *SourceAssembly, occurrence equation.Occurrence, operand equation.Operand) (int, bool) {
	if instance == nil || owner == nil || instance.state == nil || instance.rule == nil || instance.rule.schema == nil ||
		instance.rule.schema.composition == nil || instance.rule.schema.composition != instance.rule.composition ||
		instance.rule.schema.open || instance.rule.schema.inputs < 0 || !instance.rule.available() {
		return 0, false
	}
	state := owner.assemblyState()
	if state == nil || state.batch == nil || state.sealed.Load() || state.assembled.Load() || state.composition == nil ||
		state.composition != instance.rule.composition || !state.composition.Sealed() || !matchesInstanceAdmission(instance, state.batch, occurrence, operand) {
		return 0, false
	}
	return instance.rule.schema.inputs, true
}

func (instance *StructuralInstance) productionMember(value *Assembly, point AssemblyPoint, occurrence SourceOccurrence, operand SourceOperand) (AssemblyMember, bool) {
	if instance == nil || value == nil || value.sourceAssembly == nil || point.owner != value.sourceAssembly || occurrence.owner != value.sourceAssembly || operand.owner != value.sourceAssembly ||
		!point.Available() || !occurrence.Available() || !operand.Available() ||
		!matchesStructuralInstanceAdmission(instance, value.batch, occurrence.value, operand.value) {
		return AssemblyMember{}, false
	}
	member := admitStructural(value, point.value, occurrence.value, operand.value, instance)
	if member == nil {
		return AssemblyMember{}, false
	}
	return AssemblyMember{owner: value.sourceAssembly, assembly: value, value: member}, true
}

func (instance *StructuralInstance) sourceInputCount(owner *SourceAssembly, occurrence equation.Occurrence, operand equation.Operand) (int, bool) {
	if instance == nil || owner == nil || instance.state == nil || instance.rule == nil || instance.rule.composition == nil ||
		instance.rule.open || instance.rule.inputs < 0 {
		return 0, false
	}
	state := owner.assemblyState()
	if state == nil || state.batch == nil || state.sealed.Load() || state.assembled.Load() || state.composition == nil ||
		state.composition != instance.rule.composition || !state.composition.Sealed() || !matchesStructuralInstanceAdmission(instance, state.batch, occurrence, operand) {
		return 0, false
	}
	return instance.rule.inputs, true
}

// Member consumes one exact prepared source instance. No domain operand type,
// raw equation coordinate, or second admission path crosses this boundary.
func (value *Assembly) Member(point AssemblyPoint, prepared SourceInstance) (AssemblyMember, bool) {
	if value == nil || value.sourceAssembly == nil || point.assembly != value || prepared.owner != value.sourceAssembly ||
		prepared.instance == nil || prepared.activationUsed != nil || !point.Available() || !prepared.Available() {
		return AssemblyMember{}, false
	}
	return prepared.instance.productionMember(value, point, prepared.occurrence, prepared.operand)
}

// ActivationMember consumes one late-bound activation admission after the
// shared plan has been finalized and its trigger has been constructed. The
// source capability is checked against this Assembly's exact source batch and
// composition, then the ordinary structural admission remains the sole path
// that publishes the member and binds the selected variant.
func (value *Assembly) ActivationMember(point AssemblyPoint, prepared SourceInstance, instance *StructuralInstance) (AssemblyMember, bool) {
	if value == nil || value.sourceAssembly == nil || point.assembly != value || point.owner != value.sourceAssembly ||
		prepared.owner != value.sourceAssembly || prepared.instance != nil || prepared.activationUsed == nil || prepared.activationRule == nil || instance == nil ||
		instance.rule == nil || instance.rule.activation == nil || instance.variant == nil ||
		instance.rule.composition != value.composition || instance.rule != prepared.activationRule.schema || !point.Available() || !prepared.Available() {
		return AssemblyMember{}, false
	}
	state := value.sourceAssembly.assemblyState()
	if state == nil || state.composition != value.composition || state.batch != value.batch || !state.sealed.Load() || value.batch == nil || !value.batch.Sealed() ||
		prepared.occurrence.owner != value.sourceAssembly || prepared.operand.owner != value.sourceAssembly ||
		!value.batch.OwnsOccurrence(prepared.occurrence.value) || !value.batch.OwnsOperand(prepared.operand.value) {
		return AssemblyMember{}, false
	}
	if !prepared.activationUsed.CompareAndSwap(false, true) {
		return AssemblyMember{}, false
	}
	member := admitStructural(value, point.value, prepared.occurrence.value, prepared.operand.value, instance)
	if member == nil {
		return AssemblyMember{}, false
	}
	return AssemblyMember{owner: value.sourceAssembly, assembly: value, value: member}, true
}

// Group commits one simultaneous set of opaque members at output.
func (value *Assembly) Group(output AssemblyPoint, members ...AssemblyMember) (AssemblyGroup, bool) {
	if value == nil || value.sourceAssembly == nil || output.assembly != value || !output.Available() || len(members) == 0 {
		return AssemblyGroup{}, false
	}
	raw := make([]*assemblyMember, len(members))
	for index, member := range members {
		if member.owner != value.sourceAssembly || member.assembly != value || !member.Available() {
			return AssemblyGroup{}, false
		}
		raw[index] = member.value
	}
	group := admitGroup(value, output.value, raw...)
	if group == nil {
		return AssemblyGroup{}, false
	}
	return AssemblyGroup{owner: value.sourceAssembly, assembly: value, value: group}, true
}

// Boundary attaches one exact source-issued control boundary to group. No
// transport data can be replaced or supplied at this stage.
func (value *Assembly) Boundary(group AssemblyGroup, boundary SourceBoundary) bool {
	if value == nil || value.sourceAssembly == nil || group.assembly != value || group.owner != value.sourceAssembly || !group.Available() || boundary.owner != value.sourceAssembly || !boundary.Available() {
		if value != nil {
			failAssembly(value)
		}
		return false
	}
	return admitBoundary(value, group.value, boundary.descriptor.input)
}

// EnvironmentInput attaches one exact extra SourceBoundary to a typed Group.
// It is not part of the Rule's declared Input vector: dependency ports remain
// conjunctive and zero-input Rules may still transform this environment.
func (value *Assembly) EnvironmentInput(group AssemblyGroup, boundary SourceBoundary) bool {
	if value == nil || value.sourceAssembly == nil || group.assembly != value || group.owner != value.sourceAssembly || !group.Available() || boundary.owner != value.sourceAssembly || !boundary.Available() {
		if value != nil {
			failAssembly(value)
		}
		return false
	}
	return admitEnvironmentInput(value, group.value, boundary.descriptor.input)
}

// EnvironmentEdge attaches one exact control-only boundary to a Point. It
// creates no Rule or Group and is lowered only as a structural Point edge.
func (value *Assembly) EnvironmentEdge(target AssemblyPoint, boundary SourceBoundary) bool {
	if value == nil || value.sourceAssembly == nil || target.assembly != value || !target.Available() || boundary.owner != value.sourceAssembly || !boundary.Available() {
		if value != nil {
			failAssembly(value)
		}
		return false
	}
	return admitEnvironmentEdge(value, target.value, boundary.descriptor.input)
}

type sourceQueryInstance interface {
	productionQuery(*Assembly, AssemblyPoint) (AssemblyQuery, bool)
}

func (instance *QueryInstance[R]) productionQuery(value *Assembly, point AssemblyPoint) (AssemblyQuery, bool) {
	if instance == nil || value == nil || value.sourceAssembly == nil || point.assembly != value || !point.Available() {
		return AssemblyQuery{}, false
	}
	query := admitQueryAt(value, point.value, instance)
	if query == nil {
		return AssemblyQuery{}, false
	}
	return AssemblyQuery{owner: value.sourceAssembly, assembly: value, value: query}, true
}

// Query attaches one typed QueryInstance to an opaque Point. The private
// interface parameter is implemented only by QueryInstance[R].
func (value *Assembly) Query(point AssemblyPoint, instance sourceQueryInstance) (AssemblyQuery, bool) {
	if value == nil || value.sourceAssembly == nil || point.assembly != value || !point.Available() {
		return AssemblyQuery{}, false
	}
	typed, ok := instance.(sourceQueryInstance)
	if !ok || typed == nil {
		return AssemblyQuery{}, false
	}
	return typed.productionQuery(value, point)
}
