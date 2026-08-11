package engine

import (
	"sort"
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// ActivationPlan is the source-neutral, immutable catalog of static
// target/endpoint variants for one declared activation family.  It contains
// no Application dimension: a trigger supplies that one exact identity only
// when it creates a StructuralInstance.
//
// Equation's VariantPlan and PrototypeRow remain private implementation
// details.  The source assembler can seal, populate, enumerate, and attach
// this capability, but cannot manufacture dynamic topology rows.
type ActivationPlan struct{ value *activationVariantPlan }

// SourceAssembly owns the one source Batch from its open construction through
// its final seal. Static activation plans and the production body facade both
// use this exact owner; neither path can privately create, seal, or replay a
// second source topology.
type SourceAssembly struct {
	// state is shared only by the original pointer.  A copied SourceAssembly is
	// deliberately not another authority: state.owner remains the pointer
	// issued by NewSourceAssembly and every public operation checks that fence.
	// Keeping the state behind a pointer also makes the one-shot seal and
	// assembly claims atomic when a caller copies a capability value.
	state *sourceAssemblyState
}

type sourceAssemblyState struct {
	mu          sync.Mutex
	owner       *SourceAssembly
	batch       *equation.Batch
	admission   sourceAdmissionCore
	composition *Composition
	pending     []*sourceBoundaryDescriptor
	// Lifecycle claims are atomic so read-only opaque capability checks can
	// observe the phase without taking this mutex while a mutator holds it.
	sealed    atomic.Bool
	assembled atomic.Bool
	// failed is the terminal poison claim for a rejected batch or boundary
	// publication transaction. It fences every capability after Seal failure.
	failed atomic.Bool
}

// SourceSite, SourceOccurrence, and SourceOperand are source-assembly scoped
// capabilities.  Their equation rows never cross the generic engine boundary.
type SourceSite struct {
	owner *SourceAssembly
	value equation.Site
}

type SourceOccurrence struct {
	owner *SourceAssembly
	value equation.Occurrence
}

type SourceOperand struct {
	owner *SourceAssembly
	value equation.Operand
}

// SourceInstance is the one opaque admission capability carried from source
// construction into assembly. It retains either a typed RuleInstance, an
// output-free StructuralInstance, or one exact late-bound ActivationRule
// source admission behind engine-private fields, so production compilers
// never need reflection, raw equation coordinates, or access to a domain's
// private operand type.
type SourceInstance struct {
	owner          *SourceAssembly
	occurrence     SourceOccurrence
	operand        SourceOperand
	instance       sourceMemberInstance
	activationUsed *atomic.Bool
	activationRule *ActivationRule
}

// PreparedActivationPlan is an opaque, atomically admitted static plan.  It
// contains no raw equation template or variant: finalization is the only
// route from an admitted typed prototype to an ActivationPlan.
type PreparedActivationPlan struct {
	assembly    *SourceAssembly
	composition *Composition
	family      ActivationFamily
	entries     []preparedActivationPlanEntry
	finalized   bool
}

type preparedActivationPlanEntry struct {
	entry      ActivationPlanEntry
	occurrence equation.Occurrence
	operand    equation.Operand
}

// ActivationPlanEntry is the complete owner-authored route for one static
// endpoint.  Target and Endpoint select the target-owned outcome; PortRole
// and Provenance define its one import ABI; Prototype is the exact typed rule
// instance admitted in the shared source assembly.
type ActivationPlanEntry struct {
	Target     SemanticKey
	Endpoint   SemanticKey
	PortRole   SemanticKey
	Provenance SemanticKey
	Prototype  ActivationPrototypeAdmission
	// FactorEdges are the target-owned structural transports added to this
	// variant. Endpoints name already-bound ActivationPort roles; the sealed
	// expansion turns each row into the ordinary equation FactorEdge path.
	// They carry no callback, Member, or alternate fact authority.
	FactorEdges []ActivationFactorEdge
}

// ActivationFactorEdge is one source-neutral, target-owned Factor transport
// declaration. SourceRole and TargetRole are named ActivationPort roles that
// the trigger binds to existing Assembly points. The selected Member premise
// is conjoined during graph expansion, so an unselected variant contributes
// no edge transport. Factor is an owner-issued CarryForm; Provenance is the
// source-neutral semantic relation key.
type ActivationFactorEdge struct {
	SourceRole SemanticKey
	TargetRole SemanticKey
	// SourceSite selects an already-issued SourceAssembly point for a
	// target-specific summary. It is mutually exclusive with SourceRole;
	// unlike a port, it is not alpha-renamed per Member. The owning Assembly
	// must include this site as a Point before activation expansion.
	SourceSite SourceSite
	// TargetSite is the symmetric already-issued target Point form. It is
	// mutually exclusive with TargetRole and lets a selected fragment land on
	// a static body point without manufacturing an export port.
	TargetSite SourceSite
	Factor     CarryForm
	Provenance SemanticKey
}

// ActivationPrototypeAdmission describes one exact static prototype source
// row. Its typed instance remains opaque after preparation, so one complete
// plan can contain heterogeneous rule and operand types without reopening a
// sequential attachment path.
type ActivationPrototypeAdmission struct {
	source    SemanticKey
	candidate activationPrototypeCandidate
}

type activationPrototypeCandidate interface {
	admissionState() *ruleInstanceState
	admissionComposition() *Composition
	admissionEntity() (composition.Key, bool)
	admissionSchema() composition.Key
	commitAdmission(*equation.Batch, equation.Occurrence, equation.Operand)
	completeTemplate(*equation.Batch, equation.Occurrence, equation.Operand, SemanticKey, SemanticKey, []ActivationFactorEdge) (equation.Template, bool)
	attachmentPayload(*ActivationPlan, SemanticKey, SemanticKey) (activationPrototypePayload, bool)
}

type typedActivationPrototypeCandidate[V, O any] struct{ instance *RuleInstance[V, O] }

// ActivationPrototypeAdmissionFor retains an exact typed activation instance
// behind the source-neutral complete-set boundary.
func ActivationPrototypeAdmissionFor[V, O any](source SemanticKey, instance *RuleInstance[V, O]) (ActivationPrototypeAdmission, bool) {
	candidate := typedActivationPrototypeCandidate[V, O]{instance: instance}
	if !source.Available() || candidate.admissionState() == nil {
		return ActivationPrototypeAdmission{}, false
	}
	if _, ok := candidate.admissionEntity(); !ok {
		return ActivationPrototypeAdmission{}, false
	}
	return ActivationPrototypeAdmission{source: source, candidate: candidate}, true
}

func (candidate typedActivationPrototypeCandidate[V, O]) admissionState() *ruleInstanceState {
	if candidate.instance == nil {
		return nil
	}
	return candidate.instance.state
}

func (candidate typedActivationPrototypeCandidate[V, O]) admissionComposition() *Composition {
	if candidate.instance == nil || candidate.instance.rule == nil {
		return nil
	}
	return candidate.instance.rule.composition
}

func (candidate typedActivationPrototypeCandidate[V, O]) admissionEntity() (composition.Key, bool) {
	if candidate.instance == nil || candidate.instance.state == nil || candidate.instance.rule == nil || candidate.instance.rule.schema == nil || candidate.instance.kind != ruleInstanceActivation ||
		candidate.instance.declare == nil || candidate.instance.rule.schema.outputKind != ruleFactorOutput || candidate.instance.rule.schema.output == nil || candidate.instance.rule.schema.support != nil || candidate.instance.rule.schema.activation != nil {
		return composition.Key{}, false
	}
	_, digest, valid := canonicalInstanceOperand(candidate.instance)
	entity, entityOK := operandEntityForContent(digest)
	if !valid || !entityOK {
		return composition.Key{}, false
	}
	return entity, true
}

func (candidate typedActivationPrototypeCandidate[V, O]) admissionSchema() composition.Key {
	if candidate.instance == nil || candidate.instance.rule == nil || candidate.instance.rule.schema == nil {
		return composition.Key{}
	}
	return candidate.instance.rule.schema.semantic.compositionKey()
}

func (candidate typedActivationPrototypeCandidate[V, O]) commitAdmission(batch *equation.Batch, occurrence equation.Occurrence, operand equation.Operand) {
	candidate.instance.state.admitClosed = true
	candidate.instance.state.admit = &ruleInstanceAdmission{batch: batch, occurrence: occurrence, operand: operand}
}

func (candidate typedActivationPrototypeCandidate[V, O]) completeTemplate(batch *equation.Batch, occurrence equation.Occurrence, operand equation.Operand, role, provenance SemanticKey, edges []ActivationFactorEdge) (equation.Template, bool) {
	template, ok := activationPrototype(batch, occurrence, operand, candidate.instance)
	if !ok {
		return equation.Template{}, false
	}
	return completeActivationTemplate(template, occurrence.Site(), role, provenance, edges, candidate.instance.rule.composition)
}

func (candidate typedActivationPrototypeCandidate[V, O]) attachmentPayload(plan *ActivationPlan, target, endpoint SemanticKey) (activationPrototypePayload, bool) {
	return activationPrototypeAttachmentPayload(plan, target, endpoint, candidate.instance)
}

// ActivationPort accumulates the typed exact reads for one named source port.
// Its mutable construction phase ends permanently when a trigger snapshots
// it.  There is deliberately no generic/raw surface constructor.
type ActivationPort struct {
	mu     sync.Mutex
	role   SemanticKey
	base   ActivationBase
	reads  []activationPortRead
	slots  []SemanticKey
	closed bool
}

// activationPrototype executes one admitted activation instance's typed
// declaration against a disposable binding and returns equation's canonical
// data row. It is the only source-compiler path from owner-private Refs to a
// template row: callers cannot supply Schema, OperandFamily, or raw surfaces.
//
// Exact reads may be supplied by named import ports. Staged reads remain local
// to the immutable prototype: the typed Rule declaration supplies their one
// selector implementation and the template retains only its authenticated
// static surface. Summary, selector-write, route, and weak surfaces fail
// closed until their complete template-owned catalog can be authenticated in
// the same transaction.
func activationPrototype[V, O any](batch *equation.Batch, occurrence equation.Occurrence, operand equation.Operand, instance *RuleInstance[V, O]) (equation.Template, bool) {
	if batch == nil || !batch.Sealed() || instance == nil || instance.state == nil || instance.rule == nil || instance.rule.schema == nil ||
		instance.kind != ruleInstanceActivation || instance.declare == nil || instance.rule.schema.outputKind != ruleFactorOutput || instance.rule.schema.output == nil ||
		instance.rule.schema.support != nil || instance.rule.schema.activation != nil || !matchesInstanceAdmission(instance, batch, occurrence, operand) {
		return equation.Template{}, false
	}
	state := instance.state
	state.admitMu.Lock()
	if state.activationPrototypeClosed || state.activationPlan != nil || state.activationRow.Available() {
		state.admitMu.Unlock()
		return equation.Template{}, false
	}
	state.activationPrototypeClosed = true
	state.admitMu.Unlock()

	scratch := newAssembly(instance.rule.composition, batch)
	if scratch == nil {
		return equation.Template{}, false
	}
	point := &assemblyPoint{assembly: scratch, site: occurrence.Site()}
	scratch.points = append(scratch.points, point)
	member := &assemblyMember{assembly: scratch, point: point, rule: instance.rule.schema, occurrence: occurrence, operand: operand}
	binding := &RuleBinding[V, O]{assembly: scratch, member: member, rule: instance.rule, gate: newColdGate(), prototype: true}
	declared, closed := false, false
	func() {
		defer func() {
			closed = binding.gate.close()
			if recover() != nil {
				failAssembly(scratch)
				declared = false
			}
		}()
		declared = instance.declare(binding)
	}()
	row, lowered := lowerAssemblyRule(member)
	if !declared || !closed || !lowered || scratch.failed.Load() || scratch.selectorWriteLocal != 0 || scratch.routeWriteLocal != 0 ||
		scratch.summaryLocal != 0 || scratch.weakLocal != 0 || len(scratch.summaries) != 0 || len(scratch.weakTargets) != 0 ||
		!activationPrototypeRow(row) || activationPrototypeSelectorCount(row) != scratch.selectorLocal {
		return equation.Template{}, false
	}
	frozen := cloneActivationPrototypeRow(row)
	ports := cloneActivationPrototypePorts(scratch.prototypePorts)
	state.admitMu.Lock()
	defer state.admitMu.Unlock()
	if state.activationPrototype != nil || state.activationPlan != nil || state.activationRow.Available() {
		return equation.Template{}, false
	}
	state.activationPrototype = &frozen
	state.activationPrototypePorts = cloneActivationPrototypePorts(ports)
	return equation.Template{Rules: []equation.RuleInstance{cloneActivationPrototypeRow(frozen)}, Ports: ports}, true
}

func activationPrototypeRow(row equation.RuleInstance) bool {
	if !row.Schema.Available() || !row.OperandFamily.Available() || !row.Occurrence.Available() || !row.Operand.Available() || len(row.Supports) != 0 || len(row.Prunes) != 0 {
		return false
	}
	selectorLocal := uint64(0)
	for index, read := range row.Reads {
		if read.Index != uint64(index) || read.Surface.Mode != equation.TargetModeNone {
			return false
		}
		switch read.Surface.Form {
		case equation.SurfaceReadExact:
			if read.Surface.Semantic.Available() || read.Surface.Normalizer.Available() {
				return false
			}
		case equation.SurfaceReadSelect:
			selectorLocal++
			if read.Surface.Local != selectorLocal || !read.Surface.Semantic.Available() || read.Surface.Semantic != read.Surface.Factor || read.Surface.Normalizer.Available() {
				return false
			}
		default:
			return false
		}
	}
	for index, write := range row.Writes {
		if write.Index != uint64(index) || write.Surface.Form != equation.SurfaceWriteExact || write.Surface.Mode != equation.TargetModeStrong || write.Surface.Semantic.Available() || write.Surface.Normalizer.Available() ||
			write.Route != 0 || len(write.Candidates) != 0 || len(write.TargetCandidates) != 0 || len(write.Relations) != 0 {
			return false
		}
	}
	return true
}

func activationPrototypeSelectorCount(row equation.RuleInstance) uint64 {
	var result uint64
	for _, read := range row.Reads {
		if read.Surface.Form == equation.SurfaceReadSelect {
			result++
		}
	}
	return result
}

func cloneActivationPrototypeRow(row equation.RuleInstance) equation.RuleInstance {
	return equation.RuleInstance{
		Schema: row.Schema, OperandFamily: row.OperandFamily, Occurrence: row.Occurrence, Operand: row.Operand,
		Reads: append([]equation.ResolvedRead(nil), row.Reads...), Carries: append([]equation.ResolvedCarry(nil), row.Carries...),
		Writes: cloneResolvedWrites(row.Writes), Supports: append([]equation.ResolvedSupport(nil), row.Supports...), Prunes: append([]equation.ResolvedPrune(nil), row.Prunes...),
	}
}

func cloneActivationPrototypePorts(values []equation.Port) []equation.Port {
	result := make([]equation.Port, len(values))
	for index, value := range values {
		result[index] = equation.Port{Role: value.Role, Mode: value.Mode, Reads: append([]equation.PortRead(nil), value.Reads...)}
		sort.Slice(result[index].Reads, func(left, right int) bool {
			return lessCompositionKey(result[index].Reads[left].Role, result[index].Reads[right].Role)
		})
	}
	sort.Slice(result, func(left, right int) bool { return lessCompositionKey(result[left].Role, result[right].Role) })
	return result
}

// completeActivationTemplate supplies the one ordinary fragment shape for a
// single exact prototype row.  Its port imports the source payload into the
// selected endpoint; target and endpoint remain the surrounding Variant key.
func completeActivationTemplate(template equation.Template, site equation.Site, role, provenance SemanticKey, edges []ActivationFactorEdge, source *Composition) (equation.Template, bool) {
	if !site.Available() || !role.Available() || !provenance.Available() || len(template.Rules) != 1 || len(template.Ports) != 1 || template.Ports[0].Role != role.compositionKey() {
		return equation.Template{}, false
	}
	// The prototype occurrence already retains its exact static Site.  The
	// shared fragment itself is intentionally port-to-port: introducing a
	// local output here would leave an unowned endpoint between the caller's
	// dynamic base and the selected static variant.
	template.Points = nil
	template.Ports[0].Mode = equation.PortImportExport
	template.Groups = []equation.FragmentGroup{{
		Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.FragmentPoint{Port: role.compositionKey()},
		Inputs: []equation.FragmentInput{{Point: equation.FragmentPoint{Port: role.compositionKey()}, Provenance: provenance.compositionKey(), Pre: equation.TrueExpr(), Reindex: equation.IdentityReindex(equation.EmptyScope()), Post: equation.TrueExpr()}},
	}}
	template.FactorEdges = nil
	seen := make(map[[6]composition.Key]struct{}, len(edges))
	for _, edge := range edges {
		sourceExternal := edge.SourceSite.Available()
		sourceRole := edge.SourceRole.Available()
		targetExternal := edge.TargetSite.Available()
		targetRole := edge.TargetRole.Available()
		if sourceExternal == sourceRole || targetExternal == targetRole || !validActivationCarry(edge.Factor, source) || !edge.Provenance.Available() {
			return equation.Template{}, false
		}
		if sourceExternal && edge.SourceSite.owner == nil || targetExternal && edge.TargetSite.owner == nil {
			return equation.Template{}, false
		}
		identity := activationFactorEdgeIdentity(edge)
		if _, duplicate := seen[identity]; duplicate {
			return equation.Template{}, false
		}
		seen[identity] = struct{}{}
		if !sourceExternal && !ensureActivationTemplatePort(&template, edge.SourceRole, equation.PortImport) {
			return equation.Template{}, false
		}
		if !targetExternal && !ensureActivationTemplatePort(&template, edge.TargetRole, equation.PortExport) {
			return equation.Template{}, false
		}
		sourceScope := equation.EmptyScope()
		if sourceExternal {
			sourceScope = edge.SourceSite.value.Scope()
		}
		targetScope := equation.EmptyScope()
		if targetExternal {
			targetScope = edge.TargetSite.value.Scope()
		}
		reindex, reindexOK := activationFactorReindex(sourceScope, targetScope)
		if !reindexOK {
			return equation.Template{}, false
		}
		fragment := equation.FragmentFactorEdge{
			Factor: edge.Factor.factor.semantic.compositionKey(), Provenance: edge.Provenance.compositionKey(),
			Pre: equation.TrueExpr(), Reindex: reindex, Post: equation.TrueExpr(),
		}
		if sourceExternal {
			fragment.ExternalSource = edge.SourceSite.value
		} else {
			fragment.Source = equation.FragmentPoint{Port: edge.SourceRole.compositionKey()}
		}
		if targetExternal {
			fragment.ExternalTarget = edge.TargetSite.value
		} else {
			fragment.Target = equation.FragmentPoint{Port: edge.TargetRole.compositionKey()}
		}
		template.FactorEdges = append(template.FactorEdges, fragment)
	}
	return template, true
}

// completeStructuralActivationTemplate builds the same ordinary fragment
// boundary as the typed path, but intentionally has no Rule/Group payload.
// The resulting FactorEdges are sealed by equation and expanded through the
// ordinary TopologySpec edge list only after a selected Member exists.
func completeStructuralActivationTemplate(entry ActivationPlanEntry, source *Composition) (equation.Template, bool) {
	if len(entry.FactorEdges) == 0 {
		return equation.Template{}, false
	}
	template := equation.Template{}
	seen := make(map[[6]composition.Key]struct{}, len(entry.FactorEdges))
	for _, edge := range entry.FactorEdges {
		sourceExternal := edge.SourceSite.Available()
		sourceRole := edge.SourceRole.Available()
		targetExternal := edge.TargetSite.Available()
		targetRole := edge.TargetRole.Available()
		if sourceExternal == sourceRole || targetExternal == targetRole || !validActivationCarry(edge.Factor, source) || !edge.Provenance.Available() {
			return equation.Template{}, false
		}
		if sourceExternal && edge.SourceSite.owner == nil || targetExternal && edge.TargetSite.owner == nil {
			return equation.Template{}, false
		}
		identity := activationFactorEdgeIdentity(edge)
		if _, duplicate := seen[identity]; duplicate {
			return equation.Template{}, false
		}
		seen[identity] = struct{}{}
		if sourceRole && !ensureActivationTemplatePort(&template, edge.SourceRole, equation.PortImport) {
			return equation.Template{}, false
		}
		if targetRole && !ensureActivationTemplatePort(&template, edge.TargetRole, equation.PortExport) {
			return equation.Template{}, false
		}
		sourceScope := equation.EmptyScope()
		if sourceExternal {
			sourceScope = edge.SourceSite.value.Scope()
		}
		targetScope := equation.EmptyScope()
		if targetExternal {
			targetScope = edge.TargetSite.value.Scope()
		}
		reindex, ok := activationFactorReindex(sourceScope, targetScope)
		if !ok {
			return equation.Template{}, false
		}
		fragment := equation.FragmentFactorEdge{
			Factor: edge.Factor.factor.semantic.compositionKey(), Provenance: edge.Provenance.compositionKey(),
			Pre: equation.TrueExpr(), Reindex: reindex, Post: equation.TrueExpr(),
		}
		if sourceExternal {
			fragment.ExternalSource = edge.SourceSite.value
		} else {
			fragment.Source = equation.FragmentPoint{Port: edge.SourceRole.compositionKey()}
		}
		if targetExternal {
			fragment.ExternalTarget = edge.TargetSite.value
		} else {
			fragment.Target = equation.FragmentPoint{Port: edge.TargetRole.compositionKey()}
		}
		template.FactorEdges = append(template.FactorEdges, fragment)
	}
	return template, true
}

func activationFactorEdgeIdentity(edge ActivationFactorEdge) [6]composition.Key {
	var source, target composition.Key
	if edge.SourceRole.Available() {
		source = edge.SourceRole.compositionKey()
	} else if edge.SourceSite.value.Available() {
		source = edge.SourceSite.value.Key()
	}
	if edge.TargetRole.Available() {
		target = edge.TargetRole.compositionKey()
	} else if edge.TargetSite.value.Available() {
		target = edge.TargetSite.value.Key()
	}
	var factor composition.Key
	if edge.Factor.factor != nil {
		factor = edge.Factor.factor.semantic.compositionKey()
	}
	return [6]composition.Key{source, target, factor, edge.Provenance.compositionKey(), edge.SourceSite.value.Key(), edge.TargetSite.value.Key()}
}

func validActivationCarry(carry CarryForm, source *Composition) bool {
	if carry.composition == nil || carry.factor == nil || source != nil && carry.composition != source {
		return false
	}
	if source != nil {
		return knownAssemblyFactor(source, carry.factor)
	}
	return carry.factor.composition == carry.composition && carry.factor.semantic.Available()
}

// activationFactorReindex closes the formal transport relation for one
// FactorEdge-only row. Source decisions that are also present at the target
// retain identity; all other source decisions are explicitly forgotten.
// Target-only decisions are introduced by the ordinary reindex boundary.
func activationFactorReindex(source, target equation.Scope) (equation.Reindex, bool) {
	if !source.Available() || !target.Available() {
		return equation.Reindex{}, false
	}
	maps := make([]equation.DecisionMap, 0, source.Count())
	for index := 0; index < source.Count(); index++ {
		decision, ok := source.At(index)
		if !ok {
			return equation.Reindex{}, false
		}
		if scopeContains(target, decision) {
			maps = append(maps, equation.Identity(decision))
		} else {
			maps = append(maps, equation.Forget(decision))
		}
	}
	return equation.NewReindex(source, target, maps)
}

func scopeContains(scope equation.Scope, decision equation.Decision) bool {
	for index := 0; index < scope.Count(); index++ {
		candidate, ok := scope.At(index)
		if ok && candidate.Key() == decision.Key() {
			return true
		}
	}
	return false
}

func ensureActivationTemplatePort(template *equation.Template, role SemanticKey, needed equation.PortMode) bool {
	if template == nil || !role.Available() {
		return false
	}
	key := role.compositionKey()
	for index := range template.Ports {
		if template.Ports[index].Role != key {
			continue
		}
		mode := template.Ports[index].Mode
		switch {
		case mode == equation.PortImport && needed == equation.PortExport,
			mode == equation.PortExport && needed == equation.PortImport:
			template.Ports[index].Mode = equation.PortImportExport
		case mode != needed && mode != equation.PortImportExport:
			return false
		}
		return true
	}
	template.Ports = append(template.Ports, equation.Port{Role: key, Mode: needed})
	return true
}

// NewSourceAssembly creates the one Batch owned by a source construction and
// binds it to one exact sealed Composition. Every later stage must use this
// same composition; there is no detached source Batch that can be rebound.
func NewSourceAssembly(composition *Composition) *SourceAssembly {
	if composition == nil || !composition.Sealed() {
		return nil
	}
	assembly := &SourceAssembly{}
	batch := equation.NewBatch()
	state := &sourceAssemblyState{owner: assembly, batch: batch, admission: newSourceAdmissionCore(batch), composition: composition}
	assembly.state = state
	return assembly
}

func (assembly *SourceAssembly) assemblyState() *sourceAssemblyState {
	if assembly == nil || assembly.state == nil || assembly.state.owner != assembly || assembly.state.batch == nil || assembly.state.failed.Load() {
		return nil
	}
	return assembly.state
}

// poisonPending is called under state.mu when the one-shot Seal transaction
// cannot publish a complete boundary set. It removes every admission-only
// payload before fencing the state, so retained witnesses cannot preserve a
// second route after poison.
func (state *sourceAssemblyState) poisonPending() {
	for _, descriptor := range state.pending {
		if descriptor == nil {
			continue
		}
		descriptor.source = equation.Site{}
		descriptor.target = equation.Site{}
		descriptor.provenance = composition.Key{}
		descriptor.pre = equation.Expr{}
		descriptor.reindex = equation.Reindex{}
		descriptor.post = equation.Expr{}
		descriptor.input = equation.Input{}
	}
	state.pending = nil
	state.failed.Store(true)
}

// At admits the base occurrence for a source site in this assembly.
func (assembly *SourceAssembly) At(site SourceSite) (SourceOccurrence, bool) {
	state := assembly.assemblyState()
	if state == nil || site.owner != assembly {
		return SourceOccurrence{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.batch.Sealed() {
		return SourceOccurrence{}, false
	}
	occurrence, ok := state.admission.admitOccurrence(site.value, equation.OccurrenceAt, site.value.Source())
	return SourceOccurrence{owner: assembly, value: occurrence}, ok
}

// Relation admits one relation occurrence for a source site in this assembly.
func (assembly *SourceAssembly) Relation(site SourceSite, relation SemanticKey) (SourceOccurrence, bool) {
	state := assembly.assemblyState()
	if state == nil || site.owner != assembly || !relation.Available() {
		return SourceOccurrence{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.batch.Sealed() {
		return SourceOccurrence{}, false
	}
	occurrence, ok := state.admission.admitOccurrence(site.value, equation.OccurrenceRelation, relation.compositionKey())
	return SourceOccurrence{owner: assembly, value: occurrence}, ok
}

// Operand admits one exact operand for an occurrence in this assembly.
func (assembly *SourceAssembly) Operand(occurrence SourceOccurrence, entity SemanticKey) (SourceOperand, bool) {
	state := assembly.assemblyState()
	if state == nil || occurrence.owner != assembly || !entity.Available() {
		return SourceOperand{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.batch.Sealed() {
		return SourceOperand{}, false
	}
	operand, ok := state.admission.admitOperand(occurrence.value, entity.compositionKey())
	return SourceOperand{owner: assembly, value: operand}, ok
}

// Seal is the SourceAssembly's sole phase barrier. It first seals the one
// source batch, then materializes every admitted boundary descriptor into the
// same immutable equation input set. Publication is all-or-nothing: a batch
// or boundary validation failure poisons this assembly and exposes no partial
// boundary set.
func (assembly *SourceAssembly) Seal() bool {
	state := assembly.assemblyState()
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.assembled.Load() {
		return false
	}
	if !state.batch.Seal() {
		state.poisonPending()
		return false
	}
	inputs := make([]equation.Input, len(state.pending))
	for index, descriptor := range state.pending {
		if descriptor == nil {
			state.poisonPending()
			return false
		}
		input := state.admission.admitBoundary(descriptor.source, descriptor.target, descriptor.provenance, descriptor.pre, descriptor.reindex, descriptor.post)
		if !input.Available() {
			state.poisonPending()
			return false
		}
		inputs[index] = input
	}
	for index, descriptor := range state.pending {
		descriptor.input = inputs[index]
		// Once the canonical Input is installed, discard the admission-only
		// fields. The issued witness retains exactly one immutable route and no
		// parallel descriptor representation survives publication.
		descriptor.source = equation.Site{}
		descriptor.target = equation.Site{}
		descriptor.provenance = composition.Key{}
		descriptor.pre = equation.Expr{}
		descriptor.reindex = equation.Reindex{}
		descriptor.post = equation.Expr{}
	}
	state.pending = nil
	state.sealed.Store(true)
	return true
}

// PrepareInstance atomically admits one typed RuleInstance operand and retains
// that exact instance behind an opaque source capability for later assembly.
// This is the sole production Rule-instance route.
func (assembly *SourceAssembly) PrepareInstance(occurrence SourceOccurrence, instance sourceRuleInstance) (SourceInstance, bool) {
	if assembly == nil || instance == nil {
		return SourceInstance{}, false
	}
	operand, ok := instance.productionOperand(assembly, occurrence)
	if !ok {
		return SourceInstance{}, false
	}
	return SourceInstance{owner: assembly, occurrence: occurrence, operand: operand, instance: instance}, true
}

// PrepareStructural atomically admits one factor-free Support instance under
// a source-authored topology entity. It reuses the existing Structural member
// path and retains no factor, control graph, or parallel instance registry.
func (assembly *SourceAssembly) PrepareStructural(occurrence SourceOccurrence, entity SemanticKey, instance *StructuralInstance) (SourceInstance, bool) {
	state := assembly.assemblyState()
	if state == nil || occurrence.owner != assembly || !entity.Available() || instance == nil || instance.rule == nil ||
		instance.rule.composition != state.composition || instance.variant != nil || instance.rule.support == nil || instance.rule.activation != nil {
		return SourceInstance{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.batch.Sealed() {
		return SourceInstance{}, false
	}
	operand, ok := admitStructuralOperand(state.admission, occurrence.value, entity.compositionKey(), instance)
	if !ok {
		return SourceInstance{}, false
	}
	prepared := SourceOperand{owner: assembly, value: operand}
	return SourceInstance{owner: assembly, occurrence: occurrence, operand: prepared, instance: instance}, true
}

// PrepareActivation admits the exact source operand for a late-bound
// activation trigger. The trigger instance cannot be created until the
// sealed plan is finalized, so this capability carries only the one
// source-owned Occurrence/Operand pair across that phase boundary. The
// Assembly.ActivationMember facade consumes it exactly once through the same
// ordinary structural member admission used by Support instances.
func (assembly *SourceAssembly) PrepareActivation(occurrence SourceOccurrence, entity SemanticKey, rule *ActivationRule) (SourceInstance, bool) {
	state := assembly.assemblyState()
	if state == nil || occurrence.owner != assembly || !entity.Available() || rule == nil || !rule.available() || rule.composition != state.composition ||
		rule.schema == nil || !validColdActivationRule(state.composition, rule.schema) {
		return SourceInstance{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.batch.Sealed() {
		return SourceInstance{}, false
	}
	operand, ok := state.admission.admitOperand(occurrence.value, entity.compositionKey())
	if !ok {
		return SourceInstance{}, false
	}
	return SourceInstance{
		owner:          assembly,
		occurrence:     occurrence,
		operand:        SourceOperand{owner: assembly, value: operand},
		activationUsed: &atomic.Bool{},
		activationRule: rule,
	}, true
}

func (instance *RuleInstance[V, O]) productionOperand(assembly *SourceAssembly, occurrence SourceOccurrence) (SourceOperand, bool) {
	state := assembly.assemblyState()
	if state == nil || occurrence.owner != assembly || instance == nil || instance.rule == nil || instance.rule.composition != state.composition {
		return SourceOperand{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.batch.Sealed() {
		return SourceOperand{}, false
	}
	operand, ok := admitInstanceOperandCore(state.admission, occurrence.value, instance)
	return SourceOperand{owner: assembly, value: operand}, ok
}

// Assemble consumes the exact sealed SourceAssembly once and lowers through
// the private canonical solver transaction. The sourceAssembly field is
// private Assembly bookkeeping used solely to fence the opaque facade
// capabilities issued by the callback.
func (assembly *SourceAssembly) Assemble(declare func(*Assembly) bool) (solver *Solver, ok bool) {
	state := assembly.assemblyState()
	if state == nil || declare == nil {
		return nil, false
	}
	state.mu.Lock()
	if state.failed.Load() || state.composition == nil || !state.composition.Sealed() || !state.sealed.Load() || !state.batch.Sealed() || state.assembled.Load() {
		state.mu.Unlock()
		return nil, false
	}
	state.assembled.Store(true)
	composition, batch := state.composition, state.batch
	state.mu.Unlock()
	return assemble(composition, batch, func(value *Assembly) bool {
		if value == nil {
			return false
		}
		value.sourceAssembly = assembly
		return declare(value)
	})
}

func validActivationPlanEntry(assembly *SourceAssembly, source *Composition, entry ActivationPlanEntry) bool {
	if assembly == nil || source == nil || !entry.Target.Available() || !entry.Endpoint.Available() {
		return false
	}
	state := assembly.assemblyState()
	if state == nil || state.composition != source || state.batch == nil || state.batch.Sealed() {
		return false
	}
	typed := entry.Prototype.candidate != nil
	if !typed && len(entry.FactorEdges) == 0 {
		return false
	}
	if !typed {
		anchored := false
		for _, edge := range entry.FactorEdges {
			if edge.SourceSite.owner != nil || edge.TargetSite.owner != nil {
				anchored = true
				break
			}
		}
		if !anchored {
			return false
		}
	}
	if typed && (!entry.PortRole.Available() || !entry.Provenance.Available()) {
		return false
	}
	if typed {
		if !entry.Prototype.source.Available() || entry.Prototype.candidate.admissionComposition() != source {
			return false
		}
	}
	for _, edge := range entry.FactorEdges {
		if !validActivationCarry(edge.Factor, source) || !edge.Provenance.Available() {
			return false
		}
		sourceExternal := edge.SourceSite.owner != nil
		sourceRole := edge.SourceRole.Available()
		targetExternal := edge.TargetSite.owner != nil
		targetRole := edge.TargetRole.Available()
		if sourceExternal == sourceRole || targetExternal == targetRole {
			return false
		}
		if sourceExternal {
			if edge.SourceSite.owner != assembly || !state.batch.OwnsOpenSite(edge.SourceSite.value) {
				return false
			}
		}
		if targetExternal {
			if edge.TargetSite.owner != assembly || !state.batch.OwnsOpenSite(edge.TargetSite.value) {
				return false
			}
		}
	}
	return true
}

// StageActivationPlan atomically admits every entry into SourceAssembly's one
// open Batch.  A plan is only prepared here: raw templates, variants, and
// payload attachment remain private until that exact assembly seals.
func StageActivationPlan(assembly *SourceAssembly, source *Composition, family ActivationFamily, entries []ActivationPlanEntry) (*PreparedActivationPlan, bool) {
	if assembly == nil || source == nil || !source.Sealed() || !family.available() || family.schema.composition != source || len(entries) == 0 {
		return nil, false
	}
	state := assembly.assemblyState()
	if state == nil || state.composition != source {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.batch.Sealed() {
		return nil, false
	}
	typedEntries := make([]ActivationPrototypeAdmission, 0, len(entries))
	typedPositions := make([]int, 0, len(entries))
	for index, entry := range entries {
		if !validActivationPlanEntry(assembly, source, entry) {
			state.admission.reject()
			return nil, false
		}
		if entry.Prototype.candidate != nil {
			typedEntries = append(typedEntries, entry.Prototype)
			typedPositions = append(typedPositions, index)
		}
	}
	admitted := make([]admittedActivationPrototype, len(entries))
	if len(typedEntries) != 0 {
		values, ok := admitActivationPrototypeSet(state.admission, typedEntries)
		if !ok || len(values) != len(typedEntries) {
			return nil, false
		}
		for index, position := range typedPositions {
			admitted[position] = values[index]
		}
	}
	prepared := &PreparedActivationPlan{assembly: assembly, composition: source, family: family, entries: make([]preparedActivationPlanEntry, len(entries))}
	for index, entry := range entries {
		owned := entry
		owned.FactorEdges = append([]ActivationFactorEdge(nil), entry.FactorEdges...)
		prepared.entries[index] = preparedActivationPlanEntry{entry: owned, occurrence: admitted[index].occurrence, operand: admitted[index].operand}
	}
	return prepared, true
}

// FinalizeActivationPlan compiles one sealed prepared plan to its immutable
// activation catalog. It accepts only the exact SourceAssembly that staged the
// entries and performs prototype lowering, template completion, variant
// creation, plan sealing, and payload attachment as one private operation.
func FinalizeActivationPlan(assembly *SourceAssembly, prepared *PreparedActivationPlan) (*ActivationPlan, bool) {
	if assembly == nil || prepared == nil || prepared.assembly != assembly || prepared.composition == nil || !prepared.composition.Sealed() || !prepared.family.available() || prepared.family.schema.composition != prepared.composition {
		return nil, false
	}
	state := assembly.assemblyState()
	if state == nil || state.composition != prepared.composition {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.sealed.Load() || state.assembled.Load() || !state.batch.Sealed() || prepared.finalized || len(prepared.entries) == 0 {
		return nil, false
	}
	variants := make([]equation.Variant, len(prepared.entries))
	for index, item := range prepared.entries {
		entry := item.entry
		var template equation.Template
		var ok bool
		if entry.Prototype.candidate != nil {
			template, ok = entry.Prototype.candidate.completeTemplate(state.batch, item.occurrence, item.operand, entry.PortRole, entry.Provenance, entry.FactorEdges)
		} else {
			template, ok = completeStructuralActivationTemplate(entry, prepared.composition)
		}
		if !ok {
			return nil, false
		}
		variants[index] = equation.Variant{Target: entry.Target.compositionKey(), Endpoint: entry.Endpoint.compositionKey(), Template: template}
	}
	value, ok := newActivationVariantPlan(prepared.composition, prepared.family, variants)
	if !ok {
		return nil, false
	}
	value.sourceAssembly = assembly
	plan := &ActivationPlan{value: value}
	payloads := make([]activationPrototypePayload, 0, len(prepared.entries))
	for _, item := range prepared.entries {
		if item.entry.Prototype.candidate == nil {
			continue
		}
		payload, matched := item.entry.Prototype.candidate.attachmentPayload(plan, item.entry.Target, item.entry.Endpoint)
		if !matched {
			return nil, false
		}
		payloads = append(payloads, payload)
	}
	if !plan.value.attachPrototypePayloads(payloads) {
		return nil, false
	}
	prepared.finalized = true
	return plan, true
}

type admittedActivationPrototype struct {
	occurrence equation.Occurrence
	operand    equation.Operand
}

// admitActivationPrototypeSet validates every member before publishing any
// row into the one open Batch.  It is private because a source owner may only
// obtain the issued occurrence/operand through PreparedActivationPlan.
func admitActivationPrototypeSet(admission sourceAdmissionCore, values []ActivationPrototypeAdmission) ([]admittedActivationPrototype, bool) {
	if admission.batch == nil || len(values) == 0 {
		admission.reject()
		return nil, false
	}
	type prepared struct {
		index     int
		candidate activationPrototypeCandidate
		state     *ruleInstanceState
		source    composition.Key
		operand   composition.Key
		schema    composition.Key
		entity    equation.Admission
	}
	preparedValues := make([]prepared, len(values))
	seen := make(map[*ruleInstanceState]struct{}, len(values))
	type sourceOperand struct {
		source  composition.Key
		operand composition.Key
		schema  composition.Key
	}
	seenRows := make(map[sourceOperand]struct{}, len(values))
	for index, value := range values {
		if !value.source.Available() || value.candidate == nil {
			admission.reject()
			return nil, false
		}
		state := value.candidate.admissionState()
		entity, entityOK := value.candidate.admissionEntity()
		if state == nil || !entityOK {
			admission.reject()
			return nil, false
		}
		if _, duplicate := seen[state]; duplicate {
			admission.reject()
			return nil, false
		}
		seen[state] = struct{}{}
		source := value.source.compositionKey()
		schema := value.candidate.admissionSchema()
		if !schema.Available() {
			admission.reject()
			return nil, false
		}
		key := sourceOperand{source: source, operand: entity, schema: schema}
		if _, duplicate := seenRows[key]; duplicate {
			admission.reject()
			return nil, false
		}
		seenRows[key] = struct{}{}
		preparedValues[index] = prepared{index: index, candidate: value.candidate, state: state, source: source, operand: entity, schema: schema, entity: equation.Admission{
			Source: source, Scope: equation.EmptyScope(), Init: equation.FalseExpr(), Disposition: equation.InitAbsent,
			Kind: equation.OccurrenceAt, Entity: source, Operand: entity,
		}}
	}
	sort.Slice(preparedValues, func(left, right int) bool {
		if preparedValues[left].source != preparedValues[right].source {
			return lessCompositionKey(preparedValues[left].source, preparedValues[right].source)
		}
		if preparedValues[left].operand != preparedValues[right].operand {
			return lessCompositionKey(preparedValues[left].operand, preparedValues[right].operand)
		}
		return lessCompositionKey(preparedValues[left].schema, preparedValues[right].schema)
	})
	for _, value := range preparedValues {
		value.state.admitMu.Lock()
	}
	defer func() {
		for index := len(preparedValues) - 1; index >= 0; index-- {
			preparedValues[index].state.admitMu.Unlock()
		}
	}()
	for _, value := range preparedValues {
		state := value.state
		if state.admitBusy || state.admitClosed || state.admitOverlap || state.admit != nil || state.activationPlan != nil || state.activationRow.Available() || state.activationPrototype != nil || state.activationPrototypeClosed {
			admission.reject()
			return nil, false
		}
	}
	rows := make([]equation.Admission, len(preparedValues))
	for index, value := range preparedValues {
		rows[index] = value.entity
	}
	admitted, admittedOK := admission.admitSourceRows(rows)
	if !admittedOK || len(admitted) != len(preparedValues) {
		return nil, false
	}
	result := make([]admittedActivationPrototype, len(preparedValues))
	for index, value := range preparedValues {
		value.candidate.commitAdmission(admission.batch, admitted[index].Occurrence, admitted[index].Operand)
		result[value.index] = admittedActivationPrototype{occurrence: admitted[index].Occurrence, operand: admitted[index].Operand}
	}
	return result, true
}

// EndpointCount is allocation-free and reports the exact static target /
// endpoint catalog sealed in this plan.
func (plan *ActivationPlan) EndpointCount() int {
	if plan == nil || plan.value == nil || plan.value.plan == (equation.VariantPlan{}) {
		return 0
	}
	return len(plan.value.endpoints)
}

// EndpointAt exposes one canonical static target/endpoint pair for the
// source-owned route table.  It never exposes a VariantPlan or permits a
// caller to select a dynamic Member.
func (plan *ActivationPlan) EndpointAt(index int) (target, endpoint SemanticKey, ok bool) {
	if plan == nil || plan.value == nil || index < 0 || index >= len(plan.value.endpoints) {
		return SemanticKey{}, SemanticKey{}, false
	}
	value := plan.value.endpoints[index]
	return value.target, value.endpoint, value.target.Available() && value.endpoint.Available()
}

func activationPrototypeAttachmentPayload[V, O any](plan *ActivationPlan, target, endpoint SemanticKey, instance *RuleInstance[V, O]) (activationPrototypePayload, bool) {
	if plan == nil || plan.value == nil || plan.value.source == nil || !target.Available() || !endpoint.Available() || instance == nil || instance.rule == nil || instance.rule.schema == nil ||
		instance.kind != ruleInstanceActivation || instance.declare == nil || instance.state == nil || instance.rule.composition != plan.value.source || !instance.rule.available() || instance.rule.schema.outputKind != ruleFactorOutput {
		return nil, false
	}
	instance.state.admitMu.Lock()
	if instance.state.activationPrototype == nil {
		instance.state.admitMu.Unlock()
		return nil, false
	}
	prototype := cloneActivationPrototypeRow(*instance.state.activationPrototype)
	prototypePorts := cloneActivationPrototypePorts(instance.state.activationPrototypePorts)
	instance.state.admitMu.Unlock()
	_, digest, contentOK := canonicalInstanceOperand(instance)
	if !contentOK {
		return nil, false
	}
	rows := plan.value.plan.PrototypeRows(target.compositionKey(), endpoint.compositionKey())
	var match equation.PrototypeRow
	for _, row := range rows {
		if row.Schema() != instance.rule.schema.semantic.compositionKey() || row.OperandFamily() != instance.rule.schema.operandFamily.compositionKey() ||
			!(OperandEntity{key: row.Operand().Entity()}).MatchesContentDigest(digest) || !matchesInstancePrototypeRow(instance, row) || !row.MatchesExact(prototype) || !row.MatchesPortReads(prototypePorts) {
			continue
		}
		if match.Available() {
			return nil, false
		}
		match = row
	}
	if !match.Available() {
		return nil, false
	}
	return activationPrototypePayloadFor(instance, match), true
}

// NewActivationPort begins one exact typed import binding for a trigger-owned
// Assembly point. A port is a local assembly builder only; its role is a
// sealed ABI key, not a Factor coordinate. The opaque base must be issued by
// ActivationBaseAt in the same still-open Assemble transaction.
func NewActivationPort(role SemanticKey, base ActivationBase) (*ActivationPort, bool) {
	if !role.Available() || !base.available() {
		return nil, false
	}
	return &ActivationPort{role: role, base: base}, true
}

// AddActivationPortRead records one named exact Ref read.  Slot keys are
// canonicalized at snapshot and cannot be repeated; foreign Factors and raw
// equation surfaces are rejected by the existing typed Ref boundary.
func AddActivationPortRead[K ~uint32 | ~uint64, V any](port *ActivationPort, slot SemanticKey, factor *Factor[K, V], ref Ref[K]) bool {
	if port == nil || !slot.Available() {
		return false
	}
	port.mu.Lock()
	defer port.mu.Unlock()
	if port.closed {
		return false
	}
	for _, known := range port.slots {
		if known == slot {
			return false
		}
	}
	read := activationPortReadOf(port.role, port.base, slot, factor, ref)
	readRole, readBase, readSlot, _, ok := read.activationPortRead()
	if !ok || readRole != port.role || readBase != port.base || readSlot != slot {
		return false
	}
	port.reads = append(port.reads, read)
	port.slots = append(port.slots, slot)
	return true
}

// NewActivationTrigger creates the sole Application-bearing structural
// instance.  It snapshots every supplied port exactly once, checks duplicate
// roles before closing them, and delegates all later member selection to the
// one sealed equation plan.
func NewActivationTrigger(rule *ActivationRule, application SemanticKey, plan *ActivationPlan, ports []*ActivationPort, declare func(*StructuralBinding) bool) (*StructuralInstance, bool) {
	if rule == nil || rule.schema == nil || rule.schema.activation == nil || !rule.available() || plan == nil || plan.value == nil || plan.value.source == nil || plan.value.plan == (equation.VariantPlan{}) ||
		rule.composition != plan.value.source || !application.Available() || declare == nil || plan.value.family != rule.schema.activation.family.semantic {
		return nil, false
	}
	for _, port := range ports {
		if port == nil || plan.value.sourceAssembly != nil && port.base.assembly != nil && port.base.assembly.sourceAssembly != nil && port.base.assembly.sourceAssembly != plan.value.sourceAssembly {
			return nil, false
		}
	}
	ordered := append([]*ActivationPort(nil), ports...)
	sort.Slice(ordered, func(left, right int) bool {
		return compareSemanticKey(ordered[left].role, ordered[right].role) < 0
	})
	for index, port := range ordered {
		if index > 0 && port.role == ordered[index-1].role {
			return nil, false
		}
	}
	// Lock in canonical role order.  That makes concurrent trigger attempts
	// deterministic and lets us validate every port before closing any one.
	for _, port := range ordered {
		port.mu.Lock()
	}
	defer func() {
		for index := len(ordered) - 1; index >= 0; index-- {
			ordered[index].mu.Unlock()
		}
	}()
	bindings := make([]activationPortBinding, len(ordered))
	for index, port := range ordered {
		if port.closed || !port.role.Available() || !port.base.available() {
			return nil, false
		}
		binding, ok := activationPortBindingOf(port.role, port.base, port.reads...)
		if !ok {
			return nil, false
		}
		bindings[index] = binding
	}
	for _, port := range ordered {
		port.closed = true
	}
	return newVariantActivationRuleInstance(rule, application, plan.value, bindings, declare)
}
