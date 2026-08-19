package engine

import (
	"crypto/sha256"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestArtifactScalarTemplateRetainsSealedPlanes pins the sealed template:
// row planes stay on the template and are not republished by the mount pair.
func TestArtifactScalarTemplateRetainsSealedPlanes(t *testing.T) {
	artifactID, program, schema := artifactScalarLawID(1), artifactScalarLawID(2), artifactScalarLawID(3)
	pointID, regionID, bodyID := artifactScalarLawID(4), artifactScalarLawID(5), artifactScalarLawID(6)
	spec, ok := rows.NewArtifactScalarSpec(artifactID, program, schema, rows.ArtifactScalarCapacity{Points: 1, Regions: 1, Events: 3, Bodies: 1})
	if !ok || spec == nil {
		t.Fatal("artifact scalar builder")
	}
	point, pointOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: pointID, Initial: true})
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: regionID, Head: pointID})
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: bodyID})
	if !pointOK || point != 0 || !regionOK || region != 0 || !bodyOK || body != 0 ||
		!spec.AddRegionMember(region, pointID) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: regionID}) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: pointID}) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: regionID}) ||
		!spec.AddBodyEntry(body, pointID) || !spec.AddBodyExit(body, pointID) {
		t.Fatal("artifact scalar rows")
	}
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	if !templateOK || template == nil {
		t.Fatal("artifact scalar template")
	}
	if _, ok := sealMountedProgramArtifacts([]MountedProgramArtifact{{Template: template, Module: artifactScalarLawID(7)}}); !ok {
		t.Fatal("zero-role template did not seal")
	}
	sealedRegion, sealedRegionOK := template.RegionAt(0)
	sealedBody, sealedBodyOK := template.BodyAt(0)
	if template.PointCount() != 1 || template.RegionCount() != 1 || !sealedRegionOK || len(sealedRegion.Members) != 1 ||
		template.BodyCount() != 1 || !sealedBodyOK || len(sealedBody.Entry) != 1 || len(sealedBody.Exits) != 1 {
		t.Fatal("artifact scalar template planes")
	}
}

func artifactScalarLawID(value byte) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte{0xA5, value}))
}
