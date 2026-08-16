package grammar

import "testing"

func TestZZSealedDigestProbe(t *testing.T) {
	receipt, ok := Global()
	if !ok {
		t.Fatalf("grammar did not seal")
	}
	t.Logf("DIGEST=%x rules=%d axes=%d", receipt.Digest(), RuleCount(), AxisCount())
}
