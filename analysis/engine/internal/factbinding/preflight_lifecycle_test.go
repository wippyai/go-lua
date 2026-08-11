package factbinding

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestPreflightClaimsBindingExactlyOnce(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := bindTest(capabilityInput(nil), manager)
	if !ok {
		t.Fatal("binding")
	}
	const contenders = 32
	start := make(chan struct{})
	results := make(chan carrier.SlotOperation, contenders)
	var workers sync.WaitGroup
	for contender := 0; contender < contenders; contender++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			operation, prepared := binding.Preflight()
			if prepared {
				results <- operation
			}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	wins := 0
	for operation := range results {
		if operation == nil {
			t.Fatal("preflight returned nil operation")
		}
		wins++
	}
	if wins != 1 || !binding.InitialRootReady() {
		t.Fatalf("preflight claim: winners=%d initial-ready=%t", wins, binding.InitialRootReady())
	}
	if operation, prepared := binding.Preflight(); prepared || operation != nil {
		t.Fatal("claimed binding was reusable")
	}
}
