package operation

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

const semanticCodecVersion uint64 = 8

const (
	semanticOperationAnchor uint64 = 80
	semanticProducedAnchor  uint64 = 81
	semanticOpaqueAnchor    uint64 = 82
)

// operationAnchor issues the stable semantic anchor for one already-frozen
// binding set. The operation owner, rather than Boot, is the only issuer of
// this identity.
func operationAnchor(bindings []vocabulary.BindingSpec, keys exactkey.Table) (identity.ContentID, error) {
	for _, binding := range bindings {
		if !vocabulary.ValidBinding(binding) {
			return identity.ContentID{}, errors.New("target/operation: invalid operation binding")
		}
	}
	return semanticID(semanticOperationAnchor, func(writer *framing.Writer) error {
		if err := writer.Count(uint64(len(bindings))); err != nil {
			return err
		}
		for _, binding := range bindings {
			if err := writer.Uint(uint64(binding.Namespace)); err != nil {
				return err
			}
			if err := encodeBindingSegments(writer, binding.Owner, keys); err != nil {
				return err
			}
			if err := encodeBindingSegments(writer, binding.Member, keys); err != nil {
				return err
			}
		}
		return nil
	})
}

func encodeBindingSegments(writer *framing.Writer, segments []string, keys exactkey.Table) error {
	if err := writer.Count(uint64(len(segments))); err != nil {
		return err
	}
	for _, segment := range segments {
		key, ok := keys.Handle(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment})
		if !ok {
			return errors.New("target/operation: unresolved operation binding key")
		}
		if err := exactkey.Encode(writer, &keys, key); err != nil {
			return err
		}
	}
	return nil
}

func semanticID(kind uint64, encode func(*framing.Writer) error) (identity.ContentID, error) {
	hash := sha256.New()
	var writer framing.Writer
	if err := writer.Reset(hash, "program/target-semantic", semanticCodecVersion); err != nil {
		return identity.ContentID{}, err
	}
	if err := writer.Record(kind); err != nil {
		return identity.ContentID{}, err
	}
	if err := encode(&writer); err != nil {
		return identity.ContentID{}, err
	}
	if err := writer.Finish(); err != nil {
		return identity.ContentID{}, err
	}
	var id identity.ContentID
	if got := hash.Sum(id[:0]); len(got) != len(id) {
		return identity.ContentID{}, errors.New("target/operation: semantic digest failure")
	}
	return id, nil
}

// CompileAnchors returns the second immutable operation value. It resolves
// exact-key-dependent binding anchors and then derives produced anchors in
// canonical parent-before-child order. Geometry itself is never modified.
func CompileAnchors(geometry Geometry, keys exactkey.Table) (Core, error) {
	if geometry.operations.Count() < 1 {
		return Core{}, errors.New("target/operation: unavailable geometry")
	}
	anchors := make([]anchorRow, geometry.operations.Count())
	incoming := make([]producedRow, geometry.operations.Count())
	foundIncoming := make([]bool, geometry.operations.Count())
	for index := 0; index < geometry.sourceN; index++ {
		operation, ok := geometry.operations.At(index)
		if !ok {
			return Core{}, errors.New("target/operation: malformed operation geometry")
		}
		for _, row := range geometry.produced.All(operation.produced) {
			child := int(row.child) - 1
			parent := int(row.parent) - 1
			if parent < 0 || parent >= geometry.sourceN || child < 0 || child >= geometry.sourceN {
				return Core{}, errors.New("target/operation: produced anchor operation outside geometry")
			}
			childRow, childOK := geometry.operations.At(child)
			if !childOK {
				return Core{}, errors.New("target/operation: produced child outside geometry")
			}
			if geometry.bindings.Count(childRow.bindings) == 0 {
				if foundIncoming[child] {
					return Core{}, errors.New("target/operation: duplicate produced anchor parent")
				}
				foundIncoming[child] = true
				incoming[child] = row
			}
		}
	}
	for index := 0; index < geometry.sourceN; index++ {
		row, ok := geometry.operations.At(index)
		if !ok {
			return Core{}, errors.New("target/operation: malformed operation geometry")
		}
		bindings, err := geometry.bindingSpecs(row.bindings)
		if err != nil {
			return Core{}, err
		}
		if len(bindings) != 0 {
			id, anchorErr := operationAnchor(bindings, keys)
			if anchorErr != nil {
				return Core{}, anchorErr
			}
			anchors[index] = anchorRow{id: id}
			continue
		}
		if !foundIncoming[index] {
			return Core{}, errors.New("target/operation: produced-only operation has no parent")
		}
		parent := incoming[index]
		if parent.parent == 0 || int(parent.parent) > index {
			return Core{}, errors.New("target/operation: produced anchor parent is not canonical predecessor")
		}
		parentAnchor := anchors[int(parent.parent)-1].id
		if !parentAnchor.Available() {
			return Core{}, errors.New("target/operation: unresolved produced parent anchor")
		}
		parentRow, ok := geometry.operations.At(int(parent.parent) - 1)
		if !ok || parent.outcome >= uint32(parentRow.outcomes.Len()) {
			return Core{}, errors.New("target/operation: malformed produced outcome")
		}
		outcome, selectorOK := geometry.outcomes.At(parentRow.outcomes, int(parent.outcome))
		if !selectorOK {
			return Core{}, errors.New("target/operation: malformed produced outcome selector")
		}
		selector := make([]byte, geometry.anchors.Count(outcome.anchor))
		for selectorIndex := range selector {
			value, valueOK := geometry.anchors.At(outcome.anchor, selectorIndex)
			if !valueOK {
				return Core{}, errors.New("target/operation: malformed produced outcome selector")
			}
			selector[selectorIndex] = value
		}
		id, anchorErr := semanticID(semanticProducedAnchor, func(writer *framing.Writer) error {
			if err := writer.Bytes(parentAnchor[:]); err != nil {
				return err
			}
			if err := writer.Bytes(selector); err != nil {
				return err
			}
			if err := writer.Uint(uint64(parent.outcome)); err != nil {
				return err
			}
			return writer.Uint(uint64(parent.result))
		})
		if anchorErr != nil {
			return Core{}, anchorErr
		}
		anchors[index] = anchorRow{id: id}
	}
	opaqueIndex := geometry.operations.Count() - 1
	opaque, anchorErr := semanticID(semanticOpaqueAnchor, func(writer *framing.Writer) error {
		return writer.Uint(1)
	})
	if anchorErr != nil {
		return Core{}, anchorErr
	}
	anchors[opaqueIndex] = anchorRow{id: opaque}
	for _, anchor := range anchors {
		if !anchor.id.Available() {
			return Core{}, errors.New("target/operation: unresolved operation anchor")
		}
	}
	return Core{geometry: geometry, anchors: rows.NewRows(anchors)}, nil
}

func (geometry Geometry) bindingSpecs(span rows.Span) ([]vocabulary.BindingSpec, error) {
	bindings := make([]vocabulary.BindingSpec, geometry.bindings.Count(span))
	for index := range bindings {
		row, ok := geometry.bindings.At(span, index)
		if !ok {
			return nil, errors.New("target/operation: malformed binding geometry")
		}
		owner := make([]string, geometry.segments.Count(row.owner))
		for segment := range owner {
			value, valueOK := geometry.segments.At(row.owner, segment)
			if !valueOK {
				return nil, errors.New("target/operation: malformed binding owner")
			}
			owner[segment] = value
		}
		member := make([]string, geometry.segments.Count(row.member))
		for segment := range member {
			value, valueOK := geometry.segments.At(row.member, segment)
			if !valueOK {
				return nil, errors.New("target/operation: malformed binding member")
			}
			member[segment] = value
		}
		bindings[index] = vocabulary.BindingSpec{Namespace: row.namespace, Owner: owner, Member: member}
	}
	return bindings, nil
}
