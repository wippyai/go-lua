package programschema

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// ValueSourceCode maps the canonical literal/TypeValue families to the
// historical ValueSource occurrence codes. The numeric mapping is part of
// the published identity and remains independent of declaration order.
func ValueSourceCode(family keyspace.Family) (uint8, bool) {
	switch family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString:
		return uint8(family), true
	case keyspace.FamilyTypeValue:
		return 6, true
	default:
		return 0, false
	}
}

// ValueSourceAnchorIdentity issues the owner-neutral anchor equation for a
// source term's direct or lexical-root evaluation geometry.
func ValueSourceAnchorIdentity(direct bool, path identity.ContentID) (identity.ContentID, bool) {
	if !path.Available() {
		return identity.ContentID{}, false
	}
	return valueSourceSemanticIdentity("program/transformer/value-source-anchor", func(writer *framing.Writer) bool {
		return writer.Bool(direct) == nil && writer.Bytes(path[:]) == nil
	}, path)
}

// ValueSourceIdentity issues the owner-neutral scalar identity of one
// authored literal or TypeValue source row.
func ValueSourceIdentity(code uint8, bodyPath, bodyID, anchorID identity.ContentID) (identity.ContentID, bool) {
	if code < 1 || code > 6 {
		return identity.ContentID{}, false
	}
	return valueSourceSemanticIdentity("program/transformer/value-source-occurrence", func(writer *framing.Writer) bool {
		return writer.Uint(uint64(code)) == nil && writer.Bytes(bodyPath[:]) == nil &&
			writer.Bytes(bodyID[:]) == nil && writer.Bytes(anchorID[:]) == nil
	}, bodyPath, bodyID, anchorID)
}

// ValueSourceSpanIdentity is the exact root-fenced span equation used when a
// positionless source term falls back to its lexical root. Direct evaluation
// spans continue to come from Program's existing Span query; this codec keeps
// the fallback bytes identical without retaining a Program source row.
func ValueSourceSpanIdentity(programID identity.ContentID, authored keyspace.Term, entryID, finishID identity.ContentID) (identity.ContentID, bool) {
	if authored == 0 || keyspace.TermFamily(authored) == keyspace.FamilyInvalid || keyspace.TermOrdinal(authored) == 0 {
		return identity.ContentID{}, false
	}
	return valueSourceRoleIdentity("program/transformer/span", programID, func(writer *framing.Writer) bool {
		return writer.Uint(uint64(keyspace.TermFamily(authored))) == nil &&
			writer.Uint(uint64(keyspace.TermOrdinal(authored))) == nil && writer.Bytes(entryID[:]) == nil &&
			writer.Bytes(finishID[:]) == nil
	}, entryID, finishID)
}

func valueSourceSemanticIdentity(domain string, write func(*framing.Writer) bool, fields ...identity.ContentID) (identity.ContentID, bool) {
	for _, field := range fields {
		if !field.Available() {
			return identity.ContentID{}, false
		}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func valueSourceRoleIdentity(domain string, owner identity.ContentID, write func(*framing.Writer) bool, fields ...identity.ContentID) (identity.ContentID, bool) {
	if !owner.Available() {
		return identity.ContentID{}, false
	}
	for _, field := range fields {
		if !field.Available() {
			return identity.ContentID{}, false
		}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || writer.Bytes(owner[:]) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}
