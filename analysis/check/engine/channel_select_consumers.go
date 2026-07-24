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

// selectConsumerDiagnostic is a source-facing consequence of closed select facts.
type selectConsumerDiagnostic struct {
	Key, Message string
	Span         wir.Span
	Evidence     []DiagnosticEvidence
}

// channelSelectCoverageConsumers proves complete authored elseif chains against evaluated select catalogs.
// Missing facts, mixed predicates, unresolved aliases, and a final else fail closed.
func channelSelectCoverageConsumers(compilation front.Compilation) []selectConsumerDiagnostic {
	catalog, body := selectCatalog(compilation.Artifact), compilation.WIR
	if body == nil || !catalog.published || len(catalog.cases) < 2 {
		return nil
	}
	var out []selectConsumerDiagnostic
	body.ForEachIfChainDescriptor(func(chain wir.IfChainDescriptor) bool {
		if diagnostic, ok := channelSelectCoverageConsumer(body, compilation.Body, chain, catalog.cases); ok {
			out = append(out, diagnostic)
		}
		return true
	})
	return out
}

func channelSelectCoverageConsumer(body *wir.Body, identity [32]byte, chain wir.IfChainDescriptor, cases []string) (selectConsumerDiagnostic, bool) {
	if chain.HasElse || len(chain.Branches) < 2 {
		return selectConsumerDiagnostic{}, false
	}
	handled, result := make(map[string]bool, len(chain.Branches)), ""
	for _, branch := range chain.Branches {
		checks := body.BranchChecks(branch.Point)
		if len(checks) != 1 || checks[0].Kind != wir.CheckPathEqual {
			return selectConsumerDiagnostic{}, false
		}
		check := checks[0]
		selected, candidate := "", ""
		if strings.HasSuffix(check.Path.String(), ".channel") {
			selected, candidate = strings.TrimSuffix(check.Path.String(), ".channel"), check.OtherPath.String()
		} else if strings.HasSuffix(check.OtherPath.String(), ".channel") {
			selected, candidate = strings.TrimSuffix(check.OtherPath.String(), ".channel"), check.Path.String()
		} else {
			return selectConsumerDiagnostic{}, false
		}
		if result == "" {
			result = selected
		}
		if result != selected || !containsSelectCase(cases, candidate) {
			return selectConsumerDiagnostic{}, false
		}
		handled[candidate] = true
	}
	covered, missing := selectCoveredMissing(cases, handled)
	if len(missing) == 0 || len(covered) == 0 || !chain.HeadSpan.Valid() {
		return selectConsumerDiagnostic{}, false
	}
	span := chain.HeadSpan
	return selectConsumerDiagnostic{Key: fmt.Sprintf("channel.select.exhaustiveness/%x/%d", identity, chain.ID), Message: "channel select is not exhaustive; missing " + caseWord(missing) + ": " + quotedCases(missing), Span: span, Evidence: []DiagnosticEvidence{
		{Span: span, Kind: "abstract fact", Trust: "proven", Message: "branch chain checks channel `" + result + ".channel`"},
		{Span: span, Kind: "abstract fact", Trust: "proven", Message: "handled cases: " + quotedCases(covered)},
		{Span: span, Kind: "missing proof", Trust: "unknown", Message: "missing cases: " + quotedCases(missing)},
		{Span: span, Kind: "missing proof", Trust: "unknown", Message: "no default case handles the remaining channel cases"},
	}}, true
}

type selectCatalogFact struct {
	published bool
	cases     []string
	payloads  []typ.Type
}

func selectCatalog(artifact equation.Artifact) (catalog selectCatalogFact) {
	casesSet := false
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
				if payload, ok := shapefact.DecodeTarget(operand.Term.Encoding); ok && payload != nil {
					catalog.payloads = append(catalog.payloads, payload)
				}
			}
			if !casesSet && strings.HasPrefix(operand.Role, "case-display-") && len(operand.Term.Encoding) != 0 {
				catalog.cases = append(catalog.cases, string(operand.Term.Encoding))
			}
		}
		if caseCount > 0 && caseCount == payloadCount {
			catalog.published = true
		}
		casesSet = true
	}
	sort.Strings(catalog.cases)
	return catalog
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

func selectCoveredMissing(cases []string, handled map[string]bool) (covered, missing []string) {
	covered, missing = make([]string, 0, len(cases)), make([]string, 0, len(cases))
	for _, item := range cases {
		if handled[item] {
			covered = append(covered, item)
		} else {
			missing = append(missing, item)
		}
	}
	return covered, missing
}

// channelSelectUnionConsumers uses a select arm's closed payload type for nested literal-discriminant chains.
// It rejects mixed chains and payloads without a finite registered origin family.
func channelSelectUnionConsumers(compilation front.Compilation) []selectConsumerDiagnostic {
	catalog, body := selectCatalog(compilation.Artifact), compilation.WIR
	if body == nil || len(catalog.payloads) == 0 {
		return nil
	}
	var out []selectConsumerDiagnostic
	body.ForEachIfChainDescriptor(func(chain wir.IfChainDescriptor) bool {
		if diagnostic, ok := channelSelectUnionConsumer(body, compilation.Body, chain, catalog.payloads); ok {
			out = append(out, diagnostic)
		}
		return true
	})
	return out
}

func channelSelectUnionConsumer(body *wir.Body, identity [32]byte, chain wir.IfChainDescriptor, payloads []typ.Type) (selectConsumerDiagnostic, bool) {
	if chain.HasElse || len(chain.Branches) < 2 || !chain.HeadSpan.Valid() {
		return selectConsumerDiagnostic{}, false
	}
	path, handled := "", make(map[string]bool, len(chain.Branches))
	for _, branch := range chain.Branches {
		checks := body.BranchChecks(branch.Point)
		if len(checks) != 1 || checks[0].Kind != wir.CheckLiteralEqual || checks[0].Literal == nil {
			return selectConsumerDiagnostic{}, false
		}
		if path == "" {
			path = checks[0].Path.String()
		}
		if path != checks[0].Path.String() {
			return selectConsumerDiagnostic{}, false
		}
		handled[path+" == "+typeformat.Short(checks[0].Literal)] = true
	}
	if !strings.HasSuffix(path, ".kind") {
		return selectConsumerDiagnostic{}, false
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
		covered, missing := selectCoveredMissing(possible, handled)
		if len(covered) == 0 || len(missing) == 0 {
			continue
		}
		span := chain.HeadSpan
		return selectConsumerDiagnostic{Key: fmt.Sprintf("lint.union.exhaustiveness/%x/%d", identity, chain.ID), Message: "discriminated union handling is not exhaustive; missing " + caseWord(missing) + ": " + quotedCases(missing), Span: span, Evidence: []DiagnosticEvidence{
			{Span: span, Kind: "abstract fact", Trust: "proven", Message: "branch chain checks discriminant `" + path + "`"},
			{Span: span, Kind: "abstract fact", Trust: "proven", Message: "possible cases: " + quotedCases(possible)},
			{Span: span, Kind: "abstract fact", Trust: "proven", Message: "handled cases: " + quotedCases(covered)},
			{Span: span, Kind: "missing proof", Trust: "unknown", Message: "missing cases: " + quotedCases(missing)},
			{Span: span, Kind: "missing proof", Trust: "unknown", Message: "no default branch handles the remaining union cases"},
		}}, true
	}
	return selectConsumerDiagnostic{}, false
}
