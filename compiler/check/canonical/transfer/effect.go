package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

type DynamicWriteMode uint8

const (
	DynamicWriteForeign DynamicWriteMode = iota
	DynamicWriteSelfDerived
)

type MutatorKind uint8

const (
	MutatorAppendElement MutatorKind = iota + 1
	MutatorAppendMapElement
	MutatorContainerElementUnion
)

type referenceWriteMode uint8

const (
	referenceWritePreserve referenceWriteMode = iota
	referenceWriteFromSource
	referenceWriteExplicit
)

type functionRefsWrite struct {
	Mode referenceWriteMode
	Refs flow.FunctionRefs
}

type closureRefsWrite struct {
	Mode referenceWriteMode
	Refs flow.ClosureRefs
}

type RelationEffectKind uint8

const (
	RelationSeedSiblingNil RelationEffectKind = iota + 1
	RelationKillSymbols
	RelationSeedTargetLengthParam
	RelationKillLengthTargets
)

// RelationEffect is the canonical reducer payload for point-local relation facts.
// Relations are must-facts in PointState.Rel, so transfer code seeds and kills
// them through this reducer rather than editing the relation axis directly.
type RelationEffect struct {
	Kind       RelationEffectKind
	ErrSym     cfg.SymbolID
	ValueSyms  []cfg.SymbolID
	Symbols    []cfg.SymbolID
	TargetRoot cfg.SymbolID
	TargetKey  constraint.PathKey
	ParamIndex int
}

// ReturnSlotEffect is the transfer payload for one caller-visible return slot.
// Source records function/closure identity facts under the slot placeholder;
// Value records the product value for non-symbol expressions whose value would
// otherwise be lost before summary projection.
type ReturnSlotEffect struct {
	Index        int
	Source       ast.Expr
	Value        product.AbstractValue
	FunctionRefs flow.FunctionRefs
}

// ReturnEffect is the canonical reducer payload for a normal return boundary.
// It owns the PointState axes consumed by summary projection: return-slot values,
// return-slot function/closure identities, and return-slot relations.
type ReturnEffect struct {
	Relations flow.ReturnRelations
	Slots     []ReturnSlotEffect
}

// ReferenceEffect is the canonical reducer payload for function/closure identity
// facts. Source-mode effects derive identity facts from a source expression and
// clear stale facts at the target. Explicit-mode effects install an already
// rebased reference map. Place targets use exact static paths when available and
// fall back to root-subtree invalidation for dynamic places.
type ReferenceEffect struct {
	Place        Place
	Path         constraint.Path
	Source       ast.Expr
	FunctionRefs functionRefsWrite
	ClosureRefs  closureRefsWrite
}

// PrototypeSelfEffect records the runtime `self` value that flows into methods
// of one prototype table in Lua split-pattern OOP. Both setmetatable publishing
// sites and method-body self writes lower through this reducer.
type PrototypeSelfEffect struct {
	Prototype cfg.SymbolID
	Value     product.AbstractValue
}

func sourceFunctionRefsWrite() functionRefsWrite {
	return functionRefsWrite{Mode: referenceWriteFromSource}
}

func explicitFunctionRefsWrite(refs flow.FunctionRefs) functionRefsWrite {
	return functionRefsWrite{Mode: referenceWriteExplicit, Refs: refs}
}

func sourceClosureRefsWrite() closureRefsWrite {
	return closureRefsWrite{Mode: referenceWriteFromSource}
}

func explicitClosureRefsWrite(refs flow.ClosureRefs) closureRefsWrite {
	return closureRefsWrite{Mode: referenceWriteExplicit, Refs: refs}
}

// CellEffect is the canonical reducer payload for applying captured-cell effects
// produced by a call. Effects update the caller's live cell store when they touch
// cells owned by the current frame, are always composed into the caller-visible
// summary effect, and can also update a stored closure environment when the call
// target is a closure value.
type CellEffect struct {
	Effects     flow.CaptureEffects
	ClosurePath constraint.Path
}

func (t *Transfer) applyCellEffect(out *flow.PointState, effect CellEffect) bool {
	if out == nil || flow.CaptureEffectsDomain.Equal(effect.Effects, flow.CaptureEffectsDomain.Bottom()) {
		return false
	}
	if t.applyClosureCellEffect(out, effect) {
		t.applyCellStoreEffects(out, t.currentCellEffects(out, effect.Effects))
		t.recordCellEffects(out, effect.Effects)
		return true
	}
	t.applyCellStoreEffects(out, effect.Effects)
	t.recordCellEffects(out, effect.Effects)
	return true
}

func (t *Transfer) applyClosureCellEffect(out *flow.PointState, effect CellEffect) bool {
	if effect.ClosurePath.IsEmpty() {
		return false
	}
	key := effect.ClosurePath.Key()
	if _, ok := flow.ClosureRefAt(out.ClosureRefs, key); !ok {
		return false
	}
	out.ClosureRefs = flow.ApplyClosureRefCellEffects(out.ClosureRefs, key, effect.Effects)
	return true
}

func (t *Transfer) currentCellEffects(out *flow.PointState, effects flow.CaptureEffects) flow.CaptureEffects {
	if flow.CaptureEffectsDomain.Equal(effects, flow.CaptureEffectsDomain.Bottom()) {
		return flow.CaptureEffectsDomain.Bottom()
	}
	if effects.IsTop() {
		if len(out.Cells.Entries()) > 0 {
			return effects
		}
		return flow.CaptureEffectsDomain.Bottom()
	}
	var entries []flow.CaptureEffect
	for _, entry := range effects.Entries() {
		if t.symbolStorage.hasCellEffectTarget(out, entry.Symbol) {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return flow.CaptureEffectsDomain.Bottom()
	}
	return flow.CaptureEffectsOf(entries)
}

func (t *Transfer) applyCellStoreEffects(out *flow.PointState, effects flow.CaptureEffects) {
	if flow.CaptureEffectsDomain.Equal(effects, flow.CaptureEffectsDomain.Bottom()) {
		return
	}
	out.Cells = effects.Apply(out.Cells)
}

func (t *Transfer) recordCellEffects(out *flow.PointState, effects flow.CaptureEffects) {
	if flow.CaptureEffectsDomain.Equal(effects, flow.CaptureEffectsDomain.Bottom()) {
		return
	}
	out.CellEffects = out.CellEffects.Then(effects)
}

func (t *Transfer) recordReceiverEffect(out *flow.PointState, slot int, value product.AbstractValue) bool {
	if out == nil || slot < 0 || value.IsZero() {
		return false
	}
	before := out.ReceiverEffects
	out.ReceiverEffects = out.ReceiverEffects.Then(flow.ReceiverMustWrite(slot, value))
	return !flow.ReceiverEffectsDomain.Equal(before, out.ReceiverEffects)
}

func (t *Transfer) applyRelationEffect(out *flow.PointState, effect RelationEffect) bool {
	if out == nil {
		return false
	}
	before := out.Rel
	switch effect.Kind {
	case RelationSeedSiblingNil:
		out.Rel = out.Rel.WithSiblingNil(effect.ErrSym, effect.ValueSyms)
	case RelationKillSymbols:
		out.Rel = out.Rel.KillSymbols(effect.Symbols...)
	case RelationSeedTargetLengthParam:
		out.Rel = out.Rel.WithTargetLengthParam(effect.TargetRoot, effect.TargetKey, effect.ParamIndex)
	case RelationKillLengthTargets:
		out.Rel = out.Rel.KillLengthTargets(effect.Symbols...)
	default:
		return false
	}
	return !flow.PointRelationsDomain.Equal(before, out.Rel)
}

func (t *Transfer) applyReturnEffect(out *flow.PointState, effect ReturnEffect) bool {
	if out == nil {
		return false
	}
	changed := false
	if !flow.ReturnRelationsDomain.Equal(out.ReturnRel, effect.Relations) {
		out.ReturnRel = effect.Relations
		changed = true
	}
	writer := flow.NewPointWriter(out)
	for _, slot := range effect.Slots {
		if slot.Index < 0 {
			continue
		}
		key := ReturnSlotKey(slot.Index)
		changed = writer.DeleteValueKey(key) || changed
		if slot.Source != nil {
			changed = t.applyReferenceEffect(out, ReferenceEffect{
				Path:         constraint.NewPlaceholder(slot.Index),
				Source:       slot.Source,
				FunctionRefs: sourceFunctionRefsWrite(),
				ClosureRefs:  sourceClosureRefsWrite(),
			}) || changed
		}
		if !flow.FunctionRefsDomain.Equal(slot.FunctionRefs, flow.FunctionRefsDomain.Bottom()) {
			changed = t.applyReferenceEffect(out, ReferenceEffect{
				Path:         constraint.NewPlaceholder(slot.Index),
				FunctionRefs: explicitFunctionRefsWrite(slot.FunctionRefs),
			}) || changed
		}
		if slot.Value.IsZero() {
			continue
		}
		changed = writer.WriteValueKey(key, slot.Value, false) || changed
	}
	return changed
}

func (t *Transfer) applyReferenceEffect(out *flow.PointState, effect ReferenceEffect) bool {
	if out == nil {
		return false
	}
	path, exact, ok := effect.referenceTarget()
	if !ok {
		return false
	}
	changed := t.applyFunctionReferenceEffect(out, path, exact, effect.Source, effect.FunctionRefs)
	changed = t.applyClosureReferenceEffect(out, path, exact, effect.Source, effect.ClosureRefs) || changed
	return changed
}

func (effect ReferenceEffect) referenceTarget() (constraint.Path, bool, bool) {
	if !effect.Path.IsEmpty() {
		return effect.Path, true, true
	}
	if effect.Place.Root == 0 {
		return constraint.Path{}, false, false
	}
	if path, ok := effect.Place.StaticPath(); ok && !path.IsEmpty() {
		return path, true, true
	}
	return constraint.NewPath(effect.Place.Root, effect.Place.RootName), false, true
}

func (t *Transfer) applyFunctionReferenceEffect(
	out *flow.PointState,
	path constraint.Path,
	exact bool,
	src ast.Expr,
	write functionRefsWrite,
) bool {
	before := out.FunctionRefs
	switch write.Mode {
	case referenceWritePreserve:
		return false
	case referenceWriteFromSource:
		if !exact {
			out.FunctionRefs = flow.WithoutFunctionRefSubtree(out.FunctionRefs, path.Key())
			return !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs)
		}
		t.recordFunctionRefAt(out, path, src)
	case referenceWriteExplicit:
		out.FunctionRefs = flow.WithoutFunctionRefSubtree(out.FunctionRefs, path.Key())
		out.FunctionRefs = flow.FunctionRefsDomain.Join(out.FunctionRefs, write.Refs)
	default:
		return false
	}
	return !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func (t *Transfer) applyClosureReferenceEffect(
	out *flow.PointState,
	path constraint.Path,
	exact bool,
	src ast.Expr,
	write closureRefsWrite,
) bool {
	before := out.ClosureRefs
	switch write.Mode {
	case referenceWritePreserve:
		return false
	case referenceWriteFromSource:
		if !exact {
			out.ClosureRefs = flow.WithoutClosureRefSubtree(out.ClosureRefs, path.Key())
			return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
		}
		t.recordClosureRefAt(out, path, src)
	case referenceWriteExplicit:
		out.ClosureRefs = flow.WithoutClosureRefSubtree(out.ClosureRefs, path.Key())
		out.ClosureRefs = flow.ClosureRefsDomain.Join(out.ClosureRefs, write.Refs)
	default:
		return false
	}
	return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func (t *Transfer) applyPrototypeSelfEffect(out *flow.PointState, effect PrototypeSelfEffect) bool {
	if out == nil || effect.Prototype == 0 || effect.Value.IsZero() {
		return false
	}
	before := out.PrototypeSelf
	out.PrototypeSelf = out.PrototypeSelf.JoinValue(effect.Prototype, effect.Value)
	return !flow.PrototypeSelfDomain.Equal(before, out.PrototypeSelf)
}

func (t *Transfer) applyPrototypeSelfWriteEffect(out *flow.PointState, effect WriteEffect, updated product.AbstractValue) bool {
	if !effect.RecordProto {
		return false
	}
	return t.recordPrototypeSelfWrite(out, effect.Place.Root, updated)
}

func referenceEffectForWrite(effect WriteEffect) ReferenceEffect {
	return ReferenceEffect{
		Place:        effect.Place,
		Source:       effect.Source,
		FunctionRefs: effect.FunctionRefs,
		ClosureRefs:  effect.ClosureRefs,
	}
}

// WriteEffect is the canonical product-state effect of writing a value to a
// Place. It owns every PointState axis that must move with that write, so callers
// do not hand-maintain parallel Env/Cells/static-member/ref/provenance updates.
type WriteEffect struct {
	Place         Place
	Value         product.AbstractValue
	ClearValue    bool
	JoinExisting  bool
	Source        ast.Expr
	IndexTarget   cfg.AssignTarget
	DynamicMode   DynamicWriteMode
	LengthTarget  cfg.AssignTarget
	LengthBase    flow.ValueKey
	KeyArrayTable constraint.Path

	FunctionRefs functionRefsWrite
	ClosureRefs  closureRefsWrite

	KillRelations bool
	RecordProto   bool
	RecordStatic  bool
}

// MutatorEffect is the canonical product-state effect of mutating the value at a
// Place without replacing the place itself. This covers Lua table mutators such
// as `table.insert(arr, v)` and `table.insert(m[k], v)`: the reducer owns the
// value-domain mutation plus the side axes that are invalidated by that semantic
// mutation.
type MutatorEffect struct {
	Place                 Place
	Kind                  MutatorKind
	Element               product.AbstractValue
	Key                   product.AbstractValue
	LengthKey             constraint.PathKey
	LengthIncrement       int64
	InvalidateKeyPresence bool
}

func (t *Transfer) applyMutatorEffect(out *flow.PointState, effect MutatorEffect) bool {
	if out == nil {
		return false
	}
	changed := false
	if effect.InvalidateKeyPresence && effect.Place.Root != 0 {
		t.invalidateKeyPresenceForPlace(out, effect.Place)
		changed = true
	}
	if effect.LengthKey != "" && effect.LengthIncrement > 0 {
		changed = t.incrementLenBound(out, effect.LengthKey, effect.LengthIncrement) || changed
	}
	if effect.Place.Root == 0 || effect.Element.IsZero() {
		return changed
	}
	updated, _, ok := t.updatePlace(out, effect.Place, func(base product.AbstractValue) (product.AbstractValue, bool) {
		switch effect.Kind {
		case MutatorAppendElement:
			return product.AppendElement(base, effect.Element), true
		case MutatorAppendMapElement:
			if effect.Key.IsZero() {
				return product.AbstractValue{}, false
			}
			return product.AppendMapElement(base, effect.Key, effect.Element), true
		case MutatorContainerElementUnion:
			return product.ContainerElementUnion(base, effect.Element), true
		default:
			return product.AbstractValue{}, false
		}
	})
	if !ok {
		return changed
	}
	t.writeRootContainer(out, effect.Place.Root, updated)
	return true
}

func (t *Transfer) applyWriteEffect(out *flow.PointState, effect WriteEffect) bool {
	if out == nil || effect.Place.Root == 0 {
		return false
	}
	changed := false
	if effect.KillRelations && len(effect.Place.Steps) == 0 {
		changed = t.applyRelationEffect(out, RelationEffect{Kind: RelationKillSymbols, Symbols: []cfg.SymbolID{effect.Place.Root}}) || changed
	} else if len(effect.Place.Steps) > 0 {
		changed = t.applyRelationEffect(out, RelationEffect{Kind: RelationKillLengthTargets, Symbols: []cfg.SymbolID{effect.Place.Root}}) || changed
	}
	if effect.RecordStatic {
		t.invalidateStaticMembersForPlace(out, effect.Place)
		if !effect.ClearValue {
			t.installStaticMemberWriteFactForPlace(out, effect.Place, effect.Value)
		}
	}
	t.invalidateKeyPresenceForPlace(out, effect.Place)
	t.seedKeyArrayForWriteEffect(out, effect)
	if effect.LengthBase != "" {
		t.applyIndexWriteLength(out, effect.LengthTarget, effect.LengthBase)
	}
	if len(effect.Place.Steps) == 0 {
		return t.applyRootWriteEffect(out, effect) || changed
	}
	if effect.ClearValue || effect.Value.IsZero() {
		return t.applyReferenceEffect(out, referenceEffectForWrite(effect)) || changed
	}
	admittedIndexWrite := false
	admittedIndexKey := product.AbstractValue{}
	admittedIndexValue := product.AbstractValue{}
	sealedIndexTarget := false
	if targetPath, ok := indexWriteTargetPath(effect.Place); ok {
		sealedIndexTarget = t.indexWriteTargetSealed(targetPath)
	}
	updated, _, ok := t.assignPlaceValue(out, effect.Place, effect.Value, func(base product.AbstractValue, step PlaceStep, val product.AbstractValue) (product.AbstractValue, bool) {
		if sealedIndexTarget {
			if !product.SealedIndexWriteAdmits(base, step.Key, val) {
				return product.AbstractValue{}, false
			}
			admittedIndexWrite = true
			admittedIndexKey = step.Key
			admittedIndexValue = val
			return product.WriteIndex(base, step.Key, val), true
		}
		if effect.DynamicMode == DynamicWriteSelfDerived {
			admittedIndexWrite = true
			admittedIndexKey = step.Key
			admittedIndexValue = val
			return product.WriteSelfDerivedIndex(base, step.Key, val), true
		}
		if product.IndexWriteAdmits(base, step.Key, val) {
			admittedIndexWrite = true
			admittedIndexKey = step.Key
			admittedIndexValue = val
		}
		return product.WriteIndexForeign(base, step.Key, val), true
	})
	if !ok {
		return changed
	}
	if admittedIndexWrite {
		t.recordIndexWriteAdmission(out, effect, admittedIndexKey, admittedIndexValue)
	}
	t.writeRootContainer(out, effect.Place.Root, updated)
	t.applyPrototypeSelfWriteEffect(out, effect, updated)
	t.applyReferenceEffect(out, referenceEffectForWrite(effect))
	return true
}

func (t *Transfer) applySymbolWriteEffect(
	out *flow.PointState,
	target cfg.AssignTarget,
	val product.AbstractValue,
	clear bool,
	joinExisting bool,
	src ast.Expr,
	keyArrayTable constraint.Path,
	functionRefs functionRefsWrite,
	closureRefs closureRefsWrite,
) bool {
	if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return false
	}
	changed := t.applyWriteEffect(out, WriteEffect{
		Place:         Place{Root: target.Symbol},
		Value:         val,
		ClearValue:    clear,
		JoinExisting:  joinExisting,
		Source:        src,
		KeyArrayTable: keyArrayTable,
		FunctionRefs:  functionRefs,
		ClosureRefs:   closureRefs,
		KillRelations: true,
		RecordProto:   true,
		RecordStatic:  true,
	})
	if t.applySetMetatableInstanceBinding(out, src, target.Symbol) {
		changed = true
	}
	return changed
}

func (t *Transfer) applyRootWriteEffect(out *flow.PointState, effect WriteEffect) bool {
	if effect.ClearValue {
		t.clearPrototypeInstance(out, effect.Place.Root)
		t.symbolStorage.clear(out, effect.Place.Root, effect.JoinExisting, true)
	} else {
		if effect.Value.IsZero() {
			return t.applyReferenceEffect(out, referenceEffectForWrite(effect))
		}
		if _, ok := t.setMetatablePrototypeFromSource(effect.Source); !ok {
			t.clearPrototypeInstance(out, effect.Place.Root)
		}
		t.writeSymbolValue(out, effect.Place.Root, effect.Value, effect.JoinExisting, true)
		t.applyPrototypeSelfWriteEffect(out, effect, effect.Value)
	}
	t.applyReferenceEffect(out, referenceEffectForWrite(effect))
	return true
}

func (t *Transfer) seedKeyArrayForWriteEffect(out *flow.PointState, effect WriteEffect) {
	if out == nil || effect.KeyArrayTable.IsEmpty() || len(effect.Place.Steps) != 0 {
		return
	}
	path, ok := effect.Place.StaticPath()
	if !ok {
		return
	}
	out.KeyPresence = out.KeyPresence.WithKeyArrayPaths(path, effect.KeyArrayTable)
}

func (t *Transfer) invalidateStaticMembersForPlace(out *flow.PointState, place Place) {
	if out == nil {
		return
	}
	if path, ok := place.StaticPath(); ok && path.Symbol != 0 {
		for i := 1; i < len(path.Segments); i++ {
			prefix := constraint.Path{
				Root:     path.Root,
				Symbol:   path.Symbol,
				Version:  path.Version,
				Segments: append([]constraint.Segment(nil), path.Segments[:i]...),
			}
			out.StaticMembers = out.StaticMembers.With(flow.SymbolPathKey(prefix.Symbol, prefix.Segments), product.Domain.Bottom())
		}
		out.StaticMembers = out.StaticMembers.KillSubtree(flow.SymbolPathKey(path.Symbol, path.Segments))
		return
	}
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return
	}
	out.StaticMembers = out.StaticMembers.KillSubtree(flow.SymbolPathKey(path.Symbol, path.Segments))
}

func (t *Transfer) invalidateKeyPresenceForPlace(out *flow.PointState, place Place) {
	if out == nil {
		return
	}
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return
	}
	out.KeyPresence = out.KeyPresence.KillAffectedByWrite(flow.KeyPresencePathKey(path))
	out.ValueOrigins = out.ValueOrigins.KillAffectedByWrite(flow.KeyPresencePathKey(path))
}
