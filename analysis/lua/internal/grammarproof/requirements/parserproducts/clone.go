package parserproducts

import (
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/recursion"
)

// clone prevents a checked-in authority from being mutated through a caller's
// returned slice. It is intentionally explicit: every typed arena and row
// family must opt in rather than sharing a hidden backing array.
func clone(source Evidence) Evidence {
	result := source
	result.Fields = append([]FieldState(nil), source.Fields...)
	result.Products = make([]Product, len(source.Products))
	for index, row := range source.Products {
		result.Products[index] = row
		result.Products[index].States = append([]grammarproof.FieldState(nil), row.States...)
	}
	result.ProductLaws = make([]ProductLaw, len(source.ProductLaws))
	for index, law := range source.ProductLaws {
		result.ProductLaws[index] = cloneProductLaw(law)
	}
	result.HelperLaws = make([]HelperLaw, len(source.HelperLaws))
	for index, law := range source.HelperLaws {
		result.HelperLaws[index] = cloneHelperLaw(law)
	}
	result.Sequences = make([]SequenceLaw, len(source.Sequences))
	for index, row := range source.Sequences {
		result.Sequences[index] = row
		result.Sequences[index].Segments = append([]SequenceSegment(nil), row.Segments...)
	}
	result.Mutations = make([]FieldMutation, len(source.Mutations))
	for index, row := range source.Mutations {
		result.Mutations[index] = row
		result.Mutations[index].Edit = cloneEdit(row.Edit)
	}
	result.ActionTerms.Symbols = append([]ActionSymbol(nil), source.ActionTerms.Symbols...)
	result.ActionTerms.Scopes = append([]ActionScope(nil), source.ActionTerms.Scopes...)
	result.ActionTerms.Terms = append([]ActionTerm(nil), source.ActionTerms.Terms...)
	result.ActionTerms.Edges = append([]ActionEdge(nil), source.ActionTerms.Edges...)
	result.ActionTerms.ChainTails = append([]ChainTail(nil), source.ActionTerms.ChainTails...)
	result.ActionTerms.PlaceSteps = append([]PlaceStep(nil), source.ActionTerms.PlaceSteps...)
	result.ActionTerms.GuardSymbols = append([]ActionSymbolID(nil), source.ActionTerms.GuardSymbols...)
	result.Carriers = append([]Carrier(nil), source.Carriers...)
	result.Recursion = append([]recursion.Obligation(nil), source.Recursion...)
	return result
}

func cloneProductLaw(source ProductLaw) ProductLaw {
	result := source
	result.RHS = append([]string(nil), source.RHS...)
	result.Products = cloneProducts(source.Products)
	result.Helpers = cloneApplications(source.Helpers)
	result.Edits = cloneEdits(source.Edits)
	result.Rejects = cloneRejects(source.Rejects)
	result.Chains = cloneChains(source.Chains)
	return result
}

func cloneHelperLaw(source HelperLaw) HelperLaw {
	result := source
	result.Returns = make([]GuardedReturn, len(source.Returns))
	for index, row := range source.Returns {
		result.Returns[index] = row
		result.Returns[index].Guard = cloneGuard(row.Guard)
		result.Returns[index].Values = append([]ActionTermID(nil), row.Values...)
	}
	result.Rejects = cloneRejects(source.Rejects)
	result.Products = cloneProducts(source.Products)
	result.Helpers = cloneApplications(source.Helpers)
	result.Edits = cloneEdits(source.Edits)
	result.Summary.Maps = append([]MapIndex(nil), source.Summary.Maps...)
	result.Summary.Presence = append([]ConditionalPresence(nil), source.Summary.Presence...)
	return result
}

func cloneProducts(source []ConstructorProduct) []ConstructorProduct {
	result := make([]ConstructorProduct, len(source))
	for index, row := range source {
		result[index] = row
		result[index].Guard = cloneGuard(row.Guard)
		result[index].Fields = append([]ProductField(nil), row.Fields...)
	}
	return result
}

func cloneApplications(source []HelperApplication) []HelperApplication {
	result := make([]HelperApplication, len(source))
	for index, row := range source {
		result[index] = row
		result[index].Guard = cloneGuard(row.Guard)
		result[index].Actuals = append([]ActionTermID(nil), row.Actuals...)
		result[index].Results = append([]Place(nil), row.Results...)
	}
	return result
}

func cloneEdits(source []Edit) []Edit {
	result := make([]Edit, len(source))
	for index, row := range source {
		result[index] = cloneEdit(row)
	}
	return result
}

func cloneEdit(source Edit) Edit {
	result := source
	result.Guard = cloneGuard(source.Guard)
	return result
}

func cloneRejects(source []Reject) []Reject {
	result := make([]Reject, len(source))
	for index, row := range source {
		result[index] = row
		result[index].Guard = cloneGuard(row.Guard)
	}
	return result
}

func cloneChains(source []ChainLaw) []ChainLaw {
	result := make([]ChainLaw, len(source))
	for index, row := range source {
		result[index] = row
		result[index].Guard = cloneGuard(row.Guard)
	}
	return result
}

func cloneGuard(source Guard) Guard {
	return Guard{Atoms: append([]GuardAtom(nil), source.Atoms...)}
}
