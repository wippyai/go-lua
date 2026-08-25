package call

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// Target is an opaque, validated capability for one selector in one Call
// Algebra. It is deliberately comparable so a source-owned route table can
// use it as a key, but it exposes neither the selector ordinal nor the
// underlying Program/Target representation. The capability therefore cannot
// become a second target vocabulary outside Call.
//
// A Target only denotes an explicitly known call alternative. It does not
// witness the opaque alternative carried by an open or top Value.
type Target struct {
	owner    *Algebra
	selector selector
}

// Body is an opaque capability for one canonical executable Program body
// target. It retains only Call's private target coordinate; all semantic
// fields remain in the target row that owns them. Only Target.Body can issue
// one.
//
// Keeping the capability opaque is important at the Call/Effect boundary:
// callers can transport a proven body target, but cannot manufacture a body
// from a raw Program term or turn Call's dense target layout into a second
// semantic vocabulary.
type Body struct {
	owner    *Algebra
	selector selector
}

// Bodies is Call's one global executable-function-body view.  It carries
// only the issuing Algebra: Count addresses the retained body-target prefix,
// while At and Index revalidate every capability against that exact owner.
// It is deliberately not scoped to an Application because Call values may
// flow to any admitted body target.
type Bodies struct{ owner *Algebra }

// Seeds is Call's zero-copy view over the external-seed suffix of the sole
// canonical target table. It introduces neither a second list nor a second
// inverse: Count and At derive their range from bodyTargetCount, which already
// seals the Body prefix boundary.
type Seeds struct{ owner *Algebra }

// Bodies returns the sole global body-target cursor for this Algebra.
func (algebra *Algebra) Bodies() Bodies {
	if algebra == nil || !algebra.Valid() {
		return Bodies{}
	}
	return Bodies{owner: algebra}
}

// Seeds returns the sole external-seed target cursor for this Algebra.
func (algebra *Algebra) Seeds() Seeds {
	if algebra == nil || !algebra.Valid() {
		return Seeds{}
	}
	return Seeds{owner: algebra}
}

// Count reports the exact external-seed suffix width.
func (seeds Seeds) Count() int {
	if seeds.owner == nil || !seeds.owner.Valid() || seeds.owner.bodyTargetCount < 0 || seeds.owner.bodyTargetCount > len(seeds.owner.targets) {
		return 0
	}
	return len(seeds.owner.targets) - seeds.owner.bodyTargetCount
}

// At projects one exact seed target in Call's canonical target order.
func (seeds Seeds) At(index int) (Target, bool) {
	if index < 0 || index >= seeds.Count() {
		return Target{}, false
	}
	return seeds.owner.targetForSelector(selector(seeds.owner.bodyTargetCount + index + 1))
}

// Count reports the exact executable function-body prefix width.
func (bodies Bodies) Count() int {
	if bodies.owner == nil || !bodies.owner.Valid() || bodies.owner.bodyTargetCount < 0 || bodies.owner.bodyTargetCount > len(bodies.owner.targets) {
		return 0
	}
	return bodies.owner.bodyTargetCount
}

// At projects one body capability in Call's canonical target order.
func (bodies Bodies) At(index int) (Body, bool) {
	if index < 0 || index >= bodies.Count() {
		return Body{}, false
	}
	target, ok := bodies.owner.targetForSelector(selector(index + 1))
	if !ok {
		return Body{}, false
	}
	return target.Body()
}

// Index returns a compact cursor coordinate for an exact body capability.
// Foreign and resealed capabilities fail even when their Program content and
// body term are equivalent.
func (bodies Bodies) Index(body Body) (int, bool) {
	if bodies.owner == nil || body.owner != bodies.owner || !body.Valid() {
		return 0, false
	}
	index := int(body.selector) - 1
	if index < 0 || index >= bodies.Count() {
		return 0, false
	}
	projected, ok := bodies.At(index)
	return index, ok && projected.Same(body)
}

// targetForSelector is the sole Call-internal conversion from dense target
// storage to an owner-bound public capability.
func (algebra *Algebra) targetForSelector(selector selector) (Target, bool) {
	if algebra == nil || !algebra.Valid() || !selector.valid() || uint64(selector) > uint64(len(algebra.targets)) {
		return Target{}, false
	}
	return Target{owner: algebra, selector: selector}, true
}

// Valid reports whether this capability still names one target of its sealed
// Algebra.
func (target Target) Valid() bool {
	return target.owner != nil && target.owner.Valid() && target.selector.valid() && uint64(target.selector) <= uint64(len(target.owner.targets))
}

// Same proves exact capability identity. Equality is also safe because Target
// is comparable, but Same makes the intended semantic comparison explicit at
// source-owned route boundaries.
func (target Target) Same(other Target) bool {
	return target.Valid() && other.Valid() && target.owner == other.owner && target.selector == other.selector
}

// Body projects an executable function target to its owner-fenced body
// capability.  Operation/seed targets intentionally return no Body.
func (capability Target) Body() (Body, bool) {
	if !capability.Valid() {
		return Body{}, false
	}
	row := capability.owner.targets[capability.selector-1]
	if row.kind != targetBody || !row.bodyPath.Available() {
		return Body{}, false
	}
	if _, mountOK := capability.owner.mountRow(row.mount); !mountOK {
		return Body{}, false
	}
	body := Body{owner: capability.owner, selector: capability.selector}
	return body, body.Valid()
}

// Valid reports whether this body capability still names one exact body in
// the Algebra that issued it.  The check includes the canonical target map,
// so a package-internal forged Body cannot cross the owner fence.
func (body Body) Valid() bool {
	if body.owner == nil || !body.owner.Valid() || !body.selector.valid() || uint64(body.selector) > uint64(len(body.owner.targets)) {
		return false
	}
	row := body.owner.targets[body.selector-1]
	_, mountOK := body.owner.mountRow(row.mount)
	return row.kind == targetBody && row.bodyPath.Available() && mountOK
}

// Same proves exact body-capability identity.  Equal source terms from
// equivalent Links remain distinct hot authorities, just like every other
// Call capability.
func (body Body) Same(other Body) bool {
	return body.Valid() && other.Valid() && body.owner == other.owner && body.selector == other.selector
}

// ModuleKey returns Project's already-issued mount identity for this exact
// body capability. It is used only as a mount provenance fence; callers
// cannot recover the underlying Shard or manufacture a Body from the scalar.
func (body Body) ModuleKey() (identity.ContentID, bool) {
	if !body.Valid() || body.owner == nil {
		return identity.ContentID{}, false
	}
	row := body.owner.targets[body.selector-1]
	mount, ok := body.owner.mountRow(row.mount)
	return mount.moduleID, ok
}

// ProgramID returns the source identity attached to this body row.
func (body Body) ProgramID() (identity.ContentID, bool) {
	if !body.Valid() {
		return identity.ContentID{}, false
	}
	row := body.owner.targets[body.selector-1]
	mount, ok := body.owner.mountRow(row.mount)
	return mount.programID, ok
}

// BodyPath returns the artifact body identity; it is not a dense ordinal.
func (body Body) BodyPath() (identity.ContentID, bool) {
	if !body.Valid() {
		return identity.ContentID{}, false
	}
	return body.owner.targets[body.selector-1].bodyPath, true
}

// Operation projects the exact sealed target operation of one known seed
// alternative. Function-body targets and the opaque remainder of an open Call
// value deliberately have no operation projection.
func (capability Target) Operation() (vocabulary.Operation, bool) {
	if !capability.Valid() {
		return 0, false
	}
	row := capability.owner.targets[capability.selector-1]
	if row.kind != targetSeed {
		return 0, false
	}
	return row.seedOperation, row.seedOperation != 0
}

// IsScopedLoader distinguishes the mounted, shard-local require target from
// an ordinary admitted operation target. Both retain the same exact Target
// operation handle; this provenance bit is authored by Boundary's loader
// seed and prevents a direct operation with equal identity from masquerading
// as the scoped loader.
func (capability Target) IsScopedLoader() bool {
	if !capability.Valid() {
		return false
	}
	row := capability.owner.targets[capability.selector-1]
	return row.kind == targetSeed && row.scopedLoader
}

func (value Value) knownTargetCount() int {
	if !value.valid() || value.top {
		return 0
	}
	return len(value.selectors)
}

// KnownTargetCount is the number of explicitly known alternatives. It has
// identical meaning for Open and Complete Values: Open's omitted alternatives
// remain opaque and are not enumerated. Top has no finite known image.
func (value Value) KnownTargetCount() int { return value.knownTargetCount() }

// KnownTargetAt projects one explicitly known alternative without allocating
// or scanning the Algebra's global target universe. It works for both Open
// and Complete Values; callers must separately preserve
// HasOpaqueAlternative when it is true.
func (value Value) KnownTargetAt(index int) (Target, bool) {
	if index < 0 || index >= value.knownTargetCount() {
		return Target{}, false
	}
	return Target{owner: value.owner, selector: value.selectors[index]}, true
}

// BodyTargetCount is the executable body prefix of this Algebra's canonical
// target order, counted as TARGETS rather than as body capabilities.
//
// It exists beside Bodies because a consumer enumerating what a call may reach
// is asking about targets - the same rows Bodies answers, named as what a Call
// value's alternatives are - and pairing it with BodyTargetAt lets that
// enumeration be declared rather than walked through two vocabularies.
func (algebra *Algebra) BodyTargetCount() int { return algebra.Bodies().Count() }

// BodyTargetAt projects one body target in that same canonical order.
func (algebra *Algebra) BodyTargetAt(index int) (Target, bool) {
	bodies := algebra.Bodies()
	if index < 0 || index >= bodies.Count() {
		return Target{}, false
	}
	return bodies.owner.targetForSelector(selector(index + 1))
}
