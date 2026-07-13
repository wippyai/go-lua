package transformer

import (
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
)

func TestStructuralPredicatePurityClassificationIsExhaustive(t *testing.T) {
	for _, kind := range operationplan.Kinds() {
		if _, known := structuralPredicatePointKindPurity(kind); !known {
			t.Fatalf("operation kind %v is unclassified", kind)
		}
	}
	for _, kind := range []operationplan.Kind{operationplan.CallSite, operationplan.RootAssignment, operationplan.ObjectLiteral, operationplan.ChannelSelect} {
		if pure, _ := structuralPredicatePointKindPurity(kind); pure {
			t.Fatalf("executable kind %v classified pure", kind)
		}
	}
}

func TestReturnedPredicateRefinementCertificateRejectsForgedModesAndPaths(t *testing.T) {
	inner, ok := factflow.NewPathValueSource(pathdom.NewPath(11, "value").Key(), 0, 0, 0, factflow.ValueSourceShape{Final: true})
	if !ok {
		t.Fatal("path source rejected")
	}
	outer, ok := factflow.NewExpressionValueSource(1, 0, 0, 0, factflow.ValueSourceShape{Final: true})
	if !ok {
		t.Fatal("expression source rejected")
	}
	for _, test := range []struct {
		name       string
		refinement factflow.ExpressionRefinement
		paths      map[factflow.ExprRef]pathdom.Path
		want       string
	}{
		{name: "meet", refinement: factflow.NewExpressionRefinement(inner, product.Top()), want: "not a runtime cast"},
		{name: "declared contract", refinement: factflow.NewExpressionDeclaredContract(inner, product.Top()), want: "not a runtime cast"},
		{name: "unrelated path sidecar", refinement: factflow.NewExpressionRuntimeValidation(inner, product.Top()), paths: map[factflow.ExprRef]pathdom.Path{1: pathdom.NewPath(12, "other")}, want: "unrelated path"},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts := factflow.NewFacts(factflow.FactsInput{
				ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{1: test.refinement},
				ExpressionPaths:       test.paths,
			})
			ctx := planCompileContext{facts: facts, predicateRefinements: make(map[factflow.ExprRef]struct{})}
			err := validatePredicateSource(&ctx, outer, make(map[factflow.ExprRef]bool))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("certificate error = %v, want %q", err, test.want)
			}
		})
	}
}
