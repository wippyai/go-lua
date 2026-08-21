package contract

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// encodeFormalEffectRow writes the neutral operation ownership declaration.
// It is intentionally independent of encodeEffect: invocation effects carry
// target ABI substitutions, while formal effects carry only their own closed
// ownership vocabulary.
func (c *Contract) encodeFormalEffectRow(w *framing.Writer, op vocabulary.Operation) error {
	tail, ok := c.Operations.FormalEffectTail(op)
	if !ok || (tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen) {
		return errors.New("target: malformed formal effect tail")
	}
	if err := w.Uint(uint64(tail)); err != nil {
		return err
	}
	count := c.Operations.FormalEffectCount(op)
	if err := w.Count(uint64(count)); err != nil {
		return err
	}
	for index := 0; index < count; index++ {
		effect, found := c.Operations.FormalEffectAt(op, index)
		if !found {
			return errors.New("target: malformed formal effect")
		}
		if err := encodeFormalEffect(w, effect); err != nil {
			return err
		}
	}
	return nil
}

func encodeFormalEffect(w *framing.Writer, effect vocabulary.FormalEffectSpec) error {
	if err := w.Uint(uint64(effect.Kind)); err != nil {
		return err
	}
	signed := func(value int32) error { return w.Uint(uint64(uint32(value))) }
	switch effect.Kind {
	case vocabulary.FormalEffectBorrow, vocabulary.FormalEffectRetain,
		vocabulary.FormalEffectSendParam, vocabulary.FormalEffectExport,
		vocabulary.FormalEffectOpaque, vocabulary.FormalEffectFreeze:
		if effect.Param < -1 || effect.Into != 0 || effect.HasInto || effect.FromParam != 0 {
			return errors.New("target: malformed formal effect operands")
		}
		return signed(effect.Param)
	case vocabulary.FormalEffectStore:
		if effect.Param < -1 || effect.FromParam != 0 ||
			(!effect.HasInto && effect.Into != -1) || (effect.HasInto && effect.Into < 0) {
			return errors.New("target: malformed formal Store operands")
		}
		if err := signed(effect.Param); err != nil {
			return err
		}
		if err := w.Bool(effect.HasInto); err != nil {
			return err
		}
		if effect.HasInto {
			return signed(effect.Into)
		}
		return nil
	case vocabulary.FormalEffectBorrowAll:
		if effect.Param != 0 || effect.Into != 0 || effect.HasInto || effect.FromParam != 0 {
			return errors.New("target: malformed formal BorrowAll operands")
		}
		return nil
	case vocabulary.FormalEffectSendSuffix:
		if effect.FromParam < 0 || effect.Param != 0 || effect.Into != 0 || effect.HasInto {
			return errors.New("target: malformed formal SendSuffix operands")
		}
		return signed(effect.FromParam)
	default:
		return errors.New("target: malformed formal effect kind")
	}
}
