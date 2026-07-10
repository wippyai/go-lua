package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/staticmemberwitness"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func callerPathEntryState(reg *axis.Registry, ks *keyspace.KeySpace, in state.State) (state.State, bool) {
	if reg == nil {
		return state.State{}, false
	}
	out := state.State{}
	edit := out.EditPathEvidence(reg)
	seen := false
	bottom := product.Bottom(reg)

	if snapshot := in.PathRefinementsSnapshot(ks); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			edit.WritePathKey(ks, pathKey, value)
			seen = true
		}
	}
	if snapshot := in.PathStaticMembersSnapshot(ks); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			edit.WritePathStaticMember(ks, pathKey, value)
			seen = true
		}
	}
	return edit.DoneOn(out), seen
}

func applyCapturedUpvalueEntryState(
	reg *axis.Registry,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	caller state.State,
	entry state.State,
) (state.State, bool) {
	env := closureCaptureSeeder{
		reg:      reg,
		bindings: bindings,
		caller:   caller,
	}
	return env.apply(fn, entry)
}

func applyCapturedClosureEntryState(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	caller state.State,
	entry state.State,
	source captureSeedSource,
) (state.State, bool) {
	env := closureCaptureSeeder{
		reg:      reg,
		ks:       ks,
		bindings: bindings,
		caller:   caller,
		source:   source,
	}
	return env.apply(fn, entry)
}

type captureSeedScope uint8

const (
	captureSeedAtDefinition captureSeedScope = iota
	captureSeedAtContext
	captureSeedAtEscapedDefinition
)

type captureSeedSource struct {
	result *body.Result
	point  cfg.Point
	scope  captureSeedScope
}

func (s captureSeedSource) capturedValue(id symbol.ID) (product.Value, bool) {
	if s.result == nil || id == 0 {
		return product.Value{}, false
	}
	if value, ok := s.result.SymbolValueAtBoundary(s.point, id); ok {
		return value, true
	}
	return s.result.UninitializedLocalDeclarationValueAtBoundary(s.point, id)
}

func (s captureSeedSource) invariantValue(id symbol.ID) (product.Value, bool) {
	if s.result == nil || s.result.Registry() == nil || id == 0 {
		return product.Value{}, false
	}
	reg := s.result.Registry()
	typeValues := s.result.TypeValues()
	if typeValues == nil {
		typeValues = typevalue.NewCache()
	}
	value, ok := s.capturedValue(id)
	if !ok {
		if st, stateOK := s.result.StateAtBoundary(s.point); stateOK {
			slot := statekey.SymbolValue(id)
			if slot != 0 {
				stateValue := st.ReadValue(reg, slot)
				if contextEntryValueUseful(reg, stateValue) {
					value = stateValue
					ok = true
				}
			}
		}
	}
	if s.result.SymbolHasWrite(id) {
		if t, ok := s.result.SymbolDeclaredType(id); ok && t != nil {
			return typeValues.FromTypeWithWitness(reg, t), true
		}
		if s.scope == captureSeedAtContext && ok && captureValueIsAbsent(value) && captureHasFunctionStaticType(s.result, id) {
			return value, true
		}
		return product.Value{}, false
	}
	if ok {
		if shape, ok := captureNonStackStableShape(s.result, s.point, value); ok {
			return typeValues.FromTypeWithWitness(reg, shape), true
		}
	}
	if t, ok := s.result.SymbolStaticType(id); ok && t != nil {
		return typeValues.FromTypeWithWitness(reg, t), true
	}
	if ok {
		if t, ok := s.result.ValueStructuralType(value); ok && t != nil {
			return typeValues.FromTypeWithWitness(reg, t), true
		}
	}
	return product.Value{}, false
}

func (s captureSeedSource) fullGraphInvariantValue(id symbol.ID) (product.Value, bool) {
	if s.scope != captureSeedAtEscapedDefinition || s.result == nil || s.result.Registry() == nil || id == 0 || s.result.SymbolHasWrite(id) {
		return product.Value{}, false
	}
	reg := s.result.Registry()
	value, ok := s.capturedValue(id)
	if !ok || !contextEntryValueUseful(reg, value) {
		return product.Value{}, false
	}
	receiver := pathdom.Path{Root: s.result.SymbolName(id), Symbol: id}
	if s.result.ValueHasStructuralMutationAtBoundary(s.point, value, receiver) {
		return product.Value{}, false
	}
	return value, true
}

func captureValueIsAbsent(value product.Value) bool {
	return presence.Equal(product.PresenceOf(value), presence.Absent())
}

func captureHasFunctionStaticType(result *body.Result, id symbol.ID) bool {
	if result == nil || id == 0 {
		return false
	}
	if result.IsFunctionDefinitionTarget(id) {
		return true
	}
	t, ok := result.SymbolStaticType(id)
	if !ok || t == nil {
		return false
	}
	_, ok = unwrap.Annotated(t).(*typ.Function)
	return ok
}

func captureNonStackStableShape(result *body.Result, point cfg.Point, value product.Value) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	shape, ok := result.StableShapeForValueAtBoundary(point, value)
	if !ok || shape.Shape == nil {
		return nil, false
	}
	st, ok := result.StateAtBoundary(point)
	if !ok {
		return nil, false
	}
	if st.ReadPlacement(shape.Identity) == placement.Stack {
		return nil, false
	}
	return shape.Shape, true
}

type closureCaptureSeeder struct {
	reg      *axis.Registry
	ks       *keyspace.KeySpace
	bindings *bind.Result
	caller   state.State
	source   captureSeedSource

	seenFns              map[*ast.FunctionExpr]struct{}
	targetFuncs          map[symbol.ID]*ast.FunctionExpr
	calledCapturedSymbol map[*ast.FunctionExpr]map[symbol.ID]struct{}
}

type captureFactMode uint8

const (
	captureFullFactGraph captureFactMode = iota
	captureWriteInvariantFacts
	captureEscapedInvariantFacts
)

// CapturePolicy is computed once per captured symbol and drives fact survival.
type CapturePolicy struct {
	mode       captureFactMode
	entryValue product.Value
	heapValue  product.Value
}

type capturePolicyFacts struct {
	structuralWrite         bool
	opaqueCallbackReachable bool
	requireModule           bool
	functionValue           bool
}

func captureFactModeFor(facts capturePolicyFacts) captureFactMode {
	if facts.structuralWrite {
		return captureWriteInvariantFacts
	}
	if facts.opaqueCallbackReachable && !facts.requireModule && !facts.functionValue {
		return captureEscapedInvariantFacts
	}
	return captureFullFactGraph
}

func (s *closureCaptureSeeder) apply(
	fn *ast.FunctionExpr,
	entry state.State,
) (state.State, bool) {
	if s == nil || s.reg == nil || s.bindings == nil || fn == nil {
		return entry, false
	}
	if _, seen := s.seenFns[fn]; seen {
		return entry, false
	}
	if s.seenFns == nil {
		s.seenFns = make(map[*ast.FunctionExpr]struct{})
	}
	s.seenFns[fn] = struct{}{}
	seen := false
	for _, capture := range s.entryCaptures(fn) {
		if capture.Captured == 0 {
			continue
		}
		slot := statekey.SymbolValue(capture.Captured)
		if slot == 0 {
			continue
		}
		policy := s.capturePolicy(capture, slot)
		var captureSeen bool
		entry, captureSeen = s.seedCapture(capture, slot, policy, entry)
		seen = seen || captureSeen
		if capturedFn, ok := s.functionForCapturedSymbol(capture.Captured); ok && s.functionCallsCapturedSymbol(fn, capture.Captured) {
			var capturedSeen bool
			entry, capturedSeen = s.apply(capturedFn, entry)
			seen = seen || capturedSeen
		}
	}
	for _, global := range s.bindings.DirectGlobalReads(fn) {
		if !s.bindings.HasWrite(global) {
			continue
		}
		slot := statekey.SymbolValue(global)
		if slot == 0 {
			continue
		}
		value := s.captureValue(global, slot)
		if !contextEntryValueUseful(s.reg, value) {
			continue
		}
		entry = entry.WriteValue(s.reg, slot, value)
		if updated, ok := seedEntryHeapObjectsForValue(s.reg, s.caller, entry, value); ok {
			entry = updated
		}
		seen = true
	}
	return entry, seen
}

func (s *closureCaptureSeeder) captureValue(sym symbol.ID, slot statekey.Value) product.Value {
	value := s.caller.ReadValue(s.reg, slot)
	if solved, ok := s.source.capturedValue(sym); ok && contextEntryValueUseful(s.reg, solved) {
		return preciseCapturedValue(s.reg, value, solved)
	}
	return value
}

func (s *closureCaptureSeeder) capturePolicy(capture bind.Capture, slot statekey.Value) CapturePolicy {
	fullValue := s.captureValue(capture.Captured, slot)
	policy := CapturePolicy{
		mode:       captureFullFactGraph,
		entryValue: fullValue,
		heapValue:  fullValue,
	}
	facts := capturePolicyFacts{
		structuralWrite: s.captureHasWrite(capture.Captured),
		requireModule:   s.captureIsRequireModule(capture.Captured),
		functionValue:   capturedValueIsFunction(s.reg, fullValue),
	}
	if !facts.structuralWrite &&
		!facts.requireModule &&
		!facts.functionValue &&
		!s.captureHasDirectNonFunctionInvariantMember(capture) &&
		s.captureEscapesMutable(fullValue) {
		if invariantFull, ok := s.fullGraphInvariantCaptureValue(capture.Captured, fullValue); ok {
			policy.entryValue = invariantFull
			policy.heapValue = invariantFull
		} else {
			facts.opaqueCallbackReachable = true
		}
	}
	policy.mode = captureFactModeFor(facts)
	if policy.mode != captureFullFactGraph {
		policy.entryValue = s.invariantCaptureValue(capture.Captured, fullValue)
	}
	return policy
}

func (s *closureCaptureSeeder) fullGraphInvariantCaptureValue(sym symbol.ID, full product.Value) (product.Value, bool) {
	invariant, ok := s.source.fullGraphInvariantValue(sym)
	if !ok || !contextEntryValueUseful(s.reg, invariant) {
		return product.Value{}, false
	}
	invariant = preciseCapturedValue(s.reg, full, invariant)
	if captureFullGraphCanOverlayStaticMemberWitness(s.reg, invariant) {
		invariant = s.withInvariantHeapStaticMemberWitness(full, invariant)
	}
	return invariant, true
}

func captureFullGraphCanOverlayStaticMemberWitness(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return true
	}
	switch unwrap.Annotated(t).(type) {
	case *typ.Array, *typ.Tuple, *typ.Map, *typ.ReadonlyMap:
		return false
	default:
		return true
	}
}

func (s *closureCaptureSeeder) invariantCaptureValue(sym symbol.ID, full product.Value) product.Value {
	if invariant, ok := s.source.invariantValue(sym); ok && contextEntryValueUseful(s.reg, invariant) {
		if id, ok := identityvalue.ExactID(s.reg, full); ok && !identityvalue.HasExact(s.reg, invariant) {
			invariant = identityvalue.WithExact(s.reg, invariant, id)
		}
		return s.withInvariantHeapStaticMemberWitness(full, invariant)
	}
	return product.Top()
}

func (s *closureCaptureSeeder) captureHasWrite(sym symbol.ID) bool {
	return s != nil && s.bindings != nil && sym != 0 && s.bindings.HasWrite(sym)
}

func (s *closureCaptureSeeder) captureEscapesMutable(full product.Value) bool {
	return s != nil && s.source.scope == captureSeedAtEscapedDefinition && capturedValueIsMutable(s.reg, full)
}

func (s *closureCaptureSeeder) captureIsRequireModule(sym symbol.ID) bool {
	if s == nil || s.bindings == nil || sym == 0 {
		return false
	}
	_, ok := moduleidentity.LocalRequireModulePath(s.bindings, sym)
	return ok
}

func capturedValueIsMutable(reg *axis.Registry, value product.Value) bool {
	id, ok := identityvalue.ExactID(reg, value)
	return ok && id.Kind == "lua.table"
}
func capturedValueIsFunction(reg *axis.Registry, value product.Value) bool {
	id, ok := identityvalue.ExactID(reg, value)
	return ok && id.Kind == "lua.function"
}

func preciseCapturedValue(reg *axis.Registry, slot, solved product.Value) product.Value {
	if !contextEntryValueUseful(reg, slot) {
		return solved
	}
	if product.LessOrEq(reg, solved, slot) {
		return solved
	}
	if product.LessOrEq(reg, slot, solved) {
		return slot
	}
	meet := product.Meet(reg, slot, solved)
	if contextEntryValueUseful(reg, meet) {
		return meet
	}
	return slot
}

// seedCapture consumes CapturePolicy without re-deciding capture rules.
func (s *closureCaptureSeeder) seedCapture(capture bind.Capture, slot statekey.Value, policy CapturePolicy, entry state.State) (state.State, bool) {
	if s == nil || s.reg == nil || capture.Captured == 0 || slot == 0 {
		return entry, false
	}
	if policy.mode != captureFullFactGraph {
		entry = s.stripCapturedPathEvidence(capture, entry)
	}
	entry, pathSeen := s.seedCapturedPathEvidence(capture, policy, entry)
	if !contextEntryValueUseful(s.reg, policy.entryValue) {
		return entry, pathSeen
	}
	entry = entry.WriteValue(s.reg, slot, policy.entryValue)
	if policy.mode != captureFullFactGraph {
		if updated, ok := seedInvariantHeapObjectsForValue(s.reg, s.caller, entry, policy.heapValue); ok {
			entry = updated
		}
	} else if updated, ok := seedEntryHeapObjectsForValue(s.reg, s.caller, entry, policy.heapValue); ok {
		entry = updated
	}
	return entry, true
}

func (s *closureCaptureSeeder) seedCapturedPathEvidence(capture bind.Capture, policy CapturePolicy, entry state.State) (state.State, bool) {
	if s == nil || s.reg == nil || s.ks == nil || capture.Captured == 0 {
		return entry, false
	}
	bottom := product.Bottom(s.reg)
	out := entry
	edit := out.EditPathEvidence(s.reg)
	seen := false
	captures := []bind.Capture{capture}
	if policy.mode == captureFullFactGraph {
		if snapshot := s.caller.PathRefinementsSnapshot(s.ks); !snapshot.Top {
			for pathKey, value := range snapshot.Refinements {
				if pathKey == "" || product.Equal(s.reg, value, bottom) {
					continue
				}
				rebased, ok := rebaseCapturedPathKey(pathKey, captures)
				if !ok {
					continue
				}
				edit.WritePathKey(s.ks, rebased, value)
				seen = true
			}
		}
	}
	if snapshot := s.caller.PathStaticMembersSnapshot(s.ks); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			if pathKey == "" || product.Equal(s.reg, value, bottom) {
				continue
			}
			if policy.mode == captureWriteInvariantFacts {
				continue
			}
			if policy.mode == captureEscapedInvariantFacts && !captureInvariantStaticMemberValue(s.reg, value) {
				continue
			}
			if policy.mode == captureEscapedInvariantFacts && !captureEscapedInvariantStaticMemberPath(s.reg, s.ks, pathKey, value) {
				continue
			}
			rebased, ok := rebaseCapturedPathKey(pathKey, captures)
			if !ok {
				continue
			}
			edit.WritePathStaticMember(s.ks, rebased, value)
			seen = true
		}
	}
	return edit.DoneOn(out), seen
}

func captureInvariantStaticMemberValue(reg *axis.Registry, value product.Value) bool {
	ok, _ := captureInvariantStaticMemberValueKind(reg, value)
	return ok
}

func captureNonFunctionInvariantStaticMemberValue(reg *axis.Registry, value product.Value) bool {
	ok, function := captureInvariantStaticMemberValueKind(reg, value)
	return ok && !function
}

func captureInvariantStaticMemberValueKind(reg *axis.Registry, value product.Value) (bool, bool) {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false, false
	}
	switch tt := unwrap.Annotated(t).(type) {
	case *typ.Function:
		return true, true
	case *typ.Literal:
		return tt.Base == kind.String, false
	case *typ.Record:
		return !tt.Open && !tt.HasMapComponent() && tt.Metatable == nil, false
	case *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return true, false
	default:
		return typ.TypeEquals(tt, typ.String), false
	}
}

func captureDirectStaticMemberPath(ks *keyspace.KeySpace, pathKey pathdom.PathKey) bool {
	if ks == nil || pathKey == "" {
		return false
	}
	key, ok := ks.FromStateKey(pathKey)
	if !ok {
		return false
	}
	segments, ok := ks.SegmentsView(key)
	return ok && len(segments) == 1
}

func captureEscapedInvariantStaticMemberPath(reg *axis.Registry, ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) bool {
	if captureDirectStaticMemberPath(ks, pathKey) {
		return true
	}
	return captureNestedScalarInvariantStaticMemberValue(reg, value)
}

func captureNestedScalarInvariantStaticMemberValue(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false
	}
	switch tt := unwrap.Annotated(t).(type) {
	case *typ.Literal:
		return tt.Base == kind.String
	default:
		return typ.TypeEquals(tt, typ.String)
	}
}

type captureInvariantHeapMember struct {
	suffix []segment.Segment
	value  product.Value
}

func (s *closureCaptureSeeder) withInvariantHeapStaticMemberWitness(full, invariant product.Value) product.Value {
	if s == nil || s.reg == nil || s.ks == nil {
		return invariant
	}
	id, ok := identityvalue.ExactID(s.reg, full)
	if !ok {
		return invariant
	}
	members := s.invariantHeapStaticMembers(id)
	if len(members) == 0 {
		return invariant
	}

	builder := staticmemberwitness.NewBuilder()
	for _, member := range members {
		t, ok := typevalue.TypeOf(s.reg, member.value)
		if !ok || t == nil {
			continue
		}
		builder.Add(member.suffix, t)
	}
	witness, ok := builder.Build()
	if !ok || witness == nil {
		return invariant
	}
	if existing, ok := typevalue.TypeOf(s.reg, invariant); ok && existing != nil {
		if merged, mergedOK := typetable.OverlayRecordMembers(existing, witness); mergedOK {
			witness = merged
		} else if typetable.IsLike(existing) {
			witness = typeexpr.Intersection(existing, witness)
		} else {
			return invariant
		}
		if typ.SameNodeOrRecursiveIdentityEqual(existing, witness) {
			return invariant
		}
	}
	return typevalue.WithWitness(s.reg, invariant, witness)
}

func (s *closureCaptureSeeder) invariantHeapStaticMembers(root identity.ID) []captureInvariantHeapMember {
	var out []captureInvariantHeapMember
	s.collectInvariantHeapStaticMembers(root, nil, make(map[identity.ID]struct{}), &out)
	return out
}

func (s *closureCaptureSeeder) collectInvariantHeapStaticMembers(
	id identity.ID,
	prefix []segment.Segment,
	active map[identity.ID]struct{},
	out *[]captureInvariantHeapMember,
) bool {
	if s == nil || s.reg == nil || s.ks == nil || id == (identity.ID{}) || out == nil {
		return false
	}
	if _, ok := active[id]; ok {
		return false
	}
	active[id] = struct{}{}
	defer delete(active, id)

	object := s.caller.ReadHeapTableObject(s.reg, id)
	if heapidentity.ObjectDomain(s.reg).Equal(object, heapidentity.BottomObject(s.reg)) {
		return false
	}
	seen := false
	for key, value := range object.StaticMembers() {
		if product.Equal(s.reg, value, product.Bottom(s.reg)) {
			continue
		}
		segments, ok := s.ks.SuffixSegmentsView(key)
		if !ok || len(segments) == 0 {
			continue
		}
		suffix := appendCaptureInvariantSegments(prefix, segments)
		if captureNestedScalarInvariantStaticMemberValue(s.reg, value) {
			*out = append(*out, captureInvariantHeapMember{
				suffix: suffix,
				value:  value,
			})
			seen = true
			continue
		}
		childID, ok := identityvalue.ExactID(s.reg, value)
		if !ok {
			continue
		}
		if s.collectInvariantHeapStaticMembers(childID, suffix, active, out) {
			seen = true
		}
	}
	return seen
}

func appendCaptureInvariantSegments(prefix, suffix []segment.Segment) []segment.Segment {
	if len(prefix) == 0 {
		return append([]segment.Segment(nil), suffix...)
	}
	out := make([]segment.Segment, 0, len(prefix)+len(suffix))
	out = append(out, prefix...)
	out = append(out, suffix...)
	return out
}

func seedInvariantHeapObjectsForValue(reg *axis.Registry, caller state.State, entry state.State, value product.Value) (state.State, bool) {
	id, ok := identityvalue.ExactID(reg, value)
	if !ok {
		return entry, false
	}
	return seedInvariantHeapObject(reg, caller, entry, id, make(map[identity.ID]bool), make(map[identity.ID]struct{}))
}

func seedInvariantHeapObject(
	reg *axis.Registry,
	caller state.State,
	entry state.State,
	id identity.ID,
	memo map[identity.ID]bool,
	active map[identity.ID]struct{},
) (state.State, bool) {
	if id == (identity.ID{}) {
		return entry, false
	}
	if hasInvariant, ok := memo[id]; ok {
		return entry, hasInvariant
	}
	if _, ok := active[id]; ok {
		return entry, false
	}
	active[id] = struct{}{}
	defer delete(active, id)

	object := caller.ReadHeapTableObject(reg, id)
	if heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
		memo[id] = false
		return entry, false
	}

	out := entry
	var staticMembers map[keyspace.Key]product.Value
	for key, value := range object.StaticMembers() {
		if captureNestedScalarInvariantStaticMemberValue(reg, value) {
			if staticMembers == nil {
				staticMembers = make(map[keyspace.Key]product.Value)
			}
			staticMembers[key] = value
			continue
		}
		childID, ok := identityvalue.ExactID(reg, value)
		if !ok {
			continue
		}
		var copied bool
		out, copied = seedInvariantHeapObject(reg, caller, out, childID, memo, active)
		if copied {
			if staticMembers == nil {
				staticMembers = make(map[keyspace.Key]product.Value)
			}
			staticMembers[key] = value
		}
	}
	if len(staticMembers) == 0 {
		memo[id] = false
		return entry, false
	}
	memo[id] = true
	return out.WriteHeapTableObject(reg, id, heapidentity.NewOwnedStaticTableObject(object.Root(), staticMembers)), true
}

func (s *closureCaptureSeeder) captureHasDirectNonFunctionInvariantMember(capture bind.Capture) bool {
	if s == nil || s.reg == nil || s.ks == nil || capture.Captured == 0 {
		return false
	}
	snapshot := s.caller.PathStaticMembersSnapshot(s.ks)
	if snapshot.Bottom || snapshot.Top {
		return false
	}
	captures := []bind.Capture{capture}
	for pathKey, value := range snapshot.Members {
		if pathKey == "" || !captureDirectStaticMemberPath(s.ks, pathKey) {
			continue
		}
		if !captureNonFunctionInvariantStaticMemberValue(s.reg, value) {
			continue
		}
		if _, ok := rebaseCapturedPathKey(pathKey, captures); ok {
			return true
		}
	}
	return false
}

func (s *closureCaptureSeeder) stripCapturedPathEvidence(capture bind.Capture, entry state.State) state.State {
	if s == nil || s.reg == nil || s.ks == nil || capture.Captured == 0 {
		return entry
	}
	slot := statekey.SymbolValue(capture.Captured)
	if slot != 0 {
		value := entry.ReadValue(s.reg, slot)
		if id, ok := identityvalue.ExactID(s.reg, value); ok {
			entry = entry.WriteHeapTableObject(s.reg, id, heapidentity.BottomObject(s.reg))
		}
	}
	root := pathdom.Path{
		Root:    capture.CapturedName,
		Symbol:  capture.Captured,
		Version: 1,
	}
	if root.Root == "" {
		root.Root = s.bindings.Name(capture.Captured)
	}
	if stripped, ok := entry.InvalidateOnlyPathEvidenceSubtree(s.ks, root.Key()); ok {
		entry = stripped
	}
	stableRoot := pathdom.Path{
		Root:   root.Root,
		Symbol: root.Symbol,
	}
	if stripped, ok := entry.InvalidateOnlyPathEvidenceSubtree(s.ks, stableRoot.Key()); ok {
		entry = stripped
	}
	return entry
}

func (s *closureCaptureSeeder) entryCaptures(fn *ast.FunctionExpr) []bind.Capture {
	if s == nil || s.bindings == nil || fn == nil {
		return nil
	}
	out := append([]bind.Capture(nil), s.bindings.DirectCaptures(fn)...)
	seen := make(map[symbol.ID]struct{}, len(out))
	for _, capture := range out {
		if capture.Captured != 0 {
			seen[capture.Captured] = struct{}{}
		}
	}
	s.bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
		if origin.Func == nil || origin.Func == fn || !functionOriginDescendsFrom(s.bindings, origin.Func, fn) {
			return true
		}
		for _, capture := range s.bindings.DirectCaptures(origin.Func) {
			if capture.Captured == 0 {
				continue
			}
			if owner, ok := s.bindings.DeclaringFunction(capture.Captured); ok && owner == fn {
				continue
			}
			if _, ok := seen[capture.Captured]; ok {
				continue
			}
			seen[capture.Captured] = struct{}{}
			out = append(out, capture)
		}
		return true
	})
	return out
}

func functionEntryCaptureCount(bindings *bind.Result, fn *ast.FunctionExpr) int {
	seeder := &closureCaptureSeeder{bindings: bindings}
	return len(seeder.entryCaptures(fn))
}

func functionOriginDescendsFrom(bindings *bind.Result, fn, ancestor *ast.FunctionExpr) bool {
	if bindings == nil || fn == nil {
		return false
	}
	for {
		parent, ok := bindings.ParentFunction(fn)
		if !ok || parent == nil {
			return ancestor == nil
		}
		if parent == ancestor {
			return true
		}
		fn = parent
	}
}

func (s *closureCaptureSeeder) functionForCapturedSymbol(sym symbol.ID) (*ast.FunctionExpr, bool) {
	if s == nil || s.bindings == nil || sym == 0 {
		return nil, false
	}
	if fn, ok := s.bindings.FunctionBySymbol(sym); ok && fn != nil {
		return fn, true
	}
	if s.targetFuncs == nil {
		s.targetFuncs = make(map[symbol.ID]*ast.FunctionExpr)
		s.bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
			if origin.Func == nil {
				return true
			}
			if origin.Symbol != 0 {
				s.targetFuncs[origin.Symbol] = origin.Func
			}
			if origin.HasTargetSymbol && origin.TargetSymbol != 0 {
				s.targetFuncs[origin.TargetSymbol] = origin.Func
			}
			return true
		})
	}
	fn, ok := s.targetFuncs[sym]
	return fn, ok && fn != nil
}

func (s *closureCaptureSeeder) functionCallsCapturedSymbol(fn *ast.FunctionExpr, sym symbol.ID) bool {
	if s == nil || s.bindings == nil || fn == nil || sym == 0 {
		return false
	}
	if s.calledCapturedSymbol == nil {
		s.calledCapturedSymbol = make(map[*ast.FunctionExpr]map[symbol.ID]struct{})
	}
	called, ok := s.calledCapturedSymbol[fn]
	if !ok {
		called = make(map[symbol.ID]struct{})
		collectDirectFunctionCallees(s.bindings, fn.Stmts, called)
		s.calledCapturedSymbol[fn] = called
	}
	_, ok = called[sym]
	return ok
}

func collectDirectFunctionCallees(bindings *bind.Result, stmts []ast.Stmt, out map[symbol.ID]struct{}) {
	if bindings == nil || out == nil {
		return
	}
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.AssignStmt:
			collectDirectFunctionCalleesInExprs(bindings, stmt.Lhs, out)
			collectDirectFunctionCalleesInExprs(bindings, stmt.Rhs, out)
		case *ast.LocalAssignStmt:
			collectDirectFunctionCalleesInExprs(bindings, stmt.Exprs, out)
		case *ast.FuncCallStmt:
			collectDirectFunctionCalleesInExpr(bindings, stmt.Expr, out)
		case *ast.DoBlockStmt:
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
		case *ast.WhileStmt:
			collectDirectFunctionCalleesInExpr(bindings, stmt.Condition, out)
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
		case *ast.RepeatStmt:
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
			collectDirectFunctionCalleesInExpr(bindings, stmt.Condition, out)
		case *ast.IfStmt:
			collectDirectFunctionCalleesInExpr(bindings, stmt.Condition, out)
			collectDirectFunctionCallees(bindings, stmt.Then, out)
			collectDirectFunctionCallees(bindings, stmt.Else, out)
		case *ast.NumberForStmt:
			collectDirectFunctionCalleesInExpr(bindings, stmt.Init, out)
			collectDirectFunctionCalleesInExpr(bindings, stmt.Limit, out)
			collectDirectFunctionCalleesInExpr(bindings, stmt.Step, out)
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
		case *ast.GenericForStmt:
			collectDirectFunctionCalleesInExprs(bindings, stmt.Exprs, out)
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
		case *ast.ReturnStmt:
			collectDirectFunctionCalleesInExprs(bindings, stmt.Exprs, out)
		}
	}
}

func collectDirectFunctionCalleesInExprs(bindings *bind.Result, exprs []ast.Expr, out map[symbol.ID]struct{}) {
	for _, expr := range exprs {
		collectDirectFunctionCalleesInExpr(bindings, expr, out)
	}
}

func collectDirectFunctionCalleesInExpr(bindings *bind.Result, expr ast.Expr, out map[symbol.ID]struct{}) {
	if bindings == nil || expr == nil || out == nil {
		return
	}
	switch expr := expr.(type) {
	case *ast.IdentExpr:
		return
	case *ast.AttrGetExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Object, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Key, out)
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			collectDirectFunctionCalleesInExpr(bindings, field.Key, out)
			collectDirectFunctionCalleesInExpr(bindings, field.Value, out)
		}
	case *ast.FuncCallExpr:
		if ident, ok := expr.Func.(*ast.IdentExpr); ok {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				out[sym] = struct{}{}
			}
		}
		collectDirectFunctionCalleesInExpr(bindings, expr.Func, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Receiver, out)
		collectDirectFunctionCalleesInExprs(bindings, expr.Args, out)
	case *ast.LogicalOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Lhs, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Rhs, out)
	case *ast.RelationalOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Lhs, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Rhs, out)
	case *ast.StringConcatOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Lhs, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Rhs, out)
	case *ast.ArithmeticOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Lhs, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Rhs, out)
	case *ast.UnaryMinusOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Expr, out)
	case *ast.UnaryNotOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Expr, out)
	case *ast.UnaryLenOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Expr, out)
	case *ast.UnaryBNotOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Expr, out)
	case *ast.FunctionExpr:
		return
	}
}
