package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

// copySubjectAliasFailure carries Flow's Alias/Unknown events and canonical
// route scopes into normalized cold lifecycle planes. Routes are authenticated
// once per scope; candidate rows carry only the resulting scope identity.
func (compiler *compiler) copySubjectAliasFailure() CompileFailure {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	projection := compiler.input.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	compiler.publication.Lifecycle.SubjectEvents = make([]lifecycle.SubjectEvent, 0, projection.EventCount())
	for index := 0; index < projection.EventCount(); index++ {
		flowRow, rowOK := projection.EventAt(index)
		if rowOK && flowRow.Kind != subjectflow.EventAlias && flowRow.Kind != subjectflow.EventUnknown {
			continue
		}
		if !rowOK || !flowRow.ID.Available() || !flowRow.Path.Available() || flowRow.Term == 0 {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		path, pathOK := compiler.input.Flow().SemanticTermPath(flowRow.Term)
		if !pathOK || path != flowRow.Path {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		kind, kindOK := artifactSubjectEventKind(flowRow.Kind)
		subjectKind, subjectOK := artifactSubjectLivenessKind(flowRow.Subject.Kind)
		if !kindOK || !subjectOK || !compiler.authenticatedSubject(flowRow.Subject) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		relatedKind := lifecycle.SubjectLivenessInvalid
		var related identity.ContentID
		if flowRow.Related.Kind != subjectflow.SubjectInvalid || flowRow.Related.ID.Available() || flowRow.Related.Term != 0 {
			var relatedOK bool
			relatedKind, relatedOK = artifactSubjectLivenessKind(flowRow.Related.Kind)
			if !relatedOK || !compiler.authenticatedSubject(flowRow.Related) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
			}
			related = flowRow.Related.ID
		}
		id, idOK := lifecycle.SubjectEventIdentity(flowRow.ID, flowRow.Path, kind, uint8(flowRow.Role), flowRow.Index, subjectKind, flowRow.Subject.ID, relatedKind, related)
		row, emitted := lifecycle.NewSubjectEvent(id, flowRow.ID, flowRow.Path, kind, uint8(flowRow.Role), flowRow.Index, subjectKind, flowRow.Subject.ID, relatedKind, related)
		if !idOK || !emitted {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		compiler.publication.Lifecycle.SubjectEvents = append(compiler.publication.Lifecycle.SubjectEvents, row)
	}

	compiler.publication.Lifecycle.AliasRouteScopes = make([]lifecycle.SubjectAliasRouteScope, 0, projection.AliasRouteScopeCount())
	compiler.publication.Lifecycle.AliasRouteMembers = nil
	scopeBySource := make(map[identity.ContentID]identity.ContentID, projection.AliasRouteScopeCount())
	var multiplicity map[identity.ContentID]int
	var multiplicityOK bool
	multiplicityComputed := false
	for index := 0; index < projection.AliasRouteScopeCount(); index++ {
		flowScope, scopeOK := projection.AliasRouteScopeAt(index)
		if !scopeOK || !flowScope.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		kind, kindOK := artifactSubjectAliasRouteScopeKind(flowScope.Kind)
		if !kindOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		routes := make([]identity.ContentID, flowScope.RouteCount())
		for routeIndex := range routes {
			route, routeOK := flowScope.RouteAt(routeIndex)
			if !routeOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, routeIndex, CompileReasonBodyUnavailable)
			}
			routes[routeIndex] = route
		}
		if len(routes) != 0 && !multiplicityComputed {
			multiplicity, multiplicityOK = compiler.computeCausalRouteMultiplicity()
			multiplicityComputed = true
		}
		if !authenticatedCausalRoutes(routes, multiplicity, multiplicityOK) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		id, idOK := lifecycle.SubjectAliasRouteScopeIdentity(flowScope.ID, kind, flowScope.Body, routes)
		if uint64(len(compiler.publication.Lifecycle.AliasRouteMembers)) > uint64(^uint32(0)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		offset := uint32(len(compiler.publication.Lifecycle.AliasRouteMembers))
		row, emitted := lifecycle.NewSubjectAliasRouteScope(id, flowScope.ID, kind, flowScope.Body, offset, routes)
		if !idOK || !emitted {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := scopeBySource[flowScope.ID]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		compiler.publication.Lifecycle.AliasRouteScopes = append(compiler.publication.Lifecycle.AliasRouteScopes, row)
		scopeBySource[flowScope.ID] = id
		for routeIndex, route := range routes {
			memberID, memberIDOK := lifecycle.SubjectAliasRouteScopeMemberIdentity(id, uint32(routeIndex), route)
			member, memberOK := lifecycle.NewSubjectAliasRouteScopeMember(memberID, id, uint32(routeIndex), route)
			if !memberIDOK || !memberOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, routeIndex, CompileReasonBodyUnavailable)
			}
			compiler.publication.Lifecycle.AliasRouteMembers = append(compiler.publication.Lifecycle.AliasRouteMembers, member)
		}
	}

	compiler.publication.Lifecycle.AliasCandidates = make([]lifecycle.SubjectAliasCandidate, 0, projection.AliasCandidateCount())
	for index := 0; index < projection.AliasCandidateCount(); index++ {
		flowRow, rowOK := projection.AliasCandidateAt(index)
		if !rowOK || !flowRow.Available() || !compiler.authenticatedSubject(flowRow.Candidate) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		candidateKind, kindOK := artifactSubjectLivenessKind(flowRow.Candidate.Kind)
		scope, scopeOK := scopeBySource[flowRow.Scope]
		if !kindOK || !scopeOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		id, idOK := lifecycle.SubjectAliasCandidateIdentity(flowRow.ID, candidateKind, flowRow.Candidate.ID, scope, flowRow.Closed)
		row, emitted := lifecycle.NewSubjectAliasCandidate(id, flowRow.ID, candidateKind, flowRow.Candidate.ID, scope, flowRow.Closed)
		if !idOK || !emitted {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		compiler.publication.Lifecycle.AliasCandidates = append(compiler.publication.Lifecycle.AliasCandidates, row)
	}
	return CompileFailure{}
}

func artifactSubjectAliasRouteScopeKind(kind subjectflow.AliasRouteScopeKind) (lifecycle.SubjectAliasRouteScopeKind, bool) {
	switch kind {
	case subjectflow.AliasRouteScopeBody:
		return lifecycle.SubjectAliasRouteScopeBody, true
	case subjectflow.AliasRouteScopeGlobal:
		return lifecycle.SubjectAliasRouteScopeGlobal, true
	default:
		return lifecycle.SubjectAliasRouteScopeInvalid, false
	}
}

func artifactSubjectEventKind(kind subjectflow.EventKind) (lifecycle.SubjectEventKind, bool) {
	switch kind {
	case subjectflow.EventUnknown:
		return lifecycle.SubjectEventUnknown, true
	case subjectflow.EventAlias:
		return lifecycle.SubjectEventAlias, true
	default:
		return lifecycle.SubjectEventInvalid, false
	}
}

func (compiler *compiler) authenticatedSubject(subject subjectflow.Subject) bool {
	if compiler == nil || compiler.input == nil || !subject.ID.Available() || subject.Term == 0 {
		return false
	}
	path, ok := compiler.input.Flow().SemanticTermPath(subject.Term)
	return ok && path == subject.ID
}

func authenticatedCausalRoutes(routes []identity.ContentID, multiplicity map[identity.ContentID]int, multiplicityOK bool) bool {
	if len(routes) == 0 {
		return true
	}
	if !multiplicityOK {
		return false
	}
	for _, routeID := range routes {
		if !routeID.Available() || multiplicity[routeID] != 1 {
			return false
		}
	}
	return true
}

// computeCausalRouteMultiplicity derives the sealed causal route denominator
// without retaining it in compiler state. Unissued and invalid projections
// are absent from the denominator; callers decide whether an empty route
// scope needs this projection at all.
func (compiler *compiler) computeCausalRouteMultiplicity() (map[identity.ContentID]int, bool) {
	if compiler == nil || compiler.input == nil {
		return nil, false
	}
	successors := compiler.input.Flow().Causal()
	if successors == nil {
		return nil, false
	}
	plane := successors.Successors()
	total := plane.TotalCount()
	multiplicity := make(map[identity.ContentID]int, total)
	for index := 0; index < total; index++ {
		candidate, candidateOK := plane.SemanticIDAt(index)
		if !candidateOK || !candidate.Available() {
			// A successor that issues no semantic identity publishes no route
			// ID, so it is absent from the denominator rather than a refusal
			// of every candidate route in the compile.
			continue
		}
		multiplicity[candidate]++
	}
	return multiplicity, true
}
