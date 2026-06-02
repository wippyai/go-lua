// Package siblings provides sibling type construction for nested function analysis.
//
// Sibling types are function types visible to other functions in the same scope group,
// enabling mutual recursion and forward references within a scope block. When multiple
// functions are defined at the same scope level (e.g., in the same do block), they
// form a sibling group that can call each other.
//
// # Problem Statement
//
// Consider this Lua code:
//
//	local function even(n) return n == 0 or odd(n-1) end
//	local function odd(n)  return n ~= 0 and even(n-1) end
//
// Without sibling type propagation, `even` cannot see `odd`'s type (not yet defined),
// and vice versa. This package enables both functions to see each other's types
// during type checking by computing a unified sibling type map for the group.
//
// # Build Algorithm
//
// The Build function constructs sibling types through three steps:
//  1. Seed from previous iteration (monotonic accumulation across fixpoint iterations)
//  2. Merge captured variable types from the parent scope
//  3. Add sibling function types from canonical function facts
//
// The result is a SymbolID -> Type map that can be injected into the type environment
// when analyzing any function in the group.
//
// # Integration with Fixpoint
//
// Sibling types are recomputed on each fixpoint iteration as canonical function
// facts improve.
// The monotonic accumulation (step 1) ensures that types only grow more precise,
// guaranteeing convergence.
package siblings

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// FuncEntry captures the minimum info needed for sibling type construction.
//
// Each entry represents a function defined in the scope group. The entry includes
// the AST node, CFG location, symbol identity, locality flag, and synthesized
// function type. Non-local functions (e.g., module-level definitions) are included
// for captured variable resolution but do not contribute their types to siblings.
type FuncEntry struct {
	Func       *ast.FunctionExpr
	Point      cfg.Point
	Symbol     cfg.SymbolID
	IsLocal    bool
	TargetPath constraint.Path
}

// BuildConfig holds inputs for sibling type construction.
//
// This configuration bundles all dependencies needed by the Build function.
// The services (captured symbols, type lookup, record enrichment) are provided
// by the checker session to avoid tight coupling between packages.
type BuildConfig struct {
	// Funcs are the function entries in this scope group.
	Funcs []FuncEntry

	// GroupHash identifies the scope group.
	GroupHash uint64

	// SiblingTypesPrev are sibling types from the previous iteration (monotonic accumulation).
	SiblingTypesPrev map[cfg.SymbolID]typ.Type

	// FunctionFacts are canonical local function facts for this scope group.
	FunctionFacts api.FunctionFacts

	// Services provides required lookups for sibling construction.
	Services BuildServices
}

// BuildServices provides lookups for sibling construction.
type BuildServices interface {
	CapturedSymbols(fn *ast.FunctionExpr) []cfg.SymbolID
	TypeAtPoint(point cfg.Point, sym cfg.SymbolID) typ.Type
	EnrichRecord(rec *typ.Record, sym cfg.SymbolID) typ.Type
}

// BuildServicesFuncs adapts functions to BuildServices.
type BuildServicesFuncs struct {
	CapturedSymbolsFn func(fn *ast.FunctionExpr) []cfg.SymbolID
	TypeAtPointFn     func(point cfg.Point, sym cfg.SymbolID) typ.Type
	EnrichRecordFn    func(rec *typ.Record, sym cfg.SymbolID) typ.Type
}

func (b BuildServicesFuncs) CapturedSymbols(fn *ast.FunctionExpr) []cfg.SymbolID {
	if b.CapturedSymbolsFn == nil {
		return nil
	}
	return b.CapturedSymbolsFn(fn)
}

func (b BuildServicesFuncs) TypeAtPoint(point cfg.Point, sym cfg.SymbolID) typ.Type {
	if b.TypeAtPointFn == nil {
		return nil
	}
	return b.TypeAtPointFn(point, sym)
}

func (b BuildServicesFuncs) EnrichRecord(rec *typ.Record, sym cfg.SymbolID) typ.Type {
	if b.EnrichRecordFn == nil {
		return nil
	}
	return b.EnrichRecordFn(rec, sym)
}

// Build constructs the sibling types map for a scope group.
//
// The algorithm proceeds in four phases:
//
// Phase 1 - Seed from Previous: Copy types from SiblingTypesPrev to preserve
// types accumulated in prior fixpoint iterations. This ensures monotonicity:
// types only grow more precise, never regress.
//
// Phase 2 - Captured Variables: For each function, find captured variables
// from the parent scope and add their types. This enables nested functions
// to see types of variables defined in enclosing scopes.
//
// Phase 3 - Sibling Functions: Add canonical function types for locally-defined siblings.
//
// The result maps each symbol to its best-known type. Functions in the group
// use this map as an overlay during type checking to resolve sibling references.
func Build(c BuildConfig) map[cfg.SymbolID]typ.Type {
	if len(c.Funcs) == 0 {
		return nil
	}

	result := make(map[cfg.SymbolID]typ.Type, len(c.Funcs)*4)

	// Step 1: Seed from SiblingTypesPrev for monotonic accumulation.
	for sym, ty := range c.SiblingTypesPrev {
		result[sym] = ty
	}

	// Step 2: Merge captured variable types.
	if c.Services != nil {
		for _, entry := range c.Funcs {
			if entry.Func == nil {
				continue
			}
			captured := c.Services.CapturedSymbols(entry.Func)
			for _, sym := range captured {
				if sym == 0 {
					continue
				}
				capturedType := c.Services.TypeAtPoint(entry.Point, sym)
				if capturedType == nil {
					continue
				}
				if typ.IsSoft(capturedType, typ.SoftAnnotationPolicy) {
					continue
				}
				if rec, ok := unwrap.Alias(capturedType).(*typ.Record); ok {
					if enriched := c.Services.EnrichRecord(rec, sym); enriched != nil {
						capturedType = enriched
					}
				}
				prev := result[sym]
				if typ.IsSoft(prev, typ.SoftAnnotationPolicy) && !typ.IsSoft(capturedType, typ.SoftAnnotationPolicy) {
					result[sym] = capturedType
				} else {
					result[sym] = typ.JoinPreferNonSoft(prev, capturedType)
				}
			}
		}
	}

	// Step 3: Add canonical local function types.
	for _, entry := range c.Funcs {
		if !entry.IsLocal || entry.Symbol == 0 {
			continue
		}
		fnType := siblingFunctionType(c, entry)
		if fnType == nil {
			continue
		}
		result[entry.Symbol] = functionfact.MergeType(result[entry.Symbol], fnType)
	}

	applyFieldFunctionSurface(result, c)

	if len(result) == 0 {
		return nil
	}
	return result
}

// ReceiverSelfType composes the receiver object observed by parent flow with
// the sibling surface owned by the same scope group. Method bodies should get
// their self contract from this product, not from ad hoc AST enrichment.
func ReceiverSelfType(base, surface typ.Type) typ.Type {
	if surface == nil {
		return base
	}
	if base == nil || typ.IsAbsentOrUnknown(base) {
		return surface
	}
	if fields := receiverSurfaceFields(surface); len(fields) > 0 {
		return overlaymut.MergeRequiredFieldsIntoType(base, fields)
	}
	if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(surface, base); ok && reconciled != nil {
		return reconciled
	}
	return value.JoinPrecise(base, surface)
}

func receiverSurfaceFields(surface typ.Type) interprocdomain.FieldValues {
	rec := unwrap.Record(surface)
	if rec == nil || len(rec.Fields) == 0 {
		return nil
	}
	fields := make(interprocdomain.FieldValues, len(rec.Fields))
	for _, field := range rec.Fields {
		if field.Name == "" || field.Type == nil {
			continue
		}
		// The sibling surface injects resolved method signatures into the
		// receiver as required fields so mutual references within the scope
		// group resolve. Data fields belong to the value-tracked base and keep
		// their declared shape there; merging them as required would drop a
		// base field's optionality (e.g. an optional data field `f?: string`
		// would become a required non-nil `f: string`).
		if unwrap.Function(field.Type) == nil {
			continue
		}
		fieldKey, ok := interprocdomain.FieldKeyFromName(field.Name)
		if !ok {
			continue
		}
		fields[fieldKey] = product.FromType(field.Type)
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func siblingFunctionType(c BuildConfig, entry FuncEntry) typ.Type {
	if entry.Symbol == 0 {
		return nil
	}
	if fnType := functionfact.SiblingTypeProjection(c.FunctionFacts, entry.Symbol, api.PhaseScopeCompute); fnType != nil {
		return fnType
	}
	if c.Services == nil {
		return nil
	}
	return c.Services.TypeAtPoint(entry.Point, entry.Symbol)
}

func applyFieldFunctionSurface(result map[cfg.SymbolID]typ.Type, c BuildConfig) {
	if len(c.Funcs) == 0 {
		return
	}
	fields := make(overlaymut.FieldAssignments)
	points := make(map[cfg.SymbolID]cfg.Point)
	for _, entry := range c.Funcs {
		baseSym, fieldName := directFieldTarget(entry.TargetPath)
		if baseSym == 0 || fieldName == "" {
			continue
		}
		fnType := siblingFunctionType(c, entry)
		if fnType == nil {
			continue
		}
		fieldKey, ok := interprocdomain.FieldKeyFromName(fieldName)
		if !ok {
			continue
		}
		if fields[baseSym] == nil {
			fields[baseSym] = make(interprocdomain.FieldValues)
		}
		if existing := fields[baseSym][fieldKey]; !existing.IsZero() {
			fields[baseSym][fieldKey] = product.FromType(functionfact.MergeType(existing.ProjectValue(), fnType))
		} else {
			fields[baseSym][fieldKey] = product.FromType(fnType)
		}
		if points[baseSym] == 0 || entry.Point < points[baseSym] {
			points[baseSym] = entry.Point
		}
	}
	if len(fields) == 0 {
		return
	}
	for _, sym := range cfg.SortedSymbolIDs(fields) {
		if result[sym] == nil && c.Services != nil {
			result[sym] = c.Services.TypeAtPoint(points[sym], sym)
		}
		result[sym] = overlaymut.MergeRequiredFieldsIntoType(result[sym], fields[sym])
	}
}

func directFieldTarget(path constraint.Path) (cfg.SymbolID, string) {
	if path.Symbol == 0 || len(path.Segments) != 1 {
		return 0, ""
	}
	seg := path.Segments[0]
	if (seg.Kind != constraint.SegmentField && seg.Kind != constraint.SegmentIndexString) || seg.Name == "" {
		return 0, ""
	}
	return path.Symbol, seg.Name
}

// Compute extracts sibling types for a function's scope group from the store.
//
// The store is keyed by group hash (computed from the parent scope). This
// function looks up the sibling types for the given group and returns them.
// Returns nil if no sibling types exist for the group.
func Compute(store map[uint64]map[cfg.SymbolID]typ.Type, groupHash uint64) map[cfg.SymbolID]typ.Type {
	if store == nil {
		return nil
	}
	if siblings := store[groupHash]; len(siblings) > 0 {
		return siblings
	}
	return nil
}

// Copy returns a shallow copy of a sibling types map.
func Copy(m map[cfg.SymbolID]typ.Type) map[cfg.SymbolID]typ.Type {
	if m == nil {
		return nil
	}
	cp := make(map[cfg.SymbolID]typ.Type, len(m))
	for sym, t := range m {
		cp[sym] = t
	}
	return cp
}
