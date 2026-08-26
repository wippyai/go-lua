package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/sendsafety"
)

// appendNativeSendSafetyRows detaches already-proved send decisions. Program
// is consulted only to attach its canonical artifact/body provenance; no send
// verdict is recomputed here.
func appendNativeSendSafetyRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, geometry Geometry, mounts []programmount.MountedArtifact, decisions []sendsafety.Decision) bool {
	if rows == nil || seen == nil || !geometry.Valid() || len(mounts) == 0 {
		return false
	}
	byMount := make(map[identity.ContentID]programmount.MountedArtifact, len(mounts))
	for _, mount := range mounts {
		if !mount.Available() {
			return false
		}
		if _, duplicate := byMount[mount.ModuleKey]; duplicate {
			return false
		}
		byMount[mount.ModuleKey] = mount
	}
	for _, decision := range decisions {
		mount, mounted := byMount[decision.Mount]
		if !decision.Available() || !mounted {
			return false
		}
		occurrence, occurrenceOK := mount.Program.OccurrenceForID(programschema.OccurrenceCall, decision.Occurrence)
		body, bodyOK := occurrence.BodyID()
		call, callOK := mount.Program.CallForID(decision.Occurrence)
		span := call.SpanID()
		artifact := mount.Snapshot.ArtifactID()
		bodyIndex, geometryBodyOK := nativeSendBodyIndex(geometry, decision.Mount, body)
		if !occurrenceOK || !bodyOK || !callOK || call.BodyID() != body || !span.Available() || !artifact.Available() || !geometryBodyOK ||
			bodyIndex < 0 || bodyIndex >= len(geometry.bodies) {
			return false
		}
		semantic, semanticOK := nativePublicationFamilySendSafety.semanticID()
		evidence, evidenceOK := nativeEvidencePoints(decision.Point)
		content := nativePublicationContent{sendSafety: decision.Verdict, points: evidence}
		if !semanticOK || !evidenceOK || !content.valid(nativePublicationFamilySendSafety) {
			return false
		}
		row := nativePublicationRow{
			semantic: semantic,
			lane:     NativePublicationLaneSend, kind: NativePublicationKindSendSafety,
			family: nativePublicationFamilySendSafety, trust: NativePublicationTrustProven,
			key:    nativePublicationFamilySendSafety.String() + "/" + decision.Publication.String() + "/" + decision.Allocation.String() + "/" + decision.Context.String(),
			module: decision.Mount.String(), term: decision.Allocation.String(), subject: decision.Publication.String(), occurrence: decision.Occurrence.String(),
			content:      content,
			provenance:   NativePublicationProvenance{context: decision.Context, mount: decision.Mount, artifact: artifact, local: decision.Publication, body: body, point: decision.Point, span: span},
			provenanceOK: true, validityOK: true,
		}
		if !appendNativePublicationRow(rows, seen, row) {
			return false
		}
	}
	return true
}

// nativeSendBodyIndex joins the Program-issued call body to the detached
// Geometry body directory. A send decision's Point is the pre-effect input
// coordinate; it belongs to the Call-stage input plane and is not required to
// be a member of Geometry.PointBodies, whose points are the result/entry
// projection plane. Body ownership is the canonical join shared by both.
func nativeSendBodyIndex(geometry Geometry, mount, body identity.ContentID) (int, bool) {
	if !geometry.Valid() || !mount.Available() || !body.Available() {
		return 0, false
	}
	key := artifactResultBody{mount: mount, body: body}
	found := -1
	for index, candidate := range geometry.bodies {
		if candidate.key != key {
			continue
		}
		if found >= 0 {
			return 0, false
		}
		found = index
	}
	return found, found >= 0
}
