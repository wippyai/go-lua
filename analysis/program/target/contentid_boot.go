package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

func encodeBoot(w *framing.Writer, c *Contract) error {
	roots := c.InitialRootCount()
	if err := w.Count(uint64(roots)); err != nil {
		return err
	}
	for index := 0; index < roots; index++ {
		root, ok := c.InitialRootAt(index)
		if !ok {
			return errors.New("target: malformed initial root")
		}
		identity, ok := c.InitialRootIdentity(root)
		if !ok {
			return errors.New("target: malformed initial root identity")
		}
		shape, ok := c.InitialRootBootShape(root)
		if !ok {
			return errors.New("target: malformed initial root shape")
		}
		shapeRoot, ok := c.bootShapeRoot(shape)
		if !ok || shapeRoot != root {
			return errors.New("target: malformed boot shape root")
		}
		aggregate, ok := c.BootShapeAggregate(shape)
		if !ok {
			return errors.New("target: malformed boot shape aggregate")
		}
		immutable, ok := c.BootShapeImmutable(shape)
		if !ok {
			return errors.New("target: malformed boot shape immutable header")
		}
		value, ok := c.BootShapeValue(shape)
		if !ok {
			return errors.New("target: malformed boot shape value")
		}
		if err := w.Record(recordInitialRoot); err != nil {
			return err
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := w.String(identity); err != nil {
			return err
		}
		if err := w.Record(recordBootShape); err != nil {
			return err
		}
		if err := w.Uint(uint64(shape)); err != nil {
			return err
		}
		if err := w.Uint(uint64(aggregate)); err != nil {
			return err
		}
		if err := w.Bool(immutable); err != nil {
			return err
		}
		if err := encodeInitialValue(w, c, value); err != nil {
			return err
		}
	}
	entries := c.InitialEntryCount()
	if err := w.Count(uint64(entries)); err != nil {
		return err
	}
	for index := 0; index < entries; index++ {
		root, key, value, mutability, ok := c.InitialEntryAt(index)
		if !ok {
			return errors.New("target: malformed initial entry")
		}
		if err := w.Record(recordInitialEntry); err != nil {
			return err
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
		if err := w.Uint(uint64(mutability)); err != nil {
			return err
		}
		if err := encodeInitialValue(w, c, value); err != nil {
			return err
		}
	}
	bindings := c.InitialBindingCount()
	if err := w.Count(uint64(bindings)); err != nil {
		return err
	}
	for index := 0; index < bindings; index++ {
		name, class, value, root, key, ok := c.InitialBindingAt(index)
		if !ok {
			return errors.New("target: malformed initial binding")
		}
		if err := w.Record(recordInitialBinding); err != nil {
			return err
		}
		if err := w.String(name); err != nil {
			return err
		}
		if err := w.Uint(uint64(class)); err != nil {
			return err
		}
		if err := encodeInitialValue(w, c, value); err != nil {
			return err
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	}
	attachments := c.InitialMetatableAttachmentCount()
	if err := w.Count(uint64(attachments)); err != nil {
		return err
	}
	for index := 0; index < attachments; index++ {
		base, metatable, ok := c.InitialMetatableAttachmentAt(index)
		if !ok {
			return errors.New("target: malformed initial metatable attachment")
		}
		if err := w.Record(recordInitialMetatableAttachment); err != nil {
			return err
		}
		if err := w.Uint(uint64(base)); err != nil {
			return err
		}
		if err := w.Uint(uint64(metatable)); err != nil {
			return err
		}
	}
	return nil
}

func encodeExactKey(w *framing.Writer, c *Contract, key vocabulary.ExactKey) error {
	value, ok := c.ExactKeyValue(key)
	if !ok {
		return errors.New("target: malformed exact key")
	}
	if err := w.Uint(uint64(value.Kind)); err != nil {
		return err
	}
	switch value.Kind {
	case keyspace.LiteralBool:
		return w.Bool(value.Bool)
	case keyspace.LiteralInteger:
		// The canonical writer carries unsigned scalars only. Reinterpreting the
		// two's-complement bit pattern is a total, injective encoding of int64;
		// the preceding literal kind keeps it separate from all other payloads.
		return w.Uint(uint64(value.Integer))
	case keyspace.LiteralFloat:
		return w.Uint(value.FloatBits)
	case keyspace.LiteralString:
		return w.String(value.String)
	default:
		return errors.New("target: malformed exact key kind")
	}
}

func encodeInitialValue(w *framing.Writer, c *Contract, value vocabulary.InitialValue) error {
	kind, ok := c.InitialValueKind(value)
	if !ok {
		return errors.New("target: malformed initial value")
	}
	if err := w.Record(recordInitialValue); err != nil {
		return err
	}
	if err := w.Uint(uint64(kind)); err != nil {
		return err
	}
	switch kind {
	case vocabulary.InitialValueNil, vocabulary.InitialValueAbsent:
		return nil
	case vocabulary.InitialValueBoolean:
		item, ok := c.InitialValueBoolean(value)
		if !ok {
			return errors.New("target: malformed initial boolean")
		}
		return w.Bool(item)
	case vocabulary.InitialValueInteger:
		item, ok := c.InitialValueInteger(value)
		if !ok {
			return errors.New("target: malformed initial integer")
		}
		return w.Uint(uint64(item))
	case vocabulary.InitialValueFloat:
		item, ok := c.InitialValueFloatBits(value)
		if !ok {
			return errors.New("target: malformed initial float")
		}
		return w.Uint(item)
	case vocabulary.InitialValueString:
		item, ok := c.InitialValueString(value)
		if !ok {
			return errors.New("target: malformed initial string")
		}
		return w.String(item)
	case vocabulary.InitialValueRoot:
		item, ok := c.InitialValueRoot(value)
		if !ok {
			return errors.New("target: malformed initial root value")
		}
		return w.Uint(uint64(item))
	case vocabulary.InitialValueOperation:
		item, ok := c.InitialValueOperation(value)
		if !ok {
			return errors.New("target: malformed initial operation value")
		}
		return w.Uint(uint64(item))
	case vocabulary.InitialValueDeniedOperation:
		namespace, ok := c.InitialValueDeniedNamespace(value)
		if !ok {
			return errors.New("target: malformed denied initial operation")
		}
		if err := w.Uint(uint64(namespace)); err != nil {
			return err
		}
		owner := c.InitialValueDeniedOwnerCount(value)
		if err := w.Count(uint64(owner)); err != nil {
			return err
		}
		for index := 0; index < owner; index++ {
			part, ok := c.initialValueDeniedOwnerKeyAt(value, index)
			if !ok {
				return errors.New("target: malformed denied initial owner")
			}
			if err := encodeExactKey(w, c, part); err != nil {
				return err
			}
		}
		member := c.InitialValueDeniedMemberCount(value)
		if err := w.Count(uint64(member)); err != nil {
			return err
		}
		for index := 0; index < member; index++ {
			part, ok := c.initialValueDeniedMemberKeyAt(value, index)
			if !ok {
				return errors.New("target: malformed denied initial member")
			}
			if err := encodeExactKey(w, c, part); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("target: invalid initial value kind")
	}
}
