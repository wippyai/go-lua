package analysis

// compile.go is the Link-local compile path. Workspace owns reusable Program
// products; runtime assemble lives in analyze.go.

import (
	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/engine/rows/scalarlower"
	"github.com/wippyai/go-lua/analysis/schema"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/composite"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	"github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	domaintypecontract "github.com/wippyai/go-lua/domain/type/typecontract"

	"github.com/wippyai/go-lua/analysis/identity"
	analysisworkspace "github.com/wippyai/go-lua/analysis/internal/workspace"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	"github.com/wippyai/go-lua/analysis/program/link/mounted"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectpublication "github.com/wippyai/go-lua/domain/effect/publication"
)

func diagnosticRuleForMountedRole(binding *composite.ProgramBinding, role engine.RuleSlotCapability) anadiag.AnalyzeDiagnosticRule {
	rules := binding.Rules()
	if rules == nil || !(role.Mounted() || role.MountedPoint()) {
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

func mountedProgramBindings(directory *scalarlower.MountDirectory, binding *composite.ProgramBinding) ([]engine.MountedProgramRole, []engine.MountedProgramFactor, bool) {
	if directory == nil || binding == nil {
		return nil, nil, false
	}
	rules := binding.Rules()
	if rules == nil {
		return nil, nil, false
	}
	roles := make([]engine.MountedProgramRole, 0, directory.RoleCount())
	for index := 0; index < directory.RoleCount(); index++ {
		key, scalar, rowOK := directory.RoleAt(index)
		capability, capabilityOK := rules.MountedCapabilityForArtifactRole(key)
		if !rowOK || !capabilityOK {
			return nil, nil, false
		}
		roles = append(roles, engine.MountedProgramRole{Scalar: scalar, Capability: capability})
	}
	factors := make([]engine.MountedProgramFactor, 0, directory.FactorCount())
	for index := 0; index < directory.FactorCount(); index++ {
		key, scalar, rowOK := directory.FactorAt(index)
		capability, capabilityOK := binding.FactorCapability(key)
		if !rowOK || !capabilityOK {
			return nil, nil, false
		}
		factors = append(factors, engine.MountedProgramFactor{Scalar: scalar, Capability: capability})
	}
	return roles, factors, true
}

func linkBootstrapWitness(state *compiledState, binding *composite.ProgramBinding) (engine.ProgramBootstrap, bool) {
	if state == nil || binding == nil || binding.Rules() == nil || !state.sourceID.Available() {
		return engine.ProgramBootstrap{}, false
	}
	catalogs, catalogsOK := binding.Rules().BootstrapCatalogs()
	if !catalogsOK || len(catalogs) != len(composite.LinkKeys(state.compilation)) {
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
func (state *compiledState) newProgramBinding(source *link.Link, compilation composite.Compilation) (*composite.ProgramBinding, anadiag.ProgramBindingFailure, composite.MountFailure, composite.BindFailure) {
	if state == nil || source == nil || !compilation.Available() || state.artifacts == nil || len(state.artifacts.mounts) == 0 {
		return nil, anadiag.ProgramBindingFailureInput, composite.MountFailure{}, composite.BindFailure{}
	}
	types := state.artifacts.types
	if types == nil || types.LinkID() != state.sourceID {
		return nil, anadiag.ProgramBindingFailureTypes, composite.MountFailure{}, composite.BindFailure{}
	}
	if len(state.artifacts.mounts) == 0 {
		return nil, anadiag.ProgramBindingFailureInput, composite.MountFailure{}, composite.BindFailure{}
	}
	staticMounts := make([]staticdomain.MountedProgram, len(state.artifacts.mounts))
	for index, published := range state.artifacts.mounts {
		if !published.Available() {
			return nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, composite.BindFailure{}
		}
		product, have := state.artifacts.products[published.ProgramID]
		artifact := product.Artifact
		if !have || artifact == nil || !artifact.Available() ||
			published.Snapshot.ArtifactID() != artifact.ID() ||
			artifact.CompileKey().ProgramID() != published.ProgramID {
			return nil, anadiag.ProgramBindingFailureTypes, composite.MountFailure{}, composite.BindFailure{}
		}
		staticMounts[index] = staticdomain.MountedProgram{Program: published.Program.Program, ModuleID: published.ModuleKey, NamespaceID: published.ModuleKey}
	}
	staticTarget, staticTargetOK := source.Boundary().Target()
	if !staticTargetOK {
		return nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, composite.BindFailure{}
	}
	static, _, err := staticdomain.SealMountedPrograms(staticdomain.MountContext{
		LinkID: state.sourceID,
		Target: staticTarget,
	}, types, staticMounts)
	if err != nil || static == nil {
		return nil, anadiag.ProgramBindingFailureStatic, composite.MountFailure{}, composite.BindFailure{}
	}
	// The neutral sealed artifact view is the mount phase's whole artifact
	// input. Every axis that owns its mount seals its own Link authority from
	// it and from the peers it declared an edge to, so no per-domain mount row
	// is constructed here.
	inputs, mountFailure := composite.MountLink(compilation, composite.LinkInputs{
		Source:          source,
		Artifacts:       state.artifacts.mounts,
		StaticAuthority: static,
	})
	// Topology and formal mounted-actual completeness are derivations over
	// several sealed factors at once, so neither is any one axis's authority to
	// mount. The mount phase derives both after every mount has sealed and names
	// the derivation that refused in its own verdict.
	if mountFailure.Available() {
		return nil, anadiag.ProgramBindingFailureFromMount(mountFailure), mountFailure, composite.BindFailure{}
	}
	binding, failure := composite.BindProgram(compilation, inputs)
	if failure.Available() {
		return nil, anadiag.ProgramBindingFailureFromBind(failure), composite.MountFailure{}, failure
	}
	return binding, anadiag.ProgramBindingFailureNone, composite.MountFailure{}, composite.BindFailure{}
}

// publishComposition writes the Link-lifetime StorageEngine prefix. ChannelSelect
// occupies snapshot slot 0, so a select-only column seals without factor facts.
func (state *compiledState) publishComposition(module *linkmodule.Component, contextDirectory executioncontext.Directory) (anadiag.AnalyzeDiagnosticCompositionFailure, schema.Key) {
	if state == nil || module == nil || state.binding == nil || state.binding.SchemaBinding() == nil || state.artifacts == nil ||
		!contextDirectory.Available() || contextDirectory.LinkID() != state.sourceID {
		return anadiag.AnalyzeDiagnosticCompositionFailureInput, ""
	}
	publication, publicationOK := state.compilation.Publication()
	schemaID, schemaOK := publication.SchemaID()
	store, storeOK := identity.IssueStore()
	if !publicationOK || !schemaOK || !storeOK || !schemaID.Available() {
		return anadiag.AnalyzeDiagnosticCompositionFailurePublicationSchema, ""
	}
	selectColumn, selectProjected := analysiscatalog.ProjectAxis[identity.ContentID, channelselect.CaseFact](publication, selectapply.OutputKey)
	if !selectProjected || !selectColumn.Available() {
		return anadiag.AnalyzeDiagnosticCompositionFailureSelectColumn, selectapply.AxisKey
	}
	write, minted := engine.MintColumnWrite[identity.ContentID, channelselect.CaseFact](state.binding.SchemaBinding(), selectapply.OutputKey, selectapply.AxisKey)
	if !minted || !write.Available() {
		return anadiag.AnalyzeDiagnosticCompositionFailureColumnGrant, selectapply.AxisKey
	}
	apps := state.artifacts.selectApplications
	handlers := state.artifacts.selectHandlers
	mountWrite, mountMinted := engine.MintColumnWrite[identity.ContentID, programmount.Program](state.binding.SchemaBinding(), programmount.OutputKey, programmount.AxisKey)
	if !mountMinted || !mountWrite.Available() {
		return anadiag.AnalyzeDiagnosticCompositionFailureColumnGrant, programmount.AxisKey
	}
	denominator, denominatorOK := programmount.DenominatorID(state.sourceID)
	if !denominatorOK {
		return anadiag.AnalyzeDiagnosticCompositionFailureDenominator, programmount.AxisKey
	}
	directoryRows := make([]programmount.Program, len(state.artifacts.mounts))
	for index, mount := range state.artifacts.mounts {
		directoryRows[index] = mount.Program
	}
	directory, directoryOK := programmount.Content(directoryRows, denominator)
	if !directoryOK {
		return anadiag.AnalyzeDiagnosticCompositionFailureContent, programmount.AxisKey
	}
	composition, compositionOK := state.binding.ModuleComposition()
	if !compositionOK || !composition.Available() || composition.LinkID() != state.sourceID {
		return anadiag.AnalyzeDiagnosticCompositionFailureRows, ""
	}
	imports := composition.Imports()
	caches := composition.Caches()
	transitions := composition.Transitions()
	generations := composition.Generations()
	outcomes := composition.Outcomes()
	stateEdges := composition.StateEdges()
	terminals := composition.Terminals()
	origins := composition.CallableOrigins()
	ingresses := composition.CallableIngresses()
	importDenominator, importDenominatorOK := modulecomposition.ImportDenominatorID(state.sourceID)
	cacheDenominator, cacheDenominatorOK := modulecomposition.CacheDenominatorID(state.sourceID)
	transitionDenominator, transitionDenominatorOK := modulecomposition.ModuleCallTransitionDenominatorID(state.sourceID)
	generationDenominator, generationDenominatorOK := modulecomposition.GenerationDenominatorID(state.sourceID)
	outcomeDenominator, outcomeDenominatorOK := modulecomposition.OutcomeDenominatorID(state.sourceID)
	stateEdgeDenominator, stateEdgeDenominatorOK := modulecomposition.ModuleReturnStateEdgeDenominatorID(state.sourceID)
	terminalDenominator, terminalDenominatorOK := modulecomposition.TerminalDenominatorID(state.sourceID)
	originDenominator, originDenominatorOK := modulecomposition.ModuleExportCallableOriginDenominatorID(state.sourceID)
	ingressDenominator, ingressDenominatorOK := modulecomposition.ModuleExportCallableIngressDenominatorID(state.sourceID)
	importContent, importContentOK := modulecomposition.ImportContent(imports, importDenominator)
	cacheContent, cacheContentOK := modulecomposition.CacheContent(caches, cacheDenominator)
	transitionContent, transitionContentOK := modulecomposition.ModuleCallTransitionContent(transitions, transitionDenominator)
	generationContent, generationContentOK := modulecomposition.GenerationContent(generations, generationDenominator)
	outcomeContent, outcomeContentOK := modulecomposition.OutcomeContent(outcomes, outcomeDenominator)
	stateEdgeContent, stateEdgeContentOK := modulecomposition.ModuleReturnStateEdgeContent(stateEdges, stateEdgeDenominator)
	terminalContent, terminalContentOK := modulecomposition.TerminalContent(terminals, terminalDenominator)
	originContent, originContentOK := modulecomposition.ModuleExportCallableOriginContent(origins, originDenominator)
	ingressContent, ingressContentOK := modulecomposition.ModuleExportCallableIngressContent(ingresses, ingressDenominator)
	for _, minted := range []compositionRowStep{
		{importDenominatorOK, importContentOK, modulecomposition.ImportAxisKey},
		{cacheDenominatorOK, cacheContentOK, modulecomposition.CacheAxisKey},
		{transitionDenominatorOK, transitionContentOK, modulecomposition.ModuleCallTransitionAxisKey},
		{generationDenominatorOK, generationContentOK, modulecomposition.GenerationAxisKey},
		{outcomeDenominatorOK, outcomeContentOK, modulecomposition.OutcomeAxisKey},
		{stateEdgeDenominatorOK, stateEdgeContentOK, modulecomposition.ModuleReturnStateEdgeAxisKey},
		{terminalDenominatorOK, terminalContentOK, modulecomposition.TerminalAxisKey},
		{originDenominatorOK, originContentOK, modulecomposition.ModuleExportCallableOriginAxisKey},
		{ingressDenominatorOK, ingressContentOK, modulecomposition.ModuleExportCallableIngressAxisKey},
	} {
		if !minted.denominator {
			return anadiag.AnalyzeDiagnosticCompositionFailureDenominator, minted.axis
		}
		if !minted.content {
			return anadiag.AnalyzeDiagnosticCompositionFailureContent, minted.axis
		}
	}
	importWrite, importMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.ResolvedImport](state.binding.SchemaBinding(), modulecomposition.ImportOutputKey, modulecomposition.ImportAxisKey)
	cacheWrite, cacheMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.CacheIngress](state.binding.SchemaBinding(), modulecomposition.CacheOutputKey, modulecomposition.CacheAxisKey)
	transitionWrite, transitionMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.ModuleCallTransition](state.binding.SchemaBinding(), modulecomposition.ModuleCallTransitionOutputKey, modulecomposition.ModuleCallTransitionAxisKey)
	generationWrite, generationMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.InitGeneration](state.binding.SchemaBinding(), modulecomposition.GenerationOutputKey, modulecomposition.GenerationAxisKey)
	outcomeWrite, outcomeMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.InitOutcome](state.binding.SchemaBinding(), modulecomposition.OutcomeOutputKey, modulecomposition.OutcomeAxisKey)
	stateEdgeWrite, stateEdgeMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.ModuleReturnStateEdge](state.binding.SchemaBinding(), modulecomposition.ModuleReturnStateEdgeOutputKey, modulecomposition.ModuleReturnStateEdgeAxisKey)
	terminalWrite, terminalMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.InitTerminal](state.binding.SchemaBinding(), modulecomposition.TerminalOutputKey, modulecomposition.TerminalAxisKey)
	originWrite, originMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.ModuleExportCallableOrigin](state.binding.SchemaBinding(), modulecomposition.ModuleExportCallableOriginOutputKey, modulecomposition.ModuleExportCallableOriginAxisKey)
	ingressWrite, ingressMinted := engine.MintColumnWrite[identity.ContentID, modulecomposition.ModuleExportCallableIngress](state.binding.SchemaBinding(), modulecomposition.ModuleExportCallableIngressOutputKey, modulecomposition.ModuleExportCallableIngressAxisKey)
	for _, grant := range []compositionGrantStep{
		{importMinted && importWrite.Available(), modulecomposition.ImportAxisKey},
		{cacheMinted && cacheWrite.Available(), modulecomposition.CacheAxisKey},
		{transitionMinted && transitionWrite.Available(), modulecomposition.ModuleCallTransitionAxisKey},
		{generationMinted && generationWrite.Available(), modulecomposition.GenerationAxisKey},
		{outcomeMinted && outcomeWrite.Available(), modulecomposition.OutcomeAxisKey},
		{stateEdgeMinted && stateEdgeWrite.Available(), modulecomposition.ModuleReturnStateEdgeAxisKey},
		{terminalMinted && terminalWrite.Available(), modulecomposition.TerminalAxisKey},
		{originMinted && originWrite.Available(), modulecomposition.ModuleExportCallableOriginAxisKey},
		{ingressMinted && ingressWrite.Available(), modulecomposition.ModuleExportCallableIngressAxisKey},
	} {
		if !grant.granted {
			return anadiag.AnalyzeDiagnosticCompositionFailureColumnGrant, grant.axis
		}
	}
	// Effect's publication directory is the receipts it admitted on this Link,
	// detached from the algebra that sealed them. It is published here because
	// admission is complete at binding and no rule revises it.
	effects := state.binding.EffectAuthority()
	declaredVocabulary, vocabularyOK := state.compilation.Structure()
	if effects == nil || !vocabularyOK {
		return anadiag.AnalyzeDiagnosticCompositionFailureInput, effectpublication.AxisKey
	}
	publicationDirectory, publicationDirectoryOK := effectfactor.DetachPublications(effects.Algebra(), state.binding.ValueSchema())
	if !publicationDirectoryOK {
		return anadiag.AnalyzeDiagnosticCompositionFailureRows, effectpublication.AxisKey
	}
	publicationDenominator, publicationDenominatorOK := effectpublication.DenominatorID(state.sourceID, publicationDirectory.Rows)
	callsDenominator, callsDenominatorOK := effectpublication.CallsDenominatorID(state.sourceID, publicationDirectory.Calls)
	membersDenominator, membersDenominatorOK := effectpublication.MembersDenominatorID(state.sourceID, publicationDirectory.MemberRows())
	if !publicationDenominatorOK || !callsDenominatorOK || !membersDenominatorOK {
		return anadiag.AnalyzeDiagnosticCompositionFailureDenominator, effectpublication.AxisKey
	}
	publicationContent, publicationContentOK := effectpublication.Content(publicationDirectory.Rows, publicationDenominator, declaredVocabulary)
	callsContent, callsContentOK := effectpublication.CallsContent(publicationDirectory.Calls, len(publicationDirectory.Rows), callsDenominator)
	membersContent, membersContentOK := effectpublication.MembersContent(publicationDirectory.MemberRows(), publicationDirectory.Rows, membersDenominator)
	if !publicationContentOK || !callsContentOK || !membersContentOK {
		return anadiag.AnalyzeDiagnosticCompositionFailureContent, effectpublication.AxisKey
	}
	publicationWrite, publicationMinted := engine.MintColumnWrite[identity.ContentID, effectfactor.PublicationRow](state.binding.SchemaBinding(), effectpublication.OutputKey, effectpublication.AxisKey)
	callsWrite, callsMinted := engine.MintColumnWrite[identity.ContentID, effectfactor.PublicationCallRow](state.binding.SchemaBinding(), effectpublication.CallsOutputKey, effectpublication.AxisKey)
	membersWrite, membersMinted := engine.MintColumnWrite[identity.ContentID, effectfactor.PublicationMemberRow](state.binding.SchemaBinding(), effectpublication.MembersOutputKey, effectpublication.AxisKey)
	if !publicationMinted || !publicationWrite.Available() || !callsMinted || !callsWrite.Available() || !membersMinted || !membersWrite.Available() {
		return anadiag.AnalyzeDiagnosticCompositionFailureColumnGrant, effectpublication.AxisKey
	}
	builder := snapshot.NewBuilder(schemaID, store, identity.Generation(1))
	if err := selectapply.Publish(write, &builder, apps); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, selectapply.AxisKey
	}
	if err := engine.PublishColumn(mountWrite, &builder, directory); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, programmount.AxisKey
	}
	if err := engine.PublishColumn(importWrite, &builder, importContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.ImportAxisKey
	}
	if err := engine.PublishColumn(cacheWrite, &builder, cacheContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.CacheAxisKey
	}
	if err := engine.PublishColumn(transitionWrite, &builder, transitionContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.ModuleCallTransitionAxisKey
	}
	if err := engine.PublishColumn(generationWrite, &builder, generationContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.GenerationAxisKey
	}
	if err := engine.PublishColumn(outcomeWrite, &builder, outcomeContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.OutcomeAxisKey
	}
	if err := engine.PublishColumn(stateEdgeWrite, &builder, stateEdgeContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.ModuleReturnStateEdgeAxisKey
	}
	if err := engine.PublishColumn(terminalWrite, &builder, terminalContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.TerminalAxisKey
	}
	if err := engine.PublishColumn(originWrite, &builder, originContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.ModuleExportCallableOriginAxisKey
	}
	if err := engine.PublishColumn(ingressWrite, &builder, ingressContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, modulecomposition.ModuleExportCallableIngressAxisKey
	}
	if err := engine.PublishColumn(publicationWrite, &builder, publicationContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, effectpublication.AxisKey
	}
	if err := engine.PublishColumn(membersWrite, &builder, membersContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, effectpublication.AxisKey
	}
	if err := engine.PublishColumn(callsWrite, &builder, callsContent); err != nil {
		return anadiag.AnalyzeDiagnosticCompositionFailureWrite, effectpublication.AxisKey
	}
	sealed, err := builder.Seal()
	if err != nil || !sealed.Published() {
		return anadiag.AnalyzeDiagnosticCompositionFailureSeal, ""
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
			return anadiag.AnalyzeDiagnosticCompositionFailureSelectSite, selectapply.AxisKey
		}
		sites[index] = anadiag.SelectSite{Site: app.Site, Bound: bound}
	}
	state.composition = sealed
	state.selectColumn = selectColumn
	state.selectSites = sites
	state.selectHandlers = handlers
	return anadiag.AnalyzeDiagnosticCompositionFailureNone, ""
}

// compositionRowStep and compositionGrantStep pair one composition column's
// predicate with the key that names it, so a refusal reports the column it is
// about rather than the whole family.
type compositionRowStep struct {
	denominator bool
	content     bool
	axis        schema.Key
}

type compositionGrantStep struct {
	granted bool
	axis    schema.Key
}

type compiledArtifactSet struct {
	mounts             []programmount.MountedArtifact
	products           map[identity.ContentID]analysisworkspace.ArtifactProduct
	sites              mounted.ObservationSites
	selectApplications []selectapply.Application
	selectHandlers     []selectapply.Handler
	// types is the one immutable Type Authority issued from the canonical
	// mounted Program rows. Static binding and diagnostic projection consume
	// this product; neither consumer may reseal the same graph.
	types *typeauthority.Authority
	// declared is the declared-type column of the sealed conformance sites.
	// It is derived once here because this is the first place holding both the
	// artifact type rows and the sites measured against them.
	declared anadiag.DeclaredTypes
}

func (artifacts *compiledArtifactSet) observationCensus() ([]anadiag.Observation, bool) {
	if artifacts == nil {
		return nil, false
	}
	return anadiag.ProjectSites(artifacts.sites, artifacts.mounts, artifacts.declared)
}

// sealDeclaredConformanceTypes publishes the declared-type column every
// conformance site is judged against: the runtime families its declaration
// admits and the spelling that names it.
//
// The projection belongs here because this is the first place that holds both
// the sealed artifact type rows and the sites measured against them. The
// snapshot a downstream reader holds carries a static node's kind alone, so
// nothing downstream can decide what a record, array, optional, or union
// declaration admits; publishing the projection here is what keeps that
// judgment from collapsing every non-primitive declaration to the whole
// runtime vocabulary.
func (artifacts *compiledArtifactSet) sealDeclaredConformanceTypes() bool {
	if artifacts == nil || artifacts.types == nil || !artifacts.sites.Available() || len(artifacts.products) == 0 {
		return false
	}
	snapshots := make(map[identity.ContentID]*ingress.Snapshot, len(artifacts.mounts))
	for _, mount := range artifacts.mounts {
		if !mount.Available() {
			return false
		}
		snapshots[mount.ModuleKey] = mount.Snapshot
	}
	declared := make(anadiag.DeclaredTypes)
	for index := 0; index < artifacts.sites.Count(); index++ {
		site, siteOK := artifacts.sites.At(index)
		if !siteOK || !site.Available() {
			return false
		}
		if site.Kind != structure.DiagnosticObservationTypeConformance {
			continue
		}
		snapshot, held := snapshots[site.Mount]
		if !held || snapshot == nil {
			return false
		}
		cold, coldOK := snapshot.Program().ColdState()
		view, viewOK := programdiagnostic.NewView(cold)
		observation, observed := view.DiagnosticObservationForID(site.Local)
		if !coldOK || !viewOK || !observed || observation.Kind() != structure.DiagnosticObservationTypeConformance {
			return false
		}
		declaredID := observation.DeclaredStaticTypeID()
		if !declaredID.Available() {
			return false
		}
		if _, projected := declared[declaredID]; projected {
			continue
		}
		row, rowOK := declaredTypeProjection(artifacts.types, declaredID)
		if !rowOK {
			return false
		}
		declared[declaredID] = row
	}
	artifacts.declared = declared
	return true
}

// declaredTypeProjection states one declaration in the vocabulary the
// conformance judgment reads. The type domain owns both halves: the artifact
// type authority resolves the declared graph, and Static projects the runtime
// families that graph admits.
//
// A declaration the authority cannot resolve is refused. The whole vocabulary
// is the abstention an unnarrowed declaration earns, and a site measured
// against it conforms for every observation; an unresolved declaration has
// earned no such abstention, so publishing that mask for it would prove
// conformance the declaration never stated.
func declaredTypeProjection(types *typeauthority.Authority, declared identity.ContentID) (anadiag.DeclaredType, bool) {
	projection, projected := types.ProjectionByReferenceID(declared)
	may, mayOK := projection.MayRuntimeKinds()
	name, nameOK := projection.Name()
	root, rootOK := projection.RootKind()
	if !projected || !mayOK || !nameOK || !rootOK {
		return anadiag.DeclaredType{}, false
	}
	spelling := root.String()
	if _, renderable := anadiag.NewTargetType(name); renderable {
		spelling = name
	}
	row := anadiag.DeclaredType{May: may, Spelling: spelling}
	return row, row.Available()
}

// compileProgramArtifacts compiles each distinct ProgramID once and records
// every mounted occurrence's exact Link substitution. No Link/domain/runtime
// authority enters the Workspace-owned artifact directory.
//
// A refusal raised by the artifact compiler travels back at the compiler's own
// evidence type, so the item-issuance phase can name the compile stage, row
// family, and row that stopped it. A refusal this join raises itself carries
// no compiler evidence: the absent failure states that every mounted Program
// compiled and the Link substitution is what refused.
func compileProgramArtifacts(products *analysisworkspace.Artifacts, source *link.Link, compilation composite.Compilation) (*compiledArtifactSet, artifactcompiler.CompileFailure, bool) {
	if products == nil || source == nil || !source.ContentID().Available() || !compilation.Available() || source.Project() == nil {
		return nil, artifactcompiler.CompileFailure{}, false
	}
	mounts := source.Project().Mounts()
	if mounts.Count() == 0 {
		return nil, artifactcompiler.CompileFailure{}, false
	}
	result := &compiledArtifactSet{mounts: make([]programmount.MountedArtifact, 0, mounts.Count()), products: make(map[identity.ContentID]analysisworkspace.ArtifactProduct)}
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mounted, programOK := mounts.Program(shard)
		moduleKey, moduleOK := source.Project().ModuleKey(shard)
		if !shardOK || !programOK || mounted == nil || !moduleOK || !moduleKey.Available() {
			return nil, artifactcompiler.CompileFailure{}, false
		}
		programID := mounted.ContentID()
		if !mounted.Available() || !programID.Available() {
			return nil, artifactcompiler.CompileFailure{}, false
		}
		applications := selectapply.Apply(mounted)
		result.selectApplications = append(result.selectApplications, applications...)
		result.selectHandlers = append(result.selectHandlers, selectapply.Handlers(mounted, applications)...)
		product, compileFailure, compiled := products.Compile(mounted, compilation)
		if !compiled {
			return nil, compileFailure, false
		}
		artifact, snapshot := product.Artifact, product.Snapshot
		if _, held := result.products[programID]; !held {
			result.products[programID] = product
		}
		compiledProgram := artifact.Program()
		catalog, catalogOK := programcatalog.CatalogID(compiledProgram.SchemaID)
		if !compiledProgram.Available() || !catalogOK || !catalog.Available() {
			return nil, artifactcompiler.CompileFailure{}, false
		}
		program := programmount.Program{
			ModuleKey: moduleKey,
			Program:   compiledProgram,
		}
		if !program.Available() {
			return nil, artifactcompiler.CompileFailure{}, false
		}
		mount := programmount.MountedArtifact{Program: program, Snapshot: snapshot}
		if !mount.Available() {
			return nil, artifactcompiler.CompileFailure{}, false
		}
		result.mounts = append(result.mounts, mount)
	}
	producerAxes, axesOK := composite.ProducedValueAxes(compilation)
	if !axesOK {
		return nil, artifactcompiler.CompileFailure{}, false
	}
	sites, sitesOK := mounted.SealObservationSites(source.Boundary(), result.mounts, producerAxes)
	if !sitesOK || !sites.Available() {
		return nil, artifactcompiler.CompileFailure{}, false
	}
	result.sites = sites
	programs := make([]programschema.Program, 0, len(result.products))
	for _, product := range result.products {
		if product.Artifact == nil || !product.Artifact.Available() {
			return nil, artifactcompiler.CompileFailure{}, false
		}
		compiled := product.Artifact.Program()
		if !compiled.Available() {
			return nil, artifactcompiler.CompileFailure{}, false
		}
		programs = append(programs, compiled)
	}
	qualified, qualifiedOK := linkQualifiedTypes(source)
	if !qualifiedOK {
		return nil, artifactcompiler.CompileFailure{}, false
	}
	types, typesErr := typeauthority.SealProgramRows(source.ContentID(), programs, qualified)
	if typesErr != nil || types == nil {
		return nil, artifactcompiler.CompileFailure{}, false
	}
	result.types = types
	if !result.sealDeclaredConformanceTypes() {
		return nil, artifactcompiler.CompileFailure{}, false
	}
	return result, artifactcompiler.CompileFailure{}, true
}

// linkQualifiedTypes reads the target's sealed qualified type index into the
// type domain. The target owns the names and their denominator and the type
// domain owns what a declaration means, so the index crosses into the type
// authority as finished types under their exact published names. A Link whose
// target publishes no qualified type yields none.
func linkQualifiedTypes(source *link.Link) ([]typeauthority.QualifiedType, bool) {
	if source == nil {
		return nil, false
	}
	target, targetOK := source.Boundary().Target()
	if !targetOK || target == nil {
		return nil, false
	}
	index := target.Types()
	if index.Count() == 0 {
		return nil, true
	}
	qualified := make([]typeauthority.QualifiedType, 0, index.Count())
	for position := 0; position < index.Count(); position++ {
		name, handle, rowOK := index.At(position)
		if !rowOK {
			return nil, false
		}
		declaration, declarationOK := index.Declaration(handle)
		if !declarationOK {
			return nil, false
		}
		value, valueOK := domaintypecontract.Reading()(declaration)
		if !valueOK {
			// A declaration the type domain cannot read is not a type this
			// Link can name. It is left out under its own name rather than
			// admitted as an unreadable row, so a reference to it refuses by
			// name at the authority.
			continue
		}
		qualified = append(qualified, typeauthority.QualifiedType{Name: name, Value: value})
	}
	return qualified, true
}
