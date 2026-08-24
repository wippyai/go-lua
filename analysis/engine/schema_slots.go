package engine

// This file is the callback-free declaration surface for the reusable cold
// schema. It is intentionally independent of Factor algebra, executable
// transfer functions, key domains, and runtime coordinates. The one-shot
// builder is the sole public construction path.
//
// Every issued token is one slotHandle over a shared identity cell, and every
// declaration row is one of three row shapes tagged by its declaration role.
// The public named types exist to keep each role's published capability set
// distinct; they add only the methods that role publishes.

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

const schemaSlotMax = uint64(^uint32(0))

func schemaSlotCardinality(count int) bool {
	return count >= 0 && uint64(count) <= schemaSlotMax
}

// Declaration roles name the disjoint declaration vocabularies. A role rides
// as a phantom type parameter so that two rows with the same shape remain
// distinct handle and draft types: a token issued for one role can never
// recover another role's declaration row.
type (
	factorRole struct{}
	familyRole struct{}
	inputRole  struct{}
	readRole   struct{}
	writeRole  struct{}
	carryRole  struct{}
)

// SchemaBuilder is a single-use, callback-free cold-schema candidate.  Its
// declaration rows are discarded after Seal; the returned Schema is the only
// retained semantic authority.
type SchemaBuilder struct {
	phase      schemaBuilderPhase
	candidate  coldcomposition.Candidate
	factors    []*keyDraft[factorRole]
	families   []*keyDraft[familyRole]
	rules      []*schemaRuleDraft
	queries    []*schemaQueryDraft
	forms      []*schemaFormDraft
	rows       []*ruleRow
	completion *schemaCompletionDraft
	tokens     []*schemaTokenCell
	claims     map[identity.SemanticKey]struct{}
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
	return &SchemaBuilder{phase: schemaBuilderFactors, claims: make(map[identity.SemanticKey]struct{})}
}

// builderOpen reports the two phases that still admit declarations.
func builderOpen(builder *SchemaBuilder) bool {
	return builder != nil && builder.phase != schemaBuilderPoisoned && builder.phase != schemaBuilderSealed
}

// claim reserves each public semantic identity exactly once before its row is
// appended. References (operand families and route families) are not
// claims; Factors, optional forms, completion/prune, activation families,
// Rules, carry transforms, and Query/freezer rows are.
func (builder *SchemaBuilder) claim(keys ...identity.SemanticKey) bool {
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

// schemaTokenCell is the shared mutable identity cell behind every issued
// value token. Copies of a token therefore observe the same post-Seal owner
// and ordinal. Once bound, ordinary declaration storage is dropped from the
// cell; a generated Rule additionally retains its immutable engine projection.
type schemaTokenCell struct {
	builder *SchemaBuilder
	schema  *Schema
	ordinal uint64
	draft   any
	kind    SchemaFormKind
	// generated retains the engine-only generated Rule projection after Seal.
	// It is deliberately not part of any public token surface: a
	// GeneratedRuleSlot is an opaque identity, while the later generated
	// binding pass may recover this sealed projection from engine code.
	generated *generatedRuleCell
}

func (cell *schemaTokenCell) bind(schema *Schema, ordinal uint64) {
	if cell == nil {
		return
	}
	cell.schema, cell.ordinal = schema, ordinal
	cell.builder, cell.draft = nil, nil
}

func tokenDraft[D any](cell *schemaTokenCell) (*D, bool) {
	if cell == nil || cell.draft == nil {
		return nil, false
	}
	draft, ok := cell.draft.(*D)
	return draft, ok
}

// slotHandle is the single identity mechanism behind every declaration token.
// D names the declaration row the handle addresses; each public token type is
// one defined type over it. Before Seal a handle is a draft reference; Seal
// rewrites its cell to exactly (Schema pointer, canonical ordinal).
type slotHandle[D any] struct {
	cell *schemaTokenCell
}

func (handle slotHandle[D]) draft() (*D, bool) { return tokenDraft[D](handle.cell) }

func (handle slotHandle[D]) available() bool {
	return handle.cell != nil && (handle.cell.schema != nil || builderOpen(handle.cell.builder))
}

func (handle slotHandle[D]) schema() *Schema {
	if handle.cell == nil {
		return nil
	}
	return handle.cell.schema
}

// formKind is the declared directional evidence a form token retains after
// its declaration storage is dropped.
func (handle slotHandle[D]) formKind() SchemaFormKind {
	if handle.cell == nil {
		return SchemaFormInvalid
	}
	return handle.cell.kind
}

func (handle slotHandle[D]) ordinal() (uint64, bool) {
	if handle.cell != nil && handle.cell.schema != nil && handle.cell.schema.Available() {
		return handle.cell.ordinal, true
	}
	return 0, false
}

// declarationRow is the token back-reference every declaration row carries.
// issue installs it, so Seal binds each row through the row's own token and
// never searches the builder for it.
type declarationRow[D any] interface {
	*D
	setToken(*schemaTokenCell)
}

// issue installs one declaration row's identity cell in the builder and
// returns the handle that addresses it.
func issue[P declarationRow[D], D any](builder *SchemaBuilder, draft P, kind SchemaFormKind) slotHandle[D] {
	cell := &schemaTokenCell{builder: builder, draft: draft, kind: kind}
	draft.setToken(cell)
	builder.tokens = append(builder.tokens, cell)
	return slotHandle[D]{cell: cell}
}

// keyRow is the declaration state shared by the top-level keyed rows: a
// Factor and an activation family. Both bind to the canonical ordinal their
// semantic identity holds in the sealed composition.
type keyRow struct {
	builder  *SchemaBuilder
	index    int
	semantic identity.SemanticKey
	token    *schemaTokenCell
}

func (row *keyRow) setToken(cell *schemaTokenCell) { row.token = cell }

type keyDraft[R any] struct{ keyRow }

// ruleRow is the declaration state shared by the four ordered rows a Rule
// owns: predecessor ports, reads, writes, and the carry. All four bind to the
// same coordinate, the owning Rule's ordinal paired with the row index.
type ruleRow struct {
	builder *SchemaBuilder
	rule    *schemaRuleDraft
	index   int
	token   *schemaTokenCell
}

func (row *ruleRow) setToken(cell *schemaTokenCell) { row.token = cell }

type rowDraft[R any] struct{ ruleRow }

// appendRuleRow declares one ordered Rule row: its draft, its token, and its
// binding registration are installed together.
func appendRuleRow[R any](rule *schemaRuleDraft, index int) (*rowDraft[R], slotHandle[rowDraft[R]]) {
	builder := rule.builder
	draft := &rowDraft[R]{ruleRow{builder: builder, rule: rule, index: index}}
	builder.rows = append(builder.rows, &draft.ruleRow)
	return draft, issue(builder, draft, SchemaFormInvalid)
}

// FactorRef is a typed output/reference projection of a FactorSlot.  It
// carries no algebra and is valid only against the exact builder or Schema
// that issued it.
type FactorRef[V any] struct {
	slotHandle[keyDraft[factorRole]]
}

func (ref FactorRef[V]) validBuilder(builder *SchemaBuilder) bool {
	draft, ok := ref.draft()
	return ok && builderOpen(builder) && draft.builder == builder
}

// Any erases the value type of a Factor reference so unlike Factors can be
// ordered into one declared vector.
func (ref FactorRef[V]) Any() AnyFactorRef { return AnyFactorRef{ref.slotHandle} }

// AnyFactorRef is a positional Factor reference whose value type is erased.
// It retains the issuing cell, so a caller can order references to unlike
// Factors without gaining the ability to mint one.
type AnyFactorRef struct {
	slotHandle[keyDraft[factorRole]]
}

// FactorSlot is a typed Factor owner handle.  Before Seal it is a draft
// reference; Seal rewrites it to exactly (Schema pointer, canonical ordinal).
type FactorSlot[T any] struct {
	slotHandle[keyDraft[factorRole]]
}

func (slot *FactorSlot[T]) Ref() FactorRef[T] {
	if slot == nil {
		return FactorRef[T]{}
	}
	return FactorRef[T]{slot.slotHandle}
}

func (slot *FactorSlot[T]) Available() bool { return slot != nil && slot.available() }

func (slot *FactorSlot[T]) Schema() *Schema {
	if slot == nil {
		return nil
	}
	return slot.schema()
}

func (slot *FactorSlot[T]) Ordinal() (uint64, bool) {
	if slot == nil {
		return 0, false
	}
	return slot.ordinal()
}

func (slot *FactorSlot[T]) factorDraft() (*keyDraft[factorRole], bool) {
	if slot == nil {
		return nil, false
	}
	return slot.draft()
}

// SchemaReadForm and SchemaWriteForm are typed form capabilities.  They are
// only schema shapes; no normalizer, callback, key end, or runtime slot is
// retained.  Their concrete capabilities are issued by FactorSlot methods.
type SchemaReadForm[T any] struct{ slotHandle[schemaFormDraft] }

type SchemaWriteForm[T any] struct{ slotHandle[schemaFormDraft] }

func (form SchemaReadForm[T]) Kind() SchemaFormKind { return form.formKind() }

func (form SchemaWriteForm[T]) Kind() SchemaFormKind { return form.formKind() }

func (form SchemaReadForm[T]) valid(builder *SchemaBuilder) bool {
	draft, ok := form.draft()
	return ok && builderOpen(builder) && draft.builder == builder &&
		(draft.formKind == SchemaFormReadExact || summaryReadFormKind(draft.formKind))
}

// summaryReadFormKind reports the two declared summary read folds. Both are
// summary reads of the same declared key vector; they differ only in whether
// the reader observes the joint partition or the coordinate-wise fold.
func summaryReadFormKind(kind SchemaFormKind) bool {
	return kind == SchemaFormReadSummary || kind == SchemaFormReadDistributiveSummary
}

// summaryReadRowKind is the cold counterpart of summaryReadFormKind.
func summaryReadRowKind(kind coldcomposition.FactorFormKind) bool {
	return kind == coldcomposition.FactorSummaryRead || kind == coldcomposition.FactorDistributiveSummaryRead
}

func (form SchemaWriteForm[T]) valid(builder *SchemaBuilder) bool {
	draft, ok := form.draft()
	return ok && builderOpen(builder) && draft.builder == builder && draft.formKind == SchemaFormWriteExact
}

func (form SchemaReadForm[T]) Schema() *Schema { return form.schema() }

func (form SchemaWriteForm[T]) Schema() *Schema { return form.schema() }

func (slot *FactorSlot[T]) ExactRead() (SchemaReadForm[T], bool) {
	handle, ok := slot.declareForm(SchemaFormReadExact, identity.SemanticKey{})
	return SchemaReadForm[T]{handle}, ok
}

func (slot *FactorSlot[T]) SummaryRead(semantic identity.SemanticKey) (SchemaReadForm[T], bool) {
	handle, ok := slot.declareForm(SchemaFormReadSummary, semantic)
	return SchemaReadForm[T]{handle}, ok
}

// DistributiveSummaryRead declares a summary read whose reader folds each
// declared coordinate independently. The declaration is the sole authority
// for that fold: readers of this form never observe the joint partition of
// the declared vector, and no observation call may choose otherwise.
func (slot *FactorSlot[T]) DistributiveSummaryRead(semantic identity.SemanticKey) (SchemaReadForm[T], bool) {
	handle, ok := slot.declareForm(SchemaFormReadDistributiveSummary, semantic)
	return SchemaReadForm[T]{handle}, ok
}

func (slot *FactorSlot[T]) ExactWrite() (SchemaWriteForm[T], bool) {
	handle, ok := slot.declareForm(SchemaFormWriteExact, identity.SemanticKey{})
	return SchemaWriteForm[T]{handle}, ok
}

func (slot *FactorSlot[T]) declareForm(kind SchemaFormKind, semantic identity.SemanticKey) (slotHandle[schemaFormDraft], bool) {
	draft, ok := slot.factorDraft()
	if !ok || draft.builder.phase != schemaBuilderFactors {
		return slotHandle[schemaFormDraft]{}, false
	}
	form, ok := draft.builder.addForm(draft, kind, semantic)
	if !ok {
		return slotHandle[schemaFormDraft]{}, false
	}
	return issue(draft.builder, form, kind), true
}

// DeclareFactorSlot adds one typed Factor schema without asking for algebra
// callbacks or a key-domain enumeration.
func DeclareFactorSlot[T any](builder *SchemaBuilder, semantic identity.SemanticKey) (*FactorSlot[T], bool) {
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
	draft := &keyDraft[factorRole]{keyRow{builder: builder, index: index, semantic: semantic}}
	builder.candidate.Factors = append(builder.candidate.Factors, coldcomposition.Factor{Key: compositionKeyOf(semantic)})
	builder.factors = append(builder.factors, draft)
	return &FactorSlot[T]{issue(builder, draft, SchemaFormInvalid)}, true
}

// FactorSlot is a concise alias for DeclareFactorSlot at call sites that use
// the new API's role vocabulary.
func NewFactorSlot[T any](builder *SchemaBuilder, semantic identity.SemanticKey) (*FactorSlot[T], bool) {
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
	SchemaFormReadDistributiveSummary
)

type schemaFormDraft struct {
	builder  *SchemaBuilder
	factor   *keyDraft[factorRole]
	semantic identity.SemanticKey
	formKind SchemaFormKind
	token    *schemaTokenCell
}

func (draft *schemaFormDraft) setToken(cell *schemaTokenCell) { draft.token = cell }

// factorFormRowKind is the declaration-kind table for optional Factor forms.
// It is total over coldcomposition.FactorFormKind; exact forms report no row
// because they are the Factor's intrinsic surface.
func factorFormRowKind(kind SchemaFormKind) (coldcomposition.FactorFormKind, bool) {
	switch kind {
	case SchemaFormReadSummary:
		return coldcomposition.FactorSummaryRead, true
	case SchemaFormReadDistributiveSummary:
		return coldcomposition.FactorDistributiveSummaryRead, true
	}
	return 0, false
}

func (builder *SchemaBuilder) addForm(factor *keyDraft[factorRole], kind SchemaFormKind, semantic identity.SemanticKey) (*schemaFormDraft, bool) {
	if builder == nil || factor == nil || factor.builder != builder || factor.index < 0 || factor.index >= len(builder.candidate.Factors) {
		builder.poison()
		return nil, false
	}
	if !schemaSlotCardinality(len(builder.forms)) ||
		(kind != SchemaFormReadExact && !summaryReadFormKind(kind) && kind != SchemaFormWriteExact) {
		builder.poison()
		return nil, false
	}
	rowKind, optional := factorFormRowKind(kind)
	if optional && !semantic.Available() {
		builder.poison()
		return nil, false
	}
	if !optional {
		factorSemantic, factorSemanticOK := semanticKeyFromComposition(builder.candidate.Factors[factor.index].Key)
		if !factorSemanticOK {
			builder.poison()
			return nil, false
		}
		semantic = factorSemantic
	}
	for _, prior := range builder.forms {
		if prior.factor == factor && prior.formKind == kind && prior.semantic == semantic {
			builder.poison()
			return nil, false
		}
	}
	if optional && !builder.claim(semantic) {
		return nil, false
	}
	form := &schemaFormDraft{builder: builder, factor: factor, semantic: semantic, formKind: kind}
	builder.forms = append(builder.forms, form)
	if optional {
		builder.candidate.Factors[factor.index].Forms = append(builder.candidate.Factors[factor.index].Forms, coldcomposition.FactorForm{Kind: rowKind, Semantic: compositionKeyOf(semantic)})
	}
	return form, true
}

// SchemaInput is a typed predecessor port.  It is not a concrete point or
// equation coordinate.
type SchemaInput struct {
	slotHandle[rowDraft[inputRole]]
}

func (input SchemaInput) validBuilder(builder *SchemaBuilder, rule *schemaRuleDraft) bool {
	draft, ok := input.draft()
	return ok && builder != nil && builder.phase == schemaBuilderChildren && rule != nil && draft.builder == builder && draft.rule == rule
}

// SchemaRuleSpec declares one Factor-output Rule.  Structural Rules use the
// dedicated support/activation constructors below.
type SchemaRuleSpec[V any] struct {
	Semantic      identity.SemanticKey
	OperandFamily identity.SemanticKey
	Inputs        uint64
	Output        FactorRef[V]
}

type RuleSlot[V, O any] struct{ slotHandle[schemaRuleDraft] }

// GeneratedRuleSlot is the domain-free identity of a generated Rule row.
// Unlike RuleSlot it carries no caller-selected V/O marker types and exposes
// no old read/write/carry declaration methods. Its only public authority is
// the sealed Schema and dense Rule ordinal, which are sufficient for the
// later engine capability handoff.
type GeneratedRuleSlot struct{ slotHandle[schemaRuleDraft] }

// RuleFamilyTarget is the sealed rule identity a family is claimed against.
// A hand-declared RuleSlot and a Program-declared GeneratedRuleSlot are the
// same thing to an install: one sealed rule ordinal in one schema. They claim
// through this one seam, so a Program-declared rule acquires no second family
// lifecycle of its own.
type RuleFamilyTarget interface {
	// Ordinal is the sealed rule ordinal the claim is made against.
	Ordinal() (uint64, bool)
	// ruleFamilyCell is the issuing declaration cell. It is unexported so only
	// a rule slot this package minted can be a claim target.
	ruleFamilyCell() *schemaTokenCell
}

func (slot *GeneratedRuleSlot) Available() bool { return slot != nil && slot.available() }

// ruleFamilyCell exposes the generated rule row's issuing cell to the family
// install seam.
func (slot *GeneratedRuleSlot) ruleFamilyCell() *schemaTokenCell {
	if slot == nil {
		return nil
	}
	return slot.cell
}

func (slot *GeneratedRuleSlot) Ordinal() (uint64, bool) {
	if slot == nil {
		return 0, false
	}
	return slot.ordinal()
}

func (slot *GeneratedRuleSlot) Schema() *Schema {
	if slot == nil {
		return nil
	}
	return slot.schema()
}

func (slot *RuleSlot[V, O]) Available() bool { return slot != nil && slot.available() }

func (slot *RuleSlot[V, O]) Ordinal() (uint64, bool) {
	if slot == nil {
		return 0, false
	}
	return slot.ordinal()
}

// ruleFamilyCell exposes the hand-declared rule row's issuing cell to the
// family install seam.
func (slot *RuleSlot[V, O]) ruleFamilyCell() *schemaTokenCell {
	if slot == nil {
		return nil
	}
	return slot.cell
}

func (slot *RuleSlot[V, O]) ruleDraft() (*schemaRuleDraft, bool) {
	if slot == nil {
		return nil, false
	}
	return slot.draft()
}

type schemaRuleDraft struct {
	builder   *SchemaBuilder
	index     int
	inputs    []*rowDraft[inputRole]
	output    *keyDraft[factorRole]
	route     bool
	generated *generatedRuleCell
	token     *schemaTokenCell
}

func (draft *schemaRuleDraft) setToken(cell *schemaTokenCell) { draft.token = cell }

// factorOutput is the exact disposition for a normal rule. Structural rules
// have no output capability and may never acquire writes, carries, routes, or
// selectors through the generic factor path.
func (rule *schemaRuleDraft) factorOutput() bool {
	return rule != nil && rule.output != nil
}

// DeclareRuleSlot adds a callback-free Factor-output Rule.
func DeclareRuleSlot[V, O any](builder *SchemaBuilder, spec SchemaRuleSpec[V]) (*RuleSlot[V, O], bool) {
	outputDraft, outputOK := spec.Output.draft()
	if builder == nil || builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren || !spec.Semantic.Available() || !spec.OperandFamily.Available() || !outputOK || !spec.Output.validBuilder(builder) {
		if builder != nil {
			builder.poison()
		}
		return nil, false
	}
	builder.phase = schemaBuilderChildren
	if !builder.claim(spec.Semantic) {
		return nil, false
	}
	index := len(builder.candidate.Rules)
	if !schemaSlotCardinality(index) || spec.Inputs > schemaSlotMax {
		builder.poison()
		return nil, false
	}
	draft := &schemaRuleDraft{builder: builder, index: index, output: outputDraft}
	builder.candidate.Rules = append(builder.candidate.Rules, coldcomposition.Rule{Key: compositionKeyOf(spec.Semantic), OperandFamily: compositionKeyOf(spec.OperandFamily), OutputKind: coldcomposition.FactorOutput, Output: compositionKeyOf(outputDraft.semantic), Inputs: spec.Inputs})
	builder.rules = append(builder.rules, draft)
	return &RuleSlot[V, O]{issue(builder, draft, SchemaFormInvalid)}, true
}

// NewRuleSlot is the concise constructor spelling for DeclareRuleSlot.
func NewRuleSlot[V, O any](builder *SchemaBuilder, spec SchemaRuleSpec[V]) (*RuleSlot[V, O], bool) {
	return DeclareRuleSlot[V, O](builder, spec)
}

func (slot *RuleSlot[V, O]) Input(index uint64) (SchemaInput, bool) {
	rule, ok := slot.ruleDraft()
	return ruleInput(rule, ok, index)
}

// ruleInput issues one ordered predecessor port. A port is identified by its
// index, so a repeated declaration returns the port's original token.
func ruleInput(rule *schemaRuleDraft, ok bool, index uint64) (SchemaInput, bool) {
	if !ok || index > uint64(int(^uint(0)>>1)) || rule.builder.phase != schemaBuilderChildren || index >= uint64(ruleInputs(rule)) || !schemaSlotCardinality(len(rule.inputs)) {
		return SchemaInput{}, false
	}
	for _, prior := range rule.inputs {
		if uint64(prior.index) == index {
			return SchemaInput{slotHandle[rowDraft[inputRole]]{cell: prior.token}}, prior.token != nil
		}
	}
	draft, handle := appendRuleRow[inputRole](rule, int(index))
	rule.inputs = append(rule.inputs, draft)
	return SchemaInput{handle}, true
}

func ruleInputs(rule *schemaRuleDraft) int {
	if rule == nil || rule.builder == nil || rule.index < 0 || rule.index >= len(rule.builder.candidate.Rules) {
		return -1
	}
	return int(rule.builder.candidate.Rules[rule.index].Inputs)
}

// canonicalOrder sorts declared predecessor evidence into its canonical order
// and reports whether the evidence is duplicate-free.
func canonicalOrder[E comparable](rows []E, before func(left, right E) bool) bool {
	sort.Slice(rows, func(left, right int) bool { return before(rows[left], rows[right]) })
	for index := 1; index < len(rows); index++ {
		if rows[index-1] == rows[index] {
			return false
		}
	}
	return true
}

func canonicalReadPredecessors(rule *schemaRuleDraft, refs []SchemaReadRef) ([]uint64, bool) {
	if rule == nil || rule.builder == nil || len(refs) == 0 || !schemaSlotCardinality(len(refs)) {
		return nil, false
	}
	result := make([]uint64, len(refs))
	for index, ref := range refs {
		read, ok := ref.draft()
		if !ok || read.rule != rule || read.index < 0 || read.index >= len(rule.builder.candidate.Rules[rule.index].Reads) {
			return nil, false
		}
		result[index] = uint64(read.index)
	}
	if !canonicalOrder(result, func(left, right uint64) bool { return left < right }) {
		return nil, false
	}
	return result, true
}

// SchemaReadSlot is one callback-free ordered Rule read capability.
type SchemaReadSlot[T any] struct{ slotHandle[rowDraft[readRole]] }

// SchemaReadRef is an opaque same-rule predecessor capability. It never
// exposes a cold row index.
type SchemaReadRef struct{ slotHandle[rowDraft[readRole]] }

func (slot SchemaReadSlot[T]) Ref() SchemaReadRef { return SchemaReadRef{slot.slotHandle} }

func SchemaRead[T, V, O any](rule *RuleSlot[V, O], form SchemaReadForm[T], input SchemaInput) (SchemaReadSlot[T], bool) {
	ruleDraft, ok := rule.ruleDraft()
	return appendSchemaRead(ruleDraft, ok, form, input, nil)
}

// SchemaActivationRead attaches a typed Factor projection to the exact opaque
// structural activation Rule.  It never exports ActivationResult/ruleUnit.
func SchemaActivationRead[T any](rule *SchemaActivationRuleSlot, form SchemaReadForm[T], input SchemaInput) (SchemaReadSlot[T], bool) {
	if rule == nil {
		return SchemaReadSlot[T]{}, false
	}
	ruleDraft, ok := rule.draft()
	return appendSchemaRead(ruleDraft, ok, form, input, nil)
}

// readRowKind is the declaration-kind table for read rows. It is total over
// coldcomposition.ReadKind, so every read row is emitted complete and no
// constructor rewrites a row after it has entered the candidate.
func readRowKind(kind SchemaFormKind, selected bool) (coldcomposition.ReadKind, bool) {
	switch {
	case selected:
		if kind != SchemaFormReadExact {
			return 0, false
		}
		return coldcomposition.ReadSelect, true
	case kind == SchemaFormReadExact:
		return coldcomposition.ReadExact, true
	case summaryReadFormKind(kind):
		return coldcomposition.ReadSummary, true
	}
	return 0, false
}

func appendSchemaRead[T any](rule *schemaRuleDraft, ok bool, form SchemaReadForm[T], input SchemaInput, predecessors []uint64) (SchemaReadSlot[T], bool) {
	if !ok || !form.valid(rule.builder) || !input.validBuilder(rule.builder, rule) {
		return SchemaReadSlot[T]{}, false
	}
	inputDraft, _ := input.draft()
	formDraft, _ := form.draft()
	kind, kindOK := readRowKind(formDraft.formKind, predecessors != nil)
	if !kindOK || inputDraft.index < 0 || inputDraft.index >= ruleInputs(rule) {
		return SchemaReadSlot[T]{}, false
	}
	builder := rule.builder
	index := len(builder.candidate.Rules[rule.index].Reads)
	if !schemaSlotCardinality(index) {
		builder.poison()
		return SchemaReadSlot[T]{}, false
	}
	row := coldcomposition.Read{Kind: kind, Input: uint64(inputDraft.index), Factor: compositionKeyOf(formDraft.factor.semantic), PointBound: true}
	switch kind {
	case coldcomposition.ReadSummary:
		row.Semantic = compositionKeyOf(formDraft.semantic)
		row.Normalizer = row.Semantic
	case coldcomposition.ReadSelect:
		row.Semantic = row.Factor
		row.Dependencies = predecessors
	}
	builder.candidate.Rules[rule.index].Reads = append(builder.candidate.Rules[rule.index].Reads, row)
	_, handle := appendRuleRow[readRole](rule, index)
	return SchemaReadSlot[T]{handle}, true
}

// SchemaSelectedRead adds a staged exact read and its ordered predecessor
// dependencies. Dependencies are read ordinals in the same Rule.
func SchemaSelectedRead[T, FV, V, O any](rule *RuleSlot[V, O], form SchemaReadForm[FV], input SchemaInput, dependencies ...SchemaReadRef) (SchemaReadSlot[T], bool) {
	ruleDraft, ruleOK := rule.ruleDraft()
	if !ruleOK || len(dependencies) == 0 || !input.validBuilder(ruleDraft.builder, ruleDraft) {
		return SchemaReadSlot[T]{}, false
	}
	predecessors, ok := canonicalReadPredecessors(ruleDraft, dependencies)
	if !ok {
		return SchemaReadSlot[T]{}, false
	}
	base, ok := appendSchemaRead(ruleDraft, true, form, input, predecessors)
	if !ok {
		return SchemaReadSlot[T]{}, false
	}
	return SchemaReadSlot[T]{base.slotHandle}, true
}

type SchemaWriteSlot[V any] struct {
	slotHandle[rowDraft[writeRole]]
}

// writeDeclaration is the declared shape of one write beyond its form. A
// routed write names its selected read. Every write row is emitted from it in
// one step.
type writeDeclaration struct {
	route uint64
}

// writeRowKind is the declaration-kind table for write rows. It is total over
// coldcomposition.WriteKind, so every write row is emitted complete and no
// constructor rewrites a row after it has entered the candidate.
func writeRowKind(kind SchemaFormKind, routed bool) (coldcomposition.WriteKind, bool) {
	switch {
	case routed:
		if kind != SchemaFormWriteExact {
			return 0, false
		}
		return coldcomposition.WriteRoute, true
	case kind == SchemaFormWriteExact:
		return coldcomposition.WriteExact, true
	}
	return 0, false
}

func SchemaWrite[V, O any](rule *RuleSlot[V, O], form SchemaWriteForm[V]) (SchemaWriteSlot[V], bool) {
	formDraft, formOK := form.draft()
	if !formOK || formDraft.formKind != SchemaFormWriteExact {
		return SchemaWriteSlot[V]{}, false
	}
	return appendSchemaWrite(rule, form, writeDeclaration{})
}

func appendSchemaWrite[V, O any](rule *RuleSlot[V, O], form SchemaWriteForm[V], declaration writeDeclaration) (SchemaWriteSlot[V], bool) {
	ruleDraft, ok := rule.ruleDraft()
	formDraft, formOK := form.draft()
	if !ok || !ruleDraft.factorOutput() || ruleDraft.route || !formOK || !form.valid(ruleDraft.builder) || formDraft.factor != ruleDraft.output {
		return SchemaWriteSlot[V]{}, false
	}
	kind, kindOK := writeRowKind(formDraft.formKind, declaration.route != 0)
	if !kindOK {
		return SchemaWriteSlot[V]{}, false
	}
	builder := ruleDraft.builder
	index := len(builder.candidate.Rules[ruleDraft.index].Writes)
	if !schemaSlotCardinality(index) {
		builder.poison()
		return SchemaWriteSlot[V]{}, false
	}
	row := coldcomposition.Write{Kind: kind, Factor: compositionKeyOf(formDraft.factor.semantic), Route: declaration.route}
	builder.candidate.Rules[ruleDraft.index].Writes = append(builder.candidate.Rules[ruleDraft.index].Writes, row)
	_, handle := appendRuleRow[writeRole](ruleDraft, index)
	return SchemaWriteSlot[V]{handle}, true
}

func SchemaRouteWrite[V, O, T any](rule *RuleSlot[V, O], form SchemaWriteForm[V], read SchemaReadSlot[T]) (SchemaWriteSlot[V], bool) {
	formDraft, formOK := form.draft()
	ruleDraft, ruleOK := rule.ruleDraft()
	readDraft, readOK := read.draft()
	if !formOK || !ruleOK || !readOK || !ruleDraft.factorOutput() || ruleDraft.route || ruleDraft.builder.phase != schemaBuilderChildren {
		return SchemaWriteSlot[V]{}, false
	}
	candidate := &ruleDraft.builder.candidate.Rules[ruleDraft.index]
	if len(candidate.Writes) != 0 || formDraft.formKind != SchemaFormWriteExact || readDraft.rule != ruleDraft || readDraft.index < 0 || readDraft.index >= len(candidate.Reads) ||
		candidate.Reads[readDraft.index].Kind != coldcomposition.ReadSelect || candidate.Reads[readDraft.index].Factor != compositionKeyOf(ruleDraft.output.semantic) {
		return SchemaWriteSlot[V]{}, false
	}
	write, ok := appendSchemaWrite(rule, form, writeDeclaration{route: uint64(readDraft.index + 1)})
	if !ok {
		return SchemaWriteSlot[V]{}, false
	}
	ruleDraft.route = true
	return write, true
}

type SchemaCarrySlot[V any] struct {
	slotHandle[rowDraft[carryRole]]
}

func (slot SchemaCarrySlot[V]) Schema() *Schema { return slot.schema() }

// SchemaCarryFrom records the ordinary whole-output-Factor predecessor used
// by CarryFrom. Its zero Transform is a closed cold disposition, not a
// missing semantic identity and not permission to infer an executable map.
func SchemaCarryFrom[V, O any](rule *RuleSlot[V, O], input SchemaInput, factor FactorRef[V]) (SchemaCarrySlot[V], bool) {
	return appendSchemaCarry(rule, input, factor, identity.SemanticKey{}, false)
}

// SchemaCarry records the transformed whole-output-Factor predecessor used
// by TransformCarryFrom. The transform identity is globally claimed exactly
// once; executable behavior belongs to the later hot Binding.
func SchemaCarry[V, O any](rule *RuleSlot[V, O], input SchemaInput, factor FactorRef[V], transform identity.SemanticKey) (SchemaCarrySlot[V], bool) {
	return appendSchemaCarry(rule, input, factor, transform, true)
}

func appendSchemaCarry[V, O any](rule *RuleSlot[V, O], input SchemaInput, factor FactorRef[V], transform identity.SemanticKey, transformed bool) (SchemaCarrySlot[V], bool) {
	ruleDraft, ok := rule.ruleDraft()
	factorDraft, factorOK := factor.draft()
	inputDraft, inputOK := input.draft()
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
	builder.candidate.Rules[ruleDraft.index].Carries = append(builder.candidate.Rules[ruleDraft.index].Carries, coldcomposition.Carry{Input: uint64(inputDraft.index), Factor: compositionKeyOf(factorDraft.semantic), Transform: compositionKeyOf(transform)})
	_, handle := appendRuleRow[carryRole](ruleDraft, index)
	return SchemaCarrySlot[V]{handle}, true
}

// SchemaCompletion and SchemaPrune are structural capabilities.  Their
// semantic rows contain no executable support evaluator.  Both address the
// one completion declaration row, so both bind through the same token.
type SchemaCompletion struct {
	slotHandle[schemaCompletionDraft]
}

type SchemaPrune struct {
	slotHandle[schemaCompletionDraft]
}

type schemaCompletionDraft struct {
	builder         *SchemaBuilder
	semantic, prune identity.SemanticKey
	token           *schemaTokenCell
}

func (draft *schemaCompletionDraft) setToken(cell *schemaTokenCell) { draft.token = cell }

func (completion SchemaCompletion) Schema() *Schema { return completion.schema() }

func (prune SchemaPrune) Schema() *Schema { return prune.schema() }

func DeclareSchemaCompletion(builder *SchemaBuilder, semantic identity.SemanticKey) (SchemaCompletion, bool) {
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
	builder.candidate.Completion.Semantic = compositionKeyOf(semantic)
	draft := &schemaCompletionDraft{builder: builder, semantic: semantic}
	builder.completion = draft
	return SchemaCompletion{issue(builder, draft, SchemaFormInvalid)}, true
}

func (completion SchemaCompletion) Prune(semantic identity.SemanticKey) (SchemaPrune, bool) {
	draft, ok := completion.draft()
	if !ok || draft.builder == nil || draft.builder.phase != schemaBuilderChildren || !draft.semantic.Available() || !semantic.Available() || draft.prune.Available() || semantic == draft.semantic {
		return SchemaPrune{}, false
	}
	if !draft.builder.claim(semantic) {
		return SchemaPrune{}, false
	}
	draft.prune = semantic
	draft.builder.candidate.Completion.Prune = compositionKeyOf(semantic)
	return SchemaPrune{completion.slotHandle}, true
}

type SchemaActivationFamily struct {
	slotHandle[keyDraft[familyRole]]
}

func (family SchemaActivationFamily) Schema() *Schema { return family.schema() }

func DeclareSchemaActivationFamily(builder *SchemaBuilder, semantic identity.SemanticKey) (SchemaActivationFamily, bool) {
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
	builder.candidate.ActivationFamilies = append(builder.candidate.ActivationFamilies, coldcomposition.ActivationFamily{Semantic: compositionKeyOf(semantic)})
	draft := &keyDraft[familyRole]{keyRow{builder: builder, index: index, semantic: semantic}}
	builder.families = append(builder.families, draft)
	return SchemaActivationFamily{issue(builder, draft, SchemaFormInvalid)}, true
}

type SchemaStructuralRuleSpec struct {
	Semantic   identity.SemanticKey
	Inputs     uint64
	Completion SchemaCompletion
	Prune      SchemaPrune
	Activation SchemaActivationFamily
}

// SchemaSupportRuleSlot is the engine-owned cold capability for a structural
// support Rule.  It deliberately does not pretend that Support/ruleUnit are
// caller-selected generic types; only engine binding code may recover those
// private hot proof types.
type SchemaSupportRuleSlot struct{ slotHandle[schemaRuleDraft] }

// SchemaActivationRuleSlot is the engine-owned cold capability for a
// structural activation Rule.  ActivationResult and ruleUnit remain private
// engine proofs and therefore cannot be erased to caller-provided marker
// types.
type SchemaActivationRuleSlot struct{ slotHandle[schemaRuleDraft] }

func (slot *SchemaSupportRuleSlot) Available() bool { return slot != nil && slot.available() }

func (slot *SchemaActivationRuleSlot) Available() bool { return slot != nil && slot.available() }

func (slot *SchemaActivationRuleSlot) Ordinal() (uint64, bool) {
	if slot == nil {
		return 0, false
	}
	return slot.ordinal()
}

func (slot *SchemaActivationRuleSlot) Input(index uint64) (SchemaInput, bool) {
	if slot == nil {
		return SchemaInput{}, false
	}
	rule, ok := slot.draft()
	return ruleInput(rule, ok, index)
}

func DeclareSchemaSupportRule(builder *SchemaBuilder, spec SchemaStructuralRuleSpec) (*SchemaSupportRuleSlot, bool) {
	completion, completionOK := spec.Completion.draft()
	_, activationOK := spec.Activation.draft()
	if builder == nil || !completionOK || !completion.semantic.Available() || !spec.Prune.completionValid(builder, spec.Completion) || activationOK {
		if builder != nil {
			builder.poison()
		}
		return nil, false
	}
	handle, ok := builder.addStructuralRule(spec, false)
	if !ok {
		return nil, false
	}
	return &SchemaSupportRuleSlot{handle}, true
}

func DeclareSchemaActivationRule(builder *SchemaBuilder, spec SchemaStructuralRuleSpec) (*SchemaActivationRuleSlot, bool) {
	activation, activationOK := spec.Activation.draft()
	completion, completionOK := spec.Completion.draft()
	if builder == nil || !activationOK || activation.builder != builder || completionOK && completion.semantic.Available() || spec.Prune.cell != nil {
		if builder != nil {
			builder.poison()
		}
		return nil, false
	}
	handle, ok := builder.addStructuralRule(spec, true)
	if !ok {
		return nil, false
	}
	return &SchemaActivationRuleSlot{handle}, true
}

func (prune SchemaPrune) completionValid(builder *SchemaBuilder, completion SchemaCompletion) bool {
	draft, ok := prune.draft()
	return ok && draft.builder == builder && completion.cell != nil && prune.cell == completion.cell && draft.prune.Available() && builder.candidate.Completion.Prune.Available()
}

func (builder *SchemaBuilder) addStructuralRule(spec SchemaStructuralRuleSpec, activation bool) (slotHandle[schemaRuleDraft], bool) {
	if builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren || !spec.Semantic.Available() {
		builder.poison()
		return slotHandle[schemaRuleDraft]{}, false
	}
	builder.phase = schemaBuilderChildren
	if !builder.claim(spec.Semantic) {
		return slotHandle[schemaRuleDraft]{}, false
	}
	index := len(builder.candidate.Rules)
	if !schemaSlotCardinality(index) || spec.Inputs > schemaSlotMax {
		builder.poison()
		return slotHandle[schemaRuleDraft]{}, false
	}
	draft := &schemaRuleDraft{builder: builder, index: index}
	// Structural Rules have the engine-owned unit operand family.  It is not a
	// caller-supplied semantic identity: support and activation execution both
	// consume the engine's private ruleUnit proof.
	row := coldcomposition.Rule{Key: compositionKeyOf(spec.Semantic), OperandFamily: compositionKeyOf(unitOperandFamily), OutputKind: coldcomposition.StructuralOutput, Inputs: spec.Inputs}
	if activation {
		family, _ := spec.Activation.draft()
		row.Activations = []coldcomposition.ActivationRange{{Family: compositionKeyOf(family.semantic)}}
	} else {
		completion, _ := spec.Completion.draft()
		row.Supports = []coldcomposition.Support{{Semantic: compositionKeyOf(completion.semantic)}}
		row.Prunes = []coldcomposition.Prune{{Semantic: compositionKeyOf(completion.prune)}}
	}
	builder.candidate.Rules = append(builder.candidate.Rules, row)
	builder.rules = append(builder.rules, draft)
	return issue(builder, draft, SchemaFormInvalid), true
}

// SchemaQuerySpec is the callback-free Query family shape.
type SchemaQuerySpec struct {
	Semantic   identity.SemanticKey
	Freezer    identity.SemanticKey
	Population queryschema.PopulationKind
}

type QuerySlot[R any] struct{ slotHandle[schemaQueryDraft] }

type schemaQueryDraft struct {
	builder *SchemaBuilder
	index   int
	token   *schemaTokenCell
}

func (draft *schemaQueryDraft) setToken(cell *schemaTokenCell) { draft.token = cell }

func (slot *QuerySlot[R]) queryDraft() (*schemaQueryDraft, bool) {
	if slot == nil {
		return nil, false
	}
	return slot.draft()
}

func (slot *QuerySlot[R]) Available() bool { return slot != nil && slot.available() }

func (slot *QuerySlot[R]) Schema() *Schema {
	if slot == nil {
		return nil
	}
	return slot.schema()
}

func (slot *QuerySlot[R]) Ordinal() (uint64, bool) {
	if slot == nil {
		return 0, false
	}
	return slot.ordinal()
}

func DeclareQuerySlot[R any](builder *SchemaBuilder, spec SchemaQuerySpec) (*QuerySlot[R], bool) {
	if builder == nil || builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren || !spec.Semantic.Available() || !spec.Freezer.Available() || !spec.Population.Available() {
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
	builder.candidate.Queries = append(builder.candidate.Queries, coldcomposition.QueryFamily{
		Key: compositionKeyOf(spec.Semantic), Freezer: compositionKeyOf(spec.Freezer), Population: spec.Population,
	})
	draft := &schemaQueryDraft{builder: builder, index: index}
	builder.queries = append(builder.queries, draft)
	return &QuerySlot[R]{issue(builder, draft, SchemaFormInvalid)}, true
}

func NewQuerySlot[R any](builder *SchemaBuilder, spec SchemaQuerySpec) (*QuerySlot[R], bool) {
	return DeclareQuerySlot[R](builder, spec)
}

func SchemaQueryRead[T, R any](query *QuerySlot[R], form SchemaReadForm[T]) bool {
	queryDraft, ok := query.queryDraft()
	formDraft, formOK := form.draft()
	if !ok || !formOK || !form.valid(queryDraft.builder) {
		return false
	}
	builder := queryDraft.builder
	if builder == nil || queryDraft.index < 0 || queryDraft.index >= len(builder.candidate.Queries) {
		return false
	}
	row := &builder.candidate.Queries[queryDraft.index]
	if len(row.Projections) == 1 && row.Projections[0].Kind == coldcomposition.QuerySupport {
		return false
	}
	if formDraft.formKind != SchemaFormReadExact && !summaryReadFormKind(formDraft.formKind) {
		return false
	}
	projection := coldcomposition.QueryProjection{Factor: compositionKeyOf(formDraft.factor.semantic)}
	if formDraft.formKind == SchemaFormReadExact {
		projection.Kind = coldcomposition.QueryFactorExact
	} else {
		projection.Kind = coldcomposition.QueryFactorSummary
		projection.Normalizer = compositionKeyOf(formDraft.semantic)
	}
	row.Projections = append(row.Projections, projection)
	return true
}

func DeclareSchemaSupportQuery[R any](builder *SchemaBuilder, spec SchemaQuerySpec) (*QuerySlot[R], bool) {
	query, ok := DeclareQuerySlot[R](builder, spec)
	if !ok {
		return nil, false
	}
	draft, _ := query.queryDraft()
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
	schema.available = schema.completeGrammar()
	if !schema.Available() || !builder.bindSealed(schema, sealed) {
		builder.poison()
		return nil, false
	}
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
	for _, factor := range builder.factors {
		index, ok := sealed.FactorIndex(compositionKeyOf(factor.semantic))
		if !ok || !add(factor.token, index) {
			return false
		}
	}
	for _, family := range builder.families {
		index, ok := sealed.ActivationIndex(compositionKeyOf(family.semantic))
		if !ok || !add(family.token, index) {
			return false
		}
	}
	ruleOrdinals := make(map[*schemaRuleDraft]uint64, len(builder.rules))
	for _, rule := range builder.rules {
		if rule == nil {
			return false
		}
		index, ok := sealed.RuleIndex(builder.candidateRuleKey(rule))
		if !ok || !add(rule.token, index) {
			return false
		}
		ruleOrdinals[rule] = index
	}
	for _, query := range builder.queries {
		index, ok := sealed.QueryIndex(builder.queryKey(query))
		if !ok || !add(query.token, index) {
			return false
		}
	}
	factors := sealed.Factors()
	for _, form := range builder.forms {
		factorIndex, ok := sealed.FactorIndex(compositionKeyOf(form.factor.semantic))
		if !ok {
			return false
		}
		canonical := factorIndex
		if wantKind, optional := factorFormRowKind(form.formKind); optional {
			formIndex, formFound := uint64(0), false
			for index, candidate := range factors[int(factorIndex)].Forms {
				if candidate.Kind == wantKind && candidate.Semantic == compositionKeyOf(form.semantic) {
					formIndex, formFound = uint64(index), true
					break
				}
			}
			if !formFound {
				return false
			}
			canonical = factorIndex<<32 | formIndex
		}
		if !add(form.token, canonical) {
			return false
		}
	}
	for _, row := range builder.rows {
		parent, ok := ruleOrdinals[row.rule]
		if !ok || !add(row.token, parent<<32|uint64(row.index)) {
			return false
		}
	}
	// Generated descriptors are copied from the Rule cells' sealed Plan
	// projection into one canonical ordinal table. No later member bind may
	// derive a descriptor from occurrence geometry.
	_, ruleCount, _, _, shapeCountOK := sealed.ShapeCount()
	if !shapeCountOK {
		return false
	}
	// The descriptor table is the sealed RULE table, one entry per rule, and a
	// hand-declared rule simply leaves its entry absent. It is not a directory
	// of its own with its own numbering: a generated rule's coordinate is its
	// position here, which is the position the composition already gave it.
	generatedPrograms := make([]generated.CompiledRule, ruleCount)
	generatedPresent := false
	for rule, ordinal := range ruleOrdinals {
		if rule == nil || rule.generated == nil {
			continue
		}
		if ordinal > uint64(^uint32(0)) {
			return false
		}
		// This is the ONE assignment of a generated rule's coordinate. The
		// descriptor carries none, so nothing is rebased; the Rule cell - the
		// construction seam generated member binding consumes - is handed the
		// same ordinal its descriptor is placed at, as a foreign key.
		rule.generated.rule = uint32(ordinal)
		generatedPrograms[ordinal] = rule.generated.program
		generatedPresent = true
	}
	// The completion row is the schema's single structural capability: it has
	// no canonical vector of its own, so its token binds at ordinal zero.
	if builder.candidate.Completion.Semantic.Available() {
		if builder.completion == nil || !add(builder.completion.token, 0) {
			return false
		}
	}
	for _, item := range plan {
		item.cell.bind(schema, item.ordinal)
	}
	schema.generatedPrograms = generatedPrograms
	schema.generatedPresent = generatedPresent
	return true
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
		if cell == nil || cell.schema != nil {
			continue
		}
		cell.builder, cell.draft, cell.ordinal, cell.kind = nil, nil, 0, SchemaFormInvalid
	}
	builder.candidate = coldcomposition.Candidate{}
	builder.factors = nil
	builder.families = nil
	builder.rules = nil
	builder.queries = nil
	builder.forms = nil
	builder.rows = nil
	builder.completion = nil
	builder.tokens = nil
	builder.claims = nil
}
