package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

const (
	instanceOperandEntityVersion uint64 = 1
	instanceOperandEntityDomain         = "analysis/engine/operand-entity"
)

// operandEntity is the opaque equation-owned content identity used by the
// runtime binder when it validates a materialized member. It is not a
// source-admission capability.
type operandEntity struct{ key composition.Key }

// operandEntityForContent derives the operand entity identity from an issuer
// content digest. The digest is a framed preimage under this domain rather
// than the identity itself, so an operand entity occupies its own key space
// and can never coincide with a Factor, Rule, or Query identity that happens
// to carry the same bytes.
func operandEntityForContent(digest [32]byte) (composition.Key, bool) {
	if digest == [32]byte{} {
		return composition.Key{}, false
	}
	var writer canonical.DigestWriter
	if writer.Reset(instanceOperandEntityDomain, instanceOperandEntityVersion) != nil || writer.Bytes(digest[:]) != nil || writer.Finish() != nil {
		return composition.Key{}, false
	}
	entity := composition.Key{ID: composition.ID(writer.Sum()), Version: instanceOperandEntityVersion}
	return entity, entity.Available()
}
