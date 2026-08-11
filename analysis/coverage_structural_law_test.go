package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	valuecontract "github.com/wippyai/go-lua/analysis/domain/value/contract"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/semanticsource"
	"github.com/wippyai/go-lua/program/target"
	"github.com/wippyai/go-lua/program/target/profile"
)

// TestBranchLoopBreakAggregateRemainsStructural checks the aggregate control
// publication from a branch/loop/break body. The Value owner must not claim
// it; the sealed Flow child must own the structural treatment instead.
func TestBranchLoopBreakAggregateRemainsStructural(t *testing.T) {
	linked := directFieldHostileLink(t, `if true then while true do break end end; return 1`)
	contracts, plans, ok := programStructuralCoverage(linked)
	if !ok {
		t.Fatal("sealed owner structural coverage unavailable")
	}
	controlDefinition, definitionOK := semanticsource.Definition(semanticsource.OriginProgramFlowControl, 0)
	if !definitionOK {
		t.Fatal("generated FlowControl definition unavailable")
	}
	controlFound := false
	for _, contract := range contracts {
		if contract.Source != controlDefinition.Token() {
			continue
		}
		controlFound = true
		if contract.Class != coverage.OwnerStructural || contract.AuthorityKind != coverage.StructuralAuthorityFlow || !contract.Authority.Available() {
			t.Fatalf("FlowControl escaped its sealed Flow owner: %#v", contract)
		}
	}
	if !controlFound {
		t.Fatal("FlowControl structural contract missing")
	}
	planFound := false
	for _, plan := range plans {
		for _, requirement := range plan.Covers {
			if requirement.Source == controlDefinition.Token() {
				planFound = true
				if requirement.Class != coverage.OwnerStructural || requirement.AuthorityKind != coverage.StructuralAuthorityFlow {
					t.Fatalf("FlowControl plan was not structural: %#v", requirement)
				}
			}
		}
	}
	if !planFound {
		t.Fatal("FlowControl structural treatment missing")
	}

	semantics, semanticsOK := newSemanticBundle(linked.ContentID())
	if !semanticsOK {
		t.Fatal("semantic bundle")
	}
	valuePlan, valueOK := valuecontract.BuildPlan(semantics.valueFactor, valuecontract.PlanBindings{
		Source: semantics.valueSourceRule.rule, RawGet: semantics.rawGetRule.rule,
		Allocation: semantics.valueAllocationRule.rule, Bootstrap: semantics.valueBootstrapRule.rule,
		Transfer: semantics.valueTransferRule.rule, Query: semantics.valueQuery,
	})
	if !valueOK {
		t.Fatal("Value plan")
	}
	for _, rule := range valuePlan.Rules {
		for _, requirement := range rule.Covers {
			if requirement.Source.Origin() == semanticsource.OriginProgramFlowControl {
				t.Fatalf("Value Rule claimed FlowControl: %#v", requirement)
			}
		}
	}
}

func TestProgramStructuralCoverageDeduplicatesDuplicateMounts(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "duplicate-structural.lua", Text: []byte(`return 1`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{
		Target: contract,
		Modules: []linkproject.Module{
			{Name: "first", Program: program},
			{Name: "second", Program: program},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	structuralContracts, plans, ok := programStructuralCoverage(linked)
	if !ok {
		t.Fatal("duplicate-mounted structural coverage unavailable")
	}
	countContracts := func(kind coverage.StructuralAuthorityKind) int {
		count := 0
		for _, declared := range structuralContracts {
			if declared.Class == coverage.OwnerStructural && declared.AuthorityKind == kind {
				count++
			}
		}
		return count
	}
	countPlans := func(kind coverage.StructuralAuthorityKind) int {
		count := 0
		for _, plan := range plans {
			if plan.AuthorityKind == kind {
				count++
			}
		}
		return count
	}
	if countContracts(coverage.StructuralAuthoritySource) != 2 || countPlans(coverage.StructuralAuthoritySource) != 1 ||
		countContracts(coverage.StructuralAuthorityFlow) != 1 || countPlans(coverage.StructuralAuthorityFlow) != 1 ||
		countContracts(coverage.StructuralAuthorityModule) != 1 || countPlans(coverage.StructuralAuthorityModule) != 1 {
		t.Fatalf("duplicate Program mounts were not authority-deduplicated: contracts=%d/%d/%d plans=%d/%d/%d",
			countContracts(coverage.StructuralAuthoritySource), countContracts(coverage.StructuralAuthorityFlow), countContracts(coverage.StructuralAuthorityModule),
			countPlans(coverage.StructuralAuthoritySource), countPlans(coverage.StructuralAuthorityFlow), countPlans(coverage.StructuralAuthorityModule))
	}

	// The Link denominator still counts both authored mounts. Structural
	// treatment identity is deduplicated independently of those cardinalities.
	receipt, receiptOK := program.SemanticSourceReceipt()
	views, viewsOK := receipt.Views()
	publications, publicationsErr := linked.SourcePublications()
	catalog, catalogOK := coverage.NewSourceCatalog(publications)
	if !receiptOK || !viewsOK || publicationsErr != nil || !catalogOK {
		t.Fatal("duplicate-mounted detached denominator")
	}
	source := views.Source()
	for index := 0; index < source.Count(); index++ {
		publication, publicationOK := source.At(index)
		if !publicationOK {
			t.Fatal("Program Source cursor")
		}
		for catalogIndex := 0; catalogIndex < catalog.Count(); catalogIndex++ {
			measure, measured := catalog.MeasureAt(catalogIndex)
			if measured && measure.Token() == publication.Definition().Token() {
				if measure.Count() != 2*publication.Count() {
					t.Fatalf("duplicate mount denominator count for %v = %d, want %d", publication.Definition().Token(), measure.Count(), 2*publication.Count())
				}
				break
			}
		}
	}
}

func TestProgramStructuralCoverageTracksCanonicalOwnerTokens(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "canonical-owner-tokens.lua", Text: []byte(`return 1`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "main", Program: program}},
	})
	if err != nil {
		t.Fatal(err)
	}
	structuralContracts, _, ok := programStructuralCoverage(linked)
	if !ok {
		t.Fatal("canonical owner structural coverage unavailable")
	}
	targetContract, targetOK := linked.Boundary().Target()
	targetReceipt, targetReceiptOK := targetContract.SemanticSourceReceipt()
	targetViews, targetViewsOK := targetReceipt.Views()
	if !targetOK || !targetReceiptOK || !targetViewsOK || targetViews.Operation().Count() == 0 {
		t.Fatal("positive Target owner rows unavailable")
	}

	expected := map[coverage.StructuralAuthorityKind]map[semanticsource.Token]struct{}{
		coverage.StructuralAuthorityTarget:     {},
		coverage.StructuralAuthorityLinkModule: {},
		coverage.StructuralAuthorityLinkStatic: {},
	}
	schema := semanticsource.CatalogSchema()
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined {
			t.Fatalf("canonical schema definition %d unavailable", index)
		}
		kind := coverage.StructuralAuthorityUnset
		switch {
		case isTargetSourceOrigin(definition.Token().Origin()):
			kind = coverage.StructuralAuthorityTarget
		case definition.Token().Origin() == semanticsource.OriginLinkModule:
			kind = coverage.StructuralAuthorityLinkModule
		case definition.Token().Origin() == semanticsource.OriginLinkStatic:
			kind = coverage.StructuralAuthorityLinkStatic
		}
		if kind != coverage.StructuralAuthorityUnset {
			expected[kind][definition.Token()] = struct{}{}
		}
	}

	observed := map[coverage.StructuralAuthorityKind]map[semanticsource.Token]struct{}{
		coverage.StructuralAuthorityTarget:     {},
		coverage.StructuralAuthorityLinkModule: {},
		coverage.StructuralAuthorityLinkStatic: {},
	}
	for _, declared := range structuralContracts {
		if declared.Class != coverage.OwnerStructural {
			continue
		}
		if _, tracked := observed[declared.AuthorityKind]; !tracked {
			continue
		}
		if _, duplicate := observed[declared.AuthorityKind][declared.Source]; duplicate {
			t.Fatalf("duplicate canonical owner token %v/%d", declared.Source, declared.AuthorityKind)
		}
		observed[declared.AuthorityKind][declared.Source] = struct{}{}
	}
	for kind, tokens := range expected {
		if len(observed[kind]) != len(tokens) {
			t.Fatalf("canonical owner token count kind %d = %d, want %d", kind, len(observed[kind]), len(tokens))
		}
		for token := range tokens {
			if _, found := observed[kind][token]; !found {
				t.Fatalf("canonical owner token %v/%d missing", token, kind)
			}
		}
	}
}

func TestProgramReceiptCursorTraversesWithoutPublicationSliceAllocation(t *testing.T) {
	linked := directFieldHostileLink(t, `return 1`)
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	receipt, receiptOK := program.SemanticSourceReceipt()
	if !shardOK || !programOK || !receiptOK || !receipt.Valid() {
		t.Fatal("Program receipt")
	}
	want := receipt.Count()
	observed := 0
	allocations := testing.AllocsPerRun(100, func() {
		cursor := receipt.Cursor()
		count := 0
		for {
			_, ok := cursor.Next()
			if !ok {
				break
			}
			count++
		}
		observed = count
	})
	if observed != want {
		t.Fatalf("receipt cursor count = %d, want %d", observed, want)
	}
	if allocations != 0 {
		t.Fatalf("receipt cursor allocated %f times; detached Publications materialized", allocations)
	}
}

// TestTypedOwnerViewsFenceForeignReceiptsAndTraverseWithoutPublicationAllocation
// keeps the three independent post-Program owners honest.  The coverage
// bridge consumes their generated typed views directly; it must never recover
// rows by asking each owner to rebuild a Publications slice or by accepting a
// receipt from an equal-looking foreign owner.
func TestTypedOwnerViewsFenceForeignReceiptsAndTraverseWithoutPublicationAllocation(t *testing.T) {
	linked := directFieldHostileLink(t, `return 1`)
	boundary := linked.Boundary()
	localTarget, targetOK := boundary.Target()
	localTargetReceipt, targetReceiptOK := localTarget.SemanticSourceReceipt()
	localTargetViews, targetViewsOK := localTargetReceipt.Views()
	module := linked.Module()
	localModuleReceipt, moduleReceiptOK := module.SemanticSourceReceipt()
	localModuleViews, moduleViewsOK := localModuleReceipt.Views()
	static := linked.Static()
	localStaticReceipt, staticReceiptOK := static.SemanticSourceReceipt()
	localStaticViews, staticViewsOK := localStaticReceipt.Views()
	if !targetOK || !targetReceiptOK || !targetViewsOK || !moduleReceiptOK || !moduleViewsOK || !staticReceiptOK || !staticViewsOK {
		t.Fatal("typed owner receipts unavailable")
	}
	if localTargetReceipt.OwnerID() != localTarget.ContentID() || localTargetViews.OwnerID() != localTarget.ContentID() {
		t.Fatal("Target view escaped its Contract owner")
	}
	if localModuleReceipt.OwnerID() != module.ContentID() || localModuleViews.OwnerID() != module.ContentID() {
		t.Fatal("LinkModule view escaped its Component owner")
	}
	if localStaticReceipt.OwnerID() != static.Cold().ContentID() || localStaticViews.OwnerID() != static.Cold().ContentID() {
		t.Fatal("LinkStatic view escaped its Component owner")
	}

	// A distinct sealed Target and Link provide equal-shaped but foreign
	// receipts. Each exact owner identity must remain different from the local
	// receipt rather than being accepted by ordinal or count alone.
	program, err := lower.Lower(lower.Source{Name: "foreign-owner.lua", Text: []byte(`return 1`)})
	if err != nil {
		t.Fatal(err)
	}
	foreignTarget, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	foreignLinked, err := link.Seal(&link.Spec{
		Target:  foreignTarget,
		Modules: []linkproject.Module{{Name: "main", Program: program}},
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignTargetContract, foreignTargetOK := foreignLinked.Boundary().Target()
	foreignTargetReceipt, foreignTargetReceiptOK := foreignTargetContract.SemanticSourceReceipt()
	foreignModuleReceipt, foreignModuleReceiptOK := foreignLinked.Module().SemanticSourceReceipt()
	foreignStaticReceipt, foreignStaticReceiptOK := foreignLinked.Static().SemanticSourceReceipt()
	if !foreignTargetOK || !foreignTargetReceiptOK || !foreignModuleReceiptOK || !foreignStaticReceiptOK {
		t.Fatal("foreign typed owner receipts unavailable")
	}
	if foreignTargetReceipt.OwnerID() == localTargetReceipt.OwnerID() || foreignModuleReceipt.OwnerID() == localModuleReceipt.OwnerID() || foreignStaticReceipt.OwnerID() == localStaticReceipt.OwnerID() {
		t.Fatal("foreign typed owner receipt crossed an owner identity fence")
	}
	foreignTargetViews, foreignTargetViewsOK := foreignTargetReceipt.Views()
	if !foreignTargetViewsOK || foreignTargetViews.OwnerID() == localTargetViews.OwnerID() {
		t.Fatal("foreign Target view crossed an owner identity fence")
	}

	allocations := testing.AllocsPerRun(100, func() {
		// These owner-local walkers use Count/DigestAt over the already detached
		// typed views. They intentionally do not call Publications or Digests.
		_ = walkTargetView(localTargetViews.Contract())
		_ = walkTargetView(localTargetViews.Operation())
		_ = walkTargetView(localTargetViews.ABI())
		_ = walkTargetView(localTargetViews.Subedge())
		_ = walkTargetView(localTargetViews.Callback())
		_ = walkTargetView(localTargetViews.Binding())
		_ = walkTargetView(localTargetViews.Resume())
		_ = walkTargetView(localTargetViews.Spawn())
		_ = walkTargetView(localTargetViews.Opaque())
		_ = walkTargetView(localTargetViews.OperationEffect())
		_ = walkTargetView(localTargetViews.CallbackEffect())
		_ = walkTargetView(localTargetViews.CallbackRelease())
		_ = walkTargetView(localTargetViews.Outcome())
		_ = walkTargetView(localTargetViews.Transfer())
		_ = walkTargetView(localTargetViews.TransferOutcome())
		_ = walkTargetView(localTargetViews.Suspension())
		_ = walkTargetView(localTargetViews.ResumeOutcome())
		_ = walkTargetView(localTargetViews.SpawnSibling())
		_ = walkTargetView(localTargetViews.SubedgeArgumentOrigin())
		_ = walkTargetView(localTargetViews.CallbackResult())
		_ = walkTargetView(localTargetViews.ResultAlias())
		_ = walkTargetView(localTargetViews.Produced())
		_ = walkTargetView(localTargetViews.ProducedCapture())
		_ = walkTargetView(localTargetViews.FreshResult())
		_ = walkTargetView(localTargetViews.Protocol())
		_ = walkTargetView(localTargetViews.ProtocolState())
		_ = walkTargetView(localTargetViews.ProtocolAcquisition())
		_ = walkTargetView(localTargetViews.ProtocolTransition())
		_ = walkTargetView(localTargetViews.ProtocolTransitionOutcome())
		_ = walkTargetView(localTargetViews.ProtocolEscape())
		_ = walkTargetView(localTargetViews.ProtocolCallbackHolder())
		_ = walkTargetView(localTargetViews.Boot())
		_ = walkTargetView(localTargetViews.BootEntry())
		_ = walkTargetView(localTargetViews.BootMetatableAttachment())
		_ = walkTargetView(localTargetViews.BootBinding())
		_ = walkTargetView(localTargetViews.Gsub())

		_ = walkModuleView(localModuleViews.Module())
		_ = walkModuleView(localModuleViews.Cache())
		_ = walkModuleView(localModuleViews.Representative())
		_ = walkModuleView(localModuleViews.Transport())
		_ = walkModuleView(localModuleViews.AnalysisRoot())
		_ = walkModuleView(localModuleViews.InitGeneration())
		_ = walkModuleView(localModuleViews.InitOutcome())
		_ = walkModuleView(localModuleViews.InitTerminal())

		_ = walkStaticView(localStaticViews.Static())
		_ = walkStaticView(localStaticViews.Resolution())
		_ = walkStaticView(localStaticViews.Expression())
		_ = walkStaticView(localStaticViews.Export())
		_ = walkStaticView(localStaticViews.Input())
	})
	if allocations != 0 {
		t.Fatalf("typed Target/LinkModule/LinkStatic traversal allocated %f times; Publications materialized", allocations)
	}
}
