package certificate

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	checkauthority "github.com/wippyai/go-lua/analysis/relation/check/authority"
	checkrecurrence "github.com/wippyai/go-lua/analysis/relation/check/recurrence"
	checkregistry "github.com/wippyai/go-lua/analysis/relation/check/registry"
	checktyping "github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Pass identifies the independent proof pass that contributed an issue to a
// refusal.  Structural findings are emitted by the shared registry; the
// remaining passes only contribute their own semantic findings.
type Pass uint8

const (
	PassStructural Pass = iota + 1
	PassTyping
	PassAuthority
	PassRecurrence
)

// Short aliases keep the pass vocabulary pleasant at call sites while the
// prefixed names remain unambiguous in documentation and diagnostics.
const (
	Structural = PassStructural
	Typing     = PassTyping
	Authority  = PassAuthority
	Recurrence = PassRecurrence
)

func (pass Pass) String() string {
	switch pass {
	case PassStructural:
		return "structural"
	case PassTyping:
		return "typing"
	case PassAuthority:
		return "authority"
	case PassRecurrence:
		return "recurrence"
	default:
		return "unknown"
	}
}

// Issue is one deterministic proof finding. Code is the numeric value of the
// typed code owned by the pass named by Pass; conversion is direct and never
// depends on reflection or diagnostic-string parsing.
type Issue struct {
	Pass   Pass
	Code   uint16
	Path   string
	Detail string
}

// String returns a compact deterministic representation suitable for an
// aggregate refusal. It is intentionally based on typed numeric codes and
// stable logical paths, never declaration order or physical state.
func (issue Issue) String() string {
	result := fmt.Sprintf("%s[%d]", issue.Pass, issue.Code)
	if issue.Path != "" {
		result += " " + issue.Path
	}
	if issue.Detail != "" {
		result += ": " + issue.Detail
	}
	return result
}

// Refusal is the complete deterministic result of a failed certificate
// check. Its issue slice is private so callers cannot mutate the refusal's
// canonical ordering.
type Refusal struct {
	issues []Issue
}

// Issues returns a defensive copy in pass/path/code/detail order.
func (refusal *Refusal) Issues() []Issue {
	if refusal == nil {
		return nil
	}
	return append([]Issue(nil), refusal.issues...)
}

// Valid reports whether this refusal is empty. A nil refusal is the success
// result returned by Check.
func (refusal *Refusal) Valid() bool { return refusal == nil || len(refusal.issues) == 0 }

// Error implements error for admission gates that only need the aggregate
// diagnostic. The typed Issues remain available to callers that need exact
// pass/code classification.
func (refusal *Refusal) Error() string {
	if refusal == nil || len(refusal.issues) == 0 {
		return ""
	}
	parts := make([]string, len(refusal.issues))
	for index, issue := range refusal.issues {
		parts[index] = issue.String()
	}
	return "relation/check/certificate: " + joinIssues(parts)
}

// Certificate is the checked logical capability accepted by mount. The
// registry is the sole declaration source; the certificate deliberately does
// not copy an ExecutionSchema or retain any physical/mount data.
type Certificate struct {
	registry            *checkregistry.View
	mergeRequirements   []checktyping.MergeRequirement
	algebraRequirements []model.TypeID
	recurrenceProof     checkrecurrence.Proof
	digest              identity.ContentID
}

// Check builds the shared declaration registry once, then runs all three
// independent proof passes against that same immutable view. Structural
// findings are collected once from the registry. When the registry is
// malformed, semantic pass results are withheld so one malformed declaration
// does not become several indistinguishable structural issues.
//
// A non-nil Refusal always accompanies the zero Certificate. A nil Refusal
// means the returned certificate is available.
func Check(schema plan.ExecutionSchema) (Certificate, *Refusal) {
	indexed := checkregistry.Build(schema)
	issues := make([]Issue, 0)
	structural := indexed.Issues()
	for _, issue := range structural {
		issues = append(issues, structuralIssue(issue))
	}
	if len(issues) != 0 {
		return Certificate{}, newRefusal(issues)
	}

	// The registry is the admission boundary for all proof passes. Once it
	// has established a structurally valid view, every pass sees that same
	// immutable index and contributes only its own typed findings.
	typingReport := checktyping.CheckView(indexed)
	authorityReport := checkauthority.CheckView(indexed)
	recurrenceProof, recurrenceErr := checkrecurrence.CheckView(indexed)
	for _, issue := range typingReport.Issues() {
		issues = append(issues, typingIssue(issue))
	}
	for _, issue := range authorityReport.Issues() {
		issues = append(issues, authorityIssue(issue))
	}
	if recurrenceErr != nil {
		issues = append(issues, recurrenceIssue(recurrenceErr))
	}
	if len(issues) != 0 {
		return Certificate{}, newRefusal(issues)
	}

	requirements := typingReport.MergeRequirements()
	canonicalizeRequirements(requirements)
	algebraRequirements := typingReport.AlgebraRequirements()
	digest, ok := certificateDigest(schema.Digest())
	if !ok {
		// An unavailable schema is normally already represented by the shared
		// registry. Keep this fallback typed and deterministic if a future
		// schema implementation violates that invariant.
		return Certificate{}, newRefusal([]Issue{{
			Pass:   PassStructural,
			Code:   uint16(checkregistry.CodeSchemaUnavailable),
			Path:   "schema",
			Detail: "execution schema digest is unavailable",
		}})
	}
	return Certificate{
		registry:            indexed,
		mergeRequirements:   requirements,
		algebraRequirements: algebraRequirements,
		recurrenceProof:     recurrenceProof,
		digest:              digest,
	}, nil
}

const certificateDigestDomain = "analysis/relation/check/certificate/v1"

func certificateDigest(schemaDigest identity.ContentID) (identity.ContentID, bool) {
	return identity.DeriveContentID(certificateDigestDomain, schemaDigest[:])
}

// Available reports whether this value is a complete checked certificate.
func (certificate Certificate) Available() bool {
	return certificate.registry != nil && certificate.digest.Available() && certificate.recurrenceProof.Available()
}

// SchemaID returns the owner-issued identity of the checked logical schema.
func (certificate Certificate) SchemaID() model.SchemaID {
	if certificate.registry == nil {
		return model.SchemaID{}
	}
	return certificate.registry.Schema().SchemaID()
}

// Digest returns the versioned certificate identity. It is derived solely
// from the checked ExecutionSchema digest.
func (certificate Certificate) Digest() identity.ContentID { return certificate.digest }

// Relations returns defensive copies of the canonical relation declarations.
func (certificate Certificate) Relations() []model.RelationSchema {
	if certificate.registry == nil {
		return nil
	}
	return certificate.registry.Relations()
}

// Columns returns defensive copies of the canonical column declarations.
func (certificate Certificate) Columns() []model.ColumnSchema {
	if certificate.registry == nil {
		return nil
	}
	return certificate.registry.Columns()
}

// Keys returns defensive copies of the canonical key declarations.
func (certificate Certificate) Keys() []model.KeySchema {
	if certificate.registry == nil {
		return nil
	}
	return certificate.registry.Keys()
}

// Scopes returns defensive copies of the canonical scope declarations.
func (certificate Certificate) Scopes() []model.ScopeSchema {
	if certificate.registry == nil {
		return nil
	}
	return certificate.registry.Scopes()
}

// Expressions returns defensive copies of the canonical expression entries.
func (certificate Certificate) Expressions() []plan.ExpressionRef {
	if certificate.registry == nil {
		return nil
	}
	return certificate.registry.Expressions()
}

// Dependencies returns defensive copies of the canonical dependency
// declarations.
func (certificate Certificate) Dependencies() []plan.Dependency {
	if certificate.registry == nil {
		return nil
	}
	return certificate.registry.Dependencies()
}

// Signatures returns defensive copies of the canonical semantic signatures.
func (certificate Certificate) Signatures() []signature.Signature {
	if certificate.registry == nil {
		return nil
	}
	return certificate.registry.Signatures()
}

// SCCs returns defensive copies of the canonical recurrence declarations.
func (certificate Certificate) SCCs() []plan.SCC {
	if certificate.registry == nil {
		return nil
	}
	return certificate.registry.SCCs()
}

// MergeRequirements returns the canonical typed obligations emitted by the
// typing proof.
func (certificate Certificate) MergeRequirements() []checktyping.MergeRequirement {
	return append([]checktyping.MergeRequirement(nil), certificate.mergeRequirements...)
}

// AlgebraRequirements returns the canonical semantic TypeIDs needed by mount
// for committed relation values and validated operation frames/outputs.
func (certificate Certificate) AlgebraRequirements() []model.TypeID {
	return append([]model.TypeID(nil), certificate.algebraRequirements...)
}

// RecurrenceProof returns the immutable recurrence proof. Its own accessors
// return defensive copies of nested projections.
func (certificate Certificate) RecurrenceProof() checkrecurrence.Proof {
	return certificate.recurrenceProof
}

// WideningHeads returns the validated recurrence-head projection. Mount uses
// this proof output directly and does not inspect or correlate raw SCCs.
func (certificate Certificate) WideningHeads() []plan.WideningHead {
	return certificate.recurrenceProof.WideningHeads()
}

func structuralIssue(issue checkregistry.Issue) Issue {
	return Issue{
		Pass:   PassStructural,
		Code:   uint16(issue.Code),
		Path:   issue.Path,
		Detail: issue.Detail,
	}
}

func typingIssue(issue checktyping.Issue) Issue {
	return Issue{
		Pass:   PassTyping,
		Code:   uint16(issue.Code),
		Path:   issue.Path,
		Detail: issue.Detail,
	}
}

func authorityIssue(issue checkauthority.Issue) Issue {
	return Issue{
		Pass: PassAuthority,
		Code: uint16(issue.Code),
		Path: issue.Path,
	}
}

func recurrenceIssue(value error) Issue {
	issue := Issue{Pass: PassRecurrence, Detail: "recurrence checker refused"}
	if typed, ok := value.(*checkrecurrence.Error); ok && typed != nil {
		issue.Code = uint16(typed.Code)
		issue.Path = recurrenceErrorPath(typed)
		issue.Detail = typed.Detail
	}
	return issue
}

func recurrenceErrorPath(value *checkrecurrence.Error) string {
	if value == nil {
		return ""
	}
	if value.Dependency.Available() {
		return "dependency/" + value.Dependency.Owner().Content().String() + "/" + value.Dependency.Content().String()
	}
	if value.Relation.Available() {
		return "relation/" + value.Relation.Owner().Content().String() + "/" + value.Relation.Content().String()
	}
	return ""
}

func canonicalizeRequirements(values []checktyping.MergeRequirement) {
	sort.SliceStable(values, func(left, right int) bool {
		if values[left].Path != values[right].Path {
			return values[left].Path < values[right].Path
		}
		if key := compareColumn(values[left].Column, values[right].Column); key != 0 {
			return key < 0
		}
		return typeKey(values[left].Type) < typeKey(values[right].Type)
	})
}

func compareColumn(left, right model.ColumnID) int {
	leftOwner, rightOwner := left.Owner().Content().String(), right.Owner().Content().String()
	if leftOwner != rightOwner {
		if leftOwner < rightOwner {
			return -1
		}
		return 1
	}
	leftContent, rightContent := left.Content().String(), right.Content().String()
	if leftContent < rightContent {
		return -1
	}
	if leftContent > rightContent {
		return 1
	}
	return 0
}

func typeKey(value model.TypeID) string {
	return value.Owner().Content().String() + "/" + value.Content().String()
}

func newRefusal(issues []Issue) *Refusal {
	sort.SliceStable(issues, func(left, right int) bool {
		if issues[left].Pass != issues[right].Pass {
			return issues[left].Pass < issues[right].Pass
		}
		if issues[left].Path != issues[right].Path {
			return issues[left].Path < issues[right].Path
		}
		if issues[left].Code != issues[right].Code {
			return issues[left].Code < issues[right].Code
		}
		return issues[left].Detail < issues[right].Detail
	})
	return &Refusal{issues: append([]Issue(nil), issues...)}
}

func joinIssues(values []string) string {
	result := ""
	for index, value := range values {
		if index != 0 {
			result += "; "
		}
		result += value
	}
	return result
}
