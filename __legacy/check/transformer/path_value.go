package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// BoundaryPathBinding identifies the symbolic value and path namespace for a
// lexical root. Owner is the value at the path root. Base is either a
// substitutable call-frame path or this body's sealed environment path. Root
// is non-zero only for the former and retains parameter provenance needed by
// boundary diagnostics.
type BoundaryPathBinding struct {
	Symbol symbol.ID
	Root   Root
	Base   PathTerm
	Owner  ValueTerm
	Point  cfg.Point
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
	if !a.validBoundaryPathBinding(binding) {
		return 0, 0, fmt.Errorf("transformer: boundary path binding is invalid")
	}
	fullPath := a.AppendPath(binding.Base, p.Segments...)
	if len(p.Segments) == 0 {
		return binding.Owner, fullPath, nil
	}
	last := p.Segments[len(p.Segments)-1]
	key, ok := sourcevalue.StaticPathSegmentValue(a.reg, last)
	if !ok {
		return 0, 0, fmt.Errorf("transformer: path has a non-scalar terminal segment")
	}
	tablePath := a.AppendPath(binding.Base, p.Segments[:len(p.Segments)-1]...)
	var value ValueTerm
	if len(p.Segments) == 1 && binding.Root == (Root{}) {
		// The owner is the exact root table value; its optional path contributes
		// flow evidence but cannot be required as a second State spelling. This
		// form is only valid for body-local/environment owners: substitutable
		// boundary roots carry a semantic path across the call frame below.
		value = a.DynamicReadTableValueAt(binding.Point, binding.Owner, tablePath, a.Constant(key))
	} else {
		value = a.DynamicReadValueAt(binding.Point, binding.Owner, tablePath, a.Constant(key))
	}
	if value == 0 || tablePath == 0 || fullPath == 0 {
		return 0, 0, fmt.Errorf("transformer: boundary path term construction failed")
	}
	return value, fullPath, nil
}

// LowerBoundaryRequiredPathValue lowers a named function-boundary carrier path
// whose applicability depends on current flow/path evidence. Unlike the
// ordinary optional environment read, every descendant is a mandatory
// DynamicReadValue: invalidation or a missing visible path must therefore make
// the enclosing guarded transaction fail closed instead of falling back to the
// carrier's declared object type.
//
// carrier is the already-recovered Param/Capture/Global namespace. It is
// separate from binding.Root because structural environments store those
// carriers in environment slots after N4 while retaining their algebraic
// boundary identity at the caller-owned term constructor.
func (a *Arena) LowerBoundaryRequiredPathValue(p pathdom.Path, binding BoundaryPathBinding, carrier Root) (ValueTerm, PathTerm, error) {
	if a == nil || a.reg == nil {
		return 0, 0, fmt.Errorf("transformer: required boundary path lowering requires a registered arena")
	}
	if p.Symbol == 0 || p.Symbol != binding.Symbol || p.Version != 0 || !a.validBoundaryPathBinding(binding) {
		return 0, 0, fmt.Errorf("transformer: required path is not a canonical boundary path")
	}
	switch carrier.Kind {
	case RootParam, RootCapture, RootGlobal:
	default:
		return 0, 0, fmt.Errorf("transformer: required path has no parameter, capture, or global carrier")
	}
	if binding.Root != (Root{}) && binding.Root != carrier {
		return 0, 0, fmt.Errorf("transformer: required path carrier differs from its binding root")
	}
	fullPath := a.AppendPath(binding.Base, p.Segments...)
	if len(p.Segments) == 0 {
		return binding.Owner, fullPath, nil
	}
	last := p.Segments[len(p.Segments)-1]
	key, ok := sourcevalue.StaticPathSegmentValue(a.reg, last)
	if !ok {
		return 0, 0, fmt.Errorf("transformer: required path has a non-scalar terminal segment")
	}
	tablePath := a.AppendPath(binding.Base, p.Segments[:len(p.Segments)-1]...)
	value := a.DynamicReadValueAt(binding.Point, binding.Owner, tablePath, a.Constant(key))
	if value == 0 || tablePath == 0 || fullPath == 0 {
		return 0, 0, fmt.Errorf("transformer: required boundary path term construction failed")
	}
	return value, fullPath, nil
}

// LowerBoundaryDynamicReadValue lowers tablePath[key] when the table path is
// rooted in one lexical boundary binding and key is already an exact symbolic
// value. This is the syntax-free counterpart of factflow.DynamicIndexExpression
// and shares the same specialization resolver as static descendant reads.
func (a *Arena) LowerBoundaryDynamicReadValue(tablePath pathdom.Path, binding BoundaryPathBinding, key ValueTerm) (ValueTerm, PathTerm, error) {
	return a.LowerBoundaryDynamicReadValueAtPath(tablePath, binding, key, 0)
}

// LowerBoundaryDynamicReadValueAtPath retains the optional lexical source path
// of key so registered relational evidence can compose with the read.
func (a *Arena) LowerBoundaryDynamicReadValueAtPath(tablePath pathdom.Path, binding BoundaryPathBinding, key ValueTerm, keyPath PathTerm) (ValueTerm, PathTerm, error) {
	if a == nil || a.reg == nil {
		return 0, 0, fmt.Errorf("transformer: boundary dynamic read lowering requires a registered arena")
	}
	if tablePath.Symbol == 0 || tablePath.Symbol != binding.Symbol || tablePath.Version != 0 {
		return 0, 0, fmt.Errorf("transformer: dynamic table path is not a canonical lexical boundary path")
	}
	if !a.validBoundaryPathBinding(binding) || key == 0 || int(key) >= len(a.values) {
		return 0, 0, fmt.Errorf("transformer: boundary dynamic read binding is invalid")
	}
	pathTerm := a.AppendPath(binding.Base, tablePath.Segments...)
	value := a.DynamicReadValueAtPaths(binding.Point, binding.Owner, pathTerm, key, keyPath)
	if value == 0 || pathTerm == 0 {
		return 0, 0, fmt.Errorf("transformer: boundary dynamic read term construction failed")
	}
	return value, pathTerm, nil
}

func (a *Arena) validBoundaryPathBinding(binding BoundaryPathBinding) bool {
	if a == nil || binding.Symbol == 0 || binding.Owner == 0 || int(binding.Owner) >= len(a.values) ||
		binding.Base == 0 || int(binding.Base) >= len(a.paths) {
		return false
	}
	base := a.paths[binding.Base]
	if len(base.segments) != 0 {
		return false
	}
	if base.environment != 0 {
		return binding.Root == (Root{}) && base.root == (Root{}) && base.environment == binding.Symbol &&
			a.validEnvironmentSlot(statekey.SymbolValue(binding.Symbol))
	}
	if base.root != binding.Root {
		return false
	}
	switch binding.Root.Kind {
	case RootParam, RootCapture, RootGlobal, RootAmbient:
		return true
	default:
		return false
	}
}
