package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/diagnostic"
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
	catalog := selectCatalog(compilation.Artifact)
	if !catalog.published || len(catalog.cases) < 2 {
		return nil
	}
	var out []selectConsumerDiagnostic
	for _, chain := range selectBranchChains(compilation.Artifact) {
		if diagnostic, ok := channelSelectCoverageConsumer(compilation.Body, chain, catalog.cases); ok {
			out = append(out, diagnostic)
		}
	}
	return out
}

func channelSelectCoverageConsumer(identity [32]byte, chain selectBranchChain, cases []string) (selectConsumerDiagnostic, bool) {
	if chain.HasElse || len(chain.Checks) < 2 {
		return selectConsumerDiagnostic{}, false
	}
	handled, resultKey, resultDisplay := make(map[string]bool, len(chain.Checks)), "", ""
	for _, branch := range chain.Checks {
		if len(branch.Checks) != 1 || branch.Checks[0].Predicate.Kind != "path-equal" {
			return selectConsumerDiagnostic{}, false
		}
		check := branch.Checks[0]
		selectedKey, selectedDisplay, candidate := "", "", ""
		if check.Path.FinalField == "channel" {
			selectedKey, selectedDisplay, candidate = check.Path.ParentKey, check.Path.ParentDisplay, check.OtherPath.Display
		} else if check.OtherPath.FinalField == "channel" {
			selectedKey, selectedDisplay, candidate = check.OtherPath.ParentKey, check.OtherPath.ParentDisplay, check.Path.Display
		} else {
			return selectConsumerDiagnostic{}, false
		}
		if resultKey == "" {
			resultKey, resultDisplay = selectedKey, selectedDisplay
		}
		if resultKey != selectedKey || !containsSelectCase(cases, candidate) {
			return selectConsumerDiagnostic{}, false
		}
		handled[candidate] = true
	}
	covered, missing := selectCoveredMissing(cases, func(index int) bool { return handled[cases[index]] })
	if len(missing) == 0 || len(covered) == 0 || !chain.HeadSpan.Valid() {
		return selectConsumerDiagnostic{}, false
	}
	span := chain.HeadSpan
	return selectConsumerDiagnostic{Key: fmt.Sprintf(diagnosticFamilyPrefix(DiagnosticFamilyChannelSelectExhaustiveness)+"%x/%d", identity, chain.ID), Message: "channel select is not exhaustive; missing " + caseWord(missing) + ": " + quotedCases(missing), Span: span, Evidence: []DiagnosticEvidence{
		{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "branch chain checks channel `" + resultDisplay + ".channel`"},
		{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "handled cases: " + quotedCases(covered)},
		{Span: span, Kind: diagnostic.EvidenceMissingProof, Trust: diagnostic.TrustUnknown, Message: "missing cases: " + quotedCases(missing)},
		{Span: span, Kind: diagnostic.EvidenceMissingProof, Trust: diagnostic.TrustUnknown, Message: "no default case handles the remaining channel cases"},
	}}, true
}

type selectBranchChain struct {
	front.BranchChainWire
	Checks []front.BranchChainWire
}

// selectBranchChains admits a chain only when every published position is
// present exactly once and all copies agree on its closed topology.
func selectBranchChains(artifact equation.Artifact) []selectBranchChain {
	byID := make(map[uint32][]front.BranchChainWire)
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role != equation.RoleBranchChain {
				continue
			}
			chain, present, err := front.DecodeBranchChainWire(operand.Term.Encoding)
			if err == nil && present {
				byID[chain.ID] = append(byID[chain.ID], chain)
			}
		}
	}
	ids := make([]uint32, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]selectBranchChain, 0, len(ids))
	for _, id := range ids {
		copies := byID[id]
		if len(copies) == 0 || int(copies[0].Count) != len(copies) {
			continue
		}
		ordered := make([]front.BranchChainWire, len(copies))
		valid := true
		for _, chain := range copies {
			if chain.ID != id || chain.Count != copies[0].Count || chain.HasElse != copies[0].HasElse ||
				chain.HeadSpan != copies[0].HeadSpan || int(chain.Position) >= len(ordered) ||
				ordered[chain.Position].ID != 0 {
				valid = false
				break
			}
			ordered[chain.Position] = chain
		}
		if !valid {
			continue
		}
		out = append(out, selectBranchChain{BranchChainWire: copies[0], Checks: ordered})
	}
	return out
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
			if _, ok := operand.Role.Index(equation.RoleFamilyCase); ok {
				caseCount++
			}
			if _, ok := operand.Role.Index(equation.RoleFamilyPayloadType); ok {
				payloadCount++
				if payload, ok := shapefact.DecodeTarget(operand.Term.Encoding); ok && payload != nil {
					catalog.payloads = append(catalog.payloads, payload)
				}
			}
			if _, ok := operand.Role.Index(equation.RoleFamilyCaseDisplay); !casesSet && ok && len(operand.Term.Encoding) != 0 {
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
func selectDiscriminantField(subject front.BranchChainPathWire) bool {
	return subject.FinalField == selectDiscriminantName
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
	catalog := selectCatalog(compilation.Artifact)
	if len(catalog.payloads) == 0 {
		return nil
	}
	var out []selectConsumerDiagnostic
	for _, chain := range selectBranchChains(compilation.Artifact) {
		if diagnostic, ok := channelSelectUnionConsumer(compilation.Body, chain, catalog.payloads); ok {
			out = append(out, diagnostic)
		}
	}
	return out
}

func channelSelectUnionConsumer(identity [32]byte, chain selectBranchChain, payloads []typ.Type) (selectConsumerDiagnostic, bool) {
	if chain.HasElse || len(chain.Checks) < 2 || !chain.HeadSpan.Valid() {
		return selectConsumerDiagnostic{}, false
	}
	var discriminant front.BranchChainPathWire
	handled := make([]typ.Type, 0, len(chain.Checks))
	for position, branch := range chain.Checks {
		if len(branch.Checks) != 1 || branch.Checks[0].Predicate.Kind != "literal-equal" {
			return selectConsumerDiagnostic{}, false
		}
		check := branch.Checks[0]
		literal, ok := shapefact.DecodeTarget([]byte(check.LiteralTarget))
		if !ok || literal == nil {
			return selectConsumerDiagnostic{}, false
		}
		if position == 0 {
			discriminant = check.Path
		}
		if discriminant.Key != check.Path.Key {
			return selectConsumerDiagnostic{}, false
		}
		handled = append(handled, literal)
	}
	if !selectDiscriminantField(discriminant) {
		return selectConsumerDiagnostic{}, false
	}
	spelling := discriminant.Display
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
		return selectConsumerDiagnostic{Key: fmt.Sprintf(diagnosticFamilyPrefix(DiagnosticFamilyUnionExhaustiveness)+"%x/%d", identity, chain.ID), Message: "discriminated union handling is not exhaustive; missing " + caseWord(missing) + ": " + quotedCases(missing), Span: span, Evidence: []DiagnosticEvidence{
			{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "branch chain checks discriminant `" + spelling + "`"},
			{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "possible cases: " + quotedCases(displays)},
			{Span: span, Kind: diagnostic.EvidenceAbstractFact, Trust: diagnostic.TrustProven, Message: "handled cases: " + quotedCases(covered)},
			{Span: span, Kind: diagnostic.EvidenceMissingProof, Trust: diagnostic.TrustUnknown, Message: "missing cases: " + quotedCases(missing)},
			{Span: span, Kind: diagnostic.EvidenceMissingProof, Trust: diagnostic.TrustUnknown, Message: "no default branch handles the remaining union cases"},
		}}, true
	}
	return selectConsumerDiagnostic{}, false
}
