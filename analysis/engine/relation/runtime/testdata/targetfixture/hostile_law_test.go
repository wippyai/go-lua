package targetfixture

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

func TestBuildAcceptsOneTypedSeed(t *testing.T) {
	fixture := newLawSpec(t)
	world := Build(t, fixture.spec)
	if !world.Mounted().Available() || !world.Base().Available() || !world.View().Available() {
		t.Fatal("target fixture baseline world")
	}
}

func TestBuildRejectsDuplicateFactory(t *testing.T) {
	fixture := newLawSpec(t)
	fixture.spec.Bindings = []binding.Factory{lawFactory{operation: fixture.spec.Initials[0].Operation}}
	rejects(t, func(probe Probe) { Build(probe, fixture.spec) })
}

func TestBuildRejectsDuplicateAuthority(t *testing.T) {
	fixture := newLawSpec(t)
	fixture.spec.Authorities = func(issuer binding.Issuer) (Registry, bool) {
		algebra, ok := relbindgen.NewAlgebra[uint64, lawUintLattice](fixture.codec, issuer, lawUintLattice{})
		if !ok {
			return Registry{}, false
		}
		return Registry{Algebras: []binding.ValueAlgebra{algebra, algebra}}, true
	}
	rejects(t, func(probe Probe) { Build(probe, fixture.spec) })
}

func TestBuildRejectsDuplicatePopulationScopeAndInitial(t *testing.T) {
	t.Run("population", func(t *testing.T) {
		fixture := newLawSpec(t)
		fixture.spec.Populations = append(fixture.spec.Populations, fixture.spec.Populations[0])
		rejects(t, func(probe Probe) { Build(probe, fixture.spec) })
	})
	t.Run("scope", func(t *testing.T) {
		fixture := newLawSpec(t)
		fixture.spec.Scopes = append(fixture.spec.Scopes, fixture.spec.Scopes[0])
		rejects(t, func(probe Probe) { Build(probe, fixture.spec) })
	})
	t.Run("initial", func(t *testing.T) {
		fixture := newLawSpec(t)
		fixture.spec.Initials = append(fixture.spec.Initials, fixture.spec.Initials[0])
		rejects(t, func(probe Probe) { Build(probe, fixture.spec) })
	})
}

func TestBuildRejectsInitialCellOutsideItsSignatureAuthority(t *testing.T) {
	fixture := newLawSpec(t)
	wrongKey := fixture.identity.Key(t, fixture.relation, "wrong")
	wrongDenominator, ok := model.NewDenominatorRef(fixture.relation, wrongKey)
	if !ok {
		t.Fatal("law wrong denominator")
	}
	initial := fixture.spec.Initials[0]
	initial.Cells = func(issuer binding.Issuer) ([]Cell, bool) {
		value, ok := fixture.codec.Encode(issuer, uint64(1))
		if !ok {
			return nil, false
		}
		cell, ok := Present(wrongDenominator, fixture.row, fixture.column, value)
		if !ok {
			return nil, false
		}
		return []Cell{cell}, true
	}
	fixture.spec.Initials = []Initial{initial}
	rejects(t, func(probe Probe) { Build(probe, fixture.spec) })
}

// The generic substrate is deliberately domain-blind. Family specimen
// packages live below it; this root package must never import a Placement (or
// any other domain) package to choose codecs, values, or inference.
func TestGenericPackageHasNoDomainImports(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("target fixture source path")
	}
	directory := filepath.Dir(source)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, entry.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, imported := range file.Imports {
			path := strings.Trim(imported.Path.Value, "\"")
			if strings.Contains(path, "/domain/") || strings.HasSuffix(path, "/domain") {
				t.Fatalf("generic targetfixture imports domain package %q in %s", path, entry.Name())
			}
		}
	}
}

type lawSpec struct {
	spec     Spec
	identity Identity
	relation model.RelationID
	column   model.ColumnID
	row      model.RowID
	codec    *relbindgen.Column[uint64]
}

func newLawSpec(t *testing.T) lawSpec {
	t.Helper()
	identity := NewIdentity(t, "analysis/engine/relation/runtime/testdata/targetfixture/law/v1")
	schema := identity.Schema(t, "law")
	typeID := identity.Type(t, "value")
	scope := identity.Scope(t, "law")
	relation := identity.Relation(t, "seed")
	column := identity.Column(t, relation, "value")
	key := identity.Key(t, relation, "value")
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("law denominator")
	}
	row := identity.Row(t, relation, "only")
	operation := identity.Operation(t, "seed")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("law cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.Refused)
	if !ok {
		t.Fatal("law outcomes")
	}
	semantic, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: identity.Owner(), Schema: schema},
		Outputs:     []signature.Output{{Relation: relation, Column: column, Type: typeID, Presence: signature.ProducePresent, Denominator: denominator}},
		Cardinality: cardinality,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("law signature")
	}
	capability, ok := model.NewAscendingCapability(typeID)
	if !ok {
		t.Fatal("law capability")
	}
	scopeAtom, ok := region.NewAtom(mustContent(t, identity, "scope-region"))
	if !ok {
		t.Fatal("law scope atom")
	}
	scopeRegion, ok := region.FromAtom(scopeAtom)
	if !ok {
		t.Fatal("law scope region")
	}
	store, ok := relbindgen.NewStore[uint64](mustContent(t, identity, "codec"), 1)
	if !ok {
		t.Fatal("law store")
	}
	codec, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatal("law codec")
	}
	initial := Initial{
		Operation: semantic,
		Scope:     scope,
		Cells: func(issuer binding.Issuer) ([]Cell, bool) {
			value, ok := codec.Encode(issuer, uint64(1))
			if !ok {
				return nil, false
			}
			cell, ok := Present(denominator, row, column, value)
			if !ok {
				return nil, false
			}
			return []Cell{cell}, true
		},
	}
	return lawSpec{
		spec: Spec{
			Identity: identity,
			Declaration: relcompile.Declaration{
				SchemaID:         schema,
				Relations:        []model.RelationSchema{model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)},
				Columns:          []model.ColumnSchema{model.DefineColumnSchema(column, typeID)},
				TypeCapabilities: []model.TypeCapability{capability},
				Keys:             []model.KeySchema{model.DefineKeySchema(key, []model.ColumnID{column})},
				Scopes:           []model.ScopeSchema{model.DefineScopeSchema(scope, nil, scopeRegion)},
				Signatures:       []signature.Signature{semantic},
			},
			Populations: []Population{{Denominator: denominator, Rows: []model.RowID{row}}},
			Scopes:      []Scope{{ID: scope, Region: "law"}},
			Initials:    []Initial{initial},
			Authorities: func(issuer binding.Issuer) (Registry, bool) {
				algebra, ok := relbindgen.NewAlgebra[uint64, lawUintLattice](codec, issuer, lawUintLattice{})
				if !ok {
					return Registry{}, false
				}
				return Registry{Algebras: []binding.ValueAlgebra{algebra}}, true
			},
			MountByte: 0xE1,
		},
		identity: identity,
		relation: relation,
		column:   column,
		row:      row,
		codec:    codec,
	}
}

type lawFactory struct{ operation signature.Signature }

func (value lawFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || operation.Digest() != value.operation.Digest() {
		return nil, false
	}
	return lawBinding{operation: value.operation}, true
}

type lawBinding struct{ operation signature.Signature }

func (value lawBinding) Signature() signature.Signature { return value.operation }
func (lawBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return nil, false
}

type lawUintLattice struct{}

func (lawUintLattice) Join(left, right uint64) (uint64, bool) { return left, left == right }
func (lawUintLattice) Widen(left, right uint64) (uint64, bool) {
	return left, left == right
}
func (lawUintLattice) LessOrEq(left, right uint64) bool { return left == right }

type rejectingProbe struct{}

func (rejectingProbe) Helper() {}
func (rejectingProbe) Fatal(arguments ...any) {
	panic(fmt.Sprint(arguments...))
}
func (rejectingProbe) Fatalf(format string, arguments ...any) {
	panic(fmt.Sprintf(format, arguments...))
}

func rejects(t *testing.T, call func(Probe)) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("target fixture unexpectedly accepted hostile input")
		}
	}()
	call(rejectingProbe{})
}

func mustContent(t *testing.T, identity Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.Content(label)
	if !ok {
		t.Fatalf("law content %q", label)
	}
	return value
}
