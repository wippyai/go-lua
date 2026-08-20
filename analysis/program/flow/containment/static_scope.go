package containment

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// emitStaticFallbacks closes the one cross-owner part of the static query
// boundary.  TypeOf and Annotation retain their authored scope handles in
// Static; the ordinary occurrence remains owned by Flow.  If that occurrence
// has no ordinary consumer, the kernel may use its proven lexical Body as the
// only deferred parent.  This function does not emit static containment
// edges, and its roots are reference endpoints for the later static-mark
// projection rather than kernel roots.
func emitStaticFallbacks(
	preimage source.Preimage,
	staticView staticquery.View,
	view authored.View,
	resolver *staticScopeResolver,
	counts [keyspace.FamilyCount]uint32,
) (fallback []kernelEdge, roots []keyspace.Term, err error) {
	if err := validateStaticFallbackInputs(preimage, staticView, view, counts); err != nil {
		return nil, nil, err
	}
	if resolver == nil {
		return nil, nil, errors.New("program/flow/containment: nil static scope resolver")
	}

	// Endpoint roots are a dense membership derivative.  They are intentionally
	// appended in authored TypeOf order followed by authored Annotation order;
	// no map or sort can become a second ordering authority here.
	var seen [keyspace.FamilyCount][]bool
	// Scope forwarding is an owner-issued semantic boundary, not a generic
	// fallback graph. Keep its rank construction-local and keyed by the exact
	// endpoint so repeated TypeOf/Annotation observations retain one role.
	var scopeRanks = make(map[keyspace.Term]uint32)
	var nextScopeRank = make(map[keyspace.Term]uint32)
	appendFallback := func(child, parent keyspace.Term) {
		rank, ok := scopeRanks[child]
		if !ok {
			nextScopeRank[parent]++
			rank = nextScopeRank[parent]
			scopeRanks[child] = rank
		}
		fallback = append(fallback, kernelEdge{
			child: child, parent: parent,
			role: structuralRoleStaticScope, rank: rank,
		})
	}
	addRoot := func(endpoint keyspace.Term) error {
		if !flowrole.ValueOccurrence(counts, endpoint) &&
			!termInFamily(endpoint, keyspace.FamilyValues, counts) {
			return errors.New("program/flow/containment: invalid static reference endpoint")
		}
		family := keyspace.TermFamily(endpoint)
		ordinal := keyspace.TermOrdinal(endpoint)
		if seen[family] == nil {
			seen[family] = make([]bool, int(counts[family]))
		}
		if seen[family][ordinal-1] {
			return nil
		}
		seen[family][ordinal-1] = true
		roots = append(roots, endpoint)
		return nil
	}

	typeOfs := staticView.Operators().TypeOfs()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeOf]; ordinal++ {
		typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, ordinal)
		at, ok := typeOfs.At(int(ordinal - 1))
		if !ok || at != typeOf {
			return nil, nil, errors.New("program/flow/containment: noncanonical TypeOf ordinal")
		}
		scope, operand, ok := typeOfs.Get(typeOf)
		if !ok || !staticrole.ScopeHandle(counts, scope) || !flowrole.ValueOccurrence(counts, operand) {
			return nil, nil, errors.New("program/flow/containment: invalid TypeOf reference")
		}
		body, _, ok := resolver.resolveObservation(scope)
		if !ok {
			return nil, nil, errors.New("program/flow/containment: TypeOf scope does not resolve")
		}
		owner, ok := flowLexicalOwner(preimage, view, operand, counts)
		if !ok || owner != body {
			return nil, nil, errors.New("program/flow/containment: TypeOf operand crosses lexical owner")
		}
		appendFallback(operand, body)
		if err := addRoot(operand); err != nil {
			return nil, nil, err
		}
	}

	annotations := staticView.Operands().Annotations()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyAnnotation]; ordinal++ {
		annotation := keyspace.MakeTerm(keyspace.FamilyAnnotation, ordinal)
		at, ok := annotations.At(int(ordinal - 1))
		if !ok || at != annotation {
			return nil, nil, errors.New("program/flow/containment: noncanonical Annotation ordinal")
		}
		row, ok := annotations.Get(annotation)
		if !ok || !staticrole.ScopeHandle(counts, row.Scope) ||
			!staticrole.AnnotationTarget(counts, row.Target) || row.Name == 0 ||
			!termInFamily(row.Values, keyspace.FamilyValues, counts) {
			return nil, nil, errors.New("program/flow/containment: invalid Annotation reference")
		}
		body, _, ok := resolver.resolveObservation(row.Scope)
		if !ok {
			return nil, nil, errors.New("program/flow/containment: Annotation scope does not resolve")
		}
		owner, ok := flowLexicalOwner(preimage, view, row.Values, counts)
		if !ok || owner != body {
			return nil, nil, errors.New("program/flow/containment: Annotation Values crosses lexical owner")
		}
		appendFallback(row.Values, body)
		if err := addRoot(row.Values); err != nil {
			return nil, nil, err
		}
	}
	return fallback, roots, nil
}

// staticScopeResolver is a dense memoized forwarding walk.  state 1 is an
// active path, state 2 is a completed answer (body zero means rejected).  A
// path is entered and finalized iteratively, so a maliciously deep or cyclic
// static scope cannot consume the Go call stack or require a depth budget.
type staticScopeResolver struct {
	static staticquery.View
	view   authored.View
	counts [keyspace.FamilyCount]uint32
	body   [keyspace.FamilyCount][]keyspace.Term
	obs    [keyspace.FamilyCount][]scopeObservation
	state  [keyspace.FamilyCount][]uint8
	path   []keyspace.Term
}

// newStaticScopeResolver is the sole construction boundary for the shared
// static-scope walk.  Memo planes are allocated lazily when a family is first
// reached; only the reusable iterative path scratch is initialized here.
func newStaticScopeResolver(staticView staticquery.View, view authored.View, counts [keyspace.FamilyCount]uint32) *staticScopeResolver {
	return &staticScopeResolver{
		static: staticView,
		view:   view,
		counts: counts,
		path:   make([]keyspace.Term, 0, 8),
	}
}

// resolveObservation resolves one static-scope handle and retains the one
// terminal observation needed by the assembly-local Static proof.  The body
// and observation are memoized by the same forwarding walk; this is not a
// second resolver or relation.  A rejected path is cached with a zero body
// and zero observation so cycles and malformed foreign handles fail closed.
func (r *staticScopeResolver) resolveObservation(start keyspace.Term) (keyspace.Term, scopeObservation, bool) {
	if !staticrole.ScopeHandle(r.counts, start) {
		return 0, scopeObservation{}, false
	}
	path := r.path[:0]
	current := start
	body := keyspace.Term(0)
	observation := scopeObservation{}
	valid := false
	for current != 0 {
		if !staticrole.ScopeHandle(r.counts, current) {
			break
		}
		family := keyspace.TermFamily(current)
		ordinal := keyspace.TermOrdinal(current)
		if !staticrole.ScopeHandleFamily(family) {
			break
		}
		if r.state[family] == nil {
			count := r.counts[family]
			if count == 0 {
				break
			}
			r.body[family] = make([]keyspace.Term, int(count))
			r.obs[family] = make([]scopeObservation, int(count))
			r.state[family] = make([]uint8, int(count))
		}
		states := r.state[family]
		bodies := r.body[family]
		observations := r.obs[family]
		if ordinal == 0 || uint64(ordinal) > uint64(len(states)) {
			break
		}
		switch states[ordinal-1] {
		case 2:
			body = bodies[ordinal-1]
			observation = observations[ordinal-1]
			valid = body != 0
			current = 0
			continue
		case 1:
			// A state-1 node can only be on this path: cycles are finalized
			// as rejected below, so no stale in-progress marker survives.
			current = 0
			valid = false
			continue
		}
		states[ordinal-1] = 1
		path = append(path, current)

		next, direct, terminal, ok := r.step(current)
		if !ok {
			current = 0
			valid = false
			continue
		}
		if direct != 0 {
			body = direct
			observation = terminal
			valid = true
			current = 0
			continue
		}
		current = next
	}

	if !valid {
		body = 0
		observation = scopeObservation{}
	}
	for _, term := range path {
		family := keyspace.TermFamily(term)
		ordinal := keyspace.TermOrdinal(term)
		r.state[family][ordinal-1] = 2
		r.body[family][ordinal-1] = body
		r.obs[family][ordinal-1] = observation
	}
	r.path = path
	return body, observation, valid
}

func (r *staticScopeResolver) step(term keyspace.Term) (next, direct keyspace.Term, terminal scopeObservation, ok bool) {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyCell:
		// A global Cell has no lexical Body and is never a valid static scope.
		// Binding's Host is a definition identity (Bind, Loop, or Function),
		// not a replacement for the Cell's authored lexical Body: formal and
		// loop hosts deliberately differ from that Body.  The Cell row remains
		// the sole lexical-Body authority.
		cellKind, lexicalBody, key, cellOK := r.view.Storage().Cells().Get(term)
		if !cellOK || cellKind != authored.CellLocal || key != 0 ||
			!termInFamily(lexicalBody, keyspace.FamilyBody, r.counts) {
			return 0, 0, scopeObservation{}, false
		}
		return 0, lexicalBody, scopeObservation{kind: ScopeObservationCellIntroduction, term: term}, true

	case keyspace.FamilyTypeAlias:
		owner, _, _, _, rowOK := r.static.Declarations().Aliases().Get(term)
		body, ok := ownerBody(owner, rowOK, r.counts)
		return 0, body, scopeObservation{kind: ScopeObservationSourceOccurrence, term: term}, ok
	case keyspace.FamilyTypeInterface:
		owner, _, _, rowOK := r.static.Declarations().Interfaces().Get(term)
		body, ok := ownerBody(owner, rowOK, r.counts)
		return 0, body, scopeObservation{kind: ScopeObservationSourceOccurrence, term: term}, ok
	case keyspace.FamilyTypeParam:
		owner, _, _, rowOK := r.static.Declarations().TypeParams().Get(term)
		if !rowOK || !staticrole.ScopeHandle(r.counts, owner) {
			return 0, 0, scopeObservation{}, false
		}
		if keyspace.TermFamily(owner) == keyspace.FamilyFunction {
			functionOwner, _, _, rowOK := r.view.Functions().Get(owner)
			body, ok := ownerBody(functionOwner, rowOK, r.counts)
			return 0, body, scopeObservation{kind: ScopeObservationFunctionGeneric, term: owner}, ok
		}
		return owner, 0, scopeObservation{}, true
	case keyspace.FamilyTypeFunction:
		scope, _, _, _, rowOK := r.static.Signatures().TypeFunctions().Get(term)
		if !rowOK || !staticrole.ScopeHandle(r.counts, scope) {
			return 0, 0, scopeObservation{}, false
		}
		return scope, 0, scopeObservation{}, true
	case keyspace.FamilyValueClaim:
		owner, _, _, rowOK := r.view.Claims().Get(term)
		body, ok := ownerBody(owner, rowOK, r.counts)
		return 0, body, scopeObservation{kind: ScopeObservationSourceOccurrence, term: term}, ok
	case keyspace.FamilyCall:
		owner, _, _, _, rowOK := r.view.Calls().Get(term)
		body, ok := ownerBody(owner, rowOK, r.counts)
		return 0, body, scopeObservation{kind: ScopeObservationSourceOccurrence, term: term}, ok
	case keyspace.FamilyAnnotation:
		row, rowOK := r.static.Operands().Annotations().Get(term)
		if !rowOK || !staticrole.ScopeHandle(r.counts, row.Scope) {
			return 0, 0, scopeObservation{}, false
		}
		return row.Scope, 0, scopeObservation{}, true
	case keyspace.FamilyFunction:
		_, functionBody, _, rowOK := r.view.Functions().Get(term)
		body, ok := ownerBody(functionBody, rowOK, r.counts)
		return 0, body, scopeObservation{kind: ScopeObservationFunctionHeader, term: term}, ok
	default:
		return 0, 0, scopeObservation{}, false
	}
}

func ownerBody(owner keyspace.Term, ok bool, counts [keyspace.FamilyCount]uint32) (keyspace.Term, bool) {
	if !ok || !termInFamily(owner, keyspace.FamilyBody, counts) {
		return 0, false
	}
	return owner, true
}

func validateStaticFallbackInputs(
	preimage source.Preimage,
	staticView staticquery.View,
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
) error {
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return errors.New("program/flow/containment: invalid static fallback denominator")
	}
	identity := preimage.Identity()
	if !identity.ContentID().Available() || identity.Name() == "" || identity.TermCount() == 0 ||
		!staticView.Available() || !view.ContentID().Available() {
		return errors.New("program/flow/containment: static fallback owner view expired")
	}
	return nil
}
