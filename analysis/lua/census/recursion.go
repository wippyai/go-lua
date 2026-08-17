package census

import "fmt"

// RecursionFamily is a closed grammar recursion family with a distinct semantic
// consumer. Parameter forms stay separate from value-list forms: they are
// different obligations downstream even where their grammar shape rhymes.
type RecursionFamily uint8

const (
	RecursionFamilyInvalid RecursionFamily = iota
	RecursionFamilyChunk
	RecursionFamilyExpressionList
	RecursionFamilyVariableList
	RecursionFamilyFieldList
	RecursionFamilyElseIfList
	RecursionFamilyTypedNameList
	RecursionFamilyFunctionParameterList
	RecursionFamilyTypeExpressionList
	RecursionFamilyNesting
)

// RecursionStage is one structural induction premise. Only the two premises the
// grammar itself can discharge are stated: a base alternative that terminates
// the family, and a step alternative that re-enters it. Final-adjustment laws
// are not induction premises and must name Program rows, not parser rows.
type RecursionStage uint8

const (
	RecursionStageInvalid RecursionStage = iota
	RecursionStageBase
	RecursionStageStep
)

// RecursionPremise is one structural induction premise over an exact parser
// nonterminal. The nonterminal is a parser symbol, never a rule description.
type RecursionPremise struct {
	Family      RecursionFamily
	Stage       RecursionStage
	Nonterminal string
}

// RecursionReport is the complete induction denominator and the premises the
// grammar does not discharge. It is derived from census production rows alone,
// so no accepted source, fixture, or parser run can move a premise from Missing
// to discharged.
type RecursionReport struct {
	Required []RecursionPremise
	Missing  []RecursionPremise
}

type recursionSpec struct {
	family      RecursionFamily
	nonterminal string
}

// recursionFamilies names the recursive nonterminals whose induction the
// language depends on. The list is a statement of which families exist, not a
// heuristic: each entry is an exact parser symbol, and a symbol that leaves
// parser.go.y makes its premises Missing rather than silently dropping out.
var recursionFamilies = [...]recursionSpec{
	{family: RecursionFamilyChunk, nonterminal: "chunk1"},
	{family: RecursionFamilyExpressionList, nonterminal: "exprlist"},
	{family: RecursionFamilyVariableList, nonterminal: "varlist"},
	{family: RecursionFamilyFieldList, nonterminal: "fieldlist"},
	{family: RecursionFamilyElseIfList, nonterminal: "elseifs"},
	{family: RecursionFamilyTypedNameList, nonterminal: "typednamelist"},
	{family: RecursionFamilyFunctionParameterList, nonterminal: "funcparamlist"},
	{family: RecursionFamilyTypeExpressionList, nonterminal: "typeexprlist"},
}

// Recursion derives the structural induction premises from census production
// rows. A list family is inductive when it states at least one alternative that
// does not re-enter its own nonterminal and at least one that does; expression
// nesting is inductive when the grammar states the parenthesised re-entry.
// Every input is a production row the census already carries, so this law
// outlives the parser-trace evidence it was first proved beside.
func Recursion(value Census) RecursionReport {
	report := RecursionReport{}
	for _, spec := range recursionFamilies {
		base, step := false, false
		for _, production := range value.Productions {
			if production.Nonterminal != spec.nonterminal {
				continue
			}
			if containsSymbol(production.RHS, spec.nonterminal) {
				step = true
			} else {
				base = true
			}
		}
		report.add(spec.family, spec.nonterminal, RecursionStageBase, base)
		report.add(spec.family, spec.nonterminal, RecursionStageStep, step)
	}
	nesting := false
	for _, production := range value.Productions {
		if production.Nonterminal == "prefixexp" && containsSequence(production.RHS, "'('", "expr", "')'") {
			nesting = true
			break
		}
	}
	report.add(RecursionFamilyNesting, "prefixexp", RecursionStageBase, nesting)
	return report
}

// Validate makes the report usable as a gate without letting the inventory
// itself pass for a completeness claim: an empty denominator is as much a
// failure as an undischarged premise.
func (r RecursionReport) Validate() error {
	if len(r.Required) == 0 {
		return fmt.Errorf("parser census: empty structural induction denominator")
	}
	if len(r.Missing) != 0 {
		return fmt.Errorf("parser census: %d structural induction premises are absent", len(r.Missing))
	}
	return nil
}

func (r *RecursionReport) add(family RecursionFamily, nonterminal string, stage RecursionStage, found bool) {
	premise := RecursionPremise{Family: family, Nonterminal: nonterminal, Stage: stage}
	r.Required = append(r.Required, premise)
	if !found {
		r.Missing = append(r.Missing, premise)
	}
}

func containsSymbol(values []string, want string) bool {
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
