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
)

// site is the private ordinary-call binding consumed by this package's Rule.
// It retains only canonical owner handles: the Call key, callee Value
// coordinate, and Pack call root. No cross-domain site can escape this
// package or be supplied independently of Rule.Instance(application).
type site struct {
	algebra       *calldomain.Algebra
	values        *valuedomain.Schema
	heaps         heapdomain.Schema
	requireSeedID keyspace.ContentID
	packs         *packdomain.Schema
	key           calldomain.Key
	mounted       calldomain.MountedCall
	coordinate    valuedomain.Coordinate
	root          packdomain.Root
	keyID         keyspace.ContentID
	valueID       keyspace.ContentID
	rootID        keyspace.ContentID
	id            keyspace.ContentID
}

const siteVersion uint64 = 1

// newSite consumes Call's detached mounted-dispatch row. It never reopens
// Project, Boundary, Program Flow, or materializes Application×Target
// availability.
func newSite(
	algebra *calldomain.Algebra,
	values *valuedomain.Schema,
	heaps heapdomain.Schema,
	packs *packdomain.Schema,
	applicationID keyspace.ContentID,
) (site, bool) {
	if algebra == nil || !algebra.Valid() || values == nil || packs == nil || !heaps.Valid() {
		return site{}, false
	}
	owner := algebra.LinkOwner()
	if !owner.Available() || !values.LinkOwner().Matches(owner) || !heaps.LinkOwner().Matches(owner) || !packs.LinkOwner().Matches(owner) {
		return site{}, false
	}
	key, keyOK := algebra.KeyForApplicationID(applicationID)
	if !keyOK {
		return site{}, false
	}
	mounted, mountedOK := algebra.MountedCallForApplication(applicationID)
	issuedApplication, occurrence, module, valueID, seedID, identityOK := algebra.MountedCallIdentity(mounted)
	coordinate, coordinateOK := values.CoordinateForID(valueID)
	root, rootOK := packs.CallRootForMountedSemantic(module, occurrence)
	keyID, keyIDOK := key.ContentID()
	rootID, rootIDOK := packs.RootID(root)
	if !mountedOK || !identityOK || issuedApplication != applicationID || !coordinateOK || !rootOK || !keyIDOK || !rootIDOK {
		return site{}, false
	}
	siteIdentity := siteID(owner, keyID, valueID, rootID)
	if !siteIdentity.Available() {
		return site{}, false
	}
	bound := site{algebra: algebra, values: values, heaps: heaps, requireSeedID: seedID, packs: packs, key: key, mounted: mounted, coordinate: coordinate, root: root, keyID: keyID, valueID: valueID, rootID: rootID, id: siteIdentity}
	return bound, bound.valid()
}

func siteID(owner link.OwnerCapability, keyID, valueID, rootID keyspace.ContentID) keyspace.ContentID {
	if !owner.Available() {
		return keyspace.ContentID{}
	}
	linkID := owner.ContentID()
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
		!bound.heaps.Valid() || bound.packs == nil ||
		!bound.values.LinkOwner().Matches(bound.algebra.LinkOwner()) || !bound.heaps.LinkOwner().Matches(bound.algebra.LinkOwner()) || !bound.packs.LinkOwner().Matches(bound.algebra.LinkOwner()) ||
		!bound.key.Valid() ||
		!bound.mounted.Valid() ||
		!bound.values.OwnsHeapSchema(bound.heaps) ||
		!bound.key.IsApplication() || !bound.coordinate.Valid() ||
		!bound.requireSeedID.Available() ||
		!bound.keyID.Available() || !bound.valueID.Available() ||
		!bound.rootID.Available() || !bound.id.Available() || !bound.rootMatches() {
		return false
	}
	keyID, keyOK := bound.key.ContentID()
	owned, ownedOK := bound.algebra.FindKey(keyID)
	if !keyOK || !ownedOK || owned != bound.key || keyID != bound.keyID {
		return false
	}
	// The application key is the sole portable inverse. Re-fetch every
	// mount-sensitive scalar from Call's sealed row so a package-local copy
	// cannot splice another mount's loader seed, Value coordinate, or Pack
	// root while retaining this site's replay identity.
	applicationID, applicationOK := bound.key.ApplicationID()
	mounted, mountedOK := bound.algebra.MountedCallForApplication(applicationID)
	issuedApplication, contextID, moduleID, valueID, seedID, identityOK := bound.algebra.MountedCallIdentity(mounted)
	expectedCoordinate, coordinateOK := bound.values.CoordinateForID(valueID)
	expectedRoot, rootOK := bound.packs.CallRootForMountedSemantic(moduleID, contextID)
	rootID, rootIDOK := bound.packs.RootID(bound.root)
	expectedRootID, expectedRootIDOK := bound.packs.RootID(expectedRoot)
	expectedID := siteID(bound.algebra.LinkOwner(), keyID, valueID, expectedRootID)
	return applicationOK && mountedOK && identityOK && issuedApplication == applicationID && mounted == bound.mounted && coordinateOK && rootOK && rootIDOK && expectedRootIDOK &&
		valueID == bound.valueID && seedID == bound.requireSeedID &&
		expectedCoordinate == bound.coordinate && expectedRoot == bound.root && rootID == bound.rootID &&
		expectedID.Available() && expectedID == bound.id
}

// matchesSchemas is the Rule-level reseal fence. Shared Link ownership alone
// is not sufficient: a separately sealed Heap or Pack schema over the same
// Link is a distinct schema owner and cannot supply this site's recurrent
// binding.
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
