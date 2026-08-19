package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// ValuesOccurrenceID returns the canonical identity of one authored Values
// row. Values is a Flow-owned authored relation; the query therefore consumes
// its sealed row and semantic path directly rather than reopening Program.
func (view View) ValuesOccurrenceID(term keyspace.Term) (identity.ContentID, bool) {
	rootPath, width, tailPath, tailKind, ok := view.valuesRow(term)
	if !ok {
		return identity.ContentID{}, false
	}
	id := flowSemanticID("program/transformer/values-occurrence", func(writer *framing.Writer) bool {
		if writer.Bytes(rootPath[:]) != nil || writer.Uint(uint64(width)) != nil {
			return false
		}
		for index := 0; index < width; index++ {
			member, memberOK := view.ValuesMemberID(term, index)
			if !memberOK || writer.Bytes(member[:]) != nil {
				return false
			}
		}
		return writer.Uint(uint64(tailKind)) == nil && writer.Bytes(tailPath[:]) == nil
	})
	return id, id.Available()
}

// ValuesShape returns the fixed width and whether the authored Values row has
// an open Call or Vararg tail. Flow owns this classification; consumers must
// not repeat its row and tail validation.
func (view View) ValuesShape(term keyspace.Term) (width int, open bool, ok bool) {
	_, width, _, tailKind, ok := view.valuesRow(term)
	return width, ok && tailKind != valuesTailInvalid, ok
}

// ValuesMemberID returns the canonical identity of one ordered member in an
// authored Values row. The index is the authored fixed-position index, not a
// directory ordinal.
func (view View) ValuesMemberID(term keyspace.Term, index int) (identity.ContentID, bool) {
	rootPath, width, _, _, ok := view.valuesRow(term)
	if !ok || index < 0 || index >= width {
		return identity.ContentID{}, false
	}
	memberTerm, memberOK := view.Authored().Values().Member(term, index)
	memberPath, pathOK := view.SemanticTermPath(memberTerm)
	if !memberOK || !pathOK || !memberPath.Available() {
		return identity.ContentID{}, false
	}
	id := flowSemanticID("program/transformer/values-member", func(writer *framing.Writer) bool {
		return writer.Bytes(rootPath[:]) == nil && writer.Uint(uint64(index)) == nil && writer.Bytes(memberPath[:]) == nil
	})
	return id, id.Available()
}

// ValuesTailID returns the canonical identity of an authored Values tail.
// Closed Values rows return no ID. The tail kind is derived from the sealed
// authored Call or Vararg relation and contributes to the identity exactly as
// it did in the former Program projection.
func (view View) ValuesTailID(term keyspace.Term) (identity.ContentID, bool) {
	_, _, tailPath, tailKind, ok := view.valuesRow(term)
	if !ok || tailKind == valuesTailInvalid || !tailPath.Available() {
		return identity.ContentID{}, false
	}
	id := flowSemanticID("program/transformer/tail-producer", func(writer *framing.Writer) bool {
		return writer.Uint(uint64(tailKind)) == nil && writer.Bytes(tailPath[:]) == nil
	})
	return id, id.Available()
}

// valuesTailKind is private to Flow's Values owner. Its numeric order is part
// of the identity format: zero is no tail, one is a Call tail, and two is a
// Vararg tail.
type valuesTailKind uint8

const (
	valuesTailInvalid valuesTailKind = iota
	valuesTailCall
	valuesTailVararg
)

// valuesRow returns only the scalar inputs needed by the Values identity
// equations. A Values owner must be a canonical Body and an open tail must
// name an authored Call or Vararg row. Executability is a later projection and
// cannot shrink this fixed authored denominator.
func (view View) valuesRow(term keyspace.Term) (rootPath identity.ContentID, width int, tailPath identity.ContentID, tailKind valuesTailKind, ok bool) {
	if !view.available() || keyspace.TermFamily(term) != keyspace.FamilyValues {
		return identity.ContentID{}, 0, identity.ContentID{}, valuesTailInvalid, false
	}
	values := view.Authored().Values()
	owner, tail, rowOK := values.Get(term)
	width, widthOK := values.Len(term)
	rootPath, rootOK := view.SemanticTermPath(term)
	if !rowOK || !widthOK || width < 0 || !rootOK || !rootPath.Available() || keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return identity.ContentID{}, 0, identity.ContentID{}, valuesTailInvalid, false
	}
	if tail == 0 {
		return rootPath, width, identity.ContentID{}, valuesTailInvalid, true
	}
	tailPath, tailKind, tailOK := view.valuesTail(tail)
	if !tailOK {
		return identity.ContentID{}, 0, identity.ContentID{}, valuesTailInvalid, false
	}
	return rootPath, width, tailPath, tailKind, true
}

func (view View) valuesTail(term keyspace.Term) (identity.ContentID, valuesTailKind, bool) {
	if !view.available() || term == 0 {
		return identity.ContentID{}, valuesTailInvalid, false
	}
	var kind valuesTailKind
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyCall:
		_, _, _, _, callOK := view.Authored().Calls().Get(term)
		if !callOK {
			return identity.ContentID{}, valuesTailInvalid, false
		}
		kind = valuesTailCall
	case keyspace.FamilyVararg:
		_, _, varargOK := view.Authored().Storage().Varargs().Get(term)
		if !varargOK {
			return identity.ContentID{}, valuesTailInvalid, false
		}
		kind = valuesTailVararg
	default:
		return identity.ContentID{}, valuesTailInvalid, false
	}
	path, pathOK := view.SemanticTermPath(term)
	return path, kind, pathOK && path.Available()
}
