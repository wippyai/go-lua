package program

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// valuesTailKind is the private, closed numeric classification used by the
// Values identity equations. Its order is part of the identity format:
// zero is no tail, one is a Call tail, and two is a Vararg tail. The kind is
// deliberately not exported; callers need only the scalar IDs.
type valuesTailKind uint8

const (
	valuesTailInvalid valuesTailKind = iota
	valuesTailCall
	valuesTailVararg
)

// ValuesOccurrenceID returns the canonical identity of one authored Values
// row. The row is read directly from the sealed Flow view on every call; no
// catalog, row, or proof handle is retained by this query.
func (program *Program) ValuesOccurrenceID(term keyspace.Term) (identity.ContentID, bool) {
	rootPath, width, tailPath, tailKind, ok := program.valuesRow(term)
	if !ok {
		return identity.ContentID{}, false
	}
	members := make([]identity.ContentID, width)
	for index := range members {
		member, memberOK := program.ValuesMemberID(term, index)
		if !memberOK {
			return identity.ContentID{}, false
		}
		members[index] = member
	}
	id := programSemanticID("program/transformer/values-occurrence", func(writer *framing.Writer) bool {
		if writer.Bytes(rootPath[:]) != nil || writer.Uint(uint64(len(members))) != nil {
			return false
		}
		for _, member := range members {
			if writer.Bytes(member[:]) != nil {
				return false
			}
		}
		return writer.Uint(uint64(tailKind)) == nil && writer.Bytes(tailPath[:]) == nil
	})
	return id, id.Available()
}

// ValuesMemberID returns the canonical identity of one ordered member in an
// authored Values row. The index is the authored fixed-position index, not a
// directory ordinal.
func (program *Program) ValuesMemberID(term keyspace.Term, index int) (identity.ContentID, bool) {
	rootPath, width, _, _, ok := program.valuesRow(term)
	if !ok || index < 0 || index >= width {
		return identity.ContentID{}, false
	}
	flowView := program.Flow()
	memberTerm, memberOK := flowView.Authored().Values().Member(term, index)
	memberPath, pathOK := flowView.SemanticTermPath(memberTerm)
	if !memberOK || !pathOK || !memberPath.Available() {
		return identity.ContentID{}, false
	}
	id := programSemanticID("program/transformer/values-member", func(writer *framing.Writer) bool {
		return writer.Bytes(rootPath[:]) == nil && writer.Uint(uint64(index)) == nil && writer.Bytes(memberPath[:]) == nil
	})
	return id, id.Available()
}

// ValuesTailID returns the canonical identity of an authored Values tail.
// Closed Values rows return no ID. The private tail classification is derived
// from the sealed authored family and contributes to the identity exactly as
// it did in the former Program proof and Artifact copy equations.
func (program *Program) ValuesTailID(term keyspace.Term) (identity.ContentID, bool) {
	_, _, tailPath, tailKind, ok := program.valuesRow(term)
	if !ok || tailKind == valuesTailInvalid || !tailPath.Available() {
		return identity.ContentID{}, false
	}
	id := programSemanticID("program/transformer/tail-producer", func(writer *framing.Writer) bool {
		return writer.Uint(uint64(tailKind)) == nil && writer.Bytes(tailPath[:]) == nil
	})
	return id, id.Available()
}

// valuesRow returns only the scalar inputs needed by the identity equations.
// It does not retain the authored row, member list, or tail proof. A Values
// owner must be a canonical Body and an open tail must name an authored Call
// or Vararg row. Executability is a later projection and cannot shrink this
// fixed authored denominator.
func (program *Program) valuesRow(term keyspace.Term) (rootPath identity.ContentID, width int, tailPath identity.ContentID, tailKind valuesTailKind, ok bool) {
	if program == nil || !program.ContentID().Available() || keyspace.TermFamily(term) != keyspace.FamilyValues {
		return identity.ContentID{}, 0, identity.ContentID{}, valuesTailInvalid, false
	}
	flowView := program.Flow()
	values := flowView.Authored().Values()
	owner, tail, rowOK := values.Get(term)
	width, widthOK := values.Len(term)
	rootPath, rootOK := flowView.SemanticTermPath(term)
	if !rowOK || !widthOK || width < 0 || !rootOK || !rootPath.Available() || keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return identity.ContentID{}, 0, identity.ContentID{}, valuesTailInvalid, false
	}
	if tail == 0 {
		return rootPath, width, identity.ContentID{}, valuesTailInvalid, true
	}
	tailPath, tailKind, tailOK := program.valuesTail(tail)
	if !tailOK {
		return identity.ContentID{}, 0, identity.ContentID{}, valuesTailInvalid, false
	}
	return rootPath, width, tailPath, tailKind, true
}

func (program *Program) valuesTail(term keyspace.Term) (identity.ContentID, valuesTailKind, bool) {
	if program == nil || term == 0 {
		return identity.ContentID{}, valuesTailInvalid, false
	}
	flowView := program.Flow()
	var kind valuesTailKind
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyCall:
		_, _, _, _, callOK := flowView.Authored().Calls().Get(term)
		if !callOK {
			return identity.ContentID{}, valuesTailInvalid, false
		}
		kind = valuesTailCall
	case keyspace.FamilyVararg:
		_, _, varargOK := flowView.Authored().Storage().Varargs().Get(term)
		if !varargOK {
			return identity.ContentID{}, valuesTailInvalid, false
		}
		kind = valuesTailVararg
	default:
		return identity.ContentID{}, valuesTailInvalid, false
	}
	path, pathOK := flowView.SemanticTermPath(term)
	return path, kind, pathOK && path.Available()
}

// programSemanticID is Program's owner-neutral codec for identities derived
// entirely from canonical authored structure. It retains no row or proof.
func programSemanticID(domain string, write func(*framing.Writer) bool) identity.ContentID {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

// programRoleID is Program's owner-fenced companion for identities whose
// canonical structure is reusable but still belongs to one published root.
func programRoleID(domain string, owner identity.ContentID, write func(*framing.Writer) bool) identity.ContentID {
	if !owner.Available() || write == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || writer.Bytes(owner[:]) != nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
