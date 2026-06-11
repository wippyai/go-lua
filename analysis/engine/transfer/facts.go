package transfer

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ExprRef is an opaque, comparable reference to a source expression.
type ExprRef uint32

// ValueSourceKind classifies where a value-list slot comes from.
type ValueSourceKind uint8

const (
	ValueSourceUnknown ValueSourceKind = iota
	ValueSourceExpression
	ValueSourceCall
	ValueSourceVararg
	ValueSourceNil
)

// NoValueSourceIndex marks an index field that does not point at a source,
// target, or result slot.
const NoValueSourceIndex = -1

// ValueSource describes one value-list slot without retaining source AST.
type ValueSource struct {
	Kind ValueSourceKind

	ExprRef ExprRef
	HasExpr bool

	ExprIndex   int
	TargetIndex int
	ResultIndex int

	CallPoint    cfg.Point
	HasCallPoint bool

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// LocalAssignment describes a local-root write at a CFG point.
type LocalAssignment struct {
	targetSymbol symbol.ID
	targetPath   path.Path
	source       ValueSource
}

// NewLocalAssignment creates a local-root assignment fact.
func NewLocalAssignment(targetSymbol symbol.ID, targetPath path.Path, source ValueSource) LocalAssignment {
	return LocalAssignment{
		targetSymbol: targetSymbol,
		targetPath:   copyPath(targetPath),
		source:       source,
	}
}

// TargetSymbol returns the assignment target's symbol identity.
func (a LocalAssignment) TargetSymbol() symbol.ID { return a.targetSymbol }

// TargetPath returns the assignment target's path identity.
func (a LocalAssignment) TargetPath() path.Path { return copyPath(a.targetPath) }

// Source returns the value assigned to the target.
func (a LocalAssignment) Source() ValueSource { return a.source }

func (a LocalAssignment) copy() LocalAssignment {
	a.targetPath = copyPath(a.targetPath)
	return a
}

// OrdinaryAssignment describes a non-declaration root write at a CFG point.
type OrdinaryAssignment struct {
	targetSymbol symbol.ID
	targetPath   path.Path
	source       ValueSource
}

// NewOrdinaryAssignment creates a non-declaration root assignment fact.
func NewOrdinaryAssignment(targetSymbol symbol.ID, targetPath path.Path, source ValueSource) OrdinaryAssignment {
	return OrdinaryAssignment{
		targetSymbol: targetSymbol,
		targetPath:   copyPath(targetPath),
		source:       source,
	}
}

// TargetSymbol returns the assignment target's symbol identity.
func (a OrdinaryAssignment) TargetSymbol() symbol.ID { return a.targetSymbol }

// TargetPath returns the assignment target's path identity.
func (a OrdinaryAssignment) TargetPath() path.Path { return copyPath(a.targetPath) }

// Source returns the value assigned to the target.
func (a OrdinaryAssignment) Source() ValueSource { return a.source }

func (a OrdinaryAssignment) copy() OrdinaryAssignment {
	a.targetPath = copyPath(a.targetPath)
	return a
}

// PathAssignment describes a member/path refinement write at a CFG point.
type PathAssignment struct {
	targetPath path.Path
	source     ValueSource
}

// NewPathAssignment creates a member/path assignment fact.
func NewPathAssignment(targetPath path.Path, source ValueSource) PathAssignment {
	return PathAssignment{
		targetPath: copyPath(targetPath),
		source:     source,
	}
}

// TargetPath returns the assignment target's path identity.
func (a PathAssignment) TargetPath() path.Path { return copyPath(a.targetPath) }

// Source returns the value assigned to the target path.
func (a PathAssignment) Source() ValueSource { return a.source }

func (a PathAssignment) copy() PathAssignment {
	a.targetPath = copyPath(a.targetPath)
	return a
}

// ObjectEntry describes one static value written under an object constructor.
type ObjectEntry struct {
	suffix path.Path
	source ValueSource
}

// NewObjectEntry creates a static object-entry descriptor.
func NewObjectEntry(suffix path.Path, source ValueSource) ObjectEntry {
	return ObjectEntry{
		suffix: copyPath(suffix),
		source: source,
	}
}

// Suffix returns the relative static suffix under the constructed object.
func (e ObjectEntry) Suffix() path.Path { return copyPath(e.suffix) }

// Source returns the value assigned to the entry.
func (e ObjectEntry) Source() ValueSource { return e.source }

func (e ObjectEntry) copy() ObjectEntry {
	e.suffix = copyPath(e.suffix)
	return e
}

// ObjectLiteral describes static entries associated with an expression.
type ObjectLiteral struct {
	entries []ObjectEntry
}

// NewObjectLiteral creates an object literal sidecar from static entries.
func NewObjectLiteral(entries []ObjectEntry) ObjectLiteral {
	return ObjectLiteral{entries: copyObjectEntries(entries)}
}

// Entries returns the static entries for this object literal.
func (l ObjectLiteral) Entries() []ObjectEntry { return copyObjectEntries(l.entries) }

func (l ObjectLiteral) copy() ObjectLiteral {
	l.entries = copyObjectEntries(l.entries)
	return l
}

// Assertion describes one user-written assertion expression. The source is the
// asserted inner value; value is an assertion-axis indicator, not proof.
type Assertion struct {
	source ValueSource
	value  assertion.Value
}

// NewAssertion creates an assertion sidecar for an expression.
func NewAssertion(source ValueSource, value assertion.Value) Assertion {
	return Assertion{
		source: source,
		value:  value,
	}
}

// Source returns the asserted inner value source.
func (a Assertion) Source() ValueSource { return a.source }

// Value returns the assertion-axis indicator attached by this sidecar.
func (a Assertion) Value() assertion.Value { return a.value }

func (a Assertion) copy() Assertion { return a }

// ValueRefinement describes one conjunctive product-value refinement. Each axis
// is optional so branch edges can carry only the evidence the condition proves.
type ValueRefinement struct {
	presence    presence.Value
	hasPresence bool

	runtimeKind    runtimekind.Value
	hasRuntimeKind bool
}

// NewValueRefinement creates an empty value refinement.
func NewValueRefinement() ValueRefinement {
	return ValueRefinement{}
}

// WithPresence returns r with a presence refinement.
func (r ValueRefinement) WithPresence(value presence.Value) ValueRefinement {
	r.presence = value
	r.hasPresence = true
	return r
}

// WithRuntimeKind returns r with a runtime-kind refinement.
func (r ValueRefinement) WithRuntimeKind(value runtimekind.Value) ValueRefinement {
	r.runtimeKind = value
	r.hasRuntimeKind = true
	return r
}

// Presence returns the presence refinement, if present.
func (r ValueRefinement) Presence() (presence.Value, bool) {
	return r.presence, r.hasPresence
}

// RuntimeKind returns the runtime-kind refinement, if present.
func (r ValueRefinement) RuntimeKind() (runtimekind.Value, bool) {
	return r.runtimeKind, r.hasRuntimeKind
}

// IsEmpty reports whether r carries no axis refinements.
func (r ValueRefinement) IsEmpty() bool {
	return !r.hasPresence && !r.hasRuntimeKind
}

// BranchRefinement describes branch-edge value refinements for one access path.
// Each edge may be independently absent when the condition gives no fact for
// that direction.
type BranchRefinement struct {
	targetPath path.Path

	trueValue    ValueRefinement
	hasTrueValue bool

	falseValue    ValueRefinement
	hasFalseValue bool
}

// NewBranchRefinement creates a branch refinement fact.
func NewBranchRefinement(
	targetPath path.Path,
	trueValue ValueRefinement,
	hasTrueValue bool,
	falseValue ValueRefinement,
	hasFalseValue bool,
) BranchRefinement {
	return BranchRefinement{
		targetPath:    copyPath(targetPath),
		trueValue:     trueValue,
		hasTrueValue:  hasTrueValue,
		falseValue:    falseValue,
		hasFalseValue: hasFalseValue,
	}
}

// TargetPath returns the refined path.
func (r BranchRefinement) TargetPath() path.Path { return copyPath(r.targetPath) }

// TrueValue returns the true-edge value refinement, if present.
func (r BranchRefinement) TrueValue() (ValueRefinement, bool) {
	return r.trueValue, r.hasTrueValue
}

// FalseValue returns the false-edge value refinement, if present.
func (r BranchRefinement) FalseValue() (ValueRefinement, bool) {
	return r.falseValue, r.hasFalseValue
}

// ValueForEdge returns the refinement selected by a CFG branch edge.
func (r BranchRefinement) ValueForEdge(cond bool) (ValueRefinement, bool) {
	if cond {
		return r.TrueValue()
	}
	return r.FalseValue()
}

func (r BranchRefinement) copy() BranchRefinement {
	r.targetPath = copyPath(r.targetPath)
	return r
}

// Return describes the ordered value sources returned at a CFG point.
type Return struct {
	sources []ValueSource
}

// NewReturn creates a return fact from ordered return-slot sources.
func NewReturn(sources []ValueSource) Return {
	return Return{sources: copyValueSources(sources)}
}

// Sources returns the ordered return-slot sources.
func (r Return) Sources() []ValueSource { return copyValueSources(r.sources) }

func (r Return) copy() Return {
	r.sources = copyValueSources(r.sources)
	return r
}

// CallProducerContext identifies the value-list context that produced a call.
type CallProducerContext uint8

const (
	CallProducerContextUnknown CallProducerContext = iota
	CallProducerContextAssignment
	CallProducerContextReturn
)

// CallResultTargetKind classifies where a call result is consumed.
type CallResultTargetKind uint8

const (
	CallResultTargetUnknown CallResultTargetKind = iota
	CallResultTargetLocalAssignment
	CallResultTargetOrdinaryAssignment
	CallResultTargetReturn
)

// CallResultTarget describes one target that consumes a call result.
type CallResultTarget struct {
	kind         CallResultTargetKind
	index        int
	targetSymbol symbol.ID
	targetPath   path.Path
}

// NewCallResultTarget creates a call result target descriptor.
func NewCallResultTarget(kind CallResultTargetKind, index int, targetSymbol symbol.ID, targetPath path.Path) CallResultTarget {
	return CallResultTarget{
		kind:         kind,
		index:        index,
		targetSymbol: targetSymbol,
		targetPath:   copyPath(targetPath),
	}
}

// Kind returns the target category.
func (t CallResultTarget) Kind() CallResultTargetKind { return t.kind }

// Index returns the target's value-list index.
func (t CallResultTarget) Index() int { return t.index }

// TargetSymbol returns the target's symbol identity.
func (t CallResultTarget) TargetSymbol() symbol.ID { return t.targetSymbol }

// TargetPath returns the target's path identity.
func (t CallResultTarget) TargetPath() path.Path { return copyPath(t.targetPath) }

func (t CallResultTarget) copy() CallResultTarget {
	t.targetPath = copyPath(t.targetPath)
	return t
}

// CallProducerConfig carries constructor input for CallProducer.
type CallProducerConfig struct {
	Context CallProducerContext

	CalleeSymbol symbol.ID
	CalleePath   path.Path

	ExprRef ExprRef
	HasExpr bool

	ExprIndex int

	ResultTargets []CallResultTarget

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// CallProducer describes a top-level assignment or return call producer.
type CallProducer struct {
	context CallProducerContext

	calleeSymbol symbol.ID
	calleePath   path.Path

	exprRef ExprRef
	hasExpr bool

	exprIndex int

	resultTargets []CallResultTarget

	final    bool
	expanded bool
	adjusted bool
	openTail bool
}

// NewCallProducer creates a call producer fact.
func NewCallProducer(config CallProducerConfig) CallProducer {
	return CallProducer{
		context:       config.Context,
		calleeSymbol:  config.CalleeSymbol,
		calleePath:    copyPath(config.CalleePath),
		exprRef:       config.ExprRef,
		hasExpr:       config.HasExpr,
		exprIndex:     config.ExprIndex,
		resultTargets: copyCallResultTargets(config.ResultTargets),
		final:         config.Final,
		expanded:      config.Expanded,
		adjusted:      config.Adjusted,
		openTail:      config.OpenTail,
	}
}

// Context returns the producer's value-list context.
func (c CallProducer) Context() CallProducerContext { return c.context }

// CalleeSymbol returns the callee's symbol identity.
func (c CallProducer) CalleeSymbol() symbol.ID { return c.calleeSymbol }

// CalleePath returns the callee's path identity.
func (c CallProducer) CalleePath() path.Path { return copyPath(c.calleePath) }

// Expr returns the producer expression reference, if present.
func (c CallProducer) Expr() (ExprRef, bool) { return c.exprRef, c.hasExpr }

// ExprIndex returns the expression's index in its containing value list.
func (c CallProducer) ExprIndex() int { return c.exprIndex }

// ResultTargets returns the targets that consume this call's results.
func (c CallProducer) ResultTargets() []CallResultTarget {
	return copyCallResultTargets(c.resultTargets)
}

// Final reports whether this producer is the final value-list expression.
func (c CallProducer) Final() bool { return c.final }

// Expanded reports whether this producer contributes multiple result slots.
func (c CallProducer) Expanded() bool { return c.expanded }

// Adjusted reports whether this producer is adjusted to one result.
func (c CallProducer) Adjusted() bool { return c.adjusted }

// OpenTail reports whether this producer is an open tail return.
func (c CallProducer) OpenTail() bool { return c.openTail }

func (c CallProducer) copy() CallProducer {
	c.calleePath = copyPath(c.calleePath)
	c.resultTargets = copyCallResultTargets(c.resultTargets)
	return c
}

// FactsInput carries point-keyed facts used to construct an immutable Facts snapshot.
type FactsInput struct {
	LocalAssignments    map[cfg.Point]LocalAssignment
	OrdinaryAssignments map[cfg.Point]OrdinaryAssignment
	PathAssignments     map[cfg.Point]PathAssignment
	BranchRefinements   map[cfg.Point]BranchRefinement
	Returns             map[cfg.Point]Return
	Calls               map[cfg.Point]CallProducer
	ObjectLiterals      map[ExprRef]ObjectLiteral
	Assertions          map[ExprRef]Assertion
}

// Facts is an immutable point-keyed transfer facts snapshot.
type Facts struct {
	localAssignments    map[cfg.Point]LocalAssignment
	ordinaryAssignments map[cfg.Point]OrdinaryAssignment
	pathAssignments     map[cfg.Point]PathAssignment
	branchRefinements   map[cfg.Point]BranchRefinement
	returns             map[cfg.Point]Return
	calls               map[cfg.Point]CallProducer
	objectLiterals      map[ExprRef]ObjectLiteral
	assertions          map[ExprRef]Assertion
}

// NewFacts copies the supplied point-keyed facts into an immutable snapshot.
func NewFacts(input FactsInput) Facts {
	return Facts{
		localAssignments:    copyLocalAssignmentMap(input.LocalAssignments),
		ordinaryAssignments: copyOrdinaryAssignmentMap(input.OrdinaryAssignments),
		pathAssignments:     copyPathAssignmentMap(input.PathAssignments),
		branchRefinements:   copyBranchRefinementMap(input.BranchRefinements),
		returns:             copyReturnMap(input.Returns),
		calls:               copyCallProducerMap(input.Calls),
		objectLiterals:      copyObjectLiteralMap(input.ObjectLiterals),
		assertions:          copyAssertionMap(input.Assertions),
	}
}

// LocalAssignment returns the local assignment fact at point.
func (f Facts) LocalAssignment(point cfg.Point) (LocalAssignment, bool) {
	fact, ok := f.localAssignments[point]
	if !ok {
		return LocalAssignment{}, false
	}
	return fact.copy(), true
}

// OrdinaryAssignment returns the ordinary assignment fact at point.
func (f Facts) OrdinaryAssignment(point cfg.Point) (OrdinaryAssignment, bool) {
	fact, ok := f.ordinaryAssignments[point]
	if !ok {
		return OrdinaryAssignment{}, false
	}
	return fact.copy(), true
}

// PathAssignment returns the member/path assignment fact at point.
func (f Facts) PathAssignment(point cfg.Point) (PathAssignment, bool) {
	fact, ok := f.pathAssignments[point]
	if !ok {
		return PathAssignment{}, false
	}
	return fact.copy(), true
}

// BranchRefinement returns the branch-edge value refinement at point.
func (f Facts) BranchRefinement(point cfg.Point) (BranchRefinement, bool) {
	fact, ok := f.branchRefinements[point]
	if !ok {
		return BranchRefinement{}, false
	}
	return fact.copy(), true
}

// Return returns the return fact at point.
func (f Facts) Return(point cfg.Point) (Return, bool) {
	fact, ok := f.returns[point]
	if !ok {
		return Return{}, false
	}
	return fact.copy(), true
}

// Call returns the call producer fact at point.
func (f Facts) Call(point cfg.Point) (CallProducer, bool) {
	fact, ok := f.calls[point]
	if !ok {
		return CallProducer{}, false
	}
	return fact.copy(), true
}

// ObjectLiteral returns the static-entry sidecar for expr, if present.
func (f Facts) ObjectLiteral(expr ExprRef) (ObjectLiteral, bool) {
	fact, ok := f.objectLiterals[expr]
	if !ok {
		return ObjectLiteral{}, false
	}
	return fact.copy(), true
}

// Assertion returns the assertion sidecar for expr, if present.
func (f Facts) Assertion(expr ExprRef) (Assertion, bool) {
	fact, ok := f.assertions[expr]
	if !ok {
		return Assertion{}, false
	}
	return fact.copy(), true
}

// SourceValues resolves ValueSource descriptors into product values.
type SourceValues interface {
	ValueOfSource(point cfg.Point, source ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool)
}

// CallResult is one indexed abstract result produced by a call.
type CallResult struct {
	Index int
	Value product.Value
}

// CallResultProvider resolves generic call-producer facts into indexed return
// slots. Call result targets remain metadata for downstream facts; providers
// produce only ReturnSlot(index) values.
type CallResultProvider func(ctx NodeContext, call CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult

func copyPath(p path.Path) path.Path {
	if len(p.Segments) == 0 {
		return p
	}
	out := p
	out.Segments = append(p.Segments[:0:0], p.Segments...)
	return out
}

func copyValueSources(in []ValueSource) []ValueSource {
	if len(in) == 0 {
		return nil
	}
	out := make([]ValueSource, len(in))
	copy(out, in)
	return out
}

func copyObjectEntries(in []ObjectEntry) []ObjectEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]ObjectEntry, len(in))
	for i := range in {
		out[i] = in[i].copy()
	}
	return out
}

func copyCallResultTargets(in []CallResultTarget) []CallResultTarget {
	if len(in) == 0 {
		return nil
	}
	out := make([]CallResultTarget, len(in))
	for i := range in {
		out[i] = in[i].copy()
	}
	return out
}

func copyLocalAssignmentMap(in map[cfg.Point]LocalAssignment) map[cfg.Point]LocalAssignment {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]LocalAssignment, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

func copyOrdinaryAssignmentMap(in map[cfg.Point]OrdinaryAssignment) map[cfg.Point]OrdinaryAssignment {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]OrdinaryAssignment, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

func copyPathAssignmentMap(in map[cfg.Point]PathAssignment) map[cfg.Point]PathAssignment {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]PathAssignment, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

func copyBranchRefinementMap(in map[cfg.Point]BranchRefinement) map[cfg.Point]BranchRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]BranchRefinement, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

func copyReturnMap(in map[cfg.Point]Return) map[cfg.Point]Return {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]Return, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

func copyCallProducerMap(in map[cfg.Point]CallProducer) map[cfg.Point]CallProducer {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]CallProducer, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

func copyObjectLiteralMap(in map[ExprRef]ObjectLiteral) map[ExprRef]ObjectLiteral {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]ObjectLiteral, len(in))
	for expr, fact := range in {
		out[expr] = fact.copy()
	}
	return out
}

func copyAssertionMap(in map[ExprRef]Assertion) map[ExprRef]Assertion {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]Assertion, len(in))
	for expr, fact := range in {
		out[expr] = fact.copy()
	}
	return out
}
