package factflow

import "github.com/wippyai/go-lua/analysis/symbol"

// SourceContainsFunction reports whether source's canonical expression-source
// DAG contains the exact lexical function identity. Refinements, operations,
// dynamic indexes, and object literals are representation nodes, not new
// function-definition semantics; all consumers therefore traverse them here
// instead of maintaining partial expression-family walkers.
func (f Facts) SourceContainsFunction(source ValueSource, function symbol.ID) bool {
	if function == 0 {
		return false
	}
	stack := []ValueSource{source}
	seen := make(map[ExprRef]struct{})
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !current.HasExpr || current.ExprRef == 0 {
			continue
		}
		ref := current.ExprRef
		if _, visited := seen[ref]; visited {
			continue
		}
		seen[ref] = struct{}{}
		if candidate, ok := f.ExpressionFunction(ref); ok && candidate == function {
			return true
		}
		if refinement, ok := f.ExpressionRefinement(ref); ok {
			stack = append(stack, refinement.Source())
		}
		if operation, ok := f.ExpressionOperation(ref); ok {
			stack = append(stack, operation.Left())
			if operation.Kind() == ExpressionOperationBinary {
				stack = append(stack, operation.Right())
			}
		}
		if dynamic, ok := f.DynamicIndexExpression(ref); ok {
			stack = append(stack, dynamic.KeySource())
			if table, present := dynamic.TableSource(); present {
				stack = append(stack, table)
			}
		}
		if literal, ok := f.ObjectLiteralView(ref); ok {
			literal.ForEachEntry(func(entry ObjectEntryView) bool {
				stack = append(stack, entry.Source())
				return true
			})
			if list, present := literal.ListElementSource(); present {
				stack = append(stack, list)
			}
		}
	}
	return false
}
