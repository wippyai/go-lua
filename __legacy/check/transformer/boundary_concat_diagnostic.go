package transformer

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func boundaryConcatOperandObligationType() typ.Type {
	return normalize.UnionForEvidence(typ.String, typ.Number)
}

// boundaryConcatOperandParamIndices reads the already-compiled symbolic value
// DAG. A typed consumer of a concat result requires every unresolved leaf to
// satisfy Lua's string-or-number operand contract. Stable locals therefore
// need no scanner/source replay: their assignment has already substituted the
// concat DAG into the call argument term.
func boundaryConcatOperandParamIndices(ctx planCompileContext, term ValueTerm) []int {
	arena := ctx.builder.Arena()
	if arena == nil || term == 0 || int(term) >= len(arena.values) || arena.values[term].op != valueStringConcat {
		return nil
	}
	cursor, ok := boundaryDeclaredValueCursor(ctx)
	if !ok {
		return nil
	}
	want := boundaryConcatOperandObligationType()
	seenTerms := make(map[ValueTerm]bool)
	seenParams := make(map[int]bool)
	var out []int
	var visit func(ValueTerm)
	visit = func(current ValueTerm) {
		if current == 0 || int(current) >= len(arena.values) || seenTerms[current] {
			return
		}
		seenTerms[current] = true
		node := arena.values[current]
		if node.op == valueStringConcat && len(node.args) == 2 {
			visit(node.args[0])
			visit(node.args[1])
			return
		}
		if value, exact := arena.evalValue(current, cursor, SpecializationContext{}); exact {
			if got, typed := typevalue.TypeOf(ctx.registry, value); typed && got != nil &&
				!typ.IsAny(got) && !typ.IsUnknown(got) && !typ.IsNever(got) && subtype.IsSubtype(got, want) {
				return
			}
		}
		switch node.op {
		case valueRoot:
			if node.root.Kind == RootParam {
				index := int(node.root.Index)
				if !seenParams[index] {
					seenParams[index] = true
					out = append(out, index)
				}
			}
		case valueEnvironment:
			symbol, exact := key.ParseSymbolValue(node.slot)
			if !exact {
				return
			}
			index, boundary := ctx.plan.BoundaryParamIndex(symbol)
			if boundary && !seenParams[index] {
				seenParams[index] = true
				out = append(out, index)
			}
		case valueJoin, valueRefinement, valueFalsyAbsentRefinement, valueExpressionRefinement:
			for _, argument := range node.args {
				visit(argument)
			}
			// A failed static projection denotes the projected path, not its whole
			// owner record. Path-obligation production owns that future extension;
			// never invent a root obligation by descending through valueStaticIndex.
		}
	}
	visit(term)
	return out
}

func boundaryConcatParamObligations(ctx planCompileContext, term ValueTerm) []callpayload.CallParamObligation {
	indices := boundaryConcatOperandParamIndices(ctx, term)
	if len(indices) == 0 {
		return nil
	}
	want := boundaryConcatOperandObligationType()
	value := typevalue.WithWitness(ctx.registry, typevalue.FromType(ctx.registry, want), want)
	contracts := ctx.plan.BoundaryParamContracts()
	out := make([]callpayload.CallParamObligation, 0, len(indices))
	for _, index := range indices {
		if index < 0 || index >= len(contracts) {
			continue
		}
		if got, exact := typevalue.TypeOf(ctx.registry, contracts[index]); exact && got != nil &&
			!typ.IsAny(got) && !typ.IsUnknown(got) && !typ.IsNever(got) && subtype.IsSubtype(got, want) {
			continue
		}
		out = append(out, callpayload.CallParamObligation{ParamIndex: index, Value: value})
	}
	return out
}

func boundaryDeclaredValueCursor(ctx planCompileContext) (BindingCursor, bool) {
	if ctx.builder == nil || ctx.registry == nil {
		return BindingCursor{}, false
	}
	shape := ctx.builder.shape
	values := make([]product.Value, shape.InputCount())
	paths := make([]pathdom.Path, shape.InputCount())
	for index := range values {
		values[index] = product.Top()
	}
	contracts := ctx.plan.BoundaryParamContracts()
	for index := uint32(0); index < shape.Params; index++ {
		if int(index) < len(contracts) && product.BelongsToRegistry(ctx.registry, contracts[index]) {
			values[shape.offset(RootParam)+int(index)] = contracts[index]
		}
		paths[shape.offset(RootParam)+int(index)] = pathdom.NewPlaceholder(int(index))
	}
	for _, namespace := range []struct {
		kind    RootKind
		symbols []symbol.ID
	}{{RootCapture, ctx.plan.BoundaryCaptures()}, {RootGlobal, ctx.plan.BoundaryGlobals()}} {
		for index := uint32(0); index < shape.count(namespace.kind); index++ {
			if int(index) < len(namespace.symbols) {
				paths[shape.offset(namespace.kind)+int(index)] = pathdom.NewPath(namespace.symbols[index], "")
			}
		}
	}
	cursor, err := NewBindingCursor(shape, values, paths)
	return cursor, err == nil
}
