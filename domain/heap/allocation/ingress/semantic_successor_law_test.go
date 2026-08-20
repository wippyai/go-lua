package ingress_test

import (
	"context"
	"testing"

	analysis "github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	analysisresult "github.com/wippyai/go-lua/analysis/result"
	valuepublication "github.com/wippyai/go-lua/domain/value/publication"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// The zero-input ingress runs through the mounted solver and exposes a typed
// Value publication at the detached result boundary. WorldZero itself is an
// internal Heap witness, so this law checks only the public receipt shape.
func TestIngressReceiptPublishesReadableValueFamilyThroughMountedSolver(t *testing.T) {
	linked := ingressSuccessorLink(t, `return {}`)
	plan, compileStatus := analysis.Compile(linked)
	if compileStatus != analysis.CompileComplete || plan == nil {
		t.Fatalf("receipt compile=%v plan=%t", compileStatus, plan != nil)
	}
	defer plan.Close()
	result, solveStatus := plan.Solve(context.Background())
	if solveStatus != analysis.AnalyzeComplete || result == nil {
		t.Fatalf("mounted solver status=%v result=%t", solveStatus, result != nil)
	}
	body, bodyOK := result.BodyAt(0)
	if !bodyOK {
		t.Fatal("ingress selected body unavailable")
	}
	assertIngressValuePublicationReadable(t, result, body)
}

func assertIngressValuePublicationReadable(t testing.TB, input *analysisresult.Result, selected analysisresult.Body) {
	t.Helper()
	bodyID, bodyIDOK := selected.ID()
	if !bodyIDOK {
		t.Fatal("ingress selected body has no identity")
	}
	family, familyOK := valuepublication.Open(input)
	if !familyOK {
		t.Fatal("ingress value publication family unavailable")
	}
	if family.QueryCount() == 0 {
		t.Fatal("ingress value publication has no queries")
	}
	selectedBodyReferences := 0
	for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
		query, queryOK := family.QueryAt(queryIndex)
		if !queryOK {
			t.Fatalf("ingress value query[%d] unavailable", queryIndex)
		}
		status := query.Status()
		if status != analysisresult.QueryHit && status != analysisresult.QueryProvenAbsent {
			t.Fatalf("ingress value query[%d] status=%d", queryIndex, status)
		}
		matched := false
		for bodyIndex := 0; bodyIndex < query.BodyCount(); bodyIndex++ {
			queryBody, queryBodyOK := query.BodyAt(bodyIndex)
			if !queryBodyOK {
				t.Fatalf("ingress value query[%d] body[%d] unavailable", queryIndex, bodyIndex)
			}
			queryBodyID, queryBodyIDOK := queryBody.ID()
			if !queryBodyIDOK {
				t.Fatalf("ingress value query[%d] body[%d] has no identity", queryIndex, bodyIndex)
			}
			if queryBodyID == bodyID {
				matched = true
			}
		}
		if matched {
			selectedBodyReferences++
		}
		if status != analysisresult.QueryHit {
			continue
		}
		summary, summaryOK := query.Summary()
		if !summaryOK || !summary.Available() || !summary.LinkID().Available() || summary.CoordinateCount() == 0 {
			t.Fatalf("ingress value query[%d] summary unavailable or incomplete", queryIndex)
		}
		iterator := summary.Coordinates()
		coordinateCount := 0
		for {
			coordinate, coordinateOK := iterator.Next()
			if !coordinateOK {
				break
			}
			if !coordinate.Available() || !coordinate.ID().Available() {
				t.Fatalf("ingress value query[%d] coordinate unavailable or unidentified", queryIndex)
			}
			coordinateCount++
		}
		if coordinateCount != summary.CoordinateCount() {
			t.Fatalf("ingress value query[%d] coordinates=%d summary=%d", queryIndex, coordinateCount, summary.CoordinateCount())
		}
	}
	if selectedBodyReferences == 0 {
		t.Fatalf("ingress value publication does not reference selected body %s", bodyID.String())
	}
}

func ingressSuccessorLink(t testing.TB, text string) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "ingress_successor.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	return linked
}
