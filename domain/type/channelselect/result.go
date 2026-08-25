// Package channelselect owns pure channel-select result type schema.
package channelselect

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// Role is the semantic-role spelling of the channel-select case fact family.
// The structure row is declared from the type domain declaration.
const Role = "fact/channel-select-case"

// FamilyKey is the structural identity of the channel-select case fact
// family. Correlation is this family keyed by parent-issued select-site
// identity plus case ordinal.
const FamilyKey schema.Key = "semantic/fact/channel-select-case"

const caseFactIdentityDomain = "analysis/channel-select-case/v1"

const (
	ResultChannelField = "channel"
	ResultValueField   = "value"
	ResultOKField      = "ok"
	ResultDefaultField = "default"
	DefaultCaseIndex   = -1
)

// ResultCase describes one receive arm of a select result union.
type ResultCase struct {
	Index   int
	Channel typ.Type
	Payload typ.Type
}

// CaseFact is the correlation identity of one accepted select arm: the
// parent-issued select site plus the case ordinal. It is not encoded in the
// result type.
type CaseFact struct {
	Site             identity.ContentID
	Ordinal          int
	Channel, Payload typ.Type
}

// CaseFactAvailable reports a usable select-site/ordinal pair. Default is not
// a receive ordinal.
func CaseFactAvailable(fact CaseFact) bool {
	return fact.Site.Available() && fact.Ordinal >= 0 && fact.Ordinal != DefaultCaseIndex
}

// CaseFactID is the fact identity of one accepted select arm. A Snapshot
// publisher writes a row at this identity; the identity itself is not a
// published column.
func CaseFactID(fact CaseFact) (identity.ContentID, bool) {
	if !CaseFactAvailable(fact) {
		return identity.ContentID{}, false
	}
	var ordinal [8]byte
	binary.BigEndian.PutUint64(ordinal[:], uint64(fact.Ordinal))
	return identity.DeriveContentID(caseFactIdentityDomain, fact.Site[:], ordinal[:])
}

// ResultCaseType builds one member of the select result union. The record
// carries only the public runtime fields of channel.select.
func ResultCaseType(channel, payload typ.Type) typ.Type {
	if channel == nil {
		channel = typ.Never
	}
	if payload == nil {
		payload = typ.Never
	}
	return typetable.NewRecord().
		Field(ResultChannelField, channel).
		Field(ResultValueField, payload).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.Nil).
		Build()
}

func resultDefaultType() typ.Type {
	return typetable.NewRecord().
		Field(ResultChannelField, typ.Nil).
		Field(ResultValueField, typ.Nil).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.LiteralBool(true)).
		Build()
}

// ResultValueTypeWithDefault builds the select result union from case
// payloads plus an optional default arm.
func ResultValueTypeWithDefault(cases []ResultCase, hasDefault bool) (typ.Type, bool) {
	if len(cases) == 0 && !hasDefault {
		return nil, false
	}
	caseTypes := make([]typ.Type, 0, len(cases)+1)
	for _, c := range cases {
		if c.Index == DefaultCaseIndex {
			continue
		}
		caseTypes = append(caseTypes, ResultCaseType(c.Channel, c.Payload))
	}
	if hasDefault {
		caseTypes = append(caseTypes, resultDefaultType())
	}
	if len(caseTypes) == 0 {
		return nil, false
	}
	return normalize.UnionForEvidence(caseTypes...), true
}

// ResultWithoutCase removes one explicit receive case from resultType by
// public member identity. Default members are preserved. A user-authored
// lookalike that is not that member is left in place.
func ResultWithoutCase(resultType, caseType typ.Type) (typ.Type, bool) {
	if caseType == nil || isDefaultMember(caseType) {
		return nil, false
	}
	resultType = unwrap.Annotations(resultType)
	if union, ok := resultType.(*typ.Union); ok {
		kept := make([]typ.Type, 0, len(union.Members))
		removed := false
		for _, member := range union.Members {
			if typ.TypeEquals(member, caseType) {
				removed = true
				continue
			}
			kept = append(kept, member)
		}
		if !removed {
			return nil, false
		}
		if len(kept) == 0 {
			return typ.Never, true
		}
		return normalize.UnionForEvidence(kept...), true
	}
	if typ.TypeEquals(resultType, caseType) {
		return typ.Never, true
	}
	return nil, false
}

func isDefaultMember(caseType typ.Type) bool {
	record, ok := unwrap.Alias(unwrap.Annotations(caseType)).(*typ.Record)
	if !ok {
		return false
	}
	field := record.GetField(ResultDefaultField)
	if field == nil {
		return false
	}
	return typ.TypeEquals(field.Type, typ.LiteralBool(true))
}

// ResultCaseTypeFromValue returns the matching select result case type, if any.
func ResultCaseTypeFromValue(resultType, caseType typ.Type) (typ.Type, bool) {
	if caseType == nil {
		return nil, false
	}
	resultType = unwrap.Annotations(resultType)
	if union, ok := resultType.(*typ.Union); ok {
		for _, member := range union.Members {
			if typ.TypeEquals(member, caseType) {
				return member, true
			}
		}
		return nil, false
	}
	if typ.TypeEquals(resultType, caseType) {
		return resultType, true
	}
	return nil, false
}
