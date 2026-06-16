package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
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
	Context CallSiteContext

	CalleeSymbol symbol.ID
	CalleePath   path.Path

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

	receiverSource    ValueSource
	hasReceiverSource bool

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

// CallSiteView provides read-only access to call-site evidence without
// exposing mutable internal slices or path segment storage.
type CallSiteView struct {
	site CallSite
}

// NewCallSiteView wraps an owned call site in a read-only view.
func NewCallSiteView(site CallSite) CallSiteView { return CallSiteView{site: site} }

// View returns a read-only view of the owned call site.
func (c CallSite) View() CallSiteView { return CallSiteView{site: c} }

// NewCallSite creates a call-site evidence fact.
func NewCallSite(config CallSiteConfig) CallSite {
	return CallSite{
		context:           config.Context,
		calleeSymbol:      config.CalleeSymbol,
		calleePath:        copyPath(config.CalleePath),
		receiverPath:      copyPath(config.ReceiverPath),
		hasReceiverPath:   config.HasReceiverPath,
		methodPath:        copyPath(config.MethodPath),
		hasMethodPath:     config.HasMethodPath,
		methodName:        config.MethodName,
		receiverSource:    config.ReceiverSource,
		hasReceiverSource: config.HasReceiverSource,
		exprRef:           config.ExprRef,
		hasExpr:           config.HasExpr,
		exprIndex:         config.ExprIndex,
		argumentSources:   copyValueSources(config.ArgumentSources),
		typeArgs:          copyTypeRefs(config.TypeArgs),
		resultTargets:     copyCallResultTargets(config.ResultTargets),
		final:             config.Final,
		expanded:          config.Expanded,
		adjusted:          config.Adjusted,
		openTail:          config.OpenTail,
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

// ReceiverSource returns the value source for a colon-method call's receiver
// expression, if the receiver was not a resolvable symbol path.
func (c CallSite) ReceiverSource() (ValueSource, bool) {
	return c.receiverSource, c.hasReceiverSource
}

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

// CallSite returns a defensive copy of the viewed call-site evidence.
func (v CallSiteView) CallSite() CallSite { return v.site.copy() }

// Context returns the call site's semantic context.
func (v CallSiteView) Context() CallSiteContext { return v.site.context }

// CalleeSymbol returns the callee's symbol identity.
func (v CallSiteView) CalleeSymbol() symbol.ID { return v.site.calleeSymbol }

// CalleePath returns a defensive copy of the callee's path identity.
func (v CallSiteView) CalleePath() path.Path { return copyPath(v.site.calleePath) }

// CalleePathKey returns the callee path's structural key.
func (v CallSiteView) CalleePathKey() path.PathKey { return v.site.calleePath.Key() }

// CalleePathEqual reports whether p matches the callee path.
func (v CallSiteView) CalleePathEqual(p path.Path) bool { return v.site.calleePath.Equal(p) }

// ReceiverPath returns the receiver path identity, if one was resolved.
func (v CallSiteView) ReceiverPath() (path.Path, bool) {
	return copyPath(v.site.receiverPath), v.site.hasReceiverPath
}

// MethodPath returns the receiver-method path identity, if one was resolved.
func (v CallSiteView) MethodPath() (path.Path, bool) {
	return copyPath(v.site.methodPath), v.site.hasMethodPath
}

// MethodName returns the method name carried by receiver-call syntax.
func (v CallSiteView) MethodName() string { return v.site.methodName }

// Expr returns the call expression reference, if present.
func (v CallSiteView) Expr() (ExprRef, bool) { return v.site.exprRef, v.site.hasExpr }

// ExprIndex returns the expression's index in its containing value list.
func (v CallSiteView) ExprIndex() int { return v.site.exprIndex }

// ArgumentSources returns the ordered argument value sources.
func (v CallSiteView) ArgumentSources() []ValueSource {
	return copyValueSources(v.site.argumentSources)
}

// TypeArgs returns the ordered explicit type argument identities.
func (v CallSiteView) TypeArgs() []TypeRef { return copyTypeRefs(v.site.typeArgs) }

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
	c.calleePath = copyPath(c.calleePath)
	c.receiverPath = copyPath(c.receiverPath)
	c.methodPath = copyPath(c.methodPath)
	c.argumentSources = copyValueSources(c.argumentSources)
	c.typeArgs = copyTypeRefs(c.typeArgs)
	c.resultTargets = copyCallResultTargets(c.resultTargets)
	return c
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
