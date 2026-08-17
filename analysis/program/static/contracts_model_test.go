package static

import "testing"

func TestContractsModelZeroViewsFailClosed(t *testing.T) {
	var contracts Contracts
	if contracts.Functions().Count() != 0 || contracts.Calls().Count() != 0 {
		t.Fatal("zero Contracts model exposed rows")
	}
	var functions Functions
	if functions.Count() != 0 {
		t.Fatal("zero Functions model exposed a term")
	}
	if got, ok := functions.At(0); ok || got != 0 {
		t.Fatalf("zero Functions At(0) = %v/%v", got, ok)
	}
	var calls Calls
	if calls.Count() != 0 {
		t.Fatal("zero Calls model exposed a term")
	}
	if got, ok := calls.At(0); ok || got != 0 {
		t.Fatalf("zero Calls At(0) = %v/%v", got, ok)
	}
}
