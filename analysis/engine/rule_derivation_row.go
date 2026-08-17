// rule_derivation_row.go reads row-level derivation values and carries the read, target and schema Rule runtime proofs.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ruleResultRow is one exact Product row attached internally to a Rule
// disposition. Proof code receives a complete disposition, never a freely
// pairable row, so a typed proof read is structurally about that same row.
type ruleResultRow struct {
	ticket *ruleAdmissionTicket
	index  int
}

// DerivationDispositionReadValue resolves precisely the typed input observed
// by disposition's own Product row. It reuses the one live Product session:
// no erased value, State access, or second product is created. The
// disposition membership witness and canonical row must agree, so a foreign,
// stale, swapped, forged, or post-callback access fails closed.
func DerivationDispositionReadValue[V, O, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[S]) (S, bool) {
	var zero S
	ordinal := disposition.ordinal
	if !derivation.liveProduct() || ordinal < 0 || ordinal >= len(derivation.dispositions) || derivation.dispositions[ordinal].ordinal != ordinal || derivation.dispositions[ordinal].row != disposition.row || disposition.row.ticket != derivation.ticket || disposition.row.index != ordinal || disposition.row.index < 0 || disposition.row.index >= len(derivation.product.values) || !read.matchesRuleProof(derivation.proof) || read.index >= len(derivation.product.reads) || read.resolve == nil {
		return zero, false
	}
	id, found := derivation.product.readID(disposition.row.index, read.index)
	if !found {
		return zero, false
	}
	return read.resolve(derivation.product, read.index, id)
}

// DerivationDispositionSelectionCount exposes the cardinality of one live
// staged read for this exact disposition row. It is proof-time only: neither
// the dynamic Ref route, Unit, State, nor a mutable execution Access escapes.
func DerivationDispositionSelectionCount[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]]) (int, bool) {
	selection, row, ok := derivationDispositionSelection(derivation, disposition, read)
	if !ok || selection.count == nil {
		return 0, false
	}
	return selection.count(row)
}

// DerivationDispositionSelectionAt exposes one canonical Tag/value pair from
// a live staged read for this exact disposition row. It is intentionally not
// an Access-based Selection operation, which lets a cross-package checker
// replay its selection evidence without receiving a mutation capability.
func DerivationDispositionSelectionAt[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]], ordinal int) (Tag, S, bool) {
	var tag Tag
	var value S
	selection, row, ok := derivationDispositionSelection(derivation, disposition, read)
	if !ok || selection.at == nil || ordinal < 0 {
		return tag, value, false
	}
	return selection.at(row, ordinal)
}

// DerivationDispositionSelectionMatchesRef proves that one selected staged
// route was resolved to exactly expected. It is deliberately a predicate:
// evidence can authenticate a tag-to-Ref association without receiving a
// route, Factor key, carrier Unit, or mutable selection capability.
//
// This is distinct from DerivationReadMatchesRef. A staged read has no one
// fixed Ref, and distinct selected routes may carry equal normalized facts.
// It is also distinct from DerivationDispositionRouteValue, which applies
// only when the selected route is itself a RouteWrite output target.
func DerivationDispositionSelectionMatchesRef[V any, O any, Tag selectionTag, S any, K ~uint32 | ~uint64](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]], ordinal int, expected Ref[K]) bool {
	selection, row, ok := derivationDispositionSelection(derivation, disposition, read)
	if !ok || ordinal < 0 || selection.count == nil || selection.route == nil {
		return false
	}
	count, counted := selection.count(row)
	if !counted || ordinal >= count {
		return false
	}
	route, routed := selection.route(row, ordinal)
	return routed && selectionRouteMatchesRef(route, expected)
}

// selectionRefIdentity is the sealed identity shared by a public expected
// Ref and the private exactRef retained by one staged route. Its fields never
// leave engine: external evidence receives only the boolean result above.
// The actual route was already validated against its staged target Factor by
// stagedRouteSink, so this comparison authenticates the same owner-issued
// exact coordinate without constructing a second route authority.
type selectionRefIdentity struct {
	compositionID    CompositionID
	bindingAuthority *schemaBindingAuthority
	factorKey        composition.Key
	factorIndex      uint64
	raw              uint64
}

type selectionRefIdentityProvider interface {
	selectionRefIdentity() selectionRefIdentity
}

func (ref Ref[K]) selectionRefIdentity() selectionRefIdentity {
	return selectionRefIdentity{
		compositionID:    ref.compositionID,
		bindingAuthority: ref.bindingAuthority,
		factorKey:        ref.factorKey,
		factorIndex:      ref.factorIndex,
		raw:              uint64(ref.raw),
	}
}

func selectionRouteMatchesRef[K ~uint32 | ~uint64](route exactRef, expected Ref[K]) bool {
	if route == nil {
		return false
	}
	actual, ok := route.(selectionRefIdentityProvider)
	return ok && actual.selectionRefIdentity() == expected.selectionRefIdentity()
}

// derivationSelectedRead authenticates one selected-read capability through
// the sealed selected-read proof.
func derivationSelectedRead[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], read Read[Selection[Tag, S]]) bool {
	if derivation.proof == nil || !derivation.proof.valid() || read.index < 0 || read.resolve == nil || !read.matchesRuleProof(derivation.proof) {
		return false
	}
	selected := derivation.proof.selectedReadAt(uint64(read.index))
	return selected != nil && selected.Valid() && selected.read == uint64(read.index)
}

func derivationDispositionSelection[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]]) (Selection[Tag, S], int, bool) {
	ordinal := disposition.ordinal
	if !derivation.liveProduct() || ordinal < 0 || ordinal >= len(derivation.dispositions) || derivation.dispositions[ordinal].ordinal != ordinal ||
		derivation.dispositions[ordinal].row != disposition.row || disposition.row.ticket != derivation.ticket || disposition.row.index != ordinal || !derivationSelectedRead(derivation, read) {
		return Selection[Tag, S]{}, 0, false
	}
	id, found := derivation.product.readID(disposition.row.index, read.index)
	if !found {
		return Selection[Tag, S]{}, 0, false
	}
	selection, resolved := read.resolve(derivation.product, read.index, id)
	if !resolved || selection.count == nil {
		return Selection[Tag, S]{}, 0, false
	}
	return selection, disposition.row.index, true
}

// DerivationDispositionRouteValue resolves the exact tag/value pair that
// justified one atomic route output. It is deliberately narrower than a
// general Selection projection: the requested Read must be the Rule's one
// declared RouteWrite input, the disposition must belong to this live
// derivation row, and the output ordinal must name the same canonical route.
// Neither Ref, Unit, State, nor an alternate fact projection escapes.
func DerivationDispositionRouteValue[V any, O any, Tag selectionTag, S any](derivation RuleDerivation[V, O], disposition RuleDisposition[V], read Read[Selection[Tag, S]], output RuleOutput[V]) (Tag, S, bool) {
	var tag Tag
	var value S
	ordinal := disposition.ordinal
	if !derivation.liveProduct() || ordinal < 0 || ordinal >= len(derivation.dispositions) || derivation.dispositions[ordinal].ordinal != ordinal ||
		derivation.dispositions[ordinal].row != disposition.row || disposition.row.ticket != derivation.ticket || disposition.row.index != ordinal || !derivationSelectedRead(derivation, read) || output.ordinal < 0 {
		return tag, value, false
	}
	var routeRead uint64
	if derivation.proof != nil && derivation.proof.routeWrite != nil && derivation.proof.routeWrite.Valid() {
		routeRead = derivation.proof.routeWrite.read + 1
	}
	if routeRead == 0 || int(routeRead-1) != read.index {
		return tag, value, false
	}
	// A target and ordinal are not an output identity: another derivation may
	// lawfully stage the same target at the same ordinal with a distinct V.
	// The private witness is installed before the checker sees this derivation
	// and ties the public output handle to this exact ticket, row, and route
	// ordinal without asking V to be comparable.
	candidate, candidateOK := disposition.OutputAt(output.ordinal)
	if !candidateOK || candidate.ordinal != output.ordinal || !candidate.target.Same(output.target) ||
		candidate.witness.ticket == nil || candidate.witness != output.witness || candidate.witness.ticket != derivation.ticket ||
		candidate.witness.row != ordinal || candidate.witness.ordinal != output.ordinal {
		return tag, value, false
	}
	selection, row, resolved := derivationDispositionSelection(derivation, disposition, read)
	if !resolved || selection.route == nil {
		return tag, value, false
	}
	count, counted := selection.count(row)
	if !counted || output.ordinal >= count {
		return tag, value, false
	}
	return DerivationDispositionSelectionAt(derivation, disposition, read, output.ordinal)
}

// ruleReadProof is the compact sealed identity retained only for exact reads.
// Summary and selector runtimes carry the zero value and therefore cannot
// satisfy an exact Ref proof.
type ruleReadProof struct {
	schema           *Schema
	bindingAuthority *schemaBindingAuthority
	factorIndex      uint64
	raw              uint64
	exact            bool
}

func newRuleReadReceiptProof(receipt factorRuntimeReceipt, surface equation.Surface) (ruleReadProof, bool) {
	if !receipt.valid() || surface.Factor != receipt.semantic || surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 || surface.Local > receipt.keyEnd {
		return ruleReadProof{}, false
	}
	return ruleReadProof{schema: receipt.schema, bindingAuthority: receipt.authority, factorIndex: receipt.ordinal, raw: surface.Local - 1, exact: true}, true
}

// ruleSummaryReadProof is the private sealed identity of one summary Unit.
// The canonical key vector is retained for cold shape bookkeeping; hot
// evidence authenticates the sealed scalar digest and never replays it.
type ruleSummaryReadProof struct {
	receipt     factorRuntimeReceipt
	formReceipt factorFormReceipt
	keys        []uint64
	digest      [32]byte
}

func summaryProofMatchesRefs[K ~uint32 | ~uint64](proof ruleSummaryReadProof, refs *ClosedRefs[K]) bool {
	return proof.receipt.valid() && refs != nil && refs.closed && refs.validIssuer() && refs.receipt.state == proof.receipt.state && refs.receipt.authority == proof.receipt.authority && refs.receipt.schema == proof.receipt.schema && refs.receipt.ordinal == proof.receipt.ordinal && refs.receipt.semantic == proof.receipt.semantic && proof.formReceipt.kind == SchemaFormReadSummary && proof.formReceipt.semantic != (composition.Key{}) && len(refs.refs) == len(proof.keys) && refs.digest == proof.digest
}

// DerivationReadMatchesRef proves that one checker-visible typed read was
// bound to exactly the owner-issued Ref supplied by the domain. It inspects
// the live product's sealed read runtime; no coordinate, equation Surface, or
// carrier Unit is exposed or reconstructed.
func DerivationReadMatchesRef[V, O, S any, K ~uint32 | ~uint64](derivation RuleDerivation[V, O], read Read[S], ref Ref[K]) bool {
	if !derivation.liveProduct() || !read.matchesRuleProof(derivation.proof) ||
		read.index >= len(derivation.product.reads) || read.resolve == nil {
		return false
	}
	runtime := derivation.product.reads[read.index]
	if runtime == nil {
		return false
	}
	proof := runtime.exactProof()
	return proof.schema != nil && proof.exact && ref.bindingAuthority == proof.bindingAuthority && ref.compositionID == proof.schema.ID() && ref.factorKey == proof.schema.factorSemanticAt(proof.factorIndex) && ref.factorIndex == proof.factorIndex && proof.raw == uint64(ref.raw)
}

// DerivationReadMatchesSummaryRefs proves that one checker-visible typed
// summary read was bound to exactly the closed, owner-issued Ref vector
// supplied by the domain. The comparison remains inside the sealed runtime:
// it exposes neither coordinates nor the summary Unit, and accepts no
// alternate evidence path.
func DerivationReadMatchesSummaryRefs[V, O, S any, K ~uint32 | ~uint64](derivation RuleDerivation[V, O], read Read[S], refs *ClosedRefs[K]) bool {
	if !derivation.liveProduct() || !read.matchesRuleProof(derivation.proof) ||
		read.index >= len(derivation.product.reads) || read.resolve == nil {
		return false
	}
	runtime := derivation.product.reads[read.index]
	return runtime != nil && summaryProofMatchesRefs(runtime.summaryProof(), refs)
}

type ruleTargetProof struct {
	schema           *Schema
	state            *schemaBindingState
	bindingAuthority *schemaBindingAuthority
	factorIndex      uint64
	raw              uint64
	strong           bool
}

func newRuleTargetReceiptProof(receipt factorRuntimeReceipt, surface equation.Surface) (ruleTargetProof, bool) {
	if !receipt.valid() || surface.Factor != receipt.semantic || surface.Form != equation.SurfaceWriteExact || surface.Mode != equation.TargetModeStrong || surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 || surface.Local > receipt.keyEnd {
		return ruleTargetProof{}, false
	}
	return ruleTargetProof{schema: receipt.schema, state: receipt.state, bindingAuthority: receipt.authority, factorIndex: receipt.ordinal, raw: surface.Local - 1, strong: true}, true
}

func (proof ruleTargetProof) validOrigin() bool {
	return proof.schema != nil && proof.schema.Available() && proof.state != nil && proof.bindingAuthority != nil && proof.state.phase == schemaBindingSealed && proof.state.schema == proof.schema && proof.state.authority == proof.bindingAuthority && proof.factorIndex < proof.schema.factorCount() && proof.strong
}

// TargetMatchesRef proves that one checker-visible staged target is exactly
// the owner-issued Ref supplied by the domain. It compares only authenticated
// sealed surface identity; neither the raw coordinate nor the equation
// representation is exposed.
func TargetMatchesRef[K ~uint32 | ~uint64](target RuleTarget, ref Ref[K]) bool {
	if target.target == (carrier.Target{}) || !target.proof.validOrigin() {
		return false
	}
	return ref.bindingAuthority == target.proof.bindingAuthority && ref.compositionID == target.proof.schema.ID() && ref.factorKey == target.proof.schema.factorSemanticAt(target.proof.factorIndex) && ref.factorIndex == target.proof.factorIndex && target.proof.raw == uint64(ref.raw)
}

type ruleAdmissionSchema struct {
	kind     ruleAdmissionKind
	identity identity.SemanticKey
}

// ruleRuntimeProof is the sole private runtime identity of one sealed receipt
// Rule implementation. It names the SchemaBinding state, canonical ordinal,
// semantic shape, and sealed read/write receipts used by admission.
type ruleRuntimeProof struct {
	schema           *Schema
	state            *schemaBindingState
	bindingAuthority *schemaBindingAuthority
	ordinal          uint64
	semantic         composition.Key
	operandFamily    composition.Key
	admission        ruleAdmissionSchema
	outputKind       composition.OutputKind
	output           composition.Key
	inputs           uint64
	reads            uint64
	carries          uint64
	writes           uint64
	selectedReads    []*SchemaSelectedReadReceipt
	routeWrite       *SchemaRouteWriteReceipt
}

func (proof *ruleRuntimeProof) selectedReadAt(read uint64) *SchemaSelectedReadReceipt {
	if proof == nil || read >= uint64(len(proof.selectedReads)) {
		return nil
	}
	return proof.selectedReads[read]
}

// newSchemaRuleRuntimeProof issues the private proof from the exact shared
// SchemaBinding state and canonical Rule ordinal.
func newSchemaRuleRuntimeProof(state *schemaBindingState, authority *schemaBindingAuthority, ordinal uint64) (*ruleRuntimeProof, bool) {
	if state == nil || authority == nil || state.phase != schemaBindingSealed || state.authority != authority || state.schema == nil || ordinal >= uint64(len(state.rules)) {
		return nil, false
	}
	cell, ok := state.rules[ordinal].(schemaRuleBindingCell)
	if !ok || cell == nil || cell.schemaBindingSchema() != state.schema || cell.schemaRuleOrdinal() != ordinal || !cell.schemaRuleComplete() {
		return nil, false
	}
	shape, shapeOK := state.schema.ruleShapeAt(ordinal)
	if !shapeOK {
		return nil, false
	}
	admission, admitted := coldRuleAdmission(shape.Admission)
	if !admitted {
		return nil, false
	}
	proof := &ruleRuntimeProof{
		schema: state.schema, state: state, bindingAuthority: authority,
		ordinal: ordinal, semantic: state.schema.ruleSemanticAt(ordinal),
		operandFamily: shape.OperandFamily, admission: admission, outputKind: shape.OutputKind,
		output: shape.Output, inputs: shape.Inputs, reads: shape.ReadCount,
		carries: shape.CarryCount, writes: shape.WriteCount,
	}
	if shape.ReadCount > uint64(^uint(0)>>1) {
		return nil, false
	}
	proof.selectedReads = make([]*SchemaSelectedReadReceipt, int(shape.ReadCount))
	fence := schemaRuleReceiptFence{state: state, authority: authority, schema: state.schema, rule: ordinal}
	if ordinal < uint64(len(state.rules)) {
		fence.cell, _ = state.rules[ordinal].(schemaRuleBindingCell)
	}
	for read := uint64(0); read < shape.ReadCount; read++ {
		readShape, readOK := state.schema.ruleReadShapeAt(ordinal, read)
		if !readOK {
			return nil, false
		}
		if readShape.Kind == composition.ReadSelect {
			receipt, receiptOK := issueSchemaSelectedReadReceiptFence(fence, fence.valid(), read)
			if !receiptOK {
				return nil, false
			}
			proof.selectedReads[read] = &receipt
		}
	}
	for write := uint64(0); write < shape.WriteCount; write++ {
		writeShape, writeOK := state.schema.ruleWriteShapeAt(ordinal, write)
		if !writeOK {
			return nil, false
		}
		switch writeShape.Kind {
		case composition.WriteRoute:
			receipt, receiptOK := issueSchemaRouteWriteReceiptFence(fence, fence.valid(), write)
			if !receiptOK {
				return nil, false
			}
			proof.routeWrite = &receipt
		}
	}
	if !proof.valid() {
		return nil, false
	}
	return proof, true
}

func (proof *ruleRuntimeProof) valid() bool {
	if proof == nil || proof.schema == nil || !proof.schema.Available() || !proof.semantic.Available() || !proof.operandFamily.Available() || !proof.admission.valid() || proof.ordinal >= uint64(schemaRuleCount(proof.schema)) || proof.schema.ruleSemanticAt(proof.ordinal) != proof.semantic {
		return false
	}
	shape, ok := proof.schema.ruleShapeAt(proof.ordinal)
	admission, admitted := coldRuleAdmission(shape.Admission)
	if !ok || !admitted || shape.OperandFamily != proof.operandFamily || admission != proof.admission || shape.OutputKind != proof.outputKind || shape.Output != proof.output || shape.Inputs != proof.inputs || shape.ReadCount != proof.reads || shape.CarryCount != proof.carries || shape.WriteCount != proof.writes {
		return false
	}
	if proof.state == nil || proof.bindingAuthority == nil || proof.state.phase != schemaBindingSealed || proof.state.authority != proof.bindingAuthority || proof.state.schema != proof.schema || proof.ordinal >= uint64(len(proof.state.rules)) {
		return false
	}
	cell, ok := proof.state.rules[proof.ordinal].(schemaRuleBindingCell)
	if !ok || cell == nil || cell.schemaBindingSchema() != proof.schema || cell.schemaRuleOrdinal() != proof.ordinal || !cell.schemaRuleComplete() || !cell.schemaRuleProofMatches(proof) {
		return false
	}
	if uint64(len(proof.selectedReads)) != proof.reads {
		return false
	}
	for read := uint64(0); read < proof.reads; read++ {
		shape, shapeOK := proof.schema.ruleReadShapeAt(proof.ordinal, read)
		if !shapeOK {
			return false
		}
		if shape.Kind == composition.ReadSelect {
			selectedRead := proof.selectedReadAt(read)
			if selectedRead == nil || !selectedRead.Valid() || selectedRead.fence.state != proof.state || selectedRead.fence.authority != proof.bindingAuthority || selectedRead.fence.rule != proof.ordinal || selectedRead.read != read {
				return false
			}
		} else if proof.selectedReads[read] != nil {
			return false
		}
	}
	for write := uint64(0); write < proof.writes; write++ {
		shape, shapeOK := proof.schema.ruleWriteShapeAt(proof.ordinal, write)
		if !shapeOK {
			return false
		}
		switch shape.Kind {
		case composition.WriteRoute:
			if proof.routeWrite == nil || !proof.routeWrite.Valid() || proof.routeWrite.fence.state != proof.state || proof.routeWrite.fence.authority != proof.bindingAuthority || proof.routeWrite.fence.rule != proof.ordinal || proof.routeWrite.write != write {
				return false
			}
		}
	}
	return true
}

func (proof *ruleRuntimeProof) compositionID() CompositionID {
	if proof == nil || !proof.valid() {
		return CompositionID{}
	}
	return proof.schema.ID()
}

func schemaRuleCount(schema *Schema) int {
	if schema == nil {
		return 0
	}
	_, rules, _, _, ok := schema.shapeCount()
	if !ok {
		return 0
	}
	return rules
}

func (schema ruleAdmissionSchema) valid() bool {
	return schema.identity.Available() && (schema.kind == ruleAdmissionTrustedTheorem || schema.kind == ruleAdmissionDerivation)
}
