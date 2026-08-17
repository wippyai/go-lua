package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Body is a Program-owned view over one existing Flow Body boundary.
// It retains no body/outcome rows and exposes only existing Causal Sites plus
// the optional Function boundary needed for formal/capture access.
type Body struct {
	program  *Program
	boundary flow.BodyBoundary
	function flow.FunctionBoundary
}

// BodyCount forwards Source's sole canonical Body denominator. It does not
// retain an input-owned index or promote the Function-boundary join to one.
func (input *Program) BodyCount() int {
	if !input.Available() {
		return 0
	}
	return input.Source().Identity().FamilyCount(keyspace.FamilyBody)
}

// BodyAt returns one existing Body view in canonical Body order.
func (input *Program) BodyAt(index int) (Body, bool) {
	if !input.Available() || index < 0 || index >= input.BodyCount() {
		return Body{}, false
	}
	term := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
	return input.Body(term)
}

// Body joins an authored Body to its published boundary. Root/non-Function
// Bodies retain an unavailable Function boundary rather than a fabricated one.
func (input *Program) Body(term keyspace.Term) (Body, bool) {
	if !input.Available() {
		return Body{}, false
	}
	boundaries := input.Flow().FunctionBoundaries()
	boundary, ok := boundaries.ForBody(term)
	if !ok {
		return Body{}, false
	}
	function, _ := boundaries.ForFunctionBody(term)
	body := Body{program: input, boundary: boundary, function: function}
	return body, body.Available()
}

// OwnsBody authenticates a Body issued by this exact Program.
// Equivalent replay Bodies deliberately do not pass: mount-local consumers
// must retain their own issued view rather than substitute one.
func (input *Program) OwnsBody(body Body) bool {
	if !input.Available() || body.program != input || !body.Available() {
		return false
	}
	boundaries := input.Flow().FunctionBoundaries()
	if !boundaries.OwnsBody(body.boundary) {
		return false
	}
	if body.function.Available() {
		return boundaries.OwnsFunction(body.function)
	}
	return true
}

// OwnsSite authenticates an exact hot Causal Site issued by this Program.
// Equivalent replay Sites are intentionally rejected at mount-local joins.
func (input *Program) OwnsSite(site flow.Site) bool {
	if !input.Available() || !site.Available() {
		return false
	}
	term, ok := site.Term()
	if !ok {
		return false
	}
	sites := input.Flow().Causal().Sites()
	issued, ok := sites.ForTerm(term)
	return ok && sites.Owns(site) && issued.Equal(site) && issued.ContextID() == site.ContextID()
}

// Outcome resolves one existing Body Outcome through FunctionBoundary's sole
// dense inverse and the existing Causal Site table. It does not scan a Body's
// Outcome range or create another outcome index.
func (input *Program) Outcome(term keyspace.Term) (Outcome, bool) {
	if !input.Available() || term == 0 {
		return Outcome{}, false
	}
	boundary, boundaryOK := input.Flow().FunctionBoundaries().ForOutcome(term)
	bodyTerm, bodyTermOK := boundary.Body()
	body, bodyOK := input.Body(bodyTerm)
	exit, ordinal, exitOK := boundary.OutcomeForTerm(term)
	if !boundaryOK || !bodyTermOK || !bodyOK || !body.boundary.Equal(boundary) || !exitOK || exit.Outcome != term {
		return Outcome{}, false
	}
	site, _ := input.Flow().Causal().Sites().ForTerm(term)
	outcome := Outcome{ordinal: ordinal, body: body, site: site, kind: exit.Kind, target: exit.Target}
	return outcome, outcome.Available()
}

// ContainingBody resolves Source's existing lexical containment internally
// and returns only the opaque Body proof. It does not expose the containing
// raw Body coordinate for callers to rejoin.
func (input *Program) ContainingBody(term keyspace.Term) (Body, bool) {
	if !input.Available() {
		return Body{}, false
	}
	body, _, _, ok := input.Source().Index().Position(term)
	if !ok {
		return Body{}, false
	}
	return input.Body(body)
}

func (body Body) Available() bool {
	if !body.program.Available() || !body.boundary.Available() {
		return false
	}
	term, ok := body.boundary.Body()
	if !ok {
		return false
	}
	want, wantOK := body.program.Flow().FunctionBoundaries().ForBody(term)
	if !wantOK || !body.boundary.Equal(want) {
		return false
	}
	function, functionOK := body.program.Flow().FunctionBoundaries().ForFunctionBody(term)
	if functionOK {
		return body.function.Available() && body.function.Equal(function)
	}
	return !body.function.Available()
}

// Equal compares the existing exact-quartet Body boundary proof. It never
// compares or exposes an authored Body term.
func (body Body) Equal(other Body) bool {
	return body.Available() && other.Available() && body.boundary.Equal(other.boundary)
}

// ContextID is Flow's stable exact-quartet identity for this Body boundary.
func (body Body) ContextID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	return body.boundary.ContextID()
}

// ProgramID returns the already-published Program owner of this exact Body
// proof. It is a scalar provenance fence for reusable artifact consumers;
// it neither reopens Program state nor exposes the authored Body term.
func (body Body) ProgramID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	return body.program.ContentID()
}

// PathID returns Flow's owner-local lexical Body path. Unlike ContextID it
// carries no quartet identity and is therefore suitable for semantic
// descriptors that must replay across equivalent owner publication.
func (body Body) PathID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	term, ok := body.boundary.Body()
	if !ok {
		return identity.ContentID{}
	}
	path, ok := body.program.Flow().BodyPath(term)
	if !ok {
		return identity.ContentID{}
	}
	return path
}

// Executable reports the exact sealed Flow executable membership for this
// Body. It is a scalar proof copied for artifact boundary filtering.
func (body Body) Executable() bool {
	if !body.Available() {
		return false
	}
	term, ok := body.boundary.Body()
	return ok && body.program.Flow().Executable().Contains(term)
}

// RootCount returns Source's existing root denominator for this Body. The
// Body term remains internal to this proof-native join.
func (body Body) RootCount() (int, bool) {
	if !body.Available() {
		return 0, false
	}
	term, ok := body.boundary.Body()
	if !ok {
		return 0, false
	}
	return body.program.Source().Index().BodyRootLen(term)
}

// Root is one Source-owned Body root proof. Its Flow span is optional because
// Source roots include non-executable authored structure; callers never
// receive the containing raw Body coordinate to rejoin themselves.
type Root struct {
	body     Body
	ordinal  int
	authored keyspace.Term
	span     Span
}

func (root Root) Available() bool {
	if !root.body.Available() || root.authored == 0 {
		return false
	}
	term, ok := root.body.boundary.Body()
	if !ok {
		return false
	}
	candidate, ok := root.body.program.Source().Index().BodyRootAt(term, root.ordinal)
	if !ok || candidate != root.authored {
		return false
	}
	issued, issuedOK := root.body.program.Span(root.authored)
	if issuedOK != root.span.Available() {
		return false
	}
	return !issuedOK || (root.body.program.OwnsSpan(root.span) && root.span == issued)
}

func (root Root) Authored() (keyspace.Term, bool) {
	if !root.Available() {
		return 0, false
	}
	return root.authored, true
}

// Executable forwards Flow's exact executable membership for this authored
// Source root. Span availability is deliberately not used as a classifier.
func (root Root) Executable() bool {
	return root.Available() && root.body.program.Flow().Executable().Contains(root.authored)
}

// Span returns the existing Flow Span if Ports/Causal publish one. Source-root
// executability and Span availability are independent sealed relations.
func (root Root) Span() (Span, bool) {
	if !root.Available() || !root.span.Available() {
		return Span{}, false
	}
	return root.span, true
}

// RootAt returns one existing Source root proof. It attempts the existing
// Flow join once but does not reject non-executable Source structure.
func (body Body) RootAt(index int) (Root, bool) {
	if !body.Available() || index < 0 {
		return Root{}, false
	}
	term, ok := body.boundary.Body()
	if !ok {
		return Root{}, false
	}
	root, ok := body.program.Source().Index().BodyRootAt(term, index)
	if !ok {
		return Root{}, false
	}
	span, _ := body.program.Span(root)
	result := Root{body: body, ordinal: index, authored: root, span: span}
	return result, result.Available()
}

// ExecutableRoot is an artifact-safe proof of one direct executable Body
// root. It carries only Flow's sealed semantic identity, family, and dense
// executable ordinal: neither the authored Term nor its Span may cross the
// ProgramArtifact boundary.
type ExecutableRoot struct {
	catalog *executableRootCatalog
	ordinal int
	id      identity.ContentID
	family  keyspace.Family
}

// ExecutableRoots is Program's complete dense artifact-safe root catalog for
// one Body. Construction fails closed when any Source denominator row cannot
// be joined to Flow; consumers never infer a denominator by silently skipping
// malformed source rows.
type ExecutableRoots struct {
	catalog *executableRootCatalog
}

type executableRootCatalog struct {
	body   Body
	rows   []ExecutableRoot
	sealed bool
}

func (roots ExecutableRoots) Available() bool {
	return roots.catalog != nil && roots.catalog.sealed && roots.catalog.body.Available()
}
func (roots ExecutableRoots) Count() int {
	if !roots.Available() {
		return 0
	}
	return len(roots.catalog.rows)
}
func (roots ExecutableRoots) At(index int) (ExecutableRoot, bool) {
	if !roots.Available() || index < 0 || index >= len(roots.catalog.rows) {
		return ExecutableRoot{}, false
	}
	root := roots.catalog.rows[index]
	return root, root.Available()
}

func (root ExecutableRoot) Available() bool {
	if root.catalog == nil || !root.catalog.sealed || !root.catalog.body.Available() || root.ordinal < 0 || !root.id.Available() || root.family == keyspace.FamilyInvalid || root.ordinal >= len(root.catalog.rows) {
		return false
	}
	issued := root.catalog.rows[root.ordinal]
	return issued.catalog == root.catalog && issued.ordinal == root.ordinal && issued.id == root.id && issued.family == root.family
}
func (root ExecutableRoot) ID() identity.ContentID {
	if !root.Available() {
		return identity.ContentID{}
	}
	return root.id
}
func (root ExecutableRoot) Family() keyspace.Family {
	if !root.Available() {
		return keyspace.FamilyInvalid
	}
	return root.family
}

// ExecutableRootCount is the dense denominator after filtering Source roots
// through Flow's sealed executable and semantic-path proofs.
func (body Body) ExecutableRootCount() int {
	roots, ok := body.ExecutableRoots()
	if !ok {
		return 0
	}
	return roots.Count()
}

// ExecutableRootAt issues one dense artifact-safe root proof.
func (body Body) ExecutableRootAt(index int) (ExecutableRoot, bool) {
	roots, ok := body.ExecutableRoots()
	if !ok {
		return ExecutableRoot{}, false
	}
	return roots.At(index)
}

func (body Body) ExecutableRoots() (ExecutableRoots, bool) {
	if !body.Available() {
		return ExecutableRoots{}, false
	}
	count, countOK := body.RootCount()
	if !countOK {
		return ExecutableRoots{}, false
	}
	bodyTerm, bodyOK := body.boundary.Body()
	if !bodyOK {
		return ExecutableRoots{}, false
	}
	catalog := &executableRootCatalog{body: body, rows: make([]ExecutableRoot, 0, count)}
	for sourceIndex := 0; sourceIndex < count; sourceIndex++ {
		authored, rootOK := body.program.Source().Index().BodyRootAt(bodyTerm, sourceIndex)
		if !rootOK {
			return ExecutableRoots{}, false
		}
		if !body.program.Flow().Executable().Contains(authored) {
			continue
		}
		id, idOK := body.program.Flow().SemanticTermPath(authored)
		root := ExecutableRoot{catalog: catalog, ordinal: len(catalog.rows), id: id, family: keyspace.TermFamily(authored)}
		if !idOK || !root.id.Available() || root.family == keyspace.FamilyInvalid {
			return ExecutableRoots{}, false
		}
		catalog.rows = append(catalog.rows, root)
	}
	catalog.sealed = true
	return ExecutableRoots{catalog: catalog}, true
}

func (input *Program) OwnsExecutableRoot(root ExecutableRoot) bool {
	return input.Available() && root.catalog != nil && root.catalog.body.program == input && input.OwnsBody(root.catalog.body) && root.Available()
}
func (input *Program) OwnsExecutableRoots(roots ExecutableRoots) bool {
	return input.Available() && roots.catalog != nil && roots.catalog.body.program == input && input.OwnsBody(roots.catalog.body) && roots.Available()
}

// Function returns the existing sealed Flow boundary for this Body. It is the
// remaining construction seam: Artifact consumes scalar callable identities
// through Program queries and never retains this handle.
func (body Body) Function() (flow.FunctionBoundary, bool) {
	if !body.Available() || !body.function.Available() {
		return flow.FunctionBoundary{}, false
	}
	return body.function, true
}

// EntrySite returns the existing Causal Site at this Body's boundary Entry.
func (body Body) EntrySite() (flow.Site, bool) {
	if !body.Available() {
		return flow.Site{}, false
	}
	entry, ok := body.boundary.Entry()
	if !ok {
		return flow.Site{}, false
	}
	return body.program.Flow().Causal().Sites().ForTerm(entry)
}
