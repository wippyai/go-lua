package result

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// resultFormat is part of the Result identity preimage. The normalized
// family/point tables and context-qualified detached points are a different
// wire shape from older projections, so they must never share an identity with
// an older Result.
const resultFormat uint64 = 12

// Result is a detached projection of canonical body/root geometry and generic
// query publications.  Query family contracts and point geometry are stored
// once and rows refer to those tables by ordinal; no engine or domain handle
// crosses this boundary.
type Result struct {
	source   identity.ContentID
	content  identity.ContentID
	bodies   []resultBody
	points   []resultPoint
	families []resultFamily
	// Native publication is detached output owned by this Result. The explicit
	// bit keeps a published empty output distinct from an absent producer.
	nativeContent   identity.ContentID
	nativeRows      []nativePublicationRow
	nativeByID      map[identity.ContentID]uint32
	nativePublished bool
	sealed          bool
}

type resultBody struct {
	id    identity.ContentID
	roots []resultRoot
}

type resultRoot struct {
	id     identity.ContentID
	family keyspace.Family
}

// resultPoint owns the context- and mount-qualified point identity and the
// body ordinals reached by that point.  Context is retained only on detached
// publication geometry; the generic Geometry plane remains mount/point
// qualified. Body ordinals are one-based so zero remains the invalid value in
// every detached table.
type resultPoint struct {
	context identity.ContentID
	mount   identity.ContentID
	point   identity.ContentID
	bodies  []uint32
}

// detachedPointKey is the identity used by the detached point table. Keep it
// separate from Point: Point is also the generic, context-free Geometry key
// used to resolve mounted body membership.
type detachedPointKey struct {
	context identity.ContentID
	mount   identity.ContentID
	point   identity.ContentID
}

type Body struct {
	owner   *Result
	ordinal uint32
}

// Root is a detached exact executable root row of one Body.
type Root struct {
	owner *Result
	body  uint32
	index uint32
}

func (result *Result) ContentID() identity.ContentID {
	if !result.valid() {
		return identity.ContentID{}
	}
	return result.content
}

func (result *Result) SourceID() identity.ContentID {
	if !result.valid() {
		return identity.ContentID{}
	}
	return result.source
}

func (result *Result) BodyCount() int {
	if !result.valid() {
		return 0
	}
	return len(result.bodies)
}

func (result *Result) BodyAt(index int) (Body, bool) {
	if !result.valid() || index < 0 || index >= len(result.bodies) {
		return Body{}, false
	}
	return Body{owner: result, ordinal: uint32(index + 1)}, true
}

func (body Body) row() (resultBody, bool) {
	if body.owner == nil || !body.owner.valid() || body.ordinal == 0 || uint64(body.ordinal) > uint64(len(body.owner.bodies)) {
		return resultBody{}, false
	}
	return body.owner.bodies[body.ordinal-1], true
}

func (body Body) ID() (identity.ContentID, bool) {
	row, ok := body.row()
	return row.id, ok
}

func (body Body) RootCount() int {
	// Root rows are the exact mount-qualified ProgramArtifact root plane; no
	// Solve-time Program, Source, or Flow reconstruction participates here.
	row, ok := body.row()
	if !ok {
		return 0
	}
	return len(row.roots)
}

func (body Body) RootAt(index int) (Root, bool) {
	row, ok := body.row()
	if !ok || index < 0 || index >= len(row.roots) {
		return Root{}, false
	}
	return Root{owner: body.owner, body: body.ordinal, index: uint32(index + 1)}, true
}

func (root Root) row() (resultRoot, bool) {
	if root.owner == nil || !root.owner.valid() || root.body == 0 || root.index == 0 || uint64(root.body) > uint64(len(root.owner.bodies)) {
		return resultRoot{}, false
	}
	rows := root.owner.bodies[root.body-1].roots
	if uint64(root.index) > uint64(len(rows)) {
		return resultRoot{}, false
	}
	return rows[root.index-1], true
}

func (root Root) ID() (identity.ContentID, bool) {
	row, ok := root.row()
	return row.id, ok
}

func (root Root) Family() keyspace.Family {
	row, ok := root.row()
	if !ok {
		return keyspace.FamilyInvalid
	}
	return row.family
}

func (result *Result) valid() bool {
	// The detached projection is validated and content-addressed exactly once
	// before publication. All fields and nested slices are private and every
	// public accessor is read-only, so replaying the complete table census here
	// would be a second authority and make iteration quadratic.
	return result != nil && result.sealed && result.source.Available() && result.content.Available() && len(result.bodies) != 0
}

// validPayload is the one complete admission check for the normalized Result
// tables. It runs before sealing; public accessors only need the O(1) sealed
// check above afterward.
func (result *Result) validPayload() bool {
	if result == nil || result.sealed || !result.source.Available() || !result.content.Available() || len(result.bodies) == 0 {
		return false
	}
	for _, body := range result.bodies {
		if !body.id.Available() {
			return false
		}
		for _, root := range body.roots {
			if !root.id.Available() || root.family == keyspace.FamilyInvalid {
				return false
			}
		}
	}

	seenPoints := make(map[detachedPointKey]struct{}, len(result.points))
	for _, point := range result.points {
		key := detachedPointKey{context: point.context, mount: point.mount, point: point.point}
		if !point.context.Available() || !point.mount.Available() || !point.point.Available() {
			return false
		}
		if _, duplicate := seenPoints[key]; duplicate {
			return false
		}
		seenPoints[key] = struct{}{}
		seenBodies := make(map[uint32]struct{}, len(point.bodies))
		for _, bodyOrdinal := range point.bodies {
			if bodyOrdinal == 0 || uint64(bodyOrdinal) > uint64(len(result.bodies)) {
				return false
			}
			if _, duplicate := seenBodies[bodyOrdinal]; duplicate {
				return false
			}
			seenBodies[bodyOrdinal] = struct{}{}
		}
	}

	if len(result.families) == 0 {
		return false
	}
	seenSites := make(map[identity.ContentID]struct{})
	seenKeys := make(map[identity.ContentID]struct{})
	for familyIndex, family := range result.families {
		if family.ordinal == 0 || family.ordinal != uint32(familyIndex+1) || !family.key.Available() || !family.contract.Available() || len(family.queries) == 0 {
			return false
		}
		for _, query := range family.queries {
			if !query.valid(result.points, family.contract) {
				return false
			}
			if _, duplicate := seenSites[query.site]; duplicate {
				return false
			}
			if _, duplicate := seenKeys[query.key]; duplicate {
				return false
			}
			seenSites[query.site] = struct{}{}
			seenKeys[query.key] = struct{}{}
		}
	}
	if !nativePublicationStateAvailable(result.nativePublished, result.nativeContent, result.nativeRows, result.nativeByID) {
		return false
	}
	return true
}

// analysisResultID hashes only detached structure and sealed identities. In
// particular, canonical cell payloads are not hashed here: the cell's own
// ContentID already commits to its contract, presence, row count, and
// payload.
func analysisResultID(
	source identity.ContentID,
	bodies []resultBody,
	points []resultPoint,
	families []resultFamily,
) (identity.ContentID, bool) {
	return analysisResultIDWithPublication(source, bodies, points, families, false, identity.ContentID{}, nil, nil)
}

func writeResultFrame(hash interface{ Write([]byte) (int, error) }, value []byte) bool {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	first, firstErr := hash.Write(size[:])
	second, secondErr := hash.Write(value)
	return firstErr == nil && secondErr == nil && first == len(size) && second == len(value)
}

func analysisResultIDWithPublication(
	source identity.ContentID,
	bodies []resultBody,
	points []resultPoint,
	families []resultFamily,
	nativePublished bool,
	nativeContent identity.ContentID,
	nativeRows []nativePublicationRow,
	nativeByID map[identity.ContentID]uint32,
) (identity.ContentID, bool) {
	if !source.Available() || len(bodies) == 0 || len(families) == 0 ||
		nativePublished && !nativePublicationStateAvailable(nativePublished, nativeContent, nativeRows, nativeByID) {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	write := func(value []byte) bool { return writeResultFrame(hash, value) }
	var version, count, ordinal [8]byte
	binary.BigEndian.PutUint64(version[:], resultFormat)
	if !write([]byte("analysis/result")) || !write(version[:]) || !write(source[:]) {
		return identity.ContentID{}, false
	}

	binary.BigEndian.PutUint64(count[:], uint64(len(bodies)))
	if !write(count[:]) {
		return identity.ContentID{}, false
	}
	for _, body := range bodies {
		binary.BigEndian.PutUint64(count[:], uint64(len(body.roots)))
		if !body.id.Available() || !write(body.id[:]) || !write(count[:]) {
			return identity.ContentID{}, false
		}
		for _, root := range body.roots {
			if !root.id.Available() || root.family == keyspace.FamilyInvalid || !write(root.id[:]) || !write([]byte{byte(root.family)}) {
				return identity.ContentID{}, false
			}
		}
	}

	binary.BigEndian.PutUint64(count[:], uint64(len(points)))
	if !write(count[:]) {
		return identity.ContentID{}, false
	}
	for _, point := range points {
		if !point.context.Available() || !point.mount.Available() || !point.point.Available() ||
			!write(point.context[:]) || !write(point.mount[:]) || !write(point.point[:]) {
			return identity.ContentID{}, false
		}
		binary.BigEndian.PutUint64(count[:], uint64(len(point.bodies)))
		if !write(count[:]) {
			return identity.ContentID{}, false
		}
		for _, body := range point.bodies {
			binary.BigEndian.PutUint64(ordinal[:], uint64(body))
			if !write(ordinal[:]) {
				return identity.ContentID{}, false
			}
		}
	}

	binary.BigEndian.PutUint64(count[:], uint64(len(families)))
	if !write(count[:]) {
		return identity.ContentID{}, false
	}
	for familyIndex, family := range families {
		if family.ordinal == 0 || family.ordinal != uint32(familyIndex+1) || !family.key.Available() || !family.contract.Available() {
			return identity.ContentID{}, false
		}
		binary.BigEndian.PutUint64(ordinal[:], uint64(family.ordinal))
		contract := family.contract.ContentID()
		familyID := family.contract.FamilyID()
		codec := family.contract.Codec()
		codecDigest := codec.Digest()
		var codecVersion [8]byte
		binary.BigEndian.PutUint64(codecVersion[:], codec.Version())
		if !write(ordinal[:]) || !write([]byte(family.key)) || !write(contract[:]) || !write(familyID[:]) || !write(codecDigest[:]) || !write(codecVersion[:]) {
			return identity.ContentID{}, false
		}
		binary.BigEndian.PutUint64(count[:], uint64(len(family.queries)))
		if !write(count[:]) {
			return identity.ContentID{}, false
		}
		for _, query := range family.queries {
			if !query.valid(points, family.contract) || !write(query.site[:]) || !write(query.key[:]) {
				return identity.ContentID{}, false
			}
			binary.BigEndian.PutUint64(ordinal[:], uint64(query.point))
			if !write(ordinal[:]) || !write([]byte{byte(query.status)}) {
				return identity.ContentID{}, false
			}
			cell := identity.ContentID{}
			if query.status == QueryHit {
				cell = query.cell.ContentID()
			}
			if !write(cell[:]) {
				return identity.ContentID{}, false
			}
		}
	}

	// Native publication is a separate sealed lane, and its content identity is
	// part of the enclosing Result identity.
	if !write([]byte{boolByte(nativePublished)}) {
		return identity.ContentID{}, false
	}
	nativeCount := 0
	if nativePublished {
		nativeCount = len(nativeRows)
	}
	binary.BigEndian.PutUint64(count[:], uint64(nativeCount))
	if !write(count[:]) {
		return identity.ContentID{}, false
	}
	if nativePublished && !write(nativeContent[:]) {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

// Point is the mount-qualified key shared by Geometry and Result's point
// table. It is intentionally a pair rather than a derived identity: both
// components are needed to resolve the body geometry without reopening a
// Program.
type Point struct {
	Mount identity.ContentID
	Point identity.ContentID
}
