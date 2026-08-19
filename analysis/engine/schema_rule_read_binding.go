// schema_rule_read_binding.go implements the typed and opaque Rule read bindings.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// schemaSelectedReadSelector is the SchemaBinding-owned selector geometry
// consumed by stagedReadRuntime. It re-reads predecessor membership from the
// immutable Schema and carries the exact Rule fence; it never stores a cold
// dependency slice or a copied Rule row.
type schemaSelectedReadSelector struct {
	fence schemaRuleReceiptFence
	read  uint64
}

func (selector *schemaSelectedReadSelector) selectorReadIndex() int {
	if selector == nil || selector.read > uint64(^uint(0)>>1) {
		return -1
	}
	return int(selector.read)
}

func (selector *schemaSelectedReadSelector) selectorDeclaresRead(index int) bool {
	if selector == nil || index < 0 {
		return false
	}
	for dependency := uint64(0); ; dependency++ {
		value, ok := selector.fence.schema.ruleReadDependencyAt(selector.fence.rule, selector.read, dependency)
		if !ok {
			return false
		}
		if value == uint64(index) {
			return true
		}
	}
}

func (selector *schemaSelectedReadSelector) selectorDependencyCount() int {
	if selector == nil || !selector.fence.valid() {
		return 0
	}
	shape, ok := selector.fence.schema.ruleReadShapeAt(selector.fence.rule, selector.read)
	if !ok || shape.DependencyCount > uint64(^uint(0)>>1) {
		return 0
	}
	return int(shape.DependencyCount)
}

type schemaSelectedRuleReadBinding[K ~uint32 | ~uint64, V any, Tag selectionTag] struct {
	origin *schemaRuleReadOrigin
	factor *schemaFactorBindingCell[K, V]
	read   Read[Selection[Tag, OrderedCells[V]]]
	locate func(SelectorContext) bool
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || binding.locate == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.origin.kind != composition.ReadSelect || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || binding.factor.impl == nil || binding.factor.impl.algebra == nil || binding.factor.impl.state != state {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSelect && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == shape.Factor && shape.Normalizer == (composition.Key{}) && shape.DependencyCount == binding.origin.dependencyCount && binding.origin.dependencyCount != 0
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.origin.matches(proof, binding.origin.readOrdinal) || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) {
		return false
	}
	selectedRead := proof.selectedReadAt(binding.origin.readOrdinal)
	if selectedRead == nil || !selectedRead.Valid() {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	target, staged := runtime.(stagedFactor[V])
	if !present || !typed || !staged || factor == nil || factor.implementation == nil || !factor.implementation.binding.valid() || factor.implementation.binding.state != binding.origin.state || factor.implementation.binding.authority != binding.origin.state.authority || factor.implementation.binding.ordinal != binding.origin.factor || factor.implementation.binding.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	row, rowOK := binding.origin.state.schema.ruleReadShapeAt(binding.origin.ruleOrdinal, binding.origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadSelect || row.Input != binding.origin.input || row.Factor != factorKey || surface.Factor != factorKey || surface.Form != equation.SurfaceReadSelect || surface.Semantic != factorKey || surface.Normalizer.Available() || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() {
		return false
	}
	selector := &schemaSelectedReadSelector{fence: selectedRead.fence, read: binding.origin.readOrdinal}
	if !selector.fence.valid() || selector.selectorDependencyCount() != int(binding.origin.dependencyCount) || selector.selectorDependencyCount() == 0 {
		return false
	}
	normalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	locate := binding.locate
	return bound.appendReadRuntime(&stagedReadRuntime[V, OrderedCells[V], Tag]{input: int(binding.origin.input), selector: selector, target: target, locate: locate, normalize: normalize})
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaSelectedRuleReadBinding[K, V, Tag]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

type schemaExactRuleReadBinding[K ~uint32 | ~uint64, V any] struct {
	origin    *schemaRuleReadOrigin
	factor    *schemaFactorBindingCell[K, V]
	read      Read[OrderedCells[V]]
	projector func(any) (uint64, bool)
}

// These heterogeneous variants retain the typed Read while delegating all
// carrier-coordinate work to the exact Factor cell issuer.
type schemaOpaqueExactRuleReadBinding[V any] struct {
	origin    *schemaRuleReadOrigin
	factor    schemaFactorBinding
	read      Read[OrderedCells[V]]
	projector func(any) (uint64, bool)
}

type schemaOpaqueSelectedRuleReadBinding[V any, Tag selectionTag] struct {
	origin *schemaRuleReadOrigin
	factor schemaFactorBinding
	read   Read[Selection[Tag, OrderedCells[V]]]
	locate func(SelectorContext) bool
}

type schemaOpaqueSummaryRuleReadBinding[V, S any] struct {
	origin *schemaRuleReadOrigin
	form   schemaOpaqueSummaryRuleReadForm[V, S]
	read   Read[S]
	admit  any
}

type schemaOpaqueOperandSelectedRuleReadBinding[RV, O any, Tag selectionTag] struct {
	origin        *schemaRuleReadOrigin
	factor        schemaFactorBinding
	read          Read[Selection[Tag, OrderedCells[RV]]]
	locateOperand func(SelectorContext, O) bool
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
	if binding == nil || binding.origin == nil || binding.factor == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || !binding.factor.schemaFactorReadComplete(state, binding.origin) {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadExact && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.DependencyCount == 0
}

func (binding *schemaOpaqueExactRuleReadBinding[V]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.origin == nil || bound == nil || factors == nil {
		return false
	}
	if _, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof }); !ok {
		return false
	}
	return binding.factor.schemaFactorBindExactRead(bound, member, factors, binding.origin)
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
	if binding == nil || binding.origin == nil || binding.form == nil || state == nil || cell == nil ||
		binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal ||
		binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) ||
		binding.read.resolve == nil || !binding.form.schemaSummaryRuleReadComplete(state, binding.origin) {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSummary && shape.Input == binding.origin.input &&
		shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == binding.origin.semantic &&
		shape.Normalizer == binding.origin.semantic && shape.DependencyCount == 0
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.origin == nil || binding.form == nil || bound == nil || factors == nil ||
		!binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) {
		return false
	}
	return binding.form.schemaSummaryRuleReadBind(bound, member, factors, binding.origin)
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

func (binding *schemaOpaqueSummaryRuleReadBinding[V, S]) summarySurfaceAdmit() any {
	if binding == nil {
		return nil
	}
	return binding.admit
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || binding.locate == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || !binding.factor.schemaFactorReadComplete(state, binding.origin) {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSelect && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == shape.Factor && shape.DependencyCount == binding.origin.dependencyCount && binding.origin.dependencyCount != 0
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.origin == nil || bound == nil || factors == nil || binding.locate == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) || !binding.origin.matches(proof, binding.origin.readOrdinal) {
		return false
	}
	selectedRead := proof.selectedReadAt(binding.origin.readOrdinal)
	if selectedRead == nil || !selectedRead.Valid() {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	targetProvider, targetOK := runtime.(stagedTargetProvider[V])
	if !present || !targetOK || targetProvider.stagedFactorTarget() == nil {
		return false
	}
	selector := &schemaSelectedReadSelector{fence: selectedRead.fence, read: binding.origin.readOrdinal}
	if !selector.fence.valid() || selector.selectorDependencyCount() != int(binding.origin.dependencyCount) || selector.selectorDependencyCount() == 0 {
		return false
	}
	return bound.appendReadRuntime(&stagedReadRuntime[V, OrderedCells[V], Tag]{input: int(binding.origin.input), selector: selector, target: targetProvider.stagedFactorTarget(), locate: binding.locate, normalize: func(value OrderedCells[V]) OrderedCells[V] { return value }})
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaOpaqueSelectedRuleReadBinding[V, Tag]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || binding.locateOperand == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || !binding.factor.schemaFactorReadComplete(state, binding.origin) {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSelect && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == shape.Factor && shape.DependencyCount == binding.origin.dependencyCount && binding.origin.dependencyCount != 0
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || binding.origin == nil || bound == nil || factors == nil || binding.locateOperand == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	operandOwner, operandOK := bound.(interface{ ruleOperand() O })
	if !ok || !operandOK {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) || !binding.origin.matches(proof, binding.origin.readOrdinal) {
		return false
	}
	selectedRead := proof.selectedReadAt(binding.origin.readOrdinal)
	if selectedRead == nil || !selectedRead.Valid() {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	targetProvider, targetOK := runtime.(stagedTargetProvider[RV])
	if !present || !targetOK || targetProvider.stagedFactorTarget() == nil || !targetProvider.stagedFactorReceiptMatches(binding.origin) {
		return false
	}
	surface, surfaceOK := member.ReadAt(int(binding.origin.readOrdinal))
	row, rowOK := binding.origin.state.schema.ruleReadShapeAt(binding.origin.ruleOrdinal, binding.origin.readOrdinal)
	factorSemantic := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadSelect || row.Input != binding.origin.input || row.Factor != factorSemantic || row.Semantic != factorSemantic || row.Normalizer.Available() || row.DependencyCount != binding.origin.dependencyCount || surface.Factor != factorSemantic || surface.Form != equation.SurfaceReadSelect || surface.Semantic != factorSemantic || surface.Mode != equation.TargetModeNone || !surface.LocalAvailable() {
		return false
	}
	selector := &schemaSelectedReadSelector{fence: selectedRead.fence, read: binding.origin.readOrdinal}
	if !selector.fence.valid() || selector.selectorDependencyCount() != int(binding.origin.dependencyCount) || selector.selectorDependencyCount() == 0 {
		return false
	}
	operand := operandOwner.ruleOperand()
	locate := func(context SelectorContext) bool { return binding.locateOperand(context, operand) }
	return bound.appendReadRuntime(&stagedReadRuntime[RV, OrderedCells[RV], Tag]{input: int(binding.origin.input), selector: selector, target: targetProvider.stagedFactorTarget(), locate: locate, normalize: func(value OrderedCells[RV]) OrderedCells[RV] { return value }})
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaOpaqueOperandSelectedRuleReadBinding[RV, O, Tag]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

type schemaSummaryRuleReadBinding[K ~uint32 | ~uint64, V, S any] struct {
	origin *schemaRuleReadOrigin
	factor *schemaFactorBindingCell[K, V]
	form   *schemaSummaryReadCell[K, V, S]
	read   Read[S]
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || binding.form == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.origin.kind != composition.ReadSummary || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || binding.factor.impl == nil || binding.factor.impl.algebra == nil || binding.factor.impl.state != state || binding.factor.ordinal != binding.origin.factor || binding.form.schema != state.schema || binding.form.factor != binding.factor || binding.form.form.cell == nil || binding.form.form.cell.schema != state.schema || !summaryReadFormKind(binding.form.form.cell.kind) || !binding.form.schemaFactorFormComplete() {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSummary && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.Semantic == binding.origin.semantic && shape.Normalizer == binding.origin.semantic && shape.DependencyCount == 0
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.origin.matches(proof, binding.origin.readOrdinal) || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || factor.implementation == nil || !factor.implementation.binding.valid() || factor.implementation.binding.state != binding.origin.state || factor.implementation.binding.authority != binding.origin.state.authority || factor.implementation.binding.ordinal != binding.origin.factor || factor.implementation.binding.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	row, rowOK := binding.origin.state.schema.ruleReadShapeAt(binding.origin.ruleOrdinal, binding.origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadSummary || row.Input != binding.origin.input || row.Factor != factorKey || row.Semantic != binding.origin.semantic || row.Normalizer != binding.origin.semantic || row.DependencyCount != 0 || surface.Factor != factorKey || surface.Form != equation.SurfaceReadSummary || surface.Semantic != binding.origin.semantic || surface.Normalizer != binding.origin.semantic || surface.Mode != equation.TargetModeNone {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	unitRow, rowPresent := factor.reads[surface]
	summaryFactor, summaryForm, summaryKeys, summaryDigest, summaryOK := factor.summaryReadAddress(surface, uint64(uint32(binding.form.ordinal)), binding.origin.semantic)
	if !unitOK || !rowPresent || unitRow.kind != carrier.SummaryUnit || !summaryOK {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, S]{input: int(binding.origin.input), binding: factor.binding, unit: unit, summaryFactor: summaryFactor, summaryForm: summaryForm, summaryKeys: summaryKeys, summaryDigest: summaryDigest, summary: true, normalize: binding.form.normalize, equal: binding.form.equal, fingerprint: binding.form.fingerprint})
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) projectLocal(operand any) (uint64, bool) {
	return 0, false
}

func (binding *schemaSummaryRuleReadBinding[K, V, S]) exactAdmitFactor() schemaFactorBinding {
	return nil
}

func (binding *schemaExactRuleReadBinding[K, V]) complete(state *schemaBindingState, cell schemaRuleBindingCell, ordinal uint64) bool {
	if binding == nil || binding.origin == nil || binding.factor == nil || state == nil || cell == nil || binding.origin.state != state || binding.origin.cell != cell || binding.origin.ruleOrdinal != ordinal || binding.read.origin != binding.origin || binding.read.index != int(binding.origin.readOrdinal) || binding.read.resolve == nil || binding.factor.impl == nil || binding.factor.impl.algebra == nil || binding.factor.impl.state != state || binding.factor.ordinal != binding.origin.factor {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(ordinal, binding.origin.readOrdinal)
	return ok && shape.Kind == composition.ReadExact && shape.Input == binding.origin.input && shape.Factor == state.schema.factorSemanticAt(binding.origin.factor) && shape.DependencyCount == 0
}

func (binding *schemaExactRuleReadBinding[K, V]) bind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if binding == nil || bound == nil || member.ReadCount() <= binding.read.index || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !binding.origin.matches(proof, binding.origin.readOrdinal) || !binding.complete(binding.origin.state, binding.origin.cell, binding.origin.ruleOrdinal) {
		return false
	}
	factorKey := binding.origin.state.schema.factorSemanticAt(binding.origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || factor.implementation == nil || !factor.implementation.binding.valid() || factor.implementation.binding.state != binding.origin.state || factor.implementation.binding.authority != binding.origin.state.authority || factor.implementation.binding.ordinal != binding.origin.factor || factor.implementation.binding.algebra != binding.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(binding.read.index)
	row, rowOK := binding.origin.state.schema.ruleReadShapeAt(binding.origin.ruleOrdinal, binding.origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadExact || row.Input != binding.origin.input || row.Factor != factorKey || row.DependencyCount != 0 || surface.Factor != factorKey || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Local == 0 {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	readLocal, localOK := exactReadLocal(factor.implementation.binding, surface)
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
	return bound.appendReadRuntime(&typedReadRuntime[K, V, OrderedCells[V]]{input: int(binding.origin.input), binding: factor.binding, unit: unit, exactFactor: factor.implementation.binding, exactRaw: readLocal, exact: true, normalize: normalize, equal: equal, fingerprint: fingerprint})
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
