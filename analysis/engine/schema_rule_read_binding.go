// schema_rule_read_binding.go implements the typed and opaque Rule read bindings.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// readRowReady checks the owner fence shared by a Read and its binding. The
// row is compiled once at issuance; all later checks consume its copied
// geometry instead of reopening the Schema.
func readRowReady(row *schemaRuleReadRow, readIndex int, state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64, kind composition.ReadKind) bool {
	return row != nil && readIndex >= 0 && row.selectorReadIndex() == readIndex && row.live(state, cell, ordinal) && row.kind == kind
}

type schemaSelectedRuleReadBinding[K ~uint32 | ~uint64, V any, Tag selectionTag] struct {
	row    *schemaRuleReadRow
	factor *schemaFactorBindingCell[K, V]
	read   Read[Selection[Tag, OrderedCells[V]]]
	locate func(SelectorContext) bool
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) readRow() *schemaRuleReadRow {
	if binding == nil {
		return nil
	}
	return binding.row
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	return binding != nil && binding.factor != nil && binding.locate != nil &&
		readRowReady(binding.row, binding.read.index, state, cell, ordinal, composition.ReadSelect) &&
		binding.read.row == binding.row && binding.read.resolve != nil && binding.factor.impl != nil &&
		binding.factor.impl.algebra != nil && binding.factor.state == state &&
		binding.factor.ordinal == binding.row.factorOrdinal && binding.row.factor == binding.row.semantic &&
		!binding.row.normalizer.Available() && len(binding.row.dependencies) != 0
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	state := binding.row.ownerState()
	if !binding.complete(state, binding.row.owner, binding.row.ownerOrdinal) || !binding.row.sealed() {
		return false
	}
	factorKey := binding.row.factor
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	target, staged := runtime.(stagedFactor[V])
	if !present || !typed || !staged || factor == nil || factor.implementation == nil || !factorRowAvailable(factor.implementation.row) ||
		factor.implementation.row != binding.factor || factor.implementation.ordinal != binding.row.factorOrdinal || factor.implementation.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	if !surfaceOK || surface.Factor != factorKey || surface.Form != equation.SurfaceReadSelect || surface.Semantic != factorKey ||
		surface.Normalizer.Available() || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() {
		return false
	}
	return bound.appendReadRuntime(&stagedReadRuntime[V, OrderedCells[V], Tag]{input: int(binding.row.input), selector: binding.row, target: target, locate: binding.locate, normalize: func(value OrderedCells[V]) OrderedCells[V] { return value }})
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

type schemaExactRuleReadBinding[K ~uint32 | ~uint64, V any] struct {
	row       *schemaRuleReadRow
	factor    *schemaFactorBindingCell[K, V]
	read      Read[OrderedCells[V]]
	projector func(any) (uint64, bool)
}

func (binding *schemaExactRuleReadBinding[K, V]) readRow() *schemaRuleReadRow {
	if binding == nil {
		return nil
	}
	return binding.row
}

// These heterogeneous variants retain the typed Read while delegating all
// carrier-coordinate work to the exact Factor cell issuer.
type schemaOpaqueExactRuleReadBinding[V any] struct {
	row       *schemaRuleReadRow
	factor    schemaFactorBinding
	read      Read[OrderedCells[V]]
	projector func(any) (uint64, bool)
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) readRow() *schemaRuleReadRow {
	if binding == nil {
		return nil
	}
	return binding.row
}

type schemaOpaqueSelectedRuleReadBinding[V any, Tag selectionTag] struct {
	row    *schemaRuleReadRow
	factor schemaFactorBinding
	read   Read[Selection[Tag, OrderedCells[V]]]
	locate func(SelectorContext) bool
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) readRow() *schemaRuleReadRow {
	if binding == nil {
		return nil
	}
	return binding.row
}

type schemaOpaqueSummaryRuleReadBinding[V, S any] struct {
	row  *schemaRuleReadRow
	form schemaOpaqueSummaryRuleReadForm[V, S]
	read Read[S]
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) readRow() *schemaRuleReadRow {
	if binding == nil {
		return nil
	}
	return binding.row
}

type schemaOpaqueOperandSelectedRuleReadBinding[RV, O any, Tag selectionTag] struct {
	row           *schemaRuleReadRow
	factor        schemaFactorBinding
	read          Read[Selection[Tag, OrderedCells[RV]]]
	locateOperand func(SelectorContext, O) bool
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) readRow() *schemaRuleReadRow {
	if binding == nil {
		return nil
	}
	return binding.row
}

func (binding *schemaExactRuleReadBinding[K, V]) projectLocalValue(operand any) (uint64, bool) {
	if binding == nil || binding.projector == nil {
		return 0, false
	}
	return binding.projector(operand)
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) projectLocalValue(operand any) (uint64, bool) {
	if binding == nil || binding.projector == nil {
		return 0, false
	}
	return binding.projector(operand)
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	return binding != nil && binding.factor != nil && readRowReady(binding.row, binding.read.index, state, cell, ordinal, composition.ReadExact) &&
		binding.read.row == binding.row && binding.read.resolve != nil && binding.factor.schemaFactorReadComplete(state, binding.row) &&
		binding.row.factor.Available() && binding.row.factorOrdinal < uint64(len(state.factors)) && len(binding.row.dependencies) == 0
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.row == nil || bound == nil || factors == nil {
		return false
	}
	return binding.factor.schemaFactorBindExactRead(bound, member, factors, binding.row)
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) projectLocal(operand any) (uint64, bool) {
	return binding.projectLocalValue(operand)
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) exactAdmitFactor() schemaFactorBinding {
	if binding == nil {
		return nil
	}
	return binding.factor
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	return binding != nil && binding.form != nil && readRowReady(binding.row, binding.read.index, state, cell, ordinal, composition.ReadSummary) &&
		binding.read.row == binding.row && binding.read.resolve != nil && binding.row.summaryForm == binding.form &&
		binding.form.schemaSummaryRuleReadComplete(state, binding.row)
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.row == nil || bound == nil || factors == nil || !binding.row.sealed() ||
		!binding.complete(binding.row.ownerState(), binding.row.owner, binding.row.ownerOrdinal) {
		return false
	}
	return binding.form.schemaSummaryRuleReadBind(bound, member, factors, binding.row)
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	return binding != nil && binding.factor != nil && binding.locate != nil &&
		readRowReady(binding.row, binding.read.index, state, cell, ordinal, composition.ReadSelect) &&
		binding.read.row == binding.row && binding.read.resolve != nil && binding.factor.schemaFactorReadComplete(state, binding.row) &&
		binding.row.factor == binding.row.semantic && !binding.row.normalizer.Available() && len(binding.row.dependencies) != 0
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.row == nil || bound == nil || factors == nil || binding.locate == nil {
		return false
	}
	state := binding.row.ownerState()
	if !binding.complete(state, binding.row.owner, binding.row.ownerOrdinal) || !binding.row.sealed() {
		return false
	}
	factorKey := binding.row.factor
	runtime, present := factors[factorKey]
	targetProvider, targetOK := runtime.(stagedTargetProvider[V])
	if !present || !targetOK || targetProvider.stagedFactorTarget() == nil {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	if !surfaceOK || surface.Factor != factorKey || surface.Form != equation.SurfaceReadSelect || surface.Semantic != factorKey ||
		surface.Normalizer.Available() || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() {
		return false
	}
	return bound.appendReadRuntime(&stagedReadRuntime[V, OrderedCells[V], Tag]{input: int(binding.row.input), selector: binding.row, target: targetProvider.stagedFactorTarget(), locate: binding.locate, normalize: func(value OrderedCells[V]) OrderedCells[V] { return value }})
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	return binding != nil && binding.factor != nil && binding.locateOperand != nil &&
		readRowReady(binding.row, binding.read.index, state, cell, ordinal, composition.ReadSelect) &&
		binding.read.row == binding.row && binding.read.resolve != nil && binding.factor.schemaFactorReadComplete(state, binding.row) &&
		binding.row.factor == binding.row.semantic && !binding.row.normalizer.Available() && len(binding.row.dependencies) != 0
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.row == nil || bound == nil || factors == nil || binding.locateOperand == nil {
		return false
	}
	operandOwner, operandOK := bound.(interface{ ruleOperand() O })
	if !operandOK {
		return false
	}
	state := binding.row.ownerState()
	if !binding.complete(state, binding.row.owner, binding.row.ownerOrdinal) || !binding.row.sealed() {
		return false
	}
	factorKey := binding.row.factor
	runtime, present := factors[factorKey]
	targetProvider, targetOK := runtime.(stagedTargetProvider[RV])
	if !present || !targetOK || targetProvider.stagedFactorTarget() == nil {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	if !surfaceOK || surface.Factor != factorKey || surface.Form != equation.SurfaceReadSelect || surface.Semantic != factorKey ||
		surface.Normalizer.Available() || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() {
		return false
	}
	operand := operandOwner.ruleOperand()
	locate := func(context SelectorContext) bool { return binding.locateOperand(context, operand) }
	return bound.appendReadRuntime(&stagedReadRuntime[RV, OrderedCells[RV], Tag]{input: int(binding.row.input), selector: binding.row, target: targetProvider.stagedFactorTarget(), locate: locate, normalize: func(value OrderedCells[RV]) OrderedCells[RV] { return value }})
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

type schemaSummaryRuleReadBinding[K ~uint32 | ~uint64, V, S any] struct {
	row    *schemaRuleReadRow
	factor *schemaFactorBindingCell[K, V]
	form   *schemaSummaryReadCell[K, V, S]
	read   Read[S]
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) readRow() *schemaRuleReadRow {
	if binding == nil {
		return nil
	}
	return binding.row
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	return binding != nil && binding.factor != nil && binding.form != nil &&
		readRowReady(binding.row, binding.read.index, state, cell, ordinal, composition.ReadSummary) &&
		binding.read.row == binding.row && binding.read.resolve != nil && binding.factor.impl != nil && binding.factor.impl.algebra != nil &&
		binding.factor.state == state && binding.factor.ordinal == binding.row.factorOrdinal && binding.row.summaryForm == binding.form &&
		binding.form.schema == state.schema && binding.row.factor.Available() && binding.row.semantic.Available() &&
		binding.row.factor == binding.row.normalizer && len(binding.row.dependencies) == 0
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	state := binding.row.ownerState()
	if !binding.complete(state, binding.row.owner, binding.row.ownerOrdinal) || !binding.row.sealed() {
		return false
	}
	factorKey := binding.row.factor
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || factor.implementation == nil || !factorRowAvailable(factor.implementation.row) ||
		factor.implementation.row != binding.factor || factor.implementation.ordinal != binding.row.factorOrdinal || factor.implementation.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	if !surfaceOK || surface.Factor != factorKey || surface.Form != equation.SurfaceReadSummary || surface.Semantic != binding.row.semantic ||
		surface.Normalizer != binding.row.normalizer || surface.Mode != equation.TargetModeNone {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	unitRow, rowPresent := factor.reads[surface]
	summaryFactor, summaryForm, summaryKeys, summaryDigest, summaryOK := factor.summaryReadAddress(surface, binding.row)
	if !unitOK || !rowPresent || unitRow.kind != carrier.SummaryUnit || !summaryOK {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, S]{input: int(binding.row.input), binding: factor.binding, unit: unit, summaryFactor: summaryFactor, summaryForm: summaryForm, summaryKeys: summaryKeys, summaryDigest: summaryDigest, summary: true, normalize: binding.form.normalize, equal: binding.form.equal, fingerprint: binding.form.fingerprint})
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

func (binding *schemaExactRuleReadBinding[K, V]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	return binding != nil && binding.factor != nil &&
		readRowReady(binding.row, binding.read.index, state, cell, ordinal, composition.ReadExact) &&
		binding.read.row == binding.row && binding.read.resolve != nil && binding.factor.impl != nil && binding.factor.impl.algebra != nil &&
		binding.factor.state == state && binding.factor.ordinal == binding.row.factorOrdinal && binding.row.factor.Available() &&
		!binding.row.semantic.Available() && !binding.row.normalizer.Available() && len(binding.row.dependencies) == 0
}

func (binding *schemaExactRuleReadBinding[K, V]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	state := binding.row.ownerState()
	if !binding.complete(state, binding.row.owner, binding.row.ownerOrdinal) || !binding.row.sealed() {
		return false
	}
	factorKey := binding.row.factor
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || factor.implementation == nil || !factorRowAvailable(factor.implementation.row) ||
		factor.implementation.row != binding.factor || factor.implementation.ordinal != binding.row.factorOrdinal || factor.implementation.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	if !surfaceOK || surface.Factor != factorKey || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Local == 0 {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	readLocal, localOK := exactReadLocal(factor.implementation.row, surface)
	if !unitOK || !localOK {
		return false
	}
	normalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	equal := func(left, right OrderedCells[V]) bool {
		return equalOrderedCellRecords(left.record, right.record, binding.factor.impl.algebra.Equal)
	}
	fingerprint := func(value OrderedCells[V]) uint64 {
		return fingerprintOrderedCellRecord(value.record, binding.factor.impl.algebra.Fingerprint)
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, OrderedCells[V]]{input: int(binding.row.input), binding: factor.binding, unit: unit, exactFactor: factor.implementation.row, exactRaw: readLocal, exact: true, normalize: normalize, equal: equal, fingerprint: fingerprint})
}

func (binding *schemaExactRuleReadBinding[K, V]) exactAdmitFactor() schemaFactorBinding {
	if binding == nil {
		return nil
	}
	return binding.factor
}

func (binding *schemaExactRuleReadBinding[K, V]) projectLocal(operand any) (uint64, bool) {
	return binding.projectLocalValue(operand)
}
