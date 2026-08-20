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
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
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
	// binder is the sealed cell that mints this issuance's runtime row, and
	// coords are the neutral coordinates used for its declaration. Both are
	// stated here and bound by the committed program, never by this pass.
	binder ProgramRule
	coords OperandCoords
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
	Mount       identity.ContentID
	Body        identity.ContentID
	Trigger     artifactMountedPoint
	Imports     []composition.Key
	Export      composition.Key
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
func declareMountedProgram(rowsWorkspace *programRows, mounts []sealedProgramMount, bootstrap LinkBootstrapWitness, admission MountedProgramAdmission) (topologyDeclaration, programSealFailure, ProgramAdmissionStage, bool) {
	if rowsWorkspace == nil || rowsWorkspace.binding == nil || rowsWorkspace.mountedRows == nil || rowsWorkspace.state == nil || rowsWorkspace.state.schema == nil {
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
	claimedLink := make(map[identity.ContentID]RuleSlotCapability, len(admission.Link))
	for _, row := range admission.Link {
		issuance, ok := admitLinkRuleIssuance(rowsWorkspace, rows, state, row, claimedLink)
		if !ok || !claimAnchoredSurfaces(anchored, issuance.surfaces) {
			return topologyDeclaration{}, programSealFailure{}, ProgramAdmissionLink, false
		}
		pending = append(pending, issuance)
	}
	// Admissibility is decided once, by the declared issuance requirement the
	// artifact placed the row under. A placement therefore guarantees the
	// owner seals an operand for it, and an owner that cannot refuses the
	// whole assemble rather than being silently skipped here.
	for _, row := range admission.Mounted {
		issuance, ok := admitMountedRuleIssuance(rowsWorkspace, rows, state, row)
		if !ok || !claimAnchoredSurfaces(anchored, issuance.surfaces) {
			return topologyDeclaration{}, programSealFailure{}, ProgramAdmissionMounted, false
		}
		pending = append(pending, issuance)
	}
	for _, row := range admission.Activation {
		issuance, admitted, ok := admitActivationRuleIssuance(rowsWorkspace, rows, state, row)
		if !ok || !claimAnchoredSurfaces(anchored, issuance.surfaces) {
			return topologyDeclaration{}, programSealFailure{}, ProgramAdmissionMounted, false
		}
		if !admitted {
			continue
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
		bootstrap: bootstrap,
		sites:     constructedSitePlane{mounted: rows.mounted, bootstrap: rows.bootstrap.site},
	}
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
		query, summaries, ok := row.admit.declareMountedQuery(state, authority, row.ID, row.Mount, row.Point)
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

// artifactRowFailure closes one artifact-row boundary a declaration could not
// be addressed through.
func artifactRowFailure(failure programArtifactRowFailure, ordinal uint32) programSealFailure {
	return programSealFailure{phase: programSealFailureArtifactRows, ordinal: ordinal, artifact: failure}
}

func mountedIssuanceRole(issuance pendingRuleIssuance) RuleSlotCapability {
	if issuance.plane == declaredMemberMount {
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
	if !row.Declaration.Available() || !row.Capability.mounted() || !row.Mount.Available() || !row.Point.Available() || !row.Occurrence.Available() {
		return pendingRuleIssuance{}, false
	}
	if !rows.mountedRule(row.Capability, row.Mount, row.Point, row.Occurrence) {
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
		binder: row.Declaration, coords: coords,
	}
	return declareIssuanceSurfaces(rowsWorkspace, state, row.Declaration, coords, site, entity, issuance)
}

// admitLinkRuleIssuance mints one Link-global issuance from the sealed
// bootstrap catalog. It has no mount and cannot address an arbitrary site or
// occurrence: the witness assigned the role, and each occurrence is claimed
// once.
func admitLinkRuleIssuance(rowsWorkspace *programRows, rows *mountedArtifactRows, state *schemaBindingState, row LinkRuleAdmission, claimed map[identity.ContentID]RuleSlotCapability) (pendingRuleIssuance, bool) {
	if !row.Declaration.Available() || !row.Capability.link() || !row.Occurrence.Available() {
		return pendingRuleIssuance{}, false
	}
	assigned, found := rows.bootstrap.roles[row.Occurrence]
	if !found || assigned != row.Capability {
		return pendingRuleIssuance{}, false
	}
	if _, duplicate := claimed[row.Occurrence]; duplicate {
		return pendingRuleIssuance{}, false
	}
	claimed[row.Occurrence] = row.Capability
	entity, entityOK := linkRuleOccurrenceKey(row.Capability, row.Occurrence)
	member := linkRuleMemberID(row.Capability, rows.bootstrap.owner, rows.bootstrap.point.PointID, row.Occurrence)
	if !entityOK || !member.Available() {
		return pendingRuleIssuance{}, false
	}
	issuance := pendingRuleIssuance{
		plane: declaredMemberLink, role: row.Capability,
		occurrence: row.Occurrence, member: member,
		binder: row.Declaration, coords: OperandCoords{Occurrence: row.Occurrence},
	}
	// The bootstrap Site is deliberately admitted into the still-open source
	// Batch. Site.Available requires a sealed Batch and therefore cannot be
	// used at this pre-seal boundary; admitFrom authenticates the open-batch
	// capability and preserves the same fence as mounted rows.
	return declareIssuanceSurfaces(rowsWorkspace, state, row.Declaration, OperandCoords{Occurrence: row.Occurrence}, rows.bootstrap.site, entity, issuance)
}

// declareIssuanceSurfaces is the shared half of every rule issuance: mint the
// Occurrence and Operand, then read the declaration's two pure projections
// against that anchor.
func declareIssuanceSurfaces(rowsWorkspace *programRows, state *schemaBindingState, declaration ProgramRule, coords OperandCoords, site equation.Site, entity composition.Key, issuance pendingRuleIssuance) (pendingRuleIssuance, bool) {
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

// admitActivationRuleIssuance mints one mounted activation issuance. An
// occurrence whose owner bound no transport vector declares no trigger row,
// which is the lawful no-op for an activation with nothing to instantiate.
func admitActivationRuleIssuance(rowsWorkspace *programRows, rows *mountedArtifactRows, state *schemaBindingState, admit MountedActivationAdmit) (pendingRuleIssuance, bool, bool) {
	if admit.Implementation == nil || !admit.Capability.mounted() ||
		!admit.Mount.Available() || !admit.Point.Available() || !admit.Occurrence.Available() {
		return pendingRuleIssuance{}, false, false
	}
	if admit.Transport == nil {
		return pendingRuleIssuance{}, false, len(admit.Candidates) == 0
	}
	if !admit.Application.Available() {
		return pendingRuleIssuance{}, false, false
	}
	if !rows.mountedRule(admit.Capability, admit.Mount, admit.Point, admit.Occurrence) {
		return pendingRuleIssuance{}, false, false
	}
	site, sited := rows.mountedSite(admit.Mount, admit.Point)
	entity, entityOK := mountedRuleOccurrenceKey(admit.Capability, admit.Occurrence)
	member := mountedRuleMemberID(admit.Capability, admit.Mount, admit.Point, admit.Occurrence)
	activation := mountedRuleActivationID(admit.Capability, admit.Mount, admit.Point, admit.Occurrence)
	semantic, family, semanticOK := admit.Implementation.declaredRuleSchema()
	if !sited || !entityOK || !member.Available() || !activation.Available() || !semanticOK ||
		!declaredRoleOwnsRuleSchema(state, admit.Capability, semantic) ||
		!declaredActivationTransport(state, admit.Transport, admit.Capability, semantic) {
		return pendingRuleIssuance{}, false, false
	}
	anchor, anchorOK := admitRuleSurfaceAnchor(rowsWorkspace, site, entity, [32]byte(admit.Occurrence))
	if !anchorOK {
		return pendingRuleIssuance{}, false, false
	}
	program, programOK := SealActivationProgramRule(admit.Implementation)
	if !programOK {
		return pendingRuleIssuance{}, false, false
	}
	return pendingRuleIssuance{
		plane: declaredMemberMount, role: admit.Capability,
		mount: admit.Mount, point: admit.Point, occurrence: admit.Occurrence,
		member: member, activationID: activation, activation: true,
		binder:   program,
		semantic: semantic, family: family, anchor: anchor,
		surfaces:    declaredActivationSurfaces(admit.Read),
		candidates:  admit.Candidates,
		application: admit.Application,
		issuer:      admit.Transport,
	}, true, true
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
		issuer.rule != semantic || !issuer.family.Available() || len(issuer.imports) == 0 || !issuer.export.Available() ||
		!role.activation {
		return false
	}
	for _, factor := range issuer.imports {
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
		declared = append(declared, declaredActivationCandidate{
			Member: issuance.member, Family: shape.ActivationFamily, Application: application,
			Target: compositionKeyOf(candidate.Target), Endpoint: compositionKeyOf(candidate.Endpoint),
			Mount: candidate.Mount, Body: candidate.Body,
			Trigger: artifactMountedPoint{mount: issuance.mount, reusable: issuance.point},
			Imports: issuance.issuer.imports, Export: issuance.issuer.export,
		})
	}
	return declared, true
}

// appendDeclaredSummary folds one declared summary surface into the program's
// summary plane. A surface declared twice must carry the same key vector.
func appendDeclaredSummary(summaries []equation.SummaryMapping, mapping *ruleSummaryMapping, state *schemaBindingState, authority *schemaBindingAuthority) ([]equation.SummaryMapping, bool) {
	if mapping == nil || mapping.state == nil || mapping.authority == nil || len(mapping.keys) == 0 ||
		!validateSummarySurface(mapping, state, authority) {
		return nil, false
	}
	for _, existing := range summaries {
		if existing.Surface != mapping.surface {
			continue
		}
		if len(existing.Keys) != len(mapping.keys) {
			return nil, false
		}
		for index := range existing.Keys {
			if existing.Keys[index] != mapping.keys[index] {
				return nil, false
			}
		}
		return summaries, true
	}
	return append(summaries, equation.SummaryMapping{Surface: mapping.surface, Keys: append([]uint64(nil), mapping.keys...)}), true
}
