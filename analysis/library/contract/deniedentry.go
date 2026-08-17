package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// The denied-entry payload format. A denial is a member its owner declares and
// refuses to publish, and this format carries the two things such a refusal has
// to say: WHICH member is denied, as the address this package already owns the
// format of, and WHICH refusal it is.
//
// The second half is what makes the form usable by both classes that declare it.
// A library refuses a member it models: string.dump exists in the language and
// the target cannot load a binary chunk back, so the member is declared and
// withheld. An initial environment additionally boots without a member at all:
// package.path is not withheld, it is not there. Reading a withheld member and
// reading an absent one are different facts - one is a refusal a consumer reports
// as unsupported, the other is a value a consumer proves is missing - so a
// payload that could not tell them apart would make every consumer guess which it
// held.
const (
	deniedEntryDomain  = "analysis/library/contract/denied-entry"
	deniedEntryVersion = 1
)

// Denial is which refusal one denied member states. Both spellings are writable
// so that the distinction is a stated fact rather than an inference from silence.
type Denial uint8

const (
	DenialInvalid Denial = iota
	// DenialRefused is a member its owner declares and will not hand out. The
	// member exists in the model and reaching it is an unsupported operation.
	DenialRefused
	// DenialAbsent is a member the environment booted without. Nothing is
	// withheld: there is no value at the address at all.
	DenialAbsent
	denialLimit
)

func (denial Denial) Available() bool { return denial > DenialInvalid && denial < denialLimit }

// DeniedEntry is one denial: which refusal it is, and the member it refuses.
type DeniedEntry struct {
	Denial Denial
	// Entry is the address of the denied member. It restates the address the
	// member row is published at, which makes the payload self-describing: a
	// reader that holds a body and its declared format knows which member was
	// refused without the enclosing instance.
	Entry Path
}

// Available reports whether this denial names one member and states which
// refusal it is. The contract root is not a denial address: a contract that
// refused its own root would publish nothing at all, and every member reached
// through it is denied by the fact that the root is.
func (denied DeniedEntry) Available() bool {
	return denied.Denial.Available() && denied.Entry.Available() && denied.Entry.Len() != 0
}

// EncodeDeniedEntry writes one denial as a member payload body. The refused
// address is written in the export-path format this package already owns, so an
// address has one encoding wherever it appears.
func EncodeDeniedEntry(denied DeniedEntry) ([]byte, error) {
	if !denied.Available() {
		return nil, ErrMalformed
	}
	entry, err := EncodePath(denied.Entry)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, deniedEntryDomain, deniedEntryVersion); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(denied.Denial)); err != nil {
		return nil, err
	}
	if err := writer.Bytes(entry); err != nil {
		return nil, err
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecodeDeniedEntry reads one denied-entry payload body.
func DecodeDeniedEntry(data []byte) (DeniedEntry, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return DeniedEntry{}, ErrMalformed
	}
	if err := reader.Header(deniedEntryDomain, deniedEntryVersion); err != nil {
		return DeniedEntry{}, ErrMalformed
	}
	denial, err := reader.Uint()
	if err != nil || denial > uint64(^uint8(0)) {
		return DeniedEntry{}, ErrMalformed
	}
	body, err := reader.Bytes(maxBody)
	if err != nil {
		return DeniedEntry{}, ErrMalformed
	}
	entry, err := DecodePath(body)
	if err != nil {
		return DeniedEntry{}, ErrMalformed
	}
	if err := reader.Finish(); err != nil {
		return DeniedEntry{}, ErrMalformed
	}
	denied := DeniedEntry{Denial: Denial(denial), Entry: entry}
	if !denied.Available() {
		return DeniedEntry{}, ErrMalformed
	}
	return denied, nil
}
