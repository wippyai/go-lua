package parsersource

import "testing"

func TestUseDerivationPublishesTypedOriginsForConstructedFields(t *testing.T) {
	analysis, err := DiscoverProducts(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var sourced bool
	for _, use := range analysis.Uses {
		if use.Form == "" || use.Field == "" || len(use.Origins) == 0 {
			t.Fatalf("incomplete action use %#v", use)
		}
		if len(use.Symbols) != 0 || len(use.Sources) != 0 {
			sourced = true
		}
	}
	if !sourced {
		t.Fatal("use derivation emitted no operand or construction provenance")
	}
}
