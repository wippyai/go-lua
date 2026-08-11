package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

func testSemanticKey(version uint64) SemanticKey {
	var id composition.ID
	// Keep fixture spelling canonical: the semantic digest is ordered by the
	// numeric fixture version, so tests asserting Composition's sorted output
	// do not accidentally encode an unrelated hash byte order.
	for index := 0; index < 8; index++ {
		id[index] = byte(version >> uint((7-index)*8))
	}
	key, ok := NewSemanticKey([32]byte(id), version)
	if !ok {
		panic("test semantic key")
	}
	return key
}

func testTrustedTheorem[V any](version uint64) RuleAdmission[V, ruleUnit] {
	return AdmitRuleByTrustedTheorem[V, ruleUnit](testSemanticKey(version))
}
