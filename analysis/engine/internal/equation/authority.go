package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

// Topology is the one sealed structural authority for a Solver. It owns the
// ordinary graph and the immutable receipt-origin activation selections.
type Topology struct {
	source *composition.Composition
	// deferredQueries is issued only by the receipt-only observation seal. It
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

	receipts         []activationReceipt
	receiptAt        map[composition.Key]int
	receiptByTrigger map[composition.Key][]int
	reverses         []derivedActivationReverse
}

// activationReceipt is the sealed index of one pre-materialized target row.
// It contains no template, port table, or lowering capability.
type activationReceipt struct {
	key         composition.Key
	family      composition.Key
	trigger     composition.Key
	application composition.Key
	target      composition.Key
	endpoint    composition.Key
	direct      DirectActivationCandidate
}

// Deprecated compatibility storage for the in-flight selector migration.
// It is not used by graph lowering; receipt-origin rows are authoritative.
// derivedActivationReverse is produced exclusively by a sealed binding's
// actual export ports. It is not a second user-authored declaration.
type derivedActivationReverse struct {
	binding composition.Key
	target  composition.Key
	trigger composition.Key
}

func (receipt *activationReceipt) issue(owner *Topology, locator PairLocator) (Member, bool) {
	if receipt == nil || owner == nil || !receipt.key.Available() || !receipt.family.Available() || !receipt.trigger.Available() {
		return Member{}, false
	}
	if receipt.application != locator.Application || receipt.target != locator.Target || receipt.endpoint != locator.Endpoint {
		return Member{}, false
	}
	return Member{owner: owner, binding: receipt.key, locator: locator}, true
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

// SealObservationTopologyWithFailure is the narrow receipt-only topology
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
	assembly, assembled := SealTopologyAssembly(spec.Batch, spec.Materializations)
	if !assembled || !assembly.Available() {
		return nil, sealRefused(SealFailureFamilyTopology, "assembly"), false
	}
	materializations := append([]TemplateMaterialization(nil), spec.Materializations...)
	directCandidates := append([]DirectActivationCandidate(nil), spec.DirectCandidates...)
	sealed := copyTopologySpec(spec)
	if !reissueTopologySpec(&sealed, assembly) {
		return nil, sealRefused(SealFailureFamilyTopology, "reissue"), false
	}
	if !appendAssemblyTargets(&sealed, assembly.Targets()) {
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
		source:           source,
		deferredQueries:  deferredQueries,
		receiptAt:        make(map[composition.Key]int),
		receiptByTrigger: make(map[composition.Key][]int),
	}
	if !topology.sealReceipts(sealed.Batch, len(sealed.Points), materializations, directCandidates, instances) {
		return nil, sealRefused(SealFailureFamilyTopology, "receipts"), false
	}
	if !sealed.Batch.closesOperandRealms(sealed.Rules) {
		return nil, sealRefused(SealFailureFamilyTopology, "operand-realms"), false
	}
	graph, rows, compileFailure, compiled := compileTopologyWithFailure(source, sealed, topology.reverses, deferredQueries)
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

// appendAssemblyTargets joins the already-directory-owned target rows to the
// caller's ordinary topology rows. Point and rule references are local to
// each materialized target receipt, so they are shifted exactly once at this
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

// reissueTopologySpec replaces builder-local capabilities with the exact
// directory issued by TopologyAssembly. Existing base-only topologies retain
// their Batch identity; a materialized assembly can never leak a foreign
// Batch into ordinary compilation.
func reissueTopologySpec(spec *TopologySpec, assembly TopologyAssembly) bool {
	if spec == nil || !assembly.Available() || spec.Batch == nil {
		return false
	}
	if assembly.Batch() == spec.Batch {
		spec.Materializations = nil
		return true
	}
	reissueInput := func(input Input) (Input, bool) {
		if !input.Available() {
			return Input{}, false
		}
		source, sourceOK := assembly.Site(input.Source())
		target, targetOK := assembly.Site(input.Target())
		if !sourceOK || !targetOK {
			return Input{}, false
		}
		result := BoundaryInput(source, target, input.Provenance(), input.Pre(), input.Reindex(), input.Post())
		return result, result.Available()
	}
	for index := range spec.Rules {
		occurrence, occurrenceOK := assembly.Occurrence(spec.Rules[index].Occurrence)
		operand, operandOK := assembly.Operand(spec.Rules[index].Operand)
		if !occurrenceOK || !operandOK || !operand.Occurrence().Same(occurrence) {
			return false
		}
		spec.Rules[index].Occurrence, spec.Rules[index].Operand = occurrence, operand
	}
	for index := range spec.Points {
		site, ok := assembly.Site(spec.Points[index].Site)
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
	spec.Batch = assembly.Batch()
	spec.Materializations = nil
	return true
}

func (topology *Topology) sealReceipts(base *Batch, pointCount int, values []TemplateMaterialization, direct []DirectActivationCandidate, instances []canonicalInstance) bool {
	if topology == nil || topology.source == nil || base == nil || pointCount == 0 {
		return false
	}
	receipts := make([]activationReceipt, 0, len(values)+len(direct))
	seen := make(map[composition.Key]struct{}, len(values)+len(direct))
	for _, value := range values {
		origin, ok := value.Origin()
		// Structural/template assembly laws may materialize a target without
		// an activation trigger. Such rows still belong in the sealed graph,
		// but cannot mint a runtime selection receipt. Production activation
		// callers attach origin before admission.
		if !ok {
			continue
		}
		if origin.TriggerOrdinal < 0 || origin.TriggerOrdinal >= len(instances) {
			return false
		}
		if _, known := topology.source.ActivationFamily(origin.Family); !known {
			return false
		}
		instance := instances[origin.TriggerOrdinal]
		schema, schemaOK := ruleSchema(topology.source, instance.row.Schema)
		if !schemaOK || len(schema.Activations) != 1 || schema.Activations[0].Family != origin.Family {
			return false
		}
		key, keyed := identityKey("analysis/engine/equation/activation-receipt", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, value.Key()) && writeKey(writer, instance.key) && writeKey(writer, origin.Family) && writeKey(writer, origin.Application) && writeKey(writer, origin.Target) && writeKey(writer, origin.Endpoint)
		})
		if !keyed {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
		receipts = append(receipts, activationReceipt{key: key, family: origin.Family, trigger: instance.key, application: origin.Application, target: origin.Target, endpoint: origin.Endpoint})
	}
	for _, value := range direct {
		origin, ok := value.Origin()
		if !ok || !value.OwnedBy(topology.source, base) || origin.TriggerOrdinal < 0 || origin.TriggerOrdinal >= len(instances) {
			return false
		}
		if _, known := topology.source.ActivationFamily(origin.Family); !known {
			return false
		}
		instance := instances[origin.TriggerOrdinal]
		schema, schemaOK := ruleSchema(topology.source, instance.row.Schema)
		if !schemaOK || len(schema.Activations) != 1 || schema.Activations[0].Family != origin.Family {
			return false
		}
		for index := 0; index < value.TransportCount(); index++ {
			transport, transportOK := value.TransportAt(index)
			if !transportOK || int(uint64(transport.Source)) > pointCount || int(uint64(transport.Target)) > pointCount {
				return false
			}
		}
		key, keyed := identityKey("analysis/engine/equation/activation-receipt", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, value.Key()) && writeKey(writer, instance.key) && writeKey(writer, origin.Family) && writeKey(writer, origin.Application) && writeKey(writer, origin.Target) && writeKey(writer, origin.Endpoint)
		})
		if !keyed {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
		receipts = append(receipts, activationReceipt{key: key, family: origin.Family, trigger: instance.key, application: origin.Application, target: origin.Target, endpoint: origin.Endpoint, direct: value})
	}
	sort.Slice(receipts, func(left, right int) bool { return lessKey(receipts[left].key, receipts[right].key) })
	topology.receipts = receipts
	topology.receiptAt = make(map[composition.Key]int, len(receipts))
	topology.receiptByTrigger = make(map[composition.Key][]int)
	for index := range receipts {
		topology.receiptAt[receipts[index].key] = index
		topology.receiptByTrigger[receipts[index].trigger] = append(topology.receiptByTrigger[receipts[index].trigger], index)
	}
	return true
}

// ownsMember rechecks the opaque Member against this topology's sealed Axes.
// Members are deliberately issued on demand, not retained in a candidate
// catalog, so pointer ownership alone is not an authority check.
func (topology *Topology) ownsMember(member Member) bool {
	if topology == nil || member.owner != topology || !member.binding.Available() || !member.locator.Available() {
		return false
	}
	index, found := topology.receiptAt[member.binding]
	if !found || index < 0 || index >= len(topology.receipts) {
		return false
	}
	receipt := topology.receipts[index]
	return receipt.application == member.locator.Application && receipt.target == member.locator.Target && receipt.endpoint == member.locator.Endpoint
}

// ActivationReceipt returns the one sealed receipt owned by one exact trigger
// occurrence and cold family. One trigger has exactly one binding: allowing a
// slice here would let singleton axes reconstruct a compatibility table.
func (topology *Topology) ActivationReceipt(trigger, family composition.Key) (composition.Key, bool) {
	if topology == nil || !trigger.Available() || !family.Available() {
		return composition.Key{}, false
	}
	indexes := topology.receiptByTrigger[trigger]
	if len(indexes) == 0 {
		return composition.Key{}, false
	}
	for _, index := range indexes {
		if index >= 0 && index < len(topology.receipts) && topology.receipts[index].family == family {
			return topology.receipts[index].key, true
		}
	}
	return composition.Key{}, false
}

// ActivationApplication returns the exact application constituent sealed on
// one trigger binding. It exposes no plan, target, endpoint, or candidate
// enumeration and therefore cannot reconstruct the forbidden product plane.
func (topology *Topology) ActivationApplication(trigger, family composition.Key) (composition.Key, bool) {
	if topology == nil || !trigger.Available() || !family.Available() {
		return composition.Key{}, false
	}
	indexes := topology.receiptByTrigger[trigger]
	if len(indexes) == 0 {
		return composition.Key{}, false
	}
	for _, index := range indexes {
		if index >= 0 && index < len(topology.receipts) {
			receipt := topology.receipts[index]
			if receipt.family == family && receipt.application.Available() {
				return receipt.application, true
			}
		}
	}
	return composition.Key{}, false
}

// SelectReceiptMember converts one trigger locator only when an exact
// pre-materialized receipt owns that tuple.
func (topology *Topology) SelectReceiptMember(trigger composition.Key, locator PairLocator) (Member, bool) {
	if topology == nil || !trigger.Available() || !locator.Available() {
		return Member{}, false
	}
	indexes := topology.receiptByTrigger[trigger]
	if len(indexes) == 0 {
		return Member{}, false
	}
	for _, index := range indexes {
		if index < 0 || index >= len(topology.receipts) {
			continue
		}
		receipt := &topology.receipts[index]
		if member, ok := receipt.issue(topology, locator); ok {
			return member, true
		}
	}
	return Member{}, false
}

func (topology *Topology) deriveKey(base composition.Key) (composition.Key, bool) {
	if topology == nil || !base.Available() {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/topology", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, base) || writer.Uint(boolUint(topology.deferredQueries)) != nil || writer.Count(uint64(len(topology.receipts))) != nil {
			return false
		}
		for _, receipt := range topology.receipts {
			if !writeKey(writer, receipt.key) || !writeKey(writer, receipt.family) || !writeKey(writer, receipt.trigger) || !writeKey(writer, receipt.application) || !writeKey(writer, receipt.target) || !writeKey(writer, receipt.endpoint) {
				return false
			}
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
	index, bound := topology.receiptAt[member.Binding()]
	if !bound || index < 0 || index >= len(topology.receipts) {
		return composition.Key{}, false
	}
	receipt := topology.receipts[index]
	return identityKey("analysis/engine/equation/activation-evidence", func(writer *canonical.DigestWriter) bool {
		return writeMemberTuple(writer, member) && writeKey(writer, receipt.family) && writeKey(writer, receipt.trigger) && writeKey(writer, receipt.application) && writeExpr(writer, premise)
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
// Candidate target tuples remain receipt-owned and do not multiply the stable
// structural-member directory.
func (topology *Topology) ActivationMemberRow(ref RuleRef) (ActivationMemberRowLocator, bool) {
	index := int(uint64(ref)) - 1
	if topology == nil || index < 0 || index >= len(topology.rows.members) {
		return ActivationMemberRowLocator{}, false
	}
	member := topology.rows.members[index]
	if !member.Available() || len(topology.receiptByTrigger[member]) == 0 {
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
	if locator.owner == nil || !locator.member.Available() || !locator.owner.OwnsGraph(graph) || len(locator.owner.receiptByTrigger[locator.member]) == 0 {
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
// receipt compiler without exposing mutable Graph state to callers.
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
		Materializations: append([]TemplateMaterialization(nil), spec.Materializations...),
		DirectCandidates: append([]DirectActivationCandidate(nil), spec.DirectCandidates...),
		Rules:            make([]RuleInstance, len(spec.Rules)),
		Points:           append([]PointSpec(nil), spec.Points...),
		PointRanks:       append([]int(nil), spec.PointRanks...),
		Groups:           make([]Group, len(spec.Groups)),
		Queries:          cloneQueryInstances(spec.Queries),
		EnvironmentEdges: make([]EnvironmentEdge, len(spec.EnvironmentEdges)),
		FactorEdges:      make([]FactorEdge, len(spec.FactorEdges)),
		Summaries:        make([]SummaryMapping, len(spec.Summaries)),
		WeakTargets:      make([]WeakTargetMapping, len(spec.WeakTargets)),
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

func cloneQueryInstances(rows []QueryInstance) []QueryInstance {
	result := make([]QueryInstance, len(rows))
	for index, row := range rows {
		result[index] = QueryInstance{Family: row.Family, Point: row.Point, Surfaces: append([]Surface(nil), row.Surfaces...)}
	}
	return result
}
