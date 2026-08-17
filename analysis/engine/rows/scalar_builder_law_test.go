package rows

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestArtifactScalarTemplateConsumesPrivateBuilderStorageExactlyOnce pins the
// storage transfer: sealing moves the builder's row planes into the template
// rather than copying them, and every handle that shares the builder state is
// closed by that single transfer.
func TestArtifactScalarTemplateConsumesPrivateBuilderStorageExactlyOnce(t *testing.T) {
	spec, copyOfHandle := artifactScalarLawSpec(t)
	pointStorage := &spec.state.Points[0]
	memberStorage := &spec.state.Regions[0].Members[0]
	entryStorage := &spec.state.Bodies[0].Entry[0]
	template, templateOK := NewArtifactScalarTemplate(spec)
	if !templateOK || template == nil {
		t.Fatal("artifact scalar template")
	}
	if &template.points[0] != pointStorage || &template.regions[0].Members[0] != memberStorage || &template.bodies[0].Entry[0] != entryStorage {
		t.Fatal("artifact scalar template copied builder storage")
	}
	if _, ok := copyOfHandle.AddPoint(ArtifactScalarPoint{ID: artifactScalarLawID(7)}); ok || copyOfHandle.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: artifactScalarLawID(4)}) {
		t.Fatal("copied builder handle remained mutable after consumption")
	}
	if second, ok := NewArtifactScalarTemplate(copyOfHandle); ok || second != nil {
		t.Fatal("artifact scalar builder was consumed twice")
	}
}

// artifactScalarLawSpec fills one admissible builder and returns it together
// with a copied handle that shares the same private state.
func artifactScalarLawSpec(t testing.TB) (*ArtifactScalarSpec, *ArtifactScalarSpec) {
	t.Helper()
	artifactID, program, schema := artifactScalarLawID(1), artifactScalarLawID(2), artifactScalarLawID(3)
	pointID, regionID, bodyID := artifactScalarLawID(4), artifactScalarLawID(5), artifactScalarLawID(6)
	spec, ok := NewArtifactScalarSpec(artifactID, program, schema, ArtifactScalarCapacity{Points: 1, Regions: 1, Events: 3, Bodies: 1})
	if !ok || spec == nil {
		t.Fatal("artifact scalar builder")
	}
	handle := *spec
	copyOfHandle := &handle
	point, pointOK := spec.AddPoint(ArtifactScalarPoint{ID: pointID, Initial: true})
	region, regionOK := copyOfHandle.AddRegion(ArtifactScalarRegion{ID: regionID, Head: pointID})
	body, bodyOK := spec.AddBody(ArtifactScalarBody{ID: bodyID})
	if !pointOK || point != 0 || !regionOK || region != 0 || !bodyOK || body != 0 ||
		!spec.AddRegionMember(region, pointID) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventEnter, Region: regionID}) ||
		!copyOfHandle.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventPoint, Point: pointID}) ||
		!spec.AddEvent(ArtifactScalarEvent{Kind: ArtifactEventExit, Region: regionID}) ||
		!spec.AddBodyEntry(body, pointID) || !spec.AddBodyExit(body, pointID) {
		t.Fatal("artifact scalar rows")
	}
	return spec, copyOfHandle
}

func artifactScalarLawID(value byte) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte{0xA5, value}))
}
