package siblings

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// OverlayEntry captures info for a sibling function in return overlay construction.
//
// Used during SCC-based return inference to track which functions are in the
// same scope group and may be called as siblings.
type OverlayEntry struct {
	Symbol cfg.SymbolID
	Func   *ast.FunctionExpr
}

// OverlayConfig holds inputs for return inference overlay construction.
//
// This configuration is used by BuildOverlay to construct a type overlay
// for return inference within an SCC. The overlay provides types for
// sibling functions so that calls between them can be typed during
// fixpoint iteration.
type OverlayConfig struct {
	// ReturnVectors maps symbols to their current inferred return vectors.
	ReturnVectors map[cfg.SymbolID][]typ.Type

	// Siblings are the sibling functions in this scope group.
	Siblings []OverlayEntry

	// CurrentSym is the symbol of the function being analyzed (excluded from overlay).
	CurrentSym cfg.SymbolID

	// Services provides seed type resolution for siblings without return vectors.
	Services OverlayServices
}

// OverlayServices provides seed type resolution for overlay construction.
type OverlayServices interface {
	SeedType(fn *ast.FunctionExpr) typ.Type
}

// OverlayServicesFuncs adapts functions to OverlayServices.
type OverlayServicesFuncs struct {
	SeedTypeFn func(fn *ast.FunctionExpr) typ.Type
}

func (o OverlayServicesFuncs) SeedType(fn *ast.FunctionExpr) typ.Type {
	if o.SeedTypeFn == nil {
		return nil
	}
	return o.SeedTypeFn(fn)
}

// BuildOverlay constructs an overlay map for return inference.
//
// This overlay is used during SCC-based return type inference. It provides
// function types for sibling functions based on their current return vectors.
// The current function (CurrentSym) is excluded from the overlay to avoid
// circular dependence during its own analysis.
//
// For siblings without return vectors yet, placeholder function types are created
// using seed type services to preserve parameter arity. This enables the fixpoint
// to make progress even when not all return types are known.
func BuildOverlay(c OverlayConfig) map[cfg.SymbolID]typ.Type {
	overlay := make(map[cfg.SymbolID]typ.Type)
	siblingFuncs := make(map[cfg.SymbolID]*ast.FunctionExpr, len(c.Siblings))
	for _, sib := range c.Siblings {
		if sib.Symbol != 0 && sib.Func != nil {
			siblingFuncs[sib.Symbol] = sib.Func
		}
	}

	// Add sibling function types with current return vectors.
	for sym, returnTypes := range c.ReturnVectors {
		if sym == c.CurrentSym {
			continue
		}
		if len(returnTypes) > 0 {
			var seedType typ.Type
			if c.Services != nil {
				seedType = c.Services.SeedType(siblingFuncs[sym])
			}
			overlay[sym] = buildFunctionFromSeedAndReturns(seedType, returnTypes)
		}
	}

	// Seed siblings without return vectors with placeholder function types.
	for _, sib := range c.Siblings {
		if sib.Symbol == c.CurrentSym {
			continue
		}
		if _, ok := overlay[sib.Symbol]; ok {
			continue
		}
		if sib.Func == nil || c.Services == nil {
			continue
		}
		if seedType := c.Services.SeedType(sib.Func); seedType != nil {
			overlay[sib.Symbol] = seedType
		}
	}

	return overlay
}

// buildFunctionFromReturns creates a function type with the given return types.
func buildFunctionFromReturns(returnTypes []typ.Type) typ.Type {
	if len(returnTypes) == 0 {
		return nil
	}
	return typ.Func().Returns(returnTypes...).Build()
}

func buildFunctionFromSeedAndReturns(seed typ.Type, returnTypes []typ.Type) typ.Type {
	if len(returnTypes) == 0 {
		return seed
	}
	fn, ok := seed.(*typ.Function)
	if !ok || fn == nil {
		return buildFunctionFromReturns(returnTypes)
	}
	builder := typ.Func()
	for _, tp := range fn.TypeParams {
		builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range fn.Params {
		if p.Optional {
			builder.OptParam(p.Name, p.Type)
		} else {
			builder.Param(p.Name, p.Type)
		}
	}
	if fn.Variadic != nil {
		builder.Variadic(fn.Variadic)
	}
	builder.Returns(returnTypes...)
	if fn.Effects != nil {
		builder.Effects(fn.Effects)
	}
	if fn.Spec != nil {
		builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}
