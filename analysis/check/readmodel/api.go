// Package readmodel defines the syntax-free read surface consumed by
// post-solve obligation producers.
package readmodel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Reader is the reduced obligation-query surface. Implementations project
// solved analysis state into these data records; producers must not reach past
// this interface into body, syntax, or engine state internals.
type Reader interface {
	ForEachCall(func(CallSite) bool) bool
	CallCalleeReportAt(cfg.Point) (CallCalleeReport, bool)
	ForEachAssignment(func(Assignment) bool) bool
	ForEachOptionalAssignmentTarget(func(OptionalAssignmentTarget) bool) bool
	ForEachReturn(func(Return) bool) bool
	ForEachNonNilAssertion(func(NonNilAssertion) bool) bool
	ForEachNumericForOperand(func(NumericForOperand) bool) bool
	ForEachConcatOperand(func(ConcatOperand) bool) bool
	ForEachFrozenTableMutation(func(FrozenTableMutation) bool) bool
	ForEachLifecycleObligation(func(LifecycleObligation) bool) bool
	ForEachTypestateInvalidTransition(func(TypestateInvalidTransition) bool) bool
	ForEachTypestateRequirement(func(TypestateRequirement) bool) bool
	ForEachUnusedLocal(func(UnusedLocal) bool) bool
	ForEachDeadAssignment(func(DeadAssignment) bool) bool
	ForEachChannelSelectExhaustiveness(func(ChannelSelectExhaustiveness) bool) bool
	ForEachChannelLifecycleMisuse(func(ChannelLifecycleMisuse) bool) bool
	ForEachDiscriminatedUnionExhaustiveness(func(DiscriminatedUnionExhaustiveness) bool) bool
	ForEachOptionalExhaustiveness(func(OptionalExhaustiveness) bool) bool
	ForEachRegistrationExhaustiveness(func(RegistrationExhaustiveness) bool) bool
	ForEachTableDispatchExhaustiveness(func(TableDispatchExhaustiveness) bool) bool
	ForEachUnresolvedValueReference(func(UnresolvedValueReference) bool) bool
	ForEachUnresolvedTypeReference(func(UnresolvedTypeReference) bool) bool
	ForEachMissingMemberRead(func(MissingMemberRead) bool) bool
	ForEachResultShapeExhaustiveness(func(ResultShapeExhaustiveness) bool) bool
	ForEachRedundantConditionBranch(func(RedundantConditionBranch) bool) bool
	ForEachRedundantClaim(func(RedundantClaim) bool) bool
	ForEachAlwaysTrueGuard(func(AlwaysTrueGuard) bool) bool
	ForEachInvariantLoopRead(func(InvariantLoopRead) bool) bool
	ForEachSplitBirthDiscriminant(func(SplitBirthDiscriminant) bool) bool
	ForEachClosureCapture(func(ClosureCapture) bool) bool
	ForEachAllocationSite(func(AllocationSite) bool) bool
	ForEachHoistableLoad(func(HoistableLoad) bool) bool
	DominatingTruthyBranchForPath(cfg.Point, BranchCheck) (DominatingBranchProof, bool)
	DominatingBranchCheckForPath(cfg.Point, BranchCheck, func(BranchCheck, bool) bool) (DominatingBranchProof, bool)
}

// SourceSpan is a syntax-free source range exported by the read model.
type SourceSpan struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Valid reports whether the span points at a real source range.
func (s SourceSpan) Valid() bool {
	return s.StartLine != 0 || s.StartCol != 0 || s.EndLine != 0 || s.EndCol != 0
}

// BranchCheckKind is the syntax-free readmodel vocabulary for a lowered branch
// predicate. Lua syntax normalization owns producing these facts; obligation
// producers consume this DTO instead of importing the Lua lowering package.
type BranchCheckKind uint8

const (
	BranchCheckNone BranchCheckKind = iota
	BranchCheckTruthy
	BranchCheckFalsy
	BranchCheckNil
	BranchCheckNotNil
	BranchCheckTypeEqual
	BranchCheckTypeNot
	BranchCheckLiteralEqual
	BranchCheckLiteralNot
	BranchCheckPathEqual
	BranchCheckPathNot
	BranchCheckLenGe
	BranchCheckIndexInRange
	BranchCheckNumGe
	BranchCheckNumLe
)

// BranchCheck is a lowered branch predicate with no AST/source dependency.
type BranchCheck struct {
	Kind           BranchCheckKind
	Path           path.Path
	OtherPath      path.Path
	TypeName       string
	Literal        typ.Type
	LiteralString  string
	LenFloor       int64
	NumFloor       int64
	NumCeil        int64
	HasNumCeil     bool
	NumCeilNegated bool
	Negated        bool
}

func (c BranchCheck) LiteralValue() (typ.Type, bool) {
	if c.Literal != nil {
		return c.Literal, true
	}
	if c.Kind == BranchCheckLiteralEqual || c.Kind == BranchCheckLiteralNot {
		return typ.LiteralString(c.LiteralString), true
	}
	return nil, false
}

// RedundantConditionBranch is one normally reachable user-visible branch
// condition. It carries the lowered check plus source spans so obligation
// producers do not inspect syntax or body internals.
type RedundantConditionBranch struct {
	Point         cfg.Point
	Check         BranchCheck
	ConditionSpan SourceSpan
	StatementSpan SourceSpan
}

// RedundantClaim is one runtime claim/cast whose operand is independently
// proven to already satisfy the target type.
type RedundantClaim struct {
	Point        cfg.Point
	OperandLabel string
	ClaimLabel   string
	OperandType  typ.Type
	ClaimedType  typ.Type
	OperandSpan  SourceSpan
	ClaimSpan    SourceSpan
}

// AlwaysTrueGuard is one normally reachable branch condition whose abstract
// value is a singleton boolean.
type AlwaysTrueGuard struct {
	Point          cfg.Point
	Always         bool
	ConditionLabel string
	ConditionType  typ.Type
	ConditionSpan  SourceSpan
}

// InvariantLoopRead is one static member/index read inside a loop whose read
// path is stable through the loop body and whose receiver is non-nil.
type InvariantLoopRead struct {
	Point         cfg.Point
	LoopHead      cfg.Point
	ReadLabel     string
	ReceiverLabel string
	ReceiverPath  path.Path
	ReadPath      path.Path
	ReceiverType  typ.Type
	ReadSpan      SourceSpan
	LoopSpan      SourceSpan
}

// SplitBirthDiscriminant is one locally born table whose string tag field is
// assigned apart from other payload fields and later used as a discriminant.
type SplitBirthDiscriminant struct {
	Point                cfg.Point
	ReceiverLabel        string
	TagLabel             string
	TagValue             string
	BirthPoint           cfg.Point
	BirthSpan            SourceSpan
	TagWriteSpan         SourceSpan
	PayloadWrites        []SplitBirthPayloadWrite
	DiscriminantUsePoint cfg.Point
	DiscriminantUseSpan  SourceSpan
}

// SplitBirthPayloadWrite is one non-tag field write to the same split-born
// receiver.
type SplitBirthPayloadWrite struct {
	Point cfg.Point
	Label string
	Span  SourceSpan
}

const ClosureCaptureSchemaVersion = 2

const HoistableLoadSchemaVersion = 1

// HoistableLoad is the codegen-facing license for one member/index read whose
// loaded value is invariant across iterations of the identified loop. LoopSpan
// is the source-scope witness for the loop headed by LoopHead; absence of this
// record is the sound default.
type HoistableLoad struct {
	SchemaVersion int
	BodyID        uint64
	Point         cfg.Point
	ReadPath      path.Path
	LoopHead      cfg.Point
	LoopSpan      SourceSpan
}

// ClosureCapture is the codegen-facing solved export for one captured symbol at
// a closure creation site.
type ClosureCapture struct {
	SchemaVersion int
	Point         cfg.Point
	Function      uint64
	CaptureIndex  int
	Symbol        uint64
	Name          string
	Path          path.Path
	Policy        string

	Value product.Value

	Type    typ.Type
	HasType bool

	Shape       typ.Type
	HasShape    bool
	StableShape bool
	ShapeTier   string

	Nilable         bool
	NilabilityKnown bool

	Placement    placement.Value
	HasPlacement bool
	Identity     identity.ID
	HasIdentity  bool
}

const AllocationSiteSchemaVersion = 2

// AllocationSite is the codegen-facing solved export for one table allocation
// site. Decomposable is a scalar-replacement license; false is the sound
// default and may indicate either a proven blocker or unavailable proof.
type AllocationSite struct {
	SchemaVersion int
	Point         cfg.Point
	ExpressionID  uint64
	ExprRef       uint64
	Identity      identity.ID
	BirthPoint    cfg.Point
	BirthSpan     SourceSpan
	HasBirthSpan  bool

	Placement    placement.Value
	HasPlacement bool

	Shape       typ.Type
	Fields      []AllocationField
	StableShape bool

	Decomposable            bool
	FrameLocalUseProof      bool
	DiesBeforeSuspension    bool
	HasDiesBeforeSuspension bool
}

type AllocationField struct {
	Name string
	Type typ.Type
}

// DominatingBranchProof is the readmodel view of a prior branch edge that
// proves something about the same path at a later branch.
type DominatingBranchProof struct {
	Point cfg.Point
	Check BranchCheck
	Edge  bool
	Span  SourceSpan
}

type obligationActualPolicy struct {
	TypeWithPresence    typ.Type
	Expected            typ.Type
	UntrustedTopOrigin  bool
	ProvenMismatch      bool
	ShowUntrustedActual bool
}

func (p obligationActualPolicy) ActualTypeKnown() bool {
	return !typ.TypeEquals(p.TypeWithPresence, nil)
}

func (p obligationActualPolicy) EffectiveActualType() typ.Type {
	actual := p.TypeWithPresence
	if typ.TypeEquals(actual, nil) {
		actual = typ.Unknown
	}
	if p.UntrustedTopOrigin && !p.showConcreteUntrustedActual(actual) {
		return typ.Any
	}
	return actual
}

func (p obligationActualPolicy) showConcreteUntrustedActual(actual typ.Type) bool {
	if typ.IsAny(actual) || typ.IsUnknown(actual) {
		return false
	}
	if typ.TypeEquals(actual, typ.Nil) {
		return false
	}
	if p.ShowUntrustedActual {
		return true
	}
	if typetable.IsBuiltinTopMarker(actual) {
		return true
	}
	if !p.ShowUntrustedActual &&
		!typ.TypeEquals(p.Expected, nil) &&
		subtype.IsSubtype(actual, p.Expected) {
		return false
	}
	switch unwrap.Alias(unwrap.Annotations(actual)).(type) {
	case *typ.Record, *typ.Array, *typ.Tuple, *typ.Map, *typ.ReadonlyMap:
		return true
	default:
		return false
	}
}

func (p obligationActualPolicy) MissingProofRefuted() bool {
	return p.ProvenMismatch && !p.UntrustedTopOrigin
}

// Assignment is the solved read model for one annotated assignment target.
// It carries the target contract and source value evidence without exposing
// syntax or engine internals to obligation producers.
type Assignment struct {
	Point              cfg.Point
	TargetLabel        string
	SourceLabel        string
	TargetKey          string
	SourceKey          string
	SourceIndexedRead  bool
	Value              product.Value
	ValueHash          uint64
	TypeWithPresence   typ.Type
	Expected           typ.Type
	ExpectedLabel      string
	ExpectedSource     AssignmentExpectedSource
	SourceSpan         SourceSpan
	DeclarationSpan    SourceSpan
	NilableAccesses    []NilableAccessEvidence
	SourceContributors []AssignmentSourceContribution
	CallInvalidations  []AssignmentCallInvalidation
	CallResult         CallResultAssignmentSource
	ParentContext      AssignmentParentContext
	UntrustedTopOrigin bool
	ExplicitTopOrigin  bool
	RuntimeValidated   bool
	CascadeFromRefuted bool
	Check              AssignmentCheck
}

// CascadeFromRefutedAssignment reports whether this write is a dependent cascade
// of an earlier refuted assignment and should be suppressed before rendering.
func (a Assignment) CascadeFromRefutedAssignment() bool {
	return a.CascadeFromRefuted
}

// AssignmentParentContext records the parent object obligation that produced a
// projected member assignment. A renderer can explain both the focused member
// failure and the enclosing assignment without re-reading syntax.
type AssignmentParentContext struct {
	SourceLabel     string
	TargetLabel     string
	SourceType      typ.Type
	Expected        typ.Type
	SourceSpan      SourceSpan
	DeclarationSpan SourceSpan
}

// ActualTypeKnown reports whether the solved assignment source carried a
// concrete type witness at the write site.
func (a Assignment) ActualTypeKnown() bool {
	return a.actualPolicy().ActualTypeKnown()
}

// EffectiveActualType returns the type the obligation layer should attach to
// the assignment judgment. Missing solved types render as unknown; untrusted
// top flows stay visibly any when the checker cannot prove a concrete mismatch.
func (a Assignment) EffectiveActualType() typ.Type {
	return a.actualPolicy().EffectiveActualType()
}

// MissingProofRefuted reports whether the failed assignment obligation is a
// proven type contradiction rather than an untrusted-top precision boundary.
func (a Assignment) MissingProofRefuted() bool {
	return a.actualPolicy().MissingProofRefuted()
}

func (a Assignment) actualPolicy() obligationActualPolicy {
	return obligationActualPolicy{
		TypeWithPresence:   a.TypeWithPresence,
		Expected:           a.Expected,
		UntrustedTopOrigin: a.UntrustedTopOrigin,
		ProvenMismatch:     a.Check.ProvenMismatch,
	}
}

// CallResultAssignmentSource records that an assignment source is a specific
// result slot from a callable. It lets renderers explain call-result assignment
// failures without re-lowering callable contracts from syntax.
type CallResultAssignmentSource struct {
	Present       bool
	CallableName  string
	ResultIndex   int
	ReturnSpan    SourceSpan
	UnderSupplied bool
}

// AssignmentExpectedSource classifies where an assignment target obligation
// came from. Renderers use this to explain the authority without re-inspecting
// syntax or type structure.
type AssignmentExpectedSource uint8

const (
	AssignmentExpectedDeclared AssignmentExpectedSource = iota
	AssignmentExpectedDynamicTarget
)

// NilableAccessEvidence records an intermediate source access whose receiver
// may be nil before a later assignment reads from it.
type NilableAccessEvidence struct {
	Label  string
	Access string
	Span   SourceSpan
}

// AssignmentSourceContribution records a prior write that contributes one
// concrete arm to the assigned source value.
type AssignmentSourceContribution struct {
	RootLabel string
	ReadLabel string
	Type      typ.Type
	Span      SourceSpan
}

// AssignmentCallInvalidation records a prior call that invalidated the source
// read, making an earlier guard proof stale.
type AssignmentCallInvalidation struct {
	CallLabel        string
	ReadLabel        string
	InvalidatedLabel string
	Span             SourceSpan
}

// AssignmentCheck is the solved proof result for an assignment source against
// its declared target type.
type AssignmentCheck struct {
	Assignment     *Assignment
	Expected       typ.Type
	Admissible     bool
	ProvenMismatch bool
	Mismatch       AssignmentMismatch
}

// AssignmentCheckPlan carries already-solved proof inputs for one assignment.
type AssignmentCheckPlan struct {
	Assignment                Assignment
	ValueAdmissible           bool
	ValueProvenMismatch       bool
	MayBeNil                  bool
	MissingRequiredField      string
	MissingRequiredFieldType  typ.Type
	MissingRequiredMethod     string
	MissingRequiredMethodType typ.Type
	MethodMismatchName        string
	MethodMismatchExpected    typ.Type
	MethodMismatchActual      typ.Type
	IsSubtype                 func(typ.Type, typ.Type) bool
}

// AssignmentMismatchKind classifies a structural assignment mismatch reason
// discovered by the read model.
type AssignmentMismatchKind uint8

const (
	AssignmentMismatchMissingRequiredField AssignmentMismatchKind = iota + 1
	AssignmentMismatchMissingRequiredMethod
	AssignmentMismatchMethodType
	AssignmentMismatchMayBeNil
)

// AssignmentMismatch carries structured mismatch detail without diagnostic
// wording.
type AssignmentMismatch struct {
	Kind       AssignmentMismatchKind
	Field      string
	Type       typ.Type
	ActualType typ.Type
}

// OptionalAssignmentTarget is the solved read model for a write through an
// optional container, e.g. `bag.name = ...` where `bag` may be nil.
type OptionalAssignmentTarget struct {
	Point          cfg.Point
	ContainerLabel string
	TargetLabel    string
	TargetKey      string
	ContainerType  typ.Type
	ContainerSpan  SourceSpan
	TargetSpan     SourceSpan
}

// Return is the solved read model for one returned expression checked against
// an explicit function return annotation.
type Return struct {
	Point               cfg.Point
	Index               int
	Value               product.Value
	ValueHash           uint64
	TypeWithPresence    typ.Type
	Expected            typ.Type
	ExpectedLabel       string
	SourceLabel         string
	SourceIndexedRead   bool
	SourceSpan          SourceSpan
	DeclarationSpan     SourceSpan
	UntrustedTopOrigin  bool
	ExplicitTopOrigin   bool
	BodyParamObligation bool
	Check               ReturnCheck
}

// HasUnownedTopActual reports whether the return source is only absent,
// unknown, or gradual without an explicit user assertion. Such values do not
// carry enough authority for a return diagnostic under the default obligation
// policy.
func (ret Return) HasUnownedTopActual() bool {
	if ret.ExplicitTopOrigin {
		return false
	}
	return typ.TypeEquals(ret.TypeWithPresence, nil) ||
		typ.IsAny(ret.TypeWithPresence) ||
		typ.IsUnknown(ret.TypeWithPresence)
}

// ActualTypeKnown reports whether the solved return source carried a concrete
// type witness at the return site.
func (ret Return) ActualTypeKnown() bool {
	return ret.actualPolicy().ActualTypeKnown()
}

// EffectiveActualType returns the type the obligation layer should attach to
// the return judgment. Missing solved types render as unknown; untrusted top
// flows stay visibly any when the checker cannot prove a concrete mismatch.
func (ret Return) EffectiveActualType() typ.Type {
	return ret.actualPolicy().EffectiveActualType()
}

// MissingProofRefuted reports whether the failed return obligation is a proven
// type contradiction rather than an untrusted-top precision boundary.
func (ret Return) MissingProofRefuted() bool {
	return ret.actualPolicy().MissingProofRefuted()
}

// BodyParamObligationCascade reports whether this return mismatch is the
// internal top/unknown value produced by a body-owned parameter precondition.
// The call-boundary argument obligation owns the user-facing diagnostic for
// that precondition.
func (ret Return) BodyParamObligationCascade() bool {
	if !ret.BodyParamObligation || (!ret.UntrustedTopOrigin && !ret.ExplicitTopOrigin) {
		return false
	}
	return typ.TypeEquals(ret.TypeWithPresence, nil) ||
		typ.TypeEquals(ret.TypeWithPresence, typ.Nil) ||
		typ.IsAny(ret.TypeWithPresence) ||
		typ.IsUnknown(ret.TypeWithPresence)
}

func (ret Return) actualPolicy() obligationActualPolicy {
	return obligationActualPolicy{
		TypeWithPresence:    ret.TypeWithPresence,
		Expected:            ret.Expected,
		UntrustedTopOrigin:  ret.UntrustedTopOrigin,
		ProvenMismatch:      ret.Check.ProvenMismatch,
		ShowUntrustedActual: true,
	}
}

// ReturnCheck is the solved proof result for one returned value against its
// declared return type.
type ReturnCheck struct {
	Return         *Return
	Expected       typ.Type
	Admissible     bool
	ProvenMismatch bool
	Mismatch       ReturnMismatch
}

// ReturnCheckPlan carries already-solved proof inputs for one returned value.
type ReturnCheckPlan struct {
	Return                   Return
	ValueAdmissible          bool
	ValueProvenMismatch      bool
	MissingRequiredField     string
	MissingRequiredFieldType typ.Type
	IsSubtype                func(typ.Type, typ.Type) bool
}

type ReturnMismatchKind uint8

const (
	ReturnMismatchMissingRequiredField ReturnMismatchKind = iota + 1
	ReturnMismatchMayBeNil
)

type ReturnMismatch struct {
	Kind  ReturnMismatchKind
	Field string
	Type  typ.Type
}

// NonNilAssertion is the solved read model for one `expr!` runtime assertion.
// It exposes the operand's proved type at the assertion point without requiring
// obligation producers to inspect syntax or rebuild flow facts.
type NonNilAssertion struct {
	Point            cfg.Point
	OperandLabel     string
	OperandKey       string
	Value            product.Value
	ValueHash        uint64
	TypeWithPresence typ.Type
	OperandNilOnly   bool
	OperandSpan      SourceSpan
	AssertionSpan    SourceSpan
}

// NonNilAssertionOperandNilOnly reports whether an assertion operand is
// statically nil on every normally reachable path. Gradual and unknown values
// remain inconclusive so the assertion is not reported as definitely failing.
func NonNilAssertionOperandNilOnly(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	return typ.Nil.Equals(unwrap.NormalizeNil(t))
}

// ConcatOperand is the solved read model for one `..` operand whose static
// projection still includes nil at the operation boundary.
type ConcatOperand struct {
	Point            cfg.Point
	Side             string
	OperandLabel     string
	OperandKey       string
	TypeWithPresence typ.Type
	OperandSpan      SourceSpan
}

// NilRisk reports whether the operand projection can include nil and should be
// surfaced as a runtime concat risk.
func (o ConcatOperand) NilRisk() bool {
	return ConcatOperandNilRisk(o.TypeWithPresence)
}

// ConcatOperandNilRisk reports whether a projected operand type can include nil
// and is concrete enough to report.
func ConcatOperandNilRisk(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	return ProjectionHasNil(t)
}

// ProjectionHasNil reports whether the projected display type still admits nil.
// Diagnostics use this public readmodel wrapper instead of reaching into the
// value-domain projection packages directly.
func ProjectionHasNil(t typ.Type) bool {
	return typevalue.ProjectionHasNil(t)
}

// ProjectionWithoutNil returns the display projection with nil removed.
func ProjectionWithoutNil(t typ.Type) typ.Type {
	return proof.ProjectionWithoutNil(t)
}

// OptionalTypeHasConcreteValue reports whether t is a concrete optional-like
// projection with both a nil arm and a non-never value arm. Gradual and unknown
// projections are intentionally inconclusive.
func OptionalTypeHasConcreteValue(t typ.Type) bool {
	return proof.OptionalTypeHasConcreteValue(t)
}

// OptionalTruthinessPartitionsNilValue reports whether truthiness checks can
// split an optional-like type into nil and value cases. If the value arm may be
// false, truthiness cannot prove the nil arm was handled.
func OptionalTruthinessPartitionsNilValue(t typ.Type) bool {
	return proof.OptionalTruthinessPartitionsNilValue(t)
}

// NonNilProjectionProvesMismatch reports whether the non-nil arm of got still
// fails the expected contract. Renderers use this presentation helper to choose
// nilability wording without owning subtype/projection proof logic.
func NonNilProjectionProvesMismatch(got, want typ.Type) bool {
	if got == nil || want == nil {
		return false
	}
	present := ProjectionWithoutNil(got)
	if present == nil || typ.TypeEquals(present, got) ||
		typ.IsAny(present) || typ.IsUnknown(present) || typ.IsNever(present) {
		return false
	}
	return !subtype.IsSubtype(present, want)
}

// NumericForOperand is the solved read model for one numeric-for operand
// (`init`, `limit`, or `step`). It carries the operand type and source role
// without exposing syntax to obligation producers.
type NumericForOperand struct {
	Point               cfg.Point
	Role                string
	OperandLabel        string
	OperandKey          string
	TypeWithPresence    typ.Type
	OperandSpan         SourceSpan
	ExplicitTopLikeCast bool
	DefinitelyNotNumber bool
}

// NumericForDefinitelyNotNumber reports whether a numeric-for operand type is
// precise enough to prove the operand cannot be a number. Gradual, unknown, nil,
// and partly numeric unions stay admissible at this layer; the obligation pass
// should only emit when this proof is true.
func NumericForDefinitelyNotNumber(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	if base := unwrap.Optional(t); typ.IsNever(base) {
		return false
	}
	if subtype.IsSubtype(t, typ.Number) {
		return false
	}
	return !numericForMayContainNumber(t, 0)
}

func numericForMayContainNumber(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	if typ.IsNever(t) {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	if subtype.IsSubtype(t, typ.Number) {
		return true
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return numericForMayContainNumber(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return numericForMayContainNumber(v.Inner, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if numericForMayContainNumber(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded == nil || expanded == t || numericForMayContainNumber(expanded, depth+1)
	default:
		switch v.Kind() {
		case kind.Nil, kind.Boolean, kind.String, kind.Function, kind.Array, kind.Map, kind.Record, kind.Tuple, kind.ReadonlyMap:
			return false
		case kind.Literal:
			return subtype.IsSubtype(v, typ.Number)
		default:
			return true
		}
	}
}

// FrozenTableMutationKind classifies how a frozen table is mutated.
type FrozenTableMutationKind uint8

const (
	FrozenTableMutationAssignment FrozenTableMutationKind = iota + 1
	FrozenTableMutationCall
)

// FrozenTableMutation is the solved read model for a write or call that mutates
// a table identity already proved frozen at that program point.
type FrozenTableMutation struct {
	Point              cfg.Point
	Kind               FrozenTableMutationKind
	ContainerLabel     string
	ContainerKey       string
	MutationSpan       SourceSpan
	FreezeProofSpan    SourceSpan
	HasFreezeProofSpan bool
}

// LifecycleSiteKind classifies a typestate lifecycle fact site.
type LifecycleSiteKind uint8

const (
	LifecycleSiteAcquire LifecycleSiteKind = iota + 1
	LifecycleSiteTransition
	LifecycleSiteEscape
)

// LifecycleSite records one reachable lifecycle fact that contributes evidence
// to an open typestate obligation at function exit.
type LifecycleSite struct {
	Point       cfg.Point
	Kind        LifecycleSiteKind
	Resource    string
	Protocol    string
	From        string
	To          string
	TargetLabel string
	Span        SourceSpan
}

// LifecycleObligation is the solved read model for a resource whose typestate
// obligation remains open at function exit.
type LifecycleObligation struct {
	Point    cfg.Point
	Resource string
	Protocol string
	Current  string
	Finals   []string
	Sites    []LifecycleSite
}

// TypestateInvalidTransition is the solved read model for a call whose
// declared lifecycle source state is provably not the resource's current
// state. It is generic across every declared typestate protocol.
type TypestateInvalidTransition struct {
	Point    cfg.Point
	Span     SourceSpan
	Resource string
	Protocol string
	Expected string
	Found    string
	Target   string
}

// TypestateRequirement is the solved read model for one declared lifecycle
// call-entry precondition that remains unknown or is proven false.
type TypestateRequirement struct {
	Point    cfg.Point
	Span     SourceSpan
	Resource string
	Protocol string
	Expected string
	Found    string
	Target   string
	Refuted  bool
}

// UnusedLocal is the solved read model for a local declaration whose symbol has
// no reachable read in its scope.
type UnusedLocal struct {
	Point cfg.Point
	Name  string
	Key   string
	Span  SourceSpan
}

// DeadAssignment is the solved read model for a write whose assigned value is
// discarded before any reachable read on every normal path.
type DeadAssignment struct {
	Point      cfg.Point
	Name       string
	Key        string
	WriteSpan  SourceSpan
	Overwrites []DeadAssignmentOverwrite
	Exits      []DeadAssignmentExit
}

// DeadAssignmentOverwrite is one later write on the all-path frontier that
// replaces the earlier assigned value before it can be read.
type DeadAssignmentOverwrite struct {
	Point cfg.Point
	Span  SourceSpan
}

// DeadAssignmentExit is one all-path frontier exit that leaves the value
// unread. Span may be zero when the exit is the synthetic function exit.
type DeadAssignmentExit struct {
	Point cfg.Point
	Span  SourceSpan
}

// ChannelSelectExhaustiveness is the solved read model for an elseif chain
// that handles only part of a channel.select result without a default case.
type ChannelSelectExhaustiveness struct {
	Point         cfg.Point
	Span          SourceSpan
	ResultChannel string
	Handled       []string
	Missing       []string
	HasDefault    bool
}

// ChannelLifecycleOperation classifies a channel operation whose receiver is
// invalid in the proven lifecycle state.
type ChannelLifecycleOperation string

const (
	ChannelLifecycleSend  ChannelLifecycleOperation = "send"
	ChannelLifecycleClose ChannelLifecycleOperation = "close"
)

// ChannelLifecycleMisuse is the solved read model for a runtime channel
// operation whose receiver is provably closed.
type ChannelLifecycleMisuse struct {
	Point     cfg.Point
	Span      SourceSpan
	Operation ChannelLifecycleOperation
	Channel   string
	State     string
}

// DiscriminatedUnionExhaustiveness is the solved read model for an if/elseif
// branch chain that checks some, but not all, cases of a discriminated union and
// has no default else branch.
type DiscriminatedUnionExhaustiveness struct {
	Point    cfg.Point
	Span     SourceSpan
	Target   string
	Possible []string
	Handled  []string
	Missing  []string
}

// OptionalExhaustiveness is the solved read model for a branch chain that
// consumes the present/value case of an optional path but has no branch/default
// that handles the nil case.
type OptionalExhaustiveness struct {
	Point   cfg.Point
	Span    SourceSpan
	Target  string
	Missing []string
}

// RegistrationExhaustiveness is the solved read model for callback registries
// that register handlers for only part of a discriminated union before a
// dispatch call.
type RegistrationExhaustiveness struct {
	Point             cfg.Point
	Registry          string
	Target            string
	Possible          []string
	Registered        []string
	Missing           []string
	MissingFor        []string
	RegistrationSpan  SourceSpan
	RegistrationSpans []SourceSpan
	DispatchSpan      SourceSpan
}

// TableDispatchExhaustiveness is the solved read model for dispatch tables
// indexed by discriminated-union cases while missing one or more case keys.
type TableDispatchExhaustiveness struct {
	Point      cfg.Point
	Table      string
	Target     string
	Possible   []string
	Keys       []string
	Missing    []string
	MissingFor []string
	TableSpan  SourceSpan
	LookupSpan SourceSpan
}

// UnresolvedValueReference is the solved read model for an identifier read that
// binding left as an implicit global and no configured/imported/type namespace
// proves valid in this scope.
type UnresolvedValueReference struct {
	Point cfg.Point
	Name  string
	Key   string
	Span  SourceSpan
}

// UnresolvedTypeReference is an annotation type name that binding could not
// resolve in the current lexical/module type namespace.
type UnresolvedTypeReference struct {
	Point cfg.Point
	Name  string
	Key   string
	Span  SourceSpan
}

// MissingMemberRead is the solved read model for a static member read whose
// receiver is known to reject the member on this path.
type MissingMemberRead struct {
	Point        cfg.Point
	ReadLabel    string
	MemberName   string
	ReceiverType typ.Type
	Span         SourceSpan
}

// ResultShapeExhaustiveness is the solved read model for a case-specific field
// read on a discriminated union when no dominating proof establishes the
// required case.
type ResultShapeExhaustiveness struct {
	Point         cfg.Point
	ReceiverLabel string
	ReadLabel     string
	Discriminant  string
	RequiredCase  string
	Span          SourceSpan
}

// PlanReturnCheck returns the complete proof result for one returned value.
func PlanReturnCheck(plan ReturnCheckPlan) ReturnCheck {
	ret := plan.Return
	var mismatch ReturnMismatch
	if plan.MissingRequiredField != "" {
		mismatch = ReturnMismatch{
			Kind:  ReturnMismatchMissingRequiredField,
			Field: plan.MissingRequiredField,
			Type:  plan.MissingRequiredFieldType,
		}
	} else if TypeMayBeNilMismatch(ret.TypeWithPresence, ret.Expected) {
		mismatch = ReturnMismatch{Kind: ReturnMismatchMayBeNil}
	}
	return ReturnCheck{
		Return:         &ret,
		Expected:       ret.Expected,
		Admissible:     plan.returnProofAdmissible(),
		ProvenMismatch: plan.returnProvenMismatch(),
		Mismatch:       mismatch,
	}
}

func (plan ReturnCheckPlan) returnProvenMismatch() bool {
	if plan.ValueProvenMismatch || plan.MissingRequiredField != "" {
		return true
	}
	if typ.TypeEquals(plan.Return.TypeWithPresence, nil) ||
		typ.TypeEquals(plan.Return.Expected, nil) ||
		plan.IsSubtype == nil ||
		plan.Return.UntrustedTopOrigin {
		return false
	}
	if typ.IsAny(plan.Return.TypeWithPresence) ||
		typ.IsUnknown(plan.Return.TypeWithPresence) ||
		typ.IsNever(plan.Return.TypeWithPresence) {
		return false
	}
	return !plan.IsSubtype(plan.Return.TypeWithPresence, plan.Return.Expected)
}

func (plan ReturnCheckPlan) returnProofAdmissible() bool {
	if plan.MissingRequiredField != "" {
		return false
	}
	if plan.Return.UntrustedTopOrigin && typ.TypeEquals(plan.Return.TypeWithPresence, nil) {
		return false
	}
	if plan.ValueAdmissible {
		return true
	}
	if !typ.TypeEquals(plan.Return.TypeWithPresence, nil) &&
		!typ.TypeEquals(plan.Return.Expected, nil) &&
		plan.IsSubtype != nil &&
		!typ.IsAny(plan.Return.TypeWithPresence) &&
		!typ.IsUnknown(plan.Return.TypeWithPresence) &&
		!typ.IsNever(plan.Return.TypeWithPresence) &&
		!plan.IsSubtype(plan.Return.TypeWithPresence, plan.Return.Expected) {
		return false
	}
	if plan.Return.UntrustedTopOrigin || typ.TypeEquals(plan.Return.TypeWithPresence, nil) || typ.TypeEquals(plan.Return.Expected, nil) || plan.IsSubtype == nil {
		return false
	}
	if typ.IsAny(plan.Return.TypeWithPresence) || typ.IsUnknown(plan.Return.TypeWithPresence) || typ.IsNever(plan.Return.TypeWithPresence) {
		return false
	}
	return plan.IsSubtype(plan.Return.TypeWithPresence, plan.Return.Expected)
}

// PlanAssignmentCheck returns the complete proof result for one annotated
// assignment source against the declared target type.
func PlanAssignmentCheck(plan AssignmentCheckPlan) AssignmentCheck {
	assignment := plan.Assignment
	var mismatch AssignmentMismatch
	if plan.MissingRequiredField != "" {
		mismatch = AssignmentMismatch{
			Kind:  AssignmentMismatchMissingRequiredField,
			Field: plan.MissingRequiredField,
			Type:  plan.MissingRequiredFieldType,
		}
	} else if plan.MissingRequiredMethod != "" {
		mismatch = AssignmentMismatch{
			Kind:  AssignmentMismatchMissingRequiredMethod,
			Field: plan.MissingRequiredMethod,
			Type:  plan.MissingRequiredMethodType,
		}
	} else if plan.MethodMismatchName != "" {
		mismatch = AssignmentMismatch{
			Kind:       AssignmentMismatchMethodType,
			Field:      plan.MethodMismatchName,
			Type:       plan.MethodMismatchExpected,
			ActualType: plan.MethodMismatchActual,
		}
	} else if plan.MayBeNil || AssignmentMayBeNilMismatch(assignment.TypeWithPresence, assignment.Expected) {
		mismatch = AssignmentMismatch{Kind: AssignmentMismatchMayBeNil}
	}
	return AssignmentCheck{
		Assignment:     &assignment,
		Expected:       assignment.Expected,
		Admissible:     plan.assignmentProofAdmissible(),
		ProvenMismatch: plan.assignmentProvenMismatch(),
		Mismatch:       mismatch,
	}
}

func (plan AssignmentCheckPlan) assignmentProvenMismatch() bool {
	if plan.ValueProvenMismatch || plan.MayBeNil || plan.MissingRequiredField != "" || plan.MissingRequiredMethod != "" || plan.MethodMismatchName != "" {
		return true
	}
	if typ.TypeEquals(plan.Assignment.TypeWithPresence, nil) ||
		typ.TypeEquals(plan.Assignment.Expected, nil) ||
		plan.IsSubtype == nil ||
		plan.Assignment.UntrustedTopOrigin {
		return false
	}
	if typ.IsAny(plan.Assignment.TypeWithPresence) ||
		typ.IsUnknown(plan.Assignment.TypeWithPresence) ||
		typ.IsNever(plan.Assignment.TypeWithPresence) {
		return false
	}
	return !plan.IsSubtype(plan.Assignment.TypeWithPresence, plan.Assignment.Expected)
}

func (plan AssignmentCheckPlan) assignmentProofAdmissible() bool {
	if plan.MissingRequiredField != "" || plan.MissingRequiredMethod != "" || plan.MethodMismatchName != "" {
		return false
	}
	if plan.Assignment.UntrustedTopOrigin && !plan.Assignment.RuntimeValidated {
		return false
	}
	if plan.ValueAdmissible {
		return true
	}
	if !typ.TypeEquals(plan.Assignment.TypeWithPresence, nil) &&
		!typ.TypeEquals(plan.Assignment.Expected, nil) &&
		plan.IsSubtype != nil &&
		!typ.IsAny(plan.Assignment.TypeWithPresence) &&
		!typ.IsUnknown(plan.Assignment.TypeWithPresence) &&
		!typ.IsNever(plan.Assignment.TypeWithPresence) &&
		!plan.IsSubtype(plan.Assignment.TypeWithPresence, plan.Assignment.Expected) {
		return false
	}
	if plan.Assignment.UntrustedTopOrigin || typ.TypeEquals(plan.Assignment.TypeWithPresence, nil) || typ.TypeEquals(plan.Assignment.Expected, nil) || plan.IsSubtype == nil {
		return false
	}
	if typ.IsAny(plan.Assignment.TypeWithPresence) || typ.IsUnknown(plan.Assignment.TypeWithPresence) || typ.IsNever(plan.Assignment.TypeWithPresence) {
		return false
	}
	return plan.IsSubtype(plan.Assignment.TypeWithPresence, plan.Assignment.Expected)
}

// AssignmentMayBeNilMismatch reports whether an assignment source may be nil
// while the declared target type rejects nil.
func AssignmentMayBeNilMismatch(got, want typ.Type) bool {
	return TypeMayBeNilMismatch(got, want)
}

// CallSite is the solved read model for one call expression. It is the public
// obligation input: producers should consume this assembled record instead of
// rebuilding a call from independent reader queries.
type CallSite struct {
	Point      cfg.Point
	CallSpan   SourceSpan
	CalleeSpan SourceSpan
	Arguments  []CallArgument
	SendSafety []SendSafety
	Reports    []CallArgumentReport
	Arity      CallArityReport
	Callee     CallCalleeReport
}

// SendSafetyVerdict classifies whether a call-boundary send payload is
// eligible for zero-copy transfer from solved facts.
type SendSafetyVerdict uint8

const (
	SendSafetyUnknown SendSafetyVerdict = iota
	SendSafetyProvenIsolated
	SendSafetyProvenImmutable
)

func (v SendSafetyVerdict) String() string {
	switch v {
	case SendSafetyProvenIsolated:
		return "isolated"
	case SendSafetyProvenImmutable:
		return "immutable"
	case SendSafetyUnknown:
		return "copy-fallback"
	default:
		return "send-safety(invalid)"
	}
}

// SendSafety is the solved read model for a send/spawn payload admission check.
// Unknown is a successful checker outcome: the runtime copies/promotes instead
// of taking the zero-copy path.
type SendSafety struct {
	Point               cfg.Point
	Argument            CallArgument
	Target              path.Path
	Recursive           bool
	Verdict             SendSafetyVerdict
	Reason              string
	Identity            identity.ID
	HasIdentity         bool
	BirthSpan           SourceSpan
	HasBirthSpan        bool
	Placement           placement.Value
	HasPlacement        bool
	Frozen              bool
	DirectObjectLiteral bool
	GraphHasChildID     bool
	GraphUnknown        bool
}

// CallArityReportKind classifies a solved call-arity mismatch.
type CallArityReportKind uint8

const (
	CallArityReportNone CallArityReportKind = iota
	CallArityReportTooFew
	CallArityReportTooMany
)

// CallArityReport is the solved read model for a call argument-count
// obligation. It carries counts and source anchors only; renderers own wording
// and severity.
type CallArityReport struct {
	Kind            CallArityReportKind
	CallableName    string
	ExpectedCount   int
	ActualCount     int
	CallSpan        SourceSpan
	DeclarationSpan SourceSpan
	ExtraSpan       SourceSpan
}

// CallArityReportPlan carries the syntax-free inputs needed to classify a call
// arity report. Internal readmodels own extracting counts/spans; public
// readmodel owns the reporting decision.
type CallArityReportPlan struct {
	HasContract    bool
	CallableName   string
	ActualCount    int
	RequiredCount  int
	FixedCount     int
	HasVararg      bool
	CallSpan       SourceSpan
	ParameterSpans []SourceSpan
	ArgumentSpans  []SourceSpan
}

// PlanCallArityReport returns the call arity report for plan, or the zero
// report when the call satisfies the callable contract.
func PlanCallArityReport(plan CallArityReportPlan) CallArityReport {
	if !plan.HasContract {
		return CallArityReport{}
	}
	if plan.ActualCount < plan.RequiredCount {
		return CallArityReport{
			Kind:            CallArityReportTooFew,
			CallableName:    plan.CallableName,
			ExpectedCount:   plan.RequiredCount,
			ActualCount:     plan.ActualCount,
			CallSpan:        plan.CallSpan,
			DeclarationSpan: sourceSpanAt(plan.ParameterSpans, plan.ActualCount),
		}
	}
	if !plan.HasVararg && plan.ActualCount > plan.FixedCount {
		return CallArityReport{
			Kind:            CallArityReportTooMany,
			CallableName:    plan.CallableName,
			ExpectedCount:   plan.FixedCount,
			ActualCount:     plan.ActualCount,
			CallSpan:        plan.CallSpan,
			DeclarationSpan: sourceSpanAt(plan.ParameterSpans, plan.FixedCount-1),
			ExtraSpan:       sourceSpanAt(plan.ArgumentSpans, plan.FixedCount),
		}
	}
	return CallArityReport{}
}

func sourceSpanAt(spans []SourceSpan, index int) SourceSpan {
	if index < 0 || index >= len(spans) {
		return SourceSpan{}
	}
	return spans[index]
}

// CallCalleeReportKind classifies a solved direct-callee callable mismatch.
type CallCalleeReportKind uint8

const (
	CallCalleeReportNone CallCalleeReportKind = iota
	CallCalleeReportNotCallable
	CallCalleeReportMayBeNil
	CallCalleeReportMissingMember
)

// CallCalleeReport is the solved read model for a call target obligation.
// Plain callees use this path for nil/non-callable targets. Member callees use
// it for non-callable targets, optional method receivers, and conservative
// missing-member shape reports when solved receiver evidence is precise enough.
type CallCalleeReport struct {
	Kind         CallCalleeReportKind
	CallableName string
	Type         typ.Type
	Callable     bool
	MemberAccess bool
	MemberName   string
	Span         SourceSpan
}

// CallCalleeReportPlan carries the solved direct-callee information needed to
// classify callable failures. Internal readmodels own resolving the callee
// value; public readmodel owns deciding if it should report.
type CallCalleeReportPlan struct {
	CallableName                 string
	Type                         typ.Type
	Callable                     bool
	MemberAccess                 bool
	NilableReceiver              bool
	ImpreciseMemberRequiresProof bool
	Span                         SourceSpan
	CallSpan                     SourceSpan
}

// PlanCallCalleeReport returns the direct-callee report for plan, or zero when
// the callee is definitely callable or too imprecise to report.
func PlanCallCalleeReport(plan CallCalleeReportPlan) CallCalleeReport {
	if plan.Type == nil || typ.IsNever(plan.Type) {
		return CallCalleeReport{}
	}
	imprecise := typ.IsAny(plan.Type) || typ.IsUnknown(plan.Type)
	if imprecise && !plan.ImpreciseMemberRequiresProof {
		return CallCalleeReport{}
	}
	name := plan.CallableName
	if name == "" {
		name = "call target"
	}
	span := sourceSpanOr(plan.Span, plan.CallSpan)
	if typevalue.TypeIncludesNil(plan.Type) && (plan.Callable || plan.NilableReceiver) {
		return CallCalleeReport{
			Kind:         CallCalleeReportMayBeNil,
			CallableName: name,
			Type:         plan.Type,
			Callable:     plan.Callable,
			MemberAccess: plan.MemberAccess,
			Span:         span,
		}
	}
	if !plan.Callable {
		return CallCalleeReport{
			Kind:         CallCalleeReportNotCallable,
			CallableName: name,
			Type:         plan.Type,
			MemberAccess: plan.MemberAccess,
			Span:         span,
		}
	}
	return CallCalleeReport{}
}

func sourceSpanOr(primary, fallback SourceSpan) SourceSpan {
	if primary.StartLine != 0 {
		return primary
	}
	return fallback
}

// CallCalleeDeclaredTypeMoreInformative reports whether a declared callee type
// should replace the solved boundary value type for callee reporting. This
// preserves the callable half of an optional declared callee when the solved
// value at the call site is nil.
func CallCalleeDeclaredTypeMoreInformative(valueType, declared typ.Type) bool {
	if declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) {
		return false
	}
	return typ.TypeEquals(valueType, typ.Nil) && typevalue.TypeIncludesNil(declared)
}

// CallCalleeDeclaredNilOwnedByDeclaration reports whether a nil solved callee
// value is already owned by the root local's non-nil callable declaration. The
// write/declaration contract should produce the user-facing diagnostic; the
// later call through that local would only be a cascade.
func CallCalleeDeclaredNilOwnedByDeclaration(valueType, declared typ.Type) bool {
	if declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) {
		return false
	}
	if !typ.TypeEquals(valueType, typ.Nil) || typevalue.TypeIncludesNil(declared) {
		return false
	}
	_, ok := typecall.Callable(declared)
	return ok
}

// CallArgument is the solved read model for one call argument.
type CallArgument struct {
	Index                     int
	Value                     product.Value
	ValueHash                 uint64
	TypeWithPresence          typ.Type
	UntrustedTopOrigin        bool
	ExplicitTopOrigin         bool
	RuntimeValidated          bool
	ProofCandidateValue       product.Value
	ProofCandidateHash        uint64
	ProofCandidateType        typ.Type
	ProofCandidateTop         bool
	ProofCandidateExplicitTop bool
	ProofCandidateRuntime     bool
	HasProofCandidate         bool
	ExpandedSource            bool
	CallerOwnedParameter      bool
	FunctionType              *typ.Function
	Span                      SourceSpan
	Label                     string
	Mismatch                  CallArgumentMismatch
}

// CallArgumentMismatchKind classifies a structural argument mismatch reason
// discovered by the read model.
type CallArgumentMismatchKind uint8

const (
	CallArgumentMismatchNone CallArgumentMismatchKind = iota
	CallArgumentMismatchMissingRequiredField
	CallArgumentMismatchMissingRequiredMethod
	CallArgumentMismatchMethodType
	CallArgumentMismatchMayBeNil
)

// CallArgumentMismatch carries structured mismatch detail without diagnostic
// wording.
type CallArgumentMismatch struct {
	Kind       CallArgumentMismatchKind
	Field      string
	Type       typ.Type
	ActualType typ.Type
}

// CallArgumentMismatchCandidate is one nested argument member that may become
// the report subject for an argument mismatch.
type CallArgumentMismatchCandidate struct {
	Argument    CallArgument
	Expected    typ.Type
	LabelSuffix string
	Admissible  bool
}

// CallArgumentMismatchSubjectPlan carries pre-projected object-literal
// mismatch candidates. Internal readmodels own extracting member values and
// expected member types; public readmodel owns which candidate becomes the
// user-facing report subject.
type CallArgumentMismatchSubjectPlan struct {
	Argument                  CallArgument
	Expected                  typ.Type
	Candidates                []CallArgumentMismatchCandidate
	MissingRequiredField      string
	MissingRequiredMethod     string
	MissingRequiredMethodType typ.Type
	MethodMismatchName        string
	MethodMismatchExpected    typ.Type
	MethodMismatchActual      typ.Type
}

// CallArgumentMismatchSubject is the selected report subject for one argument
// mismatch.
type CallArgumentMismatchSubject struct {
	Argument    CallArgument
	Expected    typ.Type
	LabelSuffix string
}

// PlanCallArgumentMismatchSubject selects the best user-facing subject for a
// call-argument mismatch. The first failing nested member wins; when all
// present members are admissible, a missing required field becomes the subject.
func PlanCallArgumentMismatchSubject(plan CallArgumentMismatchSubjectPlan) (CallArgumentMismatchSubject, bool) {
	for _, candidate := range plan.Candidates {
		if candidate.Expected == nil || candidate.Admissible {
			continue
		}
		return CallArgumentMismatchSubject{
			Argument:    candidate.Argument,
			Expected:    candidate.Expected,
			LabelSuffix: candidate.LabelSuffix,
		}, true
	}
	if plan.MissingRequiredField != "" {
		arg := plan.Argument
		arg.Mismatch = CallArgumentMismatch{
			Kind:  CallArgumentMismatchMissingRequiredField,
			Field: plan.MissingRequiredField,
		}
		return CallArgumentMismatchSubject{
			Argument:    arg,
			Expected:    plan.Expected,
			LabelSuffix: CallArgumentExpectedLabelSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: plan.MissingRequiredField}}),
		}, true
	}
	if plan.MissingRequiredMethod != "" {
		arg := plan.Argument
		arg.Mismatch = CallArgumentMismatch{
			Kind:  CallArgumentMismatchMissingRequiredMethod,
			Field: plan.MissingRequiredMethod,
			Type:  plan.MissingRequiredMethodType,
		}
		return CallArgumentMismatchSubject{Argument: arg, Expected: plan.Expected}, true
	}
	if plan.MethodMismatchName != "" {
		arg := plan.Argument
		arg.Mismatch = CallArgumentMismatch{
			Kind:       CallArgumentMismatchMethodType,
			Field:      plan.MethodMismatchName,
			Type:       plan.MethodMismatchExpected,
			ActualType: plan.MethodMismatchActual,
		}
		return CallArgumentMismatchSubject{Argument: arg, Expected: plan.Expected}, true
	}
	return CallArgumentMismatchSubject{}, false
}

// CallArgumentMayBeNilMismatch reports whether an argument may be nil while
// the expected type rejects nil.
func CallArgumentMayBeNilMismatch(got, want typ.Type) bool {
	return TypeMayBeNilMismatch(got, want)
}

// CallArgumentExpectedTypeHasObjectEntries reports whether an expected type can
// use object-literal member diagnostics instead of reporting only the whole
// argument. Interface-only obligations intentionally stay whole-object so their
// method evidence remains visible.
func CallArgumentExpectedTypeHasObjectEntries(t typ.Type) bool {
	switch tt := unwrap.Optional(t).(type) {
	case *typ.Record, *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Tuple:
		return true
	case *typ.Union:
		for _, member := range tt.Members {
			if CallArgumentExpectedTypeHasObjectEntries(member) {
				return true
			}
		}
	}
	return false
}

// ObligationTypeReportable reports whether an expected type is precise enough
// to emit as a user-facing obligation. Gradual, unknown, and still-generic
// obligations are internal evidence, not standalone reports.
func ObligationTypeReportable(t typ.Type) bool {
	base := unwrap.Optional(t)
	return t != nil &&
		base != nil &&
		!typ.IsAny(base) &&
		!typ.IsUnknown(base) &&
		!refinement.ContainsFreeTypeParam(t)
}

// ObligationTypeContainsFreeTypeParam reports whether an obligation type still
// depends on an uninstantiated type parameter. It also checks the non-nil
// projection of optional types so nilability wrappers do not hide a free
// parameter from reportability decisions.
func ObligationTypeContainsFreeTypeParam(t typ.Type) bool {
	if refinement.ContainsFreeTypeParam(t) {
		return true
	}
	nonNil := ProjectionWithoutNil(t)
	return nonNil != nil && !typ.TypeEquals(nonNil, t) && refinement.ContainsFreeTypeParam(nonNil)
}

// TypeIncludesNil reports whether a type admits nil according to the canonical
// readmodel type projection rules.
func TypeIncludesNil(t typ.Type) bool {
	return typevalue.TypeIncludesNil(t)
}

// TypeMayBeNilMismatch reports whether got admits nil while want rejects nil.
// Obligation check planners use this single law when classifying assignment
// and call-boundary nilability proof failures.
func TypeMayBeNilMismatch(got, want typ.Type) bool {
	return got != nil && want != nil && typevalue.TypeIncludesNil(got) && !typevalue.TypeIncludesNil(want)
}

// CallArgumentProofPlan carries the already-solved proof inputs needed to
// combine value-domain proof and contextual function-type proof for one
// argument. Internal readmodels compute raw proof facts; public readmodel owns
// how those facts become report-facing admissibility and mismatch verdicts.
type CallArgumentProofPlan struct {
	Argument                    CallArgument
	ValueAdmissible             bool
	ValueProvenMismatch         bool
	FunctionTypeAdmissible      bool
	TrustedActualProvenMismatch bool
	FunctionTypeProvenMismatch  bool
}

// CallArgumentProofAdmissible reports whether an argument is proven admissible
// against Expected. A contextual function argument may be admissible even when
// the raw value proof cannot see through the function literal boundary.
func CallArgumentProofAdmissible(plan CallArgumentProofPlan) bool {
	if plan.ValueAdmissible {
		return true
	}
	if plan.Argument.UntrustedTopOrigin && !plan.Argument.RuntimeValidated {
		return false
	}
	return plan.FunctionTypeAdmissible
}

// CallArgumentWitnessProvenMismatch reports whether an argument has a concrete
// contradiction against Expected. Gradual expected types do not produce proven
// mismatches, but a contextual function argument can prove a mismatch when its
// function type rejects the expected type.
func CallArgumentWitnessProvenMismatch(plan CallArgumentProofPlan) bool {
	if plan.ValueProvenMismatch {
		return true
	}
	if plan.TrustedActualProvenMismatch {
		return true
	}
	return plan.FunctionTypeProvenMismatch
}

// CallArgumentCheck is the solved proof result for one argument against one
// expected type. It keeps refinement, admissibility, and mismatch verdict input
// together so producers do not reassemble the proof protocol from fragments.
type CallArgumentCheck struct {
	Argument       CallArgument
	Expected       typ.Type
	ExpectedLabel  string
	ExpectedSpan   SourceSpan
	ExpectedOrigin CallArgumentObligationOrigin
	Admissible     bool
	ProvenMismatch bool
}

// ActualTypeKnown reports whether the solved call argument carried a concrete
// type witness at the call boundary.
func (check CallArgumentCheck) ActualTypeKnown() bool {
	return check.Argument.TypeWithPresence != nil
}

// EffectiveActualType returns the type the obligation layer should attach to
// the call-argument judgment. It mirrors assignment/return proof policy:
// untrusted top-origin values remain visibly any when they have no concrete
// structural candidate.
func (check CallArgumentCheck) EffectiveActualType() typ.Type {
	return obligationActualPolicy{
		TypeWithPresence:    check.Argument.TypeWithPresence,
		Expected:            check.Expected,
		UntrustedTopOrigin:  check.Argument.UntrustedTopOrigin,
		ProvenMismatch:      check.ProvenMismatch,
		ShowUntrustedActual: true,
	}.EffectiveActualType()
}

// MissingProofRefuted reports whether the failed call-argument obligation is a
// proven type contradiction rather than an unknown proof gap.
func (check CallArgumentCheck) MissingProofRefuted() bool {
	return check.ProvenMismatch
}

// CallArgumentCheckPlan carries the solved facts needed to assemble the
// report-facing proof result for one argument. Internal readmodels supply facts;
// public readmodel owns nested-subject selection, nil mismatch classification,
// expected-label adjustment, and final proof verdict assembly.
type CallArgumentCheckPlan struct {
	Argument                    CallArgument
	Expected                    typ.Type
	ExpectedLabel               string
	ExpectedSpan                SourceSpan
	ExpectedOrigin              CallArgumentObligationOrigin
	ValueAdmissible             bool
	ValueProvenMismatch         bool
	FunctionTypeAdmissible      bool
	TrustedActualProvenMismatch bool
	FunctionTypeProvenMismatch  bool
	SubjectPlan                 *CallArgumentMismatchSubjectPlan
}

// PlanCallArgumentCheck returns the complete solved proof result for one
// argument against one expected type.
func PlanCallArgumentCheck(plan CallArgumentCheckPlan) CallArgumentCheck {
	arg := plan.Argument
	want := plan.Expected
	labelSuffix := ""
	if plan.SubjectPlan != nil {
		if subject, ok := PlanCallArgumentMismatchSubject(*plan.SubjectPlan); ok {
			arg = subject.Argument
			want = subject.Expected
			labelSuffix = subject.LabelSuffix
		}
	}
	if arg.Mismatch.Kind == CallArgumentMismatchNone && CallArgumentMayBeNilMismatch(arg.TypeWithPresence, want) {
		arg.Mismatch = CallArgumentMismatch{Kind: CallArgumentMismatchMayBeNil}
	}
	return CallArgumentCheck{
		Argument:       arg,
		Expected:       want,
		ExpectedLabel:  ExpectedLabelWithSuffix(plan.ExpectedLabel, labelSuffix),
		ExpectedSpan:   plan.ExpectedSpan,
		ExpectedOrigin: plan.ExpectedOrigin,
		Admissible: CallArgumentProofAdmissible(CallArgumentProofPlan{
			Argument:               arg,
			ValueAdmissible:        plan.ValueAdmissible,
			FunctionTypeAdmissible: plan.FunctionTypeAdmissible,
		}),
		ProvenMismatch: CallArgumentWitnessProvenMismatch(CallArgumentProofPlan{
			Argument:                    arg,
			ValueProvenMismatch:         plan.ValueProvenMismatch,
			TrustedActualProvenMismatch: plan.TrustedActualProvenMismatch,
			FunctionTypeProvenMismatch:  plan.FunctionTypeProvenMismatch,
		}),
	}
}

// CallArgumentReportKind classifies the rendering path for one planned
// direct-call argument report.
type CallArgumentReportKind uint8

const (
	CallArgumentReportObligation CallArgumentReportKind = iota
	CallArgumentReportGenericConflict
)

// CallArgumentReport is one ordered report candidate for a call argument. The
// read model owns report ordering and index reservation so producers do not
// reimplement direct-call precedence.
type CallArgumentReport struct {
	Kind       CallArgumentReportKind
	Argument   CallArgument
	Obligation CallArgumentObligation
	Check      CallArgumentCheck
	Conflict   CallGenericInferenceConflict
}

// IndexedCallArgumentObligation pairs a call argument index with the expected
// type and report metadata for that slot.
type IndexedCallArgumentObligation struct {
	Index      int
	Obligation CallArgumentObligation
}

// CallArgumentReportPlan carries the already-solved inputs needed to order
// direct-call argument reports. It is deliberately syntax-free: internal
// readmodels assemble values and contracts, while this planner owns precedence
// and index reservation.
type CallArgumentReportPlan struct {
	Args               []CallArgument
	GenericConflicts   []CallGenericInferenceConflict
	GenericConstraints []IndexedCallArgumentObligation
	ExplicitParams     []IndexedCallArgumentObligation
	OutcomeParams      []IndexedCallArgumentObligation
	Check              func(CallArgument, CallArgumentObligation) CallArgumentCheck
}

// PlanCallArgumentReports returns ordered direct-call argument report
// candidates. Precedence is generic inference conflict, generic constraint,
// explicit callable parameter, then solved call-outcome obligation. Once an
// argument index has a report or a proven-admissible check, later sources for
// that index are reserved and suppressed.
func PlanCallArgumentReports(plan CallArgumentReportPlan) []CallArgumentReport {
	var out []CallArgumentReport
	argsByIndex := callArgumentsByIndex(plan.Args)
	reported := make(map[int]struct{})

	for _, conflict := range plan.GenericConflicts {
		if len(conflict.Contributions) < 2 {
			continue
		}
		arg, ok := argsByIndex[conflict.Index]
		if !ok {
			arg = CallArgument{Index: conflict.Index, Span: conflict.Span}
		}
		out = append(out, CallArgumentReport{
			Kind:     CallArgumentReportGenericConflict,
			Argument: arg,
			Conflict: conflict,
		})
		reported[conflict.Index] = struct{}{}
	}

	for _, indexed := range plan.GenericConstraints {
		if _, seen := reported[indexed.Index]; seen || !ObligationTypeReportable(indexed.Obligation.Type) {
			continue
		}
		arg, ok := argsByIndex[indexed.Index]
		if !ok {
			continue
		}
		check := plan.check(arg, indexed.Obligation)
		if check.Admissible {
			reported[indexed.Index] = struct{}{}
			continue
		}
		out = append(out, CallArgumentReport{
			Kind:       CallArgumentReportObligation,
			Argument:   arg,
			Obligation: indexed.Obligation,
			Check:      check,
		})
		reported[indexed.Index] = struct{}{}
	}

	admittedExplicit := make(map[int]struct{})
	out = plan.appendObligations(out, reported, argsByIndex, plan.ExplicitParams, false, admittedExplicit, nil)
	out = plan.appendObligations(out, reported, argsByIndex, plan.OutcomeParams, true, nil, admittedExplicit)
	return out
}

func (plan CallArgumentReportPlan) appendObligations(
	out []CallArgumentReport,
	reported map[int]struct{},
	argsByIndex map[int]CallArgument,
	obligations []IndexedCallArgumentObligation,
	reserveAdmissible bool,
	markAdmissible map[int]struct{},
	skipSignatureAdmitted map[int]struct{},
) []CallArgumentReport {
	for _, indexed := range obligations {
		if _, seen := reported[indexed.Index]; seen || !ObligationTypeReportable(indexed.Obligation.Type) {
			continue
		}
		if indexed.Obligation.SignatureSurface && skipSignatureAdmitted != nil {
			if _, admitted := skipSignatureAdmitted[indexed.Index]; admitted {
				continue
			}
		}
		arg, ok := argsByIndex[indexed.Index]
		if !ok {
			continue
		}
		check := plan.check(arg, indexed.Obligation)
		if check.Admissible {
			if markAdmissible != nil {
				markAdmissible[indexed.Index] = struct{}{}
			}
			if reserveAdmissible {
				reported[indexed.Index] = struct{}{}
			}
			continue
		}
		out = append(out, CallArgumentReport{
			Kind:       CallArgumentReportObligation,
			Argument:   arg,
			Obligation: indexed.Obligation,
			Check:      check,
		})
		reported[indexed.Index] = struct{}{}
	}
	return out
}

func (plan CallArgumentReportPlan) check(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
	if plan.Check == nil {
		return CallArgumentCheck{
			Argument: arg,
			Expected: obligation.Type,
		}
	}
	return plan.Check(arg, obligation)
}

func callArgumentsByIndex(args []CallArgument) map[int]CallArgument {
	out := make(map[int]CallArgument, len(args))
	for _, arg := range args {
		out[arg.Index] = arg
	}
	return out
}

// CallContractSourceKind classifies where a callable contract came from. The
// read model owns this report-facing source vocabulary so obligation producers
// receive stable labels and spans without re-deriving call provenance.
type CallContractSourceKind uint8

const (
	CallContractSourceLocalFunction CallContractSourceKind = iota + 1
	CallContractSourceImportedSignature
	CallContractSourceFunctionValue
	CallContractSourceMemberFunction
)

// CallContractSource identifies the source of a callable contract for
// report-facing parameter labels and declaration spans.
type CallContractSource struct {
	Kind           CallContractSourceKind
	Name           string
	ParameterSpans []SourceSpan
	ResultSpans    []SourceSpan
}

// ParameterLabel returns the stable display label for parameter index.
func (s CallContractSource) ParameterLabel(index int) string {
	param := fmt.Sprintf("parameter %d", index+1)
	if s.Name == "" {
		return param
	}
	switch s.Kind {
	case CallContractSourceImportedSignature:
		return fmt.Sprintf("%s parameter %d", s.Name, index+1)
	case CallContractSourceLocalFunction, CallContractSourceFunctionValue, CallContractSourceMemberFunction:
		return fmt.Sprintf("%s parameter %d", s.Name, index+1)
	default:
		return param
	}
}

// ParameterSpan returns the declaration span for parameter index, when known.
func (s CallContractSource) ParameterSpan(index int) SourceSpan {
	if index < 0 || index >= len(s.ParameterSpans) {
		return SourceSpan{}
	}
	return s.ParameterSpans[index]
}

// ResultSpan returns the declaration span for return slot index, when known.
func (s CallContractSource) ResultSpan(index int) SourceSpan {
	if index < 0 || index >= len(s.ResultSpans) {
		return SourceSpan{}
	}
	return s.ResultSpans[index]
}

// CallArgumentObligation is one expected type for one call argument in an
// already-planned report.
type CallArgumentObligation struct {
	Type             typ.Type
	ExpectedLabel    string
	ExpectedSpan     SourceSpan
	Origin           CallArgumentObligationOrigin
	SignatureSurface bool
}

// CallArgumentObligationOrigin records why a projected call-site obligation
// exists. Direct signature checks leave HasOrigin false; summary-projected
// obligations use this to render the callee-use chain. Member-call obligations
// additionally include the provider and member parameter that required the type.
type CallArgumentObligationOrigin struct {
	HasOrigin         bool
	FunctionName      string
	SubjectLabel      string
	ProviderLabel     string
	MemberParamNumber int
}

// CallGenericInferenceConflict records an argument whose nested uses of one
// generic type parameter imply incompatible concrete types.
type CallGenericInferenceConflict struct {
	Index         int
	FunctionName  string
	ParamName     string
	Span          SourceSpan
	Contributions []CallGenericInferenceContribution
}

// CallGenericInferenceContribution records one nested argument position that
// contributed a concrete type to generic inference.
type CallGenericInferenceContribution struct {
	Type  typ.Type
	Span  SourceSpan
	Label string
}

// GenericInferenceContributionSpanCandidate is one possible source span for a
// generic inference contribution.
type GenericInferenceContributionSpanCandidate struct {
	Span         SourceSpan
	SegmentDepth int
	Matches      bool
}

// GenericInferenceContributionSpanPlan carries already-projected span
// candidates for one generic inference contribution. Internal readmodels own
// producing candidate matches; public readmodel owns choosing the anchor.
type GenericInferenceContributionSpanPlan struct {
	Fallback   SourceSpan
	Candidates []GenericInferenceContributionSpanCandidate
}

// PlanGenericInferenceContributionSpan chooses the most specific matching span
// for a generic inference contribution, falling back to the whole argument.
func PlanGenericInferenceContributionSpan(plan GenericInferenceContributionSpanPlan) SourceSpan {
	best := plan.Fallback
	bestDepth := -1
	for _, candidate := range plan.Candidates {
		if !candidate.Matches || candidate.SegmentDepth <= bestDepth {
			continue
		}
		best = candidate.Span
		bestDepth = candidate.SegmentDepth
	}
	return best
}

// CallArgumentMemberLabel returns the stable label for a nested argument member
// that became the report subject.
func CallArgumentMemberLabel(index int, segs []segment.Segment, valueLabel string) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("argument ")
	b.WriteString(strconv.Itoa(index + 1))
	for _, seg := range segs {
		if !appendSegmentLabel(&b, seg) {
			return ""
		}
	}
	if valueLabel != "" {
		b.WriteString(" (")
		b.WriteString(valueLabel)
		b.WriteByte(')')
	}
	return b.String()
}

// ExpectedLabelWithSuffix appends a nested member suffix to an existing
// expected-label owner. Empty inputs keep the original label unchanged.
func ExpectedLabelWithSuffix(label, suffix string) string {
	if label == "" || suffix == "" {
		return label
	}
	return label + suffix
}

// CallArgumentExpectedLabelSuffix returns the stable expected-label suffix for
// a nested argument member.
func CallArgumentExpectedLabelSuffix(segs []segment.Segment) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range segs {
		if !appendSegmentLabel(&b, seg) {
			return ""
		}
	}
	return b.String()
}

func appendSegmentLabel(b *strings.Builder, seg segment.Segment) bool {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if seg.Name == "" {
			return false
		}
		b.WriteByte('.')
		b.WriteString(seg.Name)
	case segment.SegmentIndexInt:
		b.WriteByte('[')
		b.WriteString(strconv.FormatInt(int64(seg.Index), 10))
		b.WriteByte(']')
	default:
		return false
	}
	return true
}
