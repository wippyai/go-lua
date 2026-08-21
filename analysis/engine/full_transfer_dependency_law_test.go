package engine

import "testing"

// TestArtifactFullTransferRemainsASchedulerDependency protects the distinction
// between a cross-point full transfer and an intra-point transport annotation.
// The target consumes the source environment, so source changes must wake it.
func TestArtifactFullTransferRemainsASchedulerDependency(t *testing.T) {
	const (
		targetPointID    = 998_610
		artifactIdentity = 998_619
		mountIdentity    = 998_621
	)
	fixture := newSelectedOverlayLawFixture(t)
	from, fromOK := fixture.graph.lookupPoint(fixture.triggerID)
	toID := mountedArtifactID("analysis/engine/artifact-point/v1", selectedOverlayLawID(mountIdentity), selectedOverlayLawID(artifactIdentity), selectedOverlayLawID(targetPointID))
	to, toOK := fixture.graph.lookupPoint(toID)
	if !fromOK || !toOK {
		t.Fatal("mounted transfer endpoints unavailable")
	}
	found := false
	for index := 0; index < fixture.graph.graph.EnvironmentOutgoingCount(from); index++ {
		edge, edgeOK := fixture.graph.graph.EnvironmentOutgoingAt(from, index)
		if !edgeOK || edge.Target().Key() != to.Key() {
			continue
		}
		found = true
		if edge.TransportOnly() {
			t.Fatal("cross-point Artifact full transfer was suppressed from scheduler dependencies")
		}
	}
	if !found {
		t.Fatal("mounted Artifact full transfer unavailable")
	}
}
