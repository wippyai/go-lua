package mounted

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// Mount is one placed compiled Program: the immutable artifact together with
// the Link-local module identity that places it. It is the complete input
// vocabulary every population in this package reads, and deliberately smaller
// than the neutral mount row the mount phase carries: a population is keyed by
// where a program sits, never by which Program it is a copy of.
type Mount struct {
	ModuleKey identity.ContentID
	Artifact  *programartifact.Artifact
}

func (mount Mount) Available() bool {
	return mount.ModuleKey.Available() && mount.Artifact != nil && mount.Artifact.Available() && mount.Artifact.ID().Available()
}

// mountsAvailable states the admission every seal shares: at least one mount,
// each one complete, and no module identity placed twice. A repeated module
// identity would collapse two mounts into one key space, so the whole
// population fails closed rather than publishing a merged census.
func mountsAvailable(mounts []Mount) bool {
	if len(mounts) == 0 {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if !mount.Available() {
			return false
		}
		if _, duplicate := seen[mount.ModuleKey]; duplicate {
			return false
		}
		seen[mount.ModuleKey] = struct{}{}
	}
	return true
}
