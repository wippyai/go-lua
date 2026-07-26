package engine

import (
	"encoding/base64"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// pathEqualityPrefix carries a proven equality between two exact paths. The
// branch that proved it owns the guard; the fact itself is a statement about
// the two symbols, not about the condition, so it stays available on every
// later coordinate the guard reaches. Either symbol's next epoch revokes it: a
// reassignment replaces the value the equality was about.
const pathEqualityPrefix = "path-equality/"

// pathEqualityFacts records the equalities a branch proves on each of its
// edges. The true edge of `p == q` proves the equality; so does the false edge
// of `p ~= q`, which is the early-return spelling of the same guard.
func pathEqualityFacts(operation equation.BoundEquation, partition equation.Partition) []equation.Fact {
	var facts []equation.Fact
	seen := make(map[string]bool)
	for _, operand := range operation.Operands {
		predicate, edge, polarity, ok := branchEvidencePredicateOnEdge(operand)
		if !ok || predicate.Path == "" || predicate.OtherPath == "" || predicate.Negated {
			continue
		}
		proves := false
		switch predicate.Kind {
		case "path-equal":
			proves = edge && polarity
		case "path-not":
			proves = !edge && !polarity
		}
		if !proves {
			continue
		}
		left, right := []byte("path/"+predicate.Path), []byte("path/"+predicate.OtherPath)
		if !congruenceOperandSealed(left, partition) || !congruenceOperandSealed(right, partition) {
			continue
		}
		key := pathEqualityKey(string(left), string(right)) + "/" + operation.Target.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		guardEdge := "true"
		if !edge {
			guardEdge = "false"
		}
		facts = append(facts, equation.Fact{
			Key: key, Value: []byte("proven"),
			Guards: []equation.Guard{{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/" + guardEdge)}},
		})
	}
	return facts
}

// pathEqualityKey orders the two terms so one relation has one key whichever
// operand order the source spelled.
func pathEqualityKey(left, right string) string {
	if right < left {
		left, right = right, left
	}
	return pathEqualityPrefix + base64.RawURLEncoding.EncodeToString([]byte(left)) + "/" +
		base64.RawURLEncoding.EncodeToString([]byte(right))
}

// provenPathEqualities reads back every equality currently visible at this
// coordinate, keyed by bare path (the vocabulary the correlation lane's cone
// already speaks). A relation established before either side's latest epoch
// belongs to an earlier value of that symbol and is dropped.
func provenPathEqualities(partition equation.Partition) map[string]map[string]bool {
	var equal map[string]map[string]bool
	for _, fact := range partition.Values() {
		rest, found := strings.CutPrefix(fact.Key, pathEqualityPrefix)
		if !found || string(fact.Value) != "proven" {
			continue
		}
		parts := strings.Split(rest, "/")
		if len(parts) < 3 {
			continue
		}
		left, leftErr := base64.RawURLEncoding.DecodeString(parts[0])
		right, rightErr := base64.RawURLEncoding.DecodeString(parts[1])
		if leftErr != nil || rightErr != nil {
			continue
		}
		if pathEqualityStale(left, fact.Key, partition) || pathEqualityStale(right, fact.Key, partition) {
			continue
		}
		leftPath, leftOK := strings.CutPrefix(string(left), "path/")
		rightPath, rightOK := strings.CutPrefix(string(right), "path/")
		if !leftOK || !rightOK {
			continue
		}
		if equal == nil {
			equal = make(map[string]map[string]bool)
		}
		if equal[leftPath] == nil {
			equal[leftPath] = make(map[string]bool)
		}
		if equal[rightPath] == nil {
			equal[rightPath] = make(map[string]bool)
		}
		equal[leftPath][rightPath] = true
		equal[rightPath][leftPath] = true
	}
	return equal
}

// pathEqualityStale reports that the term was replaced after the relation was
// proven. A member or index write cannot break reference equality, so only the
// symbol's own epoch revokes: exactly the event the cone epochs already model.
func pathEqualityStale(term []byte, proof string, partition equation.Partition) bool {
	epoch, versioned := currentEpoch(term, partition)
	return versioned && epoch > factOperation(proof)
}

// congruenceOperandSealed reports that this body installed no metatable on the
// term. A user `__eq` makes `p == q` a call rather than a reference identity,
// and a congruence transfer over such a comparison would relate two distinct
// objects. Only an install this body owns is observable here; a value that
// carries a metatable from elsewhere also carries no record facts to transfer.
func congruenceOperandSealed(term []byte, partition equation.Partition) bool {
	identity, found := tableIdentityForTerm(term, partition)
	return !found || !heapMetaAttached(identity, partition)
}

// persistentCongruenceConeFacts publishes the correlation cone for equalities
// this branch did not itself prove. The branch-local closure already owns the
// relations spelled in its own condition; this pass adds the ones proven
// earlier on the same path, which is what makes an equality outlive the
// condition that established it. Keys the selection already published are
// skipped, so one relation still yields one fact.
func persistentCongruenceConeFacts(operation equation.BoundEquation, partition equation.Partition, published []equation.Fact) ([]equation.Fact, error) {
	equal := provenPathEqualities(partition)
	if len(equal) == 0 {
		return nil, nil
	}
	nonNil := make(map[string]bool)
	for _, operand := range operation.Operands {
		predicate, trueEdge, ok := branchEvidencePredicate(operand)
		if !ok || !trueEdge || predicate.Negated || predicate.Kind != "not-nil" || predicate.Path == "" {
			continue
		}
		nonNil[predicate.Path] = true
	}
	if len(nonNil) == 0 {
		return nil, nil
	}
	equal = mergeEqualityClasses(equal, congruenceClasses(operation, partition))
	guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/true")}
	facts, err := correlationConeFacts(equal, nonNil, operation, partition, guard)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]bool, len(published))
	for _, fact := range published {
		existing[fact.Key] = true
	}
	kept := facts[:0]
	for _, fact := range facts {
		if existing[fact.Key] {
			continue
		}
		existing[fact.Key] = true
		kept = append(kept, fact)
	}
	return kept, nil
}

func mergeEqualityClasses(into, from map[string]map[string]bool) map[string]map[string]bool {
	if len(from) == 0 {
		return into
	}
	if into == nil {
		into = make(map[string]map[string]bool, len(from))
	}
	for left, peers := range from {
		if into[left] == nil {
			into[left] = make(map[string]bool, len(peers))
		}
		for right := range peers {
			into[left][right] = true
		}
	}
	return into
}

// congruenceTransfers projects each narrowing this branch derived onto the
// peers a proven equality relates it to. The peer must currently carry the same
// type as the narrowed term did before the narrowing: transferring into a
// different declared surface would claim a refinement the equality does not
// establish.
func congruenceTransfers(operation equation.BoundEquation, partition equation.Partition, narrowed map[string]typ.Type, order []string) []impliedNarrowing {
	equal := congruenceClasses(operation, partition)
	if len(equal) == 0 || len(order) == 0 {
		return nil
	}
	base := func(term string) (typ.Type, bool) {
		value, err := resolveCurrentValue([]byte(term), partition)
		if err != nil {
			return nil, false
		}
		return shapefact.DecodeTarget(value)
	}
	var out []impliedNarrowing
	for _, term := range append([]string(nil), order...) {
		source, sourceOK := strings.CutPrefix(term, "path/")
		if !sourceOK {
			continue
		}
		sourceType, sourceKnown := base(term)
		if !sourceKnown || sourceType == nil {
			continue
		}
		for _, peer := range correlatedEqualityTargets(source, equal) {
			target := "path/" + peer
			if _, already := narrowed[target]; already {
				continue
			}
			peerType, peerKnown := base(target)
			if !peerKnown || peerType == nil || !typ.TypeEquals(sourceType, peerType) {
				continue
			}
			out = append(out, impliedNarrowing{term: target, narrowed: narrowed[term]})
		}
	}
	return out
}

// congruenceClasses joins the equalities this branch proves on its own true
// edge with those already proven on the path reaching it.
func congruenceClasses(operation equation.BoundEquation, partition equation.Partition) map[string]map[string]bool {
	equal := provenPathEqualities(partition)
	for _, operand := range operation.Operands {
		predicate, trueEdge, ok := branchEvidencePredicate(operand)
		if !ok || !trueEdge || predicate.Negated || predicate.Kind != "path-equal" ||
			predicate.Path == "" || predicate.OtherPath == "" {
			continue
		}
		if !congruenceOperandSealed([]byte("path/"+predicate.Path), partition) ||
			!congruenceOperandSealed([]byte("path/"+predicate.OtherPath), partition) {
			continue
		}
		if equal == nil {
			equal = make(map[string]map[string]bool)
		}
		if equal[predicate.Path] == nil {
			equal[predicate.Path] = make(map[string]bool)
		}
		if equal[predicate.OtherPath] == nil {
			equal[predicate.OtherPath] = make(map[string]bool)
		}
		equal[predicate.Path][predicate.OtherPath] = true
		equal[predicate.OtherPath][predicate.Path] = true
	}
	return equal
}

// correlatedEqualityTargets lists the paths a source is equal to, including the
// members reached through an equal ancestor. It is correlatedPathTargets plus
// the source's own class, which a whole-value narrowing needs and a member
// narrowing does not.
func correlatedEqualityTargets(source string, equal map[string]map[string]bool) []string {
	targets := make(map[string]bool)
	for _, peer := range equalityCone(source, equal) {
		if peer != source {
			targets[peer] = true
		}
	}
	for _, peer := range correlatedPathTargets(source, equal) {
		if peer != source {
			targets[peer] = true
		}
	}
	out := make([]string, 0, len(targets))
	for target := range targets {
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}

// branchEvidencePredicateOnEdge decodes one closed branch predicate together
// with the edge it was published on and its polarity there. The narrower
// true-edge reader remains the default; a consumer that must also read the
// false edge of an inequality uses this one.
func branchEvidencePredicateOnEdge(operand equation.BoundOperand) (branchPredicateWire, bool, bool, bool) {
	encoded := operand.Value
	edge, polarity := true, true
	if strings.HasPrefix(string(encoded), branchEvidencePrefix) {
		rest := strings.TrimPrefix(string(encoded), branchEvidencePrefix)
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) != 3 {
			return branchPredicateWire{}, false, false, false
		}
		edge, polarity = parts[0] == "true", parts[1] == "true"
		encoded = []byte(parts[2])
	} else if operand.Role != "predicate" {
		return branchPredicateWire{}, false, false, false
	}
	predicate, ok := decodeBranchPredicateWire(encoded)
	if !ok {
		return branchPredicateWire{}, false, false, false
	}
	return predicate, edge, polarity, true
}
