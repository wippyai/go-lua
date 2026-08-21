// Package artifact is the owner-neutral structural declaration surface for one
// reusable Program artifact. It carries the scalar row vocabulary, the
// single-use template builder, and the relational admission fence that seals a
// template. It deliberately depends on no binding, schema, capability, or
// equation authority, so the same sealed template can be shared by independent
// Links and mounted repeatedly without rebuilding the Program interior.
package rows

import "github.com/wippyai/go-lua/analysis/identity"

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

// ArtifactRuleStage is the engine-neutral scalar encoding of the execution
// cut sealed by ProgramArtifact. It is retained as proof metadata; the mounting
// owner does not infer it from transports or interpret a domain rule name.
type ArtifactRuleStage uint8

const (
	ArtifactRuleStageInvalid ArtifactRuleStage = iota
	ArtifactRuleStageBase
	ArtifactRuleStageLocal
	// Issued native cuts. Schema structure CategoryIssuanceStage owns
	// spelling and predecessor; these are the dense ordinals that table
	// numbers.
	ArtifactRuleStageIssued3
	ArtifactRuleStageIssued4
	ArtifactRuleStageIssued5
)

func (stage ArtifactRuleStage) Valid() bool {
	return stage >= ArtifactRuleStageBase && stage <= ArtifactRuleStageIssued5
}

// ArtifactStageLaw is one declared execution-cut relation: whether the stage
// is a native-call cut and which stage must already own its input point.
type ArtifactStageLaw struct {
	Stage       ArtifactRuleStage
	Native      bool
	Predecessor ArtifactRuleStage
}

func (law ArtifactStageLaw) Valid() bool {
	return law.Stage.Valid() && (!law.Predecessor.Valid() || law.Predecessor != law.Stage)
}

// ArtifactScalarRole is one opaque Program-owned role in a reusable artifact
// template. A mounting owner compares the role identity but never interprets it
// as a domain producer tag.
type ArtifactScalarRole struct {
	semantic identity.ContentID
}

func (role ArtifactScalarRole) Available() bool { return role.semantic.Available() }

// ArtifactScalarCapacity reserves the exact immutable row planes one builder
// will fill. It is allocation shape only; final admission still validates
// every row and relation.
type ArtifactScalarCapacity struct {
	Roles, Points, Edges, Transfers, Regions, Events, Rules, Bodies int
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
	Factors      []ArtifactScalarRole
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
	Role             ArtifactScalarRole
	Stage            ArtifactRuleStage
	Point, Input, ID identity.ContentID
	Route            identity.ContentID
}

// ArtifactScalarBody is the engine-neutral body transport. It carries only the
// entry/exit point transport the engine consumes; the function declaration
// interface stays with program/artifact.
type ArtifactScalarBody struct {
	ID           identity.ContentID
	Entry, Exits []identity.ContentID
}
