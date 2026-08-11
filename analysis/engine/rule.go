package engine

// RuleSpec declares one unanchored writing judgment. Inputs retains the
// finite ordered predecessor-port arity as schema, never as a concrete action
// input. Transfer is monomorphic typed behavior retained by this Rule handle;
// it is not evaluated or included in Composition identity during Wave D.
type RuleSpec[V, O any] struct {
	Semantic      SemanticKey
	OperandFamily SemanticKey
	// OperandContent is the OperandFamily-owned canonicalization law. It must
	// be pure and idempotent; return a transitively immutable/concurrency-safe
	// O whose complete Rule-observable meaning is named by the nonzero digest.
	// Mutable slices/maps must be detached. A sealed immutable owner handle may
	// return itself. Equal digests must be observationally interchangeable for
	// declaration, transfer, effects, and evidence.
	OperandContent func(O) (O, [32]byte, bool)
	Output         Output[V]
	Inputs         int
	Admission      RuleAdmission[V, O]
	Transfer       func(Access[V, O]) bool
}

// Rule is one typed cold judgment declaration. Its capability methods record
// Factor-owned read/write/carry forms only; Program/Link anchors and concrete
// carrier handles belong exclusively to Wave E.
type Rule[V, O any] struct {
	composition    *Composition
	schema         *ruleSchema
	output         Output[V]
	admission      RuleAdmission[V, O]
	transfer       func(Access[V, O]) bool
	operandContent func(O) (O, [32]byte, bool)
	// carryTransform/carryApply are installed only by TransformCarryFrom. The
	// cold schema retains the authored semantic identity while the typed law
	// stays with its Rule owner and never enters the heterogeneous carrier.
	carryTransform SemanticKey
	carryApply     func(O, V) (V, bool)
	open           bool
}

// ruleOutputKind is the public-declaration mirror of the cold canonical
// disposition.  The only public constructors are DeclareRule (Factor) and
// DeclareSupportRule (Structural), so a caller cannot spell an untagged
// output or turn a Factor Rule into a structural mutation after declaration.
type ruleOutputKind uint8

const (
	ruleOutputInvalid ruleOutputKind = iota
	ruleFactorOutput
	ruleStructuralOutput
)

type ruleSchema struct {
	composition   *Composition
	semantic      SemanticKey
	operandFamily SemanticKey
	outputKind    ruleOutputKind
	output        *factorSchema
	inputs        int
	admission     ruleAdmissionSchema
	open          bool

	reads          []coldRead
	carries        []coldCarry
	writes         []coldWrite
	readSelectors  []coldReadSelector
	writeSelectors []coldWriteSelector
	support        *supportRuleSchema
	activation     *activationRuleSchema
	bind           ruleBinder
	bindIndex      uint64
	bound          bool
}

// Input is an explicit predecessor port. Its fields are private; it can only
// be issued by InputAt for the Rule that owns it.
type Input struct {
	rule  *ruleSchema
	index int
}

// Read is one typed, unanchored Factor observation form attached to a Rule
// input port. It cannot be constructed externally or used as a concrete
// carrier read until a Wave-E instance binds it.
type Read[S any] struct {
	rule  *ruleSchema
	input Input
	index int
	// resolve is a declaration-time type witness. It deliberately carries no
	// carrier capability or concrete equation coordinate; the private E frame
	// validates and installs those separately.
	resolve func(*productSession, int, uint64) (S, bool)
}

// Write is one typed, unanchored output target shape. The Template compiler
// later binds the Rule's exact output coordinate or finite selector surface.
type Write[V any] struct {
	rule  *ruleSchema
	index int
}

// Dependency names a preceding read or staged target node in the same Rule
// schema. It cannot name a concrete cell, Point, action, or another Rule.
type Dependency struct {
	rule  *ruleSchema
	kind  dependencyKind
	index int
}

type dependencyKind uint8

const (
	readDependency dependencyKind = iota + 1
	writeDependency
)

type coldRead struct {
	input   int
	form    *formSchema
	depends []Dependency
	bind    readBinder
}

type coldCarry struct {
	input     int
	factor    *factorSchema
	transform SemanticKey
}

type coldWrite struct {
	form *formSchema
	// route is the one-based declared staged-read ordinal consumed by a route
	// write. Zero is the ordinary exact/static-selector form, so Go's zero
	// value is always the safe non-route meaning.
	route        uint64
	dependencies []Dependency
}

// coldReadSelector is one staged exact-read node. It has no fixed candidate
// vector: its runtime locator emits only owner-issued exact Ref routes from
// completed predecessor observations.
type coldReadSelector struct {
	read    int
	depends []Dependency
	bind    func(readBinding) (func(SelectorContext) bool, bool)
}

// coldWriteSelector remains the separate finite positional write-target DAG.
// Its candidate vector is never used by read selection.
type coldWriteSelector struct {
	write      int
	candidates []int
	depends    []Dependency
	decide     func(SelectorContext) bool
}

// Access is a future E-only typed execution frame. Wave D can retain a
// monomorphic transfer closure over this type but cannot construct an Access
// or execute the closure.
type Access[V, O any] struct {
	execution *ruleExecution
	owner     *boundRule[V, O]
	epoch     uint64
	output    outputAccess[V]
}

// ActivationCoordinates is the exact accepted dynamic relation that
// materialized one Rule row. It is a read-only, opaque projection: callers
// can inspect its binding and semantic axes but cannot enumerate, forge, or
// submit a Member. Ordinary Rules have no coordinates.
type ActivationCoordinates struct {
	binding     SemanticKey
	application SemanticKey
	target      SemanticKey
	endpoint    SemanticKey
}

func (value ActivationCoordinates) Available() bool {
	return value.binding.Available() && value.application.Available() && value.target.Available() && value.endpoint.Available()
}

func (value ActivationCoordinates) Binding() SemanticKey     { return value.binding }
func (value ActivationCoordinates) Application() SemanticKey { return value.application }
func (value ActivationCoordinates) Target() SemanticKey      { return value.target }
func (value ActivationCoordinates) Endpoint() SemanticKey    { return value.endpoint }

type accessToken[V any] struct{}

// readRule is the declaration-only common projection surface for Factor and
// Structural outputs.  It deliberately grants no write/carry capability.
// Both output dispositions therefore use the same typed Factor-read and
// selector grammar without giving a structural Rule a hidden Factor patch.
type readRule interface {
	readSchema() *ruleSchema
	validReadInput(Input) bool
	validReadDependencies([]Dependency) bool
	poison()
}

// DeclareRule records a complete unanchored Rule schema. Declaration enters
// the child phase permanently, so a later Factor declaration is rejected and
// poisons the Composition.
func DeclareRule[V, O any](composition *Composition, spec RuleSpec[V, O], declare func(*Rule[V, O]) bool) (*Rule[V, O], bool) {
	if composition == nil || !validRuleSpec(composition, spec) || !composition.acceptsChild(spec.Semantic) || declare == nil {
		if composition != nil {
			composition.poison()
		}
		return nil, false
	}
	admission, admitted := spec.Admission.cold()
	if !admitted {
		composition.poison()
		return nil, false
	}
	schema := &ruleSchema{composition: composition, semantic: spec.Semantic, operandFamily: spec.OperandFamily, outputKind: ruleFactorOutput, output: spec.Output.factor, inputs: spec.Inputs, admission: admission, open: true}
	rule := &Rule[V, O]{composition: composition, schema: schema, output: spec.Output, admission: spec.Admission, transfer: spec.Transfer, operandContent: spec.OperandContent, open: true}
	schema.bind = &factorRuleBind[V, O]{owner: rule}
	composition.rules = append(composition.rules, schema)
	declared := false
	func() {
		defer func() {
			if recover() != nil {
				composition.poison()
				declared = false
			}
			rule.open = false
			schema.open = false
		}()
		declared = declare(rule)
	}()
	if !declared || !composition.usable() {
		composition.poison()
		return nil, false
	}
	return rule, true
}

func validRuleSpec[V, O any](composition *Composition, spec RuleSpec[V, O]) bool {
	return composition != nil && spec.Semantic.Available() && spec.OperandFamily.Available() && spec.OperandFamily != spec.Semantic &&
		spec.Output.composition == composition && spec.Output.factor != nil &&
		spec.Output.factor.composition == composition && spec.Output.bindOutput != nil &&
		spec.Inputs >= 0 && spec.Admission.valid() && spec.Transfer != nil && spec.OperandContent != nil
}

// InputAt returns one declared predecessor port while the Rule callback is
// open. This preserves the complete ordered product shape in cold identity.
func (rule *Rule[V, O]) InputAt(index int) (Input, bool) {
	if !rule.validOpen() || index < 0 || index >= rule.schema.inputs {
		rule.poison()
		return Input{}, false
	}
	return Input{rule: rule.schema, index: index}, true
}

// ReadFrom records one typed exact or summary form on an explicit input.
// Factor and Structural output Rules share this read-only declaration surface.
func ReadFrom[RV, S any](rule readRule, input Input, form ReadForm[RV, S]) (Read[S], bool) {
	schema := rule.readSchema()
	if schema == nil || !rule.validReadInput(input) || !form.valid() || form.schema.factor.composition != schema.composition {
		rule.poison()
		return Read[S]{}, false
	}
	index := len(schema.reads)
	read := Read[S]{rule: schema, input: input, index: index, resolve: resolveTypedRead[RV, S]}
	schema.reads = append(schema.reads, coldRead{input: input.index, form: form.schema, bind: ruleReadBind[RV, S]{read: read, form: form}})
	return read, true
}

// CarryFrom records an explicit whole-Factor predecessor at one input port.
// It consumes the owner-issued capability rather than a Factor, so a Rule
// cannot recover a raw Factor authority from its carry declaration. It is not
// a coordinate read and therefore cannot be substituted for one.
func CarryFrom[OV, O any](rule *Rule[OV, O], input Input, form CarryForm) bool {
	if !rule.validInput(input) || form.composition != rule.composition || form.factor == nil || form.factor != rule.schema.output {
		rule.poison()
		return false
	}
	if len(rule.schema.carries) != 0 {
		rule.poison()
		return false
	}
	rule.schema.carries = append(rule.schema.carries, coldCarry{input: input.index, factor: form.factor})
	return true
}

// TransformCarryFrom replaces ordinary whole-Factor CarryFrom for this Rule
// with one typed Rule law over the Factor-issued carry. The immutable Rule
// instance operand selects the exact transform (for example Age_root); the
// map is applied only to the carried predecessor closure under each staged
// Product row guard, before that row's writes enter the same Patch.
//
// apply must be deterministic, monotone, same-operand idempotent,
// join-homomorphic, and default-preserving. The runtime fences the properties
// decidable per produced value; the Rule owner's semantic laws prove the
// relational properties. A transfer need not be extensive.
func TransformCarryFrom[V, O any](rule *Rule[V, O], input Input, carry CarryForm, semantic SemanticKey, apply func(O, V) (V, bool)) bool {
	if !rule.validInput(input) || carry.composition != rule.composition || carry.factor == nil || carry.factor != rule.schema.output || !semantic.Available() || apply == nil || len(rule.schema.carries) != 0 || rule.carryTransform.Available() || rule.carryApply != nil || !rule.composition.claim(semantic) {
		rule.poison()
		return false
	}
	rule.schema.carries = append(rule.schema.carries, coldCarry{input: input.index, factor: carry.factor, transform: semantic})
	rule.carryTransform = semantic
	rule.carryApply = apply
	return true
}

// WriteTo records one exact output shape. Selector writes use SelectWrite so
// their decision callback and dependency DAG cannot be silently dropped.
func WriteTo[V, O any](rule *Rule[V, O], form WriteForm[V]) (Write[V], bool) {
	if !rule.validOpen() || !form.valid() || form.schema.writeKind != exactWriteForm || form.schema.factor.composition != rule.composition || form.schema.factor != rule.schema.output {
		rule.poison()
		return Write[V]{}, false
	}
	index := len(rule.schema.writes)
	rule.schema.writes = append(rule.schema.writes, coldWrite{form: form.schema})
	return Write[V]{rule: rule.schema, index: index}, true
}

// SelectWrite records a selector target, its ordered prior read candidates,
// and the complete earlier dependency shape that controls it. Candidate order
// is part of the cold identity: the E binder pairs each candidate read to the
// same ordinal in the Factor-issued target surface. Dependencies retain both
// completed reads and earlier staged writes; target-dependency relations are
// sealed later by E and never inferred from a callback.
func SelectWrite[V, O, S any](rule *Rule[V, O], form WriteForm[V], candidates []Read[S], dependencies []Dependency, decide func(SelectorContext) bool) (Write[V], bool) {
	if decide == nil || !form.valid() || form.schema.writeKind != selectorWriteForm || len(candidates) == 0 || len(dependencies) == 0 {
		rule.poison()
		return Write[V]{}, false
	}
	if !rule.validOpen() || form.schema.factor.composition != rule.composition || form.schema.factor != rule.schema.output || !rule.validDependencies(dependencies) {
		rule.poison()
		return Write[V]{}, false
	}
	indexes := make([]int, len(candidates))
	lastCandidate := -1
	for index, candidate := range candidates {
		if candidate.rule != rule.schema || candidate.index < 0 || candidate.index >= len(rule.schema.reads) || candidate.index <= lastCandidate {
			rule.poison()
			return Write[V]{}, false
		}
		lastCandidate = candidate.index
		indexes[index] = candidate.index
	}
	index := len(rule.schema.writes)
	rule.schema.writes = append(rule.schema.writes, coldWrite{form: form.schema, dependencies: append([]Dependency(nil), dependencies...)})
	rule.schema.writeSelectors = append(rule.schema.writeSelectors, coldWriteSelector{write: index, candidates: indexes, depends: append([]Dependency(nil), dependencies...), decide: decide})
	return Write[V]{rule: rule.schema, index: index}, true
}

// RouteWrite consumes every exact route of one preceding staged selection as
// one atomic output batch. Its target form is the Rule output Factor's
// intrinsic exact form; callers cannot supply a second target surface, a raw
// Ref, or a static candidate vector. CarryFrom may coexist to preserve the
// output Factor coordinates not selected by the route.
func RouteWrite[V, O any, Tag selectionTag, S any](rule *Rule[V, O], selection Read[Selection[Tag, S]]) (Write[V], bool) {
	if !rule.validOpen() || selection.rule != rule.schema || selection.index < 0 || selection.index >= len(rule.schema.reads) || selection.resolve == nil || rule.schema.output == nil || rule.schema.output.exactWrite == nil || len(rule.schema.writes) != 0 {
		rule.poison()
		return Write[V]{}, false
	}
	selected := rule.schema.reads[selection.index]
	if selected.form == nil || selected.form.factor != rule.schema.output || selected.form.readKind != exactReadForm || len(selected.depends) == 0 || coldSelectorForRead(rule.schema, selection.index) == nil {
		rule.poison()
		return Write[V]{}, false
	}
	index := len(rule.schema.writes)
	rule.schema.writes = append(rule.schema.writes, coldWrite{form: rule.schema.output.exactWrite, route: uint64(selection.index) + 1})
	return Write[V]{rule: rule.schema, index: index}, true
}

func coldSelectorForRead(rule *ruleSchema, read int) *coldReadSelector {
	if rule == nil || read < 0 {
		return nil
	}
	for index := range rule.readSelectors {
		selector := &rule.readSelectors[index]
		if selector.read == read {
			return selector
		}
	}
	return nil
}

// ReadDependency converts a prior completed Rule read into a selector/target
// dependency. The returned capability carries no observation value.
func ReadDependency[S any](read Read[S]) Dependency {
	return Dependency{rule: read.rule, kind: readDependency, index: read.index}
}

// WriteDependency converts an earlier staged target into a later selector or
// target dependency.
func WriteDependency[V any](write Write[V]) Dependency {
	return Dependency{rule: write.rule, kind: writeDependency, index: write.index}
}

// SelectRead declares one staged exact observation for a Factor Rule. The
// locator runs only against the completed declared predecessor projection and
// this instance's frozen operand; it emits owner-issued Ref+Tag routes through
// SelectRoute. No fixed candidate surface is recorded or materialized.
func SelectRead[V, O, RV, S any, Tag selectionTag](rule *Rule[V, O], input Input, form ReadForm[RV, S], dependencies []Dependency, locate func(SelectorContext, O) bool) (Read[Selection[Tag, S]], bool) {
	if rule == nil || locate == nil {
		if rule != nil {
			rule.poison()
		}
		return Read[Selection[Tag, S]]{}, false
	}
	return declareStagedRead[RV, S, Tag](rule, input, form, dependencies, func(target readBinding) (func(SelectorContext) bool, bool) {
		bound, ok := target.(*boundRule[V, O])
		if !ok || bound == nil || bound.rule != rule.schema {
			return nil, false
		}
		operand := bound.operand
		return func(context SelectorContext) bool { return locate(context, operand) }, true
	})
}

// SelectStructuralRead is the structural declaration front door for the same
// staged-read runtime. Structural Rules have no user operand, so its locator
// sees only the sealed predecessor observations. It deliberately creates no
// second evaluator or selector representation.
func SelectStructuralRead[RV, S any, Tag selectionTag](rule readRule, input Input, form ReadForm[RV, S], dependencies []Dependency, locate func(SelectorContext) bool) (Read[Selection[Tag, S]], bool) {
	if rule == nil || locate == nil {
		if rule != nil {
			rule.poison()
		}
		return Read[Selection[Tag, S]]{}, false
	}
	return declareStagedRead[RV, S, Tag](rule, input, form, dependencies, func(target readBinding) (func(SelectorContext) bool, bool) {
		switch bound := target.(type) {
		case *compiledSupportRule:
			if bound == nil || bound.rule != rule.readSchema() {
				return nil, false
			}
		case *compiledActivationRule:
			if bound == nil || bound.rule != rule.readSchema() {
				return nil, false
			}
		default:
			return nil, false
		}
		return locate, true
	})
}

func declareStagedRead[RV, S any, Tag selectionTag](rule readRule, input Input, form ReadForm[RV, S], dependencies []Dependency, bind func(readBinding) (func(SelectorContext) bool, bool)) (Read[Selection[Tag, S]], bool) {
	schema := rule.readSchema()
	if schema == nil || !rule.validReadInput(input) || !form.valid() || form.schema.readKind != exactReadForm || form.schema.factor.composition != schema.composition || bind == nil || !rule.validReadDependencies(dependencies) {
		rule.poison()
		return Read[Selection[Tag, S]]{}, false
	}
	index := len(schema.reads)
	read := Read[Selection[Tag, S]]{rule: schema, input: input, index: index, resolve: resolveTypedSelection[RV, S, Tag]}
	schema.reads = append(schema.reads, coldRead{input: input.index, form: form.schema, depends: append([]Dependency(nil), dependencies...), bind: stagedReadBind[RV, S, Tag]{read: read, form: form}})
	schema.readSelectors = append(schema.readSelectors, coldReadSelector{read: index, depends: append([]Dependency(nil), dependencies...), bind: bind})
	return read, true
}

func (rule *Rule[V, O]) validOpen() bool {
	return rule != nil && rule.open && rule.composition != nil && rule.schema != nil && rule.schema.open && rule.schema.composition == rule.composition && rule.composition.usable()
}

func (rule *Rule[V, O]) readSchema() *ruleSchema {
	if rule == nil {
		return nil
	}
	return rule.schema
}

func (rule *Rule[V, O]) validReadInput(input Input) bool { return rule.validInput(input) }

// available remains true after Composition.Seal so Wave E can bind this exact
// Rule schema. It grants no mutation surface: all declaration methods use
// validOpen instead.
func (rule *Rule[V, O]) available() bool {
	return rule != nil && rule.composition != nil && rule.schema != nil && rule.schema.composition == rule.composition &&
		rule.output.composition == rule.composition && rule.output.factor == rule.schema.output && rule.output.bindOutput != nil && rule.composition.available()
}

func (rule *Rule[V, O]) validInput(input Input) bool {
	return rule.validOpen() && input.rule == rule.schema && input.index >= 0 && input.index < rule.schema.inputs
}

func (rule *Rule[V, O]) validDependencies(dependencies []Dependency) bool {
	seenWrite := false
	lastRead, lastWrite := -1, -1
	for _, dependency := range dependencies {
		if dependency.rule != rule.schema || dependency.index < 0 {
			return false
		}
		switch dependency.kind {
		case readDependency:
			if seenWrite || dependency.index >= len(rule.schema.reads) || dependency.index <= lastRead {
				return false
			}
			lastRead = dependency.index
		case writeDependency:
			if dependency.index >= len(rule.schema.writes) || dependency.index <= lastWrite {
				return false
			}
			seenWrite = true
			lastWrite = dependency.index
		default:
			return false
		}
	}
	return true
}

func (rule *Rule[V, O]) validReadDependencies(dependencies []Dependency) bool {
	if len(dependencies) == 0 {
		return false
	}
	last := -1
	for _, dependency := range dependencies {
		if dependency.rule != rule.schema || dependency.kind != readDependency || dependency.index < 0 || dependency.index >= len(rule.schema.reads) || dependency.index <= last {
			return false
		}
		last = dependency.index
	}
	return true
}

func (rule *Rule[V, O]) poison() {
	if rule != nil && rule.composition != nil {
		rule.composition.poison()
	}
}

func validColdRule(composition *Composition, rule *ruleSchema) bool {
	if composition == nil || rule == nil || rule.composition != composition || rule.open || !rule.semantic.Available() || !rule.operandFamily.Available() || !rule.admission.valid() || !validRuleBind(rule) {
		return false
	}
	if rule.outputKind == ruleStructuralOutput {
		return validColdSupportRule(composition, rule) || validColdActivationRule(composition, rule)
	}
	if rule.outputKind != ruleFactorOutput || rule.support != nil || rule.activation != nil || rule.output == nil || rule.output.composition != composition || rule.inputs < 0 || len(rule.writes) == 0 && len(rule.carries) == 0 {
		return false
	}
	for _, read := range rule.reads {
		if read.input < 0 || read.input >= rule.inputs || !validColdReadForm(composition, read.form) {
			return false
		}
	}
	if len(rule.carries) > 1 {
		return false
	}
	carries := make(map[coldCarry]struct{}, len(rule.carries))
	for _, carry := range rule.carries {
		_, claimed := composition.semantics[carry.transform]
		if carry.input < 0 || carry.input >= rule.inputs || carry.factor == nil || carry.factor != rule.output || carry.factor.composition != composition ||
			carry.transform.Available() && !claimed {
			return false
		}
		if _, duplicate := carries[carry]; duplicate {
			return false
		}
		carries[carry] = struct{}{}
	}
	routeWrite := -1
	for index, write := range rule.writes {
		if !validColdWriteForm(composition, write.form) || write.form.factor != rule.output {
			return false
		}
		if write.route != 0 {
			routeRead := write.route - 1
			if routeWrite >= 0 || write.form != rule.output.exactWrite || write.form.writeKind != exactWriteForm || len(write.dependencies) != 0 || routeRead >= uint64(len(rule.reads)) {
				return false
			}
			routeWrite = index
		} else if write.form.writeKind == exactWriteForm && len(write.dependencies) != 0 || write.form.writeKind == selectorWriteForm && len(write.dependencies) == 0 {
			return false
		}
		for _, dependency := range write.dependencies {
			if dependency.rule != rule || dependency.index < 0 {
				return false
			}
			switch dependency.kind {
			case readDependency:
				if dependency.index >= len(rule.reads) {
					return false
				}
			case writeDependency:
				if dependency.index >= index {
					return false
				}
			default:
				return false
			}
		}
	}
	readSelectors := make(map[int]struct{}, len(rule.readSelectors))
	for _, selector := range rule.readSelectors {
		if selector.bind == nil || len(selector.depends) == 0 || selector.read < 0 || selector.read >= len(rule.reads) {
			return false
		}
		if _, duplicate := readSelectors[selector.read]; duplicate {
			return false
		}
		readSelectors[selector.read] = struct{}{}
		selected := rule.reads[selector.read]
		if selected.form.readKind != exactReadForm || !sameDependencies(selected.depends, selector.depends) {
			return false
		}
		lastDependency := -1
		for _, dependency := range selector.depends {
			if dependency.rule != rule || dependency.kind != readDependency || dependency.index < 0 || dependency.index >= selector.read || lastDependency >= dependency.index {
				return false
			}
			lastDependency = dependency.index
		}
	}
	writeSelectors := make(map[int]struct{}, len(rule.writeSelectors))
	for _, selector := range rule.writeSelectors {
		if selector.decide == nil || len(selector.depends) == 0 || selector.write < 0 || selector.write >= len(rule.writes) {
			return false
		}
		if _, duplicate := writeSelectors[selector.write]; duplicate || len(selector.candidates) == 0 {
			return false
		}
		writeSelectors[selector.write] = struct{}{}
		selected := rule.writes[selector.write]
		if selected.form.writeKind != selectorWriteForm || !sameDependencies(selected.dependencies, selector.depends) {
			return false
		}
		lastCandidate := -1
		for _, candidate := range selector.candidates {
			if candidate < 0 || candidate >= len(rule.reads) || candidate <= lastCandidate {
				return false
			}
			lastCandidate = candidate
		}
		for _, dependency := range selector.depends {
			if dependency.rule != rule || dependency.index < 0 {
				return false
			}
			switch dependency.kind {
			case readDependency:
				if dependency.index >= len(rule.reads) {
					return false
				}
			case writeDependency:
				if dependency.index >= selector.write {
					return false
				}
			default:
				return false
			}
		}
	}
	for index, read := range rule.reads {
		if len(read.depends) != 0 {
			if _, present := readSelectors[index]; !present {
				return false
			}
		} else if _, present := readSelectors[index]; present {
			return false
		}
	}
	for index, write := range rule.writes {
		if write.route != 0 {
			if routeWrite != index || len(rule.writes) != 1 {
				return false
			}
			selected := rule.reads[int(write.route-1)]
			if selected.form == nil || selected.form.factor != rule.output || selected.form.readKind != exactReadForm || len(selected.depends) == 0 {
				return false
			}
			if _, present := readSelectors[int(write.route-1)]; !present {
				return false
			}
		} else if write.form.writeKind == selectorWriteForm {
			if _, present := writeSelectors[index]; !present {
				return false
			}
		}
	}
	return true
}

func validColdSupportRule(composition *Composition, rule *ruleSchema) bool {
	if rule == nil || rule.outputKind != ruleStructuralOutput || rule.output != nil || rule.inputs < 0 || len(rule.carries) != 0 || len(rule.writes) != 0 {
		return false
	}
	support := rule.support
	if support == nil || support.completion == nil || support.completion.composition != composition || support.completion.prune == nil ||
		support.prune == nil || support.prune != support.completion.prune || support.prune.completion != support.completion ||
		!support.completion.semantic.Available() || !support.prune.semantic.Available() || !validColdSupportCompletion(composition, support.completion) {
		return false
	}
	for _, read := range rule.reads {
		if read.input < 0 || read.input >= rule.inputs || !validColdReadForm(composition, read.form) {
			return false
		}
	}
	readSelectors := make(map[int]struct{}, len(rule.readSelectors))
	for _, selector := range rule.readSelectors {
		if selector.bind == nil || len(selector.depends) == 0 || selector.read < 0 || selector.read >= len(rule.reads) {
			return false
		}
		if _, duplicate := readSelectors[selector.read]; duplicate {
			return false
		}
		readSelectors[selector.read] = struct{}{}
		selected := rule.reads[selector.read]
		if selected.form.readKind != exactReadForm || !sameDependencies(selected.depends, selector.depends) {
			return false
		}
		lastDependency := -1
		for _, dependency := range selector.depends {
			if dependency.rule != rule || dependency.kind != readDependency || dependency.index < 0 || dependency.index >= selector.read || dependency.index <= lastDependency {
				return false
			}
			lastDependency = dependency.index
		}
	}
	for index, read := range rule.reads {
		if len(read.depends) != 0 {
			if _, present := readSelectors[index]; !present {
				return false
			}
		} else if _, present := readSelectors[index]; present {
			return false
		}
	}
	return true
}

func sameDependencies(left, right []Dependency) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validColdReadForm(composition *Composition, form *formSchema) bool {
	return form != nil && form.factor != nil && form.factor.composition == composition && form.readKind != 0 && form.writeKind == 0 && form.semantic.Available() && form.factor.hasForm(form)
}

func validColdWriteForm(composition *Composition, form *formSchema) bool {
	return form != nil && form.factor != nil && form.factor.composition == composition && form.readKind == 0 && form.writeKind != 0 && form.semantic.Available() && form.factor.hasForm(form)
}
