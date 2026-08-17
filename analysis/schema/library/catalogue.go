package library

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// Catalogue is the neutral declaration input consumed by the library
// compiler. A domain adapter owns the conversion from its live type graph and
// manifest carrier into these rows; this package owns the catalogue laws and
// the initial-environment authoring that turn them into a Target spec.
//
// The rows deliberately carry only portable schema/typecontract declarations.
// No domain type graph, manifest object, or runtime handle crosses this
// boundary.
type Catalogue struct {
	Semantics  schematype.Semantics
	Vocabulary TypeVocabulary
	Functions  []CatalogueFunction
	Modules    []CatalogueModule
}

// TypeVocabulary is the closed set of domain-encoded declarations used by the
// Lua operation laws. Primitive declarations are normally supplied by the
// domain adapter as schema/typecontract primitive values; literal and marker
// declarations retain the domain's canonical encoded representation.
type TypeVocabulary struct {
	Any                   schematype.Type
	Integer               schematype.Type
	Nil                   schematype.Type
	String                schematype.Type
	LiteralTrue           schematype.Type
	LiteralFalse          schematype.Type
	RejectedYield         schematype.Type
	BuiltinTableTopMarker schematype.Type
}

func (v TypeVocabulary) validate() error {
	values := []struct {
		name  string
		value schematype.Type
	}{
		{"Any", v.Any},
		{"Integer", v.Integer},
		{"Nil", v.Nil},
		{"String", v.String},
		{"LiteralTrue", v.LiteralTrue},
		{"LiteralFalse", v.LiteralFalse},
		{"RejectedYield", v.RejectedYield},
		{"BuiltinTableTopMarker", v.BuiltinTableTopMarker},
	}
	for _, item := range values {
		if !item.value.Available() {
			return fmt.Errorf("library: missing type vocabulary %s", item.name)
		}
	}
	return nil
}

// BindingNamespace identifies a Lua-visible binding in a neutral declaration
// row. The compiler maps these two source forms to Target's sealed binding
// vocabulary; detached declarations are intentionally omitted from the
// operation table.
type BindingNamespace uint8

const (
	BindingNamespaceInvalid BindingNamespace = iota
	BindingNamespaceBuiltin
	BindingNamespaceModule
)

// CatalogueBinding is one Lua-visible spelling of a callable declaration.
type CatalogueBinding struct {
	Namespace BindingNamespace
	Owner     []string
	Member    []string
}

// CatalogueFunction is one canonical callable declaration and its neutral
// signature envelope. Effectful is the only effect fact needed by the closed
// target self-effect projection; the effect vocabulary remains domain-owned.
type CatalogueFunction struct {
	Path      string
	Bindings  []CatalogueBinding
	Signature CatalogueSignature
	Effectful bool
}

// CatalogueSignature is the normalized callable envelope presented to the
// neutral operation compiler. Domain adapters resolve type parameters,
// optional parameters, transparent optional results, and variadic unions
// before constructing this row.
type CatalogueSignature struct {
	Available    bool
	Input        []schematype.Type
	InputOpen    bool
	InputTail    schematype.Type
	Output       []schematype.Type
	OutputOpen   bool
	OutputTail   schematype.Type
	OutputSuffix []schematype.Type
	Never        bool
}

// CatalogueModule describes one mounted module root in the initial
// environment. Provider is the stable root identity; Path is the Lua-visible
// module owner used by callable bindings.
type CatalogueModule struct {
	Provider  string
	Path      string
	Immutable bool
}

// SealCatalogue compiles and seals one neutral declaration catalogue. The
// returned contract is the Program Target's immutable neutral projection;
// domain adapters remain responsible for producing the input rows.
func SealCatalogue(declarations Catalogue) (*target.Contract, error) {
	spec, err := CompileCatalogue(declarations)
	if err != nil {
		return nil, err
	}
	return target.Seal(&spec)
}
