package factflow

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// FactsInput carries point-keyed facts used to construct an immutable Facts snapshot.
type FactsInput struct {
	RootAssignments               map[cfg.Point]RootAssignment
	PathAssignments               map[cfg.Point]PathAssignment
	PathStaticMemberWrites        map[cfg.Point]PathStaticMemberWrite
	DynamicIndexWrites            map[cfg.Point]DynamicIndexWrite
	PathDescendantInvalidations   map[cfg.Point]PathDescendantInvalidation
	CovariantExposures            map[cfg.Point][]CovariantExposure
	NoNormalReturns               map[cfg.Point]struct{}
	BranchEdgeReachability        map[cfg.Point]BranchEdgeReachability
	BranchConditionSources        map[cfg.Point]ValueSource
	BranchRefinements             map[cfg.Point]BranchRefinementSet
	BranchPresenceRelations       map[cfg.Point]BranchPresenceRelationSet
	BranchPathRelations           map[cfg.Point]BranchPathRelationSet
	BranchPathEvidence            map[cfg.Point]BranchPathEvidenceSet
	BranchSufficientLiteralCases  map[cfg.Point]BranchSufficientLiteralCaseSet
	PathValuePresenceImplications map[cfg.Point]PathValuePresenceImplicationSet
	ChannelSelects                map[cfg.Point]ChannelSelectSet
	PostconditionRefinements      map[cfg.Point]PostconditionRefinementSet
	PostconditionPathRelations    map[cfg.Point]PostconditionPathRelationSet
	CallResultValues              map[cfg.Point]CallResultValueSet
	ReturnPresenceRelations       map[cfg.Point]ReturnPresenceRelationSet
	Returns                       map[cfg.Point]Return
	CallSites                     map[cfg.Point]CallSite
	ObjectLiterals                map[ExprRef]ObjectLiteral
	ExpressionValues              map[ExprRef]product.Value
	ExpressionOperations          map[ExprRef]ExpressionOperation
	ExpressionFunctions           map[ExprRef]symbol.ID
	ExpressionRefinements         map[ExprRef]ExpressionRefinement
	ExpressionPaths               map[ExprRef]pathdom.Path
	DynamicIndexExpressions       map[ExprRef]DynamicIndexExpression
	ExpressionConditions          map[ExprRef]ExpressionCondition
}

// Facts is an immutable point-keyed transfer facts snapshot.
type Facts struct {
	rootAssignments               map[cfg.Point]RootAssignment
	pathAssignments               map[cfg.Point]PathAssignment
	pathStaticMemberWrites        map[cfg.Point]PathStaticMemberWrite
	dynamicIndexWrites            map[cfg.Point]DynamicIndexWrite
	pathDescendantInvalidations   map[cfg.Point]PathDescendantInvalidation
	covariantExposures            map[cfg.Point][]CovariantExposure
	noNormalReturns               map[cfg.Point]struct{}
	branchEdgeReachability        map[cfg.Point]BranchEdgeReachability
	branchConditionSources        map[cfg.Point]ValueSource
	branchRefinements             map[cfg.Point]BranchRefinementSet
	branchPresenceRelations       map[cfg.Point]BranchPresenceRelationSet
	branchPathRelations           map[cfg.Point]BranchPathRelationSet
	branchPathEvidence            map[cfg.Point]BranchPathEvidenceSet
	branchSufficientLiteralCases  map[cfg.Point]BranchSufficientLiteralCaseSet
	pathValuePresenceImplications map[cfg.Point]PathValuePresenceImplicationSet
	channelSelects                map[cfg.Point]ChannelSelectSet
	postconditionRefinements      map[cfg.Point]PostconditionRefinementSet
	postconditionPathRelations    map[cfg.Point]PostconditionPathRelationSet
	callResultValues              map[cfg.Point]CallResultValueSet
	returnPresenceRelations       map[cfg.Point]ReturnPresenceRelationSet
	returns                       map[cfg.Point]Return
	callSites                     map[cfg.Point]CallSite
	objectLiterals                map[ExprRef]ObjectLiteral
	expressionValues              map[ExprRef]product.Value
	expressionOperations          map[ExprRef]ExpressionOperation
	expressionFunctions           map[ExprRef]symbol.ID
	expressionRefinements         map[ExprRef]ExpressionRefinement
	expressionPaths               map[ExprRef]pathdom.Path
	dynamicIndexExpressions       map[ExprRef]DynamicIndexExpression
	expressionConditions          map[ExprRef]ExpressionCondition
}

// NewFacts copies the supplied point-keyed facts into an immutable snapshot.
func NewFacts(input FactsInput) Facts {
	return Facts{
		rootAssignments:               copyRootAssignmentMap(input.RootAssignments),
		pathAssignments:               copyPathAssignmentMap(input.PathAssignments),
		pathStaticMemberWrites:        copyPathStaticMemberWriteMap(input.PathStaticMemberWrites),
		dynamicIndexWrites:            copyDynamicIndexWriteMap(input.DynamicIndexWrites),
		pathDescendantInvalidations:   copyPathDescendantInvalidationMap(input.PathDescendantInvalidations),
		covariantExposures:            copyCovariantExposureMap(input.CovariantExposures),
		noNormalReturns:               copyNoNormalReturnMap(input.NoNormalReturns),
		branchEdgeReachability:        copyBranchEdgeReachabilityMap(input.BranchEdgeReachability),
		branchConditionSources:        copyBranchConditionSourceMap(input.BranchConditionSources),
		branchRefinements:             copyBranchRefinementSetMap(input.BranchRefinements),
		branchPresenceRelations:       copyBranchPresenceRelationMap(input.BranchPresenceRelations),
		branchPathRelations:           copyBranchPathRelationMap(input.BranchPathRelations),
		branchPathEvidence:            copyBranchPathEvidenceMap(input.BranchPathEvidence),
		branchSufficientLiteralCases:  copyBranchSufficientLiteralCaseMap(input.BranchSufficientLiteralCases),
		pathValuePresenceImplications: copyPathValuePresenceImplicationMap(input.PathValuePresenceImplications),
		channelSelects:                copyChannelSelectMap(input.ChannelSelects),
		postconditionRefinements:      copyPostconditionRefinementMap(input.PostconditionRefinements),
		postconditionPathRelations:    copyPostconditionPathRelationMap(input.PostconditionPathRelations),
		callResultValues:              copyCallResultValueMap(input.CallResultValues),
		returnPresenceRelations:       copyReturnPresenceRelationMap(input.ReturnPresenceRelations),
		returns:                       copyReturnMap(input.Returns),
		callSites:                     copyCallSiteMap(input.CallSites),
		objectLiterals:                copyObjectLiteralMap(input.ObjectLiterals),
		expressionValues:              copyExpressionValueMap(input.ExpressionValues),
		expressionOperations:          copyExpressionOperationMap(input.ExpressionOperations),
		expressionFunctions:           copyExpressionFunctionMap(input.ExpressionFunctions),
		expressionRefinements:         copyExpressionRefinementMap(input.ExpressionRefinements),
		expressionPaths:               copyExpressionPathMap(input.ExpressionPaths),
		dynamicIndexExpressions:       copyDynamicIndexExpressionMap(input.DynamicIndexExpressions),
		expressionConditions:          copyExpressionConditionMap(input.ExpressionConditions),
	}
}

// RootAssignment returns the root assignment fact at point.
func (f Facts) RootAssignment(point cfg.Point) (RootAssignment, bool) {
	fact, ok := f.rootAssignments[point]
	if !ok {
		return RootAssignment{}, false
	}
	return fact.copy(), true
}

// LocalAssignment returns the local assignment fact at point.
func (f Facts) LocalAssignment(point cfg.Point) (RootAssignment, bool) {
	fact, ok := f.RootAssignment(point)
	if !ok || fact.Kind() != RootAssignmentLocalDeclaration {
		return RootAssignment{}, false
	}
	return fact, true
}

// OrdinaryAssignment returns the ordinary assignment fact at point.
func (f Facts) OrdinaryAssignment(point cfg.Point) (RootAssignment, bool) {
	fact, ok := f.RootAssignment(point)
	if !ok || fact.Kind() != RootAssignmentOrdinaryRootWrite {
		return RootAssignment{}, false
	}
	return fact, true
}

// PathAssignment returns the member/path assignment fact at point.
func (f Facts) PathAssignment(point cfg.Point) (PathAssignment, bool) {
	fact, ok := f.pathAssignments[point]
	if !ok {
		return PathAssignment{}, false
	}
	return fact.copy(), true
}

// PathStaticMemberWrite returns the static-member proof write event at point.
func (f Facts) PathStaticMemberWrite(point cfg.Point) (PathStaticMemberWrite, bool) {
	fact, ok := f.pathStaticMemberWrites[point]
	if !ok {
		return PathStaticMemberWrite{}, false
	}
	return fact.copy(), true
}

// DynamicIndexWrite returns the dynamic-index write event at point.
func (f Facts) DynamicIndexWrite(point cfg.Point) (DynamicIndexWrite, bool) {
	fact, ok := f.dynamicIndexWrites[point]
	if !ok {
		return DynamicIndexWrite{}, false
	}
	return fact.copy(), true
}

// PathDescendantInvalidation returns the descendant-only path invalidation fact
// at point.
func (f Facts) PathDescendantInvalidation(point cfg.Point) (PathDescendantInvalidation, bool) {
	fact, ok := f.pathDescendantInvalidations[point]
	if !ok {
		return PathDescendantInvalidation{}, false
	}
	return fact.copy(), true
}

// CovariantExposures returns the covariant mutable-view exposures at point.
func (f Facts) CovariantExposures(point cfg.Point) []CovariantExposure {
	exposures := f.covariantExposures[point]
	if len(exposures) == 0 {
		return nil
	}
	out := make([]CovariantExposure, len(exposures))
	for i := range exposures {
		out[i] = exposures[i].copy()
	}
	return out
}

// NoNormalReturn reports whether point cannot complete normally.
func (f Facts) NoNormalReturn(point cfg.Point) bool {
	_, ok := f.noNormalReturns[point]
	return ok
}

// BranchEdgeUnreachable reports whether the selected branch edge is impossible.
func (f Facts) BranchEdgeUnreachable(point cfg.Point, cond bool) bool {
	if reachability, ok := f.branchEdgeReachability[point]; ok {
		return reachability.EdgeUnreachable(cond)
	}
	return false
}

// BranchConditionSource returns the lowered value source for the condition at a
// branch point.
func (f Facts) BranchConditionSource(point cfg.Point) (ValueSource, bool) {
	source, ok := f.branchConditionSources[point]
	return source, ok
}

// BranchRefinements returns all branch-edge value refinements at point.
func (f Facts) BranchRefinements(point cfg.Point) []BranchRefinement {
	if set, ok := f.branchRefinements[point]; ok {
		return set.Refinements()
	}
	return nil
}

// BranchLenRefinements returns the length-floor branch facts at point.
func (f Facts) BranchLenRefinements(point cfg.Point) []BranchLenRefinement {
	if set, ok := f.branchRefinements[point]; ok {
		return set.LenRefinements()
	}
	return nil
}

// BranchNumFloorRefinements returns the numeric-floor branch facts at point.
func (f Facts) BranchNumFloorRefinements(point cfg.Point) []BranchNumFloorRefinement {
	if set, ok := f.branchRefinements[point]; ok {
		return set.NumFloorRefinements()
	}
	return nil
}

// BranchDiffConstraints returns the difference-logic branch facts at point.
func (f Facts) BranchDiffConstraints(point cfg.Point) []BranchDiffConstraint {
	if set, ok := f.branchRefinements[point]; ok {
		return set.DiffConstraints()
	}
	return nil
}

// BranchPresenceRelations returns branch-triggered presence relations at point.
func (f Facts) BranchPresenceRelations(point cfg.Point) []BranchPresenceRelation {
	if set, ok := f.branchPresenceRelations[point]; ok {
		return set.Relations()
	}
	return nil
}

// BranchPathRelations returns branch-triggered path relations at point.
func (f Facts) BranchPathRelations(point cfg.Point) []BranchPathRelation {
	if set, ok := f.branchPathRelations[point]; ok {
		return set.Relations()
	}
	return nil
}

// BranchPathEvidence returns branch/postcondition path evidence at point.
func (f Facts) BranchPathEvidence(point cfg.Point) []BranchPathEvidence {
	if set, ok := f.branchPathEvidence[point]; ok {
		return set.Evidence()
	}
	return nil
}

// BranchSufficientLiteralCases returns branch-edge literal cases where proving
// the path has the literal value is sufficient to take that branch edge.
func (f Facts) BranchSufficientLiteralCases(point cfg.Point) []BranchSufficientLiteralCase {
	if set, ok := f.branchSufficientLiteralCases[point]; ok {
		return set.Cases()
	}
	return nil
}

// ForEachBranchPathEvidence visits branch/postcondition path evidence at point
// without copying the evidence slice. The visited values expose read-only ref
// accessors for hot internal paths.
func (f Facts) ForEachBranchPathEvidence(point cfg.Point, fn func(BranchPathEvidence) bool) {
	if fn == nil {
		return
	}
	if set, ok := f.branchPathEvidence[point]; ok {
		set.ForEachEvidence(fn)
	}
}

// PathValuePresenceImplications returns point-local implication publishes.
func (f Facts) PathValuePresenceImplications(point cfg.Point) []PathValuePresenceImplication {
	if set, ok := f.pathValuePresenceImplications[point]; ok {
		return set.Implications()
	}
	return nil
}

// ChannelSelects returns channel-select evidence events at point.
func (f Facts) ChannelSelects(point cfg.Point) []ChannelSelect {
	if set, ok := f.channelSelects[point]; ok {
		return set.Events()
	}
	return nil
}

// HasChannelSelects reports whether channel-select evidence exists at point
// without copying the event slice.
func (f Facts) HasChannelSelects(point cfg.Point) bool {
	_, ok := f.channelSelects[point]
	return ok
}

// PostconditionRefinements returns node-local refinements that hold after point
// completes normally.
func (f Facts) PostconditionRefinements(point cfg.Point) []PostconditionRefinement {
	if set, ok := f.postconditionRefinements[point]; ok {
		return set.Refinements()
	}
	return nil
}

// PostconditionPathRelations returns node-local path relations that hold after
// point completes normally.
func (f Facts) PostconditionPathRelations(point cfg.Point) []PostconditionPathRelation {
	if set, ok := f.postconditionPathRelations[point]; ok {
		return set.Relations()
	}
	return nil
}

// CallResultValues returns fixed product values for call return slots at point.
func (f Facts) CallResultValues(point cfg.Point) []CallResultValue {
	if set, ok := f.callResultValues[point]; ok {
		return set.Values()
	}
	return nil
}

// ForEachCallResultValue visits fixed product values for call return slots at
// point without allocating a slice copy.
func (f Facts) ForEachCallResultValue(point cfg.Point, fn func(CallResultValue) bool) {
	if set, ok := f.callResultValues[point]; ok {
		set.ForEachValue(fn)
	}
}

// ReturnPresenceRelations returns return-slot presence relations at point.
func (f Facts) ReturnPresenceRelations(point cfg.Point) []ReturnPresenceRelation {
	if set, ok := f.returnPresenceRelations[point]; ok {
		return set.Relations()
	}
	return nil
}

// Return returns the return fact at point.
func (f Facts) Return(point cfg.Point) (Return, bool) {
	fact, ok := f.returns[point]
	if !ok {
		return Return{}, false
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

// HasCallSites reports whether this immutable facts snapshot contains any
// call-site evidence.
func (f Facts) HasCallSites() bool {
	return len(f.callSites) != 0
}

// CallSiteCount returns the number of statically extracted call-site facts.
func (f Facts) CallSiteCount() int {
	return len(f.callSites)
}

// HasDynamicIndexWrites reports whether this immutable facts snapshot contains
// any dynamic table write evidence.
func (f Facts) HasDynamicIndexWrites() bool {
	return len(f.dynamicIndexWrites) != 0
}

// CallSiteView returns a read-only call-site view at point. The view never
// exposes mutable internal slices or path segment storage.
func (f Facts) CallSiteView(point cfg.Point) (CallSiteView, bool) {
	fact, ok := f.callSites[point]
	if !ok {
		return CallSiteView{}, false
	}
	return CallSiteView{site: fact}, true
}

// ObjectLiteral returns the static-entry sidecar for expr, if present.
func (f Facts) ObjectLiteral(expr ExprRef) (ObjectLiteral, bool) {
	fact, ok := f.objectLiterals[expr]
	if !ok {
		return ObjectLiteral{}, false
	}
	return fact.copy(), true
}

// ObjectLiteralView returns a read-only object literal view for expr. The view
// never exposes mutable internal entry slices or path segment storage.
func (f Facts) ObjectLiteralView(expr ExprRef) (ObjectLiteralView, bool) {
	fact, ok := f.objectLiterals[expr]
	if !ok {
		return ObjectLiteralView{}, false
	}
	return ObjectLiteralView{literal: fact}, true
}

// ObjectLiterals returns static-entry sidecars keyed by expression.
func (f Facts) ObjectLiterals() map[ExprRef]ObjectLiteral {
	return copyObjectLiteralMap(f.objectLiterals)
}

// ExpressionValue returns the syntactically known value fact for expr, if present.
func (f Facts) ExpressionValue(expr ExprRef) (product.Value, bool) {
	value, ok := f.expressionValues[expr]
	return value, ok
}

// ExpressionValues returns syntactically known expression values keyed by expression.
func (f Facts) ExpressionValues() map[ExprRef]product.Value {
	return copyExpressionValueMap(f.expressionValues)
}

// ExpressionOperation returns the lowered operation fact for expr, if present.
func (f Facts) ExpressionOperation(expr ExprRef) (ExpressionOperation, bool) {
	op, ok := f.expressionOperations[expr]
	if !ok {
		return ExpressionOperation{}, false
	}
	return op.copy(), true
}

// ExpressionOperations returns lowered expression operations keyed by expression.
func (f Facts) ExpressionOperations() map[ExprRef]ExpressionOperation {
	return copyExpressionOperationMap(f.expressionOperations)
}

// ExpressionFunction returns the function identity symbol for expr, if expr is
// a function literal with a bound function summary identity.
func (f Facts) ExpressionFunction(expr ExprRef) (symbol.ID, bool) {
	id, ok := f.expressionFunctions[expr]
	return id, ok && id != 0
}

// ExpressionRefinement returns the source-value refinement fact for expr, if present.
func (f Facts) ExpressionRefinement(expr ExprRef) (ExpressionRefinement, bool) {
	fact, ok := f.expressionRefinements[expr]
	if !ok {
		return ExpressionRefinement{}, false
	}
	return fact.copy(), true
}

// ExpressionRefinements returns source-value refinement facts keyed by expression.
func (f Facts) ExpressionRefinements() map[ExprRef]ExpressionRefinement {
	return copyExpressionRefinementMap(f.expressionRefinements)
}

// ExpressionPath returns the static expression access path for expr, if present.
func (f Facts) ExpressionPath(expr ExprRef) (pathdom.Path, bool) {
	p, ok := f.expressionPaths[expr]
	if !ok {
		return pathdom.Path{}, false
	}
	return p.Clone(), true
}

// ExpressionPathRef returns the static expression access path for immediate
// read-only use. Callers must not mutate or retain the returned path.
func (f Facts) ExpressionPathRef(expr ExprRef) (pathdom.Path, bool) {
	p, ok := f.expressionPaths[expr]
	if !ok {
		return pathdom.Path{}, false
	}
	return p, true
}

// ExpressionPaths returns the static expression access paths keyed by expression.
func (f Facts) ExpressionPaths() map[ExprRef]pathdom.Path {
	return copyExpressionPathMap(f.expressionPaths)
}

// DynamicIndexExpression returns the point-sensitive dynamic-index access path
// descriptor for expr, if present.
func (f Facts) DynamicIndexExpression(expr ExprRef) (DynamicIndexExpression, bool) {
	dyn, ok := f.dynamicIndexExpressions[expr]
	if !ok {
		return DynamicIndexExpression{}, false
	}
	return dyn.copy(), true
}

// DynamicIndexExpressions returns point-sensitive dynamic-index access path
// descriptors keyed by expression.
func (f Facts) DynamicIndexExpressions() map[ExprRef]DynamicIndexExpression {
	return copyDynamicIndexExpressionMap(f.dynamicIndexExpressions)
}

// ExpressionCondition returns the normalized path facts selected by expression
// truth value, if present.
func (f Facts) ExpressionCondition(expr ExprRef) (ExpressionCondition, bool) {
	condition, ok := f.expressionConditions[expr]
	if !ok {
		return ExpressionCondition{}, false
	}
	return condition.copy(), true
}

// ExpressionConditions returns expression-conditional path facts keyed by
// expression.
func (f Facts) ExpressionConditions() map[ExprRef]ExpressionCondition {
	return copyExpressionConditionMap(f.expressionConditions)
}

func copyNoNormalReturnMap(in map[cfg.Point]struct{}) map[cfg.Point]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]struct{}, len(in))
	for point := range in {
		out[point] = struct{}{}
	}
	return out
}

func copyExpressionValueMap(in map[ExprRef]product.Value) map[ExprRef]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]product.Value, len(in))
	for ref, value := range in {
		out[ref] = value
	}
	return out
}

func copyExpressionFunctionMap(in map[ExprRef]symbol.ID) map[ExprRef]symbol.ID {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]symbol.ID, len(in))
	for ref, id := range in {
		if ref == 0 || id == 0 {
			continue
		}
		out[ref] = id
	}
	return out
}
