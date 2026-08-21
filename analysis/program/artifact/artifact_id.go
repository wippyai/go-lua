package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

// artifactID owns only the Artifact envelope. The complete Program payload is
// one canonical publication manifest, so Artifact cannot acquire a second
// replay representation or drift from a row owner's identity law.
func artifactID(artifact *Artifact) identity.ContentID {
	if artifact == nil {
		return identity.ContentID{}
	}
	sink := artifactdigest.NewSink(artifactIDDomain, artifactFormat)
	sink.Add(artifactdigest.ContentID(artifact.key.ID()))
	sink.Add(artifact.key.identityFields()...)
	sink.Add(artifactdigest.Uint(uint64(artifact.counts.Count())))
	for index := 0; index < artifact.counts.Count(); index++ {
		row, ok := artifact.counts.At(index)
		if !ok {
			return identity.ContentID{}
		}
		sink.Add(artifactdigest.ContentID(identity.ContentID(row.ID())), artifactdigest.Uint(row.Count()))
	}
	state, stateOK := programstate.New(artifact.frozen, artifact.coldCatalog)
	if !stateOK || !programpublication.WriteArtifactIdentityFields(state, &sink) {
		return identity.ContentID{}
	}
	return sink.Sum()
}
