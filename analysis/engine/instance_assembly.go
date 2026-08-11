package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// assembly is one disposable lowering from a sealed source Batch to the sole
// equation topology.  Source identities, scopes, initialization, occurrence
// kind, operands, and boundary transport arrive as issued equation values;
// this package cannot reconstruct any of them from Program keys or booleans.
type Assembly struct {
	composition *Composition
	batch       *equation.Batch
	// sourceAssembly is set only by the production SourceAssembly facade. It fences
	// the opaque public assembly capabilities to the exact source authority
	// that issued their Site/Occurrence/Operand/Boundary rows. Existing
	// equation-backed tests use the lower-level Assembly path and leave it nil.
	sourceAssembly *SourceAssembly
	operands       topologyOperands
	gate           *coldGate
	failed         atomic.Bool
	finished       atomic.Bool

	points             []*assemblyPoint
	pointBySite        map[composition.Key]*assemblyPoint
	groups             []*assemblyGroup
	members            []*assemblyMember
	observations       []*assemblyObservation
	observed           map[*querySchema]struct{}
	selectorLocal      uint64
	selectorWriteLocal uint64
	routeWriteLocal    uint64
	summaryLocal       uint64
	weakLocal          uint64
	prototypeReadLocal map[*factorSchema]uint64
	prototypePorts     []equation.Port
	summaries          []equation.SummaryMapping
	summaryIntern      map[summaryInternKey]*summaryCoverage
	weakTargets        []equation.WeakTargetMapping
	environmentEdges   []*assemblyEnvironmentEdge
	factorEdges        []*assemblyFactorEdge
}

// assemblyPoint, assemblyMember, and assemblyObservation are local builder
// wires. They have no semantic identity and cannot survive assembly sealing.
type assemblyPoint struct {
	assembly *Assembly
	site     equation.Site
}

// ActivationBase is one opaque, Assembly-issued capability for an activation
// port base. It is the exact point already being lowered by Assembly; callers
// cannot manufacture an equation point reference or rely on declaration
// order. The capability is valid only while its Assembly is open.
type ActivationBase struct {
	assembly *Assembly
	point    *assemblyPoint
}

// ActivationBaseAt issues the one activation-port capability for point. A
// caller must obtain point from Point in the current Assemble transaction;
// equation's declaration-order PointRef remains wholly internal to lowering.
func ActivationBaseAt(value *Assembly, point *assemblyPoint) (ActivationBase, bool) {
	if !validPoint(value, point) {
		return ActivationBase{}, false
	}
	return ActivationBase{assembly: value, point: point}, true
}

func (base ActivationBase) available() bool {
	return validPoint(base.assembly, base.point)
}

func (base ActivationBase) belongsTo(value *Assembly) bool {
	return base.assembly == value && validPoint(value, base.point)
}

// assemblyGroup is one exact guarded simultaneous source group. A Site may
// own zero Groups when it is source-only, or several guarded producers. It is
// a disposable lowering wire, never a second semantic identity authority.
type assemblyGroup struct {
	assembly    *Assembly
	output      *assemblyPoint
	members     []*assemblyMember
	inputs      []equation.Input
	environment *equation.Input
}

type assemblyEnvironmentEdge struct {
	assembly *Assembly
	target   *assemblyPoint
	input    equation.Input
}

type assemblyFactorEdge struct {
	assembly *Assembly
	target   *assemblyPoint
	input    equation.Input
	factor   *factorSchema
}

type assemblyMember struct {
	assembly   *Assembly
	point      *assemblyPoint
	rule       *ruleSchema
	occurrence equation.Occurrence
	operand    equation.Operand

	reads      []equation.ResolvedRead
	writes     []equation.ResolvedWrite
	readAt     int
	writeAt    int
	group      *assemblyGroup
	activation *activationVariantBinding
}

type assemblyObservation struct {
	assembly *Assembly
	point    *assemblyPoint
	query    *querySchema
	surfaces []equation.Surface
	at       int
	receipt  *queryReceipt
}

// summaryCoverage is one opaque Assembly-issued summary surface. Its typed Ref
// vector is validated and consumed at construction; later bindings can only
// reference this capability, never reopen its Factor coordinates.
type summaryCoverage struct {
	assembly *Assembly
	factor   *factorSchema
	form     *formSchema
	surface  equation.Surface
}

// summaryInternKey deliberately keys by the exact ClosedRefs pointer carried
// in refs. Independently built equal vectors remain distinct capabilities;
// only a domain that deliberately reuses one immutable vector can share its
// topology surface across summary admissions.
type summaryInternKey struct {
	form *formSchema
	refs any
}

// activationVariantBinding is Assembly-local bookkeeping for one shared
// target-indexed plan. Prototype operands are installed once per Assembly;
// selected Members only choose their endpoint variant during graph expansion.
type activationVariantBinding struct{ attachment *activationVariantAttachment }

func (binding *activationVariantBinding) complete() bool {
	return binding != nil && binding.attachment != nil && binding.attachment.plan != nil && binding.attachment.plan.plan != (equation.VariantPlan{})
}

// SelectorTarget is a sealed output-candidate capability. Exact Ref values
// and weak summary targets are the only issuers; callers cannot spell raw
// equation surfaces or target modes at the Program/Link boundary.
type SelectorTarget interface {
	selectorWaveETarget(*Assembly, *factorSchema) (equation.Surface, bool)
}

func (ref Ref[K]) selectorWaveETarget(_ *Assembly, factor *factorSchema) (equation.Surface, bool) {
	return ref.exactWaveESurface(factor, equation.SurfaceWriteExact, equation.TargetModeStrong)
}

// weakSummaryTarget is the one Factor-owned weak target implementation. It is
// backed by an issued summary surface and cannot outlive or cross its
// disposable Assembly.
type weakSummaryTarget struct {
	assembly *Assembly
	factor   *factorSchema
	surface  equation.Surface
}

func (target *weakSummaryTarget) selectorWaveETarget(assembly *Assembly, factor *factorSchema) (equation.Surface, bool) {
	if target == nil || assembly == nil || target.assembly != assembly || !validAssembly(assembly) || target.factor != factor ||
		target.surface.Form != equation.SurfaceWriteExact || target.surface.Mode != equation.TargetModeWeak {
		return equation.Surface{}, false
	}
	return target.surface, true
}

// CandidateRelation gives the complete positional relation from one
// prior staged write to the candidate vector of a later selector write. It
// is required exactly for prior WriteDependency entries; no relation is
// inferred from Go declaration order or from a runtime callback.
type CandidateRelation[V any] struct {
	prior   Write[V]
	matches [][]uint64
}

func NewCandidateRelation[V any](prior Write[V], matches [][]uint64) CandidateRelation[V] {
	return CandidateRelation[V]{prior: prior, matches: cloneOrdinals(matches)}
}

// exactRef is intentionally private. Ref is the only engine-created value
// that implements it, so callers cannot spell raw factor coordinates or a
// second surface language.
type exactRef interface {
	exactWaveESurface(*factorSchema, equation.SurfaceForm, equation.TargetMode) (equation.Surface, bool)
	stagedRaw(*factorSchema) (uint64, bool)
}

func (ref Ref[K]) exactWaveESurface(factor *factorSchema, form equation.SurfaceForm, mode equation.TargetMode) (equation.Surface, bool) {
	if !validateRefForSchema(factor, ref) || form != equation.SurfaceReadExact && form != equation.SurfaceWriteExact {
		return equation.Surface{}, false
	}
	if form == equation.SurfaceReadExact && mode != equation.TargetModeNone || form == equation.SurfaceWriteExact && mode != equation.TargetModeStrong {
		return equation.Surface{}, false
	}
	local := uint64(ref.raw) + 1
	if local == 0 {
		return equation.Surface{}, false
	}
	return equation.Surface{Factor: factor.semantic.compositionKey(), Form: form, Local: local, Mode: mode}, true
}

// stagedRaw is the sealed Ref-to-dense-key projection used only by the
// target Factor's typed staged-read authority. The generic engine never sees
// K; foreign, forged, or stale Refs fail before their raw coordinate is used.
func (ref Ref[K]) stagedRaw(factor *factorSchema) (uint64, bool) {
	if !validateRefForSchema(factor, ref) {
		return 0, false
	}
	return uint64(ref.raw), true
}

// assemble is the sole compilation transaction over one sealed source Batch.
// Its callback receives a one-shot cold builder; returning false or retaining
// and using it after the callback rejects the whole transaction.
func assemble(composition *Composition, batch *equation.Batch, declare func(*Assembly) bool) (solver *Solver, ok bool) {
	value := newAssembly(composition, batch)
	if value == nil || declare == nil {
		failAssembly(value)
		return nil, false
	}
	defer func() {
		if recover() != nil {
			failAssembly(value)
			closeAssembly(value)
			value.finished.Store(true)
			solver, ok = nil, false
		}
	}()
	declared := declare(value)
	closed := closeAssembly(value)
	if !declared || !closed {
		failAssembly(value)
		value.finished.Store(true)
		return nil, false
	}
	return compileAssembly(value)
}

func newAssembly(source *Composition, batch *equation.Batch) *Assembly {
	if source == nil || !source.Sealed() || source.coldComposition() == nil || batch == nil || !batch.Sealed() {
		return nil
	}
	return &Assembly{
		composition:   source,
		batch:         batch,
		gate:          newColdGate(),
		observed:      make(map[*querySchema]struct{}, len(source.queries)),
		pointBySite:   make(map[composition.Key]*assemblyPoint),
		summaryIntern: make(map[summaryInternKey]*summaryCoverage),
	}
}

// admitPoint attaches one exact source Site. Scope, initial formula, and
// initialization disposition are already owned by Site; there is no engine
// fallback source grammar or synthetic empty-scope path.
func admitPoint(value *Assembly, site equation.Site) *assemblyPoint {
	if value == nil || !value.gate.begin() {
		return nil
	}
	defer value.gate.end()
	if !validAssembly(value) {
		failAssembly(value)
		return nil
	}
	// A Site is readable after any Batch seals, so availability alone does not
	// prove that it was issued by this Assembly's source Batch.  Foreign sites
	// are a rejected capability at this boundary, but they must not poison an
	// otherwise valid Assembly; the exact source site may still be admitted.
	if !value.batch.OwnsSite(site) {
		return nil
	}
	if value.pointBySite[site.Key()] != nil {
		failAssembly(value)
		return nil
	}
	point := &assemblyPoint{assembly: value, site: site}
	value.points = append(value.points, point)
	value.pointBySite[site.Key()] = point
	return point
}

// bindRuleInstance atomically consumes one domain-owned typed instance at its
// exact source point. The source semantic must be the equation Operand entity;
// neither the operand nor the member is published before every declared
// surface has been validated by the instance's one-shot callback.
func admitInstance[V, O any](value *Assembly, point *assemblyPoint, occurrence equation.Occurrence, operand equation.Operand, instance *RuleInstance[V, O]) (member *assemblyMember) {
	if value == nil || !value.gate.begin() {
		return nil
	}
	defer value.gate.end()
	if !validPoint(value, point) || instance == nil || instance.state == nil || !instance.state.used.CompareAndSwap(false, true) || instance.rule == nil || instance.rule.schema == nil ||
		instance.kind != ruleInstanceDirect ||
		instance.rule.composition != value.composition ||
		instance.declare == nil ||
		instance.rule.schema.outputKind != ruleFactorOutput || instance.rule.schema.output == nil || instance.rule.schema.support != nil || instance.rule.schema.activation != nil ||
		!occurrence.Available() || !operand.Available() || !occurrence.Site().Same(point.site) || !operand.Occurrence().Same(occurrence) {
		failAssembly(value)
		return nil
	}
	if !matchesInstanceAdmission(instance, value.batch, occurrence, operand) {
		failAssembly(value)
		return nil
	}
	rule := instance.rule
	if rule.composition != value.composition ||
		!rule.available() || !rule.schema.bound || !validColdRule(value.composition, rule.schema) ||
		rule.schema.outputKind != ruleFactorOutput || rule.schema.output == nil || rule.schema.support != nil || rule.schema.activation != nil {
		failAssembly(value)
		return nil
	}
	if !(OperandEntity{key: operand.Entity()}).MatchesContentDigest(instance.content) {
		failAssembly(value)
		return nil
	}
	member = &assemblyMember{assembly: value, point: point, rule: rule.schema, occurrence: occurrence, operand: operand}
	binding := &RuleBinding[V, O]{assembly: value, member: member, rule: rule, gate: newColdGate()}
	declared := false
	closed := false
	func() {
		defer func() {
			closed = binding.gate.close()
			if recover() != nil {
				failAssembly(value)
				declared = false
			}
		}()
		declared = instance.declare(binding)
	}()
	if !declared || !closed || member.readAt != len(member.rule.reads) || member.writeAt != len(member.rule.writes) ||
		!addTopologyOperand(&value.operands, rule, occurrence, operand, instance.operand) {
		failAssembly(value)
		return nil
	}
	value.members = append(value.members, member)
	return member
}

// admitStructural atomically attaches one output-free Support or activation
// trigger instance and its complete declared read surface.
func admitStructural(value *Assembly, point *assemblyPoint, occurrence equation.Occurrence, operand equation.Operand, instance *StructuralInstance) (member *assemblyMember) {
	if value == nil || !value.gate.begin() {
		return nil
	}
	defer value.gate.end()
	if !validPoint(value, point) || instance == nil || instance.state == nil || !instance.state.used.CompareAndSwap(false, true) || instance.rule == nil || instance.declare == nil ||
		instance.rule.composition != value.composition || !instance.rule.bound || !validColdRule(value.composition, instance.rule) ||
		instance.rule.outputKind != ruleStructuralOutput || instance.rule.output != nil ||
		(instance.rule.support == nil) == (instance.rule.activation == nil) ||
		!occurrence.Available() || !operand.Available() || !occurrence.Site().Same(point.site) || !operand.Occurrence().Same(occurrence) {
		failAssembly(value)
		return nil
	}
	member = &assemblyMember{assembly: value, point: point, rule: instance.rule, occurrence: occurrence, operand: operand}
	if instance.rule.activation != nil {
		if instance.rule.activation.family == nil {
			failAssembly(value)
			return nil
		}
		if instance.variant == nil || !instance.variant.application.Available() || instance.variant.plan == nil || !instance.variant.plan.bind(value) {
			failAssembly(value)
			return nil
		}
		member.activation = &activationVariantBinding{attachment: instance.variant}
	}
	binding := &StructuralBinding{assembly: value, member: member, gate: newColdGate()}
	declared := false
	closed := false
	func() {
		defer func() {
			closed = binding.gate.close()
			if recover() != nil {
				failAssembly(value)
				declared = false
			}
		}()
		declared = instance.declare(binding)
	}()
	if !declared || !closed || member.readAt != len(member.rule.reads) || member.writeAt != len(member.rule.writes) {
		failAssembly(value)
		return nil
	}
	value.members = append(value.members, member)
	return member
}

// instanceExactRead supplies the next exact declared Rule read. Summary and selector
// surfaces have their own sealed mappings and never enter this exact path.
func instanceExactRead(value *Assembly, member *assemblyMember, ref exactRef) bool {
	if !validMember(value, member) || ref == nil || member.readAt >= len(member.rule.reads) {
		failAssembly(value)
		return false
	}
	declared := member.rule.reads[member.readAt]
	if declared.form == nil || declared.form.factor == nil || declared.form.readKind != exactReadForm {
		failAssembly(value)
		return false
	}
	surface, ok := ref.exactWaveESurface(declared.form.factor, equation.SurfaceReadExact, equation.TargetModeNone)
	if !ok {
		failAssembly(value)
		return false
	}
	member.reads = append(member.reads, equation.ResolvedRead{Index: uint64(member.readAt), Surface: surface})
	member.readAt++
	return true
}

// instanceStagedRead seals one target Factor/form for a dynamic exact-read
// node. It intentionally receives no candidate vector: exact Ref routes are
// emitted later from the current Product row and must not create a cold
// candidate×root surface.
func instanceStagedRead[RV, S any, Tag selectionTag](value *Assembly, member *assemblyMember, read Read[Selection[Tag, S]], form ReadForm[RV, S]) bool {
	if !validMember(value, member) || member.readAt >= len(member.rule.reads) || !form.valid() || form.schema == nil ||
		form.schema.readKind != exactReadForm {
		failAssembly(value)
		return false
	}
	index := member.readAt
	declared := member.rule.reads[index]
	if declared.form != form.schema || len(declared.depends) == 0 || read.rule != member.rule || read.index != index || read.input.rule != member.rule ||
		read.input.index != declared.input || read.resolve == nil || form.schema.factor == nil || form.schema.factor.composition != value.composition {
		failAssembly(value)
		return false
	}
	selector := coldReadSelectorForRead(member.rule, index)
	if selector == nil || selector.bind == nil || !sameDependencies(declared.depends, selector.depends) {
		failAssembly(value)
		return false
	}
	value.selectorLocal++
	if value.selectorLocal == 0 {
		failAssembly(value)
		return false
	}
	surface := equation.Surface{
		Factor:   form.schema.factor.semantic.compositionKey(),
		Form:     equation.SurfaceReadSelect,
		Local:    value.selectorLocal,
		Semantic: form.schema.semantic.compositionKey(),
	}
	if !surface.Available() {
		failAssembly(value)
		return false
	}
	member.reads = append(member.reads, equation.ResolvedRead{Index: uint64(index), Surface: surface})
	member.readAt++
	return true
}

// admitSummary admits one Factor-owned normalizer over a closed ordered
// vector of owner-issued exact Ref capabilities. Within one Assembly, the
// exact same form and immutable ClosedRefs pointer reuse one topology surface;
// independently built vectors remain distinct even when their contents agree.
// Its callers already own the Assembly gate, and the private cache is
// disposable lowering scratch that never crosses the public binding API.
func admitSummary[K ~uint32 | ~uint64, V, S any](value *Assembly, form ReadForm[V, S], refs *ClosedRefs[K]) *summaryCoverage {
	if !validSummaryForm(value, form) || refs == nil || value.summaryIntern == nil {
		failAssembly(value)
		return nil
	}
	key := summaryInternKey{form: form.schema, refs: refs}
	if shared := value.summaryIntern[key]; shared != nil {
		if !validSummaryBinding(value, shared) {
			failAssembly(value)
			return nil
		}
		return shared
	}
	sealed, sealedOK := refs.sealedRefsForAssembly(form.schema.factor)
	if !sealedOK {
		failAssembly(value)
		return nil
	}
	keys := make([]uint64, len(sealed))
	for index, ref := range sealed {
		keys[index] = uint64(ref.raw)
	}
	surface, ok := newSummarySurface(value, form.schema)
	if !ok {
		failAssembly(value)
		return nil
	}
	value.summaries = append(value.summaries, equation.SummaryMapping{Surface: surface, Keys: keys})
	shared := &summaryCoverage{assembly: value, factor: form.schema.factor, form: form.schema, surface: surface}
	value.summaryIntern[key] = shared
	return shared
}

// instanceSummaryRead supplies the next declared summary read through the opaque
// summary binding already sealed from typed Ref capabilities.
func instanceSummaryRead(value *Assembly, member *assemblyMember, summary *summaryCoverage) bool {
	if !validMember(value, member) || member.readAt >= len(member.rule.reads) || !validSummaryBinding(value, summary) {
		failAssembly(value)
		return false
	}
	index := member.readAt
	if member.rule.reads[index].form != summary.form {
		failAssembly(value)
		return false
	}
	member.reads = append(member.reads, equation.ResolvedRead{Index: uint64(index), Surface: summary.surface})
	member.readAt++
	return true
}

// instanceExactWrite supplies the next exact strong Rule output.
func instanceExactWrite(value *Assembly, member *assemblyMember, ref exactRef) bool {
	if !validMember(value, member) || ref == nil || member.writeAt >= len(member.rule.writes) {
		failAssembly(value)
		return false
	}
	declared := member.rule.writes[member.writeAt]
	if declared.form == nil || declared.form.factor == nil || declared.form != member.rule.output.exactWrite || declared.form.writeKind != exactWriteForm {
		failAssembly(value)
		return false
	}
	surface, ok := ref.exactWaveESurface(declared.form.factor, equation.SurfaceWriteExact, equation.TargetModeStrong)
	if !ok {
		failAssembly(value)
		return false
	}
	member.writes = append(member.writes, equation.ResolvedWrite{Index: uint64(member.writeAt), Surface: surface})
	member.writeAt++
	return true
}

// instanceRouteWrite attaches the one selected-read ordinal consumed by the
// next route output. The local route surface is only topology identity; its
// concrete exact targets are issued from the Selection at execution.
func instanceRouteWrite(value *Assembly, member *assemblyMember, read int) bool {
	if !validMember(value, member) || read < 0 || read >= member.readAt || read >= len(member.rule.reads) || member.writeAt >= len(member.rule.writes) {
		failAssembly(value)
		return false
	}
	index := member.writeAt
	declared := member.rule.writes[index]
	selected := member.rule.reads[read]
	if declared.form == nil || declared.form != member.rule.output.exactWrite || declared.form.writeKind != exactWriteForm || declared.route != uint64(read)+1 ||
		selected.form == nil || selected.form.factor != member.rule.output || selected.form.readKind != exactReadForm || coldSelectorForRead(member.rule, read) == nil {
		failAssembly(value)
		return false
	}
	value.routeWriteLocal++
	if value.routeWriteLocal == 0 {
		failAssembly(value)
		return false
	}
	surface := equation.Surface{Factor: declared.form.factor.semantic.compositionKey(), Form: equation.SurfaceWriteRoute, Local: value.routeWriteLocal}
	if !surface.Available() {
		failAssembly(value)
		return false
	}
	member.writes = append(member.writes, equation.ResolvedWrite{Index: uint64(index), Surface: surface, Route: uint64(read) + 1})
	member.writeAt++
	return true
}

// WeakTarget derives one weak target from an already-sealed typed Ref vector.
// The private summary surface is created and consumed under this one Assembly
// operation, so callers cannot retain or replace it.
func WeakTarget[K ~uint32 | ~uint64, V, S any](value *Assembly, form ReadForm[V, S], refs *ClosedRefs[K]) SelectorTarget {
	if value == nil || !value.gate.begin() {
		return nil
	}
	defer value.gate.end()
	summary := admitSummary(value, form, refs)
	if !validSummaryBinding(value, summary) {
		failAssembly(value)
		return nil
	}
	value.weakLocal++
	if value.weakLocal == 0 {
		failAssembly(value)
		return nil
	}
	weak := equation.Surface{Factor: summary.factor.semantic.compositionKey(), Form: equation.SurfaceWriteExact, Local: value.weakLocal, Mode: equation.TargetModeWeak}
	if !weak.Available() {
		failAssembly(value)
		return nil
	}
	value.weakTargets = append(value.weakTargets, equation.WeakTargetMapping{Surface: weak, Candidates: []equation.Surface{summary.surface}})
	return &weakSummaryTarget{assembly: value, factor: summary.factor, surface: weak}
}

// instanceSelectorWrite attaches the candidate target vector for the next
// declared selector write. A relation is required for every prior staged
// write dependency and rejected for every other dependency, keeping the
// cold dependency algebra exact at the one Assembly boundary.
func instanceSelectorWrite[V any](value *Assembly, member *assemblyMember, form WriteForm[V], targets []SelectorTarget, relations []CandidateRelation[V]) bool {
	if !validMember(value, member) || member.writeAt >= len(member.rule.writes) || !form.valid() || form.schema == nil ||
		form.schema.factor == nil || form.schema.factor.composition != value.composition || form.schema.writeKind != selectorWriteForm {
		failAssembly(value)
		return false
	}
	index := member.writeAt
	if member.rule.writes[index].form != form.schema {
		failAssembly(value)
		return false
	}
	selector := coldSelectorForWrite(member.rule, index)
	if selector == nil || len(targets) == 0 || len(targets) != len(selector.candidates) {
		failAssembly(value)
		return false
	}
	resolved := make([]equation.Surface, len(targets))
	for candidate, target := range targets {
		surface, ok := target.selectorWaveETarget(value, form.schema.factor)
		if !ok {
			failAssembly(value)
			return false
		}
		resolved[candidate] = surface
	}
	resolvedRelations, ok := bindSelectorWriteRelations(member.rule, selector, relations, len(targets))
	if !ok {
		failAssembly(value)
		return false
	}
	candidates := make([]uint64, len(selector.candidates))
	for candidate, read := range selector.candidates {
		if read < 0 {
			failAssembly(value)
			return false
		}
		candidates[candidate] = uint64(read)
	}
	value.selectorWriteLocal++
	if value.selectorWriteLocal == 0 {
		failAssembly(value)
		return false
	}
	surface := equation.Surface{Factor: form.schema.factor.semantic.compositionKey(), Form: equation.SurfaceWriteSelect, Local: value.selectorWriteLocal, Semantic: form.schema.semantic.compositionKey()}
	if !surface.Available() {
		failAssembly(value)
		return false
	}
	member.writes = append(member.writes, equation.ResolvedWrite{Index: uint64(index), Surface: surface, Candidates: candidates, TargetCandidates: resolved, Relations: resolvedRelations})
	member.writeAt++
	return true
}

func bindSelectorWriteRelations[V any](rule *ruleSchema, selector *coldWriteSelector, relations []CandidateRelation[V], candidateCount int) ([]equation.CandidateRelation, bool) {
	if rule == nil || selector == nil || candidateCount <= 0 {
		return nil, false
	}
	dependencies := make([]int, 0, len(selector.depends))
	for _, dependency := range selector.depends {
		if dependency.rule != rule {
			return nil, false
		}
		if dependency.kind == writeDependency {
			dependencies = append(dependencies, dependency.index)
		}
	}
	if len(relations) != len(dependencies) {
		return nil, false
	}
	result := make([]equation.CandidateRelation, len(relations))
	for index, dependency := range dependencies {
		relation := relations[index]
		if relation.prior.rule != rule || relation.prior.index != dependency || dependency < 0 || dependency >= len(rule.writes) ||
			len(relation.matches) != candidateCount {
			return nil, false
		}
		limit, ok := selectorWriteCandidateCount(rule, dependency)
		if !ok {
			return nil, false
		}
		for candidate, matches := range relation.matches {
			if !validCandidateOrdinals(matches, limit) {
				return nil, false
			}
			result[index].Matches = append(result[index].Matches, append([]uint64(nil), matches...))
			if candidate >= candidateCount {
				return nil, false
			}
		}
		result[index].Prior = uint64(dependency)
	}
	return result, true
}

func selectorWriteCandidateCount(rule *ruleSchema, index int) (uint64, bool) {
	if rule == nil || index < 0 || index >= len(rule.writes) {
		return 0, false
	}
	write := rule.writes[index]
	if write.form == nil {
		return 0, false
	}
	switch write.form.writeKind {
	case exactWriteForm:
		return 1, true
	case selectorWriteForm:
		selector := coldSelectorForWrite(rule, index)
		if selector == nil || len(selector.candidates) == 0 {
			return 0, false
		}
		return uint64(len(selector.candidates)), true
	default:
		return 0, false
	}
}

func validCandidateOrdinals(values []uint64, limit uint64) bool {
	for index, value := range values {
		if value >= limit || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func cloneOrdinals(rows [][]uint64) [][]uint64 {
	result := make([][]uint64, len(rows))
	for index, row := range rows {
		result[index] = append([]uint64(nil), row...)
	}
	return result
}

// admitGroup attaches one exact simultaneous member set to a source point.
// A Point may own several guarded Groups; each member belongs to exactly one.
// Sequential source effects remain separate Program points, so no ordering is
// inferred by the engine.
func admitGroup(value *Assembly, output *assemblyPoint, members ...*assemblyMember) *assemblyGroup {
	if value == nil || !value.gate.begin() {
		return nil
	}
	defer value.gate.end()
	if !validPoint(value, output) || len(members) == 0 {
		failAssembly(value)
		return nil
	}
	inputs := -1
	writers := make(map[*factorSchema]struct{}, len(members))
	for _, member := range members {
		if !validMember(value, member) || member.point != output || member.group != nil {
			failAssembly(value)
			return nil
		}
		if inputs == -1 {
			inputs = member.rule.inputs
		}
		if inputs != member.rule.inputs {
			failAssembly(value)
			return nil
		}
		if member.rule.outputKind == ruleFactorOutput {
			if member.rule.output == nil {
				failAssembly(value)
				return nil
			}
			if _, duplicate := writers[member.rule.output]; duplicate {
				failAssembly(value)
				return nil
			}
			writers[member.rule.output] = struct{}{}
		} else if member.rule.outputKind != ruleStructuralOutput || member.rule.output != nil || member.rule.support == nil && member.rule.activation == nil {
			failAssembly(value)
			return nil
		}
	}
	group := &assemblyGroup{assembly: value, output: output, members: append([]*assemblyMember(nil), members...)}
	for _, member := range members {
		member.group = group
	}
	value.groups = append(value.groups, group)
	return group
}

// admitBoundary attaches one complete Program/Link boundary. Its source and
// target Site, provenance, precondition, reindex, and postcondition were all
// issued before this engine cut; the engine never rebuilds an identity edge.
func admitBoundary(value *Assembly, group *assemblyGroup, boundary equation.Input) bool {
	if value == nil || !value.gate.begin() {
		return false
	}
	defer value.gate.end()
	if !validGroup(value, group) || !boundary.Available() || !boundary.Target().Same(group.output.site) {
		failAssembly(value)
		return false
	}
	group.inputs = append(group.inputs, boundary)
	return true
}

// admitEnvironmentInput attaches one extra exact boundary to a Group. It is
// intentionally separate from admitBoundary: the Rule schema arity and its
// conjunctive Product vector remain unchanged.
func admitEnvironmentInput(value *Assembly, group *assemblyGroup, boundary equation.Input) bool {
	if value == nil || !value.gate.begin() {
		return false
	}
	defer value.gate.end()
	if !validGroup(value, group) || !boundary.Available() || !boundary.Target().Same(group.output.site) || group.environment != nil {
		failAssembly(value)
		return false
	}
	copy := boundary
	group.environment = &copy
	return true
}

func admitEnvironmentEdge(value *Assembly, target *assemblyPoint, boundary equation.Input) bool {
	if value == nil || !value.gate.begin() {
		return false
	}
	defer value.gate.end()
	if !validPoint(value, target) || !boundary.Available() || !boundary.Target().Same(target.site) || !assemblyContainsSite(value, boundary.Source()) {
		failAssembly(value)
		return false
	}
	for _, edge := range value.environmentEdges {
		if edge != nil && edge.target == target && edge.input.Key() == boundary.Key() {
			failAssembly(value)
			return false
		}
	}
	value.environmentEdges = append(value.environmentEdges, &assemblyEnvironmentEdge{assembly: value, target: target, input: boundary})
	return true
}

// admitQueryAt atomically binds one one-shot QueryInstance at a source point.
func admitQueryAt[R any](value *Assembly, point *assemblyPoint, instance *QueryInstance[R]) (result *assemblyObservation) {
	if value == nil || !value.gate.begin() {
		return nil
	}
	defer value.gate.end()
	if !validPoint(value, point) || instance == nil || instance.state == nil || !instance.state.used.CompareAndSwap(false, true) || instance.query == nil ||
		instance.query.schema == nil || instance.query.composition != value.composition || instance.declare == nil {
		failAssembly(value)
		return nil
	}
	query := instance.query
	if query.composition != value.composition ||
		!query.composition.Sealed() || !query.schema.bound || !validColdQuery(value.composition, coldQuery{schema: query.schema}) {
		failAssembly(value)
		return nil
	}
	// A cold Query schema is a family authority, not a singleton structural
	// observation.  Each one-shot QueryInstance contributes one concrete row;
	// duplicate canonical rows are rejected later by equation.SealTopology.
	result = &assemblyObservation{assembly: value, point: point, query: query.schema, receipt: newQueryReceipt(value, query.schema)}
	binding := &QueryBinding[R]{assembly: value, observation: result, query: query, gate: newColdGate()}
	declared := false
	closed := false
	func() {
		defer func() {
			closed = binding.gate.close()
			if recover() != nil {
				failAssembly(value)
				declared = false
			}
		}()
		declared = instance.declare(binding)
	}()
	if !declared || !closed || result.at != len(result.query.reads) {
		failAssembly(value)
		return nil
	}
	value.observations = append(value.observations, result)
	value.observed[query.schema] = struct{}{}
	instance.receipt = result.receipt
	return result
}

func queryExact(value *Assembly, observation *assemblyObservation, ref exactRef) bool {
	if !validObservation(value, observation) || ref == nil || observation.at >= len(observation.query.reads) {
		failAssembly(value)
		return false
	}
	declared := observation.query.reads[observation.at]
	if declared.form == nil || declared.form.factor == nil || declared.form.readKind != exactReadForm {
		failAssembly(value)
		return false
	}
	surface, ok := ref.exactWaveESurface(declared.form.factor, equation.SurfaceReadExact, equation.TargetModeNone)
	if !ok {
		failAssembly(value)
		return false
	}
	observation.surfaces = append(observation.surfaces, surface)
	observation.at++
	return true
}

// querySummaryRead is the Query-instance summary path. Queries
// receive the same opaque binding and cannot manufacture coordinates.
func querySummaryRead(value *Assembly, observation *assemblyObservation, summary *summaryCoverage) bool {
	if !validObservation(value, observation) || observation.at >= len(observation.query.reads) || !validSummaryBinding(value, summary) {
		failAssembly(value)
		return false
	}
	declared := observation.query.reads[observation.at]
	if declared.form != summary.form {
		failAssembly(value)
		return false
	}
	observation.surfaces = append(observation.surfaces, summary.surface)
	observation.at++
	return true
}

// compileAssembly seals one ordinary equation topology then creates its initial
// runtime.  Each later accepted activation repeats only the immutable
// revision binder; boundRule keeps the direct O and the binder is discarded.
func compileAssembly(value *Assembly) (*Solver, bool) {
	if !validAssembly(value) {
		failAssembly(value)
		return nil, false
	}
	if !completeAssembly(value) {
		failAssembly(value)
		return nil, false
	}
	if !value.operands.freeze() {
		failAssembly(value)
		return nil, false
	}
	spec, ok := lowerAssembly(value)
	if !ok {
	}
	// Summary interning is disposable lowering scratch. The sealed
	// topology owns the single canonical mapping from this point onward; no
	// Ref-pointer table survives as runtime analysis state.
	value.summaryIntern = nil
	value.finished.Store(true)
	if !ok {
		return nil, false
	}
	topology, ok := equation.SealTopology(value.composition.coldComposition(), spec)
	if !ok || topology == nil {
		return nil, false
	}
	operands, detached := value.operands.detach()
	if !detached {
		return nil, false
	}
	solver, compiled := compileSolver(value.composition, topology, operands)
	if !compiled || solver == nil {
		return nil, false
	}
	solver.assembly = value
	if !bindAssemblyQueryReceipts(value, solver) {
		return nil, false
	}
	return solver, true
}

func completeAssembly(value *Assembly) bool {
	if value == nil || value.composition == nil || !value.composition.Sealed() || value.batch == nil || !value.batch.Sealed() || len(value.points) == 0 || len(value.members) == 0 {
		return false
	}
	for _, member := range value.members {
		if !validMember(value, member) || member.group == nil || member.readAt != len(member.rule.reads) || member.writeAt != len(member.rule.writes) ||
			member.rule.activation != nil && !member.activation.complete() {
			return false
		}
	}
	for _, point := range value.points {
		if !validPoint(value, point) {
			return false
		}
	}
	for _, group := range value.groups {
		if !validGroup(value, group) {
			return false
		}
		for _, member := range group.members {
			if !validMember(value, member) || member.group != group || member.point != group.output || member.rule == nil || len(group.inputs) != member.rule.inputs {
				return false
			}
		}
		for _, boundary := range group.inputs {
			if !boundary.Available() || !boundary.Target().Same(group.output.site) || !assemblyContainsSite(value, boundary.Source()) {
				return false
			}
		}
		if group.environment != nil {
			if !group.environment.Available() || !group.environment.Target().Same(group.output.site) || !assemblyContainsSite(value, group.environment.Source()) {
				return false
			}
		}
	}
	for _, edge := range value.environmentEdges {
		if edge == nil || !validPoint(value, edge.target) || !edge.input.Available() || !edge.input.Target().Same(edge.target.site) || !assemblyContainsSite(value, edge.input.Source()) {
			return false
		}
	}
	for _, edge := range value.factorEdges {
		if edge == nil || !validPoint(value, edge.target) || !edge.input.Available() || !edge.input.Target().Same(edge.target.site) || !assemblyContainsSite(value, edge.input.Source()) || !knownAssemblyFactor(value.composition, edge.factor) {
			return false
		}
	}
	if len(value.observed) != len(value.composition.queries) || len(value.observations) < len(value.composition.queries) {
		return false
	}
	for _, declared := range value.composition.queries {
		if declared.schema == nil {
			return false
		}
		if _, present := value.observed[declared.schema]; !present {
			return false
		}
	}
	for _, observation := range value.observations {
		if !validObservation(value, observation) || observation.at != len(observation.query.reads) {
			return false
		}
	}
	return true
}

func assemblyContainsSite(value *Assembly, site equation.Site) bool {
	if value == nil || !site.Available() {
		return false
	}
	point := value.pointBySite[site.Key()]
	return point != nil && point.site.Same(site)
}

func lowerAssembly(value *Assembly) (equation.TopologySpec, bool) {
	if !completeAssembly(value) || value.batch == nil || !value.batch.Sealed() {
		return equation.TopologySpec{}, false
	}
	spec := equation.TopologySpec{
		Batch:              value.batch,
		Rules:              make([]equation.RuleInstance, len(value.members)),
		Points:             make([]equation.PointSpec, len(value.points)),
		Groups:             make([]equation.Group, len(value.groups)),
		Queries:            make([]equation.QueryInstance, len(value.observations)),
		EnvironmentEdges:   make([]equation.EnvironmentEdge, len(value.environmentEdges)),
		FactorEdges:        make([]equation.FactorEdge, len(value.factorEdges)),
		ActivationBindings: make([]equation.ActivationBinding, 0),
		Summaries:          append([]equation.SummaryMapping(nil), value.summaries...),
		WeakTargets:        append([]equation.WeakTargetMapping(nil), value.weakTargets...),
	}
	memberAt := make(map[*assemblyMember]int, len(value.members))
	for index, member := range value.members {
		row, rowOK := lowerAssemblyRule(member)
		if !rowOK {
			return equation.TopologySpec{}, false
		}
		spec.Rules[index] = row
		memberAt[member] = index
	}
	pointAt := make(map[*assemblyPoint]int, len(value.points))
	for index, point := range value.points {
		if !point.site.Available() {
			return equation.TopologySpec{}, false
		}
		spec.Points[index] = equation.PointSpec{Site: point.site}
		pointAt[point] = index
	}
	for index, member := range value.members {
		if member.rule.activation == nil {
			continue
		}
		if member.activation == nil || !member.activation.complete() {
			return equation.TopologySpec{}, false
		}
		attachment := member.activation.attachment
		if attachment == nil || attachment.plan == nil {
			return equation.TopologySpec{}, false
		}
		family := member.rule.activation.family.semantic.compositionKey()
		ports, portsOK := activationEquationPortBindings(value, pointAt, attachment.ports)
		if !portsOK {
			return equation.TopologySpec{}, false
		}
		spec.ActivationBindings = append(spec.ActivationBindings, equation.ActivationBinding{Family: family, Trigger: equation.RuleAt(index), Application: attachment.application.compositionKey(), Plan: attachment.plan.plan, PortBindings: ports})
	}
	for index, group := range value.groups {
		members := make([]equation.RuleRef, len(group.members))
		for memberIndex, member := range group.members {
			memberRow, present := memberAt[member]
			if !present {
				return equation.TopologySpec{}, false
			}
			members[memberIndex] = equation.RuleAt(memberRow)
		}
		output, present := pointAt[group.output]
		if !present {
			return equation.TopologySpec{}, false
		}
		var environment equation.Input
		if group.environment != nil {
			environment = *group.environment
		}
		spec.Groups[index] = equation.Group{Members: members, Output: equation.PointAt(output), Inputs: append([]equation.Input(nil), group.inputs...), EnvironmentInput: environment}
	}
	for index, edge := range value.environmentEdges {
		if edge == nil {
			return equation.TopologySpec{}, false
		}
		target, present := pointAt[edge.target]
		if !present {
			return equation.TopologySpec{}, false
		}
		spec.EnvironmentEdges[index] = equation.EnvironmentEdge{Target: equation.PointAt(target), Input: edge.input}
	}
	for index, edge := range value.factorEdges {
		if edge == nil || edge.factor == nil {
			return equation.TopologySpec{}, false
		}
		target, present := pointAt[edge.target]
		if !present || !knownAssemblyFactor(value.composition, edge.factor) {
			return equation.TopologySpec{}, false
		}
		spec.FactorEdges[index] = equation.FactorEdge{Target: equation.PointAt(target), Input: edge.input, Factor: edge.factor.semantic.compositionKey()}
	}
	for index, observation := range value.observations {
		point, present := pointAt[observation.point]
		if !present {
			return equation.TopologySpec{}, false
		}
		spec.Queries[index] = equation.QueryInstance{Family: observation.query.semantic.compositionKey(), Point: equation.PointAt(point), Surfaces: append([]equation.Surface(nil), observation.surfaces...)}
	}
	return spec, true
}

// lowerAssemblyRule is the one conversion from a fully typed Rule binding to
// equation's data-only row. Ordinary Assembly and activation prototype
// lowering share it so the latter cannot grow a second surface grammar.
func lowerAssemblyRule(member *assemblyMember) (equation.RuleInstance, bool) {
	if member == nil || member.rule == nil || member.readAt != len(member.rule.reads) || member.writeAt != len(member.rule.writes) {
		return equation.RuleInstance{}, false
	}
	carries := make([]equation.ResolvedCarry, len(member.rule.carries))
	for carry := range carries {
		carries[carry] = equation.ResolvedCarry{Index: uint64(carry)}
	}
	row := equation.RuleInstance{
		Schema:        member.rule.semantic.compositionKey(),
		OperandFamily: member.rule.operandFamily.compositionKey(),
		Occurrence:    member.occurrence,
		Operand:       member.operand,
		Reads:         append([]equation.ResolvedRead(nil), member.reads...),
		Carries:       carries,
		Writes:        cloneResolvedWrites(member.writes),
	}
	if member.rule.support != nil {
		completion, prune := member.rule.support.completion, member.rule.support.prune
		if completion == nil || prune == nil || completion.prune != prune {
			return equation.RuleInstance{}, false
		}
		row.Supports = []equation.ResolvedSupport{{Index: 0, Surface: equation.StructuralSurface{Local: 1, Semantic: completion.semantic.compositionKey()}}}
		row.Prunes = []equation.ResolvedPrune{{Index: 0, Surface: equation.StructuralSurface{Local: 1, Semantic: prune.semantic.compositionKey()}}}
	}
	return row, true
}

func cloneResolvedWrites(values []equation.ResolvedWrite) []equation.ResolvedWrite {
	result := make([]equation.ResolvedWrite, len(values))
	for index, value := range values {
		result[index] = equation.ResolvedWrite{
			Index: value.Index, Surface: value.Surface, Route: value.Route,
			Candidates:       append([]uint64(nil), value.Candidates...),
			TargetCandidates: append([]equation.Surface(nil), value.TargetCandidates...),
			Relations:        make([]equation.CandidateRelation, len(value.Relations)),
		}
		for relation, row := range value.Relations {
			result[index].Relations[relation] = equation.CandidateRelation{Prior: row.Prior, Matches: cloneOrdinals(row.Matches)}
		}
	}
	return result
}

func validAssembly(value *Assembly) bool {
	return value != nil && !value.failed.Load() && !value.finished.Load() && value.composition != nil && value.composition.Sealed() && value.composition.coldComposition() != nil && value.batch != nil && value.batch.Sealed()
}

func knownAssemblyFactor(composition *Composition, factor *factorSchema) bool {
	if composition == nil || factor == nil || factor.composition != composition || !factor.bound || !factor.semantic.Available() {
		return false
	}
	for _, known := range composition.factors {
		if known == factor {
			return true
		}
	}
	return false
}

func validPoint(value *Assembly, point *assemblyPoint) bool {
	return validAssembly(value) && point != nil && point.assembly == value && value.batch.OwnsSite(point.site)
}

func validGroup(value *Assembly, group *assemblyGroup) bool {
	return validAssembly(value) && group != nil && group.assembly == value && validPoint(value, group.output) && len(group.members) != 0
}

func validMember(value *Assembly, member *assemblyMember) bool {
	return validAssembly(value) && member != nil && member.assembly == value && validPoint(value, member.point) && member.rule != nil &&
		member.rule.composition == value.composition && member.rule.bound && member.rule.operandFamily.Available() &&
		member.occurrence.Available() && member.operand.Available() && member.occurrence.Site().Same(member.point.site) && member.operand.Occurrence().Same(member.occurrence)
}

func validObservation(value *Assembly, observation *assemblyObservation) bool {
	return validAssembly(value) && observation != nil && observation.assembly == value && validPoint(value, observation.point) && observation.query != nil && observation.query.composition == value.composition && observation.query.bound
}

func validSummaryForm[V, S any](value *Assembly, form ReadForm[V, S]) bool {
	return validAssembly(value) && form.valid() && form.schema != nil && form.schema.factor != nil &&
		form.schema.factor.composition == value.composition && form.schema.readKind == summaryReadForm
}

func validSummaryBinding(value *Assembly, summary *summaryCoverage) bool {
	return validAssembly(value) && summary != nil && summary.assembly == value && summary.factor != nil &&
		summary.form != nil && summary.form.factor == summary.factor && summary.form.readKind == summaryReadForm && summary.surface.Available()
}

func newSummarySurface(value *Assembly, form *formSchema) (equation.Surface, bool) {
	if value == nil || form == nil || form.factor == nil || form.readKind != summaryReadForm {
		return equation.Surface{}, false
	}
	value.summaryLocal++
	if value.summaryLocal == 0 {
		return equation.Surface{}, false
	}
	surface := equation.Surface{
		Factor:     form.factor.semantic.compositionKey(),
		Form:       equation.SurfaceReadSummary,
		Local:      value.summaryLocal,
		Semantic:   form.semantic.compositionKey(),
		Normalizer: form.semantic.compositionKey(),
	}
	return surface, surface.Available()
}

func failAssembly(value *Assembly) {
	if value != nil {
		value.failed.Store(true)
	}
}

func closeAssembly(value *Assembly) bool {
	return value != nil && value.gate != nil && value.gate.close()
}
