// runtime_program_issue.go is the declared issuance pass. It reads the sealed
// admission inventory a Link publishes - one row per rule, activation and
// query issuance - mints the source capabilities those rows are addressed
// under, asks each declaration for its pure projections, and states the
// result as the topology declaration the pure constructor folds.
//
// Nothing here is a transaction handed to a domain. A declaration receives an
// anchor and returns values; the Batch, the row order and every refusal stay
// inside this pass. The geometry those rows fold into is derived by
// constructTopology alone.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
)

// pendingRuleIssuance is one issuance between source admission and row
// resolution. Its anchor is minted while the Batch is open; its equation row
// is resolved once the Batch seals, in the order the inventory declared.
type pendingRuleIssuance struct {
	plane        declaredMemberPlane
	role         RuleSlotCapability
	mount        identity.ContentID
	point        identity.ContentID
	occurrence   identity.ContentID
	member       identity.ContentID
	activationID identity.ContentID
	activation   bool
	semantic     composition.Key
	family       composition.Key
	anchor       ruleSurfaceAnchor
	surfaces     declaredRuleSurfaces
	candidates   []MountedActivationCandidate
	application  identity.SemanticKey
	issuer       *MountedActivationCandidateIssuer
	// binder is the canonical sealed schema cell that mints this issuance's
	// runtime row, and coords are the neutral coordinates used for its
	// declaration. Both are stated here and bound by the committed program,
	// never by this pass.
	binder sealedRuleCell
	coords OperandCoords
	// generated is the separate Plan-generated construction arm. It is never
	// passed through binder.bindMember and retains only compact candidate and
	// Factor-issued surfaces.
	generated *generatedMemberDeclaration
	// operand is the single canonical owner-issued value for this issuance.
	// The binder receives it directly and never resolves or canonicalizes it
	// again.
	operand declaredRuleOperand
}

// declaredActivationCandidate is one body route a mounted activation trigger
// must instantiate, stated as coordinates. The transport vector, the trigger
// Point and the body's entry/exit Points are resolved against the constructed
// point plane; no dense address is carried here.
type declaredActivationCandidate struct {
	Member      identity.ContentID
	Family      composition.Key
	Application composition.Key
	Target      composition.Key
	Endpoint    composition.Key
	Context     equation.ActivationContext
	Mount       identity.ContentID
	Body        identity.ContentID
	Trigger     artifactMountedPoint
	Imports     []composition.Key
	Exports     []composition.Key
}

// declaredRoleOwnsRuleSchema is the sealed role fence one issuance is admitted
// under: the Link's Binding must have bound this exact role to this exact rule
// semantic.
func declaredRoleOwnsRuleSchema(state *schemaBindingState, role RuleSlotCapability, semantic composition.Key) bool {
	if state == nil || state.schema == nil || !semantic.Available() || !role.available() {
		return false
	}
	if _, ok := state.schema.ruleOrdinalOf(semantic); !ok {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.phase == schemaBindingSealed && state.roleSlots[role] == semantic
}

// declareMountedProgram states one sealed admission inventory as a topology
// declaration. Source capabilities are minted in inventory order - Link rows,
// mounted rows, activation rows - the Batch is sealed once, and every equation
// row is resolved against those sealed identities afterwards.
func declareMountedProgram(rowsWorkspace *programRows, mounts []sealedProgramMount, contexts executioncontext.Directory, bootstrap LinkBootstrapWitness, admission MountedProgramAdmission, pointTransitions []ProgramPointTransitionAdmission) (topologyDeclaration, programSealFailure, ProgramAdmissionStage, bool) {
	if rowsWorkspace == nil || rowsWorkspace.binding == nil || rowsWorkspace.mountedRows == nil || rowsWorkspace.state == nil || rowsWorkspace.state.schema == nil {
		return topologyDeclaration{}, artifactRowFailure(programArtifactRowFailureOwner, 0), ProgramAdmissionNone, false
	}
	if !contexts.Available() || !bootstrap.Available() || contexts.LinkID() != bootstrap.OwnerID() {
		return topologyDeclaration{}, artifactRowFailure(programArtifactRowFailureOwner, 0), ProgramAdmissionNone, false
	}
	rows := rowsWorkspace.mountedRows
	state := rowsWorkspace.state
	authority := rowsWorkspace.authority
	if rows.bootstrap == nil {
		return topologyDeclaration{}, artifactRowFailure(programArtifactRowFailureBootstrap, 0), ProgramAdmissionNone, false
	}

	pending := make([]pendingRuleIssuance, 0, len(admission.Link)+len(admission.Mounted)+len(admission.Activation))
	anchored := make(map[equation.Surface]struct{})
	claimedLink := make(map[linkBootstrapClaim]struct{}, len(admission.Link))
	for ordinal, row := range admission.Link {
		issuance, ok := admitLinkRuleIssuance(rowsWorkspace, rows, state, row, claimedLink)
		if !ok || !claimAnchoredSurfaces(anchored, issuance.surfaces) {
			return topologyDeclaration{}, programSealFailure{phase: programSealFailureLinkIssuance, ordinal: uint32(ordinal), link: row.Capability}, ProgramAdmissionLink, false
		}
		pending = append(pending, issuance)
	}
	// Admissibility is decided once, by the declared issuance requirement the
	// artifact placed the row under. A placement therefore guarantees the
	// owner seals an operand for it, and an owner that cannot refuses the
	// whole assemble rather than being silently skipped here.
	for ordinal, row := range admission.Mounted {
		issuance, ok := admitMountedRuleIssuance(rowsWorkspace, rows, state, row)
		claimedOK := ok && claimAnchoredSurfaces(anchored, issuance.surfaces)
		if !claimedOK {
			return topologyDeclaration{}, programSealFailure{phase: programSealFailureMountedIssuance, ordinal: uint32(ordinal), mounted: row.Capability}, ProgramAdmissionMounted, false
		}
		pending = append(pending, issuance)
	}
	for ordinal, row := range admission.MountedPoint {
		issuances, ok := admitMountedPointRuleIssuances(rowsWorkspace, rows, state, mounts, row)
		if !ok {
			return topologyDeclaration{}, programSealFailure{phase: programSealFailureMountedIssuance, ordinal: uint32(ordinal), mounted: row.Capability}, ProgramAdmissionMounted, false
		}
		for _, issuance := range issuances {
			if !claimAnchoredSurfaces(anchored, issuance.surfaces) {
				return topologyDeclaration{}, programSealFailure{phase: programSealFailureMountedIssuance, ordinal: uint32(ordinal), mounted: row.Capability}, ProgramAdmissionMounted, false
			}
			pending = append(pending, issuance)
		}
	}
	for ordinal, row := range admission.Activation {
		issuance, ok := admitActivationRuleIssuance(rowsWorkspace, rows, state, row)
		if !ok || !claimAnchoredSurfaces(anchored, issuance.surfaces) {
			return topologyDeclaration{}, programSealFailure{phase: programSealFailureActivationIssuance, ordinal: uint32(ordinal), mounted: row.Capability}, ProgramAdmissionMounted, false
		}
		pending = append(pending, issuance)
	}

	if failure := rowsWorkspace.seal(); failure.Available() {
		return topologyDeclaration{}, programSealFailure{phase: programSealFailureSources, source: failure}, ProgramAdmissionSeal, false
	}

	declaration := topologyDeclaration{
		binding:   rowsWorkspace.binding,
		batch:     rowsWorkspace.batch,
		mounts:    mounts,
		contexts:  contexts,
		bootstrap: bootstrap,
		sites:     constructedSitePlane{mounted: rows.mounted, bootstrap: rows.bootstrap.site},
	}
	declaration.pointTransitions = append([]ProgramPointTransitionAdmission(nil), pointTransitions...)
	declaration.members = make([]declaredMemberRow, 0, len(pending))
	for ordinal, issuance := range pending {
		member, summaries, ok := resolveDeclaredMemberRow(state, authority, issuance, declaration.summaries)
		if !ok {
			return topologyDeclaration{}, programSealFailure{
				phase: programSealFailureRuleRow, ordinal: uint32(ordinal),
				mounted: mountedIssuanceRole(issuance), link: linkIssuanceRole(issuance),
			}, ProgramAdmissionSeal, false
		}
		declaration.summaries = summaries
		declaration.members = append(declaration.members, member)
		candidates, candidatesOK := declareActivationCandidates(state.schema, issuance)
		if !candidatesOK {
			return topologyDeclaration{}, programSealFailure{
				phase: programSealFailureRuleRow, ordinal: uint32(ordinal), mounted: issuance.role,
			}, ProgramAdmissionSeal, false
		}
		declaration.candidates = append(declaration.candidates, candidates...)
	}

	declaration.queries = make([]declaredQueryRow, 0, len(admission.Queries))
	for ordinal, row := range admission.Queries {
		if row.admit == nil || !rows.hasMountedSite(row.Mount, row.Point) {
			return topologyDeclaration{}, artifactRowFailure(programArtifactRowFailurePoint, uint32(ordinal)), ProgramAdmissionQuery, false
		}
		query, summaries, ok := row.admit.declareMountedQuery(state, authority, row.Context, row.ID, row.Mount, row.Point)
		if !ok {
			return topologyDeclaration{}, programSealFailure{phase: programSealFailureQueryBatch, ordinal: uint32(ordinal)}, ProgramAdmissionQuery, false
		}
		for _, summary := range summaries {
			var appended bool
			declaration.summaries, appended = appendDeclaredSummary(declaration.summaries, summary, state, authority)
			if !appended {
				return topologyDeclaration{}, programSealFailure{phase: programSealFailureQueryBatch, ordinal: uint32(ordinal)}, ProgramAdmissionQuery, false
			}
		}
		query.Admit = row.admit
		declaration.queries = append(declaration.queries, query)
	}
	return declaration, programSealFailure{}, ProgramAdmissionNone, true
}

// admitMountedPointRuleIssuances expands one closure occurrence over the
// stable mount/template Point order. No artifact RuleOccurrence row is
// consulted: the Point plane itself is this lane's complete denominator.
func admitMountedPointRuleIssuances(rowsWorkspace *programRows, rows *mountedArtifactRows, state *schemaBindingState, mounts []sealedProgramMount, row MountedPointRuleAdmission) ([]pendingRuleIssuance, bool) {
	if !row.Capability.mountedPoint() || !row.Occurrence.Available() {
		return nil, false
	}
	generated, generatedOK := resolveGeneratedRuleCell(row.Capability)
	if generatedOK {
		var result []pendingRuleIssuance
		for _, mount := range mounts {
			if mount.template == nil || !mount.module.Available() {
				return nil, false
			}
			for pointIndex := 0; pointIndex < mount.template.PointCount(); pointIndex++ {
				point, pointOK := mount.template.PointAt(pointIndex)
				site, siteOK := rows.mountedSite(mount.module, point.ID)
				entity, entityOK := mountedPointRuleOccurrenceKey(row.Capability, mount.module, point.ID, row.Occurrence)
				member := mountedPointRuleMemberID(row.Capability, mount.module, point.ID, row.Occurrence)
				if !pointOK || !siteOK || !entityOK || !member.Available() {
					return nil, false
				}
				coords := OperandCoords{Mount: mount.module, Point: point.ID, Occurrence: row.Occurrence}
				issuance := pendingRuleIssuance{plane: declaredMemberMountedPoint, role: row.Capability, mount: mount.module, point: point.ID, occurrence: row.Occurrence, member: member, coords: coords}
				declared, ok := declareGeneratedIssuanceSurfaces(rowsWorkspace, state, generated, coords, site, entity, issuance)
				if !ok {
					return nil, false
				}
				result = append(result, declared)
			}
		}
		return result, len(result) != 0
	}
	binder, binderOK := resolveOrdinaryRuleCell(row.Capability)
	if !binderOK {
		return nil, false
	}
	var result []pendingRuleIssuance
	for _, mount := range mounts {
		if mount.template == nil || !mount.module.Available() {
			return nil, false
		}
		for pointIndex := 0; pointIndex < mount.template.PointCount(); pointIndex++ {
			point, pointOK := mount.template.PointAt(pointIndex)
			site, sited := rows.mountedSite(mount.module, point.ID)
			entity, entityOK := mountedPointRuleOccurrenceKey(row.Capability, mount.module, point.ID, row.Occurrence)
			member := mountedPointRuleMemberID(row.Capability, mount.module, point.ID, row.Occurrence)
			if !pointOK || !sited || !entityOK || !member.Available() {
				return nil, false
			}
			coords := OperandCoords{Mount: mount.module, Point: point.ID, Occurrence: row.Occurrence}
			issuance := pendingRuleIssuance{
				plane: declaredMemberMountedPoint, role: row.Capability,
				mount: mount.module, point: point.ID, occurrence: row.Occurrence,
				member: member, binder: binder, coords: coords,
			}
			declared, ok := declareIssuanceSurfaces(rowsWorkspace, state, binder, coords, site, entity, issuance)
			if !ok {
				return nil, false
			}
			result = append(result, declared)
		}
	}
	return result, len(result) != 0
}

// artifactRowFailure closes one artifact-row boundary a declaration could not
// be addressed through.
func artifactRowFailure(failure programArtifactRowFailure, ordinal uint32) programSealFailure {
	return programSealFailure{phase: programSealFailureArtifactRows, ordinal: ordinal, artifact: failure}
}

func mountedIssuanceRole(issuance pendingRuleIssuance) RuleSlotCapability {
	if issuance.plane == declaredMemberMount || issuance.plane == declaredMemberMountedPoint {
		return issuance.role
	}
	return RuleSlotCapability{}
}

func linkIssuanceRole(issuance pendingRuleIssuance) RuleSlotCapability {
	if issuance.plane == declaredMemberLink {
		return issuance.role
	}
	return RuleSlotCapability{}
}

// claimAnchoredSurfaces records the occurrence-anchored coordinates one
// issuance declared. An anchored coordinate is derived from the issuance
// anchor, so two issuances reaching the same one is a construction fault.
func claimAnchoredSurfaces(claimed map[equation.Surface]struct{}, surfaces declaredRuleSurfaces) bool {
	for _, read := range surfaces.reads {
		if !read.anchored {
			continue
		}
		if _, duplicate := claimed[read.value]; duplicate {
			return false
		}
		claimed[read.value] = struct{}{}
	}
	for _, write := range surfaces.writes {
		if !write.anchored {
			continue
		}
		if _, duplicate := claimed[write.value]; duplicate {
			return false
		}
		claimed[write.value] = struct{}{}
	}
	return true
}

// admitMountedRuleIssuance mints one mounted issuance's source anchor and asks
// its declaration for the operand and surfaces. The member identity is
// mount+point+occurrence qualified, so equal reusable artifacts and same IDs
// on different mounts cannot alias.
func admitMountedRuleIssuance(rowsWorkspace *programRows, rows *mountedArtifactRows, state *schemaBindingState, row MountedRuleAdmission) (pendingRuleIssuance, bool) {
	if !row.Capability.mounted() || row.Capability.activation || !row.Mount.Available() || !row.Point.Available() || !row.Occurrence.Available() {
		return pendingRuleIssuance{}, false
	}
	generated, generatedOK := resolveGeneratedRuleCell(row.Capability)
	if generatedOK {
		if !rows.mountedRule(row.Capability, row.Mount, row.Point, row.Occurrence) {
			return pendingRuleIssuance{}, false
		}
		site, siteOK := rows.mountedSite(row.Mount, row.Point)
		entity, entityOK := mountedRuleOccurrenceKey(row.Capability, row.Occurrence)
		member := mountedRuleMemberID(row.Capability, row.Mount, row.Point, row.Occurrence)
		activation := mountedRuleActivationID(row.Capability, row.Mount, row.Point, row.Occurrence)
		if !siteOK || !entityOK || !member.Available() || !activation.Available() {
			return pendingRuleIssuance{}, false
		}
		coords := OperandCoords{Mount: row.Mount, Point: row.Point, Occurrence: row.Occurrence}
		issuance := pendingRuleIssuance{plane: declaredMemberMount, role: row.Capability, mount: row.Mount, point: row.Point, occurrence: row.Occurrence, member: member, activationID: activation, coords: coords}
		return declareGeneratedIssuanceSurfaces(rowsWorkspace, state, generated, coords, site, entity, issuance)
	}
	binder, binderOK := resolveOrdinaryRuleCell(row.Capability)
	if !binderOK {
		return pendingRuleIssuance{}, false
	}
	mountedRuleOK := rows.mountedRule(row.Capability, row.Mount, row.Point, row.Occurrence)
	if !mountedRuleOK {
		return pendingRuleIssuance{}, false
	}
	site, sited := rows.mountedSite(row.Mount, row.Point)
	entity, entityOK := mountedRuleOccurrenceKey(row.Capability, row.Occurrence)
	if !sited || !entityOK {
		return pendingRuleIssuance{}, false
	}
	member := mountedRuleMemberID(row.Capability, row.Mount, row.Point, row.Occurrence)
	activation := mountedRuleActivationID(row.Capability, row.Mount, row.Point, row.Occurrence)
	if !member.Available() || !activation.Available() {
		return pendingRuleIssuance{}, false
	}
	coords := OperandCoords{Mount: row.Mount, Point: row.Point, Occurrence: row.Occurrence}
	issuance := pendingRuleIssuance{
		plane: declaredMemberMount, role: row.Capability,
		mount: row.Mount, point: row.Point, occurrence: row.Occurrence,
		member: member, activationID: activation,
		binder: binder, coords: coords,
	}
	result, ok := declareIssuanceSurfaces(rowsWorkspace, state, binder, coords, site, entity, issuance)
	return result, ok
}

// admitLinkRuleIssuance mints one Link-global issuance from the sealed
// bootstrap catalog. It has no mount and cannot address an arbitrary site or
// occurrence: the witness admits only the exact capability+occurrence address,
// and each such address is claimed once.
func admitLinkRuleIssuance(rowsWorkspace *programRows, rows *mountedArtifactRows, state *schemaBindingState, row LinkRuleAdmission, claimed map[linkBootstrapClaim]struct{}) (pendingRuleIssuance, bool) {
	if !row.Capability.link() || !row.Occurrence.Available() {
		return pendingRuleIssuance{}, false
	}
	if rows.bootstrap == nil || !rows.bootstrap.witness.admits(row.Capability, row.Occurrence) {
		return pendingRuleIssuance{}, false
	}
	claim := linkBootstrapClaim{capability: row.Capability, occurrence: row.Occurrence}
	if _, duplicate := claimed[claim]; duplicate {
		return pendingRuleIssuance{}, false
	}
	claimed[claim] = struct{}{}
	entity, entityOK := linkRuleOccurrenceKey(row.Capability, row.Occurrence)
	member := linkRuleMemberID(row.Capability, rows.bootstrap.owner, rows.bootstrap.point.PointID, row.Occurrence)
	if !entityOK || !member.Available() {
		return pendingRuleIssuance{}, false
	}
	coords := OperandCoords{Occurrence: row.Occurrence}
	issuance := pendingRuleIssuance{
		plane: declaredMemberLink, role: row.Capability,
		occurrence: row.Occurrence, member: member, coords: coords,
	}
	// The bootstrap Site is deliberately admitted into the still-open source
	// Batch. Site.Available requires a sealed Batch and therefore cannot be
	// used at this pre-seal boundary; admitFrom authenticates the open-batch
	// capability and preserves the same fence as mounted rows.
	if generated, generatedOK := resolveGeneratedRuleCell(row.Capability); generatedOK {
		return declareGeneratedIssuanceSurfaces(rowsWorkspace, state, generated, coords, rows.bootstrap.site, entity, issuance)
	}
	binder, binderOK := resolveOrdinaryRuleCell(row.Capability)
	if !binderOK {
		return pendingRuleIssuance{}, false
	}
	issuance.binder = binder
	return declareIssuanceSurfaces(rowsWorkspace, state, binder, coords, rows.bootstrap.site, entity, issuance)
}

// declareIssuanceSurfaces is the shared half of every rule issuance: mint the
// Occurrence and Operand, then read the declaration's two pure projections
// against that anchor.
func declareIssuanceSurfaces(rowsWorkspace *programRows, state *schemaBindingState, declaration ordinarySealedRuleCell, coords OperandCoords, site equation.Site, entity composition.Key, issuance pendingRuleIssuance) (pendingRuleIssuance, bool) {
	semantic, family, semanticOK := declaration.declaredRuleSchema()
	if !semanticOK || !declaredRoleOwnsRuleSchema(state, issuance.role, semantic) {
		return pendingRuleIssuance{}, false
	}
	operand, operandOK := declaration.declareRuleOperand(coords)
	if !operandOK || !operand.Available() {
		return pendingRuleIssuance{}, false
	}
	anchor, anchorOK := admitRuleSurfaceAnchor(rowsWorkspace, site, entity, operand.digest)
	if !anchorOK {
		return pendingRuleIssuance{}, false
	}
	surfaces, surfacesOK := declaration.declareRuleSurfaces(operand, anchor)
	if !surfacesOK {
		return pendingRuleIssuance{}, false
	}
	issuance.semantic, issuance.family, issuance.anchor, issuance.surfaces, issuance.operand = semantic, family, anchor, surfaces, operand
	return issuance, true
}

// declareGeneratedIssuanceSurfaces is the generated construction arm. It
// resolves candidate/source/destination dense locals through the axis owners,
// then mints exact Factor surfaces from those locals. No operand provider,
// callback, type-erased value, or legacy binder participates.
func declareGeneratedIssuanceSurfaces(rowsWorkspace *programRows, state *schemaBindingState, declaration *generatedRuleBindingCell, coords OperandCoords, site equation.Site, entity composition.Key, issuance pendingRuleIssuance) (pendingRuleIssuance, bool) {
	// Site is intentionally still open here: admitRuleSurfaceAnchor owns the
	// Batch admission and authenticates the opaque Site against that exact open
	// Batch. Site.Available is a post-seal predicate, so requiring it here would
	// make the production construction phase impossible while admitting the
	// same capability through the ordinary arm.
	if rowsWorkspace == nil || state == nil || declaration == nil || !coords.Occurrence.Available() || !entity.Available() {
		return pendingRuleIssuance{}, false
	}
	// The mount is the issuance plane's own statement, not a free coordinate:
	// an artifact-addressed plane must carry one, and the Link plane is
	// mount-neutral by construction. The candidate relation named by the plan
	// decides whether it accepts that addressing.
	if coords.Mount.Available() != (issuance.plane != declaredMemberLink) {
		return pendingRuleIssuance{}, false
	}
	semantic, family, schemaOK := generatedRuleSchema(declaration)
	if !schemaOK || !declaredRoleOwnsRuleSchema(state, issuance.role, semantic) {
		return pendingRuleIssuance{}, false
	}
	cell := declaration.generatedCell()
	if cell == nil || !cell.available() || !cell.planDigest.Available() {
		return pendingRuleIssuance{}, false
	}
	descriptor := cell.program
	denseCandidate, programSource, candidateOK := declaredGeneratedCandidate(rowsWorkspace, state, descriptor, issuance.role, coords)
	if !candidateOK {
		return pendingRuleIssuance{}, false
	}
	// The anchor is admitted before any surface, because a selected read and a
	// routed write are ADDRESSED BY it: neither has a static coordinate, and
	// what identifies them instead is the occurrence and operand this issuance
	// mints here.
	anchor, anchorOK := admitRuleSurfaceAnchor(rowsWorkspace, site, entity, [32]byte(cell.planDigest))
	if !anchorOK {
		return pendingRuleIssuance{}, false
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(semantic)
	if !ruleSemanticOK {
		return pendingRuleIssuance{}, false
	}
	reads, readsOK := declareGeneratedReadSurfaces(state, declaration, descriptor, anchor, ruleSemantic, coords, denseCandidate)
	if !readsOK {
		return pendingRuleIssuance{}, false
	}
	// A structural rule publishes no Factor surface: its output is the
	// activation row set its candidate branches mount into the construct
	// topology, and the issuance arm that mounts them is the generated
	// activation lane. That lane is not built. The rule binds and installs its
	// family through the ordinary seam; this is where it stops, named by the
	// disposition rather than by a write surface that cannot be minted for a
	// rule that writes nothing.
	if declaration.writeMode == directRuleWriteStructural {
		return pendingRuleIssuance{}, false
	}
	writeSurface, writeSurfaceOK := declareGeneratedWriteSurface(state, declaration, descriptor, anchor, ruleSemantic, denseCandidate)
	if !writeSurfaceOK {
		return pendingRuleIssuance{}, false
	}
	surfaces := declaredRuleSurfaces{
		writes:  []ruleWriteSurface{writeSurface},
		carries: generatedCarryCount(descriptor),
	}
	if len(reads) != 0 {
		surfaces.reads = reads
	}
	issuance.semantic, issuance.family, issuance.anchor, issuance.surfaces = semantic, family, anchor, surfaces
	issuance.generated = &generatedMemberDeclaration{
		cell: cell, operand: anchor.operand, candidate: denseCandidate, source: programSource,
		reads: reads, writeSurface: writeSurface,
	}
	return issuance, true
}

// declaredGeneratedCandidate resolves one generated rule's dense candidate
// under whichever authority its plan named.
//
// An axis relation answers from its owner directory, keyed by the mount and
// occurrence this issuance is for. An issued Program row answers from the
// mounted placement itself: issuance resolved that ordinal while it held both
// the occurrence and the row space, so the engine transports the answer and
// resolves nothing. The Program state travels with it because equal ordinals
// from different mounted Programs are different rows.
func declaredGeneratedCandidate(rowsWorkspace *programRows, state *schemaBindingState, descriptor generated.CompiledRule, role RuleSlotCapability, coords OperandCoords) (uint32, execution.ProgramSource, bool) {
	if descriptor.IssuedCandidate() {
		// An issued candidate is a mounted row. The Link plane has no mount,
		// so a rule declaring one cannot be admitted there at all.
		if rowsWorkspace == nil || rowsWorkspace.mountedRows == nil || !coords.Mount.Available() {
			return 0, execution.ProgramSource{}, false
		}
		source, addressed := rowsWorkspace.mountedRows.mountedRuleSource(role, coords.Mount, coords.Point, coords.Occurrence)
		if !addressed || !source.present {
			return 0, execution.ProgramSource{}, false
		}
		capability, capabilityOK := execution.NewProgramSource(source.state, source.ordinal)
		if !capabilityOK {
			return 0, execution.ProgramSource{}, false
		}
		return source.ordinal, capability, true
	}
	candidate := descriptor.CandidateRelation()
	candidateOwner, candidateOwnerOK := relationOwnerForGeneratedAxis(state, candidate.Axis)
	if !candidateOwnerOK {
		return 0, execution.ProgramSource{}, false
	}
	dense, denseOK := soleDirectoryCandidate(candidateOwner, candidate.Member, coords)
	if !denseOK {
		return 0, execution.ProgramSource{}, false
	}
	return dense, execution.ProgramSource{}, true
}

// soleDirectoryCandidate resolves the one row a directory answers for this
// occurrence.
//
// The census is read before the row: a keyed generated rule instantiates one
// member per occurrence, so an occurrence whose relation publishes a candidate
// SET is not a single row and refuses here rather than silently taking a first
// candidate. An occurrence the directory answers nothing for refuses the same
// way - an absent row is not a zeroth row.
func soleDirectoryCandidate(owner memberrelation.Owner, relation uint32, coords OperandCoords) (uint32, bool) {
	if owner == nil {
		return 0, false
	}
	count, countOK := owner.CandidateCount(relation, coords.Mount, coords.Occurrence)
	if !countOK || count != 1 {
		return 0, false
	}
	return owner.CandidateAt(relation, coords.Mount, coords.Occurrence, 0)
}

// resolveGeneratedReadCandidate translates the rule's dense candidate into the
// ordinal of the directory THIS read is addressed by.
//
// A read that borrows the rule's own candidate directory is already resolved:
// the ordinal it needs is the one the rule resolved, and resolving it again
// would ask the same directory the same question. A read addressed by a
// corresponded foreign directory is not. A correspondence says the two orders
// enumerate the same subjects, never that they enumerate them in the same
// positions - each owner numbers its own rows independently - so the shared
// address is the OCCURRENCE both directories are addressed by, and the foreign
// row is resolved through it. Nothing here compares two dense ordinals, maps
// one to the other, or scans for a match.
func resolveGeneratedReadCandidate(state *schemaBindingState, candidate ruleplan.RelationAddr, plan generated.ReadPlan, coords OperandCoords, denseCandidate uint32) (uint32, bool) {
	if !plan.AddressingPresent || plan.Addressing == candidate {
		return denseCandidate, true
	}
	owner, ownerOK := relationOwnerForGeneratedAxis(state, plan.Addressing.Axis)
	if !ownerOK {
		return 0, false
	}
	return soleDirectoryCandidate(owner, plan.Addressing.Member, coords)
}

// generatedCarryCount is the declared carry cardinality of one generated rule.
// A generated rule carries at most its one output Factor.
func generatedCarryCount(descriptor generated.CompiledRule) uint64 {
	if descriptor.CarryInput() < 0 {
		return 0
	}
	return 1
}

// declareGeneratedReadSurfaces mints one surface per declared join, in plan
// order, each in the form its own row declares.
//
// An exact join resolves its coordinate now, through the owner of the relation
// the join names. A selected join resolves none: its coordinates are the
// members of a relation that exists only per invocation, so what is sealed
// here is the anchored identity of the read itself, and the members arrive
// through the family the rule's own package installs.
func declareGeneratedReadSurfaces(state *schemaBindingState, declaration *generatedRuleBindingCell, descriptor generated.CompiledRule, anchor ruleSurfaceAnchor, semantic identity.SemanticKey, coords OperandCoords, denseCandidate uint32) ([]RuleReadSurface, bool) {
	count := descriptor.ReadCount()
	if count == 0 {
		return nil, true
	}
	if len(declaration.reads) != count {
		return nil, false
	}
	candidate := descriptor.CandidateRelation()
	reads := make([]RuleReadSurface, count)
	for index := 0; index < count; index++ {
		plan, planOK := descriptor.ReadAt(index)
		row := declaration.reads[index]
		if !planOK || row == nil {
			return nil, false
		}
		if uint64(plan.Factor) >= uint64(len(state.factors)) {
			return nil, false
		}
		readFactor := state.factors[plan.Factor]
		if readFactor == nil || readFactor.schemaFactorSemanticKey() != row.factor {
			return nil, false
		}
		switch row.kind {
		case composition.ReadExact:
			joinOwner, joinOwnerOK := relationOwnerForGeneratedAxis(state, plan.Relation.Axis)
			if !joinOwnerOK {
				return nil, false
			}
			addressed, addressedOK := resolveGeneratedReadCandidate(state, candidate, plan, coords, denseCandidate)
			if !addressedOK {
				return nil, false
			}
			local, localOK := joinOwner.Project(plan.Relation.Member, plan.Key.Member, addressed)
			if !localOK {
				return nil, false
			}
			surface, surfaceOK := readFactor.schemaFactorExactRead(state, state.authority, uint64(local))
			if !surfaceOK || !surface.value.Available() || surface.value.Factor != row.factor {
				return nil, false
			}
			reads[index] = surface
		case composition.ReadSummary:
			addressed, addressedOK := resolveGeneratedReadCandidate(state, candidate, plan, coords, denseCandidate)
			if !addressedOK {
				return nil, false
			}
			keys, keysOK := generatedSummaryKeys(state, plan, addressed)
			if !keysOK {
				return nil, false
			}
			surface, surfaceOK := summaryReadSurface(state, state.authority, row, keys)
			if !surfaceOK || !surface.value.Available() || surface.value.Factor != row.factor {
				return nil, false
			}
			reads[index] = surface
		case composition.ReadSelect:
			dependencies := make([]RuleReadSurface, len(row.dependencies))
			for position, dependency := range row.dependencies {
				if dependency >= uint64(index) {
					return nil, false
				}
				dependencies[position] = reads[dependency]
			}
			surface, surfaceOK := anchoredSelectedReadSurface(state, state.authority, semantic, anchor, row, declaration.reads, dependencies, reads[:index])
			if !surfaceOK || !surface.value.Available() {
				return nil, false
			}
			reads[index] = surface
		default:
			return nil, false
		}
	}
	return reads, true
}

// generatedSummaryKeys is the owner-issued key vector a generated rule's
// vector read is delivered over.
//
// An authored rule takes this vector from its operand. A generated rule has no
// operand, so the vector comes from where the coordinates actually live: the
// relation the join names publishes the ordered member set one candidate row
// carries, and each member is projected through the join's own key projection.
// Nothing is derived here that the owner did not already publish.
//
// The order is the owner's and it must be strictly ascending, which the
// surface itself enforces: a summary read is a vector over a denominator, and
// two members answering one coordinate, or members out of order, would silently
// renumber every later cell.
func generatedSummaryKeys(state *schemaBindingState, plan generated.ReadPlan, addressedCandidate uint32) (summaryKeyVector, bool) {
	owner, ownerOK := relationOwnerForGeneratedAxis(state, plan.Relation.Axis)
	if !ownerOK {
		return summaryKeyVector{}, false
	}
	count, countOK := owner.MemberCount(plan.Relation.Member, addressedCandidate)
	if !countOK || count < 0 {
		return summaryKeyVector{}, false
	}
	keys := make([]uint64, 0, count)
	for index := 0; index < count; index++ {
		member, memberOK := owner.MemberAt(plan.Relation.Member, addressedCandidate, index)
		if !memberOK {
			return summaryKeyVector{}, false
		}
		local, localOK := owner.Project(plan.Relation.Member, plan.Key.Member, member)
		if !localOK {
			return summaryKeyVector{}, false
		}
		keys = append(keys, uint64(local))
	}
	return newSummaryKeyVector(keys), true
}

// declareGeneratedWriteSurface mints this rule's one publication surface.
//
// An exact publication projects its destination from the candidate row now.
// A routed publication projects nothing: its destinations are the members of
// the selected join it publishes over, and a member is a coordinate only once
// the relation is derived. What is sealed here is the anchored identity of the
// routed write, which is what makes the deferred destinations attributable to
// this exact issuance.
func declareGeneratedWriteSurface(state *schemaBindingState, declaration *generatedRuleBindingCell, descriptor generated.CompiledRule, anchor ruleSurfaceAnchor, semantic identity.SemanticKey, denseCandidate uint32) (ruleWriteSurface, bool) {
	outputFactorOrdinal := uint64(descriptor.OutputFactor())
	if outputFactorOrdinal >= uint64(len(state.factors)) {
		return ruleWriteSurface{}, false
	}
	outputFactor := state.factors[outputFactorOrdinal]
	if outputFactor == nil {
		return ruleWriteSurface{}, false
	}
	switch declaration.writeMode {
	case directRuleWriteExact:
		candidate := descriptor.CandidateRelation()
		destination := descriptor.DestinationProjection()
		candidateOwner, candidateOwnerOK := relationOwnerForGeneratedAxis(state, candidate.Axis)
		if !candidateOwnerOK {
			return ruleWriteSurface{}, false
		}
		destinationLocal, destinationOK := candidateOwner.Project(candidate.Member, destination.Member, denseCandidate)
		if !destinationOK {
			return ruleWriteSurface{}, false
		}
		surface, surfaceOK := outputFactor.schemaFactorExactWrite(state, state.authority, uint64(destinationLocal))
		if !surfaceOK || !surface.value.Available() || surface.value.Factor != outputFactor.schemaFactorSemanticKey() {
			return ruleWriteSurface{}, false
		}
		return surface, true
	case directRuleWriteRoute:
		route := declaration.routeRead
		if route == 0 || route > uint64(len(declaration.reads)) {
			return ruleWriteSurface{}, false
		}
		surface, surfaceOK := anchoredRouteWriteSurface(state, state.authority, semantic, anchor, declaration.ordinal, 0, route, outputFactor.schemaFactorSemanticKey(), declaration.reads[route-1])
		if !surfaceOK || !surface.value.Available() {
			return ruleWriteSurface{}, false
		}
		return surface, true
	default:
		return ruleWriteSurface{}, false
	}
}

// admitRuleSurfaceAnchor mints one issuance's Occurrence and Operand into the
// still-open source Batch.
func admitRuleSurfaceAnchor(rowsWorkspace *programRows, site equation.Site, entity composition.Key, digest [32]byte) (ruleSurfaceAnchor, bool) {
	occurrence, occurrenceOK := rowsWorkspace.admitFrom(site, entity)
	if !occurrenceOK {
		return ruleSurfaceAnchor{}, false
	}
	operandEntity, operandEntityOK := operandEntityForContent(digest)
	if !operandEntityOK {
		return ruleSurfaceAnchor{}, false
	}
	operand, operandOK := rowsWorkspace.admitOperand(occurrence, operandEntity)
	if !operandOK {
		return ruleSurfaceAnchor{}, false
	}
	return ruleSurfaceAnchor{occurrence: occurrence, operand: operand}, true
}

// admitActivationRuleIssuance mints one mounted activation issuance. The
// transport vector is what a candidate body route instantiates, so an
// admission that declares none is malformed rather than empty: the issuer
// refuses an empty vector at bind, and the equation sealer cannot represent a
// direct-transport row without one. An activation that instantiates nothing is
// the one that declares its vector and admits no candidate row, which stays
// admitted and addressable on its own trigger declaration.
func admitActivationRuleIssuance(rowsWorkspace *programRows, rows *mountedArtifactRows, state *schemaBindingState, admit MountedActivationAdmit) (pendingRuleIssuance, bool) {
	if !admit.Capability.mounted() || !admit.Capability.activation ||
		!admit.Mount.Available() || !admit.Point.Available() || !admit.Occurrence.Available() {
		return pendingRuleIssuance{}, false
	}
	binder, binderOK := resolveActivationRuleCell(admit.Capability)
	if !binderOK {
		return pendingRuleIssuance{}, false
	}
	if admit.Transport == nil {
		return pendingRuleIssuance{}, false
	}
	if !admit.Application.Available() {
		return pendingRuleIssuance{}, false
	}
	if !rows.mountedRule(admit.Capability, admit.Mount, admit.Point, admit.Occurrence) {
		return pendingRuleIssuance{}, false
	}
	site, sited := rows.mountedSite(admit.Mount, admit.Point)
	entity, entityOK := mountedRuleOccurrenceKey(admit.Capability, admit.Occurrence)
	member := mountedRuleMemberID(admit.Capability, admit.Mount, admit.Point, admit.Occurrence)
	activation := mountedRuleActivationID(admit.Capability, admit.Mount, admit.Point, admit.Occurrence)
	semantic, family, semanticOK := binder.declaredRuleSchema()
	if !sited || !entityOK || !member.Available() || !activation.Available() || !semanticOK ||
		!declaredRoleOwnsRuleSchema(state, admit.Capability, semantic) ||
		!declaredActivationTransport(state, admit.Transport, admit.Capability, semantic) {
		return pendingRuleIssuance{}, false
	}
	anchor, anchorOK := admitRuleSurfaceAnchor(rowsWorkspace, site, entity, [32]byte(admit.Occurrence))
	if !anchorOK {
		return pendingRuleIssuance{}, false
	}
	return pendingRuleIssuance{
		plane: declaredMemberMount, role: admit.Capability,
		mount: admit.Mount, point: admit.Point, occurrence: admit.Occurrence,
		member: member, activationID: activation, activation: true,
		binder:   binder,
		semantic: semantic, family: family, anchor: anchor,
		surfaces:    declaredActivationSurfaces(admit.Read),
		candidates:  admit.Candidates,
		application: admit.Application,
		issuer:      admit.Transport,
	}, true
}

// declaredActivationSurfaces states the read plane of one activation trigger.
// A shape that declares no read is admitted with none; the cold shape decides,
// so an inventory cannot add or drop a surface.
func declaredActivationSurfaces(read RuleReadSurface) declaredRuleSurfaces {
	if !read.value.Available() {
		return declaredRuleSurfaces{}
	}
	return declaredRuleSurfaces{reads: []RuleReadSurface{read}}
}

// declaredActivationTransport is the issuer fence: the bound transport vector
// must belong to this Binding and to the exact activation rule the role owns.
func declaredActivationTransport(state *schemaBindingState, issuer *MountedActivationCandidateIssuer, role RuleSlotCapability, semantic composition.Key) bool {
	if issuer == nil || issuer.state != state || state == nil || state.authority == nil ||
		issuer.rule != semantic || !issuer.family.Available() || len(issuer.imports) == 0 || len(issuer.exports) == 0 ||
		!role.activation {
		return false
	}
	for _, factor := range issuer.imports {
		if !factor.Available() {
			return false
		}
	}
	for _, factor := range issuer.exports {
		if !factor.Available() {
			return false
		}
	}
	return true
}

// resolveDeclaredMemberRow folds one pending issuance into its declared member
// row and appends the summary surfaces it declared.
func resolveDeclaredMemberRow(state *schemaBindingState, authority *schemaBindingAuthority, issuance pendingRuleIssuance, summaries []equation.SummaryMapping) (declaredMemberRow, []equation.SummaryMapping, bool) {
	row, rowOK := resolveDeclaredRuleInstance(state.schema, authority, issuance.semantic, issuance.family, issuance.anchor, issuance.surfaces)
	if !rowOK {
		return declaredMemberRow{}, nil, false
	}
	for _, read := range declaredSummaryMappings(issuance.surfaces) {
		appended, ok := appendDeclaredSummary(summaries, read.summary, state, authority)
		if !ok {
			return declaredMemberRow{}, nil, false
		}
		summaries = appended
	}
	member := declaredMemberRow{
		Plane: issuance.plane, ID: issuance.member, Role: issuance.role,
		Mount: issuance.mount, Point: issuance.point, Occurrence: issuance.occurrence,
		Row: row, Bind: issuance.binder, Coords: issuance.coords, Operand: issuance.operand,
		Generated: issuance.generated,
	}
	if issuance.generated != nil {
		member.GeneratedOperand = issuance.generated.operand
	}
	if issuance.activation {
		application := compositionKeyOf(issuance.application)
		if !application.Available() {
			return declaredMemberRow{}, nil, false
		}
		member.Activation, member.ActivationID, member.Application = true, issuance.activationID, application
	}
	return member, summaries, true
}

// declareActivationCandidates states one trigger's candidate set as
// coordinates. The transport vector comes from the bound issuer; the dense
// addresses are resolved by the constructor.
func declareActivationCandidates(schema *Schema, issuance pendingRuleIssuance) ([]declaredActivationCandidate, bool) {
	if !issuance.activation {
		return nil, true
	}
	ordinal, found := schema.ruleOrdinalOf(issuance.semantic)
	shape, shapeOK := schema.ruleShapeAt(ordinal)
	if !found || !shapeOK || shape.ActivationCount != 1 || shape.ActivationFamily != issuance.issuer.family {
		return nil, false
	}
	application := compositionKeyOf(issuance.application)
	declared := make([]declaredActivationCandidate, 0, len(issuance.candidates))
	for _, candidate := range issuance.candidates {
		if !candidate.Target.Available() || !candidate.Endpoint.Available() || candidate.Target == candidate.Endpoint ||
			!candidate.Mount.Available() || !candidate.Body.Available() || !application.Available() {
			return nil, false
		}
		context := equation.ActivationContext{
			TransitionID:  candidate.TransitionID,
			FromContextID: candidate.FromContextID,
			ToContextID:   candidate.ToContextID,
		}
		if !context.WellFormed() {
			return nil, false
		}
		declared = append(declared, declaredActivationCandidate{
			Member: issuance.member, Family: shape.ActivationFamily, Application: application,
			Target: compositionKeyOf(candidate.Target), Endpoint: compositionKeyOf(candidate.Endpoint),
			Context: context,
			Mount:   candidate.Mount, Body: candidate.Body,
			Trigger: artifactMountedPoint{mount: issuance.mount, reusable: issuance.point},
			Imports: issuance.issuer.imports, Exports: issuance.issuer.exports,
		})
	}
	return declared, true
}

// appendDeclaredSummary folds one declared summary surface into the program's
// summary plane. A surface declared twice must carry the same key vector.
func appendDeclaredSummary(summaries []equation.SummaryMapping, mapping *ruleSummaryMapping, state *schemaBindingState, authority *schemaBindingAuthority) ([]equation.SummaryMapping, bool) {
	if mapping == nil || mapping.state == nil || mapping.authority == nil ||
		!summaryKeysAllowed(state, mapping.factor, mapping.keys) ||
		!validateSummarySurface(mapping, state, authority) {
		return nil, false
	}
	for _, existing := range summaries {
		if existing.Surface != mapping.surface {
			continue
		}
		if !sameSummaryKeySource(existing.Keys, mapping.keys) {
			return nil, false
		}
		return summaries, true
	}
	keys, keysOK := materializeSummaryKeys(mapping.keys)
	if !keysOK {
		return nil, false
	}
	return append(summaries, equation.SummaryMapping{Surface: mapping.surface, Keys: keys}), true
}
