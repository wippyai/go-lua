// Package evaluation owns the Seal-local evaluation-order projection.
//
// It walks the typed authored expression and direct value-consuming statement
// relations using the language's evaluation laws. Branch/Loop topology and
// their body phases belong to recurrence and are deliberately not roots here.
// The Session retains no graph, sidecar, or source-order authority. Its only
// output is the ordered sequence of authored Select occurrences reachable from
// one root.
package evaluation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Ports is the immutable, assembly-local evaluation-port proof.  Each dense
// plane is indexed by the canonical Term family and ordinal; no graph or
// operation tag is retained.  A zero slot is never a successful query.
//
// The planes are private and are populated only by SealPorts.  Keeping the
// proof as dense typed planes makes the hot Entry/Finish queries allocation
// free and keeps the source identity and authored view out of the result.
type Ports struct {
	// Provenance is copied at seal time as scalar content identities.  Ports
	// never retain Source or authored Flow authority; consumers use Matches at
	// composition boundaries to reject foreign or unavailable owners.
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
	entry    [keyspace.FamilyCount][]keyspace.Term
	finish   [keyspace.FamilyCount][]keyspace.Term
}

// Matches reports whether ports was sealed for the exact Source, authored
// Flow, Static, and Module identities supplied by the caller. Unavailable
// identities never match, including a malformed Ports value carrying
// otherwise plausible planes.
func Matches(ports *Ports, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return ports != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		ports.sourceID.Available() && ports.flowID.Available() && ports.staticID.Available() && ports.moduleID.Available() &&
		ports.sourceID == sourceID && ports.flowID == flowID && ports.staticID == staticID && ports.moduleID == moduleID
}

func (ports *Ports) available() bool {
	return ports != nil && ports.sourceID.Available() && ports.flowID.Available() && ports.staticID.Available() && ports.moduleID.Available()
}

func (ports *Ports) plane(family keyspace.Family, ordinal uint32, planes *[keyspace.FamilyCount][]keyspace.Term) (keyspace.Term, bool) {
	if ports == nil || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 {
		return 0, false
	}
	plane := planes[family]
	if uint64(ordinal) >= uint64(len(plane)) {
		return 0, false
	}
	value := plane[ordinal]
	return value, value != 0
}
