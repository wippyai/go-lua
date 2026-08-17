package engine

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

// ruleUnit is the private synthetic operand used by engine laws whose Rule has
// no domain payload. Its digest still names the exact source entity: even a
// unit-valued judgment may not pair O with an independently chosen operand.
type ruleUnit struct{ content [32]byte }

func ruleUnitForSemantic(key identity.SemanticKey) ruleUnit { return ruleUnit{content: key.Digest()} }

func ruleUnitContent(value ruleUnit) (ruleUnit, [32]byte, bool) {
	return value, value.content, value.content != [32]byte{}
}

const (
	unitOperandFamilyDomain         = "analysis/engine/rule-unit-operand-family"
	unitOperandFamilyVersion uint64 = 1
)

// unitOperandFamily is the derived family identity of ruleUnit. It is minted
// from a framed domain preimage rather than spelled out as content bytes, so
// it shares no digest space with any hand-written key. A rejected derivation
// yields an unavailable key, which every admission fence refuses.
var unitOperandFamily = ruleUnitOperandFamily()

func ruleUnitOperandFamily() identity.SemanticKey {
	var writer canonical.DigestWriter
	if writer.Reset(unitOperandFamilyDomain, unitOperandFamilyVersion) != nil || writer.Finish() != nil {
		return identity.SemanticKey{}
	}
	key, ok := identity.NewSemanticKey(writer.Sum(), unitOperandFamilyVersion)
	if !ok {
		return identity.SemanticKey{}
	}
	return key
}
