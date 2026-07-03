// Package judgment defines the post-solve obligation records that diagnostics
// render. It intentionally carries semantic identities and references, not
// user-facing messages or severity.
package judgment

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Code is the stable semantic judgment code. Rendering layers may map it to a
// diagnostic code, message, and default severity.
type Code string

const (
	CodeCallArgType      Code = "call.argument.type"
	CodeCallArity        Code = "call.arity"
	CodeCallCallee       Code = "call.callee"
	CodeAssignment       Code = "assignment.type"
	CodeAssignmentTarget Code = "assignment.optional_target"
)

// Verdict classifies whether the solved state proves or refutes an obligation.
// Policy decides how each verdict maps to severity.
type Verdict uint8

const (
	VerdictUnknown Verdict = iota
	VerdictProven
	VerdictRefuted
)

// SubjectKind identifies the stable subject namespace used for deduplication,
// precedence, and shadow-diff matching.
type SubjectKind uint8

const (
	SubjectUnknown SubjectKind = iota
	SubjectExpression
	SubjectPath
	SubjectCallExpression
	SubjectCallArgument
	SubjectReturnValue
)

// SubjectRef is a renderer-independent identity for the code location or value
// being judged. Key is canonical within Kind and Point.
type SubjectRef struct {
	FunctionKey string
	Kind        SubjectKind
	Key         string
	Label       string
}

// NewSubjectRef builds a stable subject identity. FunctionKey should identify
// the analyzed body/content, not a rendered function name.
func NewSubjectRef(functionKey string, kind SubjectKind, key string) SubjectRef {
	return SubjectRef{FunctionKey: functionKey, Kind: kind, Key: key}
}

// WithLabel returns s with a renderer-facing subject label. The label is not
// part of StableKey; it preserves user-facing source identity without letting
// renderers inspect syntax.
func (s SubjectRef) WithLabel(label string) SubjectRef {
	s.Label = label
	return s
}

// StableKey returns a deterministic identity for dedup, precedence, and
// shadow-diff matching.
func (s SubjectRef) StableKey() string {
	var b strings.Builder
	b.WriteString(s.FunctionKey)
	b.WriteByte('|')
	b.WriteString(subjectKindString(s.Kind))
	b.WriteByte('|')
	b.WriteString(s.Key)
	return b.String()
}

// TypeRef names a resolved type. The concrete type value remains owned by the
// read model or contract provider; judgments keep a stable reference for
// matching and rendering.
type TypeRef struct {
	Key   string
	Type  typ.Type
	Label string
}

// ValueRef names a solved abstract value at a read boundary.
type ValueRef struct {
	Key           string
	ProjectedType typ.Type
	Label         string
}

// OriginRef points at the solved origin of evidence without exposing syntax
// nodes to judgment consumers.
type OriginRef struct {
	Point cfg.Point
	Key   string
}

// EvidenceKind classifies a structured proof step.
type EvidenceKind uint8

const (
	EvidenceUnknown EvidenceKind = iota
	EvidenceAbstractFact
	EvidenceUserAssertion
	EvidenceMissingProof
	EvidencePrecisionBoundary
)

// EvidenceTrust classifies how strongly an evidence node supports the
// judgment.
type EvidenceTrust uint8

const (
	EvidenceTrustUnknown EvidenceTrust = iota
	EvidenceTrustProven
	EvidenceTrustClaimed
	EvidenceTrustRefuted
)

// EvidenceDetailKind classifies structured evidence details that renderers may
// phrase for a diagnostic family. It is intentionally semantic data, not text.
type EvidenceDetailKind uint8

const (
	EvidenceDetailNone EvidenceDetailKind = iota
	EvidenceDetailMissingRequiredField
	EvidenceDetailMayBeNil
	EvidenceDetailGenericConflict
	EvidenceDetailArityTooFew
	EvidenceDetailArityTooMany
	EvidenceDetailCalleeNotCallable
	EvidenceDetailCalleeMayBeNil
	EvidenceDetailCallParamObligation
)

// EvidenceDetail carries renderer-independent detail for one evidence node.
type EvidenceDetail struct {
	Kind          EvidenceDetailKind
	Field         string
	FieldType     typ.Type
	Param         string
	Callable      bool
	ExpectedCount int
	ActualCount   int
	FunctionName  string
	SubjectLabel  string
	ProviderLabel string
	MemberParam   int
}

// MissingRequiredFieldEvidenceDetail records that a structural proof failed
// because a required record field is absent.
func MissingRequiredFieldEvidenceDetail(field string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMissingRequiredField, Field: field}
}

// MissingRequiredFieldTypeEvidenceDetail records an absent required record
// field and its expected field type.
func MissingRequiredFieldTypeEvidenceDetail(field string, fieldType typ.Type) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMissingRequiredField, Field: field, FieldType: fieldType}
}

// MayBeNilEvidenceDetail records that the argument value may be nil while the
// parameter contract does not accept nil.
func MayBeNilEvidenceDetail() EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMayBeNil}
}

// GenericConflictEvidenceDetail records that one generic type parameter was
// inferred from incompatible argument evidence.
func GenericConflictEvidenceDetail(param string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailGenericConflict, Param: param}
}

// ArityTooFewEvidenceDetail records a call with fewer arguments than the
// callable contract requires.
func ArityTooFewEvidenceDetail(expected, actual int) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailArityTooFew, ExpectedCount: expected, ActualCount: actual}
}

// ArityTooManyEvidenceDetail records a call with more arguments than the
// callable contract accepts.
func ArityTooManyEvidenceDetail(expected, actual int) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailArityTooMany, ExpectedCount: expected, ActualCount: actual}
}

// CalleeNotCallableEvidenceDetail records a call whose target has a concrete
// non-callable type.
func CalleeNotCallableEvidenceDetail() EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailCalleeNotCallable}
}

// CalleeMayBeNilEvidenceDetail records a call whose target may be nil before
// it is invoked.
func CalleeMayBeNilEvidenceDetail(callable bool) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailCalleeMayBeNil, Callable: callable}
}

// CallParamObligationEvidenceDetail records that a caller argument is being
// checked because the callee body forwarded it into a member parameter.
func CallParamObligationEvidenceDetail(functionName, subjectLabel, providerLabel string, memberParam int) EvidenceDetail {
	return EvidenceDetail{
		Kind:          EvidenceDetailCallParamObligation,
		FunctionName:  functionName,
		SubjectLabel:  subjectLabel,
		ProviderLabel: providerLabel,
		MemberParam:   memberParam,
	}
}

// Evidence is one structured proof or missing-proof step. It carries stable
// origin identity only; renderers own wording.
type Evidence struct {
	Kind   EvidenceKind
	Trust  EvidenceTrust
	Origin OriginRef
	Detail EvidenceDetail
	Span   SpanRef
}

// EvidenceChain is a deterministic, joinable list of evidence nodes.
type EvidenceChain []Evidence

// HasEvidence reports whether the judgment carries at least one evidence node
// of kind.
func (j Judgment) HasEvidence(kind EvidenceKind) bool {
	return j.Evidence.Has(kind)
}

// EvidenceTrustFor returns the trust of the first evidence node of kind.
func (j Judgment) EvidenceTrustFor(kind EvidenceKind) (EvidenceTrust, bool) {
	return j.Evidence.TrustFor(kind)
}

// Has reports whether the chain carries at least one evidence node of kind.
func (c EvidenceChain) Has(kind EvidenceKind) bool {
	_, ok := c.TrustFor(kind)
	return ok
}

// TrustFor returns the trust of the first evidence node of kind.
func (c EvidenceChain) TrustFor(kind EvidenceKind) (EvidenceTrust, bool) {
	for _, item := range c {
		if item.Kind == kind {
			return item.Trust, true
		}
	}
	return EvidenceTrustUnknown, false
}

// JoinEvidenceChains joins branch evidence. Evidence present with the same
// shape and trust on both branches remains as-is. One-sided or conflicting
// evidence remains visible, but is degraded to an unknown precision-boundary
// node so obligation renderers cannot silently treat one branch as proof for
// all paths.
func JoinEvidenceChains(a, b EvidenceChain) EvidenceChain {
	if len(a) == 0 {
		return degradeOneSidedEvidence(b)
	}
	if len(b) == 0 {
		return degradeOneSidedEvidence(a)
	}
	merged := make(map[evidenceIdentity]evidenceJoinState, len(a)+len(b))
	for _, item := range a {
		state := merged[item.identity()]
		state.left = &item
		merged[item.identity()] = state
	}
	for _, item := range b {
		state := merged[item.identity()]
		state.right = &item
		merged[item.identity()] = state
	}
	ids := make([]evidenceIdentity, 0, len(merged))
	for id := range merged {
		ids = append(ids, id)
	}
	sortEvidenceIdentities(ids)

	out := make(EvidenceChain, 0, len(ids))
	for _, id := range ids {
		state := merged[id]
		switch {
		case state.left != nil && state.right != nil:
			if state.left.Trust == state.right.Trust {
				out = append(out, *state.left)
			} else {
				out = append(out, precisionBoundaryEvidence(*state.left))
			}
		case state.left != nil:
			out = append(out, precisionBoundaryEvidence(*state.left))
		case state.right != nil:
			out = append(out, precisionBoundaryEvidence(*state.right))
		}
	}
	return out
}

// SpanRef points at a source range captured during lowering or solved-state
// projection. File is optional when Point/Subject already determines it.
type SpanRef struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Judgment is the semantic obligation record emitted after solve and consumed
// by rendering/dedup/policy layers.
type Judgment struct {
	Code     Code
	Point    cfg.Point
	Subject  SubjectRef
	Expected TypeRef
	Actual   ValueRef
	Verdict  Verdict
	Evidence EvidenceChain
	Spans    []SpanRef
}

type evidenceJoinState struct {
	left  *Evidence
	right *Evidence
}

type evidenceIdentity struct {
	kind   EvidenceKind
	point  cfg.Point
	key    string
	detail EvidenceDetail
}

func (e Evidence) identity() evidenceIdentity {
	return evidenceIdentity{kind: e.Kind, point: e.Origin.Point, key: e.Origin.Key, detail: e.Detail}
}

func degradeOneSidedEvidence(in EvidenceChain) EvidenceChain {
	if len(in) == 0 {
		return nil
	}
	out := make(EvidenceChain, len(in))
	for i, item := range in {
		out[i] = precisionBoundaryEvidence(item)
	}
	return out
}

func precisionBoundaryEvidence(item Evidence) Evidence {
	item.Kind = EvidencePrecisionBoundary
	item.Trust = EvidenceTrustUnknown
	return item
}

func sortEvidenceIdentities(ids []evidenceIdentity) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && evidenceIdentityLess(ids[j], ids[j-1]); j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}

func evidenceIdentityLess(a, b evidenceIdentity) bool {
	if a.point != b.point {
		return a.point < b.point
	}
	if a.key != b.key {
		return a.key < b.key
	}
	if a.detail.Kind != b.detail.Kind {
		return a.detail.Kind < b.detail.Kind
	}
	if a.detail.Field != b.detail.Field {
		return a.detail.Field < b.detail.Field
	}
	if a.detail.Param != b.detail.Param {
		return a.detail.Param < b.detail.Param
	}
	if a.detail.Callable != b.detail.Callable {
		return !a.detail.Callable && b.detail.Callable
	}
	if a.detail.ExpectedCount != b.detail.ExpectedCount {
		return a.detail.ExpectedCount < b.detail.ExpectedCount
	}
	if a.detail.ActualCount != b.detail.ActualCount {
		return a.detail.ActualCount < b.detail.ActualCount
	}
	return a.kind < b.kind
}

func subjectKindString(kind SubjectKind) string {
	switch kind {
	case SubjectExpression:
		return "expr"
	case SubjectPath:
		return "path"
	case SubjectCallExpression:
		return "call"
	case SubjectCallArgument:
		return "call_arg"
	case SubjectReturnValue:
		return "return"
	default:
		return "unknown"
	}
}
