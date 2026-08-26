package algebra

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func derive(tag string, parts ...[]byte) identity.ContentID {
	digest, ok := identity.DeriveContentID(tag, parts...)
	if !ok {
		return identity.ContentID{}
	}
	return digest
}

func appendUint8(dst []byte, value uint8) []byte { return append(dst, value) }

func appendUint32s(dst []byte, values []uint32) []byte {
	dst = appendLength(dst, len(values))
	for _, value := range values {
		dst = appendUint32(dst, value)
	}
	return dst
}

func appendUint32(dst []byte, value uint32) []byte {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], value)
	return append(dst, encoded[:]...)
}

func appendLength(dst []byte, length int) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(length))
	return append(dst, encoded[:]...)
}

func appendBytes(dst, value []byte) []byte {
	dst = appendLength(dst, len(value))
	return append(dst, value...)
}

func appendContent(dst []byte, value identity.ContentID) []byte {
	return appendBytes(dst, value[:])
}

func appendOwner(dst []byte, value model.OwnerID) []byte {
	return appendContent(dst, value.Content())
}

func appendRelation(dst []byte, value model.RelationID) []byte {
	dst = appendOwner(dst, value.Owner())
	return appendContent(dst, value.Content())
}

func appendColumn(dst []byte, value model.ColumnID) []byte {
	dst = appendRelation(dst, value.Relation())
	return appendContent(dst, value.Content())
}

func appendType(dst []byte, value model.TypeID) []byte {
	dst = appendOwner(dst, value.Owner())
	return appendContent(dst, value.Content())
}

func appendKey(dst []byte, value model.KeyID) []byte {
	dst = appendRelation(dst, value.Relation())
	return appendContent(dst, value.Content())
}

func appendScope(dst []byte, value model.ScopeSchema) []byte {
	dst = appendOwner(dst, value.Owner())
	dst = appendContent(dst, value.ID().Content())
	return appendColumns(dst, value.Dimensions())
}

func appendScopeID(dst []byte, value model.ScopeID) []byte {
	dst = appendOwner(dst, value.Owner())
	return appendContent(dst, value.Content())
}

func appendOperation(dst []byte, value signature.Identity) []byte {
	dst = appendOwner(dst, value.Operation.Owner())
	dst = appendContent(dst, value.Operation.Content())
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value.Version)
	return append(dst, encoded[:]...)
}

func appendColumns(dst []byte, values []model.ColumnID) []byte {
	dst = appendLength(dst, len(values))
	for _, value := range values {
		dst = appendColumn(dst, value)
	}
	return dst
}

func appendCardinality(dst []byte, value model.Cardinality) []byte {
	dst = appendUint8(dst, uint8(value.Kind()))
	bound, ok := value.Bound()
	if !ok {
		bound = 0
	}
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], bound)
	return append(dst, encoded[:]...)
}

func appendDenominator(dst []byte, value model.DenominatorRef) []byte {
	dst = appendRelation(dst, value.Relation())
	return appendKey(dst, value.Key())
}

func appendExpr(dst []byte, value Expression) []byte {
	if value == nil {
		return append(dst, 0)
	}
	dst = append(dst, 1)
	digest := value.Digest()
	return append(dst, digest[:]...)
}

func appendExprs(dst []byte, values []Expression) []byte {
	dst = appendLength(dst, len(values))
	for _, value := range values {
		dst = appendExpr(dst, value)
	}
	return dst
}

func cloneExpressions(source []Expression) []Expression {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([]Expression, len(source))
	copy(copyOf, source)
	return copyOf
}

func cloneColumns(source []model.ColumnID) []model.ColumnID {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([]model.ColumnID, len(source))
	copy(copyOf, source)
	return copyOf
}

func cloneUint32s(source []uint32) []uint32 {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([]uint32, len(source))
	copy(copyOf, source)
	return copyOf
}

func cloneSlotSources(source []SlotSource) []SlotSource {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([]SlotSource, len(source))
	copy(copyOf, source)
	return copyOf
}

func cloneColumnSlots(source []ColumnSlot) []ColumnSlot {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([]ColumnSlot, len(source))
	copy(copyOf, source)
	return copyOf
}

func cloneMappings(source []ColumnMapping) []ColumnMapping {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([]ColumnMapping, len(source))
	copy(copyOf, source)
	return copyOf
}
