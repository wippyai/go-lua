package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestRelationCoordinateFactorInventoryCarriesEarlierN2TargetIntoLaterBranch(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	graph := cfg.New()
	publisher := graph.AddNode(cfg.NodeAssign)
	branch := graph.AddBranch()
	graph.AddEdge(graph.Entry(), publisher, false)
	graph.AddEdge(publisher, branch, false)
	graph.AddEdge(branch, graph.Exit(), true)

	trigger, target := symbol.ID(9871), symbol.ID(9872)
	triggerPath := pathdom.NewPath(trigger, "trigger")
	targetPath := pathdom.NewPath(target, "target")
	builder := visibility.NewBuilder()
	triggerVersion := builder.Define(publisher, trigger, "trigger")
	targetVersion := builder.Define(publisher, target, "target")
	builder.SetVisible(branch, trigger, triggerVersion)
	builder.SetVisible(branch, target, targetVersion)
	authority := factapply.NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, nil)
	plan := operationplan.New(graph, factflow.FactsInput{
		PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{
			publisher: factflow.NewPathValuePresenceImplicationSet(
				factflow.NewPathValuePresenceImplication(
					triggerPath, typevalue.LiteralBool(reg, true), targetPath, presence.Present(),
				),
			),
		},
		BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
			branch: factflow.NewBranchPathEvidenceSet(factflow.NewBranchPathTruthyEvidenceOnEdge(triggerPath, true)),
		},
	})
	body := &relationProgramBody{
		graph: graph, plan: plan, productDomain: domain, pathSemantics: authority,
	}

	inventory, err := freezeRelationCoordinateFactorInventory(body)
	if err != nil {
		t.Fatal(err)
	}
	branchInventory, err := inventory.At(branch)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := authority.PrepareBranchRelationFactors(
		domain, factapply.PlanBranchRelationTransaction(plan.Facts(), branch, true), branchInventory,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertBranchPresenceConsequenceWritesValue(t, factors, statekey.SymbolValue(target))
}

func TestRelationCoordinateFactorInventoryCarriesEntrySeedTargetIntoBranchWithoutLocalN2(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	graph := cfg.New()
	branch := graph.AddBranch()
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, graph.Exit(), true)

	trigger, target := symbol.ID(9881), symbol.ID(9882)
	triggerPath := pathdom.NewPath(trigger, "trigger")
	targetPath := pathdom.NewPath(target, "target")
	builder := visibility.NewBuilder()
	builder.Define(branch, trigger, "trigger")
	builder.Define(branch, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	authority := factapply.NewPathSemanticAuthority(resolver, nil, nil)
	plan := operationplan.New(graph, factflow.FactsInput{
		BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
			branch: factflow.NewBranchPathEvidenceSet(factflow.NewBranchPathTruthyEvidenceOnEdge(triggerPath, true)),
		},
	})
	row := pathevidence.NewPathPresenceImplication(
		resolver.KeySpace().FromPath(triggerPath), presence.Present(),
		resolver.KeySpace().FromPath(targetPath), presence.Present(),
	)
	entry := state.Domain(reg).Bottom().AddPathPresenceImplication(row)
	if !entry.HasPathPresenceImplication(row) {
		t.Fatal("entry fixture did not retain its presence implication")
	}
	bodyID := lexicalidentity.StableLexicalBodyID{9, 8, 8, 2}
	body := &relationProgramBody{
		body: bodyID, graph: graph, plan: plan, productDomain: domain, pathSemantics: authority,
		initialStatePlan: testInitialStatePlan(t, bodyID, graph,
			state.NewInitialStateSeed(state.InitialCoordinate(graph.Entry()), entry),
		),
	}

	inventory, err := freezeRelationCoordinateFactorInventory(body)
	if err != nil {
		t.Fatal(err)
	}
	branchInventory, err := inventory.At(branch)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := authority.PrepareBranchRelationFactors(
		domain, factapply.PlanBranchRelationTransaction(plan.Facts(), branch, true), branchInventory,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertBranchPresenceConsequenceWritesValue(t, factors, statekey.SymbolValue(target))
}

func TestRelationCoordinateFactorInventoryUsesExactProducerReachability(t *testing.T) {
	for _, cycle := range []bool{false, true} {
		name := "acyclic-future-producer"
		if cycle {
			name = "cyclic-future-producer"
		}
		t.Run(name, func(t *testing.T) {
			reg := standard.Registry()
			domain := state.RegisteredProductDomain(reg)
			graph := cfg.New()
			branch := graph.AddBranch()
			publisher := graph.AddNode(cfg.NodeAssign)
			graph.AddEdge(graph.Entry(), branch, false)
			graph.AddEdge(branch, publisher, true)
			graph.AddEdge(branch, graph.Exit(), false)
			if cycle {
				graph.AddEdge(publisher, branch, false)
			} else {
				graph.AddEdge(publisher, graph.Exit(), false)
			}

			trigger, target := symbol.ID(9891), symbol.ID(9892)
			triggerPath := pathdom.NewPath(trigger, "trigger")
			targetPath := pathdom.NewPath(target, "target")
			builder := visibility.NewBuilder()
			triggerVersion := builder.Define(branch, trigger, "trigger")
			targetVersion := builder.Define(branch, target, "target")
			builder.SetVisible(publisher, trigger, triggerVersion)
			builder.SetVisible(publisher, target, targetVersion)
			authority := factapply.NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, nil)
			plan := operationplan.New(graph, factflow.FactsInput{
				PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{
					publisher: factflow.NewPathValuePresenceImplicationSet(
						factflow.NewPathValuePresenceImplication(
							triggerPath, typevalue.LiteralBool(reg, true), targetPath, presence.Present(),
						),
					),
				},
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(factflow.NewBranchPathTruthyEvidenceOnEdge(triggerPath, true)),
				},
			})
			body := &relationProgramBody{graph: graph, plan: plan, productDomain: domain, pathSemantics: authority}
			inventories, err := freezeRelationCoordinateFactorInventory(body)
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := inventories.At(branch)
			if err != nil {
				t.Fatal(err)
			}
			factors, err := authority.PrepareBranchRelationFactors(
				domain, factapply.PlanBranchRelationTransaction(plan.Facts(), branch, true), inventory,
			)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for index := 0; index < factors.Len(); index++ {
				_, found = factors.PresenceImplicationDependencyPlan(index)
				if found {
					break
				}
			}
			if found != cycle {
				t.Fatalf("future producer consequence present=%t, want cycle reachability %t", found, cycle)
			}
		})
	}
}

func assertBranchPresenceConsequenceWritesValue(t *testing.T, factors factapply.BranchRelationFactors, want statekey.Value) {
	t.Helper()
	foundConsequence := false
	for index := 0; index < factors.Len(); index++ {
		if _, present := factors.PresenceImplicationDependencyPlan(index); !present {
			continue
		}
		foundConsequence = true
		factor, present := factors.Factor(index)
		if !present {
			t.Fatalf("presence consequence factor %d is missing", index)
		}
		for _, write := range factor.ValueWrites() {
			if write == want {
				return
			}
		}
	}
	if !foundConsequence {
		t.Fatal("branch omitted its presence consequence factor")
	}
	t.Fatalf("branch presence consequence does not write %v", want)
}
