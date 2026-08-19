package analysis

// compile.go is the Compile path: reusable artifact cache, scalar template
// lowering, and Link-local binding. Runtime assemble lives in analyze.go.

import (
	"crypto/sha256"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/rows"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/composite"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	"github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/channelselect"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/link/mounted"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

var compositionStores atomic.Uint64

type artifactScalarRoleBinding struct {
	key    schema.Key
	scalar rows.ArtifactScalarRole
}

// artifactScalarRoleDirectory is immutable Program-template metadata. It
// contains no Link-local capability and is shared with the template cache.
type artifactScalarRoleDirectory struct{ rows []artifactScalarRoleBinding }

func (directory *artifactScalarRoleDirectory) role(key schema.Key) (rows.ArtifactScalarRole, bool) {
	if directory != nil && key.Available() {
		for _, row := range directory.rows {
			if row.key == key {
				return row.scalar, row.scalar.Available()
			}
		}
	}
	return rows.ArtifactScalarRole{}, false
}

func artifactScalarRoleSemantic(artifact identity.ContentID, key schema.Key) identity.ContentID {
	if !artifact.Available() || !key.Available() {
		return identity.ContentID{}
	}
	input := make([]byte, 0, len("analysis/artifact-scalar-role/v1")+len(artifact)+len(key))
	input = append(input, "analysis/artifact-scalar-role/v1"...)
	input = append(input, artifact[:]...)
	input = append(input, key...)
	return identity.ContentID(sha256.Sum256(input))
}

// newEngineArtifactScalarTemplate is the sole sealed-snapshot→Engine
// structural boundary. It runs once while publishing the content-addressed
// cache entry.
func newEngineArtifactScalarTemplate(snapshot *ingress.Snapshot) (*rows.ArtifactScalarTemplate, *artifactScalarRoleDirectory, bool) {
	if snapshot == nil || !snapshot.Available() {
		return nil, nil, false
	}
	usedKeys := make(map[schema.Key]struct{})
	for index := 0; index < snapshot.LocalTransferCount(); index++ {
		row, ok := snapshot.LocalTransferAt(index)
		if !ok {
			return nil, nil, false
		}
		for inner := 0; inner < row.WritesCount(); inner++ {
			write, writeOK := row.WritesAt(inner)
			if !writeOK {
				return nil, nil, false
			}
			usedKeys[write] = struct{}{}
		}
	}
	for index := 0; index < snapshot.RulePlacementCount(); index++ {
		row, ok := snapshot.RulePlacementAt(index)
		if !ok || !row.Key().Available() {
			return nil, nil, false
		}
		usedKeys[row.Key()] = struct{}{}
	}
	spec, specOK := rows.NewArtifactScalarSpec(snapshot.ArtifactID(), snapshot.ProgramID(), snapshot.SchemaID(), rows.ArtifactScalarCapacity{
		Roles: len(usedKeys), Points: snapshot.PointCount(), Edges: snapshot.StructuralEdgeCount(), Transfers: snapshot.LocalTransferCount(), Regions: snapshot.RegionCount(), Events: snapshot.EventCount(), Rules: snapshot.RulePlacementCount(), Bodies: snapshot.BodyTransportCount(),
	})
	if !specOK {
		return nil, nil, false
	}
	laws, lawsOK := composite.IssuanceStageLaws()
	if !lawsOK || !spec.InstallStageLaws(laws) {
		return nil, nil, false
	}
	directory := &artifactScalarRoleDirectory{rows: make([]artifactScalarRoleBinding, 0, len(usedKeys))}
	ordered := make([]schema.Key, 0, len(usedKeys))
	for key := range usedKeys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	for _, key := range ordered {
		scalar, scalarOK := spec.DeclareRole(artifactScalarRoleSemantic(snapshot.ArtifactID(), key))
		if !scalarOK {
			return nil, nil, false
		}
		directory.rows = append(directory.rows, artifactScalarRoleBinding{key: key, scalar: scalar})
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
		for inner := 0; inner < row.WritesCount(); inner++ {
			write, writeOK := row.WritesAt(inner)
			role, roleOK := directory.role(write)
			if !writeOK || !roleOK || !spec.AddTransferFactor(transfer, role) {
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
		scalarRole, scalarRoleOK := directory.role(row.Key())
		stage, stageOK := engineArtifactRuleStage(row.Stage())
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

func diagnosticRuleForMountedRole(binding *composite.ProgramBinding, role engine.RuleSlotCapability) anadiag.AnalyzeDiagnosticRule {
	rules := binding.Rules()
	if rules == nil || !role.Mounted() {
		return anadiag.AnalyzeDiagnosticRuleUnknown
	}
	return rules.DiagnosticForCapability(role)
}

func diagnosticRuleForLinkRole(binding *composite.ProgramBinding, role engine.RuleSlotCapability) anadiag.AnalyzeDiagnosticRule {
	rules := binding.Rules()
	if rules == nil || !role.Link() {
		return anadiag.AnalyzeDiagnosticRuleUnknown
	}
	return rules.DiagnosticForCapability(role)
}

func mountedCapability(binding *composite.ProgramBinding, key schema.Key) (engine.RuleSlotCapability, bool) {
	rules := binding.Rules()
	if rules == nil {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := rules.CapabilityByKey(key)
	return capability, ok && capability.Mounted()
}

func mountedProgramRoles(directory *artifactScalarRoleDirectory, binding *composite.ProgramBinding) ([]engine.MountedProgramRole, bool) {
	if directory == nil || binding == nil {
		return nil, false
	}
	roles := make([]engine.MountedProgramRole, 0, len(directory.rows))
	for _, row := range directory.rows {
		capability, capabilityOK := mountedCapability(binding, row.key)
		if !capabilityOK {
			return nil, false
		}
		roles = append(roles, engine.MountedProgramRole{Scalar: row.scalar, Capability: capability})
	}
	return roles, true
}

// engineArtifactRuleStage is the sealed execution-cut bijection. The placement
// already carries the issued stage; a scalar caller cannot retag it. The two
// stage vocabularies are proven ordinal-for-ordinal identical by
// TestEngineArtifactVocabularyIsTheSealedTable, so the translation is a
// range-checked cast rather than a per-member switch.
func engineArtifactRuleStage(stage uint8) (rows.ArtifactRuleStage, bool) {
	converted := rows.ArtifactRuleStage(stage)
	if !converted.Valid() {
		return rows.ArtifactRuleStageInvalid, false
	}
	return converted, true
}

// engineStructuralArm is the sealed structural-arm bijection between the
// ingress and engine spellings, proven ordinal-for-ordinal identical by
// TestEngineArtifactVocabularyIsTheSealedTable.
func engineStructuralArm(arm ingress.StructuralArm) (rows.ArtifactStructuralArm, bool) {
	if !arm.Valid() {
		return rows.ArtifactStructuralArmInvalid, false
	}
	return rows.ArtifactStructuralArm(arm), true
}

// engineEventKind is the sealed event-kind bijection between the ingress and
// engine spellings, proven ordinal-for-ordinal identical by
// TestEngineArtifactVocabularyIsTheSealedTable.
func engineEventKind(kind ingress.EventKind) (rows.ArtifactEventKind, bool) {
	if kind < ingress.EventEnter || kind > ingress.EventExit {
		return rows.ArtifactEventInvalid, false
	}
	return rows.ArtifactEventKind(kind), true
}

func linkBootstrapWitness(state *compiledState, binding *composite.ProgramBinding) (engine.ProgramBootstrap, bool) {
	if state == nil || binding == nil || binding.Rules() == nil || !state.sourceID.Available() {
		return engine.ProgramBootstrap{}, false
	}
	catalogs, catalogsOK := binding.Rules().BootstrapCatalogs()
	if !catalogsOK || len(catalogs) != 2 {
		return engine.ProgramBootstrap{}, false
	}
	pointID, pointOK := identity.DeriveContentID("analysis/link-bootstrap-point/v1", state.sourceID[:])
	if !pointOK {
		return engine.ProgramBootstrap{}, false
	}
	return engine.NewProgramBootstrap(state.sourceID, pointID, catalogs...)
}

// newProgramBinding constructs the Link-local typed owners required by
// compile. Sealed ingress rows supply the identities those owners admit;
// domain schemas are solve-local substitutions.
func (state *compiledState) newProgramBinding(source *link.Link) (composite.LinkInputs, *composite.ProgramBinding, anadiag.ProgramBindingFailure, composite.MountFailure, allocationcatalog.SealFailure) {
	if state == nil || source == nil || state.artifacts == nil || len(state.artifacts.mounts) == 0 {
		return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureInput, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	// A Shard is a cold Project coordinate. It is reissued only while Link is
	// live, to authenticate this mount set against the Project.
	if !projectAuthenticatesMounts(source, state.artifacts.mounts) {
		return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureInput, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	artifactTypes := make([]*programartifact.Artifact, 0, len(state.artifacts.byProgram))
	for _, artifact := range state.artifacts.byProgram {
		if artifact == nil || !artifact.Available() {
			return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureTypes, composite.MountFailure{}, allocationcatalog.SealFailureNone
		}
		artifactTypes = append(artifactTypes, artifact)
	}
	types, typesErr := typeauthority.SealArtifactRows(state.sourceID, artifactTypes)
	if typesErr != nil {
		return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureTypes, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	sealed, sealedOK := linkArtifactRows(state.artifacts.mounts)
	if !sealedOK {
		return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureInput, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	staticMounts := make([]staticdomain.MountedArtifact, len(state.artifacts.mounts))
	staticValueIDs := make([]staticdomain.MountedValueID, 0)
	staticValues := source.Boundary().Values()
	seenStaticValues := make(map[[2]identity.ContentID]struct{})
	for index, published := range state.artifacts.mounts {
		if !published.valid() {
			return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
		}
		artifact, have := state.artifacts.byProgram[published.programID]
		if !have || artifact == nil || !artifact.Available() ||
			published.snapshot.ArtifactID() != artifact.ID() ||
			artifact.CompileKey().ProgramID() != published.programID {
			return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureTypes, composite.MountFailure{}, allocationcatalog.SealFailureNone
		}
		staticMounts[index] = staticdomain.MountedArtifact{Artifact: artifact, ModuleID: published.moduleKey, ProgramID: published.programID, NamespaceID: published.moduleKey}
		if index >= len(sealed) || sealed[index].ModuleKey != published.moduleKey || sealed[index].Snapshot != published.snapshot {
			return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
		}
		snapshot := published.snapshot
		for rowIndex := 0; rowIndex < snapshot.StaticTypeNodeCount(); rowIndex++ {
			row, rowOK := snapshot.StaticTypeNodeAt(rowIndex)
			if !rowOK || !row.Available() || row.Owner() != published.programID {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureTypes, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for rowIndex := 0; rowIndex < snapshot.StaticExpressionCount(); rowIndex++ {
			row, rowOK := snapshot.StaticExpressionAt(rowIndex)
			if !rowOK || !row.Available() || row.Owner() != published.programID {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for rowIndex := 0; rowIndex < snapshot.StaticInputCount(); rowIndex++ {
			row, rowOK := snapshot.StaticInputAt(rowIndex)
			if !rowOK || !row.Available() || row.Owner() != published.programID {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		seenCalls := make(map[identity.ContentID]struct{}, snapshot.CallCount())
		for rowIndex := 0; rowIndex < snapshot.CallCount(); rowIndex++ {
			row, rowOK := snapshot.CallAt(rowIndex)
			if !rowOK || !row.ID().Available() || !row.BodyID().Available() || !row.TypeArgumentsID().Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			if _, duplicate := seenCalls[row.ID()]; duplicate {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			seenCalls[row.ID()] = struct{}{}
			var callee ingress.CallOperand
			calleeOK := false
			for operandIndex := 0; operandIndex < row.OperandCount(); operandIndex++ {
				operand, operandOK := row.OperandAt(operandIndex)
				if !operandOK || operand.CallID() != row.ID() {
					return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
				}
				if !operand.Callee() {
					continue
				}
				if calleeOK {
					return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
				}
				callee, calleeOK = operand, true
			}
			if !calleeOK || callee.ID() != row.CalleeID() || callee.ValueID() != row.CalleeID() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for rowIndex := 0; rowIndex < snapshot.StaticTypeArgumentCount(); rowIndex++ {
			row, rowOK := snapshot.StaticTypeArgumentAt(rowIndex)
			if !rowOK || !row.Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for rowIndex := 0; rowIndex < snapshot.StaticTypeValueCount(); rowIndex++ {
			row, rowOK := snapshot.StaticTypeValueAt(rowIndex)
			if !rowOK || !row.Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			key := [2]identity.ContentID{published.moduleKey, row.ID()}
			if _, duplicate := seenStaticValues[key]; duplicate {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			value, valueOK := staticValues.ForMountedSemantic(published.moduleKey, row.ID())
			valueID, valueIDOK := staticValues.ID(value)
			if !valueOK || !valueIDOK || !valueID.Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			seenStaticValues[key] = struct{}{}
			staticValueIDs = append(staticValueIDs, staticdomain.MountedValueID{
				ModuleID: published.moduleKey, SemanticID: row.ID(), ValueID: valueID,
			})
		}
		for rowIndex := 0; rowIndex < snapshot.ValuesCount(); rowIndex++ {
			row, rowOK := snapshot.ValuesAt(rowIndex)
			if !rowOK || !row.ID().Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for rowIndex := 0; rowIndex < snapshot.HeapIndexCount(); rowIndex++ {
			row, rowOK := snapshot.HeapIndexAt(rowIndex)
			if !rowOK || !row.Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for rowIndex := 0; rowIndex < snapshot.HeapAllocationCount(); rowIndex++ {
			row, rowOK := snapshot.HeapAllocationAt(rowIndex)
			if !rowOK || !row.Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		bodies := make(map[identity.ContentID]ingress.BodyTransport, snapshot.BodyTransportCount())
		for rowIndex := 0; rowIndex < snapshot.BodyTransportCount(); rowIndex++ {
			row, rowOK := snapshot.BodyTransportAt(rowIndex)
			if !rowOK || !row.ID().Available() || !row.ContextID().Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			if _, duplicate := bodies[row.ID()]; duplicate {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			_, callable := snapshot.FunctionBoundaryForBody(row.ID())
			if row.Callable() != callable {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			bodies[row.ID()] = row
		}
		for rowIndex := 0; rowIndex < snapshot.CallTargetCount(); rowIndex++ {
			row, rowOK := snapshot.CallTargetAt(rowIndex)
			body, bodyOK := bodies[row.BodyID()]
			function, formal := body.FunctionID(), body.CallFormalID()
			if !rowOK || !row.Available() || !bodyOK || !body.Callable() || !function.Available() || !formal.Available() ||
				row.ContextID() != body.ContextID() || row.FunctionContextID() != function || row.CallFormalID() != formal {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for rowIndex := 0; rowIndex < snapshot.FunctionBoundaryCount(); rowIndex++ {
			row, rowOK := snapshot.FunctionBoundaryAt(rowIndex)
			if !rowOK || !row.Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			for formalIndex := 0; formalIndex < row.FormalCount(); formalIndex++ {
				formal, formalOK := row.FormalAt(formalIndex)
				if !formalOK || !formal.Available() || !formal.StorageCellID().Available() {
					return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
				}
			}
		}
		for rowIndex := 0; rowIndex < snapshot.OutcomeCount(); rowIndex++ {
			row, rowOK := snapshot.OutcomeAt(rowIndex)
			if !rowOK || !row.ID().Available() || !row.BodyID().Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			for valueIndex := 0; valueIndex < row.ReturnValueCount(); valueIndex++ {
				value, valueOK := snapshot.OutcomeReturnValueAt(rowIndex, valueIndex)
				if !valueOK || !value.ID().Available() {
					return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
				}
			}
		}
		for rowIndex := 0; rowIndex < snapshot.OccurrenceKindCount(uint8(programartifact.OccurrenceStorageBind)); rowIndex++ {
			row, rowOK := snapshot.OccurrenceKindAt(uint8(programartifact.OccurrenceStorageBind), rowIndex)
			if !rowOK || row.InputCount() == 0 {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for rowIndex := 0; rowIndex < snapshot.OccurrenceKindCount(uint8(programartifact.OccurrenceAllocation)); rowIndex++ {
			row, rowOK := snapshot.OccurrenceKindAt(uint8(programartifact.OccurrenceAllocation), rowIndex)
			if !rowOK || !row.ID().Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
			if recovered, recoveredOK := snapshot.OccurrenceForID(uint8(programartifact.OccurrenceAllocation), row.ID()); !recoveredOK || recovered.ID() != row.ID() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
		for _, kind := range []uint8{uint8(programartifact.OccurrenceIndexRead), uint8(programartifact.OccurrenceIndexWrite)} {
			for rowIndex := 0; rowIndex < snapshot.OccurrenceKindCount(kind); rowIndex++ {
				row, rowOK := snapshot.OccurrenceKindAt(kind, rowIndex)
				if !rowOK || !row.ID().Available() {
					return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
				}
				if recovered, recoveredOK := snapshot.OccurrenceForID(kind, row.ID()); !recoveredOK || recovered.ID() != row.ID() {
					return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
				}
			}
		}
		for rowIndex := 0; rowIndex < snapshot.OccurrenceCount(); rowIndex++ {
			row, rowOK := snapshot.OccurrenceAt(rowIndex)
			if !rowOK || !row.ID().Available() {
				return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
			}
		}
	}
	staticTarget, staticTargetOK := source.Boundary().Target()
	if !staticTargetOK {
		return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	static, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{
		LinkID:   state.sourceID,
		Target:   staticTarget,
		ValueIDs: staticValueIDs,
	}, types, staticMounts)
	if err != nil || static == nil {
		return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
	}
	// The neutral sealed artifact view is the mount phase's whole artifact
	// input. Every axis that owns its mount seals its own Link authority from
	// it and from the peers it declared an edge to, so no per-domain mount row
	// is constructed here.
	artifactRows := sealed
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
		return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureFromMount(mountFailure), mountFailure, allocationcatalog.SealFailureNone
	}
	binding, failure := composite.BindProgram(state.receipt, inputs)
	if failure.Available() {
		return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureFromBind(failure), composite.MountFailure{}, failure.Allocation
	}
	return inputs, binding, anadiag.ProgramBindingFailureNone, composite.MountFailure{}, allocationcatalog.SealFailureNone
}

func nextCompositionStore() (identity.StoreID, bool) {
	n := compositionStores.Add(1)
	if n == 0 {
		return 0, false
	}
	return identity.StoreID(n), true
}

// publishComposition writes the Link-lifetime StorageEngine prefix. ChannelSelect
// occupies snapshot slot 0, so a select-only column seals without factor facts.
func (state *compiledState) publishComposition(source *link.Link) bool {
	if state == nil || source == nil || state.binding == nil || state.binding.SchemaBinding() == nil {
		return false
	}
	schemaID, schemaOK := composite.PublicationSchema()
	store, storeOK := nextCompositionStore()
	if !schemaOK || !storeOK || !schemaID.Available() {
		return false
	}
	write, minted := engine.MintColumnWrite[identity.ContentID, channelselect.CaseFact](state.binding.SchemaBinding(), selectapply.OutputKey, selectapply.AxisKey)
	if !minted || !write.Available() {
		return false
	}
	mounts := source.Project().Mounts()
	var apps []selectapply.Application
	var handlers []selectapply.Handler
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		prog, progOK := mounts.Program(shard)
		if !shardOK || !progOK || prog == nil {
			return false
		}
		progApps := selectapply.Apply(prog)
		apps = append(apps, progApps...)
		handlers = append(handlers, selectapply.Handlers(prog, progApps)...)
	}
	builder := snapshot.NewBuilder(schemaID, store, identity.Generation(1))
	if err := selectapply.Publish(write, &builder, apps); err != nil {
		return false
	}
	sealed, err := builder.Seal()
	if err != nil || !sealed.Published() {
		return false
	}
	sites := make([]anadiag.SelectSite, len(apps))
	for index, app := range apps {
		bound := 0
		for _, fact := range app.Facts.All() {
			if fact.Ordinal+1 > bound {
				bound = fact.Ordinal + 1
			}
		}
		if !app.Site.Available() {
			return false
		}
		sites[index] = anadiag.SelectSite{Site: app.Site, Bound: bound}
	}
	state.composition = sealed
	state.selectSites = sites
	state.selectHandlers = handlers
	return true
}

// linkArtifactRows projects the Link's private mount records onto the neutral
// artifact view the mount phase consumes. Each row carries the compile-time
// snapshot pointer.
func linkArtifactRows(mounts []mountedProgramArtifact) ([]axis.MountedArtifact, bool) {
	if len(mounts) == 0 {
		return nil, false
	}
	rows := make([]axis.MountedArtifact, len(mounts))
	for index, mounted := range mounts {
		if !mounted.valid() {
			return nil, false
		}
		row := axis.MountedArtifact{Snapshot: mounted.snapshot, ModuleKey: mounted.moduleKey, ProgramID: mounted.programID}
		if !row.Available() {
			return nil, false
		}
		rows[index] = row
	}
	return rows, true
}

// mountedProgramArtifact is the compile-time snapshot plus the exact Link
// substitution needed to place that Program occurrence in a Link. The
// snapshot and template are shared by ProgramID; the mount row is never
// shared. The owner-handoff ProgramArtifact lives on compiledArtifactSet.byProgram.
type mountedProgramArtifact struct {
	snapshot  *ingress.Snapshot
	template  *rows.ArtifactScalarTemplate
	roles     *artifactScalarRoleDirectory
	programID identity.ContentID
	moduleKey identity.ContentID
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
	if mount.snapshot == nil || !mount.snapshot.Available() ||
		mount.template == nil || !mount.template.Available() || mount.roles == nil ||
		!mount.programID.Available() || !mount.moduleKey.Available() {
		return false
	}
	schemaID := mount.snapshot.SchemaID()
	return mount.snapshot.ProgramID() == mount.programID &&
		mount.snapshot.ArtifactID().Available() &&
		mount.template.ArtifactID() == mount.snapshot.ArtifactID() &&
		mount.template.ProgramID() == mount.programID &&
		mount.template.SchemaID() == schemaID
}

type compiledArtifactSet struct {
	receipt   composite.Compilation
	mounts    []mountedProgramArtifact
	byProgram map[identity.ContentID]*programartifact.Artifact
	sites     mounted.ObservationSites
}

func (artifacts *compiledArtifactSet) observationCensus(coordinates []compiledValueCoordinate) ([]anadiag.Observation, bool) {
	if artifacts == nil {
		return nil, false
	}
	mounts := make([]anadiag.MountedCensus, len(artifacts.mounts))
	for index, mount := range artifacts.mounts {
		mounts[index] = anadiag.MountedCensus{ModuleKey: mount.moduleKey, Snapshot: mount.snapshot}
	}
	values := make([]anadiag.ValueCoordinate, len(coordinates))
	for index, coordinate := range coordinates {
		values[index] = anadiag.ValueCoordinate{Mount: coordinate.mount, ID: coordinate.id}
	}
	return anadiag.ProjectSites(artifacts.sites, mounts, values)
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
	snapshot *ingress.Snapshot
	template *rows.ArtifactScalarTemplate
	roles    *artifactScalarRoleDirectory
	complete bool
}

// globalArtifactCache owns the reusable sealed ProgramArtifact together with
// its owner-neutral Engine template and the sealed ingress snapshot they
// were projected from. No payload retains Link authority.
var globalArtifactCache = artifactCacheState{entries: make(map[artifactCacheKey]*artifactCacheEntry)}

func cachedProgramArtifact(input *program.Program, receipt composite.Compilation) (*programartifact.Artifact, *ingress.Snapshot, *rows.ArtifactScalarTemplate, *artifactScalarRoleDirectory, bool) {
	compileKey, keyOK := composite.NewArtifactCompileKey(input, receipt)
	programID, schemaID := input.ContentID(), receipt.Digest()
	if !keyOK || !compileKey.Available() || !input.Available() || !programID.Available() || !receipt.Available() || !schemaID.Available() {
		return nil, nil, nil, nil, false
	}
	key := compileKey.ID()
	globalArtifactCache.Lock()
	entry := globalArtifactCache.entries[key]
	if entry == nil {
		entry = &artifactCacheEntry{ready: make(chan struct{})}
		globalArtifactCache.entries[key] = entry
		globalArtifactCache.Unlock()

		artifact, compiled := composite.CompileArtifact(input, receipt)
		var snapshot *ingress.Snapshot
		var template *rows.ArtifactScalarTemplate
		var roles *artifactScalarRoleDirectory
		if compiled {
			structural, structuralOK := composite.StructureVocabulary()
			var lowered bool
			snapshot, lowered = ingress.Lower(artifact, structural)
			compiled = structuralOK && lowered
			if compiled {
				template, roles, compiled = newEngineArtifactScalarTemplate(snapshot)
			}
		}
		valid := compiled && artifact != nil && artifact.Available() && artifact.CompileKey().ID() == key && artifact.CompileKey().ProgramID() == programID && artifact.CompileKey().SchemaDigest() == schemaID && snapshot != nil && snapshot.Available() && snapshot.ArtifactID() == artifact.ID() && snapshot.ProgramID() == programID && snapshot.SchemaID() == schemaID && template != nil && template.Available() && template.ArtifactID() == artifact.ID() && template.ProgramID() == programID && template.SchemaID() == schemaID && roles != nil
		globalArtifactCache.Lock()
		if valid {
			entry.artifact = artifact
			entry.snapshot = snapshot
			entry.template = template
			entry.roles = roles
		}
		entry.complete = valid
		close(entry.ready)
		if !valid {
			delete(globalArtifactCache.entries, key)
		}
		globalArtifactCache.Unlock()
		return artifact, snapshot, template, roles, valid
	}
	ready := entry.ready
	globalArtifactCache.Unlock()
	<-ready
	valid := entry.complete && entry.artifact != nil && entry.artifact.Available() && entry.artifact.CompileKey().ID() == key && entry.artifact.CompileKey().ProgramID() == programID && entry.artifact.CompileKey().SchemaDigest() == schemaID && entry.snapshot != nil && entry.snapshot.Available() && entry.snapshot.ArtifactID() == entry.artifact.ID() && entry.snapshot.ProgramID() == programID && entry.snapshot.SchemaID() == schemaID && entry.template != nil && entry.template.Available() && entry.template.ArtifactID() == entry.artifact.ID() && entry.template.ProgramID() == programID && entry.template.SchemaID() == schemaID && entry.roles != nil
	return entry.artifact, entry.snapshot, entry.template, entry.roles, valid
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
		snapshot *ingress.Snapshot
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
		artifact, snapshot, template, roles := product.artifact, product.snapshot, product.template, product.roles
		if !compiled {
			artifact, snapshot, template, roles, compiled = cachedProgramArtifact(input, receipt)
			if !compiled {
				return nil, false
			}
			products[programID] = cachedProduct{artifact: artifact, snapshot: snapshot, template: template, roles: roles}
		}
		if artifact == nil || !artifact.Available() || snapshot == nil || !snapshot.Available() || template == nil || !template.Available() || roles == nil || artifact.CompileKey().ProgramID() != programID || artifact.CompileKey().SchemaDigest() != receipt.Digest() || snapshot.ArtifactID() != artifact.ID() || snapshot.ProgramID() != programID || snapshot.SchemaID() != receipt.Digest() || template.ArtifactID() != artifact.ID() || template.ProgramID() != programID || template.SchemaID() != receipt.Digest() {
			return nil, false
		}
		if _, held := result.byProgram[programID]; !held {
			result.byProgram[programID] = artifact
		}
		result.mounts = append(result.mounts, mountedProgramArtifact{snapshot: snapshot, template: template, roles: roles, programID: programID, moduleKey: moduleKey})
	}
	sites, sitesOK := mounted.SealObservationSites(source.Boundary(), artifactSetMounts(result.mounts))
	if !sitesOK || !sites.Available() {
		return nil, false
	}
	result.sites = sites
	return result, true
}

func artifactSetMounts(rows []mountedProgramArtifact) []mounted.Mount {
	mounts := make([]mounted.Mount, len(rows))
	for index, row := range rows {
		mounts[index] = mounted.Mount{ModuleKey: row.moduleKey, Snapshot: row.snapshot}
	}
	return mounts
}
