package library

import (
	"fmt"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
)

// operations projects the provider-owned default ABI into Target and then
// applies only operational laws: alternative outcomes, callbacks,
// yield/resume, allocation identity, aliases, and rule delegation. Parameter
// and return declarations are never authored here.
func operations(declarations *manifest.Catalogue) (authoredCatalogue, error) {
	if declarations == nil {
		return authoredCatalogue{}, fmt.Errorf("target: nil declaration catalogue")
	}
	var catalogue authoredCatalogue
	for _, declaration := range declarations.Functions() {
		catalogue.add(declaration.CanonicalPath(), operationFromDeclaration(declaration))
	}

	// Specialization is deliberately behavioral. The default parameter and
	// return envelope above remains manifest-owned.
	for name, specialized := range map[string]OperationSpec{
		"pcall":                pcallProfile(nativeBuiltin("pcall")),
		"print":                printProfile(),
		"tostring":             tostringProfile(),
		"xpcall":               xpcallProfile(nativeBuiltin("xpcall")),
		"ipairs":               ipairsProfile(),
		"pairs":                pairsProfile(),
		"table.concat":         tableConcatProfile(),
		"table.insert":         tableInsertProfile(),
		"table.remove":         tableRemoveProfile(),
		"table.sort":           tableSortProfile(),
		"unpack":               tableUnpackProfile(),
		"string.format":        formatProfile(),
		"string.gsub":          callbackGsubProfile(),
		"math.min":             minMaxProfile(module("math", "min")),
		"math.max":             minMaxProfile(module("math", "max")),
		"coroutine.create":     callbackCreate(module("coroutine", "create")),
		"coroutine.resume":     resumeEnvelope(),
		"coroutine.wrap":       callbackWrap(module("coroutine", "wrap")),
		"coroutine.spawn":      callbackSpawn(),
		"debug.getupvalue":     alternatives(module("debug", "getupvalue"), []typ.Type{typ.Any, typ.Integer}, false, [][]typ.Type{{typ.String, typ.Any}, {typ.Nil}}),
		"errors.Error.details": alternativesTotal(OperationSpec{Bindings: []BindingSpec{{Namespace: BindingModule, Owner: []string{"errors"}, Member: []string{"Error", "details"}}}}, []typ.Type{typ.Any}, false, [][]typ.Type{{typ.BuiltinTableTopMarker()}, {typ.Nil}}),
	} {
		if err := catalogue.replace(name, specialized); err != nil {
			return authoredCatalogue{}, err
		}
	}

	if err := appendNormalAlternative(&catalogue, "string.find", closed(typ.Nil)); err != nil {
		return authoredCatalogue{}, err
	}
	if err := appendNormalAlternative(&catalogue, "string.match", closed(typ.Nil)); err != nil {
		return authoredCatalogue{}, err
	}
	if err := appendNormalAlternative(&catalogue, "math.random", closed(typ.Integer)); err != nil {
		return authoredCatalogue{}, err
	}
	if err := replaceNormalAlternatives(&catalogue, "math.tointeger", closed(typ.Integer), closed(typ.Nil)); err != nil {
		return authoredCatalogue{}, err
	}
	if err := replaceNormalAlternatives(&catalogue, "math.type", closed(typ.String), closed(typ.Nil)); err != nil {
		return authoredCatalogue{}, err
	}
	if err := replaceNormalAlternatives(&catalogue, "utf8.len", closed(typ.Integer), closed(typ.Nil, typ.Integer)); err != nil {
		return authoredCatalogue{}, err
	}
	if err := appendNormalAlternative(&catalogue, "utf8.offset", closed(typ.Nil)); err != nil {
		return authoredCatalogue{}, err
	}

	if err := catalogue.inputTailType("string.char", typ.Integer); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.outcomeTailType("string.byte", 0, typ.Integer); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.outcomeTailType("utf8.codepoint", 0, typ.Integer); err != nil {
		return authoredCatalogue{}, err
	}
	yieldRef, ok := catalogue.lookup("coroutine.yield")
	if !ok {
		return authoredCatalogue{}, fmt.Errorf("target catalogue: missing coroutine.yield declaration")
	}
	yield := catalogue.at(yieldRef)
	yield.Outcomes = append(yield.Outcomes, OutcomeSpec{Kind: flowkind.OutcomeYield, Values: values(nil, true, 0)})
	yield.Suspensions = []SuspensionSpec{{Yield: uint32(len(yield.Outcomes) - 1), Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryOnce}}

	// Produced-only callables have no provider binding and therefore remain
	// target operational declarations.
	wrapInvoke := normal(OperationSpec{}, nil, true, nil, true)
	wrapInvoke.Resumes = []ResumeSpec{resumeRelation(ResumeSourceProduced, 0, 0, 0, 1)}
	catalogue.add("coroutine.wrap.invoke", wrapInvoke)
	gmatchNext := normal(OperationSpec{}, nil, false, nil, true)
	gmatchNext.Outcomes = append(gmatchNext.Outcomes, OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values(nil, false, 0)})
	catalogue.add("string.gmatch.next", gmatchNext)
	catalogue.add("ipairs.aux", ipairsAuxProfile())
	catalogue.add("utf8.codes.aux", alternatives(OperationSpec{}, []typ.Type{typ.String, typ.Integer}, false, [][]typ.Type{nil, {typ.Integer, typ.Integer}}))
	return catalogue, nil
}

func operationFromDeclaration(declaration manifest.Function) OperationSpec {
	binding := OperationSpec{Bindings: bindingsFromDeclaration(declaration)}
	function := declaration.Signature()
	if function.Type == nil {
		return normal(binding, nil, false, nil, false)
	}
	fixed := make([]typ.Type, 0, len(function.Type.Params))
	optional := make([]typ.Type, 0)
	arguments := make([]typ.Type, len(function.Type.TypeParams))
	for index, parameter := range function.Type.TypeParams {
		arguments[index] = typ.Any
		if parameter != nil && parameter.Constraint != nil {
			arguments[index] = parameter.Constraint
		}
	}
	materialize := func(value typ.Type) typ.Type {
		return subst.Params(value, function.Type.TypeParams, arguments)
	}
	for _, parameter := range function.Type.Params {
		if parameter.Optional {
			optional = append(optional, materialize(parameter.Type))
			continue
		}
		fixed = append(fixed, materialize(parameter.Type))
	}
	open := function.Type.Variadic != nil || len(optional) != 0
	returns := make([]typ.Type, len(function.Type.Returns))
	for index, value := range function.Type.Returns {
		returns[index] = successfulResultType(materialize(value))
	}
	operation := normal(binding, fixed, open, returns, function.ResultTail != nil)
	if function.ResultTail != nil {
		operation.Outcomes[0].Values.TailType = portable(materialize(function.ResultTail))
	}
	if len(function.ResultSuffix) != 0 {
		suffix := make([]typ.Type, len(function.ResultSuffix))
		for index, value := range function.ResultSuffix {
			suffix[index] = materialize(value)
		}
		operation.Outcomes[0].Values.Suffix = portableList(suffix)
	}
	if open {
		tail := append(optional, materialize(function.Type.Variadic))
		filtered := tail[:0]
		for _, value := range tail {
			if value != nil {
				filtered = append(filtered, value)
			}
		}
		if len(filtered) == 1 {
			operation.Input.TailType = portable(filtered[0])
		} else if len(filtered) > 1 {
			operation.Input.TailType = portable(typ.MaterializeUnion(filtered))
		}
	}
	if len(returns) == 1 && typ.TypeEquals(returns[0], typ.Never) {
		operation.Outcomes = operation.Outcomes[1:]
	}
	return operation
}

func bindingsFromDeclaration(declaration manifest.Function) []BindingSpec {
	out := make([]BindingSpec, 0, len(declaration.Bindings()))
	for _, binding := range declaration.Bindings() {
		switch binding.Mount() {
		case manifest.MountGlobals:
			out = append(out, BindingSpec{Namespace: BindingBuiltin, Member: binding.Member()})
		case manifest.MountModule:
			out = append(out, BindingSpec{Namespace: BindingModule, Owner: []string{binding.ModulePath()}, Member: binding.Member()})
		case manifest.MountDetached:
		}
	}
	return out
}

func successfulResultType(value typ.Type) typ.Type {
	if optional, ok := typ.UnwrapTransparentWrappers(value).(*typ.Optional); ok && optional.Inner != nil {
		return optional.Inner
	}
	return value
}

func appendNormalAlternative(catalogue *authoredCatalogue, name string, alternative ValuesSpec) error {
	ref, err := catalogue.require(name)
	if err != nil {
		return err
	}
	catalogue.at(ref).Outcomes = append(catalogue.at(ref).Outcomes, OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: alternative})
	return nil
}

func replaceNormalAlternatives(catalogue *authoredCatalogue, name string, alternatives ...ValuesSpec) error {
	ref, err := catalogue.require(name)
	if err != nil {
		return err
	}
	op := catalogue.at(ref)
	next := make([]OutcomeSpec, 0, len(alternatives))
	for _, values := range alternatives {
		next = append(next, OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values})
	}
	op.Outcomes = next
	return nil
}
