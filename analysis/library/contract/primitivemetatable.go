package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// The primitive-metatable payload format. It carries the one CROSS-CONTRACT
// reference this surface has: the environment states that a base primitive of the
// language reaches its members through a metatable that another contract - the
// library whose members those are - publishes.
//
// The reference is written as (mount selector, metatable-key path) and
// deliberately NOT as (contract content identity, path). A content identity
// changes with every edit of the contract it identifies, so an environment that
// pinned one would name a revision rather than a library: adding one export to
// the string contract would leave the environment referring to bytes nobody
// publishes any more, and the only repair would be to re-author the environment
// for a change that did not concern it. The mount selector is what a project
// already resolves a contract by, it is stable across every edit of the contract
// it selects, and it is the one legitimate use of a name in this surface - a name
// may select a mount, it may never address a member. The member half of the
// reference stays an export path, so what is named inside the selected contract
// is still a value.
//
// A base primitive has no exported value to hang the attachment on - the string
// metatable is reachable from no slot of the environment - so the attachments are
// published as one member at the contract root, which is the value they are a
// fact about. Their order is the base order, because order is content.
const (
	primitiveMetatableDomain  = "analysis/library/contract/primitive-metatable"
	primitiveMetatableVersion = 1
	// maxAttachments bounds the decoder's reservation. The closed primitive
	// domain is small; the bound only keeps a hostile count finite.
	maxAttachments = 1 << 8
)

// PrimitiveAttachment is one primitive's metatable: which primitive it attaches
// to, the contract that owns the metatable, the metatable-key address inside that
// contract, and the mutability the attached metatable is published with.
type PrimitiveAttachment struct {
	// Base is the primitive the metatable attaches to, drawn from the same
	// closed literal domain an exported constant is drawn from: it is the
	// language's own value domain, and a metatable attaches to a family of
	// values rather than to a Go type.
	Base ConstantKind
	// Contract is the mount selector of the contract that owns the metatable.
	Contract string
	// Path is the metatable-key address inside that contract. Its last step is a
	// metatable step: an ordinary export path reaches a value published BY a
	// metatable, and naming that value would attach nothing.
	Path Path
	// Mutability is the policy the attached metatable is published under.
	Mutability Mutability
}

func (attachment PrimitiveAttachment) Available() bool {
	if !attachment.Base.Available() || attachment.Contract == "" || !attachment.Mutability.Available() {
		return false
	}
	if !attachment.Path.Available() || attachment.Path.Len() == 0 {
		return false
	}
	last, ok := attachment.Path.At(attachment.Path.Len() - 1)
	return ok && last.Kind == StepMetatable
}

// EncodePrimitiveMetatables writes the environment's primitive metatable
// attachments as one member payload body. An empty set is not written: an
// environment that attaches no metatable publishes no attachment member, rather
// than a member that claims to carry a set and carries none.
func EncodePrimitiveMetatables(attachments []PrimitiveAttachment) ([]byte, error) {
	if len(attachments) == 0 {
		return nil, ErrMalformed
	}
	for index, attachment := range attachments {
		if !attachment.Available() {
			return nil, ErrMalformed
		}
		// One primitive has one metatable, and the set is written in base order
		// so that two spellings of one environment cannot exist.
		if index != 0 && attachments[index-1].Base >= attachment.Base {
			return nil, ErrMalformed
		}
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, primitiveMetatableDomain, primitiveMetatableVersion); err != nil {
		return nil, err
	}
	if err := writer.Count(uint64(len(attachments))); err != nil {
		return nil, err
	}
	for _, attachment := range attachments {
		path, err := EncodePath(attachment.Path)
		if err != nil {
			return nil, err
		}
		if err := writer.Record(recordAttachment); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(attachment.Base)); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(attachment.Mutability)); err != nil {
			return nil, err
		}
		if err := writer.String(attachment.Contract); err != nil {
			return nil, err
		}
		if err := writer.Bytes(path); err != nil {
			return nil, err
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecodePrimitiveMetatables reads one primitive-metatable payload body.
func DecodePrimitiveMetatables(data []byte) ([]PrimitiveAttachment, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return nil, ErrMalformed
	}
	if err := reader.Header(primitiveMetatableDomain, primitiveMetatableVersion); err != nil {
		return nil, ErrMalformed
	}
	count, err := reader.Count()
	if err != nil || count == 0 || count > maxAttachments {
		return nil, ErrMalformed
	}
	attachments := make([]PrimitiveAttachment, 0, count)
	for index := uint64(0); index < count; index++ {
		if record, err := reader.Record(); err != nil || record != recordAttachment {
			return nil, ErrMalformed
		}
		base, err := reader.Uint()
		if err != nil || base > uint64(^uint8(0)) {
			return nil, ErrMalformed
		}
		mutability, err := reader.Uint()
		if err != nil || mutability > uint64(^uint8(0)) {
			return nil, ErrMalformed
		}
		selector, err := reader.String(maxKey)
		if err != nil {
			return nil, ErrMalformed
		}
		body, err := reader.Bytes(maxBody)
		if err != nil {
			return nil, ErrMalformed
		}
		path, err := DecodePath(body)
		if err != nil {
			return nil, ErrMalformed
		}
		attachment := PrimitiveAttachment{
			Base:       ConstantKind(base),
			Contract:   selector,
			Path:       path,
			Mutability: Mutability(mutability),
		}
		if !attachment.Available() {
			return nil, ErrMalformed
		}
		if index != 0 && attachments[index-1].Base >= attachment.Base {
			return nil, ErrMalformed
		}
		attachments = append(attachments, attachment)
	}
	if err := reader.Finish(); err != nil {
		return nil, ErrMalformed
	}
	return attachments, nil
}
