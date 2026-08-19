package accessgeometry

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// Available reports whether this projection is a sealed result for a complete
// owner quartet. It remains true for a valid sealed result with zero rows, so
// callers never infer availability from a count.
func (result *Result) Available() bool { return result.available() }

// ExactRead resolves one exact selector chain to its root and depth.
func (result *Result) ExactRead(read keyspace.Term) (keyspace.Term, int, bool) {
	return result.ExactReads().Get(read)
}

// ExactReadPath opens one immutable leaf-to-root cursor over a sealed exact
// selector chain. Each Segment advances one existing parent-chain edge in O(1)
// without materializing or restarting the path.
func (result *Result) ExactReadPath(read keyspace.Term) (ExactReadPath, bool) {
	return result.ExactReads().PathCursor(read)
}

// TypePublication resolves one Static type publication to its root, owner,
// and depth.
func (result *Result) TypePublication(publication keyspace.Term) (root, owner keyspace.Term, depth int, ok bool) {
	return result.TypePublications().Get(publication)
}

// TypePublicationPath opens one immutable leaf-to-root cursor over the sealed
// exact Static publication path. Segment is O(1) and allocation-free.
func (result *Result) TypePublicationPath(publication keyspace.Term) (PublicationPath, bool) {
	return result.TypePublications().PathCursor(publication)
}

// CallForm is the closed direct-call syntax disposition.
type CallForm uint8

const (
	CallFormPlain  CallForm = 1
	CallFormMethod CallForm = 2
)

// DirectCall resolves one direct call to its callee Read and typed syntax
// disposition. An unknown stored form fails closed as an invalid CallForm.
func (result *Result) DirectCall(call keyspace.Term) (keyspace.Term, CallForm, bool) {
	read, form, ok := result.DirectCalls().Get(call)
	if !ok {
		return 0, 0, false
	}
	switch form {
	case 1:
		return read, CallFormPlain, true
	case 2:
		return read, CallFormMethod, true
	default:
		return read, 0, true
	}
}
