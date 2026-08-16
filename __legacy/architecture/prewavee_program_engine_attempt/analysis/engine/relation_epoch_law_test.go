package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
)

// Relation and RelationRef share one frame-generation stamp.  Once its
// uint64 identity is exhausted, opening another resolver must fail rather
// than wrapping to a retained capability's old epoch.
func TestRelationEpochExhaustionCannotReviveRetainedCapabilities(t *testing.T) {
	transaction := &transaction{relationEpoch: ^uint64(0)}
	frame := &transaction.relationFrames[0]
	*frame = relationFrame{epoch: 1}
	retained := Relation{frame: frame, epoch: 1}
	retainedRef := RelationRef{owner: frame, epoch: 1}
	target := &ruleIdentity{anchor: ruleAnchor{inputArity: 1}}

	if next := transaction.nextRelationEpoch(); next != 0 {
		t.Fatalf("exhausted Relation epoch = %d, want zero failure capability", next)
	}
	if relation, ok := transaction.openRelation(activationSource{}, target, link.Candidate{}); ok || relation.frame != nil || relation.epoch != 0 {
		t.Fatal("exhausted Relation epoch opened a resolver")
	}
	if retained.valid() {
		t.Fatal("retained Relation revived after epoch exhaustion")
	}
	if retainedRef.owner != frame || retainedRef.epoch != 1 || frame.live || frame.epoch != 1 {
		t.Fatal("exhaustion mutated retained Relation/RelationRef authority")
	}
}
