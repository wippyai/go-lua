package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ExprRef is an opaque, comparable reference to a source expression.
type ExprRef uint32

// TypeRef is an opaque, comparable reference to a source type expression.
type TypeRef uint32

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

// RootAssignment describes a root-symbol write at a CFG point.
type RootAssignment struct {
	targetSymbol symbol.ID
	targetPath   path.Path
	source       ValueSource
}

// NewRootAssignment creates a root-symbol assignment fact.
func NewRootAssignment(targetSymbol symbol.ID, targetPath path.Path, source ValueSource) RootAssignment {
	return RootAssignment{
		targetSymbol: targetSymbol,
		targetPath:   copyPath(targetPath),
		source:       source,
	}
}

// TargetSymbol returns the assignment target's symbol identity.
func (a RootAssignment) TargetSymbol() symbol.ID { return a.targetSymbol }

// TargetPath returns the assignment target's path identity.
func (a RootAssignment) TargetPath() path.Path { return copyPath(a.targetPath) }

// Source returns the value assigned to the target.
func (a RootAssignment) Source() ValueSource { return a.source }

func (a RootAssignment) copy() RootAssignment {
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

// ValueOverlay describes a source expression whose value is resolved from an
// inner source and then conjunctively overlaid with a product value.
type ValueOverlay struct {
	source  ValueSource
	overlay product.Value
}

// NewValueOverlay creates a source-value overlay sidecar for an expression.
func NewValueOverlay(source ValueSource, overlay product.Value) ValueOverlay {
	return ValueOverlay{
		source:  source,
		overlay: overlay,
	}
}

// Source returns the inner value source.
func (o ValueOverlay) Source() ValueSource { return o.source }

// Overlay returns the product value met onto the resolved inner source value.
func (o ValueOverlay) Overlay() product.Value { return o.overlay }

func (o ValueOverlay) copy() ValueOverlay { return o }

// ValueRefinement describes one conjunctive product-value constraint. The
// explicit has bit distinguishes "no edge constraint" from a deliberate
// product.Top() no-op constraint.
type ValueRefinement struct {
	constraint    product.Value
	hasConstraint bool
}

// NewValueRefinement creates an empty value refinement.
func NewValueRefinement() ValueRefinement {
	return ValueRefinement{}
}

// NewValueConstraint creates a value refinement from an already-built product
// constraint.
func NewValueConstraint(constraint product.Value) ValueRefinement {
	return ValueRefinement{constraint: constraint, hasConstraint: true}
}

// WithConstraint returns r additionally constrained by constraint.
func (r ValueRefinement) WithConstraint(reg *axis.Registry, constraint product.Value) ValueRefinement {
	if !r.hasConstraint {
		r.constraint = constraint
		r.hasConstraint = true
		return r
	}
	r.constraint = product.Meet(reg, r.constraint, constraint)
	return r
}

// Constraint returns the product constraint, if present.
func (r ValueRefinement) Constraint() (product.Value, bool) {
	return r.constraint, r.hasConstraint
}

// IsEmpty reports whether r carries no axis refinements.
func (r ValueRefinement) IsEmpty() bool {
	return !r.hasConstraint
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

// CallSiteContext identifies the semantic context in which a call appears.
type CallSiteContext uint8

const (
	CallSiteContextUnknown CallSiteContext = iota
	CallSiteContextStatement
	CallSiteContextAssignmentSource
	CallSiteContextReturnSource
	CallSiteContextIteratorSource
	CallSiteContextCondition
)

// CallSiteConfig carries constructor input for CallSite.
type CallSiteConfig struct {
	Context CallSiteContext

	CalleeSymbol symbol.ID
	CalleePath   path.Path

	ReceiverPath    path.Path
	HasReceiverPath bool
	MethodPath      path.Path
	HasMethodPath   bool
	MethodName      string

	ExprRef ExprRef
	HasExpr bool

	ExprIndex int

	ArgumentSources []ValueSource
	TypeArgs        []TypeRef
	ResultTargets   []CallResultTarget

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// CallSite describes a semantic call occurrence used as canonical evidence.
type CallSite struct {
	context CallSiteContext

	calleeSymbol symbol.ID
	calleePath   path.Path

	receiverPath    path.Path
	hasReceiverPath bool
	methodPath      path.Path
	hasMethodPath   bool
	methodName      string

	exprRef ExprRef
	hasExpr bool

	exprIndex int

	argumentSources []ValueSource
	typeArgs        []TypeRef
	resultTargets   []CallResultTarget

	final    bool
	expanded bool
	adjusted bool
	openTail bool
}

// NewCallSite creates a call-site evidence fact.
func NewCallSite(config CallSiteConfig) CallSite {
	return CallSite{
		context:         config.Context,
		calleeSymbol:    config.CalleeSymbol,
		calleePath:      copyPath(config.CalleePath),
		receiverPath:    copyPath(config.ReceiverPath),
		hasReceiverPath: config.HasReceiverPath,
		methodPath:      copyPath(config.MethodPath),
		hasMethodPath:   config.HasMethodPath,
		methodName:      config.MethodName,
		exprRef:         config.ExprRef,
		hasExpr:         config.HasExpr,
		exprIndex:       config.ExprIndex,
		argumentSources: copyValueSources(config.ArgumentSources),
		typeArgs:        copyTypeRefs(config.TypeArgs),
		resultTargets:   copyCallResultTargets(config.ResultTargets),
		final:           config.Final,
		expanded:        config.Expanded,
		adjusted:        config.Adjusted,
		openTail:        config.OpenTail,
	}
}

// Context returns the call site's semantic context.
func (c CallSite) Context() CallSiteContext { return c.context }

// CalleeSymbol returns the callee's symbol identity.
func (c CallSite) CalleeSymbol() symbol.ID { return c.calleeSymbol }

// CalleePath returns the callee's path identity.
func (c CallSite) CalleePath() path.Path { return copyPath(c.calleePath) }

// ReceiverPath returns the receiver path identity, if one was resolved.
func (c CallSite) ReceiverPath() (path.Path, bool) {
	return copyPath(c.receiverPath), c.hasReceiverPath
}

// MethodPath returns the receiver-method path identity, if one was resolved.
func (c CallSite) MethodPath() (path.Path, bool) {
	return copyPath(c.methodPath), c.hasMethodPath
}

// MethodName returns the method name carried by receiver-call syntax.
func (c CallSite) MethodName() string { return c.methodName }

// Expr returns the call expression reference, if present.
func (c CallSite) Expr() (ExprRef, bool) { return c.exprRef, c.hasExpr }

// ExprIndex returns the expression's index in its containing value list.
func (c CallSite) ExprIndex() int { return c.exprIndex }

// ArgumentSources returns the ordered argument value sources.
func (c CallSite) ArgumentSources() []ValueSource {
	return copyValueSources(c.argumentSources)
}

// TypeArgs returns the ordered explicit type argument identities.
func (c CallSite) TypeArgs() []TypeRef { return copyTypeRefs(c.typeArgs) }

// ResultTargets returns the targets that consume this call's results.
func (c CallSite) ResultTargets() []CallResultTarget {
	return copyCallResultTargets(c.resultTargets)
}

// Final reports whether this call is the final value-list expression.
func (c CallSite) Final() bool { return c.final }

// Expanded reports whether this call contributes multiple result slots.
func (c CallSite) Expanded() bool { return c.expanded }

// Adjusted reports whether this call is adjusted to one result.
func (c CallSite) Adjusted() bool { return c.adjusted }

// OpenTail reports whether this call is an open tail return.
func (c CallSite) OpenTail() bool { return c.openTail }

func (c CallSite) copy() CallSite {
	c.calleePath = copyPath(c.calleePath)
	c.receiverPath = copyPath(c.receiverPath)
	c.methodPath = copyPath(c.methodPath)
	c.argumentSources = copyValueSources(c.argumentSources)
	c.typeArgs = copyTypeRefs(c.typeArgs)
	c.resultTargets = copyCallResultTargets(c.resultTargets)
	return c
}

// FactsInput carries point-keyed facts used to construct an immutable Facts snapshot.
type FactsInput struct {
	LocalAssignments    map[cfg.Point]RootAssignment
	OrdinaryAssignments map[cfg.Point]RootAssignment
	PathAssignments     map[cfg.Point]PathAssignment
	BranchRefinements   map[cfg.Point]BranchRefinement
	Returns             map[cfg.Point]Return
	Calls               map[cfg.Point]CallProducer
	CallSites           map[cfg.Point]CallSite
	ObjectLiterals      map[ExprRef]ObjectLiteral
	ValueOverlays       map[ExprRef]ValueOverlay
}

// Facts is an immutable point-keyed transfer facts snapshot.
type Facts struct {
	localAssignments    map[cfg.Point]RootAssignment
	ordinaryAssignments map[cfg.Point]RootAssignment
	pathAssignments     map[cfg.Point]PathAssignment
	branchRefinements   map[cfg.Point]BranchRefinement
	returns             map[cfg.Point]Return
	calls               map[cfg.Point]CallProducer
	callSites           map[cfg.Point]CallSite
	objectLiterals      map[ExprRef]ObjectLiteral
	valueOverlays       map[ExprRef]ValueOverlay
}

// NewFacts copies the supplied point-keyed facts into an immutable snapshot.
func NewFacts(input FactsInput) Facts {
	return Facts{
		localAssignments:    copyRootAssignmentMap(input.LocalAssignments),
		ordinaryAssignments: copyRootAssignmentMap(input.OrdinaryAssignments),
		pathAssignments:     copyPathAssignmentMap(input.PathAssignments),
		branchRefinements:   copyBranchRefinementMap(input.BranchRefinements),
		returns:             copyReturnMap(input.Returns),
		calls:               copyCallProducerMap(input.Calls),
		callSites:           copyCallSiteMap(input.CallSites),
		objectLiterals:      copyObjectLiteralMap(input.ObjectLiterals),
		valueOverlays:       copyValueOverlayMap(input.ValueOverlays),
	}
}

// LocalAssignment returns the local assignment fact at point.
func (f Facts) LocalAssignment(point cfg.Point) (RootAssignment, bool) {
	fact, ok := f.localAssignments[point]
	if !ok {
		return RootAssignment{}, false
	}
	return fact.copy(), true
}

// OrdinaryAssignment returns the ordinary assignment fact at point.
func (f Facts) OrdinaryAssignment(point cfg.Point) (RootAssignment, bool) {
	fact, ok := f.ordinaryAssignments[point]
	if !ok {
		return RootAssignment{}, false
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

// CallSite returns the call-site evidence fact at point.
func (f Facts) CallSite(point cfg.Point) (CallSite, bool) {
	fact, ok := f.callSites[point]
	if !ok {
		return CallSite{}, false
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

// ValueOverlay returns the source-value overlay sidecar for expr, if present.
func (f Facts) ValueOverlay(expr ExprRef) (ValueOverlay, bool) {
	fact, ok := f.valueOverlays[expr]
	if !ok {
		return ValueOverlay{}, false
	}
	return fact.copy(), true
}

// ValueOverlays returns the source-value overlay sidecars keyed by expression.
func (f Facts) ValueOverlays() map[ExprRef]ValueOverlay {
	return copyValueOverlayMap(f.valueOverlays)
}

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

func copyTypeRefs(in []TypeRef) []TypeRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]TypeRef, len(in))
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

func copyRootAssignmentMap(in map[cfg.Point]RootAssignment) map[cfg.Point]RootAssignment {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]RootAssignment, len(in))
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

func copyCallSiteMap(in map[cfg.Point]CallSite) map[cfg.Point]CallSite {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]CallSite, len(in))
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

func copyValueOverlayMap(in map[ExprRef]ValueOverlay) map[ExprRef]ValueOverlay {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]ValueOverlay, len(in))
	for expr, fact := range in {
		out[expr] = fact.copy()
	}
	return out
}
