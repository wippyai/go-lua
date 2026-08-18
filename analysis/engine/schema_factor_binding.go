// schema_factor_binding.go holds the Factor, form and Rule binding cells.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// HotFactorSpec is the Link-local portion of FactorSpec. Semantic is absent:
// the exact FactorSlot already carries the cold proof.
type HotFactorSpec[K ~uint32 | ~uint64, V any] struct {
	KeyEnd      uint64
	Lattice     lattice.Lattice[V]
	Default     V
	AdmitAt     func(K, V) bool
	Fingerprint func(V) uint64
	WidenRank   Measure[K, V]
	NarrowRank  Measure[K, V]
}

func factorRefOrdinal[V any](ref FactorRef[V], schema *Schema) (uint64, bool) {
	return anyFactorRefOrdinal(ref.Any(), schema)
}

func anyFactorRefOrdinal(ref AnyFactorRef, schema *Schema) (uint64, bool) {
	if schema == nil || ref.cell == nil || ref.cell.schema != schema || !schema.Available() {
		return 0, false
	}
	return ref.cell.ordinal, true
}

// RuleImplementation is the typed public handle for the one currently
// supported receipt lane. It intentionally contains only an opaque receipt;
// callbacks and structural draft handles remain cell-owned.
type RuleImplementation[K ~uint32 | ~uint64, V, O any] struct {
	receipt ruleRuntimeReceipt[K, V, O]
}

type schemaRuleBindingCell interface {
	schemaBindingCell
	schemaRuleOrdinal() uint64
	schemaRuleComplete() bool
	schemaRuleProofMatches(*ruleRuntimeProof) bool
}

func schemaFactorCount(schema *Schema) int {
	if schema == nil {
		return 0
	}
	factors, _, _, _, ok := schema.shapeCount()
	if !ok {
		return 0
	}
	return factors
}

type schemaFactorBinding interface {
	boundTopologyFactorReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool)
	schemaFactorOrdinal() uint64
	schemaFactorSchema() *Schema
	schemaFactorComplete() bool
	schemaFactorRuntimeBinding(*runtimeBinding) (runtimeFactor, bool)
	schemaFactorReadComplete(*schemaBindingState, *schemaRuleReadOrigin) bool
	schemaFactorBindExactRead(readBinding, equation.RuleMember, map[composition.Key]runtimeFactor, *schemaRuleReadOrigin) bool
	schemaFactorFormAt(uint64) schemaFactorFormBinding
	schemaFactorAdmitExactRead(*schemaBindingState, *schemaBindingAuthority, *RuleSourceTransaction, uint64) bool
	schemaFactorAdmitExactWrite(*schemaBindingState, *schemaBindingAuthority, *RuleSourceTransaction, uint64) bool
}

type FactorImplementation[K ~uint32 | ~uint64, V any] struct {
	state      *schemaBindingState
	algebra    *factbinding.Algebra[K, V]
	descriptor factorRuntimeDescriptor
	receipt    factorRuntimeReceipt
}

func schemaFormKind(kind composition.FactorFormKind) SchemaFormKind {
	switch kind {
	case composition.FactorSummaryRead:
		return SchemaFormReadSummary
	case composition.FactorDistributiveSummaryRead:
		return SchemaFormReadDistributiveSummary
	default:
		return SchemaFormInvalid
	}
}

type anyFactorAlgebra interface {
	KeyEnd() uint64
}

type schemaFactorFormBinding interface {
	schemaBindingSchema() *Schema
	schemaFactorFormComplete() bool
}

type schemaFactorBindingCell[K ~uint32 | ~uint64, V any] struct {
	ordinal    uint64
	schema     *Schema
	impl       *FactorImplementation[K, V]
	exactRead  schemaFactorFormBinding
	exactWrite schemaFactorFormBinding
	forms      []schemaFactorFormBinding
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorAlgebra() anyFactorAlgebra {
	if cell == nil || cell.impl == nil {
		return nil
	}
	return cell.impl.algebra
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorBindingState() *schemaBindingState {
	if cell == nil || cell.impl == nil || cell.impl.state == nil {
		return nil
	}
	return cell.impl.state
}

func (cell *schemaFactorBindingCell[K, V]) boundTopologyFactorReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, bool) {
	if cell == nil || cell.impl == nil || cell.impl.state == nil {
		return nil, nil, composition.Key{}, false
	}
	state := cell.impl.state
	authority := state.authority
	if authority == nil || state.phase != schemaBindingSealed || state.schema != cell.schema || cell.ordinal >= uint64(len(state.factors)) || state.factors[cell.ordinal] != cell || cell.impl.algebra == nil {
		return nil, nil, composition.Key{}, false
	}
	semantic := state.schema.factorSemanticAt(cell.ordinal)
	if !semantic.Available() {
		return nil, nil, composition.Key{}, false
	}
	return state, authority, semantic, true
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorRuntimeBinding(runtime *runtimeBinding) (runtimeFactor, bool) {
	if cell == nil || cell.impl == nil || runtime == nil || runtime.mode != runtimeBindingReceipt || runtime.state == nil || runtime.authority == nil {
		return nil, false
	}
	implementation, ok := cell.sealedImplementation(runtime.state, runtime.authority)
	if !ok {
		return nil, false
	}
	return bindFactorFromGraph(implementation, runtime)
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorReadComplete(state *schemaBindingState, origin *schemaRuleReadOrigin) bool {
	return cell != nil && state != nil && origin != nil && cell.schema == state.schema && cell.impl != nil && cell.impl.state == state && cell.impl.algebra != nil && cell.ordinal == origin.factor
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorFormAt(index uint64) schemaFactorFormBinding {
	if cell == nil || index >= uint64(len(cell.forms)) {
		return nil
	}
	return cell.forms[index]
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorBindExactRead(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor, origin *schemaRuleReadOrigin) bool {
	if cell == nil || bound == nil || member.ReadCount() <= int(origin.readOrdinal) || !cell.schemaFactorReadComplete(origin.state, origin) || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !origin.matches(proof, origin.readOrdinal) {
		return false
	}
	factorKey := origin.state.schema.factorSemanticAt(origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || !factor.receipt.valid() || factor.receipt.state != origin.state || factor.receipt.authority != origin.state.authority || factor.receipt.ordinal != origin.factor || factor.receipt.algebra != cell.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(int(origin.readOrdinal))
	row, rowOK := origin.state.schema.ruleReadShapeAt(origin.ruleOrdinal, origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadExact || row.Input != origin.input || row.Factor != factorKey || row.DependencyCount != 0 || surface.Factor != factorKey || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Local == 0 {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	readProof, proofOK := newRuleReadReceiptProof(factor.receipt, surface)
	if !unitOK || !proofOK {
		return false
	}
	normalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	equal := func(left, right OrderedCells[V]) bool {
		return equalOrderedCellRecords(left.record, right.record, cell.impl.algebra.Equal)
	}
	fingerprint := func(value OrderedCells[V]) uint64 {
		return fingerprintOrderedCellRecord(value.record, cell.impl.algebra.Fingerprint)
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, OrderedCells[V]]{input: int(origin.input), binding: factor.binding, unit: unit, proof: readProof, normalize: normalize, equal: equal, fingerprint: fingerprint})
}

func (cell *schemaFactorBindingCell[K, V]) sealedImplementation(state *schemaBindingState, authority *schemaBindingAuthority) (*FactorImplementation[K, V], bool) {
	if cell == nil || cell.impl == nil || state == nil || authority == nil || cell.impl.state != state || state.schema != cell.schema || state.authority != authority || state.phase != schemaBindingSealed || cell.ordinal >= uint64(len(state.factors)) || state.factors[cell.ordinal] != cell || cell.impl.algebra == nil {
		return nil, false
	}
	forms := make([]factorFormReceipt, len(cell.forms))
	for index := range forms {
		shape, shapeOK := state.schema.factorFormShapeAt(cell.ordinal, uint64(index))
		if !shapeOK {
			return nil, false
		}
		forms[index] = factorFormReceipt{ordinal: uint64(index), kind: schemaFormKind(shape.Kind), semantic: shape.Semantic}
	}
	receipt := factorRuntimeReceipt{state: state, authority: authority, schema: state.schema, ordinal: cell.ordinal, semantic: state.schema.factorSemanticAt(cell.ordinal), keyEnd: cell.impl.algebra.KeyEnd(), algebra: cell.impl.algebra, forms: forms, issued: true}
	if !receipt.validForms() {
		return nil, false
	}
	result := *cell.impl
	result.state = state
	result.receipt = receipt
	result.descriptor = factorRuntimeDescriptor{schema: state.schema, state: state, ordinal: cell.ordinal, semantic: receipt.semantic, keyEnd: receipt.keyEnd, algebra: receipt.algebra}
	return &result, true
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorComplete() bool {
	if cell == nil || cell.schema == nil || cell.impl == nil || cell.impl.state == nil || cell.exactRead == nil || cell.exactWrite == nil || !cell.exactRead.schemaFactorFormComplete() || !cell.exactWrite.schemaFactorFormComplete() {
		return false
	}
	count, ok := cell.schema.factorFormCount(cell.ordinal)
	if !ok || len(cell.forms) != count {
		return false
	}
	for _, form := range cell.forms {
		if form == nil || !form.schemaFactorFormComplete() || form.schemaBindingSchema() != cell.schema {
			return false
		}
	}
	return true
}

type schemaFactorFormCell[K ~uint32 | ~uint64, V any] struct {
	schema  *Schema
	ordinal uint64
	kind    SchemaFormKind
	factor  *schemaFactorBindingCell[K, V]
	algebra *factbinding.Algebra[K, V]
}

func (cell *schemaFactorFormCell[K, V]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaFactorFormCell[K, V]) schemaFactorFormComplete() bool {
	if cell == nil || cell.schema == nil || cell.factor == nil || cell.algebra == nil || cell.factor.schema != cell.schema || cell.factor.impl == nil || cell.factor.impl.state == nil {
		return false
	}
	switch cell.kind {
	case SchemaFormReadExact, SchemaFormWriteExact:
		return cell.ordinal == cell.factor.ordinal
	case SchemaFormReadSummary, SchemaFormReadDistributiveSummary:
		if cell.ordinal>>32 != cell.factor.ordinal {
			return false
		}
		shape, ok := cell.schema.factorFormShapeAt(cell.factor.ordinal, uint64(uint32(cell.ordinal)))
		rowKind, optional := factorFormRowKind(cell.kind)
		return ok && optional && shape.Kind == rowKind
	default:
		return false
	}
}

type schemaRuleBindingCellImpl[K ~uint32 | ~uint64, V, O any] struct {
	state   *schemaBindingState
	schema  *Schema
	ordinal uint64
	impl    *ruleHotImplementation[K, V, O]
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleComplete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.state.schema != cell.schema || cell.impl == nil || cell.impl.state != cell.state || cell.impl.rule == nil || cell.impl.rule.cell == nil || cell.impl.rule.cell.schema != cell.schema || cell.impl.write.cell == nil || cell.impl.write.cell.schema != cell.schema || cell.impl.output == nil || cell.impl.output.schema != cell.schema || cell.impl.output.impl == nil || cell.impl.output.impl.state != cell.state {
		return false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok || shape.OutputKind != composition.FactorOutput || shape.WriteCount != 1 || uint64(len(cell.impl.reads)) != shape.ReadCount || shape.CarryCount > 1 {
		return false
	}
	if shape.CarryCount == 0 {
		if cell.impl.carry != nil {
			return false
		}
	} else if !cell.impl.carry.complete(cell.state, cell, cell.ordinal, cell.impl.output) {
		return false
	}
	coldAdmission, admissionOK := coldRuleAdmission(shape.Admission)
	if !admissionOK || !cell.impl.admission.valid() || cell.impl.admission.kind != coldAdmission.kind || cell.impl.admission.identity != coldAdmission.identity {
		return false
	}
	ruleOrdinal, ruleOK := cell.impl.rule.Ordinal()
	writeOrdinal := cell.impl.write.cell.ordinal
	if !ruleOK || ruleOrdinal != cell.ordinal || writeOrdinal>>32 != cell.ordinal || uint64(uint32(writeOrdinal)) != 0 {
		return false
	}
	write, writeOK := cell.schema.ruleWriteShapeAt(cell.ordinal, 0)
	if !writeOK || write.Factor != shape.Output || write.Kind != composition.WriteExact && write.Kind != composition.WriteRoute {
		return false
	}
	if write.Kind == composition.WriteExact && write.Route != 0 || write.Kind == composition.WriteRoute && (write.Route == 0 || write.Route > shape.ReadCount) {
		return false
	}
	if write.Kind == composition.WriteRoute {
		read, readOK := cell.schema.ruleReadShapeAt(cell.ordinal, write.Route-1)
		if !readOK || read.Kind != composition.ReadSelect || read.Factor != shape.Output || read.Semantic != read.Factor || read.Normalizer.Available() || read.DependencyCount == 0 {
			return false
		}
	}
	for _, read := range cell.impl.reads {
		if read == nil || !read.complete(cell.state, cell, cell.ordinal) {
			return false
		}
	}
	outputOrdinal, outputOK := cell.impl.output.ordinalFromSchema()
	return outputOK && outputOrdinal < cell.schema.factorCount() && cell.schema.factorSemanticAt(outputOrdinal) == shape.Output && cell.impl.operandContent != nil && cell.impl.admission.valid() && cell.impl.transfer != nil
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleProofMatches(proof *ruleRuntimeProof) bool {
	if cell == nil || proof == nil || cell.state == nil || cell.impl == nil || cell.schema != proof.schema || cell.ordinal != proof.ordinal || cell.state != proof.state || cell.impl.state != cell.state || cell.state.authority != proof.bindingAuthority || cell.impl.output == nil || cell.impl.output.schema != proof.schema || cell.impl.output.impl == nil || cell.impl.output.impl.state != proof.state || cell.impl.output.impl.algebra == nil {
		return false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok {
		return false
	}
	admission, admitted := coldRuleAdmission(shape.Admission)
	return admitted && shape.OutputKind == proof.outputKind && shape.Output == proof.output && shape.Inputs == proof.inputs && shape.ReadCount == proof.reads && shape.CarryCount == proof.carries && shape.WriteCount == proof.writes && shape.OperandFamily == proof.operandFamily && admission == proof.admission && cell.impl.admission.kind == admission.kind && cell.impl.admission.identity == admission.identity
}

func (cell *schemaFactorBindingCell[K, V]) ordinalFromSchema() (uint64, bool) {
	if cell == nil || cell.schema == nil || !cell.schema.Available() {
		return 0, false
	}
	return cell.ordinal, true
}

type schemaSummaryReadCell[K ~uint32 | ~uint64, V, S any] struct {
	schema      *Schema
	ordinal     uint64
	factor      *schemaFactorBindingCell[K, V]
	algebra     *factbinding.Algebra[K, V]
	form        SchemaReadForm[V]
	normalize   func(OrderedCells[V]) S
	equal       func(S, S) bool
	fingerprint func(S) uint64
}

// schemaOpaqueSummaryRuleReadForm is the coordinate-erased summary authority
// consumed by heterogeneous Rule transactions. V and S remain typed while
// the owning Factor cell keeps its private K instantiation.
type schemaOpaqueSummaryRuleReadForm[V, S any] interface {
	schemaFactorFormBinding
	schemaSummaryRuleReadComplete(*schemaBindingState, *schemaRuleReadOrigin) bool
	schemaSummaryRuleReadBind(readBinding, equation.RuleMember, map[composition.Key]runtimeFactor, *schemaRuleReadOrigin) bool
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaFactorFormComplete() bool {
	if cell == nil || cell.schema == nil || cell.factor == nil || cell.algebra == nil || cell.factor.impl == nil || cell.factor.impl.state == nil || cell.form.cell == nil || cell.form.cell.schema != cell.schema || !summaryReadFormKind(cell.form.cell.kind) || cell.normalize == nil || cell.equal == nil || cell.fingerprint == nil {
		return false
	}
	if cell.ordinal>>32 != cell.factor.ordinal {
		return false
	}
	shape, ok := cell.schema.factorFormShapeAt(cell.factor.ordinal, uint64(uint32(cell.ordinal)))
	return ok && summaryReadRowKind(shape.Kind)
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaSummaryRuleReadComplete(state *schemaBindingState, origin *schemaRuleReadOrigin) bool {
	if cell == nil || state == nil || origin == nil || origin.state != state || origin.kind != composition.ReadSummary ||
		cell.factor == nil || cell.factor.impl == nil || cell.factor.impl.algebra == nil || cell.factor.impl.state != state ||
		cell.factor.ordinal != origin.factor || cell.schema != state.schema || cell.form.cell == nil ||
		cell.form.cell.schema != state.schema || !summaryReadFormKind(cell.form.cell.kind) || !cell.schemaFactorFormComplete() {
		return false
	}
	shape, ok := state.schema.ruleReadShapeAt(origin.ruleOrdinal, origin.readOrdinal)
	return ok && shape.Kind == composition.ReadSummary && shape.Input == origin.input &&
		shape.Factor == state.schema.factorSemanticAt(origin.factor) && shape.Semantic == origin.semantic &&
		shape.Normalizer == origin.semantic && shape.DependencyCount == 0
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaSummaryRuleReadBind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor, origin *schemaRuleReadOrigin) bool {
	if cell == nil || bound == nil || origin == nil || member.ReadCount() <= int(origin.readOrdinal) || factors == nil {
		return false
	}
	proofOwner, ok := bound.(interface{ runtimeRuleProof() *ruleRuntimeProof })
	if !ok {
		return false
	}
	proof := proofOwner.runtimeRuleProof()
	if proof == nil || !origin.matches(proof, origin.readOrdinal) || !cell.schemaSummaryRuleReadComplete(origin.state, origin) {
		return false
	}
	factorKey := origin.state.schema.factorSemanticAt(origin.factor)
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || !factor.receipt.valid() || factor.receipt.state != origin.state ||
		factor.receipt.authority != origin.state.authority || factor.receipt.ordinal != origin.factor ||
		factor.receipt.algebra != cell.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(int(origin.readOrdinal))
	row, rowOK := origin.state.schema.ruleReadShapeAt(origin.ruleOrdinal, origin.readOrdinal)
	if !surfaceOK || !rowOK || row.Kind != composition.ReadSummary || row.Input != origin.input ||
		row.Factor != factorKey || row.Semantic != origin.semantic || row.Normalizer != origin.semantic || row.DependencyCount != 0 ||
		surface.Factor != factorKey || surface.Form != equation.SurfaceReadSummary || surface.Semantic != origin.semantic ||
		surface.Normalizer != origin.semantic || surface.Mode != equation.TargetModeNone {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	unitRow, rowPresent := factor.reads[surface]
	proofSummary, proofOK := factor.summaryReadReceiptProof(surface, uint64(uint32(cell.ordinal)), origin.semantic)
	if !unitOK || !rowPresent || unitRow.kind != carrier.SummaryUnit || !proofOK {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, S]{input: int(origin.input), binding: factor.binding, unit: unit, summary: proofSummary, normalize: cell.normalize, equal: cell.equal, fingerprint: cell.fingerprint})
}

// Ref issues the callback-free Factor implementation's opaque exact-key
// capability. The Ref carries the shared sealed authority pointer, never a
// copied SchemaBinding handle or a public coordinate accessor.
func (cell *schemaFactorBindingCell[K, V]) schemaFactorAdmitExactRead(state *schemaBindingState, authority *schemaBindingAuthority, transaction *RuleSourceTransaction, local uint64) bool {
	implementation, ok := cell.sealedImplementation(state, authority)
	if !ok {
		return false
	}
	ref, refOK := implementation.Ref(K(local))
	return refOK && AddExactRead(transaction, ref)
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorAdmitExactWrite(state *schemaBindingState, authority *schemaBindingAuthority, transaction *RuleSourceTransaction, local uint64) bool {
	implementation, ok := cell.sealedImplementation(state, authority)
	if !ok {
		return false
	}
	ref, refOK := implementation.Ref(K(local))
	return refOK && AddExactWrite(transaction, ref)
}

func (implementation *FactorImplementation[K, V]) Ref(key K) (Ref[K], bool) {
	if implementation == nil || !implementation.receipt.valid() || uint64(key) >= implementation.receipt.keyEnd {
		return Ref[K]{}, false
	}
	receipt := implementation.receipt
	return Ref[K]{compositionID: receipt.schema.ID(), bindingAuthority: receipt.authority, factorKey: receipt.semantic, factorIndex: receipt.ordinal, raw: key}, true
}

func (implementation *FactorImplementation[K, V]) NewClosedRefs() *ClosedRefs[K] {
	if implementation == nil || !implementation.receipt.valid() {
		return nil
	}
	return &ClosedRefs[K]{receipt: implementation.receipt}
}

func (implementation *FactorImplementation[K, V]) OwnsClosedRefs(refs *ClosedRefs[K]) bool {
	return implementation != nil && refs != nil && refs.validIssuer() && implementation.receipt.valid() && refs.receipt.state == implementation.receipt.state && refs.receipt.authority == implementation.receipt.authority && refs.receipt.schema == implementation.receipt.schema && refs.receipt.ordinal == implementation.receipt.ordinal
}
