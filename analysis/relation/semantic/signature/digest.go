package signature

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

func digest(value Signature) identity.ContentID {
	parts := make([][]byte, 0, 20+len(value.inputs)*8+len(value.outputs)*4)
	parts = append(parts,
		nominal(value.identity.Operation.Owner().Content(), value.identity.Operation.Content()),
		uint64Bytes(value.identity.Version),
		contentBytes(value.fence.Owner.Content()),
		nominal(value.fence.Schema.Owner().Content(), value.fence.Schema.Content()),
		uint64Bytes(uint64(len(value.inputs))))
	for _, input := range value.inputs {
		parts = append(parts, relationBytes(input.Relation), columnBytes(input.Column), typeBytes(input.Type), denominatorBytes(input.Denominator), []byte{byte(input.Presence), byte(input.Delivery.Kind), byte(input.AuthorityKind())}, uint64Bytes(uint64(input.Delivery.Bound)), keyBytes(input.Delivery.Order))
		if source, joined := input.SourceAuthority.Denominator(); joined {
			parts = append(parts, denominatorBytes(source))
		} else {
			// The carrier denominator above already identifies the homogeneous
			// source.  Keep a fixed arm marker so the closed authority sum is
			// nevertheless part of the signature identity.
			parts = append(parts, []byte{0})
		}
	}
	parts = append(parts, uint64Bytes(uint64(len(value.outputs))))
	for _, output := range value.outputs {
		parts = append(parts, relationBytes(output.Relation), columnBytes(output.Column), typeBytes(output.Type), denominatorBytes(output.Denominator), []byte{byte(output.Presence)})
	}
	bound := uint32(0)
	if value, ok := value.cardinality.Bound(); ok {
		bound = value
	}
	parts = append(parts, []byte{byte(value.cardinality.Kind())}, uint64Bytes(uint64(bound)),
		codeBytes(value.outcomes.Codes()))
	result, ok := identity.DeriveContentID("relation/semantic/signature/v2", parts...)
	if !ok {
		return identity.ContentID{}
	}
	return result
}

func nominal(owner, content identity.ContentID) []byte {
	return append(append([]byte(nil), owner[:]...), content[:]...)
}

func relationBytes(value model.RelationID) []byte {
	return nominal(value.Owner().Content(), value.Content())
}

func columnBytes(value model.ColumnID) []byte {
	return append(relationBytes(value.Relation()), contentBytes(value.Content())...)
}

func typeBytes(value model.TypeID) []byte {
	return nominal(value.Owner().Content(), value.Content())
}

func keyBytes(value model.KeyID) []byte {
	if !value.Available() {
		return []byte{0}
	}
	return append(relationBytes(value.Relation()), contentBytes(value.Content())...)
}

func denominatorBytes(value model.DenominatorRef) []byte {
	key := value.Key()
	result := append(relationBytes(value.Relation()), relationBytes(key.Relation())...)
	return append(result, contentBytes(key.Content())...)
}

func contentBytes(value identity.ContentID) []byte { return append([]byte(nil), value[:]...) }

func codeBytes(values []outcome.Code) []byte {
	encoded := make([]byte, len(values))
	for index, value := range values {
		encoded[index] = byte(value)
	}
	return encoded
}

func uint64Bytes(value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return append([]byte(nil), encoded[:]...)
}
