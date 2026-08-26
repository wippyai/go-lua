package derivation

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

func digestPath(path Path) (identity.ContentID, bool) {
	parts := make([][]byte, 0, 4+len(path.readColumns)+len(path.frames)*4)
	parts = append(parts, nominalBytes(path.root.Owner().Content(), path.root.Content()))
	appendUint32(&parts, path.occurrence)
	parts = append(parts, contentBytes(path.node))
	parts = append(parts, nominalBytes(path.leafRelation.Owner().Content(), path.leafRelation.Content()))
	for _, column := range path.readColumns {
		parts = append(parts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
	}
	parts = append(parts, accessDigest(path.leaf.access), contentBytes(path.leaf.physical))
	for _, frame := range path.frames {
		frameParts := [][]byte{[]byte{byte(frame.kind), byte(frame.orientation)}, contentBytes(frame.node)}
		appendUint32(&frameParts, frame.ordinal)
		if frame.scope.Available() {
			frameParts = append(frameParts, nominalBytes(frame.scope.Owner().Content(), frame.scope.Content()))
		}
		if frame.denominator.Available() {
			frameParts = append(frameParts, nominalBytes(frame.denominator.Relation().Owner().Content(), frame.denominator.Relation().Content()))
			frameParts = append(frameParts, nominalBytes(frame.denominator.Key().Relation().Owner().Content(), frame.denominator.Key().Content()))
		}
		if frame.operation.Available() {
			frameParts = append(frameParts, nominalBytes(frame.operation.Operation.Owner().Content(), frame.operation.Operation.Content()))
			appendUint64(&frameParts, frame.operation.Version)
		}
		if frame.destination.Available() {
			frameParts = append(frameParts, nominalBytes(frame.destination.Owner().Content(), frame.destination.Content()))
		}
		if frame.key.Available() {
			frameParts = append(frameParts, nominalBytes(frame.key.Relation().Owner().Content(), frame.key.Content()))
		}
		if frame.cardinality.Available() {
			frameParts = append(frameParts, []byte{byte(frame.cardinality.Kind())})
			if bound, boundOK := frame.cardinality.Bound(); boundOK {
				appendUint32(&frameParts, bound)
			} else {
				appendUint32(&frameParts, 0)
			}
		}
		if frame.completeReplay.Available() {
			frameParts = append(frameParts, contentBytes(frame.completeReplay.Digest()))
		}
		if frame.expandContract.Available() {
			frameParts = append(frameParts, contentBytes(frame.expandContract.Digest()), contentBytes(frame.expandEvidence))
		}
		for _, column := range frame.columns {
			frameParts = append(frameParts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
		}
		for _, slot := range frame.slots {
			frameParts = append(frameParts, nominalBytes(slot.Column().Relation().Owner().Content(), slot.Column().Content()))
			appendUint32(&frameParts, slot.Cell())
		}
		for _, source := range frame.sources {
			appendUint32(&frameParts, source.Child())
			appendUint32(&frameParts, source.Cell())
		}
		for _, target := range frame.mapTargets {
			frameParts = append(frameParts, nominalBytes(target.Relation().Owner().Content(), target.Content()))
		}
		for _, sibling := range frame.siblings {
			frameParts = append(frameParts, accessDigest(sibling.access), contentBytes(sibling.physical))
		}
		// Merge carries a second, complete authored-child projection. Keep it
		// separate from the active-child-omitting sibling projection above so a
		// newly issued child physical vector necessarily changes the sealed path
		// identity even when the ordinary zipper sibling view is unchanged.
		for _, child := range frame.children {
			appendUint32(&frameParts, child.ordinal)
			frameParts = append(frameParts, accessDigest(child.value.access), contentBytes(child.value.physical), contentBytes(child.node), []byte{byte(child.kind)})
		}
		frameDigest, ok := identity.DeriveContentID(pathDigestDomain+"/frame", frameParts...)
		if !ok {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(frameDigest))
	}
	return identity.DeriveContentID(pathDigestDomain, parts...)
}

func accessDigest(access Access) []byte {
	parts := [][]byte{nominalBytes(access.relation.Owner().Content(), access.relation.Content())}
	if access.key.Available() {
		parts = append(parts, nominalBytes(access.key.Relation().Owner().Content(), access.key.Content()))
	} else {
		parts = append(parts, nil)
	}
	for _, column := range access.columns {
		parts = append(parts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
	}
	value, _ := identity.DeriveContentID(pathDigestDomain+"/access", parts...)
	return contentBytes(value)
}

func nominalBytes(owner, content identity.ContentID) []byte {
	result := make([]byte, 0, len(owner)+len(content))
	result = append(result, owner[:]...)
	return append(result, content[:]...)
}

func contentBytes(value identity.ContentID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}

func appendUint32(parts *[][]byte, value uint32) {
	encoded := make([]byte, 4)
	binary.BigEndian.PutUint32(encoded, value)
	*parts = append(*parts, encoded)
}

func appendUint64(parts *[][]byte, value uint64) {
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, value)
	*parts = append(*parts, encoded)
}
