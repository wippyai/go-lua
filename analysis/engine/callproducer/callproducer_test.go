package callproducer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFromSiteProjectsOnlyNarrowProducerEvidence(t *testing.T) {
	calleePath := path.NewPath(symbol.ID(21), "svc").Field("call")
	receiverPath := path.NewPath(symbol.ID(21), "svc")
	targetPath := path.NewPath(symbol.ID(22), "out")
	targets := []factflow.CallResultTarget{
		factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, symbol.ID(22), targetPath),
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context:         factflow.CallSiteContextAssignmentSource,
		CalleeSymbol:    symbol.ID(21),
		CalleePath:      calleePath,
		ReceiverPath:    receiverPath,
		HasReceiverPath: true,
		MethodName:      "call",
		ExprIndex:       3,
		TypeArgs:        []factflow.TypeRef{factflow.TypeRef(1)},
		ResultTargets:   targets,
		Final:           true,
		Adjusted:        true,
	})

	producer := FromSite(site)
	if producer.CalleeSymbol() != site.CalleeSymbol() || !producer.CalleePath().Equal(site.CalleePath()) {
		t.Fatalf("producer callee = %v/%v, want %v/%v", producer.CalleeSymbol(), producer.CalleePath(), site.CalleeSymbol(), site.CalleePath())
	}
	gotTargets := producer.ResultTargets()
	if len(gotTargets) != 1 || gotTargets[0].Kind() != factflow.CallResultTargetLocalAssignment || !gotTargets[0].TargetPath().Equal(targetPath) {
		t.Fatalf("producer targets = %#v, want copied local assignment target", gotTargets)
	}
	gotTargets[0] = factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, path.Path{})
	if got := producer.ResultTargets(); got[0].Kind() != factflow.CallResultTargetLocalAssignment {
		t.Fatalf("projected producer exposed mutable result targets, got %v", got[0].Kind())
	}
}

func TestFromFactsDerivesProducerFromCanonicalCallSite(t *testing.T) {
	point := cfg.Point(30)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context:      factflow.CallSiteContextAssignmentSource,
		CalleeSymbol: symbol.ID(31),
		CalleePath:   path.NewPath(symbol.ID(31), "make"),
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, symbol.ID(32), path.NewPath(symbol.ID(32), "out")),
		},
	})
	input := factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: site,
		},
	}

	facts := factflow.NewFacts(input)
	input.CallSites[point] = factflow.NewCallSite(factflow.CallSiteConfig{
		Context:      factflow.CallSiteContextAssignmentSource,
		CalleeSymbol: symbol.ID(99),
		CalleePath:   path.NewPath(symbol.ID(99), "changed"),
	})

	got, ok := FromFacts(facts, point)
	if !ok {
		t.Fatal("call producer missing")
	}
	want := FromSite(site)
	if got.CalleeSymbol() != want.CalleeSymbol() || !got.CalleePath().Equal(want.CalleePath()) {
		t.Fatalf("producer callee = %v/%v, want %v/%v", got.CalleeSymbol(), got.CalleePath(), want.CalleeSymbol(), want.CalleePath())
	}
	gotTargets := got.ResultTargets()
	wantTargets := want.ResultTargets()
	if len(gotTargets) != 1 || len(wantTargets) != 1 || gotTargets[0].Kind() != wantTargets[0].Kind() || !gotTargets[0].TargetPath().Equal(wantTargets[0].TargetPath()) {
		t.Fatalf("producer targets = %#v, want %#v", gotTargets, wantTargets)
	}
}

func TestFromFactsKeepsProducerProjectionNarrow(t *testing.T) {
	points := map[string]cfg.Point{
		"statement":          cfg.Point(40),
		"condition":          cfg.Point(41),
		"iterator":           cfg.Point(42),
		"memberAssignment":   cfg.Point(43),
		"returnSource":       cfg.Point(44),
		"expressionProducer": cfg.Point(45),
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			points["statement"]: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: symbol.ID(50),
			}),
			points["condition"]: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextCondition,
				CalleeSymbol: symbol.ID(51),
			}),
			points["iterator"]: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextIteratorSource,
				CalleeSymbol: symbol.ID(52),
			}),
			points["memberAssignment"]: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextAssignmentSource,
				CalleeSymbol: symbol.ID(53),
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetOrdinaryAssignment, 0, 0, symbol.ID(54), path.NewPath(symbol.ID(54), "t").Field("x")),
				},
			}),
			points["returnSource"]: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextReturnSource,
				CalleeSymbol: symbol.ID(55),
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, path.Path{}),
				},
			}),
			points["expressionProducer"]: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextExpressionProducer,
				CalleeSymbol: symbol.ID(56),
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetExpression, factflow.NoValueSourceIndex, 0, 0, path.Path{}),
				},
			}),
		},
	})

	for _, name := range []string{"statement", "condition", "iterator"} {
		if Has(facts, points[name]) {
			t.Fatalf("%s call unexpectedly reported producer eligibility", name)
		}
		if _, ok := FromFacts(facts, points[name]); ok {
			t.Fatalf("%s call unexpectedly produced call-result producer evidence", name)
		}
	}
	memberProducer, ok := FromFacts(facts, points["memberAssignment"])
	if !ok {
		t.Fatal("member assignment call producer missing")
	}
	if targets := memberProducer.ResultTargets(); len(targets) != 0 {
		t.Fatalf("member assignment targets leaked into producer: %#v", targets)
	}
	returnProducer, ok := FromFacts(facts, points["returnSource"])
	if !ok {
		t.Fatal("return-source call producer missing")
	}
	if targets := returnProducer.ResultTargets(); len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetReturn {
		t.Fatalf("return-source producer targets = %#v, want one return target", targets)
	}
	expressionProducer, ok := FromFacts(facts, points["expressionProducer"])
	if !ok {
		t.Fatal("expression producer call missing")
	}
	if targets := expressionProducer.ResultTargets(); len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetExpression || targets[0].ResultIndex() != 0 {
		t.Fatalf("expression producer targets = %#v, want slot-zero expression target", targets)
	}
}

func TestFromFactsCopiesProducerProjection(t *testing.T) {
	point := cfg.Point(60)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextAssignmentSource,
				CalleeSymbol: symbol.ID(35),
				CalleePath:   path.NewPath(symbol.ID(35), "callee").Field("site"),
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, symbol.ID(33), path.NewPath(symbol.ID(33), "local")),
				},
			}),
		},
	})

	if _, ok := FromFacts(facts, cfg.Point(61)); ok {
		t.Fatal("missing call producer returned ok")
	}
	call, ok := FromFacts(facts, point)
	if !ok {
		t.Fatal("call producer missing")
	}
	callCalleePath := call.CalleePath()
	callCalleePath.Segments[0].Name = "mutated"
	callAgain, _ := FromFacts(facts, point)
	assertDirectField(t, callAgain.CalleePath(), "site")
	callTargets := call.ResultTargets()
	if len(callTargets) != 1 || callTargets[0].Kind() != factflow.CallResultTargetLocalAssignment {
		t.Fatalf("call targets = %#v", callTargets)
	}
	callTargets[0] = factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, symbol.ID(33), path.NewPath(symbol.ID(33), "changed"))
	if got := callAgain.ResultTargets(); got[0].Kind() != factflow.CallResultTargetLocalAssignment {
		t.Fatalf("call producer exposed mutable targets, got %v", got[0].Kind())
	}
}

func assertDirectField(t *testing.T, got path.Path, want string) {
	t.Helper()
	if len(got.Segments) != 1 || got.Segments[0].Name != want {
		t.Fatalf("path = %#v, want direct field %q", got, want)
	}
}
