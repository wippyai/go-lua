package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// BoundaryPathBinding identifies the symbolic value and path namespace for a
// lexical boundary root. Owner is the value at the path root. Root is the
// namespace retained by PathTerm so rebasing substitutes the caller's real
// path independently of its value binding.
type BoundaryPathBinding struct {
	Symbol symbol.ID
	Root   Root
	Owner  ValueTerm
}

// LowerBoundaryPathValue lowers one canonical lexical path into the existing
// symbolic dynamic/member-read vocabulary. A root-only path is its owner. A
// descendant is represented as owner[prefix][last], with prefix retained as a
// PathTerm and last encoded by sourcevalue's canonical scalar segment helper.
//
// This routine deliberately does not inspect syntax, names, source lines,
// heap state, or visibility. Those caller-specific concerns remain in the
// DynamicRead resolver at specialization. Missing resolver/evidence therefore
// fails the complete specialization transaction rather than publishing a
// guessed product value.
func (a *Arena) LowerBoundaryPathValue(p pathdom.Path, binding BoundaryPathBinding) (ValueTerm, PathTerm, error) {
	if a == nil || a.reg == nil {
		return 0, 0, fmt.Errorf("transformer: boundary path lowering requires a registered arena")
	}
	if p.Symbol == 0 || p.Symbol != binding.Symbol || p.Version != 0 {
		return 0, 0, fmt.Errorf("transformer: path is not a canonical lexical boundary path")
	}
	if binding.Owner == 0 || int(binding.Owner) >= len(a.values) || binding.Root.Kind == 0 || binding.Root.Kind >= rootKindCount {
		return 0, 0, fmt.Errorf("transformer: boundary path binding is invalid")
	}
	fullPath := a.Path(binding.Root, p.Segments...)
	if len(p.Segments) == 0 {
		return binding.Owner, fullPath, nil
	}
	last := p.Segments[len(p.Segments)-1]
	key, ok := sourcevalue.StaticPathSegmentValue(a.reg, last)
	if !ok {
		return 0, 0, fmt.Errorf("transformer: path has a non-scalar terminal segment")
	}
	tablePath := a.Path(binding.Root, p.Segments[:len(p.Segments)-1]...)
	value := a.DynamicReadValue(binding.Owner, tablePath, a.Constant(key))
	if value == 0 || tablePath == 0 || fullPath == 0 {
		return 0, 0, fmt.Errorf("transformer: boundary path term construction failed")
	}
	return value, fullPath, nil
}

// LowerBoundaryDynamicReadValue lowers tablePath[key] when the table path is
// rooted in one lexical boundary binding and key is already an exact symbolic
// value. This is the syntax-free counterpart of factflow.DynamicIndexExpression
// and shares the same specialization resolver as static descendant reads.
func (a *Arena) LowerBoundaryDynamicReadValue(tablePath pathdom.Path, binding BoundaryPathBinding, key ValueTerm) (ValueTerm, PathTerm, error) {
	if a == nil || a.reg == nil {
		return 0, 0, fmt.Errorf("transformer: boundary dynamic read lowering requires a registered arena")
	}
	if tablePath.Symbol == 0 || tablePath.Symbol != binding.Symbol || tablePath.Version != 0 {
		return 0, 0, fmt.Errorf("transformer: dynamic table path is not a canonical lexical boundary path")
	}
	if binding.Owner == 0 || int(binding.Owner) >= len(a.values) || key == 0 || int(key) >= len(a.values) || binding.Root.Kind == 0 || binding.Root.Kind >= rootKindCount {
		return 0, 0, fmt.Errorf("transformer: boundary dynamic read binding is invalid")
	}
	pathTerm := a.Path(binding.Root, tablePath.Segments...)
	value := a.DynamicReadValue(binding.Owner, pathTerm, key)
	if value == 0 || pathTerm == 0 {
		return 0, 0, fmt.Errorf("transformer: boundary dynamic read term construction failed")
	}
	return value, pathTerm, nil
}
