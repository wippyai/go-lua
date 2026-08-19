package boot

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	semanticCodecVersion uint64 = 8
	semanticInitialValue uint64 = 91
	semanticBootRelation uint64 = 92
)

func (t *Table) semanticID(kind uint64, encode func(*framing.Writer) error) (identity.ContentID, error) {
	if t == nil || encode == nil {
		return identity.ContentID{}, errors.New("target/boot: missing semantic identity")
	}
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
		return identity.ContentID{}, errors.New("target/boot: semantic digest failure")
	}
	return id, nil
}

func (t *Table) sealValueIdentities(keys exactkey.Table, operations operation.Core) ([]identity.ContentID, error) {
	ids := make([]identity.ContentID, t.values.Count())
	for index := 0; index < t.values.Count(); index++ {
		value := vocabulary.InitialValue(index + 1)
		id, err := t.semanticID(semanticInitialValue, func(writer *framing.Writer) error {
			return t.encodeInitialValueContent(writer, value, keys, operations)
		})
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	return ids, nil
}

func (t *Table) encodeInitialValueContent(writer *framing.Writer, value vocabulary.InitialValue, keys exactkey.Table, operations operation.Core) error {
	row, ok := t.initialValue(value)
	if !ok {
		return errors.New("target/boot: malformed initial value")
	}
	if err := writer.Uint(uint64(row.kind)); err != nil {
		return err
	}
	switch row.kind {
	case vocabulary.InitialValueNil, vocabulary.InitialValueAbsent:
		return nil
	case vocabulary.InitialValueBoolean:
		return writer.Bool(row.boolean)
	case vocabulary.InitialValueInteger:
		return writer.Uint(uint64(row.integer))
	case vocabulary.InitialValueFloat:
		return writer.Uint(row.floatBits)
	case vocabulary.InitialValueString:
		return writer.String(row.string)
	case vocabulary.InitialValueRoot:
		identity, ok := t.InitialRootIdentity(row.root)
		if !ok {
			return errors.New("target/boot: malformed initial value root")
		}
		return writer.String(identity)
	case vocabulary.InitialValueOperation:
		if row.operation == 0 {
			return errors.New("target/boot: missing initial operation anchor")
		}
		anchor, ok := operations.Anchor(row.operation)
		if !ok || !anchor.Available() {
			return errors.New("target/boot: missing initial operation anchor")
		}
		return writer.Bytes(anchor[:])
	case vocabulary.InitialValueDeniedOperation:
		binding, ok := t.initialValueBinding(value)
		if !ok {
			return errors.New("target/boot: malformed denied initial value")
		}
		if err := writer.Uint(uint64(binding.namespace)); err != nil {
			return err
		}
		if err := writer.Count(uint64(binding.ownerKeys.Len())); err != nil {
			return err
		}
		for index := 0; index < binding.ownerKeys.Len(); index++ {
			key, ok := t.bindingKeys.At(binding.ownerKeys, index)
			if !ok {
				return errors.New("target/boot: malformed denied owner key")
			}
			if err := encodeExactKey(writer, keys, key); err != nil {
				return err
			}
		}
		if err := writer.Count(uint64(binding.memberKeys.Len())); err != nil {
			return err
		}
		for index := 0; index < binding.memberKeys.Len(); index++ {
			key, ok := t.bindingKeys.At(binding.memberKeys, index)
			if !ok {
				return errors.New("target/boot: malformed denied member key")
			}
			if err := encodeExactKey(writer, keys, key); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("target/boot: invalid initial value kind")
	}
}

func (t *Table) sealBootIdentity() (identity.ContentID, error) {
	return t.semanticID(semanticBootRelation, func(writer *framing.Writer) error {
		if err := writer.Count(uint64(t.roots.Count())); err != nil {
			return err
		}
		for index := 0; index < t.roots.Count(); index++ {
			root, ok := t.roots.At(index)
			if !ok {
				return errors.New("target/boot: malformed boot root")
			}
			if err := writer.String(root.identity); err != nil {
				return err
			}
			if root.shape == 0 || uint64(root.shape) > uint64(t.shapes.Count()) {
				return errors.New("target/boot: malformed boot root shape")
			}
			shape, ok := t.shapes.At(int(root.shape - 1))
			if !ok {
				return errors.New("target/boot: malformed boot root shape")
			}
			if shape.root != vocabulary.InitialRoot(index+1) {
				return errors.New("target/boot: malformed boot root relation")
			}
			if err := writer.Uint(uint64(shape.aggregate)); err != nil {
				return err
			}
			if err := writer.Bool(shape.immutable); err != nil {
				return err
			}
			if shape.value == 0 || uint64(shape.value) > uint64(len(t.valueIDs)) {
				return errors.New("target/boot: malformed boot root value")
			}
			id := t.valueIDs[shape.value-1]
			if err := writer.Bytes(id[:]); err != nil {
				return err
			}
		}
		if err := writer.Count(uint64(t.metatables.Count())); err != nil {
			return err
		}
		for index := 0; index < t.metatables.Count(); index++ {
			attachment, ok := t.metatables.At(index)
			if !ok {
				return errors.New("target/boot: malformed initial metatable")
			}
			if attachment.metatable == 0 || uint64(attachment.metatable) > uint64(t.roots.Count()) {
				return errors.New("target/boot: malformed initial metatable")
			}
			if err := writer.Uint(uint64(attachment.base)); err != nil {
				return err
			}
			identity, ok := t.InitialRootIdentity(attachment.metatable)
			if !ok {
				return errors.New("target/boot: malformed initial metatable root")
			}
			if err := writer.String(identity); err != nil {
				return err
			}
		}
		return nil
	})
}

// InitialValueContentID is a portable identity for one exact boot value.
func (t *Table) InitialValueContentID(value vocabulary.InitialValue) (identity.ContentID, bool) {
	if t == nil || value == 0 || uint64(value) > uint64(len(t.valueIDs)) {
		return identity.ContentID{}, false
	}
	id := t.valueIDs[value-1]
	if !id.Available() {
		return identity.ContentID{}, false
	}
	return id, true
}

// BootRelationID commits root identities, shapes, and metatable attachments.
func (t *Table) BootRelationID() (identity.ContentID, bool) {
	if t == nil || !t.bootID.Available() {
		return identity.ContentID{}, false
	}
	return t.bootID, true
}
