package program

import "github.com/wippyai/go-lua/analysis/program/keyspace"

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
