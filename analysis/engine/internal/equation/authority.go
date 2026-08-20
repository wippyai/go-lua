package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

// Topology is the one sealed structural authority for a Solver. It owns the
// ordinary graph and the immutable trigger/locator activation rows.
type Topology struct {
	source *composition.Composition
	// deferredQueries is issued only by the binding-only observation seal. It
	// records that every declared Query family was intentionally deferred to
	// solve-local owned observation roots rather than silently omitted.
	deferredQueries bool
	// initial is the one compiled immutable structural graph. Activation
	// revisions are tiny identity views over this payload; the builder spec
	// and assembly are deliberately not retained after sealing.
	initial *Graph
	key     composition.Key
	rows    compiledRowDirectory
	// initialRelation is the first publication of this Topology. Every later
	// Relation descends from it through Publish, so one Generation sequence
	// orders every accepted set this Topology ever admits.
	initialRelation Relation

	// These are the owner-issued activation rows. They remain immutable after
	// sealing; selection scans them directly instead of rebuilding an index or
	// reissuing a wrapper around each row.
	activation   *activationRowDirectory
	instanceKeys []composition.Key
	// triggers is the declared activation binding of every sealed trigger,
	// keyed by the trigger's canonical rule-instance key. It is the authority
	// for a trigger's family and application; a trigger with no activation row
	// is still bound.
	triggers map[composition.Key]activationTriggerBinding
}

// activationTriggerBinding is the sealed declaration of one activation
// trigger, independent of the candidates it reaches.
type activationTriggerBinding struct {
	family      composition.Key
	application composition.Key
}

// SealTopology freezes a single base graph and its binding descriptors. It
// derives every binding identity from its exact trigger occurrence, rather
// than trusting a caller-authored family key. A later semantic Run can submit
// a raw locator only through Topology.SelectMember, which validates that
// trigger's sole binding and mints one Member. It deliberately does not
// compile one graph per inactive relation.
func SealTopology(source *composition.Composition, spec TopologySpec) (*Topology, bool) {
	topology, _, ok := SealTopologyWithFailure(source, spec)
	return topology, ok
}

// SealTopologyWithFailure is the production diagnostic companion to
// SealTopology. It exposes only the first closed phase, never caller rows,
// coordinates, or mutable compiler state.
func SealTopologyWithFailure(source *composition.Composition, spec TopologySpec) (*Topology, SealFailure, bool) {
	return sealTopologyWithFailure(source, spec, false)
}

// SealObservationTopologyWithFailure is the narrow binding-only topology
// seal for a schema whose Query families are all deferred to solve-local
// observation roots. It admits exactly zero ordinary Query rows; partial and
// ordinary-query topologies remain the responsibility of SealTopology.
func SealObservationTopologyWithFailure(source *composition.Composition, spec TopologySpec) (*Topology, SealFailure, bool) {
	return sealTopologyWithFailure(source, spec, true)
}

func sealTopologyWithFailure(source *composition.Composition, spec TopologySpec, deferredQueries bool) (*Topology, SealFailure, bool) {
	if source == nil || !validTopologyBatch(spec.Batch, spec) {
		return nil, sealRefused(SealFailureFamilyTopology, "input"), false
	}
	if deferredQueries && len(source.Queries()) == 0 {
		return nil, sealRefused(SealFailureFamilyTopology, "deferred-queries"), false
	}
	// A nil slice is normalized to an empty disposable row plane.  The
	// topology always seals through the one activation-row directory.
	if spec.ActivationRows == nil {
		spec.ActivationRows = []ActivationRowSpec{}
	}
	return sealTopologyWithActivationDirectory(source, spec, deferredQueries)
}

// sealTopologyWithActivationDirectory is the seq4161 path.  All formal
// target lowering, direct transport rows, trigger tuple validation, and
// target-Batch reindexing finish before the ordinary graph compiler runs.
// Topology retains only the one activationRowDirectory.
func sealTopologyWithActivationDirectory(source *composition.Composition, spec TopologySpec, deferredQueries bool) (*Topology, SealFailure, bool) {
	directory, directoryOK := sealActivationRowDirectory(source, spec.Batch, spec.ActivationRows)
	if !directoryOK || directory == nil || !directory.available() {
		return nil, sealRefused(SealFailureFamilyTopology, "activation-directory"), false
	}
	sealed := copyTopologySpec(spec)
	sealed.ActivationRows = nil
	if !reissueTopologySpecDirectory(&sealed, directory) {
		return nil, sealRefused(SealFailureFamilyTopology, "reissue"), false
	}
	if !appendAssemblyTargets(&sealed, directory.targetSpecs()) {
		return nil, sealRefused(SealFailureFamilyTopology, "targets"), false
	}
	if deferredQueries && len(sealed.Queries) != 0 {
		return nil, sealRefused(SealFailureFamilyTopology, "deferred-queries"), false
	}
	_, _, _, baseOK := buildPoints(sealed.Points)
	if !baseOK {
		return nil, sealRefused(SealFailureFamilyTopology, "points"), false
	}
	catalog, catalogOK := buildTopologyCatalog(TopologySpec{Rules: sealed.Rules, Summaries: sealed.Summaries, WeakTargets: sealed.WeakTargets})
	if !catalogOK {
		return nil, sealRefused(SealFailureFamilyTopology, "catalog"), false
	}
	instances, instancesOK := buildInstances(source, sealed.Batch, sealed.Rules, catalog)
	if !instancesOK {
		return nil, sealRefused(SealFailureFamilyTopology, "instances"), false
	}
	topology := &Topology{
		source: source, deferredQueries: deferredQueries,
		activation: directory,
		triggers:   make(map[composition.Key]activationTriggerBinding),
	}
	if !topology.sealTriggers(sealed.ActivationTriggers, instances) {
		return nil, sealRefused(SealFailureFamilyTopology, "activation-triggers"), false
	}
	if !topology.sealActivationDirectory(directory, len(sealed.Points), instances) {
		return nil, sealRefused(SealFailureFamilyTopology, "activation-rows"), false
	}
	if !sealed.Batch.closesOperandRealms(sealed.Rules) {
		return nil, sealRefused(SealFailureFamilyTopology, "operand-realms"), false
	}
	graph, rows, compileFailure, compiled := compileTopologyWithFailure(source, sealed, nil, deferredQueries)
	if !compiled || graph == nil {
		return nil, compileFailure, false
	}
	baseKey, graphKeyFailure, keyed := graphSemanticKeyWithFailure(graph)
	if !keyed {
		return nil, graphKeyFailure, false
	}
	key, keyed := topology.deriveKey(baseKey)
	if !keyed {
		return nil, sealRefused(SealFailureFamilyIdentity, "topology-key"), false
	}
	topology.initial, topology.key, topology.rows = graph, key, rows
	graph.owner = topology
	if !topology.sealInitialRelation() {
		return nil, sealRefused(SealFailureFamilyIdentity, "topology-key"), false
	}
	graph.relation = topology.initialRelation
	return topology, SealFailure{}, true
}

func reissueTopologySpecDirectory(spec *TopologySpec, directory *activationRowDirectory) bool {
	if spec == nil || directory == nil || !directory.available() || spec.Batch == nil {
		return false
	}
	base := spec.Batch
	if directory.batch == base {
		spec.ActivationRows = nil
		return true
	}
	reissueInput := func(input Input) (Input, bool) {
		if !input.Available() {
			return Input{}, false
		}
		if input.Source().batch == directory.batch && input.Target().batch == directory.batch {
			return input, true
		}
		source, sourceOK := directory.site(base, input.Source())
		target, targetOK := directory.site(base, input.Target())
		if !sourceOK || !targetOK {
			return Input{}, false
		}
		result := BoundaryInput(source, target, input.Provenance(), input.Pre(), input.Reindex(), input.Post())
		return result, result.Available()
	}
	for index := range spec.Rules {
		occurrence, occurrenceOK := directory.occurrence(base, spec.Rules[index].Occurrence)
		operand, operandOK := directory.operand(base, spec.Rules[index].Operand)
		if !occurrenceOK || !operandOK || !operand.Occurrence().Same(occurrence) {
			return false
		}
		spec.Rules[index].Occurrence, spec.Rules[index].Operand = occurrence, operand
	}
	for index := range spec.Points {
		site, ok := directory.site(base, spec.Points[index].Site)
		if !ok {
			return false
		}
		spec.Points[index].Site = site
	}
	for index := range spec.Groups {
		for input := range spec.Groups[index].Inputs {
			bound, ok := reissueInput(spec.Groups[index].Inputs[input])
			if !ok {
				return false
			}
			spec.Groups[index].Inputs[input] = bound
		}
		if spec.Groups[index].EnvironmentInput.Available() {
			bound, ok := reissueInput(spec.Groups[index].EnvironmentInput)
			if !ok {
				return false
			}
			spec.Groups[index].EnvironmentInput = bound
		}
	}
	for index := range spec.EnvironmentEdges {
		bound, ok := reissueInput(spec.EnvironmentEdges[index].Input)
		if !ok {
			return false
		}
		spec.EnvironmentEdges[index].Input = bound
	}
	for index := range spec.FactorEdges {
		bound, ok := reissueInput(spec.FactorEdges[index].Input)
		if !ok {
			return false
		}
		spec.FactorEdges[index].Input = bound
	}
	spec.Batch = directory.batch
	spec.ActivationRows = nil
	return true
}

// appendAssemblyTargets joins the already-directory-owned target rows to the
// caller's ordinary topology rows. Point and rule references are local to
// each materialized target binding, so they are shifted exactly once at this
// boundary; no target Batch capability can reach the compiler.
func appendAssemblyTargets(spec *TopologySpec, targets []TopologySpec) bool {
	if spec == nil {
		return false
	}
	for _, target := range targets {
		if target.Batch != spec.Batch {
			return false
		}
		pointOffset, ruleOffset := len(spec.Points), len(spec.Rules)
		if len(spec.PointRanks) == 0 && pointOffset != 0 {
			spec.PointRanks = make([]int, pointOffset)
			for index := range spec.PointRanks {
				spec.PointRanks[index] = index
			}
		}
		targetRanks := target.PointRanks
		if len(targetRanks) == 0 && len(target.Points) != 0 {
			targetRanks = make([]int, len(target.Points))
			for index := range targetRanks {
				targetRanks[index] = index
			}
		}
		if len(targetRanks) != 0 && len(targetRanks) != len(target.Points) {
			return false
		}
		spec.Points = append(spec.Points, target.Points...)
		if len(targetRanks) != 0 {
			for _, rank := range targetRanks {
				spec.PointRanks = append(spec.PointRanks, rank+pointOffset)
			}
		}
		for _, rule := range target.Rules {
			spec.Rules = append(spec.Rules, copyInstance(rule))
		}
		for _, group := range target.Groups {
			members := make([]RuleRef, len(group.Members))
			for index, member := range group.Members {
				if member == 0 {
					return false
				}
				members[index] = RuleRef(uint64(member) + uint64(ruleOffset))
			}
			output := PointRef(uint64(group.Output) + uint64(pointOffset))
			if group.Output == 0 {
				return false
			}
			bound := Group{Members: members, Output: output, Inputs: append([]Input(nil), group.Inputs...), EnvironmentInput: group.EnvironmentInput, premise: group.premise}
			spec.Groups = append(spec.Groups, bound)
		}
		for _, edge := range target.FactorEdges {
			if edge.Target == 0 {
				return false
			}
			spec.FactorEdges = append(spec.FactorEdges, FactorEdge{Target: PointRef(uint64(edge.Target) + uint64(pointOffset)), Input: edge.Input, Factor: edge.Factor})
		}
		for _, edge := range target.EnvironmentEdges {
			if edge.Target == 0 {
				return false
			}
			spec.EnvironmentEdges = append(spec.EnvironmentEdges, EnvironmentEdge{Target: PointRef(uint64(edge.Target) + uint64(pointOffset)), Input: edge.Input, TransportOnly: edge.TransportOnly})
		}
		spec.Summaries = append(spec.Summaries, target.Summaries...)
		spec.WeakTargets = append(spec.WeakTargets, target.WeakTargets...)
	}
	return true
}

// sealTriggers seals the declared activation binding of every trigger. Each
// binding names a rule instance that carries exactly one activation of the
// declared family, and no instance is bound twice.
func (topology *Topology) sealTriggers(bindings []ActivationTriggerBinding, instances []canonicalInstance) bool {
	if topology == nil || topology.source == nil {
		return false
	}
	for _, binding := range bindings {
		if binding.TriggerOrdinal < 0 || binding.TriggerOrdinal >= len(instances) ||
			!binding.Family.Available() || !binding.Application.Available() {
			return false
		}
		if _, known := topology.source.ActivationFamily(binding.Family); !known {
			return false
		}
		instance := instances[binding.TriggerOrdinal]
		schema, schemaOK := ruleSchema(topology.source, instance.row.Schema)
		if !schemaOK || len(schema.Activations) != 1 || schema.Activations[0].Family != binding.Family {
			return false
		}
		if _, duplicate := topology.triggers[instance.key]; duplicate || !instance.key.Available() {
			return false
		}
		topology.triggers[instance.key] = activationTriggerBinding{family: binding.Family, application: binding.Application}
	}
	return true
}

// TriggerBound reports whether one rule-instance key is a sealed activation
// trigger of the declared family.
func (topology *Topology) TriggerBound(trigger, family composition.Key) bool {
	if topology == nil || !trigger.Available() || !family.Available() {
		return false
	}
	binding, bound := topology.triggers[trigger]
	return bound && binding.family == family
}

func (topology *Topology) sealActivationDirectory(directory *activationRowDirectory, pointCount int, instances []canonicalInstance) bool {
	if topology == nil || topology.source == nil || directory == nil || !directory.available() || pointCount == 0 {
		return false
	}
	instanceKeys := make([]composition.Key, len(instances))
	for index, instance := range instances {
		if !instance.key.Available() {
			return false
		}
		instanceKeys[index] = instance.key
	}
	seen := make(map[composition.Key]struct{}, len(directory.rows))
	for _, row := range directory.rows {
		if row.locator.triggerOrdinal < 0 || row.locator.triggerOrdinal >= len(instances) || !row.key.Available() {
			return false
		}
		if _, known := topology.source.ActivationFamily(row.locator.family); !known {
			return false
		}
		instance := instances[row.locator.triggerOrdinal]
		schema, schemaOK := ruleSchema(topology.source, instance.row.Schema)
		if !schemaOK || len(schema.Activations) != 1 || schema.Activations[0].Family != row.locator.family {
			return false
		}
		trigger := instance.key
		binding, bound := topology.triggers[trigger]
		if !bound || binding.family != row.locator.family || binding.application != row.locator.application {
			return false
		}
		if row.target != nil && (row.target != directory.batch || !row.target.Sealed()) {
			return false
		}
		for _, transport := range row.transport {
			if !transport.id.Available() || transport.source == 0 || transport.target == 0 || int(uint64(transport.source)) > pointCount || int(uint64(transport.target)) > pointCount || !transport.factor.Available() {
				return false
			}
			if _, known := topology.source.FactorIndex(transport.factor); !known {
				return false
			}
		}
		if _, duplicate := seen[row.key]; duplicate {
			return false
		}
		seen[row.key] = struct{}{}
	}
	topology.instanceKeys = instanceKeys
	return true
}

// ownsMember rechecks the opaque Member against this topology's sealed Axes.
// Members are deliberately issued on demand, not retained in a candidate
// catalog, so pointer ownership alone is not an authority check.
func (topology *Topology) ownsMember(member Member) bool {
	if topology == nil || member.owner != topology || !member.binding.Available() || !member.locator.Available() {
		return false
	}
	if topology.activation != nil {
		row, found := topology.activation.row(member.binding)
		return found && row.locator.application == member.locator.Application && row.locator.target == member.locator.Target && row.locator.endpoint == member.locator.Endpoint
	}
	return false
}

// ActivationApplication returns the exact application constituent sealed on
// one trigger binding. It exposes no plan, target, endpoint, or candidate
// enumeration and therefore cannot reconstruct the forbidden product plane.
func (topology *Topology) ActivationApplication(trigger, family composition.Key) (composition.Key, bool) {
	if topology == nil || !trigger.Available() || !family.Available() {
		return composition.Key{}, false
	}
	binding, bound := topology.triggers[trigger]
	if !bound || binding.family != family || !binding.application.Available() {
		return composition.Key{}, false
	}
	return binding.application, true
}

// SelectActivationMember converts one trigger locator only when an exact
// owner-issued activation row owns that tuple.
func (topology *Topology) SelectActivationMember(trigger composition.Key, locator PairLocator) (Member, bool) {
	if topology == nil || !trigger.Available() || !locator.Available() {
		return Member{}, false
	}
	if topology.activation != nil {
		for _, row := range topology.activation.rows {
			if row.locator.triggerOrdinal < 0 || row.locator.triggerOrdinal >= len(topology.instanceKeys) || topology.instanceKeys[row.locator.triggerOrdinal] != trigger {
				continue
			}
			if row.locator.application == locator.Application && row.locator.target == locator.Target && row.locator.endpoint == locator.Endpoint {
				return Member{owner: topology, binding: row.key, locator: locator}, true
			}
		}
		return Member{}, false
	}
	return Member{}, false
}

func (topology *Topology) deriveKey(base composition.Key) (composition.Key, bool) {
	if topology == nil || !base.Available() {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/topology", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, base) || writer.Uint(boolUint(topology.deferredQueries)) != nil {
			return false
		}
		if topology.activation != nil {
			return writeKey(writer, topology.activation.key)
		}
		return true
	})
}

// Key is the opaque canonical identity of the complete sealed topology.  It
// commits to the base ordinary graph and the symbolic family descriptors, not
// to an inactive candidate enumeration or a physical layout.
func (topology *Topology) Key() composition.Key {
	if topology == nil || !topology.key.Available() {
		return composition.Key{}
	}
	return topology.key
}

// ValidAccepted proves that an accepted set belongs to this sealed Topology
// and is canonically ordered. It is the whole membership fence and derives
// nothing: a caller that only needs to reject a foreign or malformed set never
// pays for a structural digest.
func (topology *Topology) ValidAccepted(accepted []AcceptedMember) bool {
	if topology == nil || !topology.key.Available() || !topology.validAccepted(accepted) {
		return false
	}
	for _, row := range accepted {
		if !topology.ownsMember(row.Member()) {
			return false
		}
	}
	return true
}

// deriveRelationDigest derives the exact structural identity for an accepted
// set. The evidence digest is part of the token: cancellation and a fresh
// epoch keep the same accepted fact instead of remembering only the Member
// coordinate. It is called once per publication, by Publish and by seal; a
// published Relation stores the result and no reader re-derives it.
func (topology *Topology) deriveRelationDigest(accepted []AcceptedMember) (composition.Key, bool) {
	if topology == nil || !topology.key.Available() || !topology.validAccepted(accepted) {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/topology-revision", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, topology.key) || writer.Count(uint64(len(accepted))) != nil {
			return false
		}
		for _, row := range accepted {
			if !writeMemberTuple(writer, row.Member()) || !writeExpr(writer, row.Premise()) {
				return false
			}
		}
		return true
	})
}

// OwnsComposition proves that this sealed topology was bound to the exact
// cold Composition later used by the runtime binder.
func (topology *Topology) OwnsComposition(source *composition.Composition) bool {
	return topology != nil && topology.source != nil && topology.source == source
}

// Accept validates one exact admitted activation selection and retains its
// exact equation premise. The binding itself owns the exact
// trigger Rule, registered admission identity, and anchor, so callers cannot
// supply parallel/forgeable echoes of those coordinates. The guard FormulaID
// Physical product-row/read indexes never enter identity: they are scheduler
// artifacts, not serializable evidence.
func (topology *Topology) Accept(member Member, premise Expr) (AcceptedMember, bool) {
	if topology == nil || !member.ownedBy(topology) || !premise.Available() {
		return AcceptedMember{}, false
	}
	evidence, ok := topology.acceptedEvidenceKey(member, premise)
	if !ok {
		return AcceptedMember{}, false
	}
	return AcceptedMember{member: member, premise: premise, evidence: evidence}, true
}

// acceptedEvidenceKey is the one evidence constructor for admission and
// monotone premise union. Binding provenance is semantic: a Member alone is
// not an admission proof, so the exact trigger Rule/admission/occurrence are
// retained in both construction paths.
func (topology *Topology) acceptedEvidenceKey(member Member, premise Expr) (composition.Key, bool) {
	if topology == nil || !member.ownedBy(topology) || !premise.Available() {
		return composition.Key{}, false
	}
	if topology.activation == nil {
		return composition.Key{}, false
	}
	row, found := topology.activation.row(member.Binding())
	if !found || row.locator.triggerOrdinal < 0 || row.locator.triggerOrdinal >= len(topology.instanceKeys) {
		return composition.Key{}, false
	}
	trigger := topology.instanceKeys[row.locator.triggerOrdinal]
	if _, bound := topology.triggers[trigger]; !bound {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/activation-evidence", func(writer *canonical.DigestWriter) bool {
		return writeMemberTuple(writer, member) && writeKey(writer, row.locator.family) && writeKey(writer, trigger) && writeKey(writer, row.locator.application) && writeExpr(writer, premise)
	})
}

// Graph returns the structural graph for one published Relation: the sealed
// initial graph at the first publication, and a compact view carrying that
// publication's stamp and stored digest afterwards. It visits only accepted
// Members, never an eager candidate set or a retained builder specification,
// and it derives no digest: the Relation already carries the one derived at
// its publication.
func (topology *Topology) Graph(relation Relation) (*Graph, bool) {
	if topology == nil || topology.source == nil || topology.initial == nil || !relation.OwnedBy(topology) {
		return nil, false
	}
	if relation.generation == topology.initialRelation.generation {
		return topology.initial, true
	}
	view := *topology.initial
	view.self = &view
	view.payload = topology.initial
	view.owner = topology
	view.relation = relation
	return &view, true
}

// OwnsGraph proves that a Graph was issued by this exact sealed Topology.
// The pointer and immutable publication fence prevent equal-content topologies
// from exchanging graph members or runtime bindings.
func (topology *Topology) OwnsGraph(graph *Graph) bool {
	return topology != nil && graph != nil && graph.owner == topology && graph.relation.OwnedBy(topology) && graph.valid() && topology.source != nil && graph.composition == topology.source
}

// PointRowLocator, RuleMemberRowLocator, QueryRowLocator, and
// ActivationMemberRowLocator are opaque sealed correspondences. They retain
// canonical row identity without exporting equation keys or disposable
// builder ordinals to the engine's post-Commit lookup path.
type PointRowLocator struct {
	owner *Topology
	key   composition.Key
}

type RuleMemberRowLocator struct {
	owner *Topology
	key   composition.Key
}

type QueryRowLocator struct {
	owner *Topology
	key   composition.Key
}

type ActivationMemberRowLocator struct {
	owner  *Topology
	member composition.Key
}

// PointRowCount, RuleMemberRowCount, and QueryRowCount report the sealed row
// spans of this Topology. They are the row denominator a consumer directory is
// measured against, derived from the sealed topology alone and never from a
// directory that indexes it.
func (topology *Topology) PointRowCount() int {
	if topology == nil {
		return 0
	}
	return len(topology.rows.points)
}

func (topology *Topology) RuleMemberRowCount() int {
	if topology == nil {
		return 0
	}
	return len(topology.rows.members)
}

func (topology *Topology) QueryRowCount() int {
	if topology == nil {
		return 0
	}
	return len(topology.rows.queries)
}

func (topology *Topology) PointRow(ref PointRef) (PointRowLocator, bool) {
	index := int(uint64(ref)) - 1
	if topology == nil || index < 0 || index >= len(topology.rows.points) || !topology.rows.points[index].Available() {
		return PointRowLocator{}, false
	}
	return PointRowLocator{owner: topology, key: topology.rows.points[index]}, true
}

func (topology *Topology) RuleMemberRow(ref RuleRef) (RuleMemberRowLocator, bool) {
	index := int(uint64(ref)) - 1
	if topology == nil || index < 0 || index >= len(topology.rows.members) || !topology.rows.members[index].Available() {
		return RuleMemberRowLocator{}, false
	}
	return RuleMemberRowLocator{owner: topology, key: topology.rows.members[index]}, true
}

func (topology *Topology) QueryRow(index uint64) (QueryRowLocator, bool) {
	if topology == nil || index >= uint64(len(topology.rows.queries)) || !topology.rows.queries[index].Available() {
		return QueryRowLocator{}, false
	}
	return QueryRowLocator{owner: topology, key: topology.rows.queries[index]}, true
}

// ActivationMemberRow projects one exact trigger Rule reference only when the
// sealed topology owns at least one materialized candidate for that trigger.
// Candidate target tuples remain binding-owned and do not multiply the stable
// structural-member directory.
func (topology *Topology) ActivationMemberRow(ref RuleRef) (ActivationMemberRowLocator, bool) {
	index := int(uint64(ref)) - 1
	if topology == nil || index < 0 || index >= len(topology.rows.members) {
		return ActivationMemberRowLocator{}, false
	}
	member := topology.rows.members[index]
	if _, bound := topology.triggers[member]; !member.Available() || !bound {
		return ActivationMemberRowLocator{}, false
	}
	return ActivationMemberRowLocator{owner: topology, member: member}, true
}

func (locator PointRowLocator) Resolve(graph *Graph) (Point, bool) {
	if locator.owner == nil || !locator.key.Available() || !locator.owner.OwnsGraph(graph) {
		return Point{}, false
	}
	node, found := graph.pointAt[locator.key]
	if !found || int(node) < 0 || int(node) >= len(graph.points) {
		return Point{}, false
	}
	point := graph.points[node]
	return point, graph.OwnsPoint(point) && point.key == locator.key
}

func (locator RuleMemberRowLocator) Resolve(graph *Graph) (RuleMember, bool) {
	if locator.owner == nil || !locator.key.Available() || !locator.owner.OwnsGraph(graph) {
		return RuleMember{}, false
	}
	member, found := graph.memberAt[locator.key]
	return member, found && graph.OwnsMember(member) && member.key == locator.key
}

func (locator QueryRowLocator) Resolve(graph *Graph) (Query, bool) {
	if locator.owner == nil || !locator.key.Available() || !locator.owner.OwnsGraph(graph) {
		return Query{}, false
	}
	query, found := graph.queryAt[locator.key]
	return query, found && graph.OwnsQuery(query) && query.key == locator.key
}

func (locator ActivationMemberRowLocator) Resolve(graph *Graph) (RuleMember, bool) {
	if locator.owner == nil || !locator.member.Available() || !locator.owner.OwnsGraph(graph) {
		return RuleMember{}, false
	}
	if _, bound := locator.owner.triggers[locator.member]; !bound {
		return RuleMember{}, false
	}
	member, found := graph.memberAt[locator.member]
	return member, found && graph.OwnsMember(member) && member.key == locator.member
}

func (topology *Topology) validAccepted(values []AcceptedMember) bool {
	if topology == nil {
		return false
	}
	for index, value := range values {
		if !value.Available() || !value.member.ownedBy(topology) || index > 0 && !lessAcceptedMember(values[index-1], value) {
			return false
		}
		if index > 0 && sameMember(values[index-1].Member(), value.Member()) {
			return false
		}
	}
	return true
}

// MergeAccepted unions immutable evidence facts for one Member.  The caller
// uses it only at the Solver monotone-set boundary; no epoch or Rule callback
// can mutate a previously accepted record.
func (topology *Topology) MergeAccepted(left, right AcceptedMember) (AcceptedMember, bool) {
	if topology == nil || !left.Available() || !right.Available() || !left.member.ownedBy(topology) || !right.member.ownedBy(topology) || !sameMember(left.member, right.member) {
		return AcceptedMember{}, false
	}
	if !topology.ownsMember(left.member) || !topology.ownsMember(right.member) {
		return AcceptedMember{}, false
	}
	premise, ok := OrExpr(left.premise, right.premise)
	if !ok {
		return AcceptedMember{}, false
	}
	evidence, ok := topology.acceptedEvidenceKey(left.member, premise)
	if !ok {
		return AcceptedMember{}, false
	}
	return AcceptedMember{member: left.member, premise: premise, evidence: evidence}, true
}

// graphSemanticKeyWithFailure retains the graph-key failure boundary for the
// binding compiler without exposing mutable Graph state to callers.
func graphSemanticKeyWithFailure(graph *Graph) (composition.Key, SealFailure, bool) {
	if graph == nil || graph.self != graph || graph.composition == nil {
		return composition.Key{}, sealRefused(SealFailureFamilyIdentity, "graph-key-structure"), false
	}
	type reverse struct{ target, trigger composition.Key }
	reverses := make([]reverse, 0)
	for target, triggers := range graph.activationReverses {
		if target < 0 || target >= len(graph.points) {
			return composition.Key{}, sealRefused(SealFailureFamilyIdentity, "graph-key-structure"), false
		}
		for _, trigger := range triggers {
			if trigger < 0 || int(trigger) >= len(graph.points) {
				return composition.Key{}, sealRefused(SealFailureFamilyIdentity, "graph-key-structure"), false
			}
			reverses = append(reverses, reverse{target: graph.points[target].key, trigger: graph.points[trigger].key})
		}
	}
	sort.Slice(reverses, func(left, right int) bool {
		if reverses[left].target != reverses[right].target {
			return lessKey(reverses[left].target, reverses[right].target)
		}
		return lessKey(reverses[left].trigger, reverses[right].trigger)
	})
	scheduleInvalid := false
	key, ok := identityKey("analysis/engine/equation/graph", func(writer *canonical.DigestWriter) bool {
		if writer.Count(uint64(len(graph.points))) != nil {
			return false
		}
		for _, point := range graph.points {
			if !writeKey(writer, point.key) {
				return false
			}
		}
		if writer.Count(uint64(len(graph.groups))) != nil {
			return false
		}
		for _, group := range graph.groups {
			if !writeKey(writer, group.key) {
				return false
			}
		}
		if len(graph.environments) != 0 {
			if writer.Uint(1) != nil || writer.Count(uint64(len(graph.environments))) != nil {
				return false
			}
			for _, edge := range graph.environments {
				if !writeKey(writer, edge.key) || !writeKey(writer, edge.target.key) || !writeKey(writer, edge.input.key) || writer.Uint(boolUint(edge.transportOnly)) != nil {
					return false
				}
			}
		}
		if len(graph.factorEdges) != 0 {
			if writer.Uint(2) != nil || writer.Count(uint64(len(graph.factorEdges))) != nil {
				return false
			}
			for _, edge := range graph.factorEdges {
				if !writeKey(writer, edge.key) || !writeKey(writer, edge.target.key) || !writeKey(writer, edge.input.key) || !writeKey(writer, edge.factor) {
					return false
				}
			}
		}
		if writer.Count(uint64(len(graph.queries))) != nil {
			return false
		}
		for _, query := range graph.queries {
			if !writeKey(writer, query.key) {
				return false
			}
		}
		if writer.Count(uint64(len(reverses))) != nil {
			return false
		}
		for _, row := range reverses {
			if !writeKey(writer, row.target) || !writeKey(writer, row.trigger) {
				return false
			}
		}
		if graph.scheduleRanked {
			if writer.Uint(3) != nil || graph.schedule == nil || writer.Count(uint64(graph.schedule.EventCount())) != nil {
				scheduleInvalid = true
				return false
			}
			for index := 0; index < graph.schedule.EventCount(); index++ {
				event, ok := graph.schedule.EventAt(index)
				if !ok || writer.Uint(uint64(event.Kind)) != nil || writer.Uint(uint64(event.Node)) != nil || writer.Uint(uint64(event.Region+1)) != nil {
					scheduleInvalid = true
					return false
				}
			}
			if writer.Count(uint64(graph.schedule.RegionCount())) != nil {
				scheduleInvalid = true
				return false
			}
			for index := 0; index < graph.schedule.RegionCount(); index++ {
				region, ok := graph.schedule.RegionAt(index)
				if !ok || writer.Uint(uint64(region.Head)) != nil || writer.Uint(uint64(region.Parent+1)) != nil || writer.Uint(uint64(region.Enter)) != nil || writer.Uint(uint64(region.Exit+1)) != nil {
					scheduleInvalid = true
					return false
				}
			}
		}
		return true
	})
	if !ok {
		if scheduleInvalid {
			return composition.Key{}, sealRefused(SealFailureFamilyIdentity, "graph-key-schedule"), false
		}
		return composition.Key{}, sealRefused(SealFailureFamilyIdentity, "graph-key-identity"), false
	}
	return key, SealFailure{}, true
}

func copyTopologySpec(spec TopologySpec) TopologySpec {
	result := TopologySpec{
		Batch:            spec.Batch,
		ActivationRows:   cloneActivationRowSpecs(spec.ActivationRows),
		Rules:            make([]RuleInstance, len(spec.Rules)),
		Points:           append([]PointSpec(nil), spec.Points...),
		PointRanks:       append([]int(nil), spec.PointRanks...),
		Groups:           make([]Group, len(spec.Groups)),
		Queries:          cloneQueryInstances(spec.Queries),
		EnvironmentEdges: make([]EnvironmentEdge, len(spec.EnvironmentEdges)),
		FactorEdges:      make([]FactorEdge, len(spec.FactorEdges)),
		Summaries:        make([]SummaryMapping, len(spec.Summaries)),
		WeakTargets:      make([]WeakTargetMapping, len(spec.WeakTargets)),

		ActivationTriggers: append([]ActivationTriggerBinding(nil), spec.ActivationTriggers...),
	}
	for index, rule := range spec.Rules {
		result.Rules[index] = copyInstance(rule)
	}
	for index, group := range spec.Groups {
		result.Groups[index] = Group{Members: append([]RuleRef(nil), group.Members...), Output: group.Output, Inputs: append([]Input(nil), group.Inputs...), EnvironmentInput: group.EnvironmentInput, premise: group.premise}
	}
	for index, edge := range spec.EnvironmentEdges {
		result.EnvironmentEdges[index] = EnvironmentEdge{Target: edge.Target, Input: edge.Input, TransportOnly: edge.TransportOnly}
	}
	for index, edge := range spec.FactorEdges {
		result.FactorEdges[index] = FactorEdge{Target: edge.Target, Input: edge.Input, Factor: edge.Factor}
	}
	for index, summary := range spec.Summaries {
		result.Summaries[index] = SummaryMapping{Surface: summary.Surface, Keys: append([]uint64(nil), summary.Keys...)}
	}
	for index, mapping := range spec.WeakTargets {
		result.WeakTargets[index] = WeakTargetMapping{Surface: mapping.Surface, Candidates: append([]Surface(nil), mapping.Candidates...)}
	}
	return result
}
