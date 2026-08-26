package typing

import (
	"fmt"

	checkregistry "github.com/wippyai/go-lua/analysis/relation/check/registry"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Code identifies one deterministic logical typing failure.  Codes are kept
// closed so callers can distinguish a malformed declaration from a bad
// expression without parsing text.
type Code uint16

const (
	CodeInvalid Code = iota
	CodeSchemaIdentity
	CodeUnavailable
	CodeDuplicateIdentity
	CodeForeignReference
	CodeMissingReference
	CodeMembership
	CodeDuplicateMember
	CodeTypeMismatch
	CodeShapeMismatch
	CodeExpressionDigest
	CodeExpressionCycle
	CodeOperatorContract
	CodeSignatureMismatch
	CodeDeliveryMismatch
	CodeDenominatorMismatch
	CodeScopeMismatch
	CodeKeyMismatch
	CodeDependencyMismatch
	CodeTypeCapabilityMismatch
	CodeCorrelationMismatch
)

func (code Code) String() string {
	names := [...]string{
		"Invalid", "SchemaIdentity", "Unavailable", "DuplicateIdentity",
		"ForeignReference", "MissingReference", "Membership", "DuplicateMember",
		"TypeMismatch", "ShapeMismatch", "ExpressionDigest", "ExpressionCycle",
		"OperatorContract", "SignatureMismatch", "DeliveryMismatch",
		"DenominatorMismatch", "ScopeMismatch", "KeyMismatch", "DependencyMismatch",
		"TypeCapabilityMismatch", "CorrelationMismatch",
	}
	if int(code) < 0 || int(code) >= len(names) {
		return "Unknown"
	}
	return names[code]
}

// Issue is one independent checker finding. Path is a stable declaration
// path, never a physical ordinal or a compiler diagnostic location.
type Issue struct {
	Code   Code
	Path   string
	Detail string
}

func (issue Issue) Error() string {
	if issue.Path == "" {
		return fmt.Sprintf("typing[%s]: %s", issue.Code, issue.Detail)
	}
	return fmt.Sprintf("typing[%s] %s: %s", issue.Code, issue.Path, issue.Detail)
}

// MergeRequirement is the explicit semantic obligation emitted for one
// Merge output column. The checker does not invent an algebra or a second
// reducer registry: the TypeID is the column's sole semantic authority and a
// later binding must prove Join/Widen/LessOrEqual for that type.
type MergeRequirement struct {
	Path   string
	Column model.ColumnID
	Type   model.TypeID
}

// EqualityRequirement is the explicit semantic obligation for a key
// operator. It is independent of MergeRequirement: a key may use the
// Equatable capability without having any lattice Join/Widen authority.
type EqualityRequirement struct {
	Path   string
	Column model.ColumnID
	Type   model.TypeID
}

// PresentRequirement is emitted only at a Publish boundary. A signature may
// describe a Present output that is never committed by any expression; that
// declaration alone is not a lattice obligation.
type PresentRequirement struct {
	Path   string
	Column model.ColumnID
	Type   model.TypeID
}

// Report is the complete, deterministic result of checking one unchecked
// ExecutionSchema. It is intentionally not an execution capability; the
// certificate package owns the opaque capability accepted by mount.
type Report struct {
	issues               []Issue
	requirements         []MergeRequirement
	equalityRequirements []EqualityRequirement
	presentRequirements  []PresentRequirement
	algebraRequirements  []model.TypeID
}

// Valid reports whether no logical typing obligation failed.
func (report Report) Valid() bool { return len(report.issues) == 0 }

// Issues returns a defensive copy in deterministic path/code order.
func (report Report) Issues() []Issue { return append([]Issue(nil), report.issues...) }

// MergeRequirements returns the TypeID obligations discovered while checking
// Merge nodes. A report can expose obligations even when other declarations
// are malformed, which makes the nearest failing rule visible to W1 tooling.
func (report Report) MergeRequirements() []MergeRequirement {
	return append([]MergeRequirement(nil), report.requirements...)
}

// EqualityRequirements returns the key-equality obligations discovered by
// Project, Join, Expand, Group, Merge, and Publish. It is a diagnostic
// projection, not an equality implementation or a lattice registry.
func (report Report) EqualityRequirements() []EqualityRequirement {
	return append([]EqualityRequirement(nil), report.equalityRequirements...)
}

// AlgebraRequirements returns the distinct TypeIDs required at an actual
// checked Merge or committed Present output. The projection retains malformed
// obligations too; the checker refuses a missing/DecodeOnly/Equatable policy
// rather than filtering the obligation into a false success.
func (report Report) AlgebraRequirements() []model.TypeID {
	return append([]model.TypeID(nil), report.algebraRequirements...)
}

// Error returns nil for a valid report and a compact aggregate error for an
// invalid report. The full stable findings remain available through Issues.
func (report Report) Error() error {
	if report.Valid() {
		return nil
	}
	return reportError(report.issues)
}

func (report *Report) addRegistryIssue(issue checkregistry.Issue) {
	code := CodeUnavailable
	switch issue.Code {
	case checkregistry.CodeSchemaIdentityUnavailable:
		code = CodeSchemaIdentity
	case checkregistry.CodeRelationDuplicate, checkregistry.CodeColumnDuplicate,
		checkregistry.CodeKeyDuplicate, checkregistry.CodeScopeDuplicate,
		checkregistry.CodeExpressionDuplicate, checkregistry.CodeDependencyDuplicate,
		checkregistry.CodeSignatureDuplicate:
		code = CodeDuplicateIdentity
	case checkregistry.CodeExpressionDigest:
		code = CodeExpressionDigest
	}
	report.add(code, issue.Path, issue.Detail)
}

type reportError []Issue

func (errors reportError) Error() string {
	if len(errors) == 0 {
		return ""
	}
	message := errors[0].Error()
	if len(errors) == 1 {
		return message
	}
	return fmt.Sprintf("%s (and %d more typing issue(s))", message, len(errors)-1)
}
