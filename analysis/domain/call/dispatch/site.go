package dispatch

import (
	"crypto/sha256"
	"encoding/binary"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// site is the private ordinary-call binding consumed by this package's Rule.
// It retains only canonical owner handles: the Call key, callee Value
// coordinate, and Pack call root. No cross-domain site can escape this
// package or be supplied independently of Rule.Instance(application).
type site struct {
	algebra    *calldomain.Algebra
	values     *valuedomain.Schema
	heaps      heapdomain.Schema
	boundary   *linkboundary.Component
	packs      *packdomain.Schema
	link       *link.Link
	key        calldomain.Key
	callee     linkboundary.Value
	coordinate valuedomain.Coordinate
	root       packdomain.Root
	keyID      keyspace.ContentID
	valueID    keyspace.ContentID
	rootID     keyspace.ContentID
	id         keyspace.ContentID
}

const siteVersion uint64 = 1

// newSite consumes Boundary.Calls().Callee exactly once. It never reopens
// Program Flow and never materializes Application×Target availability.
func newSite(
	algebra *calldomain.Algebra,
	values *valuedomain.Schema,
	heaps heapdomain.Schema,
	packs *packdomain.Schema,
	application linkproject.Application,
) (site, bool) {
	if algebra == nil || !algebra.Valid() || values == nil || packs == nil || !heaps.Valid() {
		return site{}, false
	}
	source := algebra.Link()
	if source == nil || values.Link() != source || heaps.Link() != source || packs.Link() != source {
		return site{}, false
	}
	boundary := source.Boundary()
	if boundary == nil || !boundary.MatchesProject(source.Project()) {
		return site{}, false
	}
	key, keyOK := algebra.KeyForApplication(application)
	callee, calleeOK := boundary.Calls().Callee(application)
	coordinate, coordinateOK := values.CoordinateFor(callee)
	root, rootOK := packs.CallRoot(application)
	keyID, keyIDOK := key.ContentID()
	valueID, valueIDOK := boundary.Values().ID(callee)
	rootID, rootIDOK := packs.RootID(root)
	if !keyOK || !calleeOK || !coordinateOK || !rootOK || !keyIDOK || !valueIDOK || !rootIDOK {
		return site{}, false
	}
	id := siteID(source.ContentID(), keyID, valueID, rootID)
	if !id.Available() {
		return site{}, false
	}
	bound := site{
		algebra: algebra, values: values, heaps: heaps, boundary: boundary,
		packs: packs, link: source, key: key, callee: callee,
		coordinate: coordinate, root: root, keyID: keyID, valueID: valueID,
		rootID: rootID, id: id,
	}
	return bound, bound.valid()
}

func siteID(linkID, keyID, valueID, rootID keyspace.ContentID) keyspace.ContentID {
	if !linkID.Available() || !keyID.Available() || !valueID.Available() || !rootID.Available() {
		return keyspace.ContentID{}
	}
	var image [32*4 + 16]byte
	copy(image[:32], linkID[:])
	copy(image[32:64], keyID[:])
	copy(image[64:96], valueID[:])
	copy(image[96:128], rootID[:])
	binary.BigEndian.PutUint64(image[128:136], 0x63616c6c2d646973) // call-dis
	binary.BigEndian.PutUint64(image[136:144], siteVersion)
	return sha256.Sum256(image[:])
}

// valid is the cold owner-fence check shared by construction, transfer, and
// admission. Replay IDs are checked after exact owner pointers.
func (bound site) valid() bool {
	if bound.algebra == nil || !bound.algebra.Valid() || bound.values == nil ||
		!bound.heaps.Valid() || bound.packs == nil || bound.link == nil ||
		bound.algebra.Link() != bound.link || bound.values.Link() != bound.link ||
		bound.heaps.Link() != bound.link || bound.packs.Link() != bound.link ||
		bound.boundary == nil || bound.boundary != bound.link.Boundary() ||
		!bound.boundary.MatchesProject(bound.link.Project()) || !bound.key.Valid() ||
		!bound.values.OwnsHeapSchema(bound.heaps) ||
		!bound.key.IsApplication() || !bound.coordinate.Valid() ||
		!bound.keyID.Available() || !bound.valueID.Available() ||
		!bound.rootID.Available() || !bound.id.Available() || !bound.rootMatches() {
		return false
	}
	keyID, keyOK := bound.key.ContentID()
	owned, ownedOK := bound.algebra.FindKey(keyID)
	rootID, rootOK := bound.packs.RootID(bound.root)
	coordinate, coordinateOK := bound.values.CoordinateFor(bound.callee)
	valueID, valueOK := bound.boundary.Values().ID(bound.callee)
	expectedID := siteID(bound.link.ContentID(), keyID, valueID, rootID)
	return keyOK && ownedOK && owned == bound.key && keyID == bound.keyID &&
		rootOK && rootID == bound.rootID && coordinateOK && coordinate == bound.coordinate &&
		valueOK && valueID == bound.valueID && expectedID.Available() && expectedID == bound.id
}

// matchesSchemas is the Rule-level reseal fence. Link equality alone is not
// sufficient: a separately sealed Heap or Pack schema over the same Link is
// a distinct owner and cannot supply this site's recurrent binding.
func (bound site) matchesSchemas(heaps heapdomain.Schema, packs *packdomain.Schema) bool {
	return bound.valid() && bound.heaps == heaps && bound.packs == packs
}

func (bound site) rootMatches() bool {
	if bound.packs == nil {
		return false
	}
	id, ok := bound.packs.RootID(bound.root)
	return ok && id == bound.rootID
}

func (bound site) contentID() (keyspace.ContentID, bool) {
	if !bound.valid() {
		return keyspace.ContentID{}, false
	}
	return bound.id, true
}

func (bound site) callKey() (calldomain.Key, bool) {
	if !bound.valid() {
		return calldomain.Key{}, false
	}
	return bound.key, true
}

func (bound site) algebraOwner() *calldomain.Algebra {
	if !bound.valid() {
		return nil
	}
	return bound.algebra
}

func (bound site) linkOwner() *link.Link {
	if !bound.valid() {
		return nil
	}
	return bound.link
}

func (bound site) valueSchema() *valuedomain.Schema {
	if !bound.valid() {
		return nil
	}
	return bound.values
}

func (bound site) valueCoordinate() (valuedomain.Coordinate, bool) {
	if !bound.valid() {
		return valuedomain.Coordinate{}, false
	}
	return bound.coordinate, true
}

func (bound site) packRoot() (packdomain.Root, bool) {
	if !bound.valid() {
		return packdomain.Root{}, false
	}
	return bound.root, true
}

func (bound site) boundaryCallee() (linkboundary.Value, bool) {
	if !bound.valid() {
		return linkboundary.Value{}, false
	}
	return bound.callee, true
}
