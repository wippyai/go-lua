package analysis

import (
	"crypto/sha256"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/domain/type/authority"
	"github.com/wippyai/go-lua/analysis/engine"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
)

// assembleReceiptGraph enters the sole receipt-native production seam. It
// returns the committed graph and the query plan so publication can finish
// without reopening owners or rescanning mounted rows.
type receiptAssemblyDiagnostic struct {
	stage      AnalyzeDiagnosticReceiptStage
	rule       AnalyzeDiagnosticRule
	seal       engine.ReceiptSealFailure
	ordinal    uint32
	lowering   engine.ReceiptAssemblyFailure
	binding    ProgramBindingFailure
	allocation allocationcatalog.SealFailure
	commit     engine.ReceiptCommitFailure
}

func (state *compiledState) assembleReceiptGraph() (*engine.ReceiptGraph, *artifactQueryPlan, receiptAssemblyDiagnostic, bool) {
	if state == nil || state.artifacts == nil || !state.receipt.Available() || state.binding == nil || state.binding.SchemaBinding() == nil || !state.binding.SchemaBinding().Sealed() {
		return nil, nil, receiptAssemblyDiagnostic{}, false
	}
	binding := state.binding
	valueIDs, heapIDs, witness, witnessOK := linkBootstrapWitness(state, binding)
	if !witnessOK {
		return nil, nil, receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageBinding}, false
	}
	mounts := make([]engine.MountedArtifactReceipt, 0, len(state.artifacts.mounts))
	receipts := make(map[identity.ContentID]*engine.ArtifactScalarReceipt, len(state.artifacts.mounts))
	for _, mount := range state.artifacts.mounts {
		if !mount.valid() {
			return nil, nil, receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageMount}, false
		}
		artifactID := mount.artifact.ID()
		receipt, receiptOK := receipts[artifactID]
		if !receiptOK {
			receipt, receiptOK = newEngineArtifactScalarReceipt(mount.template, mount.roles, binding)
			if !receiptOK {
				return nil, nil, receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageMount}, false
			}
			receipts[artifactID] = receipt
		}
		mounted, mountedOK := engine.NewMountedArtifactReceipt(receipt, mount.moduleKey)
		if !mountedOK {
			return nil, nil, receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageMount}, false
		}
		mounts = append(mounts, mounted)
	}
	var queryPlan *artifactQueryPlan
	var populated receiptAssemblyDiagnostic
	_, graph, lowering, commit, committed := engine.AssembleMountedArtifactReceipt(binding.SchemaBinding(), mounts, func(assembly *engine.ReceiptAssembly) bool {
		plan, queryOK := newArtifactQueryPlan(state.artifacts.mounts)
		if !queryOK {
			populated = receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageQueryPlan}
			return false
		}
		if !attachLinkBootstrapRules(binding, assembly, valueIDs, heapIDs) {
			populated = receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageBootstrapRules}
			return false
		}
		artifactRule, artifactRulesOK := attachArtifactRules(binding, assembly, state.artifacts.mounts)
		if !artifactRulesOK {
			populated = receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageArtifactRules, rule: artifactRule}
			return false
		}
		queryRowsRejected := false
		if !assembly.QueueMountedQueryBatch(func(batch *engine.MountedQueryBatch) bool {
			queryRowsRejected = !plan.AddRows(batch, binding)
			return !queryRowsRejected
		}) {
			populated = receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageQueryRows}
			return false
		}
		if !assembly.SealSources() {
			if queryRowsRejected {
				populated = receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageQueryRows}
				return false
			}
			failedRule := AnalyzeDiagnosticRuleUnknown
			failedStage := AnalyzeDiagnosticReceiptStageSourceSeal
			var failedSeal engine.ReceiptSealFailure
			var failedOrdinal uint32
			if failure, failureOK := assembly.SealFailure(); failureOK {
				failedSeal = failure
				if failure.Phase() == engine.ReceiptSealFailureArtifactRows {
					failedStage = AnalyzeDiagnosticReceiptStageArtifactRows
					failedOrdinal = failure.Ordinal()
				} else if role, roleOK := failure.MountedCapability(); roleOK {
					failedRule = diagnosticRuleForMountedRole(binding, role)
				} else if role, roleOK := failure.LinkCapability(); roleOK {
					failedRule = diagnosticRuleForLinkRole(binding, role)
				}
			}
			populated = receiptAssemblyDiagnostic{stage: failedStage, rule: failedRule, seal: failedSeal, ordinal: failedOrdinal}
			return false
		}
		queryPlan = plan
		return true
	}, witness)
	if !committed {
		if lowering != engine.ReceiptAssemblyFailureNone {
			return nil, nil, receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageLowering, lowering: lowering}, false
		}
		if populated.stage != AnalyzeDiagnosticReceiptStageNone {
			return nil, nil, populated, false
		}
		return nil, nil, receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageCommit, commit: commit}, false
	}
	return graph, queryPlan, receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageCommit}, true
}

type artifactScalarRoleBinding struct {
	program programartifact.RuleRole
	scalar  rows.ArtifactScalarRole
}

// artifactScalarRoleDirectory is immutable Program-template metadata. It
// contains no Link-local capability and is shared with the template cache.
type artifactScalarRoleDirectory struct{ rows []artifactScalarRoleBinding }

func (directory *artifactScalarRoleDirectory) role(programRole programartifact.RuleRole) (rows.ArtifactScalarRole, bool) {
	if directory != nil {
		for _, row := range directory.rows {
			if row.program == programRole {
				return row.scalar, row.scalar.Available()
			}
		}
	}
	return rows.ArtifactScalarRole{}, false
}

func artifactScalarRoleSemantic(artifact identity.ContentID, role programartifact.RuleRole) identity.ContentID {
	if !artifact.Available() || role == programartifact.RuleRoleInvalid {
		return identity.ContentID{}
	}
	input := make([]byte, 0, len("analysis/artifact-scalar-role/v1")+len(artifact)+1)
	input = append(input, "analysis/artifact-scalar-role/v1"...)
	input = append(input, artifact[:]...)
	input = append(input, byte(role))
	return identity.ContentID(sha256.Sum256(input))
}

// newEngineArtifactScalarTemplate is the sole Program→Engine structural
// boundary. It runs once while publishing the content-addressed cache entry.
func newEngineArtifactScalarTemplate(artifact *programartifact.Artifact) (*rows.ArtifactScalarTemplate, *artifactScalarRoleDirectory, bool) {
	if artifact == nil || !artifact.Available() {
		return nil, nil, false
	}
	structural, structuralOK := composite.StructureVocabulary()
	if !structuralOK {
		return nil, nil, false
	}
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !lowered {
		return nil, nil, false
	}
	usedRoles := make(map[programartifact.RuleRole]struct{})
	for index := 0; index < snapshot.LocalTransferCount(); index++ {
		row, ok := snapshot.LocalTransferAt(index)
		if !ok {
			return nil, nil, false
		}
		for inner := 0; inner < row.TagCount(); inner++ {
			tag, tagOK := row.TagAt(inner)
			if !tagOK {
				return nil, nil, false
			}
			usedRoles[programartifact.RuleRole(tag)] = struct{}{}
		}
	}
	for index := 0; index < snapshot.RulePlacementCount(); index++ {
		row, ok := snapshot.RulePlacementAt(index)
		if !ok {
			return nil, nil, false
		}
		usedRoles[programartifact.RuleRole(row.Tag())] = struct{}{}
	}
	spec, specOK := rows.NewArtifactScalarSpec(snapshot.ArtifactID(), snapshot.ProgramID(), snapshot.SchemaID(), rows.ArtifactScalarCapacity{
		Roles: len(usedRoles), Points: snapshot.PointCount(), Edges: snapshot.StructuralEdgeCount(), Transfers: snapshot.LocalTransferCount(), Regions: snapshot.RegionCount(), Events: snapshot.EventCount(), Rules: snapshot.RulePlacementCount(), Bodies: snapshot.BodyTransportCount(),
	})
	if !specOK {
		return nil, nil, false
	}
	directory := &artifactScalarRoleDirectory{rows: make([]artifactScalarRoleBinding, 0, len(usedRoles))}
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		role, roleOK := programartifact.MountedRuleRoleAt(index)
		_, used := usedRoles[role]
		if !roleOK {
			return nil, nil, false
		}
		if !used {
			continue
		}
		scalar, scalarOK := spec.DeclareRole(artifactScalarRoleSemantic(snapshot.ArtifactID(), role))
		if !scalarOK {
			return nil, nil, false
		}
		directory.rows = append(directory.rows, artifactScalarRoleBinding{program: role, scalar: scalar})
	}
	for index := 0; index < snapshot.PointCount(); index++ {
		row, ok := snapshot.PointAt(index)
		if !ok || !row.ID().Available() {
			return nil, nil, false
		}
		point, pointOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: row.ID(), Initial: row.Initial()})
		if !pointOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.DecisionCount(); inner++ {
			decision, decisionOK := row.DecisionAt(inner)
			if !decisionOK || !spec.AddPointDecision(point, decision) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < snapshot.StructuralEdgeCount(); index++ {
		row, ok := snapshot.StructuralEdgeAt(index)
		if !ok {
			return nil, nil, false
		}
		guard, guarded := row.GuardID()
		decision, decisionOK := row.DecisionID()
		truth, truthOK := row.Truth()
		mu, hasMu := row.MuPathID()
		reset, hasReset := row.ResetDigest()
		if guarded != decisionOK || guarded != truthOK || hasMu != hasReset {
			return nil, nil, false
		}
		arm, armOK := engineStructuralArm(row.Arm())
		if !armOK {
			return nil, nil, false
		}
		edge, edgeOK := spec.AddEdge(rows.ArtifactScalarEdge{ID: row.ID(), From: row.From(), To: row.To(), Route: row.RouteID(), Guard: guard, Decision: decision, Component: row.ComponentID(), Mu: mu, Reset: reset, Arm: arm, Guarded: guarded, Truth: truth, HasReset: hasReset})
		if !edgeOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.ResetCount(); inner++ {
			resetPoint, resetOK := row.ResetAt(inner)
			if !resetOK || !spec.AddEdgeReset(edge, resetPoint) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < snapshot.LocalTransferCount(); index++ {
		row, ok := snapshot.LocalTransferAt(index)
		if !ok {
			return nil, nil, false
		}
		transfer, transferOK := spec.AddTransfer(rows.ArtifactScalarTransfer{ID: row.ID(), From: row.From(), To: row.To(), Full: row.Full()})
		if !transferOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.TagCount(); inner++ {
			tag, tagOK := row.TagAt(inner)
			role, roleOK := directory.role(programartifact.RuleRole(tag))
			if !tagOK || !roleOK || !spec.AddTransferFactor(transfer, role) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < snapshot.RegionCount(); index++ {
		row, ok := snapshot.RegionAt(index)
		if !ok {
			return nil, nil, false
		}
		region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: row.ID(), Head: row.Head(), Parent: row.ParentID(), Cyclic: row.Cyclic()})
		if !regionOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.MemberCount(); inner++ {
			member, memberOK := row.MemberAt(inner)
			if !memberOK || !spec.AddRegionMember(region, member) {
				return nil, nil, false
			}
		}
	}
	for index := 0; index < snapshot.EventCount(); index++ {
		row, ok := snapshot.EventAt(index)
		if !ok {
			return nil, nil, false
		}
		kind, kindOK := engineEventKind(row.Kind())
		if !kindOK || !spec.AddEvent(rows.ArtifactScalarEvent{Kind: kind, Region: row.RegionID(), Point: row.PointID()}) {
			return nil, nil, false
		}
	}
	for index := 0; index < snapshot.RulePlacementCount(); index++ {
		row, ok := snapshot.RulePlacementAt(index)
		role := programartifact.RuleRole(row.Tag())
		scalarRole, scalarRoleOK := directory.role(role)
		stage, stageOK := engineArtifactRuleStage(role, programartifact.RuleStage(row.Stage()))
		if !ok || !scalarRoleOK || !stageOK || !spec.AddRule(rows.ArtifactScalarRule{Role: scalarRole, Stage: stage, Point: row.PointID(), Input: row.InputPointID(), ID: row.OccurrenceID(), Route: row.PredecessorRouteID()}) {
			return nil, nil, false
		}
	}
	for index := 0; index < snapshot.BodyTransportCount(); index++ {
		row, ok := snapshot.BodyTransportAt(index)
		if !ok {
			return nil, nil, false
		}
		body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: row.BodyID()})
		if !bodyOK {
			return nil, nil, false
		}
		for inner := 0; inner < row.EntryCount(); inner++ {
			point, pointOK := row.EntryAt(inner)
			if !pointOK || !spec.AddBodyEntry(body, point) {
				return nil, nil, false
			}
		}
		for inner := 0; inner < row.ExitCount(); inner++ {
			point, pointOK := row.ExitAt(inner)
			if !pointOK || !spec.AddBodyExit(body, point) {
				return nil, nil, false
			}
		}
	}
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	return template, directory, templateOK
}

// newEngineArtifactScalarReceipt binds only this Link's exact capabilities to
// the cached neutral Program template; it never reopens structural rows.
func newEngineArtifactScalarReceipt(template *rows.ArtifactScalarTemplate, roles *artifactScalarRoleDirectory, binding *composite.ProgramBinding) (*engine.ArtifactScalarReceipt, bool) {
	if template == nil || !template.Available() || roles == nil || binding == nil {
		return nil, false
	}
	substitution, substitutionOK := engine.NewArtifactScalarBinding(template)
	if !substitutionOK {
		return nil, false
	}
	for _, row := range roles.rows {
		capability, capabilityOK := mountedCapability(binding, row.program)
		if !capabilityOK || !substitution.BindRole(row.scalar, capability) {
			return nil, false
		}
	}
	return engine.NewArtifactScalarReceipt(substitution)
}

// engineArtifactRuleStage is the sole domain-role to engine-stage bridge.
// ProgramArtifact owns this closed pairing; a scalar caller cannot retag an
// Effect rule as a different native Call cut and ask engine to infer it from
// transport geometry.
func engineArtifactRuleStage(role programartifact.RuleRole, stage programartifact.RuleStage) (rows.ArtifactRuleStage, bool) {
	want := programartifact.RuleStageInvalid
	switch role {
	case programartifact.RuleRoleValueSource, programartifact.RuleRolePackSource, programartifact.RuleRoleHeapIngress:
		want = programartifact.RuleStageBase
	case programartifact.RuleRoleValueAllocation, programartifact.RuleRoleHeapEmpty, programartifact.RuleRoleHeapClosed,
		programartifact.RuleRoleRawGet, programartifact.RuleRoleRawSet, programartifact.RuleRoleValueStorageTransfer,
		programartifact.RuleRoleValueBinaryArithmetic, programartifact.RuleRoleValueBinaryEquality, programartifact.RuleRoleValueBinaryOrder, programartifact.RuleRoleValuePresenceRefinement:
		want = programartifact.RuleStageLocal
	case programartifact.RuleRoleCallDispatch:
		want = programartifact.RuleStageCallDispatch
	case programartifact.RuleRoleCallActivation:
		want = programartifact.RuleStageCallSummary
	case programartifact.RuleRoleEffectSelected, programartifact.RuleRoleEffectOpaque, programartifact.RuleRoleEffectBody:
		want = programartifact.RuleStageCallEffect
	default:
		return rows.ArtifactRuleStageInvalid, false
	}
	if stage != want {
		return rows.ArtifactRuleStageInvalid, false
	}
	switch stage {
	case programartifact.RuleStageBase:
		return rows.ArtifactRuleStageBase, true
	case programartifact.RuleStageLocal:
		return rows.ArtifactRuleStageLocal, true
	case programartifact.RuleStageCallDispatch:
		return rows.ArtifactRuleStageCallDispatch, true
	case programartifact.RuleStageCallSummary:
		return rows.ArtifactRuleStageCallSummary, true
	case programartifact.RuleStageCallEffect:
		return rows.ArtifactRuleStageCallEffect, true
	default:
		return rows.ArtifactRuleStageInvalid, false
	}
}

func engineStructuralArm(arm ingress.StructuralArm) (rows.ArtifactStructuralArm, bool) {
	switch arm {
	case ingress.StructuralArmLocal:
		return rows.ArtifactStructuralArmLocal, true
	case ingress.StructuralArmResume:
		return rows.ArtifactStructuralArmResume, true
	case ingress.StructuralArmTrue:
		return rows.ArtifactStructuralArmTrue, true
	case ingress.StructuralArmFalse:
		return rows.ArtifactStructuralArmFalse, true
	case ingress.StructuralArmTail:
		return rows.ArtifactStructuralArmTail, true
	case ingress.StructuralArmThrow:
		return rows.ArtifactStructuralArmThrow, true
	case ingress.StructuralArmYield:
		return rows.ArtifactStructuralArmYield, true
	case ingress.StructuralArmCancel:
		return rows.ArtifactStructuralArmCancel, true
	default:
		return rows.ArtifactStructuralArmInvalid, false
	}
}

func engineEventKind(kind ingress.EventKind) (rows.ArtifactEventKind, bool) {
	switch kind {
	case ingress.EventEnter:
		return rows.ArtifactEventEnter, true
	case ingress.EventPoint:
		return rows.ArtifactEventPoint, true
	case ingress.EventExit:
		return rows.ArtifactEventExit, true
	default:
		return rows.ArtifactEventInvalid, false
	}
}

// instantiateRuntimeTopology performs the sole transition from a cold Plan
// into instantiated Points and their immutable equation topology. It is
// runtime-owned, concurrency-safe, and cached for every later Solve: Link
// compilation never constructs Points, and repeated solves never rematerialize
// unchanged mounted Program interiors.
func (state *compiledState) instantiateRuntimeTopology() (receiptAssemblyDiagnostic, bool) {
	if state == nil {
		return receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageCommit}, false
	}
	state.runtimeOnce.Do(func() {
		state.runtimeDetail, state.runtimeOK = state.buildRuntimeTopologyWithDiagnostic()
	})
	return state.runtimeDetail, state.runtimeOK
}

func (state *compiledState) buildRuntimeTopologyWithDiagnostic() (receiptAssemblyDiagnostic, bool) {
	if state == nil || state.graph != nil {
		return receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageCommit}, state != nil && state.graph != nil
	}
	graph, queryPlan, diagnostic, ok := state.assembleReceiptGraph()
	if !ok || graph == nil {
		return diagnostic, false
	}
	state.graph = graph
	state.queryPlan = queryPlan
	// The graph's equation topology and semantic directory are now the sole
	// structural authority.  The copied artifact receipt was only needed while
	// lowering and validating mounted rows; release it before the Plan escapes
	// so it cannot overlap every solve's runtime allocation.
	if !graph.ReleaseArtifactReceipt() {
		state.graph = nil
		state.queryPlan = nil
		return receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageCommit}, false
	}
	return receiptAssemblyDiagnostic{stage: AnalyzeDiagnosticReceiptStageCommit}, true
}

// applyReceiptAssemblyDiagnostic preserves the permanent closed construction
// envelope when runtime topology instantiation fails. Moving Point ownership
// out of Compile must not weaken diagnostic evidence.
func applyReceiptAssemblyDiagnostic(diagnostics *AnalyzeDiagnostics, receipt receiptAssemblyDiagnostic) {
	if diagnostics == nil {
		return
	}
	diagnostics.ReceiptStage = receipt.stage
	diagnostics.Rule = receipt.rule
	diagnostics.ReceiptSeal = receipt.seal.Failure()
	diagnostics.ReceiptOrdinal = receipt.ordinal
	diagnostics.ReceiptLowering = receipt.lowering.Failure()
	diagnostics.Binding = receipt.binding
	if receipt.allocation != 0 {
		diagnostics.AllocationCatalog = receipt.allocation
	}
	diagnostics.ReceiptCommit = receipt.commit.Failure()
	diagnostics.ReceiptScheduleOrdinal, _ = receipt.commit.ScheduleOrdinal()
}

func linkBootstrapWitness(state *compiledState, binding *composite.ProgramBinding) ([]identity.ContentID, []identity.ContentID, engine.LinkBootstrapWitness, bool) {
	if state == nil || binding == nil || !state.sourceID.Available() {
		return nil, nil, engine.LinkBootstrapWitness{}, false
	}
	valueIDs, valueIDsOK := linkOccurrenceIDs(binding, programartifact.RuleRoleValueBootstrap)
	heapIDs, heapIDsOK := linkOccurrenceIDs(binding, programartifact.RuleRoleHeapBootstrap)
	if !valueIDsOK || !heapIDsOK {
		return nil, nil, engine.LinkBootstrapWitness{}, false
	}
	pointID, pointOK := identity.DeriveContentID("analysis/link-bootstrap-point/v1", state.sourceID[:])
	if !pointOK {
		return nil, nil, engine.LinkBootstrapWitness{}, false
	}
	valueCapability, valueCapabilityOK := linkCapability(binding, programartifact.RuleRoleValueBootstrap)
	heapCapability, heapCapabilityOK := linkCapability(binding, programartifact.RuleRoleHeapBootstrap)
	if !valueCapabilityOK || !heapCapabilityOK {
		return nil, nil, engine.LinkBootstrapWitness{}, false
	}
	witness, witnessOK := engine.NewLinkBootstrapWitnessByCapability(state.sourceID, engine.LinkBootstrapPoint{PointID: pointID, Known: true, Initial: true}, valueCapability, valueIDs, heapCapability, heapIDs)
	return valueIDs, heapIDs, witness, witnessOK
}

// newProgramBinding constructs the Link-local typed owners required by the
// receipt compiler. The reusable artifact remains the only source of
// structural rows; these domain schemas are solve-local substitutions.
func (state *compiledState) newProgramBinding(source *link.Link) (*composite.ProgramBinding, ProgramBindingFailure, composite.MountFailure, allocationcatalog.SealFailure) {
	if state == nil || source == nil || state.artifacts == nil || len(state.artifacts.mounts) == 0 {
		return nil, ProgramBindingFailureInput, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	// A Shard is a cold Project coordinate. It is reissued only while Link is
	// live, to authenticate this mount set against the Project; the published
	// artifact mount set has no Project type and cannot reopen a mounted
	// Program after Compile returns.
	if !projectAuthenticatesMounts(source, state.artifacts.mounts) {
		return nil, ProgramBindingFailureInput, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	// The sealed declaration table resolved every declared role once. The query
	// codecs below are read from that resolution rather than derived here, so a
	// published result carries the identity the schema was composed under.
	roles, rolesOK := composite.SemanticRoles()
	valueCodec, valueCodecOK := roles.Key("semantic/query-result/value-summary")
	effectCodec, effectCodecOK := roles.Key("semantic/query-result/effect-exact")
	if !rolesOK || !valueCodecOK || !effectCodecOK {
		return nil, ProgramBindingFailureSemantics, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	artifactTypes := make([]*programartifact.Artifact, 0, len(state.artifacts.byProgram))
	for _, artifact := range state.artifacts.byProgram {
		if artifact == nil || !artifact.Available() {
			return nil, ProgramBindingFailureTypes, composite.MountFailure{}, allocationcatalog.SealFailureNone
		}
		artifactTypes = append(artifactTypes, artifact)
	}
	types, typesErr := typeauthority.SealArtifactRows(state.sourceID, artifactTypes)
	if typesErr != nil {
		return nil, ProgramBindingFailureTypes, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	staticMounts := make([]staticdomain.MountedArtifact, len(state.artifacts.mounts))
	staticValueIDs := make([]staticdomain.MountedValueID, 0)
	staticValues := source.Boundary().Values()
	seenStaticValues := make(map[[2]identity.ContentID]struct{})
	for index, published := range state.artifacts.mounts {
		if published.artifact == nil || !published.artifact.Available() || !published.moduleKey.Available() || !published.programID.Available() {
			return nil, ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
		}
		// ModuleKey is the Link-owned, detached namespace identity for this
		// concrete mount.  The deleted LinkStatic relation used to rebuild the
		// same scope by reopening Program static/source terms.
		staticMounts[index] = staticdomain.MountedArtifact{Artifact: published.artifact, ModuleID: published.moduleKey, ProgramID: published.programID, NamespaceID: published.moduleKey}
		for rowIndex := 0; rowIndex < published.artifact.StaticTypeValueCount(); rowIndex++ {
			row, rowOK := published.artifact.StaticTypeValueAt(rowIndex)
			if !rowOK || !row.Available() {
				return nil, ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			key := [2]identity.ContentID{published.moduleKey, row.ID()}
			if _, duplicate := seenStaticValues[key]; duplicate {
				return nil, ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			value, valueOK := staticValues.ForMountedSemantic(published.moduleKey, row.ID())
			valueID, valueIDOK := staticValues.ID(value)
			if !valueOK || !valueIDOK || !valueID.Available() {
				return nil, ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			seenStaticValues[key] = struct{}{}
			staticValueIDs = append(staticValueIDs, staticdomain.MountedValueID{
				ModuleID: published.moduleKey, SemanticID: row.ID(), ValueID: valueID,
			})
		}
	}
	staticTarget, staticTargetOK := source.Boundary().Target()
	if !staticTargetOK {
		return nil, ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	static, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{
		LinkID:   state.sourceID,
		Target:   staticTarget,
		ValueIDs: staticValueIDs,
	}, types, staticMounts)
	if err != nil || static == nil {
		return nil, ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	// The neutral sealed artifact view is the mount phase's whole artifact
	// input. Every axis that owns its mount seals its own Link authority from
	// it and from the peers it declared an edge to, so no per-domain mount row
	// is constructed here.
	artifactRows, artifactRowsOK := linkArtifactRows(state.artifacts.mounts)
	if !artifactRowsOK {
		return nil, ProgramBindingFailureInput, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	inputs, mountFailure := composite.MountLink(composite.LinkInputs{
		Source:          source,
		Artifacts:       artifactRows,
		StaticAuthority: static,
	})
	// Topology and the activation catalog are derivations over several sealed
	// factors at once, so neither is any one axis's authority to mount. The mount
	// phase derives both itself, after every mount has sealed, and names the
	// derivation that refused in its own verdict.
	if mountFailure.Available() {
		return nil, programMountFailure(mountFailure), mountFailure, allocationcatalog.SealFailureNone
	}
	binding, failure := composite.BindProgram(state.receipt, inputs, composite.ProgramQuerySpecs{
		Value:  valueSummaryQueryHotSpec(inputs.ValueSchema, valueCodec),
		Effect: effectExactQueryHotSpec(inputs.EffectAlgebra, effectCodec),
	})
	if failure.Available() {
		return nil, programBindingFailure(failure), composite.MountFailure{}, failure.Allocation
	}
	// The receipt lowerer is issued per reusable Program artifact. The
	// Link-wide catalog above still authenticates every mount; the first
	// artifact is the current structural assembly unit and later units are
	// admitted by repeated solve-local receipt transactions.
	return binding, ProgramBindingFailureNone, composite.MountFailure{}, allocationcatalog.SealFailureNone
}

// linkArtifactRows projects the Link's private mount records onto the neutral
// artifact view the mount phase consumes. It is the sole conversion out of
// root-private mount state: a mounting domain reads the immutable artifact and
// the two Link-local identities that place it, and nothing else.
func linkArtifactRows(mounts []mountedProgramArtifact) ([]axis.MountedArtifact, bool) {
	if len(mounts) == 0 {
		return nil, false
	}
	rows := make([]axis.MountedArtifact, len(mounts))
	for index, mounted := range mounts {
		if !mounted.valid() {
			return nil, false
		}
		row := axis.MountedArtifact{Artifact: mounted.artifact, ModuleKey: mounted.moduleKey, ProgramID: mounted.programID}
		if !row.Available() {
			return nil, false
		}
		rows[index] = row
	}
	return rows, true
}

func artifactPlanOwnsBody(artifact *programartifact.Artifact, id identity.ContentID) bool {
	if artifact == nil || !artifact.Available() || !id.Available() {
		return false
	}
	for index := 0; index < artifact.BodyCount(); index++ {
		body, ok := artifact.BodyAt(index)
		if ok && body.ID() == id {
			return true
		}
	}
	return false
}

// mountedProgramArtifact is the immutable Program artifact plus the exact
// Link substitution needed to place that Program occurrence in a Link. The
// artifact itself is shared by ProgramID; the mount row is never shared.
type mountedProgramArtifact struct {
	artifact  *programartifact.Artifact
	template  *rows.ArtifactScalarTemplate
	roles     *artifactScalarRoleDirectory
	programID identity.ContentID
	moduleKey identity.ContentID

	// ruleMembers is populated once by attachArtifactRules and then read by
	// every solve. It is deliberately private and immutable after compilation.
	ruleMembers      []artifactRuleMemberRef
	ruleMembersReady bool
}

// projectAuthenticatesMounts states that this published mount set is exactly
// the live Project's mount set: same count, same order, and each row's Program
// and module identity reissued from the Project's own shard. It is the sole
// place a Shard is opened during construction, and no shard survives it.
func projectAuthenticatesMounts(source *link.Link, published []mountedProgramArtifact) bool {
	if source == nil || source.Project() == nil || len(published) == 0 {
		return false
	}
	mounts := source.Project().Mounts()
	if mounts.Count() != len(published) {
		return false
	}
	for index, mount := range published {
		shard, shardOK := mounts.At(index)
		mounted, mountedOK := mounts.Program(shard)
		module, moduleOK := source.Project().ModuleKey(shard)
		if !shardOK || !mountedOK || mounted == nil || !moduleOK || !mount.valid() || mounted.ContentID() != mount.programID || module != mount.moduleKey {
			return false
		}
	}
	return true
}

// compiledValueCoordinate is the immutable Link substitution for one Value
// factor coordinate. Its order is the sole Boundary Values denominator used
// by the Value schema and its summary query; result detachment therefore does
// not reopen Link, Program, or Flow.
type compiledValueCoordinate struct {
	id    identity.ContentID
	mount identity.ContentID
}

func compileValueCoordinates(source *link.Link) ([]compiledValueCoordinate, bool) {
	if source == nil || source.Project() == nil || source.Boundary() == nil {
		return nil, false
	}
	values := source.Boundary().Values()
	if values.Count() == 0 {
		return nil, false
	}
	rows := make([]compiledValueCoordinate, values.Count())
	seen := make(map[struct {
		mount identity.ContentID
		id    identity.ContentID
	}]struct{}, len(rows))
	for index := range rows {
		value, valueOK := values.At(index)
		id, idOK := values.ID(value)
		shard, _, originOK := values.Origin(value)
		mounted, programOK := source.Project().Mounts().Program(shard)
		module, moduleOK := source.Project().ModuleKey(shard)
		if !valueOK || !idOK || !originOK || !programOK || mounted == nil || !moduleOK || !id.Available() || !module.Available() {
			return nil, false
		}
		key := struct {
			mount identity.ContentID
			id    identity.ContentID
		}{mount: module, id: id}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		rows[index] = compiledValueCoordinate{id: id, mount: module}
	}
	return rows, true
}

func (mount mountedProgramArtifact) valid() bool {
	if mount.artifact == nil || !mount.artifact.Available() ||
		mount.template == nil || !mount.template.Available() || mount.roles == nil ||
		!mount.programID.Available() || !mount.moduleKey.Available() {
		return false
	}
	return mount.artifact.CompileKey().ProgramID() == mount.programID &&
		mount.template.ArtifactID() == mount.artifact.ID() &&
		mount.template.ProgramID() == mount.programID &&
		mount.template.SchemaID() == mount.artifact.CompileKey().SchemaDigest()
}

type compiledArtifactSet struct {
	receipt   composite.Compilation
	mounts    []mountedProgramArtifact
	byProgram map[identity.ContentID]*programartifact.Artifact
}

type artifactCacheState struct {
	sync.Mutex
	entries map[artifactCacheKey]*artifactCacheEntry
}

// artifactCacheKey is the complete Program compiler identity, not merely a
// Program/schema pair. A new grammar or compiler law therefore cannot alias
// an immutable artifact compiled under a prior contract.
type artifactCacheKey = identity.ContentID

type artifactCacheEntry struct {
	ready    chan struct{}
	artifact *programartifact.Artifact
	template *rows.ArtifactScalarTemplate
	roles    *artifactScalarRoleDirectory
	complete bool
}

// globalArtifactCache owns the reusable sealed ProgramArtifact together with
// its owner-neutral Engine template. Neither payload retains Link authority.
var globalArtifactCache = artifactCacheState{entries: make(map[artifactCacheKey]*artifactCacheEntry)}

func cachedProgramArtifact(input *program.Program, receipt composite.Compilation) (*programartifact.Artifact, *rows.ArtifactScalarTemplate, *artifactScalarRoleDirectory, bool) {
	compileKey, keyOK := composite.NewArtifactCompileKey(input, receipt)
	programID, schemaID := input.ContentID(), receipt.Digest()
	if !keyOK || !compileKey.Available() || !input.Available() || !programID.Available() || !receipt.Available() || !schemaID.Available() {
		return nil, nil, nil, false
	}
	key := compileKey.ID()
	globalArtifactCache.Lock()
	entry := globalArtifactCache.entries[key]
	if entry == nil {
		entry = &artifactCacheEntry{ready: make(chan struct{})}
		globalArtifactCache.entries[key] = entry
		globalArtifactCache.Unlock()

		artifact, compiled := composite.CompileArtifact(input, receipt)
		var template *rows.ArtifactScalarTemplate
		var roles *artifactScalarRoleDirectory
		if compiled {
			template, roles, compiled = newEngineArtifactScalarTemplate(artifact)
		}
		valid := compiled && artifact != nil && artifact.Available() && artifact.CompileKey().ID() == key && artifact.CompileKey().ProgramID() == programID && artifact.CompileKey().SchemaDigest() == schemaID && template != nil && template.Available() && template.ArtifactID() == artifact.ID() && template.ProgramID() == programID && template.SchemaID() == schemaID && roles != nil
		globalArtifactCache.Lock()
		if valid {
			entry.artifact = artifact
			entry.template = template
			entry.roles = roles
		}
		entry.complete = valid
		close(entry.ready)
		if !valid {
			delete(globalArtifactCache.entries, key)
		}
		globalArtifactCache.Unlock()
		return artifact, template, roles, valid
	}
	ready := entry.ready
	globalArtifactCache.Unlock()
	<-ready
	valid := entry.complete && entry.artifact != nil && entry.artifact.Available() && entry.artifact.CompileKey().ID() == key && entry.artifact.CompileKey().ProgramID() == programID && entry.artifact.CompileKey().SchemaDigest() == schemaID && entry.template != nil && entry.template.Available() && entry.template.ArtifactID() == entry.artifact.ID() && entry.template.ProgramID() == programID && entry.template.SchemaID() == schemaID && entry.roles != nil
	return entry.artifact, entry.template, entry.roles, valid
}

// compileProgramArtifacts compiles each distinct ProgramID once and records
// every mounted occurrence's exact Link substitution. No Link/domain/runtime
// authority enters the reusable artifact cache.
func compileProgramArtifacts(source *link.Link, receipt composite.Compilation) (*compiledArtifactSet, bool) {
	if source == nil || !source.ContentID().Available() || !receipt.Available() || source.Project() == nil {
		return nil, false
	}
	mounts := source.Project().Mounts()
	if mounts.Count() == 0 {
		return nil, false
	}
	result := &compiledArtifactSet{receipt: receipt, mounts: make([]mountedProgramArtifact, 0, mounts.Count()), byProgram: make(map[identity.ContentID]*programartifact.Artifact)}
	type cachedProduct struct {
		artifact *programartifact.Artifact
		template *rows.ArtifactScalarTemplate
		roles    *artifactScalarRoleDirectory
	}
	products := make(map[identity.ContentID]cachedProduct)
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mounted, programOK := mounts.Program(shard)
		moduleKey, moduleOK := source.Project().ModuleKey(shard)
		if !shardOK || !programOK || mounted == nil || !moduleOK || !moduleKey.Available() {
			return nil, false
		}
		input := mounted
		programID := input.ContentID()
		if !input.Available() || !programID.Available() {
			return nil, false
		}
		product, compiled := products[programID]
		artifact, template, roles := product.artifact, product.template, product.roles
		if !compiled {
			artifact, template, roles, compiled = cachedProgramArtifact(input, receipt)
			if !compiled {
				return nil, false
			}
			products[programID] = cachedProduct{artifact: artifact, template: template, roles: roles}
		}
		if artifact == nil || !artifact.Available() || template == nil || !template.Available() || roles == nil || artifact.CompileKey().ProgramID() != programID || artifact.CompileKey().SchemaDigest() != receipt.Digest() || template.ArtifactID() != artifact.ID() || template.ProgramID() != programID || template.SchemaID() != receipt.Digest() {
			return nil, false
		}
		if _, held := result.byProgram[programID]; !held {
			result.byProgram[programID] = artifact
		}
		result.mounts = append(result.mounts, mountedProgramArtifact{artifact: artifact, template: template, roles: roles, programID: programID, moduleKey: moduleKey})
	}
	return result, true
}
