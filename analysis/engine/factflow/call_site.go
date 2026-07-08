package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// CallSiteContext identifies the semantic context in which a call appears.
type CallSiteContext uint8

const (
	CallSiteContextUnknown CallSiteContext = iota
	CallSiteContextStatement
	CallSiteContextAssignmentSource
	CallSiteContextReturnSource
	CallSiteContextIteratorSource
	CallSiteContextCondition
	CallSiteContextExpressionProducer
)

// CallSiteConfig carries constructor input for CallSite.
type CallSiteConfig struct {
	Context  CallSiteContext
	Point    cfg.Point
	HasPoint bool

	CalleeSymbol       symbol.ID
	CalleePath         path.Path
	CalleeMemberAccess bool

	ReceiverPath    path.Path
	HasReceiverPath bool
	MethodPath      path.Path
	HasMethodPath   bool
	MethodName      string

	ReceiverSource    ValueSource
	HasReceiverSource bool

	ExprRef ExprRef
	HasExpr bool

	ExprIndex int
	// ConditionNegated is true when a condition call is the operand of unary
	// `not`, so the CFG true edge corresponds to the call's falsy result.
	ConditionNegated bool

	ArgumentSources []ValueSource
	CallSpan        SourceSpan
	CalleeSpan      SourceSpan
	ArgumentSpans   []SourceSpan
	ArgumentLabels  []string
	TypeArgs        []TypeRef
	ResultTargets   []CallResultTarget

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// CalleePathKey is the structural key spelling of a resolvable callsite callee
// path. It is intentionally tied to path.Path.Key(), not stable address keys:
// definition-path summary lookup must match the callee path spelling carried by
// callsite evidence.
type CalleePathKey path.PathKey

// PathKey returns the underlying structural path key for compatibility with
// existing map and test APIs.
func (k CalleePathKey) PathKey() path.PathKey { return path.PathKey(k) }

func (k CalleePathKey) String() string { return string(k) }

// Valid reports whether k is a non-empty callee path key.
func (k CalleePathKey) Valid() bool { return k != "" }

// CalleePathKeyFromPath formats a non-empty callee path as a typed key.
func CalleePathKeyFromPath(p path.Path) (CalleePathKey, bool) {
	if p.IsEmpty() {
		return "", false
	}
	return CalleePathKey(p.Key()), true
}

// CalleePathKeyFromPathKey narrows an existing structural path key for
// compatibility boundaries.
func CalleePathKeyFromPathKey(key path.PathKey) (CalleePathKey, bool) {
	if key == "" {
		return "", false
	}
	return CalleePathKey(key), true
}

// CallSite describes a semantic call occurrence used as canonical evidence.
type CallSite struct {
	context  CallSiteContext
	point    cfg.Point
	hasPoint bool

	calleeSymbol       symbol.ID
	calleePath         path.Path
	calleeKey          CalleePathKey
	calleeMemberAccess bool

	receiverPath    path.Path
	hasReceiverPath bool
	methodPath      path.Path
	hasMethodPath   bool
	methodName      string

	receiverSource    ValueSource
	hasReceiverSource bool

	exprRef ExprRef
	hasExpr bool

	exprIndex        int
	conditionNegated bool

	argumentSources []ValueSource
	callSpan        SourceSpan
	calleeSpan      SourceSpan
	argumentSpans   []SourceSpan
	argumentLabels  []string
	typeArgs        []TypeRef
	resultTargets   []CallResultTarget

	final    bool
	expanded bool
	adjusted bool
	openTail bool
}

// CallSiteView provides read-only access to call-site evidence without
// exposing mutable internal slices or path segment storage.
type CallSiteView struct {
	site CallSite
}

// View returns a read-only view of the owned call site.
func (c CallSite) View() CallSiteView { return CallSiteView{site: c} }

// NewCallSite creates a call-site evidence fact.
func NewCallSite(config CallSiteConfig) CallSite {
	calleePath := config.CalleePath.Clone()
	calleeKey, _ := CalleePathKeyFromPath(calleePath)
	return CallSite{
		context:            config.Context,
		point:              config.Point,
		hasPoint:           config.HasPoint,
		calleeSymbol:       config.CalleeSymbol,
		calleePath:         calleePath,
		calleeKey:          calleeKey,
		calleeMemberAccess: config.CalleeMemberAccess,
		receiverPath:       config.ReceiverPath.Clone(),
		hasReceiverPath:    config.HasReceiverPath,
		methodPath:         config.MethodPath.Clone(),
		hasMethodPath:      config.HasMethodPath,
		methodName:         config.MethodName,
		receiverSource:     config.ReceiverSource,
		hasReceiverSource:  config.HasReceiverSource,
		exprRef:            config.ExprRef,
		hasExpr:            config.HasExpr,
		exprIndex:          config.ExprIndex,
		conditionNegated:   config.ConditionNegated,
		argumentSources:    copyValueSources(config.ArgumentSources),
		callSpan:           config.CallSpan,
		calleeSpan:         config.CalleeSpan,
		argumentSpans:      copySourceSpans(config.ArgumentSpans),
		argumentLabels:     append([]string(nil), config.ArgumentLabels...),
		typeArgs:           append([]TypeRef(nil), config.TypeArgs...),
		resultTargets:      copyCallResultTargets(config.ResultTargets),
		final:              config.Final,
		expanded:           config.Expanded,
		adjusted:           config.Adjusted,
		openTail:           config.OpenTail,
	}
}

// Context returns the call site's semantic context.
func (c CallSite) Context() CallSiteContext { return c.context }

// Point returns the CFG point that owns this call site, if lowering supplied it.
func (c CallSite) Point() (cfg.Point, bool) { return c.point, c.hasPoint }

// CalleeSymbol returns the callee's symbol identity.
func (c CallSite) CalleeSymbol() symbol.ID { return c.calleeSymbol }

// CalleePath returns the callee's path identity.
func (c CallSite) CalleePath() path.Path { return c.calleePath.Clone() }

// CalleePathRef returns the callee path without a defensive copy for read-only
// use or for handing to a constructor that clones on store. The returned path
// shares the fact's segment storage and must never be mutated in place.
func (c CallSite) CalleePathRef() path.Path { return c.calleePath }

// CalleeMemberAccess reports whether this call's callee was written through
// member-access syntax or resolved to a member path. The lowering phase owns
// this syntax-derived evidence so post-solve consumers do not re-read the AST.
func (c CallSite) CalleeMemberAccess() bool { return c.calleeMemberAccess }

// CalleeMemberAccessPath returns the receiver path and member segment for a
// member-access callee. This is the canonical structural query for post-solve
// consumers that need the path shape; they should not re-parse CalleePathRef.
func (c CallSite) CalleeMemberAccessPath() (path.Path, segment.Segment, bool) {
	return callSiteMemberAccessPath(c)
}

// ReceiverPath returns the receiver path identity, if one was resolved.
func (c CallSite) ReceiverPath() (path.Path, bool) {
	return c.receiverPath.Clone(), c.hasReceiverPath
}

// MethodPath returns the receiver-method path identity, if one was resolved.
func (c CallSite) MethodPath() (path.Path, bool) {
	return c.methodPath.Clone(), c.hasMethodPath
}

// MethodName returns the method name carried by receiver-call syntax.
func (c CallSite) MethodName() string { return c.methodName }

// ReceiverSource returns the value source for a colon-method call's receiver
// expression, if the receiver was not a resolvable symbol path.
func (c CallSite) ReceiverSource() (ValueSource, bool) {
	return c.receiverSource, c.hasReceiverSource
}

// Expr returns the call expression reference, if present.
func (c CallSite) Expr() (ExprRef, bool) { return c.exprRef, c.hasExpr }

// ExprIndex returns the expression's index in its containing value list.
func (c CallSite) ExprIndex() int { return c.exprIndex }

// ConditionNegated reports whether this condition call is wrapped by unary not.
func (c CallSite) ConditionNegated() bool { return c.conditionNegated }

// ArgumentSources returns the ordered argument value sources.
func (c CallSite) ArgumentSources() []ValueSource {
	return copyValueSources(c.argumentSources)
}

// CallSpan returns the source range for the whole call expression.
func (c CallSite) CallSpan() SourceSpan { return c.callSpan }

// CalleeSpan returns the source range for the call target expression.
func (c CallSite) CalleeSpan() SourceSpan { return c.calleeSpan }

// ArgumentSourceCount returns the number of ordered argument value sources.
func (c CallSite) ArgumentSourceCount() int { return len(c.argumentSources) }

// ArgumentSourceAt returns one argument value source by value.
func (c CallSite) ArgumentSourceAt(index int) (ValueSource, bool) {
	if index < 0 || index >= len(c.argumentSources) {
		return ValueSource{}, false
	}
	return c.argumentSources[index], true
}

func (c CallSite) ArgumentSpanAt(index int) (SourceSpan, bool) {
	if index < 0 || index >= len(c.argumentSpans) {
		return SourceSpan{}, false
	}
	return c.argumentSpans[index], true
}

func (c CallSite) ArgumentLabelAt(index int) (string, bool) {
	if index < 0 || index >= len(c.argumentLabels) {
		return "", false
	}
	return c.argumentLabels[index], c.argumentLabels[index] != ""
}

// ForEachArgumentSource visits argument value sources without allocating a
// defensive slice. Returning false stops iteration.
func (c CallSite) ForEachArgumentSource(fn func(index int, source ValueSource) bool) {
	if fn == nil {
		return
	}
	for i := range c.argumentSources {
		if !fn(i, c.argumentSources[i]) {
			return
		}
	}
}

// TypeArgs returns the ordered explicit type argument identities.
func (c CallSite) TypeArgs() []TypeRef { return append([]TypeRef(nil), c.typeArgs...) }

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

// CallSite returns a defensive copy of the viewed call-site evidence.
func (v CallSiteView) CallSite() CallSite { return v.site.copy() }

// Context returns the call site's semantic context.
func (v CallSiteView) Context() CallSiteContext { return v.site.context }

// Point returns the CFG point that owns this call site, if lowering supplied it.
func (v CallSiteView) Point() (cfg.Point, bool) { return v.site.point, v.site.hasPoint }

// CalleeSymbol returns the callee's symbol identity.
func (v CallSiteView) CalleeSymbol() symbol.ID { return v.site.calleeSymbol }

// CalleePath returns a defensive copy of the callee's path identity.
func (v CallSiteView) CalleePath() path.Path { return v.site.calleePath.Clone() }

// CalleePathRef returns the callee path without a defensive copy for read-only
// use or for handing to a constructor that clones on store. The returned path
// shares the fact's segment storage and must never be mutated in place.
func (v CallSiteView) CalleePathRef() path.Path { return v.site.calleePath }

// CalleePathKey returns the callee path's typed structural key.
func (v CallSiteView) CalleePathKey() CalleePathKey {
	return v.site.calleeKey
}

// CalleePathEqual reports whether p matches the callee path.
func (v CallSiteView) CalleePathEqual(p path.Path) bool { return v.site.calleePath.Equal(p) }

// CalleeMemberAccess reports whether this call's callee was written through
// member-access syntax or resolved to a member path.
func (v CallSiteView) CalleeMemberAccess() bool { return v.site.calleeMemberAccess }

// CalleeMemberAccessPath returns the receiver path and member segment for a
// member-access callee.
func (v CallSiteView) CalleeMemberAccessPath() (path.Path, segment.Segment, bool) {
	return callSiteMemberAccessPath(v.site)
}

// ReceiverPath returns the receiver path identity, if one was resolved.
func (v CallSiteView) ReceiverPath() (path.Path, bool) {
	return v.site.receiverPath.Clone(), v.site.hasReceiverPath
}

// MethodPath returns the receiver-method path identity, if one was resolved.
func (v CallSiteView) MethodPath() (path.Path, bool) {
	return v.site.methodPath.Clone(), v.site.hasMethodPath
}

// MethodName returns the method name carried by receiver-call syntax.
func (v CallSiteView) MethodName() string { return v.site.methodName }

// ReceiverSource returns the value source for a colon-method call's receiver
// expression, if the receiver was not a resolvable symbol path.
func (v CallSiteView) ReceiverSource() (ValueSource, bool) {
	return v.site.receiverSource, v.site.hasReceiverSource
}

// Expr returns the call expression reference, if present.
func (v CallSiteView) Expr() (ExprRef, bool) { return v.site.exprRef, v.site.hasExpr }

// ExprIndex returns the expression's index in its containing value list.
func (v CallSiteView) ExprIndex() int { return v.site.exprIndex }

// ConditionNegated reports whether this condition call is wrapped by unary not.
func (v CallSiteView) ConditionNegated() bool { return v.site.conditionNegated }

// ArgumentSources returns the ordered argument value sources.
func (v CallSiteView) ArgumentSources() []ValueSource {
	return copyValueSources(v.site.argumentSources)
}

// CallSpan returns the source range for the whole call expression.
func (v CallSiteView) CallSpan() SourceSpan { return v.site.callSpan }

// CalleeSpan returns the source range for the call target expression.
func (v CallSiteView) CalleeSpan() SourceSpan { return v.site.calleeSpan }

// ArgumentSourceCount returns the number of ordered argument value sources.
func (v CallSiteView) ArgumentSourceCount() int { return len(v.site.argumentSources) }

// ArgumentSourceAt returns one argument value source by value.
func (v CallSiteView) ArgumentSourceAt(index int) (ValueSource, bool) {
	if index < 0 || index >= len(v.site.argumentSources) {
		return ValueSource{}, false
	}
	return v.site.argumentSources[index], true
}

func (v CallSiteView) ArgumentSpanAt(index int) (SourceSpan, bool) {
	if index < 0 || index >= len(v.site.argumentSpans) {
		return SourceSpan{}, false
	}
	return v.site.argumentSpans[index], true
}

func (v CallSiteView) ArgumentLabelAt(index int) (string, bool) {
	if index < 0 || index >= len(v.site.argumentLabels) {
		return "", false
	}
	return v.site.argumentLabels[index], v.site.argumentLabels[index] != ""
}

// ForEachArgumentSource visits argument value sources without allocating a
// defensive slice. Returning false stops iteration.
func (v CallSiteView) ForEachArgumentSource(fn func(index int, source ValueSource) bool) {
	if fn == nil {
		return
	}
	for i := range v.site.argumentSources {
		if !fn(i, v.site.argumentSources[i]) {
			return
		}
	}
}

// TypeArgs returns the ordered explicit type argument identities.
func (v CallSiteView) TypeArgs() []TypeRef { return append([]TypeRef(nil), v.site.typeArgs...) }

// TypeArgCount returns the number of explicit type argument identities.
func (v CallSiteView) TypeArgCount() int { return len(v.site.typeArgs) }

// TypeArgAt returns one explicit type argument identity by index.
func (v CallSiteView) TypeArgAt(index int) (TypeRef, bool) {
	if index < 0 || index >= len(v.site.typeArgs) {
		return 0, false
	}
	return v.site.typeArgs[index], true
}

// Final reports whether this call is the final value-list expression.
func (v CallSiteView) Final() bool { return v.site.final }

// Expanded reports whether this call contributes multiple result slots.
func (v CallSiteView) Expanded() bool { return v.site.expanded }

// Adjusted reports whether this call is adjusted to one result.
func (v CallSiteView) Adjusted() bool { return v.site.adjusted }

// OpenTail reports whether this call is an open tail return.
func (v CallSiteView) OpenTail() bool { return v.site.openTail }

// ResultTargetCount returns the number of result targets.
func (v CallSiteView) ResultTargetCount() int { return len(v.site.resultTargets) }

// ResultTargetAt returns one result target view by index.
func (v CallSiteView) ResultTargetAt(index int) (CallResultTargetView, bool) {
	if index < 0 || index >= len(v.site.resultTargets) {
		return CallResultTargetView{}, false
	}
	return CallResultTargetView{target: v.site.resultTargets[index]}, true
}

// ForEachResultTarget visits the call site's result targets without exposing
// mutable internal slices. Returning false stops iteration.
func (v CallSiteView) ForEachResultTarget(fn func(CallResultTargetView) bool) {
	if fn == nil {
		return
	}
	for i := range v.site.resultTargets {
		if !fn(CallResultTargetView{target: v.site.resultTargets[i]}) {
			return
		}
	}
}

func (c CallSite) copy() CallSite {
	c.calleePath = c.calleePath.Clone()
	c.receiverPath = c.receiverPath.Clone()
	c.methodPath = c.methodPath.Clone()
	c.argumentSources = copyValueSources(c.argumentSources)
	c.argumentSpans = copySourceSpans(c.argumentSpans)
	c.argumentLabels = append([]string(nil), c.argumentLabels...)
	c.typeArgs = append([]TypeRef(nil), c.typeArgs...)
	c.resultTargets = copyCallResultTargets(c.resultTargets)
	return c
}

func callSiteMemberAccessPath(c CallSite) (path.Path, segment.Segment, bool) {
	if !c.calleeMemberAccess {
		return path.Path{}, segment.Segment{}, false
	}
	if c.hasReceiverPath && c.methodName != "" {
		return c.receiverPath.Clone(), segment.Segment{Kind: segment.SegmentField, Name: c.methodName}, true
	}
	if c.calleePath.IsEmpty() || len(c.calleePath.Segments) == 0 {
		return path.Path{}, segment.Segment{}, false
	}
	last := c.calleePath.Segments[len(c.calleePath.Segments)-1]
	switch last.Kind {
	case segment.SegmentField, segment.SegmentIndexString, segment.SegmentIndexInt:
		receiver := c.calleePath.Parent()
		if receiver.IsEmpty() {
			return path.Path{}, segment.Segment{}, false
		}
		return receiver, last, true
	default:
		return path.Path{}, segment.Segment{}, false
	}
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
