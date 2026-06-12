package factflow

import "github.com/wippyai/go-lua/analysis/ir/cfg"
import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// FactsInput carries point-keyed facts used to construct an immutable Facts snapshot.
type FactsInput struct {
	LocalAssignments            map[cfg.Point]RootAssignment
	OrdinaryAssignments         map[cfg.Point]RootAssignment
	PathAssignments             map[cfg.Point]PathAssignment
	PathDescendantInvalidations map[cfg.Point]PathDescendantInvalidation
	BranchRefinements           map[cfg.Point]BranchRefinement
	BranchRefinementSets        map[cfg.Point]BranchRefinementSet
	BranchPresenceRelations     map[cfg.Point]BranchPresenceRelationSet
	BranchPathRelations         map[cfg.Point]BranchPathRelationSet
	PostconditionRefinements    map[cfg.Point]PostconditionRefinementSet
	Returns                     map[cfg.Point]Return
	Calls                       map[cfg.Point]CallProducer
	CallSites                   map[cfg.Point]CallSite
	ObjectLiterals              map[ExprRef]ObjectLiteral
	ValueOverlays               map[ExprRef]ValueOverlay
	ExpressionPaths             map[ExprRef]pathdom.Path
}

// Facts is an immutable point-keyed transfer facts snapshot.
type Facts struct {
	localAssignments            map[cfg.Point]RootAssignment
	ordinaryAssignments         map[cfg.Point]RootAssignment
	pathAssignments             map[cfg.Point]PathAssignment
	pathDescendantInvalidations map[cfg.Point]PathDescendantInvalidation
	branchRefinements           map[cfg.Point]BranchRefinement
	branchRefinementSets        map[cfg.Point]BranchRefinementSet
	branchPresenceRelations     map[cfg.Point]BranchPresenceRelationSet
	branchPathRelations         map[cfg.Point]BranchPathRelationSet
	postconditionRefinements    map[cfg.Point]PostconditionRefinementSet
	returns                     map[cfg.Point]Return
	calls                       map[cfg.Point]CallProducer
	callSites                   map[cfg.Point]CallSite
	objectLiterals              map[ExprRef]ObjectLiteral
	valueOverlays               map[ExprRef]ValueOverlay
	expressionPaths             map[ExprRef]pathdom.Path
}

// NewFacts copies the supplied point-keyed facts into an immutable snapshot.
func NewFacts(input FactsInput) Facts {
	return Facts{
		localAssignments:            copyRootAssignmentMap(input.LocalAssignments),
		ordinaryAssignments:         copyRootAssignmentMap(input.OrdinaryAssignments),
		pathAssignments:             copyPathAssignmentMap(input.PathAssignments),
		pathDescendantInvalidations: copyPathDescendantInvalidationMap(input.PathDescendantInvalidations),
		branchRefinements:           copyBranchRefinementMap(input.BranchRefinements),
		branchRefinementSets:        copyBranchRefinementSetMap(input.BranchRefinementSets),
		branchPresenceRelations:     copyBranchPresenceRelationMap(input.BranchPresenceRelations),
		branchPathRelations:         copyBranchPathRelationMap(input.BranchPathRelations),
		postconditionRefinements:    copyPostconditionRefinementMap(input.PostconditionRefinements),
		returns:                     copyReturnMap(input.Returns),
		calls:                       copyCallProducerMap(input.Calls),
		callSites:                   copyCallSiteMap(input.CallSites),
		objectLiterals:              copyObjectLiteralMap(input.ObjectLiterals),
		valueOverlays:               copyValueOverlayMap(input.ValueOverlays),
		expressionPaths:             copyExpressionPathMap(input.ExpressionPaths),
	}
}

// WithBranchPresenceRelations returns f plus the supplied branch-triggered
// presence relations.
func (f Facts) WithBranchPresenceRelations(relations map[cfg.Point]BranchPresenceRelationSet) Facts {
	if len(relations) == 0 {
		return f
	}
	f.branchPresenceRelations = mergeBranchPresenceRelationMap(f.branchPresenceRelations, relations)
	return f
}

// WithPostconditionRefinements returns f plus the supplied node-local normal
// return refinements.
func (f Facts) WithPostconditionRefinements(refinements map[cfg.Point]PostconditionRefinementSet) Facts {
	if len(refinements) == 0 {
		return f
	}
	f.postconditionRefinements = mergePostconditionRefinementMap(f.postconditionRefinements, refinements)
	return f
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

// PathDescendantInvalidation returns the descendant-only path invalidation fact
// at point.
func (f Facts) PathDescendantInvalidation(point cfg.Point) (PathDescendantInvalidation, bool) {
	fact, ok := f.pathDescendantInvalidations[point]
	if !ok {
		return PathDescendantInvalidation{}, false
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

// BranchRefinements returns all branch-edge value refinements at point.
func (f Facts) BranchRefinements(point cfg.Point) []BranchRefinement {
	var out []BranchRefinement
	if fact, ok := f.branchRefinements[point]; ok {
		out = append(out, fact.copy())
	}
	if set, ok := f.branchRefinementSets[point]; ok {
		out = append(out, set.Refinements()...)
	}
	return out
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

// PostconditionRefinements returns node-local refinements that hold after point
// completes normally.
func (f Facts) PostconditionRefinements(point cfg.Point) []PostconditionRefinement {
	if set, ok := f.postconditionRefinements[point]; ok {
		return set.Refinements()
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

// ExpressionPath returns the static expression access path for expr, if present.
func (f Facts) ExpressionPath(expr ExprRef) (pathdom.Path, bool) {
	p, ok := f.expressionPaths[expr]
	if !ok {
		return pathdom.Path{}, false
	}
	return copyPath(p), true
}

// ExpressionPaths returns the static expression access paths keyed by expression.
func (f Facts) ExpressionPaths() map[ExprRef]pathdom.Path {
	return copyExpressionPathMap(f.expressionPaths)
}
