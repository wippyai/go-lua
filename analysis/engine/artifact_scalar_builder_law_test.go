package engine

import (
	"crypto/sha256"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestArtifactScalarReceiptRetainsSealedTemplate pins the mounted-input pair:
// the receipt seals over the exact sealed template it was bound from and
// republishes none of the template's row planes as its own.
func TestArtifactScalarReceiptRetainsSealedTemplate(t *testing.T) {
	artifactID, program, schema := artifactScalarLawID(1), artifactScalarLawID(2), artifactScalarLawID(3)
	pointID, regionID, bodyID := artifactScalarLawID(4), artifactScalarLawID(5), artifactScalarLawID(6)
	bodyContext, semanticEntry := artifactScalarLawID(8), artifactScalarLawID(9)
	functionID, callFormal := artifactScalarLawID(10), artifactScalarLawID(11)
	spec, ok := rows.NewArtifactScalarSpec(artifactID, program, schema, rows.ArtifactScalarCapacity{Points: 1, Regions: 1, Events: 3, Bodies: 1, Functions: 1})
	if !ok || spec == nil {
		t.Fatal("artifact scalar builder")
	}
	point, pointOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: pointID, Initial: true})
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: regionID, Head: pointID})
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: bodyID, Context: bodyContext, SemanticEntry: semanticEntry, Callable: true, Function: functionID, CallFormal: callFormal})
	function, functionOK := spec.AddFunction(rows.ArtifactScalarFunction{ID: functionID, Body: bodyID, BodyContext: bodyContext, Entry: semanticEntry, CallFormal: callFormal})
	if !pointOK || point != 0 || !regionOK || region != 0 || !bodyOK || body != 0 ||
		!functionOK || function != 0 ||
		!spec.AddRegionMember(region, pointID) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: regionID}) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: pointID}) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: regionID}) ||
		!spec.AddBodyEntry(body, pointID) || !spec.AddBodyExit(body, pointID) ||
		!spec.AddFunctionFormal(function, rows.ArtifactScalarFormalPort{ID: artifactScalarLawID(12), Cell: artifactScalarLawID(13), Storage: artifactScalarLawID(14)}) ||
		!spec.SetFunctionVararg(function, rows.ArtifactScalarVarargPort{ID: artifactScalarLawID(15), Cell: artifactScalarLawID(16)}) ||
		!spec.AddFunctionOutcome(function, artifactScalarLawID(17)) {
		t.Fatal("artifact scalar rows")
	}
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	binding, bindingOK := NewArtifactScalarBinding(template)
	receipt, receiptOK := NewArtifactScalarReceipt(binding)
	if !templateOK || !bindingOK || !receiptOK || receipt == nil || !receipt.sealed || receipt.template != template {
		t.Fatal("artifact scalar template/receipt")
	}
	sealedRegion, sealedRegionOK := template.RegionAt(0)
	sealedFunction, sealedFunctionOK := template.FunctionAt(0)
	if template.PointCount() != 1 || template.RegionCount() != 1 || !sealedRegionOK || len(sealedRegion.Members) != 1 ||
		template.FunctionCount() != 1 || !sealedFunctionOK || len(sealedFunction.Formals) != 1 {
		t.Fatal("artifact scalar template planes")
	}
}

func artifactScalarLawID(value byte) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte{0xA5, value}))
}
