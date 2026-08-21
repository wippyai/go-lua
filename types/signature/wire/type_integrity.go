package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// A type payload states one type and nothing else, so absence inside it is
// never legitimate: every node a producer wrote is a node it meant, and a node
// that arrives empty or a list that arrives short is a payload that lost part
// of what was sent.
//
// Nothing in a bare tree of optional members records that loss. A union that
// shed a member is a well-formed narrower union; a function that shed a return
// is a well-formed function with one fewer result; a type parameter that shed
// its constraint is exactly the unconstrained declaration; a single flipped bit
// in an atom's kind is a different atom. In every one of those the decoder
// answers with a type the producer never stated, and the answer is a strictly
// narrower or strictly wider judgment downstream.
//
// The witness is therefore stated over the whole encoded tree rather than at
// the node positions someone thought to guard. A root node carries the digest
// of its own encoding, so any difference between what was written and what
// arrived - an emptied member, a dropped list entry, a rewritten kind, a
// flipped literal, a duplicated key, an unreadable member this reader would
// have discarded - fails to match and the payload is refused instead of
// decoded into a narrower type.
//
// This is an integrity witness and not an authentication. A producer that can
// write the payload can write the digest with it, so the witness answers "is
// this the payload that was written" and never "was the producer entitled to
// write it". Refusing a payload that no encoder could have produced is the
// separate obligation of structural validation, which the decoder discharges
// on its own.

// TypeWireIntegrityScheme names how a type payload's witness is computed. The
// scheme is written into the witness, so a reader that holds only the bytes
// refuses one it cannot verify instead of reading the payload as unwitnessed.
// Changing how the tree is digested is a new scheme, never a redefinition of
// this one.
const TypeWireIntegrityScheme = "sha256/1"

// typeWireIntegrity states the witness for one node's encoding. The digest is
// taken over this package's own JSON projection of the node, which is the same
// projection the payload travels in, so a member added to the wire is covered
// by the witness the moment it is emitted and there is no second statement of
// what a node contains to keep in step.
func typeWireIntegrity(w *TypeWire) (string, error) {
	witnessed := *w
	witnessed.Integrity = ""
	encoded, err := json.Marshal(&witnessed)
	if err != nil {
		return "", fmt.Errorf("signature/wire: digest type payload: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return TypeWireIntegrityScheme + ":" + hex.EncodeToString(sum[:]), nil
}

// sealTypeWire states the witness on one encoded root. Sealing is the last step
// of encoding, so the digest covers the finished tree and the shallow copy it
// digests shares that tree rather than duplicating it.
func sealTypeWire(w *TypeWire) error {
	if w == nil {
		return nil
	}
	witness, err := typeWireIntegrity(w)
	if err != nil {
		return err
	}
	w.Integrity = witness
	return nil
}

// verifyTypeWireIntegrity reads the witness a payload carries and refuses the
// payload unless it is the one that was written.
func verifyTypeWireIntegrity(w *TypeWire) error {
	if w == nil {
		return fmt.Errorf("signature/wire: type payload missing")
	}
	if w.Integrity == "" {
		return fmt.Errorf(
			"signature/wire: type payload carries no integrity witness, so nothing in it states what was sent")
	}
	scheme, _, stated := strings.Cut(w.Integrity, ":")
	if !stated || scheme != TypeWireIntegrityScheme {
		return fmt.Errorf("signature/wire: type payload witness scheme %q, want %q", scheme, TypeWireIntegrityScheme)
	}
	witness, err := typeWireIntegrity(w)
	if err != nil {
		return err
	}
	if witness != w.Integrity {
		return fmt.Errorf(
			"signature/wire: type payload does not match its integrity witness, so part of what was written did not arrive")
	}
	return nil
}
