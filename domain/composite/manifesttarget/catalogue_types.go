package manifesttarget

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/manifest"
)

// qualifiedTypes projects the catalogue's already-scoped named declarations
// into the neutral schema/typecontract envelope. The manifest catalogue owns
// exact names and the domain adapter owns canonical bytes; this composition
// step does not infer names from operations or reconstruct them downstream.
func qualifiedTypes(declarations *manifest.Catalogue) ([]vocabulary.QualifiedTypeSpec, error) {
	items := declarations.TypeDeclarations()
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]vocabulary.QualifiedTypeSpec, len(items))
	for index, item := range items {
		if item.Type == nil {
			return nil, fmt.Errorf("target catalogue: named type %q has no declaration", item.Name)
		}
		encoded, err := portableType(item.Type)
		if err != nil {
			return nil, fmt.Errorf("target catalogue: named type %q: %w", item.Name, err)
		}
		out[index] = vocabulary.QualifiedTypeSpec{Name: item.Name, Declaration: encoded}
	}
	return out, nil
}

func portableType(value typ.Type) (typecontract.Type, error) {
	encoded, err := domaincontract.EncodeStorage(context.Background(), value, nil)
	if err != nil {
		return typecontract.Type{}, err
	}
	return encoded, nil
}
