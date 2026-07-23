package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// selectConsumerDiagnostic is a source-facing consequence of closed select
// facts.  It intentionally contains no AST or solver callback: topology comes
// from WIR and arm completeness comes from the evaluated select publication.
type selectConsumerDiagnostic struct {
	Key, Message string
	Span         wir.Span
	Evidence     []DiagnosticEvidence
}

// channelSelectCoverageConsumers proves only a complete authored elseif chain
// against a complete evaluated select catalog.  Missing descriptors, arm
// publications, mixed predicates, aliases that cannot be resolved here, and a
// final else all fail closed.
func channelSelectCoverageConsumers(compilation front.Compilation) []selectConsumerDiagnostic {
	body := compilation.WIR
	if body == nil || !selectCatalogPublished(compilation.Artifact) {
		return nil
	}
	cases := selectCaseDisplays(compilation.Artifact)
	if len(cases) < 2 {
		return nil
	}
	var out []selectConsumerDiagnostic
	body.ForEachIfChainDescriptor(func(chain wir.IfChainDescriptor) bool {
		if chain.HasElse || len(chain.Branches) < 2 {
			return true
		}
		handled := make(map[string]bool, len(chain.Branches))
		result := ""
		for _, branch := range chain.Branches {
			checks := body.BranchChecks(branch.Point)
			if len(checks) != 1 || checks[0].Kind != wir.CheckPathEqual {
				return true
			}
			check := checks[0]
			selected, candidate := "", ""
			if strings.HasSuffix(check.Path.String(), ".channel") {
				selected, candidate = strings.TrimSuffix(check.Path.String(), ".channel"), check.OtherPath.String()
			} else if strings.HasSuffix(check.OtherPath.String(), ".channel") {
				selected, candidate = strings.TrimSuffix(check.OtherPath.String(), ".channel"), check.Path.String()
			} else {
				return true
			}
			if result == "" {
				result = selected
			}
			if result != selected || !containsSelectCase(cases, candidate) {
				return true
			}
			handled[candidate] = true
		}
		missing := make([]string, 0, len(cases))
		covered := make([]string, 0, len(cases))
		for _, item := range cases {
			if handled[item] {
				covered = append(covered, item)
			} else {
				missing = append(missing, item)
			}
		}
		if len(missing) == 0 || len(covered) == 0 || !chain.HeadSpan.Valid() {
			return true
		}
		span := chain.HeadSpan
		out = append(out, selectConsumerDiagnostic{
			Key:     fmt.Sprintf("channel.select.exhaustiveness/%x/%d", compilation.Body, chain.ID),
			Message: "channel select is not exhaustive; missing " + caseWord(missing) + ": " + quotedCases(missing),
			Span:    span,
			Evidence: []DiagnosticEvidence{
				{Span: span, Kind: "abstract fact", Trust: "proven", Message: "branch chain checks channel `" + result + ".channel`"},
				{Span: span, Kind: "abstract fact", Trust: "proven", Message: "handled cases: " + quotedCases(covered)},
				{Span: span, Kind: "missing proof", Trust: "unknown", Message: "missing cases: " + quotedCases(missing)},
				{Span: span, Kind: "missing proof", Trust: "unknown", Message: "no default case handles the remaining channel cases"},
			},
		})
		return true
	})
	return out
}

func selectCatalogPublished(artifact equation.Artifact) bool {
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "channel-select" {
			continue
		}
		caseCount, payloadCount := 0, 0
		for _, operand := range operation.Operands {
			if strings.HasPrefix(operand.Role, "case-") && !strings.HasPrefix(operand.Role, "case-display-") {
				caseCount++
			}
			if strings.HasPrefix(operand.Role, "payload-type-") {
				payloadCount++
			}
		}
		if caseCount > 0 && caseCount == payloadCount {
			return true
		}
	}
	return false
}

func selectCaseDisplays(artifact equation.Artifact) []string {
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "channel-select" {
			continue
		}
		var out []string
		for _, operand := range operation.Operands {
			if strings.HasPrefix(operand.Role, "case-display-") && len(operand.Term.Encoding) != 0 {
				out = append(out, string(operand.Term.Encoding))
			}
		}
		sort.Strings(out)
		return out
	}
	return nil
}

func containsSelectCase(cases []string, value string) bool {
	return sort.SearchStrings(cases, value) < len(cases) && cases[sort.SearchStrings(cases, value)] == value
}

func quotedCases(cases []string) string {
	parts := make([]string, len(cases))
	for i, item := range cases {
		parts[i] = "`" + item + "`"
	}
	return strings.Join(parts, ", ")
}

func caseWord(cases []string) string {
	if len(cases) == 1 {
		return "case"
	}
	return "cases"
}

// channelSelectUnionConsumers uses a select arm's closed payload type as the
// authority for a nested literal-discriminant chain.  It deliberately rejects
// mixed chains and payloads without a finite registered origin family.
func channelSelectUnionConsumers(compilation front.Compilation) []selectConsumerDiagnostic {
	body, payloads := compilation.WIR, selectPayloadTypes(compilation.Artifact)
	if body == nil || len(payloads) == 0 {
		return nil
	}
	var out []selectConsumerDiagnostic
	body.ForEachIfChainDescriptor(func(chain wir.IfChainDescriptor) bool {
		if chain.HasElse || len(chain.Branches) < 2 || !chain.HeadSpan.Valid() {
			return true
		}
		path, handled := "", make(map[string]bool, len(chain.Branches))
		for _, branch := range chain.Branches {
			checks := body.BranchChecks(branch.Point)
			if len(checks) != 1 || checks[0].Kind != wir.CheckLiteralEqual || checks[0].Literal == nil {
				return true
			}
			if path == "" {
				path = checks[0].Path.String()
			}
			if path != checks[0].Path.String() {
				return true
			}
			handled[typeformat.Short(checks[0].Literal)] = true
		}
		if !strings.HasSuffix(path, ".kind") {
			return true
		}
		for _, payload := range payloads {
			_, cases, ok := variant.OriginCasesOfType(payload)
			if !ok || len(cases) < 2 {
				continue
			}
			possible := make([]string, 0, len(cases))
			for _, item := range cases {
				literal, found := variant.FieldAtPath(item.Type, []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}})
				if !found {
					possible = nil
					break
				}
				possible = append(possible, path+" == "+typeformat.Short(literal))
			}
			if len(possible) != len(cases) {
				continue
			}
			sort.Strings(possible)
			covered, missing := make([]string, 0, len(possible)), make([]string, 0, len(possible))
			for _, item := range possible {
				if handled[strings.TrimPrefix(item, path+" == ")] {
					covered = append(covered, item)
				} else {
					missing = append(missing, item)
				}
			}
			if len(covered) == 0 || len(missing) == 0 {
				continue
			}
			span := chain.HeadSpan
			out = append(out, selectConsumerDiagnostic{Key: fmt.Sprintf("lint.union.exhaustiveness/%x/%d", compilation.Body, chain.ID), Message: "discriminated union handling is not exhaustive; missing " + caseWord(missing) + ": " + quotedCases(missing), Span: span, Evidence: []DiagnosticEvidence{
				{Span: span, Kind: "abstract fact", Trust: "proven", Message: "branch chain checks discriminant `" + path + "`"},
				{Span: span, Kind: "abstract fact", Trust: "proven", Message: "possible cases: " + quotedCases(possible)},
				{Span: span, Kind: "abstract fact", Trust: "proven", Message: "handled cases: " + quotedCases(covered)},
				{Span: span, Kind: "missing proof", Trust: "unknown", Message: "missing cases: " + quotedCases(missing)},
				{Span: span, Kind: "missing proof", Trust: "unknown", Message: "no default branch handles the remaining union cases"},
			}})
			return true
		}
		return true
	})
	return out
}

func selectPayloadTypes(artifact equation.Artifact) []typ.Type {
	var out []typ.Type
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "channel-select" {
			continue
		}
		for _, operand := range operation.Operands {
			if !strings.HasPrefix(operand.Role, "payload-type-") {
				continue
			}
			if payload, ok := shapefact.DecodeTarget(operand.Term.Encoding); ok && payload != nil {
				out = append(out, payload)
			}
		}
	}
	return out
}
