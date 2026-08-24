package subjectflow

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal derives the smallest generic subject-flow projection that the current
// Flow owners can prove.  It consumes only committed Source data, the
// lifecycle-bound authored view, executable membership, the sealed semantic
// path certificate, and final Causal routes.  It deliberately does not take
// Static or Module graphs: unresolved return/capture/call effects remain
// Unknown rows instead of being reconstructed here.
func Seal(
	sourceView source.View,
	authoredView authored.View,
	executableResult *executable.Result,
	causalResult *causal.Result,
	paths *semanticpath.Certificate,
	sourceID, flowID, staticID, moduleID identity.ContentID,
) (*Result, error) {
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() ||
		sourceView.Identity().ContentID() != sourceID || authoredView.ContentID() != flowID {
		return nil, ErrOwnerMismatch
	}
	if paths == nil || !paths.Matches(sourceID, flowID, staticID, moduleID) ||
		executableResult == nil || !executable.Matches(executableResult, sourceID, flowID, staticID, moduleID) ||
		causalResult == nil || !causal.Matches(causalResult, sourceID, flowID, staticID, moduleID) {
		return nil, ErrUnavailable
	}

	builder := &sealBuilder{
		source:     sourceView,
		authored:   authoredView,
		executable: executableResult,
		causal:     causalResult,
		paths:      paths,
		sourceID:   sourceID,
		flowID:     flowID,
		staticID:   staticID,
		moduleID:   moduleID,
	}
	if err := builder.roots(); err != nil {
		return nil, err
	}
	if err := builder.valueDefinitions(); err != nil {
		return nil, err
	}
	if err := builder.valuesAndStorage(); err != nil {
		return nil, err
	}
	if err := builder.controls(); err != nil {
		return nil, err
	}
	if err := builder.calls(); err != nil {
		return nil, err
	}
	if err := builder.captures(); err != nil {
		return nil, err
	}
	if err := builder.returns(); err != nil {
		return nil, err
	}
	// Every local authored source family, control row, opaque call, capture,
	// and return row has now been traversed successfully.  This is the
	// producer-side coverage witness used by aliasScopes; it is not inferred
	// from the presence or absence of an event row.
	builder.eventCoverageComplete = true
	if err := builder.aliasScopes(); err != nil {
		return nil, err
	}
	if err := builder.boundaries(); err != nil {
		return nil, err
	}
	if err := builder.liveness(); err != nil {
		return nil, err
	}

	return &Result{
		sourceID:    sourceID,
		flowID:      flowID,
		staticID:    staticID,
		moduleID:    moduleID,
		events:      append([]Event(nil), builder.events...),
		routeScopes: append([]AliasRouteScope(nil), builder.routeScopes...),
		candidates:  append([]AliasCandidate(nil), builder.aliasCandidates...),
		boundaries:  append([]Boundary(nil), builder.boundariesRows...),
		liveness:    append([]Liveness(nil), builder.livenessRows...),
		yieldOrder:  append([]YieldOrdinal(nil), builder.yieldOrder...),
	}, nil
}

type sealBuilder struct {
	source                source.View
	authored              authored.View
	executable            *executable.Result
	causal                *causal.Result
	paths                 *semanticpath.Certificate
	sourceID              identity.ContentID
	flowID                identity.ContentID
	staticID              identity.ContentID
	moduleID              identity.ContentID
	events                []Event
	eventCoverageComplete bool
	routeScopes           []AliasRouteScope
	aliasCandidates       []AliasCandidate
	boundariesRows        []Boundary
	livenessRows          []Liveness
	yieldOrder            []YieldOrdinal
}

type aliasCandidateState struct {
	subject Subject
	unknown bool
}

// foldAliasUnknown makes unresolved evidence absorbing over each undirected
// exact-alias component. The maps are seal-local indexes and are discarded
// before Result publication.
func foldAliasUnknown(candidates map[subjectKey]*aliasCandidateState, aliases map[subjectKey][]subjectKey) {
	visited := make(map[subjectKey]bool, len(candidates))
	for key := range candidates {
		if visited[key] {
			continue
		}
		queue := []subjectKey{key}
		component := make([]subjectKey, 0, 1)
		componentUnknown := false
		for head := 0; head < len(queue); head++ {
			current := queue[head]
			if visited[current] {
				continue
			}
			visited[current] = true
			component = append(component, current)
			if candidate := candidates[current]; candidate != nil {
				componentUnknown = componentUnknown || candidate.unknown
			}
			for _, next := range aliases[current] {
				if !visited[next] {
					queue = append(queue, next)
				}
			}
		}
		if componentUnknown {
			for _, member := range component {
				if memberState := candidates[member]; memberState != nil {
					memberState.unknown = true
				}
			}
		}
	}
}

// aliasScopes publishes each body/global route denominator once, then
// binds every Alias/Unknown candidate to that canonical scope. No empty scope
// is treated as closure by itself. A candidate with unresolved Unknown, or
// one whose source position cannot establish a body, remains explicitly open.
func (builder *sealBuilder) aliasScopes() error {
	graph, err := newSubjectGraph(builder.causal)
	if err != nil {
		return err
	}
	candidates := make(map[subjectKey]*aliasCandidateState)
	aliases := make(map[subjectKey][]subjectKey)
	addCandidate := func(subject Subject, unknown bool) {
		if !subject.Kind.valid() || !subject.ID.Available() {
			return
		}
		key := makeSubjectKey(subject)
		state := candidates[key]
		if state == nil {
			state = &aliasCandidateState{subject: subject}
			candidates[key] = state
		}
		state.unknown = state.unknown || unknown
	}
	for _, event := range builder.events {
		switch event.Kind {
		case EventAlias:
			addCandidate(event.Subject, false)
			addCandidate(event.Related, false)
			leftOK := event.Subject.Kind.valid() && event.Subject.ID.Available()
			rightOK := event.Related.Kind.valid() && event.Related.ID.Available()
			if leftOK && rightOK {
				leftKey, rightKey := makeSubjectKey(event.Subject), makeSubjectKey(event.Related)
				aliases[leftKey] = append(aliases[leftKey], rightKey)
				aliases[rightKey] = append(aliases[rightKey], leftKey)
			}
		case EventUnknown:
			addCandidate(event.Subject, true)
			addCandidate(event.Related, true)
		}
	}
	// Unknown is component-absorbing. An exact alias edge does not isolate
	// either endpoint from an unresolved effect attached to another endpoint.
	foldAliasUnknown(candidates, aliases)

	routes := make([]graphRoute, 0, len(graph.routes))
	for _, route := range graph.routes {
		routes = append(routes, route)
	}
	sort.Slice(routes, func(left, right int) bool {
		return bytes.Compare(routes[left].id[:], routes[right].id[:]) < 0
	})

	neededBodies := make(map[keyspace.Term]identity.ContentID)
	bodyByID := make(map[identity.ContentID]keyspace.Term)
	needsGlobal := false
	for _, state := range candidates {
		body, known := builder.bodyForTerm(state.subject.Term)
		if !known {
			needsGlobal = true
			continue
		}
		bodyID, bodyIDOK := builder.path(body)
		if !bodyIDOK || !bodyID.Available() {
			return fmt.Errorf("%w: alias route body identity is unavailable", ErrMalformed)
		}
		if held, duplicate := bodyByID[bodyID]; duplicate && held != body {
			return fmt.Errorf("%w: duplicate alias route body identity", ErrMalformed)
		}
		neededBodies[body] = bodyID
		bodyByID[bodyID] = body
	}

	bodyRoutes := make(map[keyspace.Term][]identity.ContentID, len(neededBodies))
	globalRoutes := make([]identity.ContentID, 0, len(routes))
	for _, route := range routes {
		globalRoutes = append(globalRoutes, route.id)
		fromBody, fromKnown := builder.bodyForTerm(route.from)
		toBody, toKnown := builder.bodyForTerm(route.to)
		if fromKnown {
			if _, needed := neededBodies[fromBody]; needed {
				bodyRoutes[fromBody] = append(bodyRoutes[fromBody], route.id)
			}
		}
		if toKnown && (!fromKnown || toBody != fromBody) {
			if _, needed := neededBodies[toBody]; needed {
				bodyRoutes[toBody] = append(bodyRoutes[toBody], route.id)
			}
		}
	}

	bodyTerms := make([]keyspace.Term, 0, len(neededBodies))
	for body := range neededBodies {
		bodyTerms = append(bodyTerms, body)
	}
	sort.Slice(bodyTerms, func(left, right int) bool {
		leftID, rightID := neededBodies[bodyTerms[left]], neededBodies[bodyTerms[right]]
		return bytes.Compare(leftID[:], rightID[:]) < 0
	})
	scopeForBody := make(map[keyspace.Term]identity.ContentID, len(bodyTerms))
	for _, body := range bodyTerms {
		scope := newAliasRouteScope(AliasRouteScopeBody, neededBodies[body], bodyRoutes[body])
		if !scope.Available() {
			return fmt.Errorf("%w: body alias route scope is unavailable", ErrMalformed)
		}
		builder.routeScopes = append(builder.routeScopes, scope)
		scopeForBody[body] = scope.ID
	}
	globalScope := identity.ContentID{}
	if needsGlobal {
		scope := newAliasRouteScope(AliasRouteScopeGlobal, identity.ContentID{}, globalRoutes)
		if !scope.Available() {
			return fmt.Errorf("%w: global alias route scope is unavailable", ErrMalformed)
		}
		builder.routeScopes = append(builder.routeScopes, scope)
		globalScope = scope.ID
	}

	for _, state := range candidates {
		candidateBody, bodyKnown := builder.bodyForTerm(state.subject.Term)
		scope := globalScope
		if bodyKnown {
			scope = scopeForBody[candidateBody]
		}
		candidate := newAliasCandidate(state.subject, scope, builder.eventCoverageComplete && bodyKnown && !state.unknown)
		if !candidate.Available() {
			return fmt.Errorf("%w: alias candidate identity is unavailable", ErrMalformed)
		}
		builder.aliasCandidates = append(builder.aliasCandidates, candidate)
	}
	sort.Slice(builder.aliasCandidates, func(left, right int) bool {
		candidateLeft, candidateRight := builder.aliasCandidates[left].Candidate, builder.aliasCandidates[right].Candidate
		if candidateLeft.Kind != candidateRight.Kind {
			return candidateLeft.Kind < candidateRight.Kind
		}
		if compared := bytes.Compare(candidateLeft.ID[:], candidateRight.ID[:]); compared != 0 {
			return compared < 0
		}
		return candidateLeft.Term < candidateRight.Term
	})
	return nil
}

func (builder *sealBuilder) bodyForTerm(term keyspace.Term) (keyspace.Term, bool) {
	if term == 0 {
		return 0, false
	}
	if keyspace.TermFamily(term) == keyspace.FamilyBody {
		return term, true
	}
	body, _, _, ok := builder.source.Index().Position(term)
	if !ok || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	return body, true
}

func (builder *sealBuilder) path(term keyspace.Term) (identity.ContentID, bool) {
	if term == 0 {
		return identity.ContentID{}, false
	}
	return builder.paths.TermPathAt(builder.sourceID, builder.flowID, builder.staticID, builder.moduleID, keyspace.TermFamily(term), keyspace.TermOrdinal(term))
}

func (builder *sealBuilder) subject(kind SubjectKind, term keyspace.Term) (Subject, bool) {
	if !kind.valid() || term == 0 {
		return Subject{}, false
	}
	id, ok := builder.path(term)
	if !ok || !id.Available() {
		return Subject{}, false
	}
	return Subject{Kind: kind, ID: id, Term: term}, true
}

func (builder *sealBuilder) callPath(call keyspace.Term) (identity.ContentID, bool) {
	if keyspace.TermFamily(call) != keyspace.FamilyCall || keyspace.TermOrdinal(call) == 0 {
		return identity.ContentID{}, false
	}
	owner, _, _, _, ok := builder.authored.Calls().Get(call)
	if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
		return identity.ContentID{}, false
	}
	bodyPath, bodyOK := builder.path(owner)
	termPath, termOK := builder.path(call)
	if !bodyOK || !termOK || !bodyPath.Available() || !termPath.Available() {
		return identity.ContentID{}, false
	}
	id := callOccurrenceID(bodyPath, termPath)
	return id, id.Available()
}

// callOccurrenceID is the same neutral occurrence equation used by Flow's
// CallPath projection. Re-deriving it from the owner-fenced certificate keeps
// a raw caller-supplied path slice from becoming a splice capability.
func callOccurrenceID(bodyPath, termPath identity.ContentID) identity.ContentID {
	var payload [96]byte
	copy(payload[:], "call-occurrence")
	copy(payload[32:64], bodyPath[:])
	copy(payload[64:], termPath[:])
	return sha256.Sum256(payload[:])
}

func (builder *sealBuilder) runtime(term keyspace.Term) bool {
	if term == 0 {
		return false
	}
	if keyspace.TermFamily(term) == keyspace.FamilyCell {
		// Global Cells intentionally do not belong to Executable's runtime bit
		// set, but their canonical semantic paths still make an exact local
		// storage relation available to a Read/Bind/Assign row.
		return true
	}
	return builder.executable.Contains(term)
}

func (builder *sealBuilder) add(kind EventKind, role EventRole, index uint32, subjectKind SubjectKind, subjectTerm keyspace.Term, relatedKind SubjectKind, relatedTerm keyspace.Term, operation keyspace.Term) error {
	if !kind.valid() || role == 0 {
		return ErrMalformed
	}
	subject, ok := builder.subject(subjectKind, subjectTerm)
	if !ok {
		return fmt.Errorf("%w: subject %v has no semantic path", ErrMalformed, subjectTerm)
	}
	var related Subject
	if relatedTerm != 0 {
		related, ok = builder.subject(relatedKind, relatedTerm)
		if !ok {
			return fmt.Errorf("%w: related subject %v has no semantic path", ErrMalformed, relatedTerm)
		}
	}
	path, ok := builder.path(operation)
	if !ok || !path.Available() {
		return fmt.Errorf("%w: operation %v has no semantic path", ErrMalformed, operation)
	}
	id := rowID(kind, role, index, subject, related, operation, path)
	if !id.Available() {
		return fmt.Errorf("%w: event identity is unavailable", ErrMalformed)
	}
	builder.events = append(builder.events, Event{ID: id, Kind: kind, Role: role, Index: index, Subject: subject, Related: related, Term: operation, Path: path})
	return nil
}

func (builder *sealBuilder) addUse(term, operation keyspace.Term, role EventRole, index uint32) error {
	if term == 0 || !builder.runtime(term) {
		return nil
	}
	return builder.add(EventUse, role, index, subjectKindForTerm(term), term, SubjectInvalid, 0, operation)
}

// subjectKindForTerm preserves the authored subject plane for each structural
// term family. A Cell is not an ordinary Value merely because it is carried by
// an authored row, and a Values aggregate is not a scalar Value merely because
// it occupies one fixed position in another Values row.
func subjectKindForTerm(term keyspace.Term) SubjectKind {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyCell:
		return SubjectCell
	case keyspace.FamilyValues:
		return SubjectValues
	default:
		return SubjectValue
	}
}

func (builder *sealBuilder) addUnknownTerm(term, operation keyspace.Term, role EventRole, index uint32) error {
	if term == 0 || !builder.runtime(term) {
		return nil
	}
	return builder.addUnknown(subjectKindForTerm(term), term, 0, operation, role, index)
}

func (builder *sealBuilder) addValuesUse(values, operation keyspace.Term, role EventRole) error {
	if values == 0 {
		return nil
	}
	if !builder.runtime(values) {
		return nil
	}
	if err := builder.add(EventUse, role, 0, SubjectValues, values, SubjectInvalid, 0, operation); err != nil {
		return err
	}
	count, ok := builder.authored.Values().Len(values)
	if !ok {
		return fmt.Errorf("%w: Values %v is unavailable", ErrMalformed, values)
	}
	for index := 0; index < count; index++ {
		member, memberOK := builder.authored.Values().Member(values, index)
		if !memberOK {
			return fmt.Errorf("%w: Values %v member %d is unavailable", ErrMalformed, values, index)
		}
		if err := builder.addUse(member, operation, RoleMember, uint32(index)); err != nil {
			return err
		}
	}
	_, tail, ok := builder.authored.Values().Get(values)
	if !ok {
		return fmt.Errorf("%w: Values %v row is unavailable", ErrMalformed, values)
	}
	if tail != 0 {
		// A dynamic Call/Vararg tail is a precise point where the current
		// local proof stops. Keep the Values subject visible as Unknown; do
		// not turn the tail producer into a guessed alias.
		return builder.addUnknownRelated(subjectKindForTerm(tail), tail, SubjectValues, values, operation, RoleTail, 0)
	}
	return nil
}

// addValuesUnknown is used at opaque call boundaries.  Without a sealed
// effect/capture proof, neither the aggregate actuals nor any fixed member
// can be treated as an exact post-call relation.  Keeping the exact Use rows
// as well preserves the local operand fact while the Unknown rows absorb any
// liveness query that would otherwise overclaim a call effect.
func (builder *sealBuilder) addValuesUnknown(values, operation keyspace.Term, role EventRole, index uint32) error {
	if values == 0 || !builder.runtime(values) {
		return nil
	}
	if err := builder.addUnknown(SubjectValues, values, 0, operation, role, index); err != nil {
		return err
	}
	count, ok := builder.authored.Values().Len(values)
	if !ok {
		return fmt.Errorf("%w: Values %v is unavailable", ErrMalformed, values)
	}
	for memberIndex := 0; memberIndex < count; memberIndex++ {
		member, memberOK := builder.authored.Values().Member(values, memberIndex)
		if !memberOK {
			return fmt.Errorf("%w: Values %v member %d is unavailable", ErrMalformed, values, memberIndex)
		}
		if err := builder.addUnknownTerm(member, operation, RoleMember, uint32(memberIndex)); err != nil {
			return err
		}
	}
	return nil
}

func (builder *sealBuilder) addUnknown(subjectKind SubjectKind, subjectTerm, relatedTerm, operation keyspace.Term, role EventRole, index uint32) error {
	return builder.addUnknownRelated(subjectKind, subjectTerm, subjectKind, relatedTerm, operation, role, index)
}

func (builder *sealBuilder) addUnknownRelated(subjectKind SubjectKind, subjectTerm keyspace.Term, relatedKind SubjectKind, relatedTerm, operation keyspace.Term, role EventRole, index uint32) error {
	return builder.add(EventUnknown, role, index, subjectKind, subjectTerm, relatedKind, relatedTerm, operation)
}

func (builder *sealBuilder) roots() error {
	count := builder.source.Identity().FamilyCount(keyspace.FamilyBody)
	for ordinal := 1; ordinal <= count; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		if !builder.executable.Contains(body) {
			continue
		}
		if err := builder.add(EventDefine, RoleRoot, uint32(ordinal), SubjectRoot, body, SubjectInvalid, 0, body); err != nil {
			return err
		}
	}
	return nil
}

// valueDefinitions emits only local runtime definitions. Calls, Reads, and
// Varargs are handled separately because their result identity crosses a
// storage or interprocedural boundary and therefore cannot be asserted as a
// plain Define.
func (builder *sealBuilder) valueDefinitions() error {
	families := [...]keyspace.Family{
		keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyUnary,
		keyspace.FamilyBinary,
		keyspace.FamilySelect, keyspace.FamilyFunction, keyspace.FamilyTable,
		keyspace.FamilyValueClaim,
	}
	for _, family := range families {
		count := builder.source.Identity().FamilyCount(family)
		for ordinal := 1; ordinal <= count; ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			if !builder.runtime(term) {
				continue
			}
			if err := builder.add(EventDefine, RoleResult, uint32(ordinal), SubjectValue, term, SubjectInvalid, 0, term); err != nil {
				return err
			}
		}
	}
	claims := builder.authored.Claims()
	for index := 0; index < claims.Count(); index++ {
		term, ok := claims.At(index)
		if !ok || !builder.runtime(term) {
			continue
		}
		_, operand, _, rowOK := claims.Get(term)
		if !rowOK {
			return fmt.Errorf("%w: ValueClaim %v is unavailable", ErrMalformed, term)
		}
		if err := builder.addUse(operand, term, RoleOperand, 0); err != nil {
			return err
		}
	}

	operators := builder.authored.Operators()
	for index := 0; index < operators.Unaries().Count(); index++ {
		term, ok := operators.Unaries().At(index)
		if !ok || !builder.runtime(term) {
			continue
		}
		_, _, operand, rowOK := operators.Unaries().Get(term)
		if !rowOK {
			return fmt.Errorf("%w: Unary %v is unavailable", ErrMalformed, term)
		}
		if err := builder.addUse(operand, term, RoleOperand, 0); err != nil {
			return err
		}
	}
	for index := 0; index < operators.Binaries().Count(); index++ {
		term, ok := operators.Binaries().At(index)
		if !ok || !builder.runtime(term) {
			continue
		}
		_, _, left, right, rowOK := operators.Binaries().Get(term)
		if !rowOK {
			return fmt.Errorf("%w: Binary %v is unavailable", ErrMalformed, term)
		}
		if err := builder.addUse(left, term, RoleLeft, 0); err != nil {
			return err
		}
		if err := builder.addUse(right, term, RoleRight, 0); err != nil {
			return err
		}
	}
	for index := 0; index < operators.Selects().Count(); index++ {
		term, ok := operators.Selects().At(index)
		if !ok || !builder.runtime(term) {
			continue
		}
		_, _, left, right, rowOK := operators.Selects().Get(term)
		if !rowOK {
			return fmt.Errorf("%w: Select %v is unavailable", ErrMalformed, term)
		}
		if err := builder.addUse(left, term, RoleLeft, 0); err != nil {
			return err
		}
		if err := builder.addUse(right, term, RoleRight, 0); err != nil {
			return err
		}
	}
	for index := 0; index < builder.authored.Access().Exact().Count(); index++ {
		term, ok := builder.authored.Access().Exact().At(index)
		if !ok || !builder.runtime(term) {
			continue
		}
		_, base, sourceTerm, _, rowOK := builder.authored.Access().Exact().Get(term)
		if !rowOK {
			return fmt.Errorf("%w: exact Lens %v is unavailable", ErrMalformed, term)
		}
		if err := builder.addUse(base, term, RoleOperand, 0); err != nil {
			return err
		}
		if err := builder.addUse(sourceTerm, term, RoleRight, 0); err != nil {
			return err
		}
	}
	for index := 0; index < builder.authored.Access().Dynamic().Count(); index++ {
		term, ok := builder.authored.Access().Dynamic().At(index)
		if !ok || !builder.runtime(term) {
			continue
		}
		_, base, key, rowOK := builder.authored.Access().Dynamic().Get(term)
		if !rowOK {
			return fmt.Errorf("%w: dynamic Lens %v is unavailable", ErrMalformed, term)
		}
		if err := builder.addUse(base, term, RoleOperand, 0); err != nil {
			return err
		}
		if err := builder.addUse(key, term, RoleRight, 0); err != nil {
			return err
		}
	}
	return builder.tables()
}

func (builder *sealBuilder) tables() error {
	tables := builder.authored.Tables()
	for index := 0; index < tables.Count(); index++ {
		table, ok := tables.At(index)
		if !ok || !builder.runtime(table) {
			continue
		}
		fieldCount, ok := tables.FieldCount(table)
		if !ok {
			return fmt.Errorf("%w: Table %v fields are unavailable", ErrMalformed, table)
		}
		for fieldIndex := 0; fieldIndex < fieldCount; fieldIndex++ {
			field, fieldOK := tables.FieldAt(table, fieldIndex)
			if !fieldOK {
				return fmt.Errorf("%w: Table %v field %d is unavailable", ErrMalformed, table, fieldIndex)
			}
			values, _, valuesOK := builder.authored.Fields().Values(field)
			if !valuesOK {
				return fmt.Errorf("%w: TableField %v Values is unavailable", ErrMalformed, field)
			}
			if err := builder.addValuesUse(values, field, RoleMember); err != nil {
				return err
			}
		}
	}
	return nil
}

func (builder *sealBuilder) valuesAndStorage() error {
	values := builder.authored.Values()
	for index := 0; index < values.Count(); index++ {
		term, ok := values.At(index)
		if !ok || !builder.runtime(term) {
			continue
		}
		// The Values row is an exact aggregate definition. Its fixed members
		// are uses of the member subjects, while an open tail remains Unknown.
		if err := builder.add(EventDefine, RoleResult, uint32(index), SubjectValues, term, SubjectInvalid, 0, term); err != nil {
			return err
		}
		if err := builder.addValuesUse(term, term, RoleMember); err != nil {
			return err
		}
	}

	if err := builder.readsAndVarargs(); err != nil {
		return err
	}
	if err := builder.binds(); err != nil {
		return err
	}
	return builder.assigns()
}

func (builder *sealBuilder) readsAndVarargs() error {
	storage := builder.authored.Storage()
	for index := 0; index < storage.Reads().Count(); index++ {
		read, ok := storage.Reads().At(index)
		if !ok || !builder.runtime(read) {
			continue
		}
		_, sourceTerm, implicit, rowOK := storage.Reads().Get(read)
		if !rowOK {
			return fmt.Errorf("%w: Read %v is unavailable", ErrMalformed, read)
		}
		if sourceTerm == 0 {
			return fmt.Errorf("%w: Read %v has no source", ErrMalformed, read)
		}
		sourceFamily := keyspace.TermFamily(sourceTerm)
		if sourceFamily != keyspace.FamilyLensExact && sourceFamily != keyspace.FamilyLensKey {
			if err := builder.addUse(sourceTerm, read, RoleOperand, 0); err != nil {
				return err
			}
		}
		if implicit {
			// The cell/key spelling is structural, but implicit globals do not
			// prove a stable local storage value. Preserve the uncertainty and
			// do not publish Cell -> Read as an Alias.
			if err := builder.addUnknown(SubjectValue, read, 0, read, RoleResult, uint32(index)); err != nil {
				return err
			}
			continue
		}
		if err := builder.add(EventDefine, RoleResult, uint32(index), SubjectValue, read, SubjectInvalid, 0, read); err != nil {
			return err
		}
		if keyspace.TermFamily(sourceTerm) == keyspace.FamilyCell {
			if err := builder.add(EventAlias, RoleOperand, uint32(index), SubjectCell, sourceTerm, SubjectValue, read, read); err != nil {
				return err
			}
		}
	}
	for index := 0; index < storage.Varargs().Count(); index++ {
		vararg, ok := storage.Varargs().At(index)
		if !ok || !builder.runtime(vararg) {
			continue
		}
		_, cell, rowOK := storage.Varargs().Get(vararg)
		if !rowOK || cell == 0 {
			return fmt.Errorf("%w: Vararg %v is unavailable", ErrMalformed, vararg)
		}
		if err := builder.addUse(cell, vararg, RoleOperand, 0); err != nil {
			return err
		}
		if err := builder.addUnknown(SubjectValue, vararg, 0, vararg, RoleResult, uint32(index)); err != nil {
			return err
		}
	}
	return nil
}

func (builder *sealBuilder) binds() error {
	storage := builder.authored.Storage()
	for index := 0; index < storage.Binds().Count(); index++ {
		bind, ok := storage.Binds().At(index)
		if !ok || !builder.runtime(bind) {
			continue
		}
		_, values, rowOK := storage.Binds().Get(bind)
		if !rowOK || values == 0 {
			return fmt.Errorf("%w: Bind %v is unavailable", ErrMalformed, bind)
		}
		if err := builder.addValuesUse(values, bind, RoleActuals); err != nil {
			return err
		}
		cellCount, cellOK := builder.source.Binds().Len(bind)
		if !cellOK {
			return fmt.Errorf("%w: Source Bind %v order is unavailable", ErrMalformed, bind)
		}
		for cellIndex := 0; cellIndex < cellCount; cellIndex++ {
			cell, cellOK := builder.source.Binds().At(bind, cellIndex)
			if !cellOK || keyspace.TermFamily(cell) != keyspace.FamilyCell {
				return fmt.Errorf("%w: Bind %v cell %d is unavailable", ErrMalformed, bind, cellIndex)
			}
			if err := builder.add(EventDefine, RoleCell, uint32(cellIndex), SubjectCell, cell, SubjectInvalid, 0, bind); err != nil {
				return err
			}
			position, positionOK := builder.authored.Values().Position(values, cellIndex)
			if !positionOK {
				return fmt.Errorf("%w: Bind %v Values position %d is unavailable", ErrMalformed, bind, cellIndex)
			}
			switch {
			case position.Fixed != 0:
				if err := builder.add(EventAlias, RoleCell, uint32(cellIndex), subjectKindForTerm(position.Fixed), position.Fixed, SubjectCell, cell, bind); err != nil {
					return err
				}
			case position.Tail != 0:
				if err := builder.addUnknownRelated(SubjectCell, cell, SubjectValues, values, bind, RoleTail, uint32(cellIndex)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (builder *sealBuilder) assigns() error {
	storage := builder.authored.Storage()
	for index := 0; index < storage.Assigns().Count(); index++ {
		assign, ok := storage.Assigns().At(index)
		if !ok || !builder.runtime(assign) {
			continue
		}
		_, values, rowOK := storage.Assigns().Get(assign)
		if !rowOK || values == 0 {
			return fmt.Errorf("%w: Assign %v is unavailable", ErrMalformed, assign)
		}
		if err := builder.addValuesUse(values, assign, RoleActuals); err != nil {
			return err
		}
		writeCount, writeOK := storage.Assigns().WriteCount(assign)
		if !writeOK {
			return fmt.Errorf("%w: Assign %v write order is unavailable", ErrMalformed, assign)
		}
		for writeIndex := 0; writeIndex < writeCount; writeIndex++ {
			write, writeOK := storage.Assigns().WriteAt(assign, writeIndex)
			if !writeOK {
				return fmt.Errorf("%w: Assign %v write %d is unavailable", ErrMalformed, assign, writeIndex)
			}
			_, target, targetOK := storage.Writes().Get(write)
			if !targetOK || target == 0 {
				return fmt.Errorf("%w: Write %v target is unavailable", ErrMalformed, write)
			}
			position, positionOK := builder.authored.Values().Position(values, writeIndex)
			if !positionOK {
				return fmt.Errorf("%w: Assign %v Values position %d is unavailable", ErrMalformed, assign, writeIndex)
			}
			if keyspace.TermFamily(target) == keyspace.FamilyCell {
				if err := builder.add(EventDefine, RoleTarget, uint32(writeIndex), SubjectCell, target, SubjectInvalid, 0, write); err != nil {
					return err
				}
			}
			switch {
			case keyspace.TermFamily(target) == keyspace.FamilyLensExact || keyspace.TermFamily(target) == keyspace.FamilyLensKey:
				// A Lens is structural selector geometry. Heap/Index owns the
				// store relation; SubjectFlow must not publish the selector itself
				// as a mutable Value or alias candidate.
			case position.Fixed != 0 && keyspace.TermFamily(target) == keyspace.FamilyCell:
				if err := builder.add(EventAlias, RoleWrite, uint32(writeIndex), subjectKindForTerm(position.Fixed), position.Fixed, SubjectCell, target, write); err != nil {
					return err
				}
			case position.Fixed != 0:
				if err := builder.addUnknownRelated(subjectKindForTerm(target), target, subjectKindForTerm(position.Fixed), position.Fixed, write, RoleTarget, uint32(writeIndex)); err != nil {
					return err
				}
			case position.Tail != 0:
				if err := builder.addUnknownRelated(subjectKindForTerm(target), target, SubjectValues, values, write, RoleTail, uint32(writeIndex)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (builder *sealBuilder) controls() error {
	control := builder.authored.Control()
	branches := control.Branches()
	for index := 0; index < branches.Count(); index++ {
		branch, ok := branches.At(index)
		if !ok || !builder.runtime(branch) {
			continue
		}
		_, condition, _, _, rowOK := branches.Get(branch)
		if !rowOK {
			return fmt.Errorf("%w: Branch %v is unavailable", ErrMalformed, branch)
		}
		if err := builder.addUse(condition, branch, RoleOperand, 0); err != nil {
			return err
		}
	}
	loops := control.Loops()
	for index := 0; index < loops.Count(); index++ {
		loop, ok := loops.At(index)
		if !ok || !builder.runtime(loop) {
			continue
		}
		_, _, _, controlTerm, rowOK := loops.Get(loop)
		if !rowOK {
			return fmt.Errorf("%w: Loop %v is unavailable", ErrMalformed, loop)
		}
		if err := builder.addUse(controlTerm, loop, RoleOperand, 0); err != nil {
			return err
		}
		cellCount, countOK := loops.CellCount(loop)
		if !countOK {
			return fmt.Errorf("%w: Loop %v cells are unavailable", ErrMalformed, loop)
		}
		for cellIndex := 0; cellIndex < cellCount; cellIndex++ {
			cell, cellOK := loops.CellAt(loop, cellIndex)
			if !cellOK {
				return fmt.Errorf("%w: Loop %v cell %d is unavailable", ErrMalformed, loop, cellIndex)
			}
			if err := builder.add(EventDefine, RoleCell, uint32(cellIndex), SubjectCell, cell, SubjectInvalid, 0, loop); err != nil {
				return err
			}
			if err := builder.addUnknown(SubjectCell, cell, 0, loop, RoleTarget, uint32(cellIndex)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (builder *sealBuilder) calls() error {
	calls := builder.authored.Calls()
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok || !builder.runtime(call) {
			continue
		}
		_, callee, receiver, actuals, rowOK := calls.Get(call)
		if !rowOK {
			return fmt.Errorf("%w: Call %v is unavailable", ErrMalformed, call)
		}
		if err := builder.addUse(callee, call, RoleCallee, 0); err != nil {
			return err
		}
		if err := builder.addUse(receiver, call, RoleReceiver, 0); err != nil {
			return err
		}
		if err := builder.addValuesUse(actuals, call, RoleActuals); err != nil {
			return err
		}
		// Calls are opaque at this neutral layer: no Static/Module effect
		// proof is available to establish receiver or actual ownership across
		// the dynamic boundary.  Preserve those operands as explicit Unknown
		// facts so liveness cannot infer an exact call effect from their local
		// Use rows.
		if err := builder.addUnknownTerm(receiver, call, RoleReceiver, uint32(index)); err != nil {
			return err
		}
		if err := builder.addValuesUnknown(actuals, call, RoleActuals, uint32(index)); err != nil {
			return err
		}
		// The call's local operand uses are retained for provenance, but its
		// result, captures, and effects are not. Unknown is the only sound
		// cross-boundary fact without an explicit effect proof.
		if err := builder.addUnknown(SubjectValue, call, 0, call, RoleResult, uint32(index)); err != nil {
			return err
		}
	}
	return nil
}

func (builder *sealBuilder) captures() error {
	functions := builder.authored.Functions()
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			return fmt.Errorf("%w: Function ordinal %d is unavailable", ErrMalformed, index)
		}
		captureCount, countOK := functions.CaptureCount(function)
		if !countOK {
			return fmt.Errorf("%w: Function %v capture range is unavailable", ErrMalformed, function)
		}
		formalCount, formalCountOK := builder.source.Formals().Len(function)
		if !formalCountOK {
			return fmt.Errorf("%w: Function %v formal range is unavailable", ErrMalformed, function)
		}
		for formalIndex := 0; formalIndex < formalCount; formalIndex++ {
			formal, formalOK := builder.source.Formals().At(function, formalIndex)
			if !formalOK {
				return fmt.Errorf("%w: Function %v formal %d is unavailable", ErrMalformed, function, formalIndex)
			}
			if err := builder.addUnknown(SubjectCell, formal, 0, function, RoleCell, uint32(formalIndex)); err != nil {
				return err
			}
		}
		_, _, vararg, functionRowOK := functions.Get(function)
		if !functionRowOK {
			return fmt.Errorf("%w: Function %v row is unavailable", ErrMalformed, function)
		}
		if vararg != 0 {
			if err := builder.addUnknown(SubjectCell, vararg, 0, function, RoleTail, 0); err != nil {
				return err
			}
		}
		for captureIndex := 0; captureIndex < captureCount; captureIndex++ {
			inner, outer, captureOK := functions.CaptureAt(function, captureIndex)
			if !captureOK {
				return fmt.Errorf("%w: Function %v capture %d is unavailable", ErrMalformed, function, captureIndex)
			}
			if err := builder.addUnknown(SubjectCell, inner, outer, function, RoleCapture, uint32(captureIndex)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (builder *sealBuilder) returns() error {
	returns := builder.authored.Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		returnTerm, ok := returns.At(index)
		if !ok {
			return fmt.Errorf("%w: Return ordinal %d is unavailable", ErrMalformed, index)
		}
		owner, values, rowOK := returns.Get(returnTerm)
		if !rowOK {
			return fmt.Errorf("%w: Return %v is unavailable", ErrMalformed, returnTerm)
		}
		if values != 0 {
			if err := builder.addUnknown(SubjectValues, values, 0, returnTerm, RoleReturn, uint32(index)); err != nil {
				return err
			}
			width, widthOK := builder.authored.Values().Len(values)
			if !widthOK {
				return fmt.Errorf("%w: Return %v Values is unavailable", ErrMalformed, returnTerm)
			}
			for memberIndex := 0; memberIndex < width; memberIndex++ {
				member, memberOK := builder.authored.Values().Member(values, memberIndex)
				if !memberOK {
					return fmt.Errorf("%w: Return %v member %d is unavailable", ErrMalformed, returnTerm, memberIndex)
				}
				if err := builder.addUnknownTerm(member, returnTerm, RoleMember, uint32(memberIndex)); err != nil {
					return err
				}
			}
			_, tail, tailOK := builder.authored.Values().Get(values)
			if !tailOK {
				return fmt.Errorf("%w: Return %v Values tail is unavailable", ErrMalformed, returnTerm)
			}
			if tail != 0 {
				if err := builder.addUnknownRelated(subjectKindForTerm(tail), tail, SubjectValues, values, returnTerm, RoleTail, 0); err != nil {
					return err
				}
			}
		} else if owner != 0 {
			if err := builder.addUnknown(SubjectRoot, owner, 0, returnTerm, RoleReturn, uint32(index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (builder *sealBuilder) boundaries() error {
	successors := builder.causal.Successors()
	all := make([]causal.Successor, 0, successors.TotalCount())
	for index := 0; index < successors.TotalCount(); index++ {
		successor, ok := successors.TotalAt(index)
		if !ok {
			return fmt.Errorf("%w: causal successor %d is unavailable", ErrMalformed, index)
		}
		all = append(all, successor)
	}
	// Index normal arms once by their owning Call.  Boundary pairing is a
	// route join, not a quadratic scan over the complete causal union.
	normalsByCall := make(map[keyspace.Term][]causal.Successor)
	for _, successor := range all {
		if isReentryArm(successor.Arm) {
			normalsByCall[successor.From] = append(normalsByCall[successor.From], successor)
		}
	}
	for _, yield := range all {
		if yield.Arm != causal.BoundaryYield {
			continue
		}
		yieldRoute, routeOK := yield.SemanticID()
		if !routeOK || !yieldRoute.Available() {
			return fmt.Errorf("%w: Yield route identity is unavailable", ErrMalformed)
		}
		callPath, callPathOK := builder.callPath(yield.From)
		yieldFrom, yieldFromOK := yield.FromPoint()
		yieldTo, yieldToOK := yield.ToPoint()
		var yieldFromPath, yieldToPath identity.ContentID
		if yieldFromOK {
			yieldFromPath = yieldFrom.PathID()
		}
		if yieldToOK {
			yieldToPath = yieldTo.PathID()
		}
		normals := normalsByCall[yield.From]
		for _, reentry := range normals {
			reentryRoute, reentryRouteOK := reentry.SemanticID()
			reentryFrom, reentryFromOK := reentry.FromPoint()
			reentryTo, reentryToOK := reentry.ToPoint()
			var reentryFromPath, reentryToPath identity.ContentID
			if reentryFromOK {
				reentryFromPath = reentryFrom.PathID()
			}
			if reentryToOK {
				reentryToPath = reentryTo.PathID()
			}
			state := BoundaryPaired
			if !reentryRouteOK || !reentryRoute.Available() || !yieldFromPath.Available() || !yieldToPath.Available() ||
				!reentryFromPath.Available() || !reentryToPath.Available() {
				state = BoundaryUnknown
			}
			if !callPathOK || !callPath.Available() {
				state = BoundaryUnknown
			}
			rowID := boundaryID(yield.From, callPath, state, yield.Arm, reentry.Arm, yieldRoute, reentryRoute, yieldFromPath, yieldToPath, reentryFromPath, reentryToPath)
			if !rowID.Available() {
				return fmt.Errorf("%w: Yield/re-entry identity is unavailable", ErrMalformed)
			}
			builder.boundariesRows = append(builder.boundariesRows, Boundary{ID: rowID, State: state, Call: yield.From, CallPath: callPath, YieldArm: yield.Arm, YieldRoute: yieldRoute, YieldFromPath: yieldFromPath, YieldToPath: yieldToPath, ReentryArm: reentry.Arm, ReentryRoute: reentryRoute, ReentryFromPath: reentryFromPath, ReentryToPath: reentryToPath})
		}
		if len(normals) == 0 {
			rowID := boundaryID(yield.From, callPath, BoundaryUnknown, yield.Arm, 0, yieldRoute, identity.ContentID{}, yieldFromPath, yieldToPath, identity.ContentID{}, identity.ContentID{})
			if !rowID.Available() {
				return fmt.Errorf("%w: terminal Yield identity is unavailable", ErrMalformed)
			}
			builder.boundariesRows = append(builder.boundariesRows, Boundary{ID: rowID, State: BoundaryUnknown, Call: yield.From, CallPath: callPath, YieldArm: yield.Arm, YieldRoute: yieldRoute, YieldFromPath: yieldFromPath, YieldToPath: yieldToPath})
		}
	}
	return nil
}

func isReentryArm(arm causal.BoundaryArmKind) bool {
	return arm == causal.BoundaryResume || arm == causal.BoundarySelectTrue || arm == causal.BoundarySelectFalse
}
