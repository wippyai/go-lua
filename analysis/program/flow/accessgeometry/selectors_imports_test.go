package accessgeometry

import "testing"

func TestImportAliasRequiresExactCrossProof(t *testing.T) {
	for _, test := range []struct {
		name string
		tail bool
	}{
		{name: "fixed", tail: false},
		{name: "open first result", tail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			proveImportAlias(t, test.tail)
		})
	}
}
