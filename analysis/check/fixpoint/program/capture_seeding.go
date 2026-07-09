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
	readCaptured captureValueReader,
	readInvariant captureValueReader,
) (state.State, bool) {
	env := closureCaptureSeeder{
		reg:           reg,
		ks:            ks,
		bindings:      bindings,
		caller:        caller,
		readCaptured:  readCaptured,
		readInvariant: readInvariant,
	}
	return env.apply(fn, entry)
}

type captureValueReader func(symbol.ID) (product.Value, bool)

func applyEscapedCapturedClosureEntryState(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	caller state.State,
	entry state.State,
	readCaptured captureValueReader,
	readInvariant captureValueReader,
	readFullInvariant captureValueReader,
) (state.State, bool) {
	env := closureCaptureSeeder{
		reg:               reg,
		ks:                ks,
		bindings:          bindings,
		caller:            caller,
		readCaptured:      readCaptured,
		readInvariant:     readInvariant,
		readFullInvariant: readFullInvariant,
		escapeUnknown:     true,
	}
	return env.apply(fn, entry)
}

func captureValueReaderAt(result *body.Result, point cfg.Point) captureValueReader {
	if result == nil {
		return nil
	}
	return func(id symbol.ID) (product.Value, bool) {
		if id == 0 {
			return product.Value{}, false
		}
		if value, ok := result.SymbolValueAtBoundary(point, id); ok {
			return value, true
		}
		return result.UninitializedLocalDeclarationValueAtBoundary(point, id)
	}
}

func captureInvariantValueReaderAt(result *body.Result, point cfg.Point) captureValueReader {
	return captureInvariantValueReaderAtWithOptions(result, point, false)
}

func contextualCaptureInvariantValueReaderAt(result *body.Result, point cfg.Point) captureValueReader {
	return captureInvariantValueReaderAtWithOptions(result, point, true)
}

func captureFullGraphInvariantValueReaderAt(result *body.Result, point cfg.Point) captureValueReader {
	if result == nil || result.Registry() == nil {
		return nil
	}
	reg := result.Registry()
	return func(id symbol.ID) (product.Value, bool) {
		if id == 0 || result.SymbolHasWrite(id) {
			return product.Value{}, false
		}
		value, ok := result.SymbolValueAtBoundary(point, id)
		if !ok {
			value, ok = result.UninitializedLocalDeclarationValueAtBoundary(point, id)
		}
		if !ok {
			return product.Value{}, false
		}
		if !contextEntryValueUseful(reg, value) {
			return product.Value{}, false
		}
		receiver := pathdom.Path{
			Root:   result.SymbolName(id),
			Symbol: id,
		}
		if result.ValueHasStructuralMutationAtBoundary(point, value, receiver) {
			return product.Value{}, false
		}
		return value, true
	}
}

func captureInvariantValueReaderAtWithOptions(result *body.Result, point cfg.Point, allowPointAbsentFunctionTarget bool) captureValueReader {
	if result == nil || result.Registry() == nil {
		return nil
	}
	reg := result.Registry()
	typeValues := result.TypeValues()
	if typeValues == nil {
		typeValues = typevalue.NewCache()
	}
	return func(id symbol.ID) (product.Value, bool) {
		if id == 0 {
			return product.Value{}, false
		}
		value, ok := result.SymbolValueAtBoundary(point, id)
		if !ok {
			value, ok = result.UninitializedLocalDeclarationValueAtBoundary(point, id)
		}
		if !ok {
			if st, stateOK := result.StateAtBoundary(point); stateOK {
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
		hasWrite := result.SymbolHasWrite(id)
		if hasWrite {
			if t, ok := result.SymbolDeclaredType(id); ok && t != nil {
				return typeValues.FromTypeWithWitness(reg, t), true
			}
			if allowPointAbsentFunctionTarget && ok && captureValueIsAbsent(value) && captureHasFunctionStaticType(result, id) {
				return value, true
			}
			return product.Value{}, false
		}
		if ok {
			if shape, ok := captureNonStackStableShape(result, point, value); ok {
				return typeValues.FromTypeWithWitness(reg, shape), true
			}
		}
		if t, ok := result.SymbolStaticType(id); ok && t != nil {
			return typeValues.FromTypeWithWitness(reg, t), true
		}
		if ok {
			if t, ok := result.ValueStructuralType(value); ok && t != nil {
				return typeValues.FromTypeWithWitness(reg, t), true
			}
		}
		return product.Value{}, false
	}
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
	reg               *axis.Registry
	ks                *keyspace.KeySpace
	bindings          *bind.Result
	caller            state.State
	readCaptured      captureValueReader
	readInvariant     captureValueReader
	readFullInvariant captureValueReader
	escapeUnknown     bool

	seenFns              map[*ast.FunctionExpr]struct{}
	targetFuncs          map[symbol.ID]*ast.FunctionExpr
	calledCapturedSymbol map[*ast.FunctionExpr]map[symbol.ID]struct{}
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
		fullValue := s.fullCapturedValue(capture.Captured, slot)
		written := s.captureHasWrite(capture.Captured)
		escapedMutable := !written &&
			!s.captureIsRequireModule(capture.Captured) &&
			!s.captureHasDirectNonFunctionInvariantMember(capture) &&
			s.captureEscapesMutable(fullValue)
		if escapedMutable {
			if invariantFull, ok := s.fullGraphInvariantCapturedValue(capture.Captured, fullValue); ok {
				fullValue = invariantFull
				escapedMutable = false
			}
		}
		degrade := written || escapedMutable
		if degrade {
			entry = s.stripCapturedPathEvidence(capture, entry)
		}
		var pathSeen bool
		entry, pathSeen = s.seedCapturedPathEvidence(capture, entry, degrade, escapedMutable)
		seen = seen || pathSeen
		value := s.capturedEntryValue(capture.Captured, fullValue, degrade)
		if contextEntryValueUseful(s.reg, value) {
			entry = entry.WriteValue(s.reg, slot, value)
			if degrade {
				if updated, ok := seedInvariantHeapObjectsForValue(s.reg, s.caller, entry, fullValue); ok {
					entry = updated
				}
			} else {
				if updated, ok := seedEntryHeapObjectsForValue(s.reg, s.caller, entry, value); ok {
					entry = updated
				}
			}
			seen = true
		}
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
		value := s.fullCapturedValue(global, slot)
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

func (s *closureCaptureSeeder) fullCapturedValue(sym symbol.ID, slot statekey.Value) product.Value {
	value := s.caller.ReadValue(s.reg, slot)
	if s.readCaptured != nil {
		if solved, ok := s.readCaptured(sym); ok && contextEntryValueUseful(s.reg, solved) {
			return preciseCapturedValue(s.reg, value, solved)
		}
	}
	return value
}

func (s *closureCaptureSeeder) fullGraphInvariantCapturedValue(sym symbol.ID, full product.Value) (product.Value, bool) {
	if s == nil || s.readFullInvariant == nil {
		return product.Value{}, false
	}
	invariant, ok := s.readFullInvariant(sym)
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

func (s *closureCaptureSeeder) capturedEntryValue(sym symbol.ID, full product.Value, degrade bool) product.Value {
	if !degrade {
		return full
	}
	if s.readInvariant != nil {
		if invariant, ok := s.readInvariant(sym); ok && contextEntryValueUseful(s.reg, invariant) {
			if id, ok := identityvalue.ExactID(s.reg, full); ok && !identityvalue.HasExact(s.reg, invariant) {
				invariant = identityvalue.WithExact(s.reg, invariant, id)
			}
			invariant = s.withInvariantHeapStaticMemberWitness(full, invariant)
			return invariant
		}
	}
	return product.Top()
}

func (s *closureCaptureSeeder) captureHasWrite(sym symbol.ID) bool {
	return s != nil && s.bindings != nil && sym != 0 && s.bindings.HasWrite(sym)
}

func (s *closureCaptureSeeder) captureEscapesMutable(full product.Value) bool {
	return s != nil && s.escapeUnknown && capturedValueIsMutable(s.reg, full)
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

func (s *closureCaptureSeeder) seedCapturedPathEvidence(capture bind.Capture, entry state.State, degrade bool, allowInvariantStaticMembers bool) (state.State, bool) {
	if s == nil || s.reg == nil || s.ks == nil || capture.Captured == 0 || (degrade && !allowInvariantStaticMembers) {
		return entry, false
	}
	bottom := product.Bottom(s.reg)
	out := entry
	edit := out.EditPathEvidence(s.reg)
	seen := false
	captures := []bind.Capture{capture}
	if !degrade {
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
			if degrade && !captureInvariantStaticMemberValue(s.reg, value) {
				continue
			}
			if degrade && !captureEscapedInvariantStaticMemberPath(s.reg, s.ks, pathKey, value) {
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
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false
	}
	switch tt := unwrap.Annotated(t).(type) {
	case *typ.Function:
		return true
	case *typ.Literal:
		return tt.Base == kind.String
	case *typ.Record:
		return !tt.Open && !tt.HasMapComponent() && tt.Metatable == nil
	case *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return true
	default:
		return typ.TypeEquals(tt, typ.String)
	}
}

func captureNonFunctionInvariantStaticMemberValue(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false
	}
	switch tt := unwrap.Annotated(t).(type) {
	case *typ.Literal:
		return tt.Base == kind.String
	case *typ.Record:
		return !tt.Open && !tt.HasMapComponent() && tt.Metatable == nil
	case *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return true
	default:
		return typ.TypeEquals(tt, typ.String)
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
	staticMembers := make(map[keyspace.Key]product.Value)
	for key, value := range object.StaticMembers() {
		if captureNestedScalarInvariantStaticMemberValue(reg, value) {
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
