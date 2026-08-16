package engine

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestArtifactScalarReceiptConsumesPrivateBuilderStorageExactlyOnce(t *testing.T) {
	artifact, program, schema := artifactScalarLawID(1), artifactScalarLawID(2), artifactScalarLawID(3)
	pointID, regionID, bodyID := artifactScalarLawID(4), artifactScalarLawID(5), artifactScalarLawID(6)
	bodyContext, semanticEntry := artifactScalarLawID(8), artifactScalarLawID(9)
	functionID, callFormal := artifactScalarLawID(10), artifactScalarLawID(11)
	spec, ok := NewArtifactScalarSpec(artifact, program, schema, ArtifactScalarCapacity{Points: 1, Regions: 1, Events: 3, Bodies: 1, Functions: 1})
	if !ok || spec == nil {
		t.Fatal("artifact scalar builder")
	}
	copyOfHandle := *spec
	point, pointOK := spec.AddPoint(ArtifactScalarPoint{ID: pointID, Initial: true})
	region, regionOK := copyOfHandle.AddRegion(ArtifactScalarRegion{ID: regionID, Head: pointID})
	body, bodyOK := spec.AddBody(ArtifactScalarBody{ID: bodyID, Context: bodyContext, SemanticEntry: semanticEntry, Callable: true, Function: functionID, CallFormal: callFormal})
	function, functionOK := spec.AddFunction(ArtifactScalarFunction{ID: functionID, Body: bodyID, BodyContext: bodyContext, Entry: semanticEntry, CallFormal: callFormal})
	if !pointOK || point != 0 || !regionOK || region != 0 || !bodyOK || body != 0 ||
		!functionOK || function != 0 ||
		!spec.AddRegionMember(region, pointID) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventEnter, Region: regionID}) ||
		!copyOfHandle.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: pointID}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventExit, Region: regionID}) ||
		!spec.AddBodyEntry(body, pointID) || !spec.AddBodyExit(body, pointID) ||
		!spec.AddFunctionFormal(function, ArtifactScalarFormalPort{ID: artifactScalarLawID(12), Cell: artifactScalarLawID(13), Storage: artifactScalarLawID(14)}) ||
		!spec.SetFunctionVararg(function, ArtifactScalarVarargPort{ID: artifactScalarLawID(15), Cell: artifactScalarLawID(16)}) ||
		!spec.AddFunctionOutcome(function, artifactScalarLawID(17)) {
		t.Fatal("artifact scalar rows")
	}
	pointStorage := &spec.state.Points[0]
	memberStorage := &spec.state.Regions[0].Members[0]
	formalStorage := &spec.state.Functions[0].Formals[0]
	template, templateOK := NewArtifactScalarTemplate(spec)
	binding, bindingOK := NewArtifactScalarBinding(template)
	receipt, receiptOK := NewArtifactScalarReceipt(binding)
	if !templateOK || !bindingOK || !receiptOK || receipt == nil || !receipt.sealed || receipt.template != template || len(template.points) != 1 || len(template.regions) != 1 || len(template.regions[0].Members) != 1 || template.FunctionCount() != 1 || len(template.functions[0].Formals) != 1 {
		t.Fatal("artifact scalar template/receipt")
	}
	if &template.points[0] != pointStorage || &template.regions[0].Members[0] != memberStorage || &template.functions[0].Formals[0] != formalStorage {
		t.Fatal("artifact scalar template copied builder storage")
	}
	if _, ok := copyOfHandle.AddPoint(ArtifactScalarPoint{ID: artifactScalarLawID(7)}); ok || copyOfHandle.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: pointID}) {
		t.Fatal("copied builder handle remained mutable after consumption")
	}
	if second, ok := NewArtifactScalarTemplate(&copyOfHandle); ok || second != nil {
		t.Fatal("artifact scalar builder was consumed twice")
	}
}

func artifactScalarLawID(value byte) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte{0xA5, value}))
}
