package transformer

import "fmt"

// formalOutcomeOccurrenceSite is the unique lexical publication site of one
// frozen outcome tuple. Production relation closure gives every Outcome node
// its own tuple reference; retaining the site here prevents a hash-consed
// value/guard term from being interpreted in a different loop lifetime.
type formalOutcomeOccurrenceSite struct {
	root  relationRootRef
	scope loopMuTerm
}

func uniqueFormalOutcomeOccurrenceSite(code *relationCode, scopes []loopMuTerm, outcome boundaryOutcomeRef) (formalOutcomeOccurrenceSite, error) {
	if code == nil || outcome == 0 || int(outcome) >= len(code.outcomes) || len(scopes) != len(code.nodes) {
		return formalOutcomeOccurrenceSite{}, fmt.Errorf("transformer: formal Outcome occurrence is unowned")
	}
	var site formalOutcomeOccurrenceSite
	for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
		node := code.nodes[root]
		if node.kind != relationNodeOutcome || node.outcome != outcome {
			continue
		}
		if site.root != 0 {
			return formalOutcomeOccurrenceSite{}, fmt.Errorf("transformer: formal Outcome %d is shared by nodes %d and %d", outcome, site.root, root)
		}
		site = formalOutcomeOccurrenceSite{root: root, scope: scopes[root]}
	}
	if site.root == 0 {
		return formalOutcomeOccurrenceSite{}, fmt.Errorf("transformer: formal Outcome %d has no lexical node", outcome)
	}
	return site, nil
}
