package result

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

const resultFormat uint64 = 9

// Result is a detached projection of canonical body-root and query rows. It
// retains neither Link/domain/engine handles nor template classifications.
type Result struct {
	source  identity.ContentID
	content identity.ContentID
	values  []identity.ContentID
	bodies  []resultBody
	// native is the one sealed post-convergence publication receipt. An
	// available empty receipt is distinct from a missing producer.
	native *nativePublicationReceipt
	// Result carries no placement plane. The placement domain declares no axis,
	// rule role, or factor, so there is no owner able to issue a placement
	// receipt; the plane lands with that factor (journal seq 2134), and the
	// identity format below already reserves its frames.
	sealed bool
}

type resultBody struct {
	id            identity.ContentID
	roots         []resultRoot
	valuePresence []uint64
	effectPresent bool
	effectTop     bool
	effects       []identity.ContentID
}

type resultRoot struct {
	id     identity.ContentID
	family keyspace.Family
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
func (body Body) ID() (identity.ContentID, bool) { row, ok := body.row(); return row.id, ok }
func (body Body) RootCount() int {
	// Root rows are the exact mount-qualified ProgramArtifact receipt plane; no
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
func (root Root) ID() (identity.ContentID, bool) { row, ok := root.row(); return row.id, ok }
func (root Root) Family() keyspace.Family {
	row, ok := root.row()
	if !ok {
		return keyspace.FamilyInvalid
	}
	return row.family
}

func (body Body) EffectDisposition() (present, top, ok bool) {
	row, ok := body.row()
	return row.effectPresent, row.effectTop, ok
}
func (body Body) EffectCount() int {
	row, ok := body.row()
	if !ok {
		return 0
	}
	return len(row.effects)
}
func (body Body) EffectAt(index int) (identity.ContentID, bool) {
	row, ok := body.row()
	if !ok || index < 0 || index >= len(row.effects) {
		return identity.ContentID{}, false
	}
	return row.effects[index], true
}

// ValueCount and ValueAt expose the per-body projection of the declared Value
// query. A body with no canonical coordinates has a valid empty projection.
func (body Body) ValueCount() int {
	if _, ok := body.row(); !ok || body.owner == nil {
		return 0
	}
	return len(body.owner.values)
}
func (body Body) ValueAt(index int) (id identity.ContentID, present, ok bool) {
	row, rowOK := body.row()
	if !rowOK || body.owner == nil || index < 0 || index >= len(body.owner.values) {
		return identity.ContentID{}, false, false
	}
	return body.owner.values[index], resultValuePresent(row.valuePresence, index), true
}

func (result *Result) valid() bool {
	// The detached projection is validated and content-addressed exactly once
	// before publication. All fields and nested slices are private and every
	// public accessor is read-only, so replaying the complete body/value/effect
	// census here would be a second authority and makes iteration quadratic.
	return result != nil && result.sealed && result.source.Available() && result.content.Available() && len(result.bodies) != 0
}

func (result *Result) validPayload() bool {
	if result == nil || result.sealed || !result.source.Available() || !result.content.Available() || len(result.bodies) == 0 {
		return false
	}
	for _, value := range result.values {
		if !value.Available() {
			return false
		}
	}
	for _, body := range result.bodies {
		if !body.id.Available() || body.effectTop && len(body.effects) != 0 {
			return false
		}
		for _, root := range body.roots {
			if !root.id.Available() || root.family == keyspace.FamilyInvalid {
				return false
			}
		}
		if !resultValuePresenceValid(body.valuePresence, len(result.values)) {
			return false
		}
		for _, effect := range body.effects {
			if !effect.Available() {
				return false
			}
		}
	}
	if result.native == nil || !result.native.valid() {
		return false
	}
	return true
}

func analysisResultID(source identity.ContentID, values []identity.ContentID, bodies []resultBody) (identity.ContentID, bool) {
	return analysisResultIDWithPublication(source, values, bodies, nil)
}

func writeResultFrame(hash interface{ Write([]byte) (int, error) }, value []byte) bool {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	first, firstErr := hash.Write(size[:])
	second, secondErr := hash.Write(value)
	return firstErr == nil && secondErr == nil && first == len(size) && second == len(value)
}

func analysisResultIDWithPublication(source identity.ContentID, values []identity.ContentID, bodies []resultBody, native *nativePublicationReceipt) (identity.ContentID, bool) {
	if !source.Available() || len(bodies) == 0 {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	write := func(value []byte) bool { return writeResultFrame(hash, value) }
	var version, count [8]byte
	binary.BigEndian.PutUint64(version[:], resultFormat)
	binary.BigEndian.PutUint64(count[:], uint64(len(values)))
	if !write([]byte("analysis/result")) || !write(version[:]) || !write(source[:]) || !write(count[:]) {
		return identity.ContentID{}, false
	}
	for _, value := range values {
		if !value.Available() || !write(value[:]) {
			return identity.ContentID{}, false
		}
	}
	binary.BigEndian.PutUint64(count[:], uint64(len(bodies)))
	if !write(count[:]) {
		return identity.ContentID{}, false
	}
	for _, body := range bodies {
		binary.BigEndian.PutUint64(count[:], uint64(len(body.roots)))
		if !write(body.id[:]) || !write(count[:]) {
			return identity.ContentID{}, false
		}
		for _, root := range body.roots {
			if !write(root.id[:]) || !write([]byte{byte(root.family)}) {
				return identity.ContentID{}, false
			}
		}
		binary.BigEndian.PutUint64(count[:], uint64(len(body.valuePresence)))
		if !write(count[:]) {
			return identity.ContentID{}, false
		}
		for _, word := range body.valuePresence {
			binary.BigEndian.PutUint64(count[:], word)
			if !write(count[:]) {
				return identity.ContentID{}, false
			}
		}
		binary.BigEndian.PutUint64(count[:], uint64(len(body.effects)))
		if !write([]byte{boolByte(body.effectPresent), boolByte(body.effectTop)}) || !write(count[:]) {
			return identity.ContentID{}, false
		}
		for _, effect := range body.effects {
			if !write(effect[:]) {
				return identity.ContentID{}, false
			}
		}
	}
	nativeAvailable := native != nil && native.valid()
	if native != nil && !nativeAvailable {
		return identity.ContentID{}, false
	}
	if !write([]byte{boolByte(nativeAvailable)}) {
		return identity.ContentID{}, false
	}
	nativeCount := 0
	if nativeAvailable {
		nativeCount = len(native.rows)
	}
	binary.BigEndian.PutUint64(count[:], uint64(nativeCount))
	if !write(count[:]) || nativeAvailable && !write(native.content[:]) {
		return identity.ContentID{}, false
	}
	// The placement plane has no owner able to issue a receipt, so its
	// availability flag and row count are written as absent. The frames stay in
	// the Result format so a future solved typed placement receipt extends this
	// identity without a parallel Result family.
	if !write([]byte{boolByte(false)}) {
		return identity.ContentID{}, false
	}
	binary.BigEndian.PutUint64(count[:], 0)
	if !write(count[:]) {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func resultValueWordCount(values int) int {
	if values <= 0 {
		return 0
	}
	return (values + 63) / 64
}

func resultValuePresent(words []uint64, index int) bool {
	if index < 0 || index/64 >= len(words) {
		return false
	}
	return words[index/64]&(uint64(1)<<uint(index%64)) != 0
}

func setResultValuePresent(words []uint64, index int) bool {
	if index < 0 || index/64 >= len(words) {
		return false
	}
	words[index/64] |= uint64(1) << uint(index%64)
	return true
}

func resultValuePresenceValid(words []uint64, values int) bool {
	if len(words) != resultValueWordCount(values) {
		return false
	}
	if values == 0 || values%64 == 0 {
		return true
	}
	validBits := uint(values % 64)
	return words[len(words)-1]&^((uint64(1)<<validBits)-1) == 0
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
