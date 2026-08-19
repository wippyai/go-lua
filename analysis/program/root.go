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
