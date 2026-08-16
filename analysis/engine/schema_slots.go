package engine

// This file is the callback-free declaration surface for the reusable cold
// schema. It is intentionally independent of Factor algebra, executable
// transfer functions, key domains, and runtime coordinates. The one-shot
// builder is the sole public construction path.

import (
	"sort"

	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

const schemaSlotMax = uint64(^uint32(0))

func schemaSlotCardinality(count int) bool {
	return count >= 0 && uint64(count) <= schemaSlotMax
}

// SchemaBuilder is a single-use, callback-free cold-schema candidate.  Its
// declaration rows are discarded after Seal; the returned Schema is the only
// retained semantic authority.
type SchemaBuilder struct {
	phase      schemaBuilderPhase
	candidate  coldcomposition.Candidate
	sealed     *Schema
	factors    []*schemaFactorDraft
	rules      []*schemaRuleDraft
	queries    []*schemaQueryDraft
	forms      []*schemaFormDraft
	reads      []*schemaReadDraft
	writes     []*schemaWriteDraft
	carries    []*schemaCarryDraft
	inputs     []*schemaInputDraft
	families   []*schemaFamilyDraft
	tokens     []*schemaTokenCell
	completion *schemaCompletionCell
	claims     map[SemanticKey]struct{}
}

type schemaBuilderPhase uint8

const (
	schemaBuilderFactors schemaBuilderPhase = iota + 1
	schemaBuilderChildren
	schemaBuilderSealed
	schemaBuilderPoisoned
)

// NewSchema opens an empty callback-free cold-schema builder.
func NewSchema() *SchemaBuilder {
	return &SchemaBuilder{phase: schemaBuilderFactors, claims: make(map[SemanticKey]struct{})}
}

// claim reserves each public semantic identity exactly once before its row is
// appended. References (operand families, admissions, route families) are not
// claims; Factors, optional forms, completion/prune, activation families,
// Rules, carry transforms, and Query/freezer rows are.
func (builder *SchemaBuilder) claim(keys ...SemanticKey) bool {
	if builder == nil || builder.phase == schemaBuilderPoisoned || builder.phase == schemaBuilderSealed || len(keys) == 0 {
		return false
	}
	for index, key := range keys {
		if !key.Available() {
			builder.poison()
			return false
		}
		if _, exists := builder.claims[key]; exists {
			builder.poison()
			return false
		}
		for prior := 0; prior < index; prior++ {
			if keys[prior] == key {
				builder.poison()
				return false
			}
		}
	}
	for _, key := range keys {
		builder.claims[key] = struct{}{}
	}
	return true
}

// SchemaAdmission is the callback-free provenance row for one Rule.  The
// identity is semantic evidence, not an executable checker.
type SchemaAdmission struct {
	Basis    RuleAdmissionBasis
	Identity SemanticKey
}

func (admission SchemaAdmission) cold() (coldcomposition.Admission, bool) {
	if !admission.Identity.Available() {
		return coldcomposition.Admission{}, false
	}
	var kind coldcomposition.AdmissionKind
	switch admission.Basis {
	case RuleAdmissionBasisTrustedTheorem:
		kind = coldcomposition.AdmissionTrustedTheorem
	case RuleAdmissionBasisDerivation:
		kind = coldcomposition.AdmissionDerivation
	default:
		return coldcomposition.Admission{}, false
	}
	return coldcomposition.Admission{Kind: kind, Identity: admission.Identity.compositionKey()}, true
}

// FactorRef is a typed output/reference projection of a FactorSlot.  It
// carries no algebra and is valid only against the exact builder or Schema
// that issued it.
type FactorRef[V any] struct {
	cell *schemaTokenCell
}

func (ref FactorRef[V]) validBuilder(builder *SchemaBuilder) bool {
	draft, ok := tokenDraft[schemaFactorDraft](ref.cell)
	return ok && builder != nil && draft.builder == builder && builder.phase != schemaBuilderPoisoned && builder.phase != schemaBuilderSealed
}

func (ref FactorRef[V]) validSchema(schema *Schema) bool {
	return schema != nil && schema.Available() && ref.cell != nil && ref.cell.schema == schema
}

// FactorSlot is a typed Factor owner handle.  Before Seal it is a draft
// reference; Seal rewrites it to exactly (Schema pointer, canonical ordinal).
type FactorSlot[T any] struct {
	cell *schemaTokenCell
}

func (slot *FactorSlot[T]) Ref() FactorRef[T] {
	if slot == nil {
		return FactorRef[T]{}
	}
	return FactorRef[T]{cell: slot.cell}
}

func (slot *FactorSlot[T]) Available() bool {
	return slot != nil && slot.cell != nil && (slot.cell.schema != nil || slot.cell.builder != nil && slot.cell.builder.phase != schemaBuilderPoisoned && slot.cell.builder.phase != schemaBuilderSealed)
}

func (slot *FactorSlot[T]) Schema() *Schema {
	if slot == nil || slot.cell == nil || slot.cell.schema == nil {
		return nil
	}
	return slot.cell.schema
}

func (slot *FactorSlot[T]) Ordinal() (uint64, bool) {
	if slot.cell != nil && slot.cell.schema != nil && slot.cell.schema.Available() {
		return slot.cell.ordinal, true
	}
	return 0, false
}

// SchemaReadForm and SchemaWriteForm are typed form capabilities.  They are
// only schema shapes; no normalizer, callback, key end, or runtime slot is
// retained.  Their concrete capabilities are issued by FactorSlot methods.
type SchemaReadForm[T any] struct {
	cell *schemaTokenCell
}

type SchemaWriteForm[T any] struct {
	cell *schemaTokenCell
}

func (form SchemaReadForm[T]) Kind() SchemaFormKind {
	if form.cell == nil {
		return 0
	}
	return SchemaFormKind(form.cell.kind)
}
func (form SchemaWriteForm[T]) Kind() SchemaFormKind {
	if form.cell == nil {
		return 0
	}
	return SchemaFormKind(form.cell.kind)
}

func (form SchemaReadForm[T]) valid(builder *SchemaBuilder) bool {
	draft, ok := tokenDraft[schemaFormDraft](form.cell)
	return ok && builder != nil && builder.phase != schemaBuilderPoisoned && builder.phase != schemaBuilderSealed && draft.builder == builder && draft.read &&
		(draft.formKind == SchemaFormReadExact || draft.formKind == SchemaFormReadSummary)
}

func (form SchemaWriteForm[T]) valid(builder *SchemaBuilder) bool {
	draft, ok := tokenDraft[schemaFormDraft](form.cell)
	return ok && builder != nil && builder.phase != schemaBuilderPoisoned && builder.phase != schemaBuilderSealed && draft.builder == builder && !draft.read &&
		(draft.formKind == SchemaFormWriteExact || draft.formKind == SchemaFormWriteSelector)
}

func (form SchemaReadForm[T]) Schema() *Schema {
	if form.cell == nil {
		return nil
	}
	return form.cell.schema
}
func (form SchemaWriteForm[T]) Schema() *Schema {
	if form.cell == nil {
		return nil
	}
	return form.cell.schema
}

func (slot *FactorSlot[T]) ExactRead() (SchemaReadForm[T], bool) {
	return slot.addReadForm(SchemaFormReadExact, SemanticKey{})
}

func (slot *FactorSlot[T]) SummaryRead(semantic SemanticKey) (SchemaReadForm[T], bool) {
	return slot.addReadForm(SchemaFormReadSummary, semantic)
}

func (slot *FactorSlot[T]) ExactWrite() (SchemaWriteForm[T], bool) {
	return slot.addWriteForm(SchemaFormWriteExact, SemanticKey{})
}

func (slot *FactorSlot[T]) SelectorWrite(semantic SemanticKey) (SchemaWriteForm[T], bool) {
	return slot.addWriteForm(SchemaFormWriteSelector, semantic)
}

func (slot *FactorSlot[T]) addReadForm(kind SchemaFormKind, semantic SemanticKey) (SchemaReadForm[T], bool) {
	draft, ok := factorDraft(slot)
	if !ok || draft.builder.phase != schemaBuilderFactors {
		return SchemaReadForm[T]{}, false
	}
	form, ok := draft.builder.addForm(draft, true, kind, semantic)
	if !ok {
		return SchemaReadForm[T]{}, false
	}
	result := SchemaReadForm[T]{cell: &schemaTokenCell{builder: draft.builder, draft: form, kind: kind}}
	draft.builder.tokens = append(draft.builder.tokens, result.cell)
	form.token = result.cell
	return result, true
}

func (slot *FactorSlot[T]) addWriteForm(kind SchemaFormKind, semantic SemanticKey) (SchemaWriteForm[T], bool) {
	draft, ok := factorDraft(slot)
	if !ok || draft.builder.phase != schemaBuilderFactors {
		return SchemaWriteForm[T]{}, false
	}
	form, ok := draft.builder.addForm(draft, false, kind, semantic)
	if !ok {
		return SchemaWriteForm[T]{}, false
	}
	result := SchemaWriteForm[T]{cell: &schemaTokenCell{builder: draft.builder, draft: form, kind: kind}}
	draft.builder.tokens = append(draft.builder.tokens, result.cell)
	form.token = result.cell
	return result, true
}

// DeclareFactorSlot adds one typed Factor schema without asking for algebra
// callbacks or a key-domain enumeration.
func DeclareFactorSlot[T any](builder *SchemaBuilder, semantic SemanticKey) (*FactorSlot[T], bool) {
	if builder == nil || builder.phase != schemaBuilderFactors || !semantic.Available() {
		if builder != nil {
			builder.poison()
		}
		return nil, false
	}
	if !builder.claim(semantic) {
		return nil, false
	}
	index := len(builder.candidate.Factors)
	if !schemaSlotCardinality(index) {
		builder.poison()
		return nil, false
	}
	draft := &schemaFactorDraft{builder: builder, index: index, semantic: semantic}
	builder.candidate.Factors = append(builder.candidate.Factors, coldcomposition.Factor{Key: semantic.compositionKey()})
	builder.factors = append(builder.factors, draft)
	slot := &FactorSlot[T]{cell: &schemaTokenCell{builder: builder, draft: draft}}
	builder.tokens = append(builder.tokens, slot.cell)
	return slot, true
}

// FactorSlot is a concise alias for DeclareFactorSlot at call sites that use
// the new API's role vocabulary.
func NewFactorSlot[T any](builder *SchemaBuilder, semantic SemanticKey) (*FactorSlot[T], bool) {
	return DeclareFactorSlot[T](builder, semantic)
}

// SchemaFormKind is closed directional form evidence. Exact reads and writes
// deliberately have distinct capabilities even though both use the factor's
// base cold identity.
type SchemaFormKind uint8

const (
	SchemaFormInvalid SchemaFormKind = iota
	SchemaFormReadExact
	SchemaFormReadSummary
	SchemaFormWriteExact
	SchemaFormWriteSelector
)

// schemaTokenCell is the shared mutable identity cell behind every issued
// value token. Copies of a token therefore observe the same post-Seal owner
// and ordinal. Once bound, all declaration storage is dropped from the cell.
type schemaTokenCell struct {
	builder *SchemaBuilder
	schema  *Schema
	ordinal uint64
	draft   any
	kind    SchemaFormKind
}

func tokenDraft[T any](cell *schemaTokenCell) (*T, bool) {
	if cell == nil || cell.draft == nil {
		return nil, false
	}
	draft, ok := cell.draft.(*T)
	return draft, ok
}

func (cell *schemaTokenCell) bind(schema *Schema, ordinal uint64) {
	if cell == nil {
		return
	}
	cell.schema, cell.ordinal = schema, ordinal
	cell.builder, cell.draft = nil, nil
}

func factorDraft[T any](slot *FactorSlot[T]) (*schemaFactorDraft, bool) {
	if slot == nil {
		return nil, false
	}
	return tokenDraft[schemaFactorDraft](slot.cell)
}

type schemaFactorDraft struct {
	builder  *SchemaBuilder
	index    int
	semantic SemanticKey
}

type schemaFormDraft struct {
	builder   *SchemaBuilder
	factor    *schemaFactorDraft
	semantic  SemanticKey
	formKind  SchemaFormKind
	read      bool
	canonical uint64
	token     *schemaTokenCell
}

func (builder *SchemaBuilder) addForm(factor *schemaFactorDraft, read bool, kind SchemaFormKind, semantic SemanticKey) (*schemaFormDraft, bool) {
	if builder == nil || factor == nil || factor.builder != builder || factor.index < 0 || factor.index >= len(builder.candidate.Factors) {
		builder.poison()
		return nil, false
	}
	if !schemaSlotCardinality(len(builder.forms)) ||
		(read && kind != SchemaFormReadExact && kind != SchemaFormReadSummary) ||
		(!read && kind != SchemaFormWriteExact && kind != SchemaFormWriteSelector) {
		builder.poison()
		return nil, false
	}
	if kind != SchemaFormReadExact && kind != SchemaFormWriteExact && !semantic.Available() {
		builder.poison()
		return nil, false
	}
	if kind == SchemaFormReadExact || kind == SchemaFormWriteExact {
		semantic = semanticKeyFromComposition(builder.candidate.Factors[factor.index].Key)
	}
	for _, prior := range builder.forms {
		if prior.factor == factor && prior.read == read && prior.formKind == kind && prior.semantic == semantic {
			builder.poison()
			return nil, false
		}
	}
	if kind != SchemaFormReadExact && kind != SchemaFormWriteExact && !builder.claim(semantic) {
		return nil, false
	}
	form := &schemaFormDraft{builder: builder, factor: factor, semantic: semantic, formKind: kind, read: read}
	builder.forms = append(builder.forms, form)
	rowKind := coldcomposition.FactorSummaryRead
	if kind == SchemaFormWriteSelector {
		rowKind = coldcomposition.FactorSelectorWrite
	}
	if kind != SchemaFormReadExact && kind != SchemaFormWriteExact {
		builder.candidate.Factors[factor.index].Forms = append(builder.candidate.Factors[factor.index].Forms, coldcomposition.FactorForm{Kind: rowKind, Semantic: semantic.compositionKey()})
	}
	return form, true
}

// SchemaInput is a typed predecessor port.  It is not a concrete point or
// equation coordinate.
type SchemaInput struct {
	cell *schemaTokenCell
}

func (input SchemaInput) validBuilder(builder *SchemaBuilder, rule *schemaRuleDraft) bool {
	draft, ok := tokenDraft[schemaInputDraft](input.cell)
	return ok && builder != nil && builder.phase == schemaBuilderChildren && rule != nil && draft.builder == builder && draft.rule == rule
}

// SchemaRuleSpec declares one Factor-output Rule.  Structural Rules use the
// dedicated support/activation constructors below.
type SchemaRuleSpec[V any] struct {
	Semantic      SemanticKey
	OperandFamily SemanticKey
	Inputs        uint64
	Admission     SchemaAdmission
	Output        FactorRef[V]
}

type RuleSlot[V, O any] struct {
	cell *schemaTokenCell
}

func (slot *RuleSlot[V, O]) Available() bool {
	return slot != nil && slot.cell != nil && (slot.cell.schema != nil || slot.cell.builder != nil && slot.cell.builder.phase != schemaBuilderPoisoned && slot.cell.builder.phase != schemaBuilderSealed)
}
func (slot *RuleSlot[V, O]) Schema() *Schema {
	if slot == nil || slot.cell == nil {
		return nil
	}
	return slot.cell.schema
}

func (slot *RuleSlot[V, O]) Ordinal() (uint64, bool) {
	if slot.cell != nil && slot.cell.schema != nil && slot.cell.schema.Available() {
		return slot.cell.ordinal, true
	}
	return 0, false
}

type schemaRuleDraft struct {
	builder    *SchemaBuilder
	index      int
	inputs     []*schemaInputDraft
	output     *schemaFactorDraft
	structural bool
	route      bool
}

// factorOutput is the exact disposition for a normal rule. Structural rules
// have no output capability and may never acquire writes, carries, routes, or
// selectors through the generic factor path.
func (rule *schemaRuleDraft) factorOutput() bool {
	return rule != nil && !rule.structural && rule.output != nil
}

type schemaInputDraft struct {
	builder *SchemaBuilder
	rule    *schemaRuleDraft
	index   int
	token   *schemaTokenCell
}

// DeclareRuleSlot adds a callback-free Factor-output Rule.
func DeclareRuleSlot[V, O any](builder *SchemaBuilder, spec SchemaRuleSpec[V]) (*RuleSlot[V, O], bool) {
	outputDraft, outputOK := tokenDraft[schemaFactorDraft](spec.Output.cell)
	if builder == nil || builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren || !spec.Semantic.Available() || !spec.OperandFamily.Available() || !outputOK || !spec.Output.validBuilder(builder) {
		if builder != nil {
			builder.poison()
		}
		return nil, false
	}
	builder.phase = schemaBuilderChildren
	admission, ok := spec.Admission.cold()
	if !ok {
		builder.poison()
		return nil, false
	}
	if !builder.claim(spec.Semantic) {
		return nil, false
	}
	index := len(builder.candidate.Rules)
	if !schemaSlotCardinality(index) || spec.Inputs > schemaSlotMax {
		builder.poison()
		return nil, false
	}
	draft := &schemaRuleDraft{builder: builder, index: index, output: outputDraft}
	builder.candidate.Rules = append(builder.candidate.Rules, coldcomposition.Rule{Key: spec.Semantic.compositionKey(), OperandFamily: spec.OperandFamily.compositionKey(), Admission: admission, OutputKind: coldcomposition.FactorOutput, Output: outputDraft.semantic.compositionKey(), Inputs: spec.Inputs})
	builder.rules = append(builder.rules, draft)
	slot := &RuleSlot[V, O]{cell: &schemaTokenCell{builder: builder, draft: draft}}
	builder.tokens = append(builder.tokens, slot.cell)
	return slot, true
}

// NewRuleSlot is the concise constructor spelling for DeclareRuleSlot.
func NewRuleSlot[V, O any](builder *SchemaBuilder, spec SchemaRuleSpec[V]) (*RuleSlot[V, O], bool) {
	return DeclareRuleSlot[V, O](builder, spec)
}

func (slot *RuleSlot[V, O]) Input(index uint64) (SchemaInput, bool) {
	rule, ok := ruleDraft(slot)
	if !ok || index > uint64(int(^uint(0)>>1)) || rule.builder.phase != schemaBuilderChildren || index >= uint64(ruleInputs(rule)) || !schemaSlotCardinality(len(rule.inputs)) {
		return SchemaInput{}, false
	}
	for _, prior := range rule.inputs {
		if uint64(prior.index) == index {
			for _, cell := range rule.builder.tokens {
				if draft, ok := tokenDraft[schemaInputDraft](cell); ok && draft == prior {
					return SchemaInput{cell: cell}, true
				}
			}
			return SchemaInput{}, false
		}
	}
	input := &schemaInputDraft{builder: rule.builder, rule: rule, index: int(index)}
	rule.inputs = append(rule.inputs, input)
	rule.builder.inputs = append(rule.builder.inputs, input)
	result := SchemaInput{cell: &schemaTokenCell{builder: rule.builder, draft: input}}
	rule.builder.tokens = append(rule.builder.tokens, result.cell)
	input.token = result.cell
	return result, true
}

func (slot *RuleSlot[V, O]) ruleInputs() int {
	rule, ok := ruleDraft(slot)
	if !ok {
		return -1
	}
	return ruleInputs(rule)
}

func ruleDraft[V, O any](slot *RuleSlot[V, O]) (*schemaRuleDraft, bool) {
	if slot == nil {
		return nil, false
	}
	return tokenDraft[schemaRuleDraft](slot.cell)
}

func ruleInputs(rule *schemaRuleDraft) int {
	if rule == nil || rule.builder == nil || rule.index < 0 || rule.index >= len(rule.builder.candidate.Rules) {
		return -1
	}
	return int(rule.builder.candidate.Rules[rule.index].Inputs)
}

func canonicalReadPredecessors(rule *schemaRuleDraft, refs []SchemaReadRef) ([]uint64, bool) {
	if rule == nil || rule.builder == nil || len(refs) == 0 || !schemaSlotCardinality(len(refs)) {
		return nil, false
	}
	result := make([]uint64, len(refs))
	for index, ref := range refs {
		read, ok := tokenDraft[schemaReadDraft](ref.cell)
		if !ok || read.rule != rule || read.index < 0 || read.index >= len(rule.builder.candidate.Rules[rule.index].Reads) {
			return nil, false
		}
		result[index] = uint64(read.index)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	for index := 1; index < len(result); index++ {
		if result[index-1] == result[index] {
			return nil, false
		}
	}
	return result, true
}

func canonicalDependencies(rule *schemaRuleDraft, refs []SchemaDependency) ([]coldcomposition.Dependency, bool) {
	if rule == nil || rule.builder == nil || !schemaSlotCardinality(len(refs)) {
		return nil, false
	}
	result := make([]coldcomposition.Dependency, len(refs))
	for index, ref := range refs {
		if read, ok := tokenDraft[schemaReadDraft](ref.cell); ok {
			if read.rule != rule || read.index < 0 || read.index >= len(rule.builder.candidate.Rules[rule.index].Reads) {
				return nil, false
			}
			result[index] = coldcomposition.Dependency{Index: uint64(read.index)}
			continue
		}
		write, ok := tokenDraft[schemaWriteDraft](ref.cell)
		if !ok || write.rule != rule || write.index < 0 || write.index >= len(rule.builder.candidate.Rules[rule.index].Writes) {
			return nil, false
		}
		result[index] = coldcomposition.Dependency{Target: true, Index: uint64(write.index)}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Target != result[right].Target {
			return !result[left].Target
		}
		return result[left].Index < result[right].Index
	})
	for index := 1; index < len(result); index++ {
		if result[index-1] == result[index] {
			return nil, false
		}
	}
	return result, true
}

// SchemaReadSlot is one callback-free ordered Rule read capability.
type SchemaReadSlot[T any] struct {
	cell *schemaTokenCell
}

func (slot SchemaReadSlot[T]) Schema() *Schema {
	if slot.cell == nil {
		return nil
	}
	return slot.cell.schema
}

// SchemaReadRef is an opaque same-rule predecessor capability. It never
// exposes a cold row index.
type SchemaReadRef struct{ cell *schemaTokenCell }

func (slot SchemaReadSlot[T]) Ref() SchemaReadRef { return SchemaReadRef{cell: slot.cell} }

// Dependency returns opaque predecessor evidence for this read. A write slot
// exposes the same narrow capability below; both are accepted only by the
// exact rule that issued them.
func (slot SchemaReadSlot[T]) Dependency() SchemaDependency { return SchemaDependency{cell: slot.cell} }

type schemaReadDraft struct {
	builder *SchemaBuilder
	rule    *schemaRuleDraft
	index   int
	token   *schemaTokenCell
}

func SchemaRead[T, V, O any](rule *RuleSlot[V, O], form SchemaReadForm[T], input SchemaInput) (SchemaReadSlot[T], bool) {
	ruleDraft, ok := ruleDraft(rule)
	return appendSchemaRead(ruleDraft, ok, form, input)
}

// SchemaReadAs binds an exact source Factor form to a typed ordered-cell
// observation. The source form owns the Factor element type V; T is the
// immutable observation representation consumed by the later hot binder.
// This keeps selected-read predecessor rows typed as OrderedCells[V] without
// pretending that the callback-free Schema form itself carries that wrapper.
func SchemaReadAs[T, V, O any](rule *RuleSlot[V, O], form SchemaReadForm[V], input SchemaInput) (SchemaReadSlot[T], bool) {
	ruleDraft, ok := ruleDraft(rule)
	if !ok {
		return SchemaReadSlot[T]{}, false
	}
	base, ok := appendSchemaRead(ruleDraft, true, form, input)
	if !ok {
		return SchemaReadSlot[T]{}, false
	}
	return SchemaReadSlot[T]{cell: base.cell}, true
}

// SchemaSupportRead attaches a typed Factor projection to the exact opaque
// structural support Rule.  The structural hot operand remains engine-owned.
func SchemaSupportRead[T any](rule *SchemaSupportRuleSlot, form SchemaReadForm[T], input SchemaInput) (SchemaReadSlot[T], bool) {
	ruleDraft, ok := tokenDraft[schemaRuleDraft](slotCell(rule))
	return appendSchemaRead(ruleDraft, ok, form, input)
}

// SchemaActivationRead attaches a typed Factor projection to the exact opaque
// structural activation Rule.  It never exports ActivationResult/ruleUnit.
func SchemaActivationRead[T any](rule *SchemaActivationRuleSlot, form SchemaReadForm[T], input SchemaInput) (SchemaReadSlot[T], bool) {
	ruleDraft, ok := tokenDraft[schemaRuleDraft](slotCell(rule))
	return appendSchemaRead(ruleDraft, ok, form, input)
}

func appendSchemaRead[T any](ruleDraft *schemaRuleDraft, ok bool, form SchemaReadForm[T], input SchemaInput) (SchemaReadSlot[T], bool) {
	if !ok || !form.valid(ruleDraft.builder) || !input.validBuilder(ruleDraft.builder, ruleDraft) {
		return SchemaReadSlot[T]{}, false
	}
	inputDraft, _ := tokenDraft[schemaInputDraft](input.cell)
	formDraft, _ := tokenDraft[schemaFormDraft](form.cell)
	if inputDraft.index < 0 || inputDraft.index >= ruleInputs(ruleDraft) {
		return SchemaReadSlot[T]{}, false
	}
	builder := ruleDraft.builder
	index := len(builder.candidate.Rules[ruleDraft.index].Reads)
	if !schemaSlotCardinality(index) {
		builder.poison()
		return SchemaReadSlot[T]{}, false
	}
	row := coldcomposition.Read{Input: uint64(inputDraft.index), Factor: formDraft.factor.semantic.compositionKey()}
	if formDraft.formKind == SchemaFormReadSummary {
		row.Kind = coldcomposition.ReadSummary
		row.Semantic = formDraft.semantic.compositionKey()
		row.Normalizer = row.Semantic
	} else {
		row.Kind = coldcomposition.ReadExact
	}
	read := &schemaReadDraft{builder: builder, rule: ruleDraft, index: index}
	builder.reads = append(builder.reads, read)
	builder.candidate.Rules[ruleDraft.index].Reads = append(builder.candidate.Rules[ruleDraft.index].Reads, row)
	result := SchemaReadSlot[T]{cell: &schemaTokenCell{builder: builder, draft: read}}
	builder.tokens = append(builder.tokens, result.cell)
	read.token = result.cell
	return result, true
}

// SchemaSelectedRead adds a staged exact read and its ordered predecessor
// dependencies. Dependencies are read ordinals in the same Rule.
func SchemaSelectedRead[T, FV, V, O any](rule *RuleSlot[V, O], form SchemaReadForm[FV], input SchemaInput, dependencies ...SchemaReadRef) (SchemaReadSlot[T], bool) {
	formDraft, formOK := tokenDraft[schemaFormDraft](form.cell)
	ruleDraft, ruleOK := ruleDraft(rule)
	if !formOK || !ruleOK || formDraft.formKind != SchemaFormReadExact || len(dependencies) == 0 || !input.validBuilder(ruleDraft.builder, ruleDraft) {
		return SchemaReadSlot[T]{}, false
	}
	predecessors, ok := canonicalReadPredecessors(ruleDraft, dependencies)
	if !ok {
		return SchemaReadSlot[T]{}, false
	}
	base, ok := appendSchemaRead(ruleDraft, true, form, input)
	if !ok {
		return SchemaReadSlot[T]{}, false
	}
	read := SchemaReadSlot[T]{cell: base.cell}
	readDraft, _ := tokenDraft[schemaReadDraft](read.cell)
	row := &ruleDraft.builder.candidate.Rules[ruleDraft.index].Reads[readDraft.index]
	row.Kind = coldcomposition.ReadSelect
	row.Semantic = row.Factor
	row.Dependencies = predecessors
	return SchemaReadSlot[T]{cell: read.cell}, true
}

type SchemaWriteSlot[V any] struct {
	cell *schemaTokenCell
}

func (slot SchemaWriteSlot[V]) Schema() *Schema {
	if slot.cell == nil {
		return nil
	}
	return slot.cell.schema
}

type schemaWriteDraft struct {
	builder *SchemaBuilder
	rule    *schemaRuleDraft
	index   int
	token   *schemaTokenCell
}

func SchemaWrite[V, O any](rule *RuleSlot[V, O], form SchemaWriteForm[V]) (SchemaWriteSlot[V], bool) {
	formDraft, formOK := tokenDraft[schemaFormDraft](form.cell)
	if !formOK || formDraft.formKind != SchemaFormWriteExact {
		return SchemaWriteSlot[V]{}, false
	}
	return appendSchemaWrite(rule, form)
}

func appendSchemaWrite[V, O any](rule *RuleSlot[V, O], form SchemaWriteForm[V]) (SchemaWriteSlot[V], bool) {
	ruleDraft, ok := ruleDraft(rule)
	formDraft, formOK := tokenDraft[schemaFormDraft](form.cell)
	if !ok || !ruleDraft.factorOutput() || ruleDraft.route || !formOK || !form.valid(ruleDraft.builder) || formDraft.factor != ruleDraft.output || formDraft.formKind != SchemaFormWriteExact && formDraft.formKind != SchemaFormWriteSelector {
		return SchemaWriteSlot[V]{}, false
	}
	builder := ruleDraft.builder
	index := len(builder.candidate.Rules[ruleDraft.index].Writes)
	if !schemaSlotCardinality(index) {
		builder.poison()
		return SchemaWriteSlot[V]{}, false
	}
	row := coldcomposition.Write{Factor: formDraft.factor.semantic.compositionKey()}
	if formDraft.formKind == SchemaFormWriteSelector {
		row.Kind = coldcomposition.WriteSelect
		row.Semantic = formDraft.semantic.compositionKey()
	} else {
		row.Kind = coldcomposition.WriteExact
	}
	draft := &schemaWriteDraft{builder: builder, rule: ruleDraft, index: index}
	builder.writes = append(builder.writes, draft)
	builder.candidate.Rules[ruleDraft.index].Writes = append(builder.candidate.Rules[ruleDraft.index].Writes, row)
	result := SchemaWriteSlot[V]{cell: &schemaTokenCell{builder: builder, draft: draft}}
	builder.tokens = append(builder.tokens, result.cell)
	draft.token = result.cell
	return result, true
}

func SchemaRouteWrite[V, O, T any](rule *RuleSlot[V, O], form SchemaWriteForm[V], read SchemaReadSlot[T]) (SchemaWriteSlot[V], bool) {
	formDraft, formOK := tokenDraft[schemaFormDraft](form.cell)
	ruleDraft, ruleOK := ruleDraft(rule)
	readDraft, readOK := tokenDraft[schemaReadDraft](read.cell)
	if !formOK || !ruleOK || !readOK || !ruleDraft.factorOutput() || ruleDraft.route || len(ruleDraft.builder.candidate.Rules[ruleDraft.index].Writes) != 0 || formDraft.formKind != SchemaFormWriteExact || readDraft.rule != ruleDraft || readDraft.index < 0 || readDraft.index >= len(ruleDraft.builder.candidate.Rules[ruleDraft.index].Reads) || ruleDraft.builder.candidate.Rules[ruleDraft.index].Reads[readDraft.index].Kind != coldcomposition.ReadSelect || ruleDraft.builder.candidate.Rules[ruleDraft.index].Reads[readDraft.index].Factor != ruleDraft.output.semantic.compositionKey() || ruleDraft.builder.phase != schemaBuilderChildren {
		return SchemaWriteSlot[V]{}, false
	}
	write, ok := appendSchemaWrite[V](rule, form)
	if !ok {
		return SchemaWriteSlot[V]{}, false
	}
	writeDraft, _ := tokenDraft[schemaWriteDraft](write.cell)
	row := &ruleDraft.builder.candidate.Rules[ruleDraft.index].Writes[writeDraft.index]
	row.Kind = coldcomposition.WriteRoute
	row.Route = uint64(readDraft.index + 1)
	ruleDraft.route = true
	return write, true
}

// SchemaDependency is opaque same-rule predecessor evidence. It intentionally
// does not expose either a target bit or a cold row index.
type SchemaDependency struct {
	cell *schemaTokenCell
}

func (slot SchemaWriteSlot[V]) Dependency() SchemaDependency {
	return SchemaDependency{cell: slot.cell}
}

func SchemaSelectWrite[V, O any](rule *RuleSlot[V, O], form SchemaWriteForm[V], candidates []SchemaReadRef, dependencies []SchemaDependency) (SchemaWriteSlot[V], bool) {
	formDraft, formOK := tokenDraft[schemaFormDraft](form.cell)
	ruleDraft, ruleOK := ruleDraft(rule)
	if !formOK || !ruleOK || formDraft.formKind != SchemaFormWriteSelector || len(candidates) == 0 || len(dependencies) == 0 || ruleDraft.builder.phase != schemaBuilderChildren {
		return SchemaWriteSlot[V]{}, false
	}
	orderedCandidates, candidatesOK := canonicalReadPredecessors(ruleDraft, candidates)
	orderedDependencies, dependenciesOK := canonicalDependencies(ruleDraft, dependencies)
	if !candidatesOK || !dependenciesOK {
		return SchemaWriteSlot[V]{}, false
	}
	write, ok := appendSchemaWrite[V](rule, form)
	if !ok {
		return SchemaWriteSlot[V]{}, false
	}
	writeDraft, _ := tokenDraft[schemaWriteDraft](write.cell)
	row := &ruleDraft.builder.candidate.Rules[ruleDraft.index].Writes[writeDraft.index]
	row.Candidates = orderedCandidates
	row.Dependencies = orderedDependencies
	return write, true
}

type SchemaCarrySlot[V any] struct {
	cell *schemaTokenCell
}

func (slot SchemaCarrySlot[V]) Schema() *Schema {
	if slot.cell == nil {
		return nil
	}
	return slot.cell.schema
}

type schemaCarryDraft struct {
	builder *SchemaBuilder
	rule    *schemaRuleDraft
	index   int
	token   *schemaTokenCell
}

// SchemaCarryFrom records the ordinary whole-output-Factor predecessor used
// by CarryFrom. Its zero Transform is a closed cold disposition, not a
// missing semantic identity and not permission to infer an executable map.
func SchemaCarryFrom[V, O any](rule *RuleSlot[V, O], input SchemaInput, factor FactorRef[V]) (SchemaCarrySlot[V], bool) {
	return appendSchemaCarry(rule, input, factor, SemanticKey{}, false)
}

// SchemaCarry records the transformed whole-output-Factor predecessor used
// by TransformCarryFrom. The transform identity is globally claimed exactly
// once; executable behavior belongs to the later hot Binding.
func SchemaCarry[V, O any](rule *RuleSlot[V, O], input SchemaInput, factor FactorRef[V], transform SemanticKey) (SchemaCarrySlot[V], bool) {
	return appendSchemaCarry(rule, input, factor, transform, true)
}

func appendSchemaCarry[V, O any](rule *RuleSlot[V, O], input SchemaInput, factor FactorRef[V], transform SemanticKey, transformed bool) (SchemaCarrySlot[V], bool) {
	ruleDraft, ok := ruleDraft(rule)
	factorDraft, factorOK := tokenDraft[schemaFactorDraft](factor.cell)
	inputDraft, inputOK := tokenDraft[schemaInputDraft](input.cell)
	if !ok || !ruleDraft.factorOutput() || len(ruleDraft.builder.candidate.Rules[ruleDraft.index].Carries) != 0 || !factorOK || factorDraft != ruleDraft.output || !inputOK || inputDraft.rule != ruleDraft || !factor.validBuilder(ruleDraft.builder) || transformed != transform.Available() || inputDraft.index < 0 || inputDraft.index >= ruleInputs(ruleDraft) {
		return SchemaCarrySlot[V]{}, false
	}
	builder := ruleDraft.builder
	if transformed && !builder.claim(transform) {
		return SchemaCarrySlot[V]{}, false
	}
	index := len(builder.candidate.Rules[ruleDraft.index].Carries)
	if !schemaSlotCardinality(index) {
		builder.poison()
		return SchemaCarrySlot[V]{}, false
	}
	draft := &schemaCarryDraft{builder: builder, rule: ruleDraft, index: index}
	builder.carries = append(builder.carries, draft)
	builder.candidate.Rules[ruleDraft.index].Carries = append(builder.candidate.Rules[ruleDraft.index].Carries, coldcomposition.Carry{Input: uint64(inputDraft.index), Factor: factorDraft.semantic.compositionKey(), Transform: transform.compositionKey()})
	result := SchemaCarrySlot[V]{cell: &schemaTokenCell{builder: builder, draft: draft}}
	builder.tokens = append(builder.tokens, result.cell)
	draft.token = result.cell
	return result, true
}

// SchemaCompletion and SchemaPrune are structural capabilities.  Their
// semantic rows contain no executable support evaluator.
type SchemaCompletion struct {
	cell *schemaCompletionCell
}
type schemaCompletionCell struct {
	builder         *SchemaBuilder
	semantic, prune SemanticKey
	schema          *Schema
	ordinal         uint64
}
type SchemaPrune struct{ cell *schemaCompletionCell }

func (completion SchemaCompletion) Schema() *Schema {
	if completion.cell == nil {
		return nil
	}
	return completion.cell.schema
}
func (prune SchemaPrune) Schema() *Schema {
	if prune.cell == nil {
		return nil
	}
	return prune.cell.schema
}

func DeclareSchemaCompletion(builder *SchemaBuilder, semantic SemanticKey) (SchemaCompletion, bool) {
	if builder == nil || builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren || !semantic.Available() || builder.candidate.Completion.Semantic.Available() {
		if builder != nil {
			builder.poison()
		}
		return SchemaCompletion{}, false
	}
	if !builder.claim(semantic) {
		return SchemaCompletion{}, false
	}
	builder.phase = schemaBuilderChildren
	builder.candidate.Completion.Semantic = semantic.compositionKey()
	cell := &schemaCompletionCell{builder: builder, semantic: semantic}
	builder.completion = cell
	return SchemaCompletion{cell: cell}, true
}

func (completion SchemaCompletion) Prune(semantic SemanticKey) (SchemaPrune, bool) {
	if completion.cell == nil || completion.cell.builder == nil || completion.cell.builder.phase != schemaBuilderChildren || !completion.cell.semantic.Available() || !semantic.Available() || completion.cell.prune.Available() || semantic == completion.cell.semantic {
		return SchemaPrune{}, false
	}
	if !completion.cell.builder.claim(semantic) {
		return SchemaPrune{}, false
	}
	completion.cell.prune = semantic
	completion.cell.builder.candidate.Completion.Prune = semantic.compositionKey()
	return SchemaPrune{cell: completion.cell}, true
}

type SchemaActivationFamily struct {
	cell *schemaTokenCell
}

func (family SchemaActivationFamily) Schema() *Schema {
	if family.cell == nil {
		return nil
	}
	return family.cell.schema
}
func (family SchemaActivationFamily) Ordinal() (uint64, bool) {
	if family.cell == nil || family.cell.schema == nil {
		return 0, false
	}
	return family.cell.ordinal, true
}

func activationDraft(family SchemaActivationFamily) (*schemaFamilyDraft, bool) {
	return tokenDraft[schemaFamilyDraft](family.cell)
}

func DeclareSchemaActivationFamily(builder *SchemaBuilder, semantic SemanticKey) (SchemaActivationFamily, bool) {
	if builder == nil || builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren || !semantic.Available() {
		if builder != nil {
			builder.poison()
		}
		return SchemaActivationFamily{}, false
	}
	if !builder.claim(semantic) {
		return SchemaActivationFamily{}, false
	}
	builder.phase = schemaBuilderChildren
	index := len(builder.candidate.ActivationFamilies)
	if !schemaSlotCardinality(index) {
		builder.poison()
		return SchemaActivationFamily{}, false
	}
	builder.candidate.ActivationFamilies = append(builder.candidate.ActivationFamilies, coldcomposition.ActivationFamily{Semantic: semantic.compositionKey()})
	family := &schemaFamilyDraft{builder: builder, index: index, semantic: semantic}
	builder.families = append(builder.families, family)
	cell := &schemaTokenCell{builder: builder, draft: family}
	builder.tokens = append(builder.tokens, cell)
	return SchemaActivationFamily{cell: cell}, true
}

type SchemaStructuralRuleSpec struct {
	Semantic   SemanticKey
	Inputs     uint64
	Admission  SchemaAdmission
	Completion SchemaCompletion
	Prune      SchemaPrune
	Activation SchemaActivationFamily
}

// SchemaSupportRuleSlot is the engine-owned cold capability for a structural
// support Rule.  It deliberately does not pretend that Support/ruleUnit are
// caller-selected generic types; only engine binding code may recover those
// private hot proof types.
type SchemaSupportRuleSlot struct{ cell *schemaTokenCell }

// SchemaActivationRuleSlot is the engine-owned cold capability for a
// structural activation Rule.  ActivationResult and ruleUnit remain private
// engine proofs and therefore cannot be erased to caller-provided marker
// types.
type SchemaActivationRuleSlot struct{ cell *schemaTokenCell }

func (slot *SchemaSupportRuleSlot) Available() bool { return structuralSlotAvailable(slotCell(slot)) }
func (slot *SchemaActivationRuleSlot) Available() bool {
	return structuralSlotAvailable(slotCell(slot))
}
func (slot *SchemaSupportRuleSlot) Schema() *Schema { return structuralSlotSchema(slotCell(slot)) }
func (slot *SchemaActivationRuleSlot) Schema() *Schema {
	return structuralSlotSchema(slotCell(slot))
}
func (slot *SchemaSupportRuleSlot) Ordinal() (uint64, bool) {
	return structuralSlotOrdinal(slotCell(slot))
}
func (slot *SchemaActivationRuleSlot) Ordinal() (uint64, bool) {
	return structuralSlotOrdinal(slotCell(slot))
}
func (slot *SchemaSupportRuleSlot) Input(index uint64) (SchemaInput, bool) {
	return structuralSlotInput(slotCell(slot), index)
}
func (slot *SchemaActivationRuleSlot) Input(index uint64) (SchemaInput, bool) {
	return structuralSlotInput(slotCell(slot), index)
}

func slotCell(slot any) *schemaTokenCell {
	switch value := slot.(type) {
	case *SchemaSupportRuleSlot:
		if value != nil {
			return value.cell
		}
	case *SchemaActivationRuleSlot:
		if value != nil {
			return value.cell
		}
	}
	return nil
}

func structuralSlotAvailable(cell *schemaTokenCell) bool {
	return cell != nil && (cell.schema != nil || cell.builder != nil && cell.builder.phase != schemaBuilderPoisoned && cell.builder.phase != schemaBuilderSealed)
}

func structuralSlotSchema(cell *schemaTokenCell) *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func structuralSlotOrdinal(cell *schemaTokenCell) (uint64, bool) {
	if cell != nil && cell.schema != nil && cell.schema.Available() {
		return cell.ordinal, true
	}
	return 0, false
}

func structuralSlotInput(cell *schemaTokenCell, index uint64) (SchemaInput, bool) {
	rule, ok := tokenDraft[schemaRuleDraft](cell)
	if !ok || index > uint64(int(^uint(0)>>1)) || rule.builder.phase != schemaBuilderChildren || index >= uint64(ruleInputs(rule)) || !schemaSlotCardinality(len(rule.inputs)) {
		return SchemaInput{}, false
	}
	for _, prior := range rule.inputs {
		if uint64(prior.index) == index {
			return SchemaInput{cell: prior.token}, prior.token != nil
		}
	}
	input := &schemaInputDraft{builder: rule.builder, rule: rule, index: int(index)}
	rule.inputs = append(rule.inputs, input)
	rule.builder.inputs = append(rule.builder.inputs, input)
	result := SchemaInput{cell: &schemaTokenCell{builder: rule.builder, draft: input}}
	rule.builder.tokens = append(rule.builder.tokens, result.cell)
	input.token = result.cell
	return result, true
}

func DeclareSchemaSupportRule(builder *SchemaBuilder, spec SchemaStructuralRuleSpec) (*SchemaSupportRuleSlot, bool) {
	_, activationOK := activationDraft(spec.Activation)
	if builder == nil || spec.Completion.cell == nil || !spec.Completion.cell.semantic.Available() || !spec.Prune.completionValid(builder, spec.Completion) || activationOK {
		if builder != nil {
			builder.poison()
		}
		return nil, false
	}
	cell, ok := builder.addStructuralRule(spec, false)
	if !ok {
		return nil, false
	}
	return &SchemaSupportRuleSlot{cell: cell}, true
}

func DeclareSchemaActivationRule(builder *SchemaBuilder, spec SchemaStructuralRuleSpec) (*SchemaActivationRuleSlot, bool) {
	activation, activationOK := activationDraft(spec.Activation)
	if builder == nil || !activationOK || activation.builder != builder || (spec.Completion.cell != nil && spec.Completion.cell.semantic.Available()) || spec.Prune.cell != nil {
		if builder != nil {
			builder.poison()
		}
		return nil, false
	}
	cell, ok := builder.addStructuralRule(spec, true)
	if !ok {
		return nil, false
	}
	return &SchemaActivationRuleSlot{cell: cell}, true
}

func (prune SchemaPrune) completionValid(builder *SchemaBuilder, completion SchemaCompletion) bool {
	return prune.cell != nil && prune.cell.builder == builder && completion.cell != nil && prune.cell == completion.cell && prune.cell.prune.Available() && builder.candidate.Completion.Prune.Available()
}

func (builder *SchemaBuilder) addStructuralRule(spec SchemaStructuralRuleSpec, activation bool) (*schemaTokenCell, bool) {
	if builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren || !spec.Semantic.Available() {
		builder.poison()
		return nil, false
	}
	builder.phase = schemaBuilderChildren
	admission, ok := spec.Admission.cold()
	if !ok {
		builder.poison()
		return nil, false
	}
	if !builder.claim(spec.Semantic) {
		return nil, false
	}
	index := len(builder.candidate.Rules)
	if !schemaSlotCardinality(index) || spec.Inputs > schemaSlotMax {
		builder.poison()
		return nil, false
	}
	draft := &schemaRuleDraft{builder: builder, index: index, structural: true}
	// Structural Rules have the engine-owned unit operand family.  It is not a
	// caller-supplied semantic identity: support and activation execution both
	// consume the engine's private ruleUnit proof.
	row := coldcomposition.Rule{Key: spec.Semantic.compositionKey(), OperandFamily: unitOperandFamily.compositionKey(), Admission: admission, OutputKind: coldcomposition.StructuralOutput, Inputs: spec.Inputs}
	if activation {
		family, _ := activationDraft(spec.Activation)
		row.Activations = []coldcomposition.ActivationRange{{Family: family.semantic.compositionKey()}}
	} else {
		row.Supports = []coldcomposition.Support{{Semantic: spec.Completion.cell.semantic.compositionKey()}}
		row.Prunes = []coldcomposition.Prune{{Semantic: spec.Prune.cell.prune.compositionKey()}}
	}
	builder.candidate.Rules = append(builder.candidate.Rules, row)
	builder.rules = append(builder.rules, draft)
	cell := &schemaTokenCell{builder: builder, draft: draft}
	builder.tokens = append(builder.tokens, cell)
	return cell, true
}

type schemaFamilyDraft struct {
	builder  *SchemaBuilder
	index    int
	semantic SemanticKey
}

// SchemaQuerySpec is the callback-free Query family shape.
type SchemaQuerySpec struct {
	Semantic SemanticKey
	Freezer  SemanticKey
}

type QuerySlot[R any] struct {
	cell *schemaTokenCell
}

type schemaQueryDraft struct {
	builder *SchemaBuilder
	index   int
	support bool
}

func queryDraft[R any](slot *QuerySlot[R]) (*schemaQueryDraft, bool) {
	if slot == nil {
		return nil, false
	}
	return tokenDraft[schemaQueryDraft](slot.cell)
}

func (slot *QuerySlot[R]) Available() bool {
	return slot != nil && slot.cell != nil && (slot.cell.schema != nil || slot.cell.builder != nil && slot.cell.builder.phase != schemaBuilderPoisoned && slot.cell.builder.phase != schemaBuilderSealed)
}
func (slot *QuerySlot[R]) Schema() *Schema {
	if slot == nil || slot.cell == nil {
		return nil
	}
	return slot.cell.schema
}

func (slot *QuerySlot[R]) Ordinal() (uint64, bool) {
	if slot != nil && slot.cell != nil && slot.cell.schema != nil && slot.cell.schema.Available() {
		return slot.cell.ordinal, true
	}
	return 0, false
}

func DeclareQuerySlot[R any](builder *SchemaBuilder, spec SchemaQuerySpec) (*QuerySlot[R], bool) {
	if builder == nil || builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren || !spec.Semantic.Available() || !spec.Freezer.Available() {
		if builder != nil {
			builder.poison()
		}
		return nil, false
	}
	if !builder.claim(spec.Semantic, spec.Freezer) {
		return nil, false
	}
	builder.phase = schemaBuilderChildren
	index := len(builder.candidate.Queries)
	if !schemaSlotCardinality(index) {
		builder.poison()
		return nil, false
	}
	builder.candidate.Queries = append(builder.candidate.Queries, coldcomposition.QueryFamily{Key: spec.Semantic.compositionKey(), Freezer: spec.Freezer.compositionKey()})
	draft := &schemaQueryDraft{builder: builder, index: index}
	builder.queries = append(builder.queries, draft)
	slot := &QuerySlot[R]{cell: &schemaTokenCell{builder: builder, draft: draft}}
	builder.tokens = append(builder.tokens, slot.cell)
	return slot, true
}

func NewQuerySlot[R any](builder *SchemaBuilder, spec SchemaQuerySpec) (*QuerySlot[R], bool) {
	return DeclareQuerySlot[R](builder, spec)
}

func SchemaQueryRead[T, R any](query *QuerySlot[R], form SchemaReadForm[T]) bool {
	queryDraft, ok := queryDraft(query)
	formDraft, formOK := tokenDraft[schemaFormDraft](form.cell)
	if !ok || !formOK || queryDraft.support || !form.valid(queryDraft.builder) {
		return false
	}
	builder := queryDraft.builder
	row := &builder.candidate.Queries[queryDraft.index]
	if formDraft.formKind != SchemaFormReadExact && formDraft.formKind != SchemaFormReadSummary {
		return false
	}
	projection := coldcomposition.QueryProjection{Factor: formDraft.factor.semantic.compositionKey()}
	if formDraft.formKind == SchemaFormReadExact {
		projection.Kind = coldcomposition.QueryFactorExact
	} else {
		projection.Kind = coldcomposition.QueryFactorSummary
		projection.Normalizer = formDraft.semantic.compositionKey()
	}
	row.Projections = append(row.Projections, projection)
	return true
}

func DeclareSchemaSupportQuery[R any](builder *SchemaBuilder, spec SchemaQuerySpec) (*QuerySlot[R], bool) {
	query, ok := DeclareQuerySlot[R](builder, spec)
	if !ok {
		return nil, false
	}
	draft, _ := queryDraft(query)
	draft.support = true
	builder.candidate.Queries[draft.index].Projections = []coldcomposition.QueryProjection{{Kind: coldcomposition.QuerySupport}}
	return query, true
}

// Seal canonicalizes all rows through internal/composition.Seal and rewrites
// every issued handle to the exact immutable Schema pointer and canonical
// ordinal. It is one-shot: all later declarations and all second seals fail.
func (builder *SchemaBuilder) Seal() (*Schema, bool) {
	if builder == nil || builder.phase == schemaBuilderPoisoned || builder.phase == schemaBuilderSealed {
		return nil, false
	}
	sealed, ok := coldcomposition.Seal(builder.candidate)
	if !ok || sealed == nil {
		builder.poison()
		return nil, false
	}
	var digest [32]byte
	sealedID := sealed.ID()
	copy(digest[:], sealedID[:])
	schema := &Schema{cold: sealed, id: CompositionID{digest: digest}}
	if !schema.Available() || !builder.bindSealed(schema, sealed) {
		builder.poison()
		return nil, false
	}
	builder.sealed = schema
	builder.phase = schemaBuilderSealed
	builder.releaseDrafts()
	return schema, true
}

func (builder *SchemaBuilder) bindSealed(schema *Schema, sealed *coldcomposition.Composition) bool {
	type plannedBinding struct {
		cell    *schemaTokenCell
		ordinal uint64
	}
	plan := make([]plannedBinding, 0, len(builder.tokens))
	add := func(cell *schemaTokenCell, ordinal uint64) bool {
		if cell == nil || cell.builder != builder || cell.draft == nil {
			return false
		}
		plan = append(plan, plannedBinding{cell: cell, ordinal: ordinal})
		return true
	}
	ruleOrdinals := make(map[*schemaRuleDraft]uint64, len(builder.rules))
	for _, factor := range builder.factors {
		index, ok := sealed.FactorIndex(factor.semantic.compositionKey())
		if !ok {
			return false
		}
		factorCell, ok := builder.tokenFor(factor)
		if !ok {
			return false
		}
		if !add(factorCell, index) {
			return false
		}
	}
	for _, rule := range builder.rules {
		if rule == nil || rule.structural == (rule.output != nil) {
			return false
		}
		key := builder.candidateRuleKey(rule)
		index, ok := sealed.RuleIndex(key)
		if !ok {
			return false
		}
		ruleOrdinals[rule] = index
		if cell, ok := builder.tokenFor(rule); ok {
			if !add(cell, index) {
				return false
			}
		} else {
			return false
		}
	}
	for _, query := range builder.queries {
		row := builder.queryKey(query)
		index, ok := sealed.QueryIndex(row)
		if !ok {
			return false
		}
		if cell, ok := builder.tokenFor(query); ok {
			if !add(cell, index) {
				return false
			}
		} else {
			return false
		}
	}
	for _, family := range builder.families {
		familyIndex, familyOK := sealed.ActivationIndex(family.semantic.compositionKey())
		if !familyOK {
			return false
		}
		if cell, ok := builder.tokenFor(family); ok {
			if !add(cell, familyIndex) {
				return false
			}
		} else {
			return false
		}
	}
	var completionBinding *schemaCompletionCell
	if builder.candidate.Completion.Semantic.Available() {
		completionBinding = builder.completion
		if completionBinding == nil {
			return false
		}
	}
	for _, form := range builder.forms {
		factorIndex, ok := sealed.FactorIndex(form.factor.semantic.compositionKey())
		if !ok {
			return false
		}
		factors := sealed.Factors()
		var formIndex uint64
		formFound := false
		for index, candidate := range factors[int(factorIndex)].Forms {
			wantKind := coldcomposition.FactorSummaryRead
			if form.formKind == SchemaFormWriteSelector {
				wantKind = coldcomposition.FactorSelectorWrite
			}
			if candidate.Kind == wantKind && candidate.Semantic == form.semantic.compositionKey() {
				formIndex = uint64(index)
				formFound = true
				break
			}
		}
		if form.formKind == SchemaFormReadExact || form.formKind == SchemaFormWriteExact {
			formIndex = factorIndex
			formFound = true
		}
		if !formFound {
			return false
		}
		if form.formKind == SchemaFormReadExact || form.formKind == SchemaFormWriteExact {
			form.canonical = factorIndex
		} else {
			form.canonical = factorIndex<<32 | formIndex
		}
		if form.token == nil {
			return false
		}
		if !add(form.token, form.canonical) {
			return false
		}
	}
	for _, input := range builder.inputs {
		parent, ok := ruleOrdinals[input.rule]
		if !ok || !add(input.token, parent<<32|uint64(input.index)) {
			return false
		}
	}
	for _, read := range builder.reads {
		parent, ok := ruleOrdinals[read.rule]
		if !ok || !add(read.token, parent<<32|uint64(read.index)) {
			return false
		}
	}
	for _, write := range builder.writes {
		parent, ok := ruleOrdinals[write.rule]
		if !ok || !add(write.token, parent<<32|uint64(write.index)) {
			return false
		}
	}
	for _, carry := range builder.carries {
		parent, ok := ruleOrdinals[carry.rule]
		if !ok || !add(carry.token, parent<<32|uint64(carry.index)) {
			return false
		}
	}
	for _, item := range plan {
		item.cell.bind(schema, item.ordinal)
	}
	if completionBinding != nil {
		completionBinding.schema, completionBinding.ordinal, completionBinding.builder = schema, 0, nil
	}
	return true
}

func (builder *SchemaBuilder) tokenFor(draft any) (*schemaTokenCell, bool) {
	for _, cell := range builder.tokens {
		if cell.draft == draft {
			return cell, true
		}
	}
	return nil, false
}

func (builder *SchemaBuilder) candidateRuleKey(rule *schemaRuleDraft) coldcomposition.Key {
	if rule == nil || rule.index < 0 || rule.index >= len(builder.candidate.Rules) {
		return coldcomposition.Key{}
	}
	return builder.candidate.Rules[rule.index].Key
}

func (builder *SchemaBuilder) queryKey(query *schemaQueryDraft) coldcomposition.Key {
	if query == nil || query.index < 0 || query.index >= len(builder.candidate.Queries) {
		return coldcomposition.Key{}
	}
	return builder.candidate.Queries[query.index].Key
}

func (builder *SchemaBuilder) poison() {
	if builder != nil && builder.phase != schemaBuilderSealed {
		builder.phase = schemaBuilderPoisoned
		builder.releaseDrafts()
	}
}

// releaseDrafts drops every mutable declaration/index/token edge after either
// terminal outcome. Bound token cells retain only their immutable Schema and
// canonical ordinal; poisoned cells retain no usable authority.
func (builder *SchemaBuilder) releaseDrafts() {
	if builder == nil {
		return
	}
	for _, cell := range builder.tokens {
		if cell == nil {
			continue
		}
		if cell.schema == nil {
			cell.builder, cell.draft, cell.ordinal, cell.kind = nil, nil, 0, SchemaFormInvalid
		}
	}
	if builder.completion != nil && builder.completion.schema == nil {
		builder.completion.builder = nil
	}
	builder.candidate = coldcomposition.Candidate{}
	builder.factors = nil
	builder.rules = nil
	builder.queries = nil
	builder.forms = nil
	builder.reads = nil
	builder.writes = nil
	builder.carries = nil
	builder.inputs = nil
	builder.families = nil
	builder.tokens = nil
	builder.completion = nil
	builder.claims = nil
}
