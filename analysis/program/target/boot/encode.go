package boot

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	recordInitialRoot                uint64 = 25
	recordBootShape                  uint64 = 26
	recordInitialEntry               uint64 = 27
	recordInitialBinding             uint64 = 28
	recordInitialValue               uint64 = 29
	recordInitialMetatableAttachment uint64 = 31
)

// Encode writes the canonical complete-contract contribution of this owner.
// The exact-key table is a shared immutable dependency, never a second boot
// literal pool.
func (t *Table) Encode(writer *framing.Writer, keys exactkey.Table) error {
	if t == nil {
		return errors.New("target/boot: unavailable table")
	}
	if err := writer.Count(uint64(t.roots.Count())); err != nil {
		return err
	}
	for index := 0; index < t.roots.Count(); index++ {
		root, ok := t.roots.At(index)
		if !ok {
			return errors.New("target/boot: malformed initial root")
		}
		rootHandle := vocabulary.InitialRoot(index + 1)
		if err := writer.Record(recordInitialRoot); err != nil {
			return err
		}
		if err := writer.Uint(uint64(rootHandle)); err != nil {
			return err
		}
		if err := writer.String(root.identity); err != nil {
			return err
		}
		// ModulePath is the authored Target relation between this root and a
		// module path. It is part of the canonical Contract identity; consumers
		// must never recover it from root.identity.
		if err := writer.String(root.modulePath); err != nil {
			return err
		}
		if root.shape == 0 || uint64(root.shape) > uint64(t.shapes.Count()) {
			return errors.New("target/boot: malformed boot shape")
		}
		shape, ok := t.shapes.At(int(root.shape - 1))
		if !ok {
			return errors.New("target/boot: malformed boot shape")
		}
		if shape.root != rootHandle {
			return errors.New("target/boot: malformed boot shape root")
		}
		if err := writer.Record(recordBootShape); err != nil {
			return err
		}
		if err := writer.Uint(uint64(root.shape)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(shape.aggregate)); err != nil {
			return err
		}
		if err := writer.Bool(shape.immutable); err != nil {
			return err
		}
		if err := encodeInitialValue(writer, t, shape.value, keys); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(t.entries.Count())); err != nil {
		return err
	}
	for index := 0; index < t.entries.Count(); index++ {
		entry, ok := t.entries.At(index)
		if !ok {
			return errors.New("target/boot: malformed initial entry")
		}
		if err := writer.Record(recordInitialEntry); err != nil {
			return err
		}
		if err := writer.Uint(uint64(entry.root)); err != nil {
			return err
		}
		if err := encodeExactKey(writer, keys, entry.key); err != nil {
			return err
		}
		if err := writer.Uint(uint64(entry.mutability)); err != nil {
			return err
		}
		if err := encodeInitialValue(writer, t, entry.value, keys); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(t.bindings.Count())); err != nil {
		return err
	}
	for index := 0; index < t.bindings.Count(); index++ {
		binding, ok := t.bindings.At(index)
		if !ok {
			return errors.New("target/boot: malformed initial binding")
		}
		value, _, ok := t.InitialEntry(binding.root, binding.key)
		if !ok {
			return errors.New("target/boot: malformed initial binding")
		}
		kind, ok := t.InitialValueKind(value)
		if !ok {
			return errors.New("target/boot: malformed initial binding value")
		}
		if err := writer.Record(recordInitialBinding); err != nil {
			return err
		}
		if err := writer.String(binding.name); err != nil {
			return err
		}
		if err := writer.Uint(uint64(initialBindingClass(kind))); err != nil {
			return err
		}
		if err := encodeInitialValue(writer, t, value, keys); err != nil {
			return err
		}
		if err := writer.Uint(uint64(binding.root)); err != nil {
			return err
		}
		if err := encodeExactKey(writer, keys, binding.key); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(t.metatables.Count())); err != nil {
		return err
	}
	for index := 0; index < t.metatables.Count(); index++ {
		attachment, ok := t.metatables.At(index)
		if !ok {
			return errors.New("target/boot: malformed initial metatable attachment")
		}
		if err := writer.Record(recordInitialMetatableAttachment); err != nil {
			return err
		}
		if err := writer.Uint(uint64(attachment.base)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(attachment.metatable)); err != nil {
			return err
		}
	}
	return nil
}

func encodeExactKey(writer *framing.Writer, keys exactkey.Table, key vocabulary.ExactKey) error {
	value, ok := keys.Value(key)
	if !ok {
		return errors.New("target/boot: malformed exact key")
	}
	if err := writer.Uint(uint64(value.Kind)); err != nil {
		return err
	}
	switch value.Kind {
	case keyspace.LiteralBool:
		return writer.Bool(value.Bool)
	case keyspace.LiteralInteger:
		return writer.Uint(uint64(value.Integer))
	case keyspace.LiteralFloat:
		return writer.Uint(value.FloatBits)
	case keyspace.LiteralString:
		return writer.String(value.String)
	default:
		return errors.New("target/boot: malformed exact key kind")
	}
}

func encodeInitialValue(writer *framing.Writer, table *Table, value vocabulary.InitialValue, keys exactkey.Table) error {
	kind, ok := table.InitialValueKind(value)
	if !ok {
		return errors.New("target/boot: malformed initial value")
	}
	if err := writer.Record(recordInitialValue); err != nil {
		return err
	}
	if err := writer.Uint(uint64(kind)); err != nil {
		return err
	}
	switch kind {
	case vocabulary.InitialValueNil, vocabulary.InitialValueAbsent:
		return nil
	case vocabulary.InitialValueBoolean:
		item, ok := table.InitialValueBoolean(value)
		if !ok {
			return errors.New("target/boot: malformed initial boolean")
		}
		return writer.Bool(item)
	case vocabulary.InitialValueInteger:
		item, ok := table.InitialValueInteger(value)
		if !ok {
			return errors.New("target/boot: malformed initial integer")
		}
		return writer.Uint(uint64(item))
	case vocabulary.InitialValueFloat:
		item, ok := table.InitialValueFloatBits(value)
		if !ok {
			return errors.New("target/boot: malformed initial float")
		}
		return writer.Uint(item)
	case vocabulary.InitialValueString:
		item, ok := table.InitialValueString(value)
		if !ok {
			return errors.New("target/boot: malformed initial string")
		}
		return writer.String(item)
	case vocabulary.InitialValueRoot:
		item, ok := table.InitialValueRoot(value)
		if !ok {
			return errors.New("target/boot: malformed initial root value")
		}
		return writer.Uint(uint64(item))
	case vocabulary.InitialValueOperation:
		item, ok := table.InitialValueOperation(value)
		if !ok {
			return errors.New("target/boot: malformed initial operation value")
		}
		return writer.Uint(uint64(item))
	case vocabulary.InitialValueDeniedOperation:
		namespace, ok := table.InitialValueDeniedNamespace(value)
		if !ok {
			return errors.New("target/boot: malformed denied initial operation")
		}
		if err := writer.Uint(uint64(namespace)); err != nil {
			return err
		}
		owner := table.InitialValueDeniedOwnerCount(value)
		if err := writer.Count(uint64(owner)); err != nil {
			return err
		}
		for index := 0; index < owner; index++ {
			key, ok := table.initialValueDeniedOwnerKeyAt(value, index)
			if !ok {
				return errors.New("target/boot: malformed denied initial owner")
			}
			if err := encodeExactKey(writer, keys, key); err != nil {
				return err
			}
		}
		member := table.InitialValueDeniedMemberCount(value)
		if err := writer.Count(uint64(member)); err != nil {
			return err
		}
		for index := 0; index < member; index++ {
			key, ok := table.initialValueDeniedMemberKeyAt(value, index)
			if !ok {
				return errors.New("target/boot: malformed denied initial member")
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
