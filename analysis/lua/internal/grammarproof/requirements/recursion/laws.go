// Package recursion owns cold structural induction obligations for recursive
// parser forms. It does not infer executable recurrence: Program Mu remains
// the sole execution recurrence representation.
package recursion

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// Family is a closed source-grammar recursion family with a distinct semantic
// consumer. Parameter forms remain separate from Values/list forms.
type Family uint8

const (
	FamilyInvalid Family = iota
	FamilyChunk
	FamilyExpressionList
	FamilyVariableList
	FamilyFieldList
	FamilyElseIfList
	FamilyTypedNameList
	FamilyFunctionParameterList
	FamilyTypeExpressionList
	FamilyNesting
)

// Stage is one structural induction premise. Final-adjustment laws are
// deliberately not claimed here: they are Program Values obligations and must
// name exact Values rows, not parser list evidence.
type Stage uint8

const (
	StageInvalid Stage = iota
	StageBase
	StageStep
)

// Obligation is one source grammar induction premise. Nonterminal is an exact
// parser symbol, not a generic rule string.
type Obligation struct {
	Family      Family
	Stage       Stage
	Nonterminal string
}

// Report is independent from the fixture corpus. Missing means parser.go.y no
// longer contains the required structural base or inductive production.
type Report struct {
	Required []Obligation
	Missing  []Obligation
}

func (r Report) RequiredCount() int { return len(r.Required) }
func (r Report) MissingCount() int  { return len(r.Missing) }

type familySpec struct {
	family      Family
	nonterminal string
}

var families = [...]familySpec{
	{family: FamilyChunk, nonterminal: "chunk1"},
	{family: FamilyExpressionList, nonterminal: "exprlist"},
	{family: FamilyVariableList, nonterminal: "varlist"},
	{family: FamilyFieldList, nonterminal: "fieldlist"},
	{family: FamilyElseIfList, nonterminal: "elseifs"},
	{family: FamilyTypedNameList, nonterminal: "typednamelist"},
	{family: FamilyFunctionParameterList, nonterminal: "funcparamlist"},
	{family: FamilyTypeExpressionList, nonterminal: "typeexprlist"},
}

// Discover verifies source-level base and step premises for every closed list
// family, plus the explicit parser nesting production. It derives only from
// grammar alternatives; no accepted source or fixture can alter the result.
func Discover(root string) (Report, error) {
	alternatives, err := parsersource.Alternatives(root)
	if err != nil {
		return Report{}, err
	}
	report := Report{}
	for _, spec := range families {
		base, step := false, false
		for _, alternative := range alternatives {
			if alternative.Nonterminal != spec.nonterminal {
				continue
			}
			if contains(alternative.RHS, spec.nonterminal) {
				step = true
			} else {
				base = true
			}
		}
		report.add(spec.family, spec.nonterminal, StageBase, base)
		report.add(spec.family, spec.nonterminal, StageStep, step)
	}
	nesting := false
	for _, alternative := range alternatives {
		if alternative.Nonterminal == "prefixexp" && containsSequence(alternative.RHS, "'('", "expr", "')'") {
			nesting = true
			break
		}
	}
	report.add(FamilyNesting, "prefixexp", StageBase, nesting)
	return report, nil
}

func (r *Report) add(family Family, nonterminal string, stage Stage, found bool) {
	obligation := Obligation{Family: family, Nonterminal: nonterminal, Stage: stage}
	r.Required = append(r.Required, obligation)
	if !found {
		r.Missing = append(r.Missing, obligation)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSequence(values []string, want ...string) bool {
	for start := 0; start+len(want) <= len(values); start++ {
		match := true
		for index := range want {
			if values[start+index] != want[index] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Validate makes the report usable by the final semantic gate without making
// the intermediate inventory itself a false Program-completeness claim.
func (r Report) Validate() error {
	if len(r.Required) == 0 {
		return fmt.Errorf("recursion requirements: empty induction denominator")
	}
	if len(r.Missing) != 0 {
		return fmt.Errorf("recursion requirements: %d structural induction premises are absent", len(r.Missing))
	}
	return nil
}
