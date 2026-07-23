package transformer

import "testing"

func TestRelationBoundaryPrefixPreservesExternalCallOperandRoles(t *testing.T) {
	step := boundaryStep{
		kind: boundaryStepExternalCall,
		operands: callOutcomeOperandTerms{
			callee: 11, hasCallee: true,
			receiver: 12, hasReceiver: true,
			arguments: []ValueTerm{13, 14},
		},
	}
	prefix, err := relationBoundaryPrefixStep(&relationCode{}, step)
	if err != nil {
		t.Fatal(err)
	}
	if !prefix.operands.hasCallee || prefix.operands.callee != 11 ||
		!prefix.operands.hasReceiver || prefix.operands.receiver != 12 ||
		len(prefix.operands.arguments) != 2 || prefix.operands.arguments[0] != 13 || prefix.operands.arguments[1] != 14 {
		t.Fatalf("prefix operands = %#v, want exact detached external-call tuple", prefix.operands)
	}
	step.operands.arguments[0] = 99
	if prefix.operands.arguments[0] != 13 {
		t.Fatal("prefix operands alias mutable relation syntax")
	}
}
