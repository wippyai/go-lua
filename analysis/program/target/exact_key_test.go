package target

import "testing"

func TestExactKeyPoolIsCanonicalAndContractLocal(t *testing.T) {
	contract := mustSeal(t, completeBootSpec("Lua", InitialMutable))
	if contract.ExactKeyCount() == 0 {
		t.Fatal("boot contract has no exact keys")
	}
	for index := 0; index < contract.ExactKeyCount(); index++ {
		key, ok := contract.ExactKeyAt(index)
		if !ok || key == 0 {
			t.Fatalf("ExactKeyAt(%d) = %d/%v", index, key, ok)
		}
		if _, ok := contract.ExactKeyValue(key); !ok {
			t.Fatalf("ExactKeyValue(%d) unavailable", key)
		}
	}
	if _, ok := contract.ExactKeyAt(contract.ExactKeyCount()); ok {
		t.Fatal("out-of-range exact key resolved")
	}
}
