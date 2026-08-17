package program

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/static"
)

func programStaticCounts(view static.View) ([10]int, error) {
	var counts [10]int
	if !view.Available() {
		return counts, errors.New("unavailable Static view")
	}
	types, declarations, signatures, contracts, operators, operands := view.Types(), view.Declarations(), view.Signatures(), view.Contracts(), view.Operators(), view.Operands()
	primaryParts := []int{
		declarations.Aliases().Count(), declarations.Interfaces().Count(), declarations.TypeParams().Count(),
		types.Primitives().Count(), types.Literals().Count(), types.Optionals().Count(), types.Unions().Count(), types.Intersections().Count(), types.Generics().Count(), types.Arrays().Count(), types.Maps().Count(), types.Records().Count(),
		view.References().Count(), signatures.TypeFunctions().Count(), signatures.Assertions().Count(), operators.TypeOfs().Count(), operators.KeyOfs().Count(), operators.IndexAccesses().Count(), operators.Conditionals().Count(),
	}
	primary, ok := sumProgramSemanticSourceMeasures(primaryParts...)
	if !ok {
		return counts, errors.New("Static primary cardinality overflow")
	}
	callArguments := 0
	for index := 0; index < contracts.Calls().Count(); index++ {
		term, ok := contracts.Calls().At(index)
		if !ok {
			return counts, errors.New("invalid Static call column")
		}
		argumentCount, ok := contracts.Calls().TypeArgumentCount(term)
		if !ok || !addProgramSemanticSourceMeasure(&callArguments, argumentCount) {
			return counts, errors.New("invalid Static call-argument column")
		}
	}
	values := []int{primary, contracts.Functions().Count(), callArguments, declarations.DeclaredTypes().Count(), operands.Claims().Count(), operands.TypeValues().Count(), operators.TypeOfs().Count(), operands.Annotations().Count(), view.Publications().Count(), view.References().Count()}
	if !programSemanticSourceCountsFit(values...) {
		return counts, errors.New("invalid Static semantic cardinality")
	}
	return [...]int{values[0], values[1], values[2], values[3], values[4], values[5], values[6], values[7], values[8], values[9]}, nil
}
