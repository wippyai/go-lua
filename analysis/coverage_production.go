package analysis

// This file is the one production bridge between the sealed source
// denominator and the already-declared analysis composition. It is cold-only:
// it calls each live Factor owner's explicit coverage contract and assigns every
// resulting requirement to one existing Rule or Query schema. It never streams
// facts, creates a copied IR, or registers a generic facet authority.

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	callcontract "github.com/wippyai/go-lua/analysis/domain/call/contract"
	effectcontract "github.com/wippyai/go-lua/analysis/domain/effect/contract"
	heapcontract "github.com/wippyai/go-lua/analysis/domain/heap/contract"
	packcontract "github.com/wippyai/go-lua/analysis/domain/pack/contract"
	valuecontract "github.com/wippyai/go-lua/analysis/domain/value/contract"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	"github.com/wippyai/go-lua/program/semanticsource"
	targetdomain "github.com/wippyai/go-lua/program/target"
)

type productionCoverage struct {
	ledger coverage.Ledger
	result coverage.Result
	valid  bool
}

// freezeProductionCoverage invokes the exact coverage fragment owned by each
// Factor that is actually declared in the production Composition. Domains
// without a declared Factor are not guessed or assigned an adapter here. Their
// source families remain subject to the exact closure checks below.
func freezeProductionCoverage(source *link.Link, sourcePublications semanticsource.Publications, semantics semanticBundle, composition *engine.Composition) productionCoverage {
	catalog, catalogOK := coverage.NewSourceCatalog(sourcePublications)
	if source == nil || !catalogOK || composition == nil || !semantics.available() {
		return productionCoverage{}
	}
	valuePlan, valueOK := valuecontract.BuildPlan(semantics.valueFactor, valuecontract.PlanBindings{
		Source: semantics.valueSourceRule.rule, Allocation: semantics.valueAllocationRule.rule,
		RawGet:    semantics.rawGetRule.rule,
		Bootstrap: semantics.valueBootstrapRule.rule, Transfer: semantics.valueTransferRule.rule,
		Query: semantics.valueQuery,
	})
	callPlan, callOK := callcontract.BuildPlan(semantics.callFactor, callcontract.PlanBindings{Dispatch: semantics.callDispatchRule.rule})
	heapPlan, heapOK := heapcontract.BuildPlan(semantics.heapFactor, heapcontract.PlanBindings{
		Ingress: semantics.heapIngressRule.rule, Closed: semantics.heapClosedRule.rule, Empty: semantics.heapEmptyRule.rule, RawSet: semantics.rawSetRule.rule,
		Bootstrap: semantics.heapBootstrapRule.rule,
	})
	packPlan, packOK := packcontract.BuildPlan(semantics.packFactor, packcontract.PlanBindings{Source: semantics.packSourceRule.rule})
	effectPlan, effectOK := effectcontract.BuildPlan(semantics.effectFactor, effectcontract.PlanBindings{
		Selected: semantics.effectSelectedRule.rule, Opaque: semantics.effectOpaqueRule.rule, Body: semantics.effectBodyRule.rule,
		Query: semantics.effectQuery,
	})
	if !valueOK || !callOK || !heapOK || !packOK || !effectOK {
		return productionCoverage{}
	}
	// The explicit domain plans carry the exact declared Rule/Query identities.
	// Freeze validates that every one is present in this sealed Composition and
	// that every Composition child is claimed; no independent inventory or
	// hardcoded Rule/Query cardinality is maintained here.
	contracts := make([]coverage.CoverageContract, 0)
	rules := make([]coverage.RulePlan, 0)
	queries := make([]coverage.QueryPlan, 0)
	appendPlan := func(planContracts []coverage.CoverageContract, planRules []coverage.RulePlan, planQueries []coverage.QueryPlan) {
		contracts = append(contracts, planContracts...)
		rules = append(rules, planRules...)
		queries = append(queries, planQueries...)
	}
	appendPlan(valuePlan.Contracts, valuePlan.Rules, valuePlan.Queries)
	appendPlan(callPlan.Contracts, callPlan.Rules, nil)
	appendPlan(heapPlan.Contracts, heapPlan.Rules, nil)
	appendPlan(packPlan.Contracts, packPlan.Rules, nil)
	appendPlan(effectPlan.Contracts, effectPlan.Rules, effectPlan.Queries)
	structuralContracts, structural, structuralOK := programStructuralCoverage(source)
	if !structuralOK {
		return productionCoverage{}
	}
	contracts = append(contracts, structuralContracts...)
	activationContracts, activationRules, activationOK := callActivationCoverage(source, semantics.callActivation)
	if !activationOK {
		return productionCoverage{}
	}
	contracts = append(contracts, activationContracts...)
	rules = append(rules, activationRules...)

	// The generated denominator is closed by its issuing structural owners
	// above; Factor plans add semantic interpretations only where a declared Rule
	// or Query can honestly discharge them.
	ledger, result := coverage.Freeze(catalog, contracts, rules, queries, structural, composition)
	return productionCoverage{ledger: ledger, result: result, valid: result.Valid()}
}

// callActivationCoverage claims the structural Call-to-body selector as an
// explicit Flow-owned treatment. It shares the Flow Call source token with
// Call's Factor dispatch contract, but remains a distinct structural owner and
// therefore cannot be mistaken for another Factor conclusion.
func callActivationCoverage(source *link.Link, semantic engine.SemanticKey) ([]coverage.CoverageContract, []coverage.RulePlan, bool) {
	if source == nil || source.Project() == nil || !semantic.Available() {
		return nil, nil, false
	}
	definition, defined := semanticsource.Definition(semanticsource.OriginProgramFlowCall, 0)
	if !defined {
		return nil, nil, false
	}
	seen := make(map[keyspace.ContentID]struct{})
	requirements := make([]coverage.Requirement, 0)
	mounts := source.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		programValue, programOK := mounts.Program(shard)
		if !shardOK || !programOK || programValue == nil {
			return nil, nil, false
		}
		receipt, receiptOK := programValue.SemanticSourceReceipt()
		if !receiptOK || !receipt.Valid() {
			return nil, nil, false
		}
		views, viewsOK := receipt.Views()
		flow := views.Flow()
		if !viewsOK || !flow.Valid() || !flow.ChildID().Available() {
			return nil, nil, false
		}
		if _, duplicate := seen[flow.ChildID()]; duplicate {
			continue
		}
		seen[flow.ChildID()] = struct{}{}
		requirements = append(requirements, coverage.Requirement{
			Source: definition.Token(), Class: coverage.OwnerStructural, Owner: semantic,
			Authority: flow.ChildID(), AuthorityKind: coverage.StructuralAuthorityFlow,
		})
	}
	requirements, requirementsOK := coverage.SealRequirements(requirements)
	if !requirementsOK || len(requirements) == 0 {
		return nil, nil, false
	}
	contracts := make([]coverage.CoverageContract, len(requirements))
	for index, requirement := range requirements {
		contracts[index] = coverage.CoverageContract{
			Source: requirement.Source, Class: requirement.Class, Owner: requirement.Owner,
			Conclusion: requirement.Conclusion, Authority: requirement.Authority, AuthorityKind: requirement.AuthorityKind,
		}
	}
	return contracts, []coverage.RulePlan{{Semantic: semantic, Covers: requirements}}, true
}

// programStructuralCoverage consumes the four named fragments of each sealed
// Program receipt plus the independent sealed Target, LinkModule, and
// LinkStatic owner receipts. Source provenance/order, Flow's primary control
// geometry, and Module's primary entry are topology authorities; they are not
// Value conclusions and never become Rule instances. Every emitted row keeps
// the exact owner ContentID and kind, including zero-count rows.
func programStructuralCoverage(source *link.Link) ([]coverage.CoverageContract, []coverage.StructuralPlan, bool) {
	if source == nil {
		return nil, nil, false
	}
	project := source.Project()
	if project == nil {
		return nil, nil, false
	}
	mounts := project.Mounts()
	if mounts.Count() == 0 {
		return nil, nil, false
	}
	structuralRequirements := make([]coverage.Requirement, 0, mounts.Count()*4)
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		program, programOK := mounts.Program(shard)
		if !shardOK || !programOK || program == nil {
			return nil, nil, false
		}
		receipt, receiptOK := program.SemanticSourceReceipt()
		if !receiptOK || !receipt.Valid() {
			return nil, nil, false
		}
		// Read the fixed detached quartet once. The owner-local views/cursors
		// retain zero-count rows without rebuilding a Publications slice for
		// every mounted occurrence of the same Program.
		views, viewsOK := receipt.Views()
		if !viewsOK {
			return nil, nil, false
		}
		sourceView := views.Source()
		flowView := views.Flow()
		moduleView := views.Module()
		if !sourceView.Valid() || !flowView.Valid() || !moduleView.Valid() ||
			sourceView.ProgramID() != receipt.ProgramID() || flowView.ProgramID() != receipt.ProgramID() || moduleView.ProgramID() != receipt.ProgramID() {
			return nil, nil, false
		}

		provenanceDefinition, provenanceFound := semanticsource.Definition(semanticsource.OriginProgramSourceProvenance, 0)
		orderDefinition, orderFound := semanticsource.Definition(semanticsource.OriginProgramSourceOrder, 0)
		if !provenanceFound || !orderFound || !sourceView.ChildID().Available() {
			return nil, nil, false
		}
		sourceRequirements := []coverage.Requirement{
			{Source: provenanceDefinition.Token(), Class: coverage.OwnerStructural, Authority: sourceView.ChildID(), AuthorityKind: coverage.StructuralAuthoritySource},
			{Source: orderDefinition.Token(), Class: coverage.OwnerStructural, Authority: sourceView.ChildID(), AuthorityKind: coverage.StructuralAuthoritySource},
		}
		sealedSource, sourceRequirementsOK := coverage.SealRequirements(sourceRequirements)
		if !sourceRequirementsOK {
			return nil, nil, false
		}
		structuralRequirements = append(structuralRequirements, sealedSource...)

		controlDefinition, controlFound := semanticsource.Definition(semanticsource.OriginProgramFlowControl, 0)
		if !controlFound || !flowView.ChildID().Available() {
			return nil, nil, false
		}
		controlRequirement := coverage.Requirement{Source: controlDefinition.Token(), Class: coverage.OwnerStructural, Authority: flowView.ChildID(), AuthorityKind: coverage.StructuralAuthorityFlow}
		sealedControl, controlRequirementsOK := coverage.SealRequirements([]coverage.Requirement{controlRequirement})
		if !controlRequirementsOK {
			return nil, nil, false
		}
		structuralRequirements = append(structuralRequirements, sealedControl...)

		entryDefinition, entryFound := semanticsource.Definition(semanticsource.OriginProgramModuleEntry, 0)
		if !entryFound || !moduleView.ChildID().Available() {
			return nil, nil, false
		}
		entryRequirement := coverage.Requirement{Source: entryDefinition.Token(), Class: coverage.OwnerStructural, Authority: moduleView.ChildID(), AuthorityKind: coverage.StructuralAuthorityModule}
		sealedEntry, entryRequirementsOK := coverage.SealRequirements([]coverage.Requirement{entryRequirement})
		if !entryRequirementsOK {
			return nil, nil, false
		}
		structuralRequirements = append(structuralRequirements, sealedEntry...)
	}

	// Target is an independent sealed owner. Walk the generated definitions and
	// the already-detached typed views directly; do not materialize a second
	// Publication slice just to recover each definition's count.
	boundary := source.Boundary()
	if boundary == nil {
		return nil, nil, false
	}
	contract, contractOK := boundary.Target()
	if !contractOK || contract == nil {
		return nil, nil, false
	}
	schema := semanticsource.CatalogSchema()
	targetReceipt, targetReceiptOK := contract.SemanticSourceReceipt()
	targetViews, targetViewsOK := targetReceipt.Views()
	if !targetReceiptOK || !targetViewsOK || !targetReceipt.Valid() || targetReceipt.OwnerID() != contract.ContentID() || targetViews.OwnerID() != contract.ContentID() {
		return nil, nil, false
	}
	targetRequirements := make([]coverage.Requirement, 0)
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined {
			return nil, nil, false
		}
		if !isTargetSourceOrigin(definition.Token().Origin()) {
			continue
		}
		view, viewOK := targetStructuralView(targetViews, definition.Token())
		if !viewOK || !walkTargetView(view) {
			return nil, nil, false
		}
		targetRequirements = append(targetRequirements, coverage.Requirement{
			Source: definition.Token(), Class: coverage.OwnerStructural,
			Authority: targetReceipt.OwnerID(), AuthorityKind: coverage.StructuralAuthorityTarget,
		})
	}
	sealedTarget, targetRequirementsOK := coverage.SealRequirements(targetRequirements)
	if !targetRequirementsOK {
		return nil, nil, false
	}
	structuralRequirements = append(structuralRequirements, sealedTarget...)

	// LinkModule and LinkStatic are likewise sealed owner receipts. Their
	// canonical zero-row publications remain structural claims; no Link root
	// aggregation or erased relation stream is introduced here.
	moduleComponent := source.Module()
	if moduleComponent == nil {
		return nil, nil, false
	}
	moduleReceipt, moduleReceiptOK := moduleComponent.SemanticSourceReceipt()
	moduleViews, moduleViewsOK := moduleReceipt.Views()
	moduleOwner := moduleComponent.ContentID()
	if !moduleReceiptOK || !moduleViewsOK || !moduleReceipt.Valid() || !moduleOwner.Available() || moduleReceipt.OwnerID() != moduleOwner || moduleViews.OwnerID() != moduleOwner {
		return nil, nil, false
	}
	moduleRequirements := make([]coverage.Requirement, 0)
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined {
			return nil, nil, false
		}
		if definition.Token().Origin() != semanticsource.OriginLinkModule {
			continue
		}
		view, viewOK := moduleStructuralView(moduleViews, definition.Token())
		if !viewOK || !walkModuleView(view) {
			return nil, nil, false
		}
		moduleRequirements = append(moduleRequirements, coverage.Requirement{
			Source: definition.Token(), Class: coverage.OwnerStructural,
			Authority: moduleReceipt.OwnerID(), AuthorityKind: coverage.StructuralAuthorityLinkModule,
		})
	}
	sealedLinkModule, linkModuleRequirementsOK := coverage.SealRequirements(moduleRequirements)
	if !linkModuleRequirementsOK {
		return nil, nil, false
	}
	structuralRequirements = append(structuralRequirements, sealedLinkModule...)

	staticComponent := source.Static()
	if staticComponent == nil {
		return nil, nil, false
	}
	staticReceipt, staticReceiptOK := staticComponent.SemanticSourceReceipt()
	staticViews, staticViewsOK := staticReceipt.Views()
	// Static exposes its sealed owner identity through the fenced receipt (it
	// deliberately has no root ContentID forwarding method). Keep the check on
	// that detached identity so coverage does not copy Cold's schema just to
	// recover an owner ID.
	staticOwner := staticReceipt.OwnerID()
	if !staticReceiptOK || !staticViewsOK || !staticReceipt.Valid() || !staticOwner.Available() || staticViews.OwnerID() != staticOwner {
		return nil, nil, false
	}
	staticRequirements := make([]coverage.Requirement, 0)
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined {
			return nil, nil, false
		}
		if definition.Token().Origin() != semanticsource.OriginLinkStatic {
			continue
		}
		view, viewOK := staticStructuralView(staticViews, definition.Token())
		if !viewOK || !walkStaticView(view) {
			return nil, nil, false
		}
		staticRequirements = append(staticRequirements, coverage.Requirement{
			Source: definition.Token(), Class: coverage.OwnerStructural,
			Authority: staticReceipt.OwnerID(), AuthorityKind: coverage.StructuralAuthorityLinkStatic,
		})
	}
	sealedStatic, staticRequirementsOK := coverage.SealRequirements(staticRequirements)
	if !staticRequirementsOK {
		return nil, nil, false
	}
	structuralRequirements = append(structuralRequirements, sealedStatic...)

	// Duplicate mounts preserve their source publication counts in the Link
	// denominator, but one exact owner publication must yield one structural
	// obligation. Canonicalize only by the owner-bound identity requested by
	// the source authority: token, owner ContentID, and owner kind.
	type structuralIdentity struct {
		source    semanticsource.Token
		authority keyspace.ContentID
		kind      coverage.StructuralAuthorityKind
	}
	type authorityIdentity struct {
		authority keyspace.ContentID
		kind      coverage.StructuralAuthorityKind
	}
	unique := make(map[structuralIdentity]coverage.Requirement, len(structuralRequirements))
	byAuthority := make(map[authorityIdentity][]coverage.Requirement)
	authorityOrder := make([]authorityIdentity, 0, len(structuralRequirements))
	for _, requirement := range structuralRequirements {
		identity := structuralIdentity{source: requirement.Source, authority: requirement.Authority, kind: requirement.AuthorityKind}
		if _, duplicate := unique[identity]; duplicate {
			continue
		}
		unique[identity] = requirement
		owner := authorityIdentity{authority: requirement.Authority, kind: requirement.AuthorityKind}
		if _, seen := byAuthority[owner]; !seen {
			authorityOrder = append(authorityOrder, owner)
		}
		byAuthority[owner] = append(byAuthority[owner], requirement)
	}
	contracts := make([]coverage.CoverageContract, 0, len(unique))
	plans := make([]coverage.StructuralPlan, 0, len(authorityOrder))
	for _, owner := range authorityOrder {
		sealed, sealedOK := coverage.SealRequirements(byAuthority[owner])
		if !sealedOK {
			return nil, nil, false
		}
		for _, requirement := range sealed {
			contracts = append(contracts, coverage.CoverageContract{
				Source: requirement.Source, Class: requirement.Class,
				Authority: requirement.Authority, AuthorityKind: requirement.AuthorityKind,
			})
		}
		plans = append(plans, coverage.StructuralPlan{
			Authority: owner.authority, AuthorityKind: owner.kind, Covers: sealed,
		})
	}
	return contracts, plans, true
}

func isTargetSourceOrigin(origin semanticsource.Origin) bool {
	switch origin {
	case semanticsource.OriginTargetContract, semanticsource.OriginTargetOperation,
		semanticsource.OriginTargetProtocol, semanticsource.OriginTargetBoot, semanticsource.OriginTargetGsub:
		return true
	default:
		return false
	}
}

func targetStructuralView(views targetdomain.SemanticSourceViews, token semanticsource.Token) (targetdomain.SemanticSourceView, bool) {
	switch token.Origin() {
	case semanticsource.OriginTargetContract:
		if token.Facet() == 0 {
			return views.Contract(), true
		}
	case semanticsource.OriginTargetOperation:
		switch token.Facet() {
		case 0:
			return views.Operation(), true
		case semanticsource.FacetTargetABI:
			return views.ABI(), true
		case semanticsource.FacetTargetSubedge:
			return views.Subedge(), true
		case semanticsource.FacetTargetCallback:
			return views.Callback(), true
		case semanticsource.FacetTargetBinding:
			return views.Binding(), true
		case semanticsource.FacetTargetResume:
			return views.Resume(), true
		case semanticsource.FacetTargetSpawn:
			return views.Spawn(), true
		case semanticsource.FacetTargetOpaque:
			return views.Opaque(), true
		case semanticsource.FacetTargetOperationEffect:
			return views.OperationEffect(), true
		case semanticsource.FacetTargetCallbackEffect:
			return views.CallbackEffect(), true
		case semanticsource.FacetTargetCallbackRelease:
			return views.CallbackRelease(), true
		case semanticsource.FacetTargetOutcome:
			return views.Outcome(), true
		case semanticsource.FacetTargetTransfer:
			return views.Transfer(), true
		case semanticsource.FacetTargetTransferOutcome:
			return views.TransferOutcome(), true
		case semanticsource.FacetTargetSuspension:
			return views.Suspension(), true
		case semanticsource.FacetTargetResumeOutcome:
			return views.ResumeOutcome(), true
		case semanticsource.FacetTargetSpawnSibling:
			return views.SpawnSibling(), true
		case semanticsource.FacetTargetSubedgeArgumentOrigin:
			return views.SubedgeArgumentOrigin(), true
		case semanticsource.FacetTargetCallbackResult:
			return views.CallbackResult(), true
		case semanticsource.FacetTargetResultAlias:
			return views.ResultAlias(), true
		case semanticsource.FacetTargetProduced:
			return views.Produced(), true
		case semanticsource.FacetTargetProducedCapture:
			return views.ProducedCapture(), true
		case semanticsource.FacetTargetFreshResult:
			return views.FreshResult(), true
		}
	case semanticsource.OriginTargetProtocol:
		switch token.Facet() {
		case 0:
			return views.Protocol(), true
		case semanticsource.FacetTargetProtocolState:
			return views.ProtocolState(), true
		case semanticsource.FacetTargetProtocolAcquisition:
			return views.ProtocolAcquisition(), true
		case semanticsource.FacetTargetProtocolTransition:
			return views.ProtocolTransition(), true
		case semanticsource.FacetTargetProtocolTransitionOutcome:
			return views.ProtocolTransitionOutcome(), true
		case semanticsource.FacetTargetProtocolEscape:
			return views.ProtocolEscape(), true
		case semanticsource.FacetTargetProtocolCallbackHolder:
			return views.ProtocolCallbackHolder(), true
		}
	case semanticsource.OriginTargetBoot:
		switch token.Facet() {
		case 0:
			return views.Boot(), true
		case semanticsource.FacetTargetBootEntry:
			return views.BootEntry(), true
		case semanticsource.FacetTargetBootMetatableAttachment:
			return views.BootMetatableAttachment(), true
		case semanticsource.FacetTargetBootBinding:
			return views.BootBinding(), true
		}
	case semanticsource.OriginTargetGsub:
		if token.Facet() == 0 {
			return views.Gsub(), true
		}
	}
	return targetdomain.SemanticSourceView{}, false
}

func walkTargetView(view targetdomain.SemanticSourceView) bool {
	count := view.Count()
	for index := 0; index < count; index++ {
		id, ok := view.DigestAt(index)
		if !ok || !id.Available() {
			return false
		}
	}
	_, beyond := view.DigestAt(count)
	return !beyond
}

func moduleStructuralView(views linkmodule.SemanticSourceViews, token semanticsource.Token) (linkmodule.SemanticSourceView, bool) {
	if token.Origin() != semanticsource.OriginLinkModule {
		return linkmodule.SemanticSourceView{}, false
	}
	switch token.Facet() {
	case 0:
		return views.Module(), true
	case semanticsource.FacetLinkModuleCache:
		return views.Cache(), true
	case semanticsource.FacetLinkModuleRepresentative:
		return views.Representative(), true
	case semanticsource.FacetLinkModuleTransport:
		return views.Transport(), true
	case semanticsource.FacetLinkModuleAnalysisRoot:
		return views.AnalysisRoot(), true
	case semanticsource.FacetLinkModuleInitGeneration:
		return views.InitGeneration(), true
	case semanticsource.FacetLinkModuleInitOutcome:
		return views.InitOutcome(), true
	case semanticsource.FacetLinkModuleInitTerminal:
		return views.InitTerminal(), true
	default:
		return linkmodule.SemanticSourceView{}, false
	}
}

func walkModuleView(view linkmodule.SemanticSourceView) bool {
	count := view.Count()
	for index := 0; index < count; index++ {
		id, ok := view.DigestAt(index)
		if !ok || !id.Available() {
			return false
		}
	}
	_, beyond := view.DigestAt(count)
	return !beyond
}

func staticStructuralView(views linkstatic.SemanticSourceViews, token semanticsource.Token) (linkstatic.SemanticSourceView, bool) {
	if token.Origin() != semanticsource.OriginLinkStatic {
		return linkstatic.SemanticSourceView{}, false
	}
	switch token.Facet() {
	case 0:
		return views.Static(), true
	case semanticsource.FacetLinkStaticResolution:
		return views.Resolution(), true
	case semanticsource.FacetLinkStaticExpression:
		return views.Expression(), true
	case semanticsource.FacetLinkStaticExport:
		return views.Export(), true
	case semanticsource.FacetLinkStaticInput:
		return views.Input(), true
	default:
		return linkstatic.SemanticSourceView{}, false
	}
}

func walkStaticView(view linkstatic.SemanticSourceView) bool {
	count := view.Count()
	for index := 0; index < count; index++ {
		id, ok := view.DigestAt(index)
		if !ok || !id.Available() {
			return false
		}
	}
	_, beyond := view.DigestAt(count)
	return !beyond
}
