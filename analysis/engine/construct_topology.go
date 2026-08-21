// construct_topology.go holds the pure program-geometry function. Given one
// sealed declaration - a sealed Binding, the sealed source Batch its rows were
// admitted into, the sealed artifact templates the parent issued, the Link
// bootstrap witness, and the member, query and activation rows the owner
// declared - it folds that declaration into the committed geometry: the sealed
// equation Topology, the initial Graph, and the semantic directory addressing
// both.
//
// It reads the sealed artifact rows themselves. A template is never expanded
// into a second row plane: a Point, an edge, a region and a rule are addressed
// by their template handle - the mount index plus the row index - and the only
// tables built here are address tables over those handles plus the equation
// values derived from them.
//
// It is a function, not a transaction. Nothing is retained across a call, no
// domain owner is called back into, and each phase hands the next a value that
// is finished at the moment it is handed over. The schedule-validity gate and
// the duplicate-identity refusal are carried here natively; every refusal is
// returned as a construction-stage failure rather than recorded on a ledger.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

// declaredPointRow is one owner-declared Point. Programs with mounted
// templates carry none: their point plane is the parent's.
type declaredPointRow struct {
	ID   identity.ContentID
	Site equation.Site
}

// declaredMemberPlane names the coordinate plane one member's published
// identity and Group inputs are derived from.
type declaredMemberPlane uint8

const (
	// declaredMemberOwner carries its published identity directly. It is the
	// plane of programs with no mounted templates.
	declaredMemberOwner declaredMemberPlane = iota
	// declaredMemberMount derives identity, stage, input site and route
	// predecessor from the sealed template rule row addressed by
	// role+mount+point+occurrence.
	declaredMemberMount
	// declaredMemberLink derives identity from the Link bootstrap witness
	// under role+occurrence.
	declaredMemberLink
)

// declaredMemberRow is one rule row of the declaration. Row is the sealed
// equation instance the owner's operand sealing produced; every other field is
// a coordinate resolved against the sealed planes.
type declaredMemberRow struct {
	Plane        declaredMemberPlane
	ID           identity.ContentID
	ActivationID identity.ContentID
	Role         RuleSlotCapability
	Mount        identity.ContentID
	Point        identity.ContentID
	Occurrence   identity.ContentID
	Row          equation.RuleInstance
	// Bind is the sealed cell that mints this row's runtime member. Operand is
	// the canonical value issued by the declaration owner; the committed
	// program passes it through at every seal without re-resolving it.
	Bind    ProgramRule
	Coords  OperandCoords
	Operand declaredRuleOperand
	// Activation marks the row as one activation trigger, and Application is
	// the application identity that trigger instantiates its candidates
	// under. The application is declared by the trigger itself, so a trigger
	// that reaches no candidate still states the one it would activate for.
	Activation       bool
	Application      composition.Key
	EnvironmentInput equation.Input
}

// declaredQueryRow is one query row plus the identity it publishes under and
// the mounted point coordinate it is anchored at. Row carries no dense Point
// address: the constructor resolves the coordinate against its own point
// plane, so a query never names a second point authority.
type declaredQueryRow struct {
	ID    identity.ContentID
	Mount identity.ContentID
	Point identity.ContentID
	Row   equation.QueryInstance
	// Admit is the sealed query cell that mints this row's runtime query.
	Admit programQueryAdmit
}

// constructedSitePlane is the admitted source plane of one construction: the
// Site each mounted point and the Link bootstrap point was admitted under. It
// is addressed by mount handle, so it carries no artifact row content.
type constructedSitePlane struct {
	mounted   map[artifactMountedPoint]equation.Site
	bootstrap equation.Site
}

// topologyDeclaration is the sealed input of one construction. Every field is
// a finished value: the Batch is already sealed, the templates are the sealed
// artifact rows themselves, and each declared row already holds the sealed
// equation instance its owner issued.
type topologyDeclaration struct {
	binding            *SchemaBinding
	batch              *equation.Batch
	mounts             []sealedProgramMount
	bootstrap          LinkBootstrapWitness
	sites              constructedSitePlane
	points             []declaredPointRow
	members            []declaredMemberRow
	queries            []declaredQueryRow
	environmentEdges   []equation.EnvironmentEdge
	factorEdges        []equation.FactorEdge
	summaries          []equation.SummaryMapping
	candidates         []declaredActivationCandidate
	observationQueries bool
}

func (declaration topologyDeclaration) mounted() bool { return len(declaration.mounts) != 0 }

// topologyConstructionStep names the exact predicate one refusal failed. It is
// diagnostic detail under the published construction stage: the stage is what
// callers branch on, the step is what a law reads.
type topologyConstructionStep uint8

const (
	topologyConstructionStepNone topologyConstructionStep = iota
	topologyConstructionStepBinding
	topologyConstructionStepSourcePlane
	topologyConstructionStepDeclarationShape
	topologyConstructionStepMountRow
	topologyConstructionStepBootstrapRow
	topologyConstructionStepPointRow
	topologyConstructionStepPointOrder
	topologyConstructionStepEdgeRow
	topologyConstructionStepBootstrapTransport
	topologyConstructionStepMemberIssuance
	topologyConstructionStepMemberRow
	topologyConstructionStepMemberGroup
	topologyConstructionStepActivationRow
	topologyConstructionStepQueryRow
	topologyConstructionStepCandidateRow
	topologyConstructionStepDuplicateIdentity
	topologyConstructionStepTopologySeal
	topologyConstructionStepGraph
	topologyConstructionStepSchedule
	topologyConstructionStepDirectory
)

// topologyConstructionRefusal is the closed refusal of one construction. It
// projects onto the published ProgramSealStage vocabulary: input and
// declaration boundaries refuse at Admission, the geometry commit itself
// refuses at TopologySeal.
type topologyConstructionRefusal struct {
	stage   ProgramSealStage
	step    topologyConstructionStep
	ordinal uint32
}

func (refusal topologyConstructionRefusal) Available() bool {
	return refusal.stage != ProgramSealStageNone
}

func (refusal topologyConstructionRefusal) Stage() ProgramSealStage { return refusal.stage }

func (refusal topologyConstructionRefusal) Step() topologyConstructionStep { return refusal.step }

func (refusal topologyConstructionRefusal) Ordinal() uint32 { return refusal.ordinal }

func (refusal topologyConstructionRefusal) Failure() SolveFailure {
	if !refusal.Available() {
		return SolveFailure{}
	}
	return ProgramStageFailure(refusal.stage)
}

// refuseAdmission closes one declaration-boundary refusal.
func refuseAdmission(step topologyConstructionStep, ordinal int) topologyConstructionRefusal {
	return topologyConstructionRefusal{stage: ProgramSealStageAdmission, step: step, ordinal: constructionOrdinal(ordinal)}
}

// refuseTopologySeal closes one geometry-commit refusal.
func refuseTopologySeal(step topologyConstructionStep, ordinal int) topologyConstructionRefusal {
	return topologyConstructionRefusal{stage: ProgramSealStageTopologySeal, step: step, ordinal: constructionOrdinal(ordinal)}
}

func constructionOrdinal(ordinal int) uint32 {
	if ordinal < 0 {
		return 0
	}
	return uint32(ordinal)
}

// constructedTopology is the committed geometry produced by construction.
type constructedTopology struct {
	program *CommittedProgram
}

func (constructed constructedTopology) Available() bool {
	return constructed.program.valid()
}

// constructedSourcePlane is the sealed source authority a construction reads.
type constructedSourcePlane struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	schema    *Schema
	batch     *equation.Batch
	factors   []schemaFactorBinding
}

// constructedRuleHandle addresses one sealed template rule row: the mount it
// is mounted under, and its index in that mount's template.
type constructedRuleHandle struct {
	mount int
	rule  int
}

// constructedRouteKey addresses one route proof inside one mount.
type constructedRouteKey struct {
	mount int
	route identity.ContentID
}

// constructedBodyHandle addresses one sealed template body row: the mount it
// is mounted under, and its index in that mount's template.
type constructedBodyHandle struct {
	mount int
	body  int
}

// constructedMountPlane is the admitted address space over the sealed
// templates. Every entry is a handle, or an identity derived from one; no
// template row content is copied into it.
type constructedMountPlane struct {
	mounts    []sealedProgramMount
	initial   []int
	rules     map[artifactMountedRule]constructedRuleHandle
	routes    map[constructedRouteKey]int
	bodies    map[artifactMountedBody]constructedBodyHandle
	stages    map[artifactMountedRuleOccurrence]artifactNativeCallStage
	bootstrap LinkBootstrapWitness
	point     LinkBootstrapPoint
	owner     identity.ContentID
}

// constructedPointPlane is the finished point geometry: the dense PointSpec
// rows, their semantic order, and the address tables later phases resolve a
// Point through.
type constructedPointPlane struct {
	specs             []equation.PointSpec
	ranks             []int
	refByID           map[identity.ContentID]equation.PointRef
	idBySite          map[equation.Site]identity.ContentID
	decisions         map[identity.ContentID][]constructedDecision
	idByMounted       map[artifactMountedPoint]identity.ContentID
	bootstrapSemantic identity.ContentID
	bootstrapRef      equation.PointRef
}

// constructedDecision is one decision of a Point in the order the sealed
// template declared it. Reindex maps are positional, so the order is part of
// the value rather than recovered from a map walk.
type constructedDecision struct {
	semantic identity.ContentID
	decision equation.Decision
}

// constructedEdgePlane is the finished environment geometry of one program.
type constructedEdgePlane struct {
	environment []equation.EnvironmentEdge
	factor      []equation.FactorEdge
}

// constructedMemberPlane is the finished member geometry: the dense rule rows,
// one Group per row, and the published member and activation addresses.
type constructedMemberPlane struct {
	bindings     []programMemberBinding
	specs        []equation.RuleInstance
	groups       []equation.Group
	refByID      map[identity.ContentID]equation.RuleRef
	activations  map[identity.ContentID]equation.RuleRef
	activationAt map[equation.RuleRef]identity.ContentID
	// applicationAt is the declared application of each activation trigger.
	// It is the completeness authority for a candidate set: candidates agree
	// with it, and an empty set is complete under it.
	applicationAt map[equation.RuleRef]composition.Key
	// triggers is the same declaration in the form the equation Topology
	// seals it under.
	triggers []equation.ActivationTriggerBinding
}

// constructedQueryPlane is the finished query geometry.
type constructedQueryPlane struct {
	specs       []equation.QueryInstance
	ordinalByID map[identity.ContentID]uint64
}

// constructedActivationPlane is the disposable activation-row recipe passed
// to equation.Topology.  The equation sealer consumes it into its one
// immutable activation-row directory; no receipt object is retained here.
type constructedActivationPlane struct {
	rows []equation.ActivationRowSpec
}

// constructTopology folds one sealed declaration into the committed geometry.
func constructTopology(declaration topologyDeclaration) (constructedTopology, topologyConstructionRefusal) {
	source, refusal := constructSourcePlane(declaration)
	if refusal.Available() {
		return constructedTopology{}, refusal
	}
	mounts, refusal := constructMountPlane(declaration, source)
	if refusal.Available() {
		return constructedTopology{}, refusal
	}
	points, refusal := constructPointPlane(declaration, source, mounts)
	if refusal.Available() {
		return constructedTopology{}, refusal
	}
	edges, refusal := constructEdgePlane(declaration, source, mounts, points)
	if refusal.Available() {
		return constructedTopology{}, refusal
	}
	members, refusal := constructMemberPlane(declaration, source, mounts, points)
	if refusal.Available() {
		return constructedTopology{}, refusal
	}
	queries, refusal := constructQueryPlane(declaration, source, points)
	if refusal.Available() {
		return constructedTopology{}, refusal
	}
	activations, refusal := constructActivationPlane(declaration, source, mounts, points, members)
	if refusal.Available() {
		return constructedTopology{}, refusal
	}
	semantic, refusal := constructSemanticRows(points, members, queries)
	if refusal.Available() {
		return constructedTopology{}, refusal
	}
	constructed, refusal := sealConstructedTopology(declaration, source, mounts, points, edges, members, queries, activations, semantic)
	return constructed, refusal
}

// constructSourcePlane fences the declaration against the sealed Binding it
// claims to be built under.
func constructSourcePlane(declaration topologyDeclaration) (constructedSourcePlane, topologyConstructionRefusal) {
	state := bindingState(declaration.binding)
	if state == nil || state.phase != schemaBindingSealed || state.authority == nil || state.schema == nil || !state.schema.Available() {
		return constructedSourcePlane{}, refuseAdmission(topologyConstructionStepBinding, 0)
	}
	if len(state.factors) != schemaFactorCount(state.schema) {
		return constructedSourcePlane{}, refuseAdmission(topologyConstructionStepBinding, 0)
	}
	for ordinal, factor := range state.factors {
		if factor == nil || !factor.schemaFactorComplete() {
			return constructedSourcePlane{}, refuseAdmission(topologyConstructionStepBinding, ordinal)
		}
	}
	batch := declaration.batch
	if batch == nil || !batch.Sealed() || !batch.Key().Available() {
		return constructedSourcePlane{}, refuseAdmission(topologyConstructionStepSourcePlane, 0)
	}
	// A mounted program's point plane is the parent's; an owner-declared point
	// row alongside it would be a second point authority.
	if declaration.mounted() {
		if len(declaration.points) != 0 || !declaration.bootstrap.Available() || declaration.sites.mounted == nil {
			return constructedSourcePlane{}, refuseAdmission(topologyConstructionStepDeclarationShape, 0)
		}
	} else if len(declaration.points) == 0 {
		return constructedSourcePlane{}, refuseAdmission(topologyConstructionStepDeclarationShape, 0)
	}
	return constructedSourcePlane{
		state:     state,
		authority: state.authority,
		schema:    state.schema,
		batch:     batch,
		factors:   append([]schemaFactorBinding(nil), state.factors...),
	}, topologyConstructionRefusal{}
}

// constructMountPlane admits the sealed templates and the bootstrap witness.
// It resolves the Link role substitution each template rule and transfer names
// and indexes the template rows by handle, so a later phase addresses a rule
// or a route without a second copy of it.
func constructMountPlane(declaration topologyDeclaration, source constructedSourcePlane) (constructedMountPlane, topologyConstructionRefusal) {
	plane := constructedMountPlane{mounts: declaration.mounts}
	if !declaration.mounted() {
		return plane, topologyConstructionRefusal{}
	}
	point, pointOK := declaration.bootstrap.Point()
	if !pointOK || !declaration.bootstrap.OwnerID().Available() {
		return constructedMountPlane{}, refuseAdmission(topologyConstructionStepBootstrapRow, 0)
	}
	if !validateSealedLinkBootstrapCatalog(source.state, declaration.bootstrap) {
		return constructedMountPlane{}, refuseAdmission(topologyConstructionStepBootstrapRow, 0)
	}
	for index := 0; index < declaration.bootstrap.OccurrenceCount(); index++ {
		occurrence, occurrenceOK := declaration.bootstrap.OccurrenceAt(index)
		capability, capabilityOK := declaration.bootstrap.capabilityFor(occurrence)
		if !occurrenceOK || !capabilityOK || !capability.link() || capability.state != source.state || capability.authority != source.authority {
			return constructedMountPlane{}, refuseAdmission(topologyConstructionStepBootstrapRow, index)
		}
	}
	authorized, authorizedOK := sealedLinkBootstrapTransportPair(source.state)
	if authorizedOK {
		seenTransport := make(map[composition.Key]struct{}, len(authorized))
		for index, capability := range authorized {
			factor, factorOK := linkTransportFactorSemantic(source.state, capability)
			if !factorOK {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepBootstrapTransport, index)
			}
			if _, duplicate := seenTransport[factor]; duplicate {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepBootstrapTransport, index)
			}
			seenTransport[factor] = struct{}{}
		}
	}
	plane.bootstrap, plane.point, plane.owner = declaration.bootstrap, point, declaration.bootstrap.OwnerID()

	// Artifact templates are fenced by the binding's explicit cold execution
	// schema, not by the narrower equation-Schema digest. Composition may keep
	// identical engine topology while changing publication layout or ABI; such
	// an Artifact must not cross this mount boundary.
	schemaID := source.state.artifactSchema
	if !schemaID.Available() {
		return constructedMountPlane{}, refuseAdmission(topologyConstructionStepBinding, 0)
	}
	plane.initial = make([]int, len(declaration.mounts))
	plane.rules = make(map[artifactMountedRule]constructedRuleHandle)
	plane.routes = make(map[constructedRouteKey]int)
	plane.bodies = make(map[artifactMountedBody]constructedBodyHandle)
	plane.stages = make(map[artifactMountedRuleOccurrence]artifactNativeCallStage)
	seenModule := make(map[identity.ContentID]struct{}, len(declaration.mounts))
	for index, mount := range declaration.mounts {
		template := mount.template
		if template == nil || !template.Available() || !mount.module.Available() || template.SchemaID() != schemaID {
			return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
		}
		if _, duplicate := seenModule[mount.module]; duplicate {
			return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
		}
		seenModule[mount.module] = struct{}{}
		initial := -1
		for pointIndex := 0; pointIndex < template.PointCount(); pointIndex++ {
			row, rowOK := template.PointAt(pointIndex)
			if !rowOK {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			if !row.Initial {
				continue
			}
			if initial >= 0 {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			initial = pointIndex
		}
		if initial < 0 {
			return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
		}
		plane.initial[index] = initial
		for edgeIndex := 0; edgeIndex < template.EdgeCount(); edgeIndex++ {
			edge, edgeOK := template.EdgeAt(edgeIndex)
			if !edgeOK {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			key := constructedRouteKey{mount: index, route: edge.Route}
			if _, taken := plane.routes[key]; !edge.Route.Available() || taken {
				continue
			}
			plane.routes[key] = edgeIndex
		}
		for transferIndex := 0; transferIndex < template.TransferCount(); transferIndex++ {
			transfer, transferOK := template.TransferAt(transferIndex)
			if !transferOK {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			for _, role := range transfer.Factors {
				if _, capabilityOK := constructedRoleCapability(source, mount, role); !capabilityOK {
					return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
				}
			}
		}
		for bodyIndex := 0; bodyIndex < template.BodyCount(); bodyIndex++ {
			body, bodyOK := template.BodyAt(bodyIndex)
			if !bodyOK {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			key := artifactMountedBody{mount: mount.module, body: body.ID}
			if _, duplicate := plane.bodies[key]; duplicate {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			plane.bodies[key] = constructedBodyHandle{mount: index, body: bodyIndex}
		}
		for ruleIndex := 0; ruleIndex < template.RuleCount(); ruleIndex++ {
			rule, ruleOK := template.RuleAt(ruleIndex)
			if !ruleOK {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			capability, capabilityOK := constructedRoleCapability(source, mount, rule.Role)
			native, nativeOK := template.RuleNativeAt(ruleIndex)
			if !capabilityOK || !rule.Stage.Valid() || !nativeOK {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			key := artifactMountedRule{role: capability, mount: mount.module, point: rule.Point, occurrence: rule.ID}
			if _, duplicate := plane.rules[key]; duplicate {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			plane.rules[key] = constructedRuleHandle{mount: index, rule: ruleIndex}
			if !native {
				continue
			}
			// The native stage inverse is the one row-shaped value a committed
			// program retains, so it is derived here rather than read
			// back from the template at solve time.
			stageKey := artifactMountedRuleOccurrence{role: capability, mount: mount.module, occurrence: rule.ID}
			if _, duplicate := plane.stages[stageKey]; duplicate {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			stage := artifactNativeCallStage{
				stage: rule.Stage, native: native, point: rule.Point, input: rule.Input,
				mountedPoint: mountedArtifactID("analysis/engine/artifact-point/v1", mount.module, template.ArtifactID(), rule.Point),
				mountedInput: mountedArtifactID("analysis/engine/artifact-point/v1", mount.module, template.ArtifactID(), rule.Input),
			}
			if !stage.mountedPoint.Available() || !stage.mountedInput.Available() {
				return constructedMountPlane{}, refuseAdmission(topologyConstructionStepMountRow, index)
			}
			plane.stages[stageKey] = stage
		}
	}
	return plane, topologyConstructionRefusal{}
}

// constructedRoleCapability resolves the Link capability one template role is
// substituted by under this mount.
func constructedRoleCapability(source constructedSourcePlane, mount sealedProgramMount, role rows.ArtifactScalarRole) (RuleSlotCapability, bool) {
	capability, capabilityOK := sealedRoleCapability(mount.capabilities, role)
	if !capabilityOK || capability.state != source.state || capability.authority != source.authority {
		return RuleSlotCapability{}, false
	}
	return capability, true
}

// constructPointPlane derives the dense point rows. A mounted program takes
// them from each template's point stream in mount order, followed by the
// single Link bootstrap anchor; an owner-declared program takes them in
// declaration order.
func constructPointPlane(declaration topologyDeclaration, source constructedSourcePlane, mounts constructedMountPlane) (constructedPointPlane, topologyConstructionRefusal) {
	plane := constructedPointPlane{
		refByID:   make(map[identity.ContentID]equation.PointRef),
		idBySite:  make(map[equation.Site]identity.ContentID),
		decisions: make(map[identity.ContentID][]constructedDecision),
	}
	place := func(id identity.ContentID, site equation.Site) bool {
		if !id.Available() || !site.Available() || !source.batch.OwnsSite(site) {
			return false
		}
		if _, duplicate := plane.refByID[id]; duplicate {
			return false
		}
		if _, duplicate := plane.idBySite[site]; duplicate {
			return false
		}
		plane.refByID[id] = equation.PointAt(len(plane.specs))
		plane.idBySite[site] = id
		plane.specs = append(plane.specs, equation.PointSpec{Site: site})
		return true
	}
	if !declaration.mounted() {
		for ordinal, point := range declaration.points {
			if !place(point.ID, point.Site) {
				return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointRow, ordinal)
			}
		}
		return plane, topologyConstructionRefusal{}
	}
	plane.idByMounted = make(map[artifactMountedPoint]identity.ContentID)
	for _, mount := range mounts.mounts {
		template := mount.template
		artifactID := template.ArtifactID()
		for pointIndex := 0; pointIndex < template.PointCount(); pointIndex++ {
			row, rowOK := template.PointAt(pointIndex)
			if !rowOK {
				return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointRow, pointIndex)
			}
			id := mountedArtifactID("analysis/engine/artifact-point/v1", mount.module, artifactID, row.ID)
			handle := artifactMountedPoint{mount: mount.module, reusable: row.ID}
			site, siteOK := declaration.sites.mounted[handle]
			if _, duplicate := plane.idByMounted[handle]; !siteOK || duplicate || !place(id, site) {
				return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointRow, pointIndex)
			}
			plane.idByMounted[handle] = id
			decisions := make([]constructedDecision, len(row.Decisions))
			for index, semanticID := range row.Decisions {
				key := mountedArtifactID("analysis/engine/artifact-decision/v1", mount.module, artifactID, semanticID)
				decision, decisionOK := equation.NewDecision(mustArtifactSourceKey(artifactPointSource, key))
				if !decisionOK {
					return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointRow, pointIndex)
				}
				decisions[index] = constructedDecision{semantic: semanticID, decision: decision}
			}
			plane.decisions[id] = decisions
		}
	}
	mountedPointCount := len(plane.specs)
	semantic := linkBootstrapPointSemanticID(mounts.owner, mounts.point.PointID)
	if !place(semantic, declaration.sites.bootstrap) {
		return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointRow, mountedPointCount)
	}
	plane.bootstrapSemantic, plane.bootstrapRef = semantic, plane.refByID[semantic]

	// The template WTO stream is the parent-issued semantic order. Its point
	// events become dense ranks aligned with the PointSpec rows; the Link
	// bootstrap anchor is the deterministic rank-zero predecessor of them all.
	ranks := make([]int, len(plane.specs))
	rank := 0
	for _, mount := range mounts.mounts {
		template := mount.template
		seen := make(map[identity.ContentID]struct{}, template.PointCount())
		for eventIndex := 0; eventIndex < template.EventCount(); eventIndex++ {
			event, eventOK := template.EventAt(eventIndex)
			if !eventOK {
				return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointOrder, eventIndex)
			}
			if event.Kind != rows.ArtifactEventPoint {
				continue
			}
			id, located := plane.idByMounted[artifactMountedPoint{mount: mount.module, reusable: event.Point}]
			ref, refOK := plane.refByID[id]
			_, duplicate := seen[event.Point]
			if !event.Point.Available() || duplicate || !located || !refOK {
				return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointOrder, eventIndex)
			}
			seen[event.Point] = struct{}{}
			ranks[int(uint64(ref))-1] = rank + 1
			rank++
		}
		if len(seen) != template.PointCount() {
			return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointOrder, template.EventCount())
		}
	}
	if rank != mountedPointCount {
		return constructedPointPlane{}, refuseAdmission(topologyConstructionStepPointOrder, rank)
	}
	ranks[int(uint64(plane.bootstrapRef))-1] = 0
	plane.ranks = ranks
	return plane, topologyConstructionRefusal{}
}

// constructEdgePlane derives the environment geometry. Route rows and local
// transfers come from the sealed templates and the Link bootstrap transports
// from the witness; owner-declared edges follow them in declaration order.
func constructEdgePlane(declaration topologyDeclaration, source constructedSourcePlane, mounts constructedMountPlane, points constructedPointPlane) (constructedEdgePlane, topologyConstructionRefusal) {
	plane := constructedEdgePlane{}
	for mountIndex, mount := range mounts.mounts {
		template := mount.template
		for edgeIndex := 0; edgeIndex < template.EdgeCount(); edgeIndex++ {
			edge, edgeOK := template.EdgeAt(edgeIndex)
			if !edgeOK {
				return constructedEdgePlane{}, refuseAdmission(topologyConstructionStepEdgeRow, edgeIndex)
			}
			environment, refusal := constructRouteEdge(declaration, source, mounts, points, mountIndex, edge, edgeIndex)
			if refusal.Available() {
				return constructedEdgePlane{}, refusal
			}
			plane.environment = append(plane.environment, environment)
		}
		for transferIndex := 0; transferIndex < template.TransferCount(); transferIndex++ {
			transfer, transferOK := template.TransferAt(transferIndex)
			if !transferOK {
				return constructedEdgePlane{}, refuseAdmission(topologyConstructionStepEdgeRow, transferIndex)
			}
			environment, factors, refusal := constructTransferEdge(declaration, source, mounts, points, mountIndex, transfer, transferIndex)
			if refusal.Available() {
				return constructedEdgePlane{}, refusal
			}
			if len(factors) != 0 {
				plane.factor = append(plane.factor, factors...)
				continue
			}
			plane.environment = append(plane.environment, environment)
		}
	}
	if declaration.mounted() {
		transports, refusal := constructBootstrapTransports(declaration, source, mounts, points)
		if refusal.Available() {
			return constructedEdgePlane{}, refusal
		}
		plane.factor = append(plane.factor, transports...)
	}
	for ordinal, edge := range declaration.environmentEdges {
		if edge.Target == 0 || !bindingOwnsInput(source.batch, edge.Input) {
			return constructedEdgePlane{}, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
		}
		plane.environment = append(plane.environment, edge)
	}
	for ordinal, edge := range declaration.factorEdges {
		if edge.Target == 0 || !edge.Factor.Available() || !bindingOwnsInput(source.batch, edge.Input) || !bindingOwnsFactorSchema(source.schema, edge.Factor) {
			return constructedEdgePlane{}, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
		}
		plane.factor = append(plane.factor, edge)
	}
	return plane, topologyConstructionRefusal{}
}

// constructedBoundary is the reindexed boundary one template route or transfer
// carries between two mounted Points.
type constructedBoundary struct {
	input  equation.Input
	target equation.PointRef
}

// constructMountedBoundary reindexes one mounted from/to pair. Decision maps
// are positional in the source Point's declared order: a decision the target
// no longer scopes, and a decision this row's reset proof forgets, both
// project to Forget.
func constructMountedBoundary(declaration topologyDeclaration, source constructedSourcePlane, points constructedPointPlane, mount, from, to identity.ContentID, provenance composition.Key, pre equation.Expr, resets []identity.ContentID) (constructedBoundary, bool) {
	fromID, fromLocated := points.idByMounted[artifactMountedPoint{mount: mount, reusable: from}]
	toID, toLocated := points.idByMounted[artifactMountedPoint{mount: mount, reusable: to}]
	fromSite, fromSited := declaration.sites.mounted[artifactMountedPoint{mount: mount, reusable: from}]
	toSite, toSited := declaration.sites.mounted[artifactMountedPoint{mount: mount, reusable: to}]
	target, targetOK := points.refByID[toID]
	if !fromLocated || !toLocated || !fromSited || !toSited || !targetOK || !provenance.Available() {
		return constructedBoundary{}, false
	}
	forget := make(map[identity.ContentID]struct{}, len(resets))
	for _, reset := range resets {
		// A recurrence reset is conditional on the decision being live at this
		// source Point. Clearing information absent from the current lexical
		// scope is an exact no-op, not malformed evidence.
		forget[reset] = struct{}{}
	}
	retained := make(map[identity.ContentID]equation.Decision, len(points.decisions[toID]))
	for _, entry := range points.decisions[toID] {
		retained[entry.semantic] = entry.decision
	}
	declared := points.decisions[fromID]
	maps := make([]equation.DecisionMap, len(declared))
	for index, entry := range declared {
		if _, forgotten := forget[entry.semantic]; forgotten {
			maps[index] = equation.Forget(entry.decision)
			continue
		}
		targetDecision, kept := retained[entry.semantic]
		if !kept {
			// Leaving a decision scope is an ordinary exact projection. Absence
			// from the target Point is the parent-owned proof that this route
			// leaves that decision's lexical scope.
			maps[index] = equation.Forget(entry.decision)
			continue
		}
		maps[index] = equation.Identity(entry.decision)
		if targetDecision != entry.decision {
			maps[index] = equation.Rename(entry.decision, targetDecision)
		}
	}
	omega, omegaOK := equation.NewReindex(fromSite.Scope(), toSite.Scope(), maps)
	input := equation.BoundaryInput(fromSite, toSite, provenance, pre, omega, equation.TrueExpr())
	if !omegaOK || !input.Available() || !bindingOwnsInput(source.batch, input) {
		return constructedBoundary{}, false
	}
	return constructedBoundary{input: input, target: target}, true
}

// constructRouteEdge lowers one sealed template route row.
func constructRouteEdge(declaration topologyDeclaration, source constructedSourcePlane, mounts constructedMountPlane, points constructedPointPlane, mountIndex int, edge rows.ArtifactScalarEdge, ordinal int) (equation.EnvironmentEdge, topologyConstructionRefusal) {
	mount := mounts.mounts[mountIndex]
	id := mountedArtifactID("analysis/engine/artifact-environment-edge/v1", mount.module, mount.template.ArtifactID(), edge.ID)
	provenance, provenanceOK := artifactSourceKey(artifactEdgeSource, id)
	if !provenanceOK {
		return equation.EnvironmentEdge{}, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
	}
	pre := equation.TrueExpr()
	if edge.Guarded {
		fromID, located := points.idByMounted[artifactMountedPoint{mount: mount.module, reusable: edge.From}]
		decision, decisionOK := constructedDecisionOf(points, fromID, edge.Decision)
		guard, guardOK := equation.DecisionExpr(decision)
		if !located || !decisionOK || !guardOK {
			return equation.EnvironmentEdge{}, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
		}
		if !edge.Truth {
			guard, guardOK = equation.NotExpr(guard)
			if !guardOK {
				return equation.EnvironmentEdge{}, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
			}
		}
		pre = guard
	}
	boundary, boundaryOK := constructMountedBoundary(declaration, source, points, mount.module, edge.From, edge.To, provenance, pre, edge.Resets)
	if !boundaryOK {
		return equation.EnvironmentEdge{}, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
	}
	transportOnly := edge.From == edge.To && !edge.Component.Available() && !edge.HasReset && !edge.Mu.Available()
	return equation.EnvironmentEdge{Target: boundary.target, Input: boundary.input, TransportOnly: transportOnly}, topologyConstructionRefusal{}
}

// constructedDecisionOf resolves one declared decision of a mounted Point.
func constructedDecisionOf(points constructedPointPlane, point, semantic identity.ContentID) (equation.Decision, bool) {
	for _, entry := range points.decisions[point] {
		if entry.semantic == semantic {
			return entry.decision, true
		}
	}
	return equation.Decision{}, false
}

// constructTransferEdge lowers one sealed template local transfer. A partial
// transfer lowers to one factor edge per transported role; a full transfer
// lowers to an ordinary cross-point environment dependency. TransportOnly is
// reserved for genuine intra-point annotations: suppressing this edge from
// the scheduler would let a target observe stale pre-transfer factor state.
func constructTransferEdge(declaration topologyDeclaration, source constructedSourcePlane, mounts constructedMountPlane, points constructedPointPlane, mountIndex int, transfer rows.ArtifactScalarTransfer, ordinal int) (equation.EnvironmentEdge, []equation.FactorEdge, topologyConstructionRefusal) {
	mount := mounts.mounts[mountIndex]
	if !transfer.ID.Available() || !transfer.From.Available() || !transfer.To.Available() || transfer.From == transfer.To || transfer.Full == (len(transfer.Factors) != 0) {
		return equation.EnvironmentEdge{}, nil, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
	}
	id := mountedArtifactID("analysis/engine/artifact-environment-edge/v1", mount.module, mount.template.ArtifactID(), transfer.ID)
	provenance, provenanceOK := artifactSourceKey(artifactEdgeSource, id)
	if !provenanceOK {
		return equation.EnvironmentEdge{}, nil, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
	}
	boundary, boundaryOK := constructMountedBoundary(declaration, source, points, mount.module, transfer.From, transfer.To, provenance, equation.TrueExpr(), nil)
	if !boundaryOK {
		return equation.EnvironmentEdge{}, nil, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
	}
	if transfer.Full {
		return equation.EnvironmentEdge{Target: boundary.target, Input: boundary.input}, nil, topologyConstructionRefusal{}
	}
	edges := make([]equation.FactorEdge, 0, len(transfer.Factors))
	seen := make(map[composition.Key]struct{}, len(transfer.Factors))
	for _, role := range transfer.Factors {
		capability, capabilityOK := constructedRoleCapability(source, mount, role)
		semantic, factorOK := constructedTransportFactor(source, capability, true)
		if _, duplicate := seen[semantic]; !capabilityOK || !factorOK || duplicate || !bindingOwnsFactorSchema(source.schema, semantic) {
			return equation.EnvironmentEdge{}, nil, refuseAdmission(topologyConstructionStepEdgeRow, ordinal)
		}
		seen[semantic] = struct{}{}
		edges = append(edges, equation.FactorEdge{Target: boundary.target, Input: boundary.input, Factor: semantic})
	}
	return equation.EnvironmentEdge{}, edges, topologyConstructionRefusal{}
}

// constructBootstrapTransports lowers the Link bootstrap factor transports:
// one edge per transported capability into every mount's initial Point.
func constructBootstrapTransports(declaration topologyDeclaration, source constructedSourcePlane, mounts constructedMountPlane, points constructedPointPlane) ([]equation.FactorEdge, topologyConstructionRefusal) {
	authorized, authorizedOK := sealedLinkBootstrapTransportPair(source.state)
	if !authorizedOK {
		return nil, topologyConstructionRefusal{}
	}
	count := len(authorized)
	edges := make([]equation.FactorEdge, 0, count*len(mounts.mounts))
	for ordinal := 0; ordinal < count; ordinal++ {
		capability := authorized[ordinal]
		semantic, factorOK := constructedTransportFactor(source, capability, false)
		if !factorOK || !bindingOwnsFactorSchema(source.schema, semantic) {
			return nil, refuseAdmission(topologyConstructionStepBootstrapTransport, ordinal)
		}
		for mountIndex, mount := range mounts.mounts {
			row, rowOK := mount.template.PointAt(mounts.initial[mountIndex])
			if !rowOK {
				return nil, refuseAdmission(topologyConstructionStepBootstrapTransport, ordinal)
			}
			handle := artifactMountedPoint{mount: mount.module, reusable: row.ID}
			id, located := points.idByMounted[handle]
			target, targetOK := points.refByID[id]
			targetSite, sited := declaration.sites.mounted[handle]
			provenance, provenanceOK := linkBootstrapTransportKey(mounts.owner, artifactPointMetadata{mount: mount.module, artifact: mount.template.ArtifactID(), reusable: row.ID}, semantic)
			reindex, reindexOK := ruleInputReindex(declaration.sites.bootstrap.Scope(), targetSite.Scope())
			input := equation.BoundaryInput(declaration.sites.bootstrap, targetSite, provenance, equation.TrueExpr(), reindex, equation.TrueExpr())
			if !located || !targetOK || !sited || !provenanceOK || !reindexOK || !input.Available() || !bindingOwnsInput(source.batch, input) {
				return nil, refuseAdmission(topologyConstructionStepBootstrapTransport, ordinal)
			}
			edges = append(edges, equation.FactorEdge{Target: target, Input: input, Factor: semantic})
		}
	}
	return edges, topologyConstructionRefusal{}
}

// constructedTransportFactor resolves the Factor one transport capability
// writes. It reads the sealed role slot table and the bound factor cell, the
// two authorities the sealed Binding published them under.
func constructedTransportFactor(source constructedSourcePlane, role RuleSlotCapability, mounted bool) (composition.Key, bool) {
	if mounted && !role.mounted() || !mounted && !role.link() {
		return composition.Key{}, false
	}
	if role.state != source.state || role.authority != source.authority {
		return composition.Key{}, false
	}
	rule, roleOK := source.state.roleSlots[role]
	ruleOrdinal, ruleOK := source.schema.ruleOrdinalOf(rule)
	shape, shapeOK := source.schema.ruleShapeAt(ruleOrdinal)
	factorOrdinal, factorOK := source.schema.factorOrdinalOf(shape.Output)
	if !roleOK || !ruleOK || !shapeOK || shape.OutputKind != composition.FactorOutput || !factorOK || factorOrdinal >= uint64(len(source.factors)) {
		return composition.Key{}, false
	}
	factor := source.factors[factorOrdinal]
	if factor == nil {
		return composition.Key{}, false
	}
	if factor.schemaFactorOrdinal() != factorOrdinal || factor.schemaFactorSchema() != source.schema {
		return composition.Key{}, false
	}
	return shape.Output, true
}

// constructMemberPlane derives the member geometry: one dense rule row and one
// Group per declared member, with the Group's Output resolved through the
// point plane and its Inputs derived from the declaring plane's coordinates.
func constructMemberPlane(declaration topologyDeclaration, source constructedSourcePlane, mounts constructedMountPlane, points constructedPointPlane) (constructedMemberPlane, topologyConstructionRefusal) {
	plane := constructedMemberPlane{
		bindings:      make([]programMemberBinding, 0, len(declaration.members)),
		specs:         make([]equation.RuleInstance, 0, len(declaration.members)),
		groups:        make([]equation.Group, 0, len(declaration.members)),
		refByID:       make(map[identity.ContentID]equation.RuleRef, len(declaration.members)),
		activations:   make(map[identity.ContentID]equation.RuleRef),
		activationAt:  make(map[equation.RuleRef]identity.ContentID),
		applicationAt: make(map[equation.RuleRef]composition.Key),
	}
	for ordinal, member := range declaration.members {
		coordinates, refusal := resolveMemberCoordinates(declaration, mounts, member, ordinal)
		if refusal.Available() {
			return constructedMemberPlane{}, refusal
		}
		row := cloneBindingRuleRow(member.Row)
		if !row.Schema.Available() || !row.OperandFamily.Available() || !row.Occurrence.Available() || !row.Operand.Available() ||
			!source.batch.OwnsOccurrence(row.Occurrence) || !source.batch.OwnsOperand(row.Operand) ||
			!row.Operand.Occurrence().Same(row.Occurrence) || !bindingOwnsRuleSchema(source.schema, row.Schema) {
			return constructedMemberPlane{}, refuseAdmission(topologyConstructionStepMemberRow, ordinal)
		}
		ref := equation.RuleAt(len(plane.specs))
		if _, duplicate := plane.refByID[coordinates.member]; duplicate {
			return constructedMemberPlane{}, refuseAdmission(topologyConstructionStepDuplicateIdentity, ordinal)
		}
		group, refusal := constructMemberGroup(declaration, source, points, member, coordinates, row, ref, ordinal)
		if refusal.Available() {
			return constructedMemberPlane{}, refusal
		}
		plane.refByID[coordinates.member] = ref
		plane.bindings = append(plane.bindings, programMemberBinding{
			member: coordinates.member, activation: coordinates.activation,
			activated: member.Activation, operand: member.Operand, binder: member.Bind,
		})
		plane.specs = append(plane.specs, row)
		plane.groups = append(plane.groups, group)
		if !member.Activation {
			continue
		}
		shape, shapeOK := memberActivationShape(source, row)
		_, duplicate := plane.activations[coordinates.activation]
		if !shapeOK || duplicate || !coordinates.activation.Available() || shape.ActivationCount != 1 || !shape.ActivationFamily.Available() ||
			!member.Application.Available() {
			return constructedMemberPlane{}, refuseAdmission(topologyConstructionStepActivationRow, ordinal)
		}
		plane.activations[coordinates.activation] = ref
		plane.activationAt[ref] = coordinates.activation
		plane.applicationAt[ref] = member.Application
		plane.triggers = append(plane.triggers, equation.ActivationTriggerBinding{
			TriggerOrdinal: int(uint64(ref)) - 1, Family: shape.ActivationFamily, Application: member.Application,
		})
	}
	return plane, topologyConstructionRefusal{}
}

// memberCoordinates is the resolved address of one member: the identity it
// publishes under, the activation identity its trigger publishes under, and
// the coordinates its Group inputs are built from.
type memberCoordinates struct {
	member     identity.ContentID
	activation identity.ContentID
	mount      identity.ContentID
	inputSite  equation.Site
	inputPoint identity.ContentID
	routed     bool
	route      rows.ArtifactScalarEdge
}

// resolveMemberCoordinates decides admission of one declared member against
// the sealed rows of its plane. A mounted member is admissible exactly when
// its template carries the role+mount+point+occurrence rule row and the sites
// that row's points were admitted under; a Link member exactly when the
// bootstrap witness assigned its role to that occurrence.
func resolveMemberCoordinates(declaration topologyDeclaration, mounts constructedMountPlane, member declaredMemberRow, ordinal int) (memberCoordinates, topologyConstructionRefusal) {
	switch member.Plane {
	case declaredMemberOwner:
		if declaration.mounted() || !member.ID.Available() {
			return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
		}
		return memberCoordinates{member: member.ID, activation: member.ActivationID}, topologyConstructionRefusal{}
	case declaredMemberMount:
		if !declaration.mounted() || !member.Role.mounted() || !member.Mount.Available() || !member.Point.Available() || !member.Occurrence.Available() {
			return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
		}
		handle, admitted := mounts.rules[artifactMountedRule{role: member.Role, mount: member.Mount, point: member.Point, occurrence: member.Occurrence}]
		if !admitted {
			return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
		}
		mount := mounts.mounts[handle.mount]
		rule, ruleOK := mount.template.RuleAt(handle.rule)
		_, sited := declaration.sites.mounted[artifactMountedPoint{mount: member.Mount, reusable: member.Point}]
		if !ruleOK || !sited {
			return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
		}
		coordinates := memberCoordinates{
			member:     mountedRuleMemberID(member.Role, member.Mount, member.Point, member.Occurrence),
			activation: mountedRuleActivationID(member.Role, member.Mount, member.Point, member.Occurrence),
			mount:      member.Mount,
			inputPoint: rule.Input,
		}
		if rule.Input.Available() {
			site, inputSited := declaration.sites.mounted[artifactMountedPoint{mount: member.Mount, reusable: rule.Input}]
			if !inputSited {
				return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
			}
			coordinates.inputSite = site
		}
		if rule.Route.Available() {
			edgeIndex, routed := mounts.routes[constructedRouteKey{mount: handle.mount, route: rule.Route}]
			edge, edgeOK := mount.template.EdgeAt(edgeIndex)
			if !routed || !edgeOK || edge.To != rule.Input {
				return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
			}
			coordinates.routed, coordinates.route = true, edge
		}
		if !coordinates.member.Available() || !coordinates.activation.Available() {
			return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
		}
		return coordinates, topologyConstructionRefusal{}
	case declaredMemberLink:
		if !declaration.mounted() || !member.Role.link() || !member.Occurrence.Available() {
			return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
		}
		assigned, found := mounts.bootstrap.capabilityFor(member.Occurrence)
		if !found || assigned != member.Role {
			return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
		}
		id := linkRuleMemberID(member.Role, mounts.owner, mounts.point.PointID, member.Occurrence)
		if !id.Available() {
			return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
		}
		return memberCoordinates{member: id}, topologyConstructionRefusal{}
	}
	return memberCoordinates{}, refuseAdmission(topologyConstructionStepMemberIssuance, ordinal)
}

// constructMemberGroup builds the one Group a member row folds into. Input
// count is the schema's, not the declaration's: a shape with inputs resolves
// them from the member's coordinates or refuses.
func constructMemberGroup(declaration topologyDeclaration, source constructedSourcePlane, points constructedPointPlane, member declaredMemberRow, coordinates memberCoordinates, row equation.RuleInstance, ref equation.RuleRef, ordinal int) (equation.Group, topologyConstructionRefusal) {
	pointID, located := points.idBySite[row.Occurrence.Site()]
	output, outputOK := points.refByID[pointID]
	ruleOrdinal, ruleOK := source.schema.ruleOrdinalOf(row.Schema)
	shape, shapeOK := source.schema.ruleShapeAt(ruleOrdinal)
	if !located || !outputOK || !ruleOK || !shapeOK {
		return equation.Group{}, refuseAdmission(topologyConstructionStepMemberGroup, ordinal)
	}
	inputs := make([]equation.Input, 0, shape.Inputs)
	if shape.Inputs != 0 {
		target := row.Occurrence.Site()
		inputSite := coordinates.inputSite
		inputPoint := coordinates.inputPoint
		if member.Plane == declaredMemberLink {
			// A Link rule has no mounted predecessor row. Its predecessor is the
			// exact owner-issued bootstrap witness, and its input is therefore a
			// bootstrap-to-bootstrap boundary. Re-read the witness here rather
			// than trusting a coordinate supplied by the declaration pass.
			bootstrap, bootstrapOK := declaration.bootstrap.Point()
			assigned, assignedOK := declaration.bootstrap.capabilityFor(member.Occurrence)
			inputSite = declaration.sites.bootstrap
			inputPoint = bootstrap.PointID
			if !bootstrapOK || !assignedOK || assigned != member.Role || !member.Role.link() ||
				!inputSite.Available() || !target.Available() || !target.Same(inputSite) {
				return equation.Group{}, refuseAdmission(topologyConstructionStepMemberGroup, ordinal)
			}
		} else if !inputPoint.Available() || !inputSite.Available() {
			return equation.Group{}, refuseAdmission(topologyConstructionStepMemberGroup, ordinal)
		}
		if member.Plane == declaredMemberMount && coordinates.routed {
			// A routed member's predecessor proof is the parent's route row:
			// the member's input must be the exact Point that route lands on,
			// under a proof the artifact declaration already sealed.
			landing, landed := points.idByMounted[artifactMountedPoint{mount: coordinates.mount, reusable: coordinates.route.To}]
			input, inputLocated := points.idByMounted[artifactMountedPoint{mount: coordinates.mount, reusable: coordinates.inputPoint}]
			if !landed || !inputLocated || landing != input || !rows.ValidArtifactScalarEdgeProof(coordinates.route) {
				return equation.Group{}, refuseAdmission(topologyConstructionStepMemberGroup, ordinal)
			}
		}
		for slot := uint64(0); slot < shape.Inputs; slot++ {
			var provenance composition.Key
			var provenanceOK bool
			if member.Plane == declaredMemberLink {
				provenance, provenanceOK = linkRuleInputKey(member.Role, declaration.bootstrap.OwnerID(), inputPoint, member.Occurrence, slot)
			} else {
				provenance, provenanceOK = mountedRuleInputKey(coordinates.member, inputPoint, slot)
			}
			reindex, reindexOK := ruleInputReindex(inputSite.Scope(), target.Scope())
			boundary := equation.BoundaryInput(inputSite, target, provenance, equation.TrueExpr(), reindex, equation.TrueExpr())
			if !provenanceOK || !reindexOK || !boundary.Available() {
				return equation.Group{}, refuseAdmission(topologyConstructionStepMemberGroup, ordinal)
			}
			inputs = append(inputs, boundary)
		}
	}
	group := equation.Group{Members: []equation.RuleRef{ref}, Output: output, Inputs: inputs, EnvironmentInput: member.EnvironmentInput}
	if uint64(len(inputs)) != shape.Inputs || !validBindingGroup(source.batch, group) {
		return equation.Group{}, refuseAdmission(topologyConstructionStepMemberGroup, ordinal)
	}
	return group, topologyConstructionRefusal{}
}

// memberActivationShape resolves the composition shape of one trigger row.
func memberActivationShape(source constructedSourcePlane, row equation.RuleInstance) (composition.RuleShape, bool) {
	ordinal, found := source.schema.ruleOrdinalOf(row.Schema)
	if !found {
		return composition.RuleShape{}, false
	}
	return source.schema.ruleShapeAt(ordinal)
}

// constructQueryPlane derives the query geometry in declaration order. The
// ordinal a query publishes under is its position in this plane, so the
// declared order is the published order.
func constructQueryPlane(declaration topologyDeclaration, source constructedSourcePlane, points constructedPointPlane) (constructedQueryPlane, topologyConstructionRefusal) {
	plane := constructedQueryPlane{
		specs:       make([]equation.QueryInstance, 0, len(declaration.queries)),
		ordinalByID: make(map[identity.ContentID]uint64, len(declaration.queries)),
	}
	for ordinal, query := range declaration.queries {
		row := query.Row
		if declaration.mounted() {
			point, pointOK := constructedMountedPoint(points, query.Mount, query.Point)
			if !pointOK {
				return constructedQueryPlane{}, refuseAdmission(topologyConstructionStepQueryRow, ordinal)
			}
			row.Point = point
		} else if query.Mount.Available() || query.Point.Available() {
			return constructedQueryPlane{}, refuseAdmission(topologyConstructionStepQueryRow, ordinal)
		}
		family := row.Family
		familyOrdinal, familyOK := source.schema.queryOrdinalOf(family)
		if !query.ID.Available() || !family.Available() || !familyOK || !bindingOwnsQuerySchema(source.schema, family) ||
			familyOrdinal >= source.schema.queryCount() || !validBindingQueryInstance(source.schema, familyOrdinal, row) ||
			duplicateBindingQuery(plane.specs, row) {
			return constructedQueryPlane{}, refuseAdmission(topologyConstructionStepQueryRow, ordinal)
		}
		if _, duplicate := plane.ordinalByID[query.ID]; duplicate {
			return constructedQueryPlane{}, refuseAdmission(topologyConstructionStepDuplicateIdentity, ordinal)
		}
		row.Surfaces = append([]equation.Surface(nil), query.Row.Surfaces...)
		plane.ordinalByID[query.ID] = uint64(len(plane.specs))
		plane.specs = append(plane.specs, row)
	}
	return plane, topologyConstructionRefusal{}
}

// constructedMountedPoint resolves one mounted point coordinate to its dense
// address in this construction's point plane.
func constructedMountedPoint(points constructedPointPlane, mount, reusable identity.ContentID) (equation.PointRef, bool) {
	id, located := points.idByMounted[artifactMountedPoint{mount: mount, reusable: reusable}]
	ref, refOK := points.refByID[id]
	return ref, located && refOK && ref != 0
}

// constructActivationPlane derives the disposable activation-row geometry.
// Every row names its trigger by ordinal; the trigger must already publish a
// stable activation identity, and every row of one trigger must name the same
// application. A registered trigger may intentionally have no row: the
// trigger binding is the completeness authority for that empty set.
func constructActivationPlane(declaration topologyDeclaration, source constructedSourcePlane, mounts constructedMountPlane, points constructedPointPlane, members constructedMemberPlane) (constructedActivationPlane, topologyConstructionRefusal) {
	declared, refusal := constructDeclaredCandidates(declaration, mounts, points, members)
	if refusal.Available() {
		return constructedActivationPlane{}, refusal
	}
	// This is an admission-local duplicate fence only. Equation.Topology owns
	// the sealed row directory after this phase has finished.
	type activationTuple struct {
		trigger             equation.RuleRef
		application, target composition.Key
		endpoint            composition.Key
	}
	tupleAt := make(map[activationTuple]struct{}, len(declared))
	plane := constructedActivationPlane{rows: make([]equation.ActivationRowSpec, 0, len(declared))}
	for ordinal, row := range declared {
		if row.TriggerOrdinal < 0 || row.TriggerOrdinal >= len(members.specs) {
			return constructedActivationPlane{}, refuseAdmission(topologyConstructionStepCandidateRow, ordinal)
		}
		ref := equation.RuleAt(row.TriggerOrdinal)
		if _, registered := members.activationAt[ref]; !registered {
			return constructedActivationPlane{}, refuseAdmission(topologyConstructionStepCandidateRow, ordinal)
		}
		shape, shapeOK := memberActivationShape(source, members.specs[row.TriggerOrdinal])
		application, applicationOK := members.applicationAt[ref]
		if !shapeOK || shape.ActivationCount != 1 || shape.ActivationFamily != row.Family || !applicationOK || application != row.Application ||
			!row.Family.Available() || !row.Application.Available() || !row.Target.Available() || !row.Endpoint.Available() || row.Target == row.Endpoint {
			return constructedActivationPlane{}, refuseAdmission(topologyConstructionStepCandidateRow, ordinal)
		}
		tuple := activationTuple{trigger: ref, application: row.Application, target: row.Target, endpoint: row.Endpoint}
		if _, duplicate := tupleAt[tuple]; duplicate {
			return constructedActivationPlane{}, refuseAdmission(topologyConstructionStepCandidateRow, ordinal)
		}
		tupleAt[tuple] = struct{}{}
		row.Imports = append([]composition.Key(nil), row.Imports...)
		row.Entries = append([]equation.PointRef(nil), row.Entries...)
		row.Exits = append([]equation.PointRef(nil), row.Exits...)
		plane.rows = append(plane.rows, row)
	}
	return plane, topologyConstructionRefusal{}
}

// constructDeclaredCandidates folds the mounted candidate coordinates into
// activation-row recipes. The trigger Point and the body's entry/exit Points
// are resolved against this construction's own planes; the declaration
// carries no dense address.
func constructDeclaredCandidates(declaration topologyDeclaration, mounts constructedMountPlane, points constructedPointPlane, members constructedMemberPlane) ([]equation.ActivationRowSpec, topologyConstructionRefusal) {
	if len(declaration.candidates) == 0 {
		return nil, topologyConstructionRefusal{}
	}
	if !declaration.mounted() {
		return nil, refuseAdmission(topologyConstructionStepCandidateRow, 0)
	}
	built := make([]equation.ActivationRowSpec, 0, len(declaration.candidates))
	transports := make(map[artifactMountedBody]constructedActivationTransport, len(declaration.candidates))
	for ordinal, candidate := range declaration.candidates {
		ref, registered := members.refByID[candidate.Member]
		activation, isTrigger := members.activationAt[ref]
		trigger, triggerOK := constructedMountedPoint(points, candidate.Trigger.mount, candidate.Trigger.reusable)
		if !registered || !isTrigger || !activation.Available() || !triggerOK {
			return nil, refuseAdmission(topologyConstructionStepCandidateRow, ordinal)
		}
		transport, resolved := transports[artifactMountedBody{mount: candidate.Mount, body: candidate.Body}]
		if !resolved {
			var setOK bool
			transport.entries, transport.exits, setOK = constructBodyTransportRows(mounts, points, candidate)
			if !setOK {
				return nil, refuseAdmission(topologyConstructionStepCandidateRow, ordinal)
			}
			transports[artifactMountedBody{mount: candidate.Mount, body: candidate.Body}] = transport
		}
		built = append(built, equation.ActivationRowSpec{
			TriggerOrdinal: int(uint64(ref)) - 1,
			Family:         candidate.Family,
			Application:    candidate.Application,
			Target:         candidate.Target,
			Endpoint:       candidate.Endpoint,
			Trigger:        trigger,
			Entries:        append([]equation.PointRef(nil), transport.entries...),
			Exits:          append([]equation.PointRef(nil), transport.exits...),
			Imports:        append([]composition.Key(nil), candidate.Imports...),
			Export:         candidate.Export,
		})
	}
	return built, topologyConstructionRefusal{}
}

type constructedActivationTransport struct {
	entries []equation.PointRef
	exits   []equation.PointRef
}

// constructBodyTransportRows resolves one mounted body's entry and exit
// Points from the sealed template. The equation sealer later pairs these rows
// with the declared transport vector and assigns immutable transport-row IDs.
func constructBodyTransportRows(mounts constructedMountPlane, points constructedPointPlane, candidate declaredActivationCandidate) ([]equation.PointRef, []equation.PointRef, bool) {
	handle, mounted := mounts.bodies[artifactMountedBody{mount: candidate.Mount, body: candidate.Body}]
	if !mounted || handle.mount < 0 || handle.mount >= len(mounts.mounts) {
		return nil, nil, false
	}
	template := mounts.mounts[handle.mount].template
	body, bodyOK := template.BodyAt(handle.body)
	if !bodyOK || body.ID != candidate.Body || len(body.Entry) == 0 || len(body.Exits) == 0 {
		return nil, nil, false
	}
	entries := make([]equation.PointRef, 0, len(body.Entry))
	for _, reusable := range body.Entry {
		ref, ok := constructedMountedPoint(points, candidate.Mount, reusable)
		if !ok {
			return nil, nil, false
		}
		entries = append(entries, ref)
	}
	exits := make([]equation.PointRef, 0, len(body.Exits))
	for _, reusable := range body.Exits {
		ref, ok := constructedMountedPoint(points, candidate.Mount, reusable)
		if !ok {
			return nil, nil, false
		}
		exits = append(exits, ref)
	}
	return entries, exits, true
}

// constructSemanticRows folds the four published address planes into the one
// identity set a program publishes. An identity carried by two planes is
// refused here: a published ContentID addresses exactly one row.
func constructSemanticRows(points constructedPointPlane, members constructedMemberPlane, queries constructedQueryPlane) (*bindingSemanticRows, topologyConstructionRefusal) {
	total := len(points.refByID) + len(members.refByID) + len(queries.ordinalByID) + len(members.activations)
	claimed := make(map[identity.ContentID]bindingSemanticRowKind, total)
	claim := func(id identity.ContentID, kind bindingSemanticRowKind) bool {
		if !id.Available() {
			return false
		}
		if _, duplicate := claimed[id]; duplicate {
			return false
		}
		claimed[id] = kind
		return true
	}
	for id := range points.refByID {
		if !claim(id, bindingSemanticPoint) {
			return nil, refuseAdmission(topologyConstructionStepDuplicateIdentity, len(claimed))
		}
	}
	for id := range members.refByID {
		if !claim(id, bindingSemanticMember) {
			return nil, refuseAdmission(topologyConstructionStepDuplicateIdentity, len(claimed))
		}
	}
	for id := range queries.ordinalByID {
		if !claim(id, bindingSemanticQuery) {
			return nil, refuseAdmission(topologyConstructionStepDuplicateIdentity, len(claimed))
		}
	}
	for id := range members.activations {
		if !claim(id, bindingSemanticActivation) {
			return nil, refuseAdmission(topologyConstructionStepDuplicateIdentity, len(claimed))
		}
	}
	if len(claimed) != total {
		return nil, refuseAdmission(topologyConstructionStepDuplicateIdentity, len(claimed))
	}
	return &bindingSemanticRows{
		points:      points.refByID,
		members:     members.refByID,
		queries:     queries.ordinalByID,
		activations: members.activations,
	}, topologyConstructionRefusal{}
}

// sealConstructedTopology commits the derived planes: it seals the equation
// Topology, resolves the initial Graph, runs the composition schedule gate
// against the parent WTO certificate, and publishes the semantic directory.
func sealConstructedTopology(declaration topologyDeclaration, source constructedSourcePlane, mounts constructedMountPlane, points constructedPointPlane, edges constructedEdgePlane, members constructedMemberPlane, queries constructedQueryPlane, activations constructedActivationPlane, semantic *bindingSemanticRows) (constructedTopology, topologyConstructionRefusal) {
	spec := equation.TopologySpec{
		Batch:              source.batch,
		ActivationRows:     activations.rows,
		ActivationTriggers: members.triggers,
		Rules:              members.specs,
		Points:             points.specs,
		PointRanks:         points.ranks,
		Groups:             members.groups,
		Queries:            queries.specs,
		EnvironmentEdges:   edges.environment,
		FactorEdges:        edges.factor,
		Summaries:          declaration.summaries,
	}
	var topology *equation.Topology
	var sealed bool
	if declaration.observationQueries {
		topology, _, sealed = equation.SealObservationTopologyWithFailure(source.schema.cold, spec)
	} else {
		topology, _, sealed = equation.SealTopologyWithFailure(source.schema.cold, spec)
	}
	if !sealed || topology == nil || !topology.OwnsComposition(source.schema.cold) {
		return constructedTopology{}, refuseTopologySeal(topologyConstructionStepTopologySeal, 0)
	}
	relation, relationOK := topology.InitialRelation()
	if !relationOK {
		return constructedTopology{}, refuseTopologySeal(topologyConstructionStepGraph, 0)
	}
	graph, graphOK := topology.Graph(relation)
	if !graphOK || graph == nil || !topology.OwnsGraph(graph) || !graph.OwnsComposition(source.schema.cold) {
		return constructedTopology{}, refuseTopologySeal(topologyConstructionStepGraph, 0)
	}
	if declaration.mounted() {
		if ordinal, scheduled := constructedScheduleValid(source, mounts, points, topology, graph); !scheduled {
			return constructedTopology{}, refuseTopologySeal(topologyConstructionStepSchedule, ordinal)
		}
	}
	directory, directoryOK := sealSemanticDirectory(topology, source.state, source.authority, semantic)
	if !directoryOK {
		return constructedTopology{}, refuseTopologySeal(topologyConstructionStepDirectory, 0)
	}
	stages, stagesOK := constructedNativeStageDirectory(mounts, directory)
	if !stagesOK {
		return constructedTopology{}, refuseTopologySeal(topologyConstructionStepDirectory, 0)
	}
	committed := &CommittedProgram{
		graph: graph, topology: topology, state: source.state, authority: source.authority,
		directory: directory, nativeCallStages: stages,
		members: members.bindings, queries: declaredQueryBindings(declaration.queries),
		artifactBacked: declaration.mounted(),
	}
	committed.self = committed
	if declaration.mounted() {
		committed.bootstrapOwner = mounts.owner
		committed.bootstrapPoint = mounts.point.PointID
		committed.bootstrapSemantic = points.bootstrapSemantic
	}
	addressed, addressedOK := committed.publishedQueryKeys()
	if !committed.valid() || !addressedOK {
		return constructedTopology{}, refuseTopologySeal(topologyConstructionStepDirectory, 0)
	}
	committed.addressed = addressed
	return constructedTopology{program: committed}, topologyConstructionRefusal{}
}

// constructedNativeStageDirectory retains the compact native-stage inverse a
// committed program answers MountedNativeCallStage from. Every entry must
// already address an attached member under its exact
// role+mount+point+occurrence identity.
func constructedNativeStageDirectory(mounts constructedMountPlane, directory *semanticDirectory) (map[artifactMountedRuleOccurrence]artifactNativeCallStage, bool) {
	if len(mounts.stages) == 0 {
		return nil, true
	}
	if directory == nil {
		return nil, false
	}
	result := make(map[artifactMountedRuleOccurrence]artifactNativeCallStage, len(mounts.stages))
	for key, stage := range mounts.stages {
		member := mountedRuleMemberID(key.role, key.mount, stage.point, key.occurrence)
		if _, attached := directory.member(member); !member.Available() || !attached {
			return nil, false
		}
		result[key] = stage
	}
	return result, true
}

// constructedScheduleValid is the composition gate between the parent WTO
// certificate and the composed equation graph. Program order is a stable rank
// input to the one equation scheduler, not a second final-schedule authority:
// Link-local factor and activation edges may legally constrain that order or
// merge structural regions. Publication therefore proves exact point coverage,
// native-stage ordering, and monotonic cycle preservation.
func constructedScheduleValid(source constructedSourcePlane, mounts constructedMountPlane, points constructedPointPlane, topology *equation.Topology, graph *equation.Graph) (int, bool) {
	compiled := graph.Schedule()
	if compiled == nil || graph.PointCount() != len(points.specs) {
		return 0, false
	}
	idByKey := make(map[composition.Key]identity.ContentID, len(points.specs))
	for id, ref := range points.refByID {
		locator, locatorOK := topology.PointRow(ref)
		point, pointOK := locator.Resolve(graph)
		if !locatorOK || !pointOK || !point.Available() {
			return 0, false
		}
		if _, duplicate := idByKey[point.Key()]; duplicate {
			return 0, false
		}
		idByKey[point.Key()] = id
	}
	rank := make(map[identity.ContentID]int, len(points.specs))
	for index := 0; index < compiled.EventCount(); index++ {
		event, eventOK := compiled.EventAt(index)
		if !eventOK {
			return index, false
		}
		if event.Kind != schedule.EventNode {
			continue
		}
		point, pointOK := graph.PointAt(event.Node)
		id, located := idByKey[point.Key()]
		_, duplicate := rank[id]
		if !pointOK || !located || duplicate {
			return index, false
		}
		rank[id] = len(rank)
	}
	if len(rank) != len(points.specs) {
		return len(rank), false
	}
	// Native Call ownership comes only from the sealed rule placements. All
	// roles sharing a native stage must attest the same exact input, and a
	// stage point must follow the point its input is staged from.
	// The scanned placements live in unordered maps, so an offending scan
	// reports the minimum rank over its whole offending set rather than
	// whichever entry a map walk reaches first. Ranks are injective over
	// mounted points, so the minimum is a total choice.
	offendingRank, offended := 0, false
	observe := func(rank int) {
		if !offended || rank < offendingRank {
			offendingRank, offended = rank, true
		}
	}
	result := func() (int, bool) {
		if !offended {
			return 0, false
		}
		return offendingRank, true
	}
	stageBase := make(map[identity.ContentID]identity.ContentID, len(mounts.stages))
	stageKind := make(map[identity.ContentID]rows.ArtifactRuleStage, len(mounts.stages))
	for _, stage := range mounts.stages {
		baseRank, baseOK := rank[stage.mountedInput]
		stageRank, stageOK := rank[stage.mountedPoint]
		if !stage.native || !baseOK || !stageOK || baseRank >= stageRank {
			observe(stageRank)
		}
	}
	if ordinal, refused := result(); refused {
		return ordinal, false
	}
	for _, stage := range mounts.stages {
		if prior, duplicate := stageBase[stage.mountedPoint]; duplicate && prior != stage.mountedInput {
			observe(rank[stage.mountedPoint])
		}
		if prior, duplicate := stageKind[stage.mountedPoint]; duplicate && prior != stage.stage {
			observe(rank[stage.mountedPoint])
		}
		stageBase[stage.mountedPoint], stageKind[stage.mountedPoint] = stage.mountedInput, stage.stage
	}
	if ordinal, refused := result(); refused {
		return ordinal, false
	}
	localStages := make(map[identity.ContentID]map[composition.Key]struct{})
	for key, handle := range mounts.rules {
		rule, ruleOK := mounts.mounts[handle.mount].template.RuleAt(handle.rule)
		native, nativeOK := mounts.mounts[handle.mount].template.RuleNativeAt(handle.rule)
		staged, located := points.idByMounted[artifactMountedPoint{mount: key.mount, reusable: key.point}]
		if !ruleOK || !nativeOK || !located {
			return 0, false
		}
		switch {
		case rule.Stage == rows.ArtifactRuleStageBase:
		case rule.Stage == rows.ArtifactRuleStageLocal:
			factor, factorOK := ruleOutputFactor(key.role)
			if _, native := stageKind[staged]; native || !factorOK {
				observe(rank[staged])
				continue
			}
			written := localStages[staged]
			if written == nil {
				written = make(map[composition.Key]struct{})
				localStages[staged] = written
			}
			written[factor] = struct{}{}
		case native:
			if owner, native := stageKind[staged]; !native || owner != rule.Stage {
				observe(rank[staged])
			}
		default:
			observe(rank[staged])
		}
	}
	if ordinal, refused := result(); refused {
		return ordinal, false
	}
	localOwners := make(map[identity.ContentID]identity.ContentID, len(localStages))
	for mountIndex, mount := range mounts.mounts {
		template := mount.template
		for transferIndex := 0; transferIndex < template.TransferCount(); transferIndex++ {
			transfer, transferOK := template.TransferAt(transferIndex)
			base, baseLocated := points.idByMounted[artifactMountedPoint{mount: mount.module, reusable: transfer.From}]
			staged, stagedLocated := points.idByMounted[artifactMountedPoint{mount: mount.module, reusable: transfer.To}]
			baseRank, baseOK := rank[base]
			stageRank, stageOK := rank[staged]
			if !transferOK || !baseLocated || !stagedLocated || !baseOK || !stageOK || baseRank >= stageRank {
				return transferIndex, false
			}
			if _, native := stageKind[staged]; native {
				continue
			}
			written, local := localStages[staged]
			if !local {
				continue
			}
			if _, duplicate := localOwners[staged]; duplicate {
				return transferIndex, false
			}
			if !transfer.Full && !constructedCompleteTransfer(source, mounts, mountIndex, transfer, written) {
				return transferIndex, false
			}
			localOwners[staged], stageBase[staged] = base, base
		}
	}
	if len(localOwners) != len(localStages) {
		return len(localOwners), false
	}
	// Added edges may merge parent regions, but can never split or erase a
	// Program-issued cyclic region.
	graphRegions := make([]map[identity.ContentID]struct{}, compiled.RegionCount())
	for index := range graphRegions {
		view, viewOK := graph.RegionAt(index)
		if !viewOK || view.PointCount() == 0 {
			return index, false
		}
		members := make(map[identity.ContentID]struct{}, view.PointCount())
		for memberIndex := 0; memberIndex < view.PointCount(); memberIndex++ {
			point, pointOK := view.PointAt(memberIndex)
			id, located := idByKey[point.Key()]
			if !pointOK || !located {
				return index, false
			}
			members[id] = struct{}{}
		}
		graphRegions[index] = members
	}
	for _, mount := range mounts.mounts {
		template := mount.template
		for regionIndex := 0; regionIndex < template.RegionCount(); regionIndex++ {
			region, regionOK := template.RegionAt(regionIndex)
			if !regionOK {
				return regionIndex, false
			}
			if !region.Cyclic {
				continue
			}
			if !constructedRegionContained(points, stageBase, graphRegions, mount.module, region) {
				return regionIndex, false
			}
		}
	}
	return 0, true
}

// constructedRegionContained proves one parent cyclic region survives in the
// composed graph. A member staged behind a native or local stage is followed
// back to the point that stage was raised from before containment is read.
func constructedRegionContained(points constructedPointPlane, stageBase map[identity.ContentID]identity.ContentID, graphRegions []map[identity.ContentID]struct{}, mount identity.ContentID, region rows.ArtifactScalarRegion) bool {
	for _, candidate := range graphRegions {
		contained := true
		for _, reusable := range region.Members {
			member, located := points.idByMounted[artifactMountedPoint{mount: mount, reusable: reusable}]
			if !located {
				return false
			}
			seen := make(map[identity.ContentID]struct{}, 4)
			for {
				base, staged := stageBase[member]
				if !staged {
					break
				}
				if _, cycle := seen[member]; cycle {
					return false
				}
				seen[member] = struct{}{}
				member = base
			}
			if _, exists := candidate[member]; !exists {
				contained = false
				break
			}
		}
		if contained {
			return true
		}
	}
	return false
}

// constructedCompleteTransfer verifies the explicit schema-derived complement
// for a strong-write stage. The engine validates the closed factor inventory;
// it does not infer which domain facts should cross the boundary.
func constructedCompleteTransfer(source constructedSourcePlane, mounts constructedMountPlane, mountIndex int, transfer rows.ArtifactScalarTransfer, written map[composition.Key]struct{}) bool {
	if transfer.Full || len(written) == 0 || len(transfer.Factors) == 0 {
		return false
	}
	seen := make(map[composition.Key]struct{}, len(transfer.Factors)+len(written))
	for factor := range written {
		seen[factor] = struct{}{}
	}
	for _, role := range transfer.Factors {
		capability, capabilityOK := constructedRoleCapability(source, mounts.mounts[mountIndex], role)
		factor, factorOK := ruleOutputFactor(capability)
		_, duplicate := seen[factor]
		if !capabilityOK || !factorOK || duplicate {
			return false
		}
		seen[factor] = struct{}{}
	}
	return len(seen) == schemaFactorCount(source.schema)
}

// ruleOutputFactor resolves the Factor one mounted rule capability writes.
func ruleOutputFactor(role RuleSlotCapability) (composition.Key, bool) {
	if !role.mounted() || role.state == nil || role.state.schema == nil {
		return composition.Key{}, false
	}
	rule, roleOK := role.state.roleSlots[role]
	ordinal, ruleOK := role.state.schema.ruleOrdinalOf(rule)
	shape, shapeOK := role.state.schema.ruleShapeAt(ordinal)
	if !roleOK || !ruleOK || !shapeOK || shape.OutputKind != composition.FactorOutput || !shape.Output.Available() {
		return composition.Key{}, false
	}
	return shape.Output, true
}
