// Package artifact is the owner-neutral structural declaration surface for one
// reusable Program artifact. It carries the scalar row vocabulary, the
// single-use template builder, and the relational admission fence that seals a
// template. It deliberately depends on no binding, schema, capability, or
// equation authority, so the same sealed template can be shared by independent
// Links and mounted repeatedly without rebuilding the Program interior.
package rows

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// ArtifactStructuralArm is the engine-neutral structural-edge arm vocabulary.
type ArtifactStructuralArm uint8

const (
	ArtifactStructuralArmInvalid ArtifactStructuralArm = iota
	ArtifactStructuralArmLocal
	ArtifactStructuralArmResume
	ArtifactStructuralArmTrue
	ArtifactStructuralArmFalse
	ArtifactStructuralArmTail
	ArtifactStructuralArmThrow
	ArtifactStructuralArmYield
	ArtifactStructuralArmCancel
)

func (arm ArtifactStructuralArm) Valid() bool {
	return arm >= ArtifactStructuralArmLocal && arm <= ArtifactStructuralArmCancel
}

// ArtifactEventKind is the engine-neutral bracket-stream vocabulary.
type ArtifactEventKind uint8

const (
	ArtifactEventInvalid ArtifactEventKind = iota
	ArtifactEventEnter
	ArtifactEventPoint
	ArtifactEventExit
)

// ArtifactScalarRole is one opaque Program-owned role in a reusable artifact
// template. A mounting owner compares the role identity but never interprets it
// as a domain producer tag.
type ArtifactScalarRole struct {
	semantic identity.ContentID
}

func (role ArtifactScalarRole) Available() bool { return role.semantic.Available() }

// ArtifactScalarFactor is one canonical Factor identity transported by a
// reusable Program artifact. It is deliberately distinct from a Rule role:
// local transfer semantics name the Factor itself, never a representative
// Rule whose output would have to be re-derived later.
type ArtifactScalarFactor struct {
	semantic identity.ContentID
}

func (factor ArtifactScalarFactor) Available() bool { return factor.semantic.Available() }

// ArtifactScalarCapacity reserves the exact immutable row planes one builder
// will fill. It is allocation shape only; final admission still validates
// every row and relation.
type ArtifactScalarCapacity struct {
	Roles, Factors, Points, Edges, Transfers, Regions, Events, Rules, Bodies int
}

type ArtifactScalarPoint struct {
	ID        identity.ContentID
	Decisions []identity.ContentID
	Initial   bool
}

type ArtifactScalarEdge struct {
	ID, From, To, Route, Guard, Decision identity.ContentID
	Component, Mu, Reset                 identity.ContentID
	Resets                               []identity.ContentID
	Arm                                  ArtifactStructuralArm
	Guarded, Truth, HasReset             bool
}

type ArtifactScalarTransfer struct {
	ID, From, To identity.ContentID
	Full         bool
	Factors      []ArtifactScalarFactor
}

type ArtifactScalarRegion struct {
	ID, Head, Parent identity.ContentID
	Cyclic           bool
	Members          []identity.ContentID
}

type ArtifactScalarEvent struct {
	Kind   ArtifactEventKind
	Region identity.ContentID
	Point  identity.ContentID
}

type ArtifactScalarRule struct {
	Role      ArtifactScalarRole
	Stage     schema.Key
	Point, ID identity.ContentID
	// Inputs is the ordered point-role vector issued by the Program artifact.
	// The fixed width mirrors the issuance instruction's six operand cells;
	// InputCount is the only authority for which prefix is present.
	Inputs     [6]identity.ContentID
	InputCount uint8
	Route      identity.ContentID
	Native     bool
}

// InputPointCount returns the dense number of point roles. A negative result
// denotes a malformed vector width; callers must reject it rather than scan
// for an available point.
func (rule ArtifactScalarRule) InputPointCount() int {
	if rule.InputCount > uint8(len(rule.Inputs)) {
		return -1
	}
	return int(rule.InputCount)
}

// InputPointAt resolves exactly one ordinal in the dense vector. It never
// substitutes another available point for a missing role.
func (rule ArtifactScalarRule) InputPointAt(index int) (identity.ContentID, bool) {
	if index < 0 {
		return identity.ContentID{}, false
	}
	if rule.InputCount > uint8(len(rule.Inputs)) || index >= int(rule.InputCount) {
		return identity.ContentID{}, false
	}
	return rule.Inputs[index], rule.Inputs[index].Available()
}

// ArtifactScalarBody is the engine-neutral body transport. It carries only the
// entry/exit point transport the engine consumes; the function declaration
// interface stays with program/artifact.
type ArtifactScalarBody struct {
	ID           identity.ContentID
	Entry, Exits []identity.ContentID
}
