// schema_factor_binding.go holds the Factor, form and Rule binding cells.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
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

// AxisAlgebra projects a bound Factor onto the algebra the axis surface
// publishes it as. It lives here because the projection is a restatement of
// this spec's own fields, and a restatement made once per axis is a restatement
// that drifts: the measures are the field pair a copy silently drops.
//
// A measure the spec does not carry projects as the absent rank rather than as
// a missing one. An axis that declares no narrowing has a narrow measure of
// width zero, and that is the same value whether it is written out or left off.
func (spec HotFactorSpec[K, V]) AxisAlgebra() (axis.Algebra[V], bool) {
	return axis.Adopt(axis.CarrierAlgebra[K, V]{
		KeyEnd:      spec.KeyEnd,
		Lattice:     spec.Lattice,
		Default:     spec.Default,
		AdmitAt:     spec.AdmitAt,
		Fingerprint: spec.Fingerprint,
		Widen:       axis.CarrierRank[K, V]{Width: spec.WidenRank.Width, At: spec.WidenRank.At},
		Narrow:      axis.CarrierRank[K, V]{Width: spec.NarrowRank.Width, At: spec.NarrowRank.At},
	})
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

// exactReadLocal and exactWriteLocal are Factor-owner checks. They validate a
// sealed Factor row and the corresponding surface in one place, then hand the
// runtime carrier only its dense local address.
func exactReadLocal(row schemaFactorBinding, surface equation.Surface) (uint64, bool) {
	if !factorRowAvailable(row) || surface.Factor != row.schemaFactorSemanticKey() || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 || surface.Local > row.schemaFactorAlgebra().KeyEnd() {
		return 0, false
	}
	return surface.Local - 1, true
}

func exactWriteLocal(row schemaFactorBinding, surface equation.Surface) (uint64, bool) {
	if !factorRowAvailable(row) || surface.Factor != row.schemaFactorSemanticKey() || surface.Form != equation.SurfaceWriteExact || surface.Mode != equation.TargetModeStrong || surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 || surface.Local > row.schemaFactorAlgebra().KeyEnd() {
		return 0, false
	}
	return surface.Local - 1, true
}

// RuleImplementation is the typed public handle for the canonical sealed Rule
// cell and ordinal. Output is the canonical Factor row handle; callbacks and
// structural draft handles remain cell-owned.
type RuleImplementation[K ~uint32 | ~uint64, V, O any] struct {
	cell    *schemaRuleBindingCellImpl[K, V, O]
	ordinal uint64
	output  schemaFactorBinding
}

// sealedRuleCell is the ordinary Rule implementation fence. It authenticates
// the three fields of RuleImplementation against the sealed cell and its
// Factor-owned output. Rule geometry was checked once at Binding.Seal and is
// never reconstructed from the cold Rule shape here.
func (implementation *RuleImplementation[K, V, O]) sealedRuleCell() (*schemaRuleBindingCellImpl[K, V, O], bool) {
	if implementation == nil || implementation.cell == nil || implementation.ordinal != implementation.cell.ordinal {
		return nil, false
	}
	cell := implementation.cell
	state := cell.state
	outputRow := implementation.output
	if state == nil || cell.schema == nil || state.schema != cell.schema || state.phase != schemaBindingSealed || state.authority == nil || implementation.ordinal >= uint64(len(state.rules)) || state.rules[implementation.ordinal] != cell || !cell.sealedRuleComplete() || !factorRowAvailable(outputRow) || outputRow.schemaFactorBindingState() != state || outputRow.schemaFactorSchema() != state.schema || outputRow.schemaFactorOrdinal() >= uint64(len(state.factors)) || state.factors[outputRow.schemaFactorOrdinal()] != outputRow {
		return nil, false
	}
	output := cell.impl.output
	if output == nil || output.impl == nil || output.state != state || output.schema != state.schema || output != outputRow || output.impl.algebra != outputRow.schemaFactorAlgebra() || output.ordinal != outputRow.schemaFactorOrdinal() {
		return nil, false
	}
	return cell, true
}

type schemaRuleBindingCell interface {
	schemaBindingCell
	schemaRuleOrdinal() uint64
	schemaRuleBindingState() *schemaBindingState
	schemaRuleReadAt(uint64) *schemaRuleReadRow
	schemaRuleComplete() bool
}

// sealedRuleGeometry is the direct, post-Seal geometry shared by ordinary and
// generated Rule cells. It is the graph compiler's structural input; runtime
// binders and domain callbacks are intentionally absent. Activation cells do
// not implement it because their relation is authenticated by equation's
// ActivationMember witness.
type sealedRuleGeometry interface {
	schemaRuleBindingCell
	sealedRuleComplete() bool
	directRuleSemantic() composition.Key
	directRuleOperandFamily() composition.Key
	directRuleOutputFactor() composition.Key
	directRuleReadCount() uint64
	directRuleWriteMode() directRuleWriteMode
	directRuleRouteRead() uint64
	directRuleCarryPresent() bool
	directRuleCarryInput() uint64
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
	schemaFactorOrdinal() uint64
	schemaFactorSchema() *Schema
	schemaFactorBindingState() *schemaBindingState
	schemaFactorAlgebra() anyFactorAlgebra
	schemaFactorSemanticKey() composition.Key
	schemaFactorComplete() bool
	// schemaFactorAdmitsRuleFamily reports whether one installer is typed in
	// this Factor's own key and fact. A family belongs to the Factor its rule
	// writes to, so an installer typed at any other pair is not this Factor's
	// family at all - and because the claim is resolved by a type assertion at
	// Program seal, an untyped one would otherwise surface as a Factor that
	// silently refuses to bind, far from the declaration that made it.
	schemaFactorAdmitsRuleFamily(installer any) bool
	schemaFactorRuntimeBinding(*runtimeBinding) (runtimeFactor, bool)
	schemaFactorReadComplete(*schemaBindingState, *schemaRuleReadRow) bool
	schemaFactorBindExactRead(readBinding, equation.RuleMember, map[composition.Key]runtimeFactor, *schemaRuleReadRow, ReadContract) bool
	schemaFactorFormAt(uint64) schemaFactorFormBinding
	schemaFactorExactRead(*schemaBindingState, *schemaBindingAuthority, uint64) (RuleReadSurface, bool)
	schemaFactorExactWrite(*schemaBindingState, *schemaBindingAuthority, uint64) (ruleWriteSurface, bool)
	schemaFactorRelationOwner() memberrelation.Owner
	setSchemaFactorRelationOwner(memberrelation.Owner) bool
}

func factorRowAvailable(row schemaFactorBinding) bool {
	if row == nil || row.schemaFactorAlgebra() == nil || !row.schemaFactorSemanticKey().Available() {
		return false
	}
	state, schema := row.schemaFactorBindingState(), row.schemaFactorSchema()
	ordinal := row.schemaFactorOrdinal()
	return state != nil && state.phase == schemaBindingSealed && schema != nil && state.schema == schema && ordinal < uint64(len(state.factors)) && state.factors[ordinal] == row
}

type FactorImplementation[K ~uint32 | ~uint64, V any] struct {
	row     schemaFactorBinding
	ordinal uint64
	algebra *factbinding.Algebra[K, V]
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
	state       *schemaBindingState
	ordinal     uint64
	schema      *Schema
	impl        *FactorImplementation[K, V]
	exactRead   schemaFactorFormBinding
	exactWrite  schemaFactorFormBinding
	forms       []schemaFactorFormBinding
	summaryKeys []uint64
	// relationOwner is immutable construction-only generated relation authority.
	// It remains on the sealed Factor cell so the same sealed composition can
	// construct more than one Program, but is never copied into runtime Factor
	// bindings or generated member rows.
	relationOwner memberrelation.Owner
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

// schemaFactorSemanticKey is the Factor-owned output identity used by sealed
// Rule consumers. It is not a Rule semantic copy: the Factor cell remains the
// sole authority for this key.
func (cell *schemaFactorBindingCell[K, V]) schemaFactorSemanticKey() composition.Key {
	if cell == nil || cell.schema == nil || cell.ordinal >= cell.schema.factorCount() {
		return composition.Key{}
	}
	return cell.schema.factorSemanticAt(cell.ordinal)
}

// schemaFactorSummaryKeys issues the closed raw-key plane from its Factor
// owner. Query bindings borrow this immutable plane; they must not reconstruct
// the same coordinates for every observation or admission.
func (cell *schemaFactorBindingCell[K, V]) schemaFactorSummaryKeys() ([]uint64, bool) {
	if cell == nil || cell.impl == nil || cell.impl.algebra == nil {
		return nil, false
	}
	keyEnd := cell.impl.algebra.KeyEnd()
	if keyEnd > uint64(^uint(0)>>1) {
		return nil, false
	}
	// A zero-width Factor has a canonical empty summary domain. It does not
	// mint a neutral key: there is no owner-issued coordinate in an empty
	// domain. The schema-level summary remains issuable so a heterogeneous
	// consumer can observe the absence without inventing a root.
	if len(cell.summaryKeys) == 0 {
		cell.summaryKeys = make([]uint64, int(keyEnd))
		for index := range cell.summaryKeys {
			cell.summaryKeys[index] = uint64(index)
		}
	}
	return cell.summaryKeys, uint64(len(cell.summaryKeys)) == keyEnd
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorBindingState() *schemaBindingState {
	if cell == nil {
		return nil
	}
	return cell.state
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorRelationOwner() memberrelation.Owner {
	if cell == nil {
		return nil
	}
	return cell.relationOwner
}

func (cell *schemaFactorBindingCell[K, V]) setSchemaFactorRelationOwner(owner memberrelation.Owner) bool {
	if cell == nil || owner == nil || cell.relationOwner != nil {
		return false
	}
	cell.relationOwner = owner
	return true
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorRuntimeBinding(runtime *runtimeBinding) (runtimeFactor, bool) {
	if cell == nil || cell.impl == nil || runtime == nil || runtime.state == nil || runtime.authority == nil {
		return nil, false
	}
	implementation, ok := cell.sealedImplementation(runtime.state, runtime.authority)
	if !ok {
		return nil, false
	}
	return bindFactorFromGraph(implementation, runtime)
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorReadComplete(state *schemaBindingState, row *schemaRuleReadRow) bool {
	if row == nil {
		return false
	}
	return row.live(state, row.owner, row.ownerOrdinal) && cell != nil && state != nil && cell.schema == state.schema && cell.state == state && cell.impl != nil && cell.impl.algebra != nil && cell.ordinal == row.factorOrdinal
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorFormAt(index uint64) schemaFactorFormBinding {
	if cell == nil || index >= uint64(len(cell.forms)) {
		return nil
	}
	return cell.forms[index]
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorBindExactRead(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor, row *schemaRuleReadRow, contract ReadContract) bool {
	if cell == nil || bound == nil || row == nil || member.ReadCount() <= int(row.readOrdinal) || !cell.schemaFactorReadComplete(row.ownerState(), row) || factors == nil {
		return false
	}
	if !row.sealed() {
		return false
	}
	factorKey := row.factor
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || factor.implementation == nil || !factorRowAvailable(factor.implementation.row) || factor.implementation.row != cell || factor.implementation.ordinal != row.factorOrdinal || factor.implementation.algebra != cell.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(int(row.readOrdinal))
	if !surfaceOK || row.kind != composition.ReadExact || surface.Factor != factorKey || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Local == 0 {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	readLocal, localOK := exactReadLocal(factor.implementation.row, surface)
	if !unitOK || !localOK {
		return false
	}
	normalize := func(value OrderedCells[V]) OrderedCells[V] { return value }
	equal := func(left, right OrderedCells[V]) bool {
		return equalOrderedCellRecords(left.record, right.record, cell.impl.algebra.Equal)
	}
	fingerprint := func(value OrderedCells[V]) uint64 {
		return fingerprintOrderedCellRecord(value.record, cell.impl.algebra.Fingerprint)
	}
	policy, admitted := exactReadPolicy(cell.impl.algebra.Default, contract)
	if !admitted {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, OrderedCells[V]]{input: int(row.input), binding: factor.binding, unit: unit, exactFactor: factor.implementation.row, exactRaw: readLocal, exact: true, normalize: normalize, equal: equal, fingerprint: fingerprint, policy: policy})
}

func (cell *schemaFactorBindingCell[K, V]) sealedImplementation(state *schemaBindingState, authority *schemaBindingAuthority) (*FactorImplementation[K, V], bool) {
	if cell == nil || cell.impl == nil || state == nil || authority == nil || cell.state != state || state.schema != cell.schema || state.authority != authority || state.phase != schemaBindingSealed || cell.ordinal >= uint64(len(state.factors)) || state.factors[cell.ordinal] != cell || cell.impl.algebra == nil || !cell.schemaFactorComplete() {
		return nil, false
	}
	result := *cell.impl
	result.row = cell
	result.ordinal = cell.ordinal
	return &result, true
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorAdmitsRuleFamily(installer any) bool {
	if cell == nil || installer == nil {
		return false
	}
	_, typed := installer.(execution.RuleFamilyInstaller[K, V])
	return typed
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorComplete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.impl == nil || cell.exactRead == nil || cell.exactWrite == nil || !cell.exactRead.schemaFactorFormComplete() || !cell.exactWrite.schemaFactorFormComplete() {
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
	if cell == nil || cell.schema == nil || cell.factor == nil || cell.algebra == nil || cell.factor.schema != cell.schema || cell.factor.impl == nil || cell.factor.state == nil {
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

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleBindingState() *schemaBindingState {
	if cell == nil {
		return nil
	}
	return cell.state
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleReadAt(index uint64) *schemaRuleReadRow {
	if cell == nil || cell.impl == nil || index >= uint64(len(cell.impl.reads)) || cell.impl.reads[index] == nil {
		return nil
	}
	return cell.impl.reads[index].readRow()
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) directRuleSemantic() composition.Key {
	if cell == nil || cell.impl == nil {
		return composition.Key{}
	}
	return cell.impl.ruleSemantic
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) directRuleOperandFamily() composition.Key {
	if cell == nil || cell.impl == nil {
		return composition.Key{}
	}
	return cell.impl.operandFamily
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) directRuleOutputFactor() composition.Key {
	if cell == nil || cell.impl == nil || cell.impl.output == nil {
		return composition.Key{}
	}
	return cell.impl.output.schemaFactorSemanticKey()
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) directRuleReadCount() uint64 {
	if cell == nil || cell.impl == nil {
		return 0
	}
	return uint64(len(cell.impl.reads))
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) directRuleWriteMode() directRuleWriteMode {
	if cell == nil || cell.impl == nil {
		return 0
	}
	return cell.impl.writeMode
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) directRuleRouteRead() uint64 {
	if cell == nil || cell.impl == nil {
		return 0
	}
	return cell.impl.routeRead
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) directRuleCarryPresent() bool {
	return cell != nil && cell.impl != nil && cell.impl.carryPresent
}

func (cell *schemaRuleBindingCellImpl[K, V, O]) directRuleCarryInput() uint64 {
	if cell == nil || cell.impl == nil {
		return 0
	}
	return cell.impl.carryInput
}

// schemaRuleComplete is intentionally phase-sensitive. During the open phase
// it is the one cold Schema preflight used by SchemaBinding.Seal. Once the
// binding is sealed it is only an immutable cell/row invariant check.
func (cell *schemaRuleBindingCellImpl[K, V, O]) schemaRuleComplete() bool {
	if cell == nil || cell.state == nil {
		return false
	}
	if cell.state.phase == schemaBindingOpen {
		return cell.validateOpenRuleCell()
	}
	return cell.sealedRuleComplete()
}

// validateOpenRuleCell is the only ordinary Rule validation that may walk the
// Rule/Write/Carry shapes. It runs while the draft handles are still present,
// and never mutates the cell.
func (cell *schemaRuleBindingCellImpl[K, V, O]) validateOpenRuleCell() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.state.phase != schemaBindingOpen || cell.state.schema != cell.schema || cell.state.authority == nil || cell.ordinal >= uint64(len(cell.state.rules)) || cell.state.rules[cell.ordinal] != cell || cell.impl == nil || cell.impl.state != cell.state || cell.impl.rule == nil || cell.impl.rule.cell == nil || cell.impl.rule.cell.schema != cell.schema || cell.impl.write.cell == nil || cell.impl.write.cell.schema != cell.schema || cell.impl.output == nil || cell.impl.output.schema != cell.schema || cell.impl.output.impl == nil || cell.impl.output.state != cell.state {
		return false
	}
	shape, ok := cell.schema.ruleShapeAt(cell.ordinal)
	if !ok || !cell.schema.ruleSemanticAt(cell.ordinal).Available() || !shape.OperandFamily.Available() || shape.OutputKind != composition.FactorOutput || !shape.Output.Available() || shape.WriteCount != 1 || uint64(len(cell.impl.reads)) != shape.ReadCount || shape.CarryCount > 1 {
		return false
	}
	if shape.CarryCount == 0 {
		if cell.impl.carry != nil {
			return false
		}
	} else if !cell.impl.carry.complete(cell.state, cell, cell.ordinal, cell.impl.output) {
		return false
	}
	ruleOrdinal, ruleOK := cell.impl.rule.Ordinal()
	writeOrdinal := cell.impl.write.cell.ordinal
	if !ruleOK || ruleOrdinal != cell.ordinal || writeOrdinal>>32 != cell.ordinal || uint64(uint32(writeOrdinal)) != 0 {
		return false
	}
	write, writeOK := cell.schema.ruleWriteShapeAt(cell.ordinal, 0)
	if !writeOK || write.Factor != shape.Output || (write.Kind != composition.WriteExact && write.Kind != composition.WriteRoute) {
		return false
	}
	if write.Kind == composition.WriteExact && write.Route != 0 || write.Kind == composition.WriteRoute && (write.Route == 0 || write.Route > shape.ReadCount) {
		return false
	}
	if write.Kind == composition.WriteRoute {
		read := cell.schemaRuleReadAt(write.Route - 1)
		if read == nil || read.readOrdinal != write.Route-1 || read.kind != composition.ReadSelect || read.factor != shape.Output || read.semantic != read.factor || read.normalizer.Available() || len(read.dependencies) == 0 {
			return false
		}
	}
	if write.Kind == composition.WriteExact && cell.impl.projectWrite == nil {
		return false
	}
	for _, read := range cell.impl.reads {
		if read == nil || !read.complete(cell.state, cell, cell.ordinal) {
			return false
		}
		row := read.readRow()
		if row == nil || row.owner != cell || row.ownerOrdinal != cell.ordinal || row.readOrdinal >= uint64(len(cell.impl.reads)) || cell.impl.reads[row.readOrdinal] != read {
			return false
		}
	}
	outputOrdinal, outputOK := cell.impl.output.ordinalFromSchema()
	return outputOK && outputOrdinal < cell.schema.factorCount() && cell.schema.factorSemanticAt(outputOrdinal) == shape.Output && cell.impl.operandContent != nil && cell.impl.operandResolver != nil && cell.impl.fold != nil
}

// sealedRuleComplete authenticates only already-published direct geometry.
// It deliberately has no Schema RuleShape/WriteShape/CarryShape calls.
func (cell *schemaRuleBindingCellImpl[K, V, O]) sealedRuleComplete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || cell.state.phase != schemaBindingSealed || cell.state.schema != cell.schema || cell.state.authority == nil || cell.ordinal >= uint64(len(cell.state.rules)) || cell.state.rules[cell.ordinal] != cell || cell.impl == nil || cell.impl.state != cell.state || cell.impl.rule != nil || cell.impl.write.cell != nil || cell.impl.carry != nil || cell.impl.output == nil || cell.impl.output.schema != cell.schema || cell.impl.output.impl == nil || cell.impl.output.state != cell.state || !cell.impl.ruleSemantic.Available() || !cell.impl.operandFamily.Available() || cell.impl.writeMode == 0 || cell.impl.operandContent == nil || cell.impl.operandResolver == nil || cell.impl.fold == nil {
		return false
	}
	if cell.impl.writeMode == directRuleWriteExact {
		if cell.impl.routeRead != 0 || cell.impl.projectWrite == nil {
			return false
		}
	} else if cell.impl.writeMode == directRuleWriteRoute {
		if cell.impl.routeRead == 0 || cell.impl.routeRead > uint64(len(cell.impl.reads)) {
			return false
		}
		row := cell.schemaRuleReadAt(cell.impl.routeRead - 1)
		if row == nil || row.owner != cell || row.ownerOrdinal != cell.ordinal || row.readOrdinal != cell.impl.routeRead-1 || row.kind != composition.ReadSelect || row.factor != cell.impl.output.schemaFactorSemanticKey() || row.semantic != row.factor || row.normalizer.Available() || len(row.dependencies) == 0 {
			return false
		}
	} else {
		return false
	}
	if cell.impl.carryPresent {
		if cell.impl.carryTransform.Available() != (cell.impl.carryApply != nil) {
			return false
		}
	} else if cell.impl.carryInput != 0 || cell.impl.carryTransform.Available() || cell.impl.carryApply != nil {
		return false
	}
	for index, read := range cell.impl.reads {
		if read == nil || !read.complete(cell.state, cell, cell.ordinal) {
			return false
		}
		row := read.readRow()
		if row == nil || row.owner != cell || row.ownerOrdinal != cell.ordinal || row.readOrdinal != uint64(index) {
			return false
		}
	}
	return true
}

// finalizeOrdinaryRuleCell is called only after every Rule has passed
// validateOpenRuleCell and all other Seal checks have succeeded. It has no
// failure path: it publishes direct geometry, then drops the construction
// handles before the sealed phase is made visible.
func (cell *schemaRuleBindingCellImpl[K, V, O]) finalizeOrdinaryRuleCell() {
	hot := cell.impl
	shape, _ := cell.schema.ruleShapeAt(cell.ordinal)
	write, _ := cell.schema.ruleWriteShapeAt(cell.ordinal, 0)
	hot.ruleSemantic = cell.schema.ruleSemanticAt(cell.ordinal)
	hot.operandFamily = shape.OperandFamily
	hot.writeMode = directRuleWriteExact
	hot.routeRead = 0
	if write.Kind == composition.WriteRoute {
		hot.writeMode = directRuleWriteRoute
		hot.routeRead = write.Route
	}
	hot.carryPresent = shape.CarryCount == 1
	hot.carryInput = 0
	hot.carryTransform = composition.Key{}
	hot.carryApply = nil
	if hot.carryPresent {
		carryShape, _ := cell.schema.ruleCarryShapeAt(cell.ordinal, 0)
		hot.carryInput = carryShape.Input
		hot.carryTransform = carryShape.Transform
		hot.carryApply = hot.carry.apply
	}
	hot.rule = nil
	hot.write = SchemaWriteSlot[V]{}
	hot.carry = nil
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
	schemaSummaryRuleReadComplete(*schemaBindingState, *schemaRuleReadRow) bool
	schemaSummaryRuleReadBind(readBinding, equation.RuleMember, map[composition.Key]runtimeFactor, *schemaRuleReadRow) bool
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaFactorFormComplete() bool {
	if cell == nil || cell.schema == nil || cell.factor == nil || cell.algebra == nil || cell.factor.impl == nil || cell.factor.state == nil || cell.form.cell == nil || cell.form.cell.schema != cell.schema || !summaryReadFormKind(cell.form.cell.kind) || cell.normalize == nil || cell.equal == nil || cell.fingerprint == nil {
		return false
	}
	if cell.ordinal>>32 != cell.factor.ordinal {
		return false
	}
	shape, ok := cell.schema.factorFormShapeAt(cell.factor.ordinal, uint64(uint32(cell.ordinal)))
	return ok && summaryReadRowKind(shape.Kind)
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaSummaryRuleReadComplete(state *schemaBindingState, row *schemaRuleReadRow) bool {
	if row == nil {
		return false
	}
	if cell == nil || state == nil || row.ownerState() != state || row.kind != composition.ReadSummary ||
		cell.factor == nil || cell.factor.impl == nil || cell.factor.impl.algebra == nil || cell.factor.state != state ||
		cell.factor.ordinal != row.factorOrdinal || cell.schema != state.schema || row.summaryForm == nil || row.summaryForm != cell ||
		cell.form.cell == nil || cell.form.cell.schema != state.schema || !summaryReadFormKind(cell.form.cell.kind) {
		return false
	}
	return row.factor.Available() && row.semantic.Available() && row.normalizer == row.semantic && len(row.dependencies) == 0
}

func (cell *schemaSummaryReadCell[K, V, S]) schemaSummaryRuleReadBind(bound readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor, row *schemaRuleReadRow) bool {
	if cell == nil || bound == nil || row == nil || member.ReadCount() <= int(row.readOrdinal) || factors == nil {
		return false
	}
	if !row.sealed() || !cell.schemaSummaryRuleReadComplete(row.ownerState(), row) {
		return false
	}
	factorKey := row.factor
	runtime, present := factors[factorKey]
	factor, typed := runtime.(*boundFactor[K, V])
	if !present || !typed || factor == nil || factor.implementation == nil || !factorRowAvailable(factor.implementation.row) || factor.implementation.row != cell.factor ||
		factor.implementation.ordinal != row.factorOrdinal || factor.implementation.algebra != cell.factor.impl.algebra {
		return false
	}
	surface, surfaceOK := member.ReadAt(int(row.readOrdinal))
	if !surfaceOK || row.kind != composition.ReadSummary || row.factor != factorKey || row.semantic != row.normalizer || len(row.dependencies) != 0 ||
		surface.Factor != factorKey || surface.Form != equation.SurfaceReadSummary || surface.Semantic != row.semantic ||
		surface.Normalizer != row.semantic || surface.Mode != equation.TargetModeNone {
		return false
	}
	unit, unitOK := factor.readUnit(surface)
	unitRow, rowPresent := factor.reads[surface]
	summaryFactor, summaryForm, summaryKeys, summaryDigest, summaryOK := factor.summaryReadAddress(surface, row)
	if !unitOK || !rowPresent || unitRow.kind != carrier.SummaryUnit || !summaryOK {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, S]{input: int(row.input), binding: factor.binding, unit: unit, summaryFactor: summaryFactor, summaryForm: summaryForm, summaryKeys: summaryKeys, summaryDigest: summaryDigest, summary: true, normalize: cell.normalize, equal: cell.equal, fingerprint: cell.fingerprint})
}

// Ref issues the callback-free Factor implementation's opaque exact-key
// capability. The Ref carries the shared sealed authority pointer, never a
// copied SchemaBinding handle or a public coordinate accessor.
func (cell *schemaFactorBindingCell[K, V]) schemaFactorExactRead(state *schemaBindingState, authority *schemaBindingAuthority, local uint64) (RuleReadSurface, bool) {
	implementation, ok := cell.sealedImplementation(state, authority)
	if !ok {
		return RuleReadSurface{}, false
	}
	ref, refOK := implementation.Ref(K(local))
	if !refOK {
		return RuleReadSurface{}, false
	}
	return ExactReadSurface(ref)
}

func (cell *schemaFactorBindingCell[K, V]) schemaFactorExactWrite(state *schemaBindingState, authority *schemaBindingAuthority, local uint64) (ruleWriteSurface, bool) {
	implementation, ok := cell.sealedImplementation(state, authority)
	if !ok {
		return ruleWriteSurface{}, false
	}
	ref, refOK := implementation.Ref(K(local))
	if !refOK {
		return ruleWriteSurface{}, false
	}
	return exactRuleWriteSurface(ref)
}

func (implementation *FactorImplementation[K, V]) Ref(key K) (Ref[K], bool) {
	if implementation == nil || !factorRowAvailable(implementation.row) || uint64(key) >= implementation.row.schemaFactorAlgebra().KeyEnd() {
		return Ref[K]{}, false
	}
	return Ref[K]{row: implementation.row, raw: key}, true
}
