package engine

import "github.com/wippyai/go-lua/analysis/identity"

// ruleUnit is the private synthetic operand used by engine laws whose Rule has
// no domain payload. Its digest still names the exact source entity: even a
// unit-valued judgment may not pair O with an independently chosen operand.
type ruleUnit struct{ content [32]byte }

func ruleUnitForSemantic(key identity.SemanticKey) ruleUnit { return ruleUnit{content: key.Digest()} }

func ruleUnitContent(value ruleUnit) (ruleUnit, [32]byte, bool) {
	return value, value.content, value.content != [32]byte{}
}

var unitOperandFamily = mustRuleOperandFamily()

func mustRuleOperandFamily() identity.SemanticKey {
	key, ok := identity.NewSemanticKey([32]byte{0x72, 0x75, 0x6c, 0x65, 0x2d, 0x75, 0x6e, 0x69, 0x74, 0x2d, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x6e, 0x64}, 1)
	if !ok {
		panic("engine unit operand family")
	}
	return key
}
