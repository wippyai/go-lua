package program

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// strictValidatedFrameCandidate is not a general read-footprint theorem. It is
// an identity-fenced candidate for the private strict transaction: symbolic
// specialization may ignore caller-only path facts only after the untouched
// full context is solved, summary-equal, and retained for observations and
// ResultVersion.
type strictValidatedFrameCandidate struct {
	context       summary.SummaryKey
	base          summary.SummaryKey
	stateEntry    summary.EntryKey
	fullEntry     state.State
	entryKeys     *keyspace.KeySpace
	prepared      uint64
	generation    uint64
	plan          *operationplan.Plan
	shape         transformer.Shape
	globalContent transformer.GlobalContentID
}

// matchesFullContext binds the candidate back to the untouched concrete
// context which must be used for validation and retained materialization.  A
// context key alone is not a state digest: refresh keeps call-site identity
// stable while its entry State grows, so the complete semantic entry digest is
// checked independently.
func (c *strictValidatedFrameCandidate) matchesFullContext(reg *axis.Registry, context *keyedFunction, base relationCellIdentity, generation uint64) bool {
	expectedShape := transformer.Shape{}
	if c != nil && c.plan != nil {
		expectedShape = transformer.Shape{
			Params: uint32(len(c.plan.BoundaryParams())), Captures: uint32(len(c.plan.BoundaryCaptures())),
			Globals: uint32(len(c.plan.BoundaryGlobals())),
		}
	}
	if c == nil || reg == nil || context == nil || !context.hasEntryState || context.entryKeys == nil ||
		c.context != context.key || c.base != base.Summary || c.entryKeys != context.entryKeys ||
		c.prepared != base.BodyDigest || base.Prepared == nil || c.plan != base.Prepared.OperationPlan() ||
		c.generation == 0 || c.generation != generation || c.shape != expectedShape {
		return false
	}
	return c.matchesFullEntry(reg, context) && c.stateEntry == semanticEntryKey(reg, context.entryState, context.entryKeys)
}

func (c *strictValidatedFrameCandidate) matchesFullEntry(reg *axis.Registry, context *keyedFunction) bool {
	return c != nil && reg != nil && context != nil && context.entryKeys == c.entryKeys &&
		state.Domain(reg).Equal(context.entryState, c.fullEntry)
}

func strictValidatedFrameContextCertificate(
	reg *axis.Registry,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	plan *operationplan.Plan,
	shape transformer.Shape,
	globalBoundary transformer.GlobalBoundary,
	context keyedFunction,
	base summary.SummaryKey,
	bodyDigest, generation uint64,
) (*relationContextEntryCertificate, *strictValidatedFrameCandidate, bool) {
	if reg == nil || bindings == nil || fn == nil || plan == nil || context.entryKeys == nil || !context.hasEntryState ||
		context.key.Entry == (summary.EntryKey{}) || bodyDigest == 0 || generation == 0 || shape.Captures != 0 ||
		shape.Results != 0 || shape.HeapTemplates != 0 || int(shape.Params) != len(plan.BoundaryParams()) ||
		int(shape.Globals) != len(plan.BoundaryGlobals()) || !stagedStrictAnnotatedParams(bindings, fn, plan.BoundaryParams()) {
		return nil, nil, false
	}
	params := plan.BoundaryParams()
	visible := make(map[symbol.ID]struct{}, len(params)+len(plan.BoundaryGlobals()))
	projected := state.State{}
	carrier := state.State{}
	for _, param := range params {
		if param == 0 {
			return nil, nil, false
		}
		visible[param] = struct{}{}
		value := context.entryState.ReadValue(reg, statekey.SymbolValue(param))
		if !contextEntryValueUseful(reg, value) {
			return nil, nil, false
		}
		projected = projected.WriteValue(reg, statekey.SymbolValue(param), value)
		carrier = carrier.WriteValue(reg, statekey.SymbolValue(param), value)
	}
	for _, global := range plan.BoundaryGlobals() {
		if global == 0 {
			return nil, nil, false
		}
		visible[global] = struct{}{}
	}
	// Direct-callee-only captures are deliberately absent from Shape.Captures:
	// their immutable lexical identity is owned by the independently sealed
	// CallSurface and later rebound to a generation-local relation cell. Keep
	// the concrete value in the validation carrier, but never project it into
	// the symbolic frame. Any other hidden value remains a hard rejection.
	directCallees := strictSealedDirectCalleeCaptures(bindings, fn, plan)
	values := context.entryState.ValuesSnapshot()
	if values.Top {
		return nil, nil, false
	}
	for slot, value := range values.Values {
		sym, symbolSlot := statekey.ParseSymbolValue(slot)
		if !symbolSlot {
			return nil, nil, false
		}
		if _, boundary := visible[sym]; boundary {
			continue
		}
		functionIdentity, sealed := directCallees[sym]
		valueIdentity, exactIdentity := identityvalue.ExactID(reg, value)
		if !sealed || !exactIdentity || valueIdentity != identity.LuaFunction(uint64(functionIdentity)) {
			return nil, nil, false
		}
		carrier = carrier.WriteValue(reg, slot, value)
	}
	valid := true
	context.entryState.ForEachPathRefinement(func(pathKey keyspace.Key, value product.Value) bool {
		carrier = carrier.WriteLocalPathKey(reg, pathKey, value)
		if _, boundary := visible[pathKey.Sym]; boundary {
			paramValue := context.entryState.ReadValue(reg, statekey.SymbolValue(pathKey.Sym))
			if pathKey.Segs != 0 || !product.Equal(reg, value, paramValue) {
				valid = false
				return false
			}
			projected = projected.WriteLocalPathKey(reg, pathKey, value)
			return true
		}
		if !strictDiscardableCallerPathRoot(bindings, fn, context.entryKeys, pathKey) {
			valid = false
			return false
		}
		return true
	})
	if !valid {
		return nil, nil, false
	}
	context.entryState.ForEachPathStaticMember(func(pathKey keyspace.Key, value product.Value) bool {
		carrier = carrier.WriteLocalPathStaticMember(pathKey, value)
		if _, boundary := visible[pathKey.Sym]; boundary || !strictDiscardableCallerPathRoot(bindings, fn, context.entryKeys, pathKey) {
			valid = false
			return false
		}
		return true
	})
	if !valid || !state.Domain(reg).Equal(context.entryState, carrier) {
		return nil, nil, false
	}
	certificate := certifyRelationContextEntry(reg, bindings, fn, params, nil, context.key, base, bodyDigest, generation, projected, context.entryKeys, true)
	if certificate == nil {
		return nil, nil, false
	}
	frame := &strictValidatedFrameCandidate{
		context: context.key, base: base, stateEntry: semanticEntryKey(reg, context.entryState, context.entryKeys), fullEntry: context.entryState.Snapshot(), entryKeys: context.entryKeys,
		prepared: bodyDigest, generation: generation, plan: plan, shape: shape, globalContent: globalBoundary.ContentID(),
	}
	return certificate, frame, true
}

func strictSealedDirectCalleeCaptures(bindings *bind.Result, fn *ast.FunctionExpr, plan *operationplan.Plan) map[symbol.ID]symbol.ID {
	if bindings == nil || fn == nil || plan == nil {
		return nil
	}
	surface, exact := plan.CallSurface()
	if !exact || !surface.Complete() {
		return nil
	}
	captures := make(map[symbol.ID]struct{})
	for _, capture := range bindings.DirectCaptures(fn) {
		if capture.Captured != 0 {
			captures[capture.Captured] = struct{}{}
		}
	}
	for _, boundary := range plan.BoundaryCaptures() {
		delete(captures, boundary)
	}
	sealed := make(map[symbol.ID]symbol.ID)
	for _, site := range surface.Sites() {
		if _, lexical := site.Target.LexicalBody(); !lexical {
			continue
		}
		call, represented := plan.Facts().CallSiteView(site.Point)
		if !represented {
			continue
		}
		callee := call.CalleeSymbol()
		if _, captured := captures[callee]; captured {
			if functionIdentity, stable := bindings.StableLocalFunctionIdentity(callee); stable {
				sealed[callee] = functionIdentity
			}
		}
	}
	return sealed
}

func strictDiscardableCallerPathRoot(bindings *bind.Result, fn *ast.FunctionExpr, keys *keyspace.KeySpace, pathKey keyspace.Key) bool {
	if bindings == nil || fn == nil || keys == nil || pathKey.Sym == 0 || pathKey.Kind == keyspace.KindInvalid {
		return false
	}
	path, ok := keys.StatePath(pathKey)
	if !ok || path.Symbol != pathKey.Sym {
		return false
	}
	if _, ok := bindings.Kind(pathKey.Sym); !ok {
		return false
	}
	declaring, local := bindings.DeclaringFunction(pathKey.Sym)
	return !local || declaring != fn
}
