package analysis

// compile.go is the Compile path: reusable artifact cache, scalar template
// lowering, and Link-local binding. Runtime assemble lives in analyze.go.

import (
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/engine/rows/scalarlower"

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
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

var compositionStores atomic.Uint64

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

func mountedProgramRoles(directory *scalarlower.RoleDirectory, binding *composite.ProgramBinding) ([]engine.MountedProgramRole, bool) {
	if directory == nil || binding == nil {
		return nil, false
	}
	roles := make([]engine.MountedProgramRole, 0, directory.Count())
	for index := 0; index < directory.Count(); index++ {
		key, scalar, rowOK := directory.At(index)
		capability, capabilityOK := mountedCapability(binding, key)
		if !rowOK || !capabilityOK {
			return nil, false
		}
		roles = append(roles, engine.MountedProgramRole{Scalar: scalar, Capability: capability})
	}
	return roles, true
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
		targetCount, targetsPublished := published.program.CallTargetCount()
		if !targetsPublished {
			return composite.LinkInputs{}, nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, allocationcatalog.SealFailureNone
		}
		for rowIndex := 0; rowIndex < targetCount; rowIndex++ {
			row, rowOK := published.program.CallTargetAt(rowIndex)
			body, bodyOK := bodies[row.Body]
			function, formal := body.FunctionID(), body.CallFormalID()
			if !rowOK || !row.Available() || !bodyOK || !body.Callable() || !function.Available() || !formal.Available() ||
				row.Context != body.ContextID() || row.Function != function || row.Formal != formal {
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
	mountWrite, mountMinted := engine.MintColumnWrite[identity.ContentID, cold.Program](state.binding.SchemaBinding(), programmount.OutputKey, programmount.AxisKey)
	if !mountMinted || !mountWrite.Available() {
		return false
	}
	if state.artifacts == nil {
		return false
	}
	denominator, denominatorOK := programmount.DenominatorID(state.sourceID)
	if !denominatorOK {
		return false
	}
	directoryRows := make([]cold.Program, len(state.artifacts.mounts))
	for index, mount := range state.artifacts.mounts {
		directoryRows[index] = mount.program
	}
	directory, directoryOK := programmount.Content(directoryRows, denominator)
	if !directoryOK {
		return false
	}
	builder := snapshot.NewBuilder(schemaID, store, identity.Generation(1))
	if err := selectapply.Publish(write, &builder, apps); err != nil {
		return false
	}
	if err := engine.PublishColumn(mountWrite, &builder, directory); err != nil {
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
		row := axis.MountedArtifact{Program: mounted.program, Snapshot: mounted.snapshot}
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
	// program is this mount's entry in the Link's mount directory: the
	// artifact's frozen cold publication under the module key it is mounted
	// at. Families that have moved onto the cold publication are read through
	// it; the ingress view below still carries the families that have not.
	program   cold.Program
	snapshot  *ingress.Snapshot
	template  *rows.ArtifactScalarTemplate
	roles     *scalarlower.RoleDirectory
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
	roles    *scalarlower.RoleDirectory
	complete bool
}

// globalArtifactCache owns the reusable sealed ProgramArtifact together with
// its owner-neutral Engine template and the sealed ingress snapshot they
// were projected from. No payload retains Link authority.
var globalArtifactCache = artifactCacheState{entries: make(map[artifactCacheKey]*artifactCacheEntry)}

func cachedProgramArtifact(input *program.Program, receipt composite.Compilation) (*programartifact.Artifact, *ingress.Snapshot, *rows.ArtifactScalarTemplate, *scalarlower.RoleDirectory, bool) {
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
		var roles *scalarlower.RoleDirectory
		if compiled {
			structural, structuralOK := composite.StructureVocabulary()
			var lowered bool
			snapshot, lowered = ingress.Lower(artifact, structural)
			compiled = structuralOK && lowered
			if compiled {
				template, roles, compiled = scalarlower.Lower(snapshot, structural)
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
		roles    *scalarlower.RoleDirectory
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
		frozen, catalog, coldOK := artifact.ColdPublication()
		if !coldOK || !catalog.Available() {
			return nil, false
		}
		program := cold.Program{
			Frozen: frozen, ModuleKey: moduleKey, ArtifactID: artifact.ID(),
			ProgramID: programID, SchemaID: receipt.Digest(),
		}
		if !program.Available() {
			return nil, false
		}
		result.mounts = append(result.mounts, mountedProgramArtifact{program: program, snapshot: snapshot, template: template, roles: roles, programID: programID, moduleKey: moduleKey})
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
