package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
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
	covered, missing := selectCoveredMissing(cases, func(index int) bool { return handled[cases[index]] })
	if len(missing) == 0 || len(covered) == 0 || !chain.HeadSpan.Valid() {
		return selectConsumerDiagnostic{}, false
	}
	span := chain.HeadSpan
	return selectConsumerDiagnostic{Key: fmt.Sprintf("channel.select.exhaustiveness/%x/%d", identity, chain.ID), Message: "channel select is not exhaustive; missing " + caseWord(missing) + ": " + quotedCases(missing), Span: span, Evidence: []DiagnosticEvidence{
		{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "branch chain checks channel `" + result + ".channel`"},
		{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "handled cases: " + quotedCases(covered)},
		{Span: span, Kind: diagnostic.EvidenceMissingProof, Trust: diagnostic.TrustUnknown, Message: "missing cases: " + quotedCases(missing)},
		{Span: span, Kind: diagnostic.EvidenceMissingProof, Trust: diagnostic.TrustUnknown, Message: "no default case handles the remaining channel cases"},
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

// selectCoveredMissing splits the cases a chain could handle into the ones it
// does and the ones it does not, in the order they are reported. The caller
// decides handledness; this states only the reported spelling of each side.
func selectCoveredMissing(cases []string, handled func(index int) bool) (covered, missing []string) {
	covered, missing = make([]string, 0, len(cases)), make([]string, 0, len(cases))
	for index, item := range cases {
		if handled(index) {
			covered = append(covered, item)
		} else {
			missing = append(missing, item)
		}
	}
	return covered, missing
}

// selectDiscriminantName is the field a discriminated union states its case in.
// The origin family and the branch chain both read that member, so the two
// sides of the exhaustiveness comparison name the same slot.
const selectDiscriminantName = "kind"

// selectDiscriminantField reports whether a branch chain tests the union's
// discriminant member. The final segment carries that decision, so the path is
// read by its segments rather than by its rendering.
func selectDiscriminantField(subject path.Path) bool {
	if len(subject.Segments) == 0 {
		return false
	}
	last := subject.Segments[len(subject.Segments)-1]
	return last.Kind == segment.SegmentField && last.Name == selectDiscriminantName
}

// selectDiscriminantCase pairs one union case's discriminant with the spelling
// the diagnostic reports it by. Coverage is decided by the discriminant; the
// spelling is presentation and is never a comparison authority.
type selectDiscriminantCase struct {
	literal typ.Type
	display string
}

// selectDiscriminantHandled reports whether a branch chain tested exactly this
// discriminant. Two literals of different base kinds render alike, so the value
// decides rather than its rendering.
func selectDiscriminantHandled(literal typ.Type, handled []typ.Type) bool {
	for _, item := range handled {
		if typ.TypeEquals(literal, item) {
			return true
		}
	}
	return false
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
	var discriminant path.Path
	handled := make([]typ.Type, 0, len(chain.Branches))
	for position, branch := range chain.Branches {
		checks := body.BranchChecks(branch.Point)
		if len(checks) != 1 || checks[0].Kind != wir.CheckLiteralEqual || checks[0].Literal == nil {
			return selectConsumerDiagnostic{}, false
		}
		if position == 0 {
			discriminant = checks[0].Path
		}
		if !discriminant.Equal(checks[0].Path) {
			return selectConsumerDiagnostic{}, false
		}
		handled = append(handled, checks[0].Literal)
	}
	if !selectDiscriminantField(discriminant) {
		return selectConsumerDiagnostic{}, false
	}
	spelling := discriminant.String()
	for _, payload := range payloads {
		_, cases, ok := variant.OriginCasesOfType(payload)
		if !ok || len(cases) < 2 {
			continue
		}
		possible := make([]selectDiscriminantCase, 0, len(cases))
		for _, item := range cases {
			literal, found := variant.FieldAtPath(item.Type, []segment.Segment{{Kind: segment.SegmentField, Name: selectDiscriminantName}})
			if !found {
				possible = nil
				break
			}
			possible = append(possible, selectDiscriminantCase{literal: literal, display: spelling + " == " + typeformat.Short(literal)})
		}
		if len(possible) != len(cases) {
			continue
		}
		sort.SliceStable(possible, func(i, j int) bool { return possible[i].display < possible[j].display })
		displays := make([]string, len(possible))
		for index, item := range possible {
			displays[index] = item.display
		}
		covered, missing := selectCoveredMissing(displays, func(index int) bool {
			return selectDiscriminantHandled(possible[index].literal, handled)
		})
		if len(covered) == 0 || len(missing) == 0 {
			continue
		}
		span := chain.HeadSpan
		return selectConsumerDiagnostic{Key: fmt.Sprintf("lint.union.exhaustiveness/%x/%d", identity, chain.ID), Message: "discriminated union handling is not exhaustive; missing " + caseWord(missing) + ": " + quotedCases(missing), Span: span, Evidence: []DiagnosticEvidence{
			{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "branch chain checks discriminant `" + spelling + "`"},
			{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "possible cases: " + quotedCases(displays)},
			{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "handled cases: " + quotedCases(covered)},
			{Span: span, Kind: diagnostic.EvidenceMissingProof, Trust: diagnostic.TrustUnknown, Message: "missing cases: " + quotedCases(missing)},
			{Span: span, Kind: diagnostic.EvidenceMissingProof, Trust: diagnostic.TrustUnknown, Message: "no default branch handles the remaining union cases"},
		}}, true
	}
	return selectConsumerDiagnostic{}, false
}
