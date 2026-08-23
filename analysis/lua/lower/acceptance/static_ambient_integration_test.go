package acceptance_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	"github.com/wippyai/go-lua/domain/type/ambient"
)

// ambientReferences returns every authored type reference of one lowered
// source under its source spelling.
func ambientReferences(t *testing.T, p *program.Program) map[string]staticrefs.Resolution {
	t.Helper()
	view := p.Static()
	types := view.StaticTypes()
	references := view.References()
	out := make(map[string]staticrefs.Resolution, types.Count())
	for index := 0; index < types.Count(); index++ {
		ref, ok := types.At(index)
		if !ok {
			t.Fatalf("static type %d is absent", index)
		}
		term := ref.Term()
		resolution, _, _, held := references.Get(term)
		if !held {
			continue
		}
		count, countOK := references.SourceCount(term)
		if !countOK || count == 0 {
			t.Fatalf("type reference %v carries no source spelling", term)
		}
		spelling := ""
		for position := 0; position < count; position++ {
			key, keyOK := references.SourceAt(term, position)
			literal, literalOK := p.Source().Keys().Exact(key)
			if !keyOK || !literalOK || literal.Kind != keyspace.LiteralString {
				t.Fatalf("type reference %v component %d is not a literal name", term, position)
			}
			if position != 0 {
				spelling += "."
			}
			spelling += literal.String
		}
		if previous, seen := out[spelling]; seen && previous != resolution {
			t.Fatalf("type reference %q resolved two ways: %v and %v", spelling, previous, resolution)
		}
		out[spelling] = resolution
	}
	return out
}

// declarationName returns the authored spelling one declaration row names.
func declarationName(t *testing.T, p *program.Program, key keyspace.Key) string {
	t.Helper()
	literal, ok := p.Source().Keys().Exact(key)
	if !ok || literal.Kind != keyspace.LiteralString {
		t.Fatalf("declaration name key %v is not a literal name", key)
	}
	return literal.String
}

// TestAmbientTypeNamesResolveAsProgramDeclarations is the ambient namespace
// law. A name the ambient catalogue declares is always in scope for an
// annotation, so it lowers to a resolved declaration reference exactly like an
// authored alias does; nothing downstream sees an unresolved reference, and
// nothing downstream needs a second, ambient-only resolution rule.
func TestAmbientTypeNamesResolveAsProgramDeclarations(t *testing.T) {
	p := parseBindLower(t, `type Job = { id: string }

local function dispatch(out: Channel<Job>, meta: table)
	return out, meta
end

return dispatch
`)
	references := ambientReferences(t, p)
	for _, spelling := range []string{ambient.Channel, ambient.Table, "Job"} {
		resolution, held := references[spelling]
		if !held {
			t.Fatalf("type reference %q is absent from the lowered Program", spelling)
		}
		if resolution != staticrefs.Declaration {
			t.Fatalf("type reference %q resolved %v, want %v", spelling, resolution, staticrefs.Declaration)
		}
	}
}

// TestAmbientGenericDeclaresItsOrderedParameters holds the shape the catalogue
// states: a parameterized ambient entry is the alias binding its ordered
// parameters over the nominal body carrying its own name. That is what makes
// Channel<T> an application of one generic declaration rather than a bare name.
func TestAmbientGenericDeclaresItsOrderedParameters(t *testing.T) {
	p := parseBindLower(t, `type Job = { id: string }

local function dispatch(out: Channel<Job>)
	return out
end

return dispatch
`)
	declaration, held := ambient.Lookup(ambient.Channel)
	if !held {
		t.Fatalf("the ambient catalogue does not declare %q", ambient.Channel)
	}
	aliases := p.Static().Declarations().Aliases()
	found := false
	for index := 0; index < aliases.Count(); index++ {
		alias, ok := aliases.At(index)
		if !ok {
			t.Fatalf("alias %d is absent", index)
		}
		_, target, key, _, aliasOK := aliases.Get(alias)
		name := declarationName(t, p, key)
		if !aliasOK {
			t.Fatalf("alias %d has no row", index)
		}
		if name != ambient.Channel {
			continue
		}
		found = true
		params, paramsOK := aliases.ParamCount(alias)
		if !paramsOK || params != len(declaration.Params) {
			t.Fatalf("ambient %q binds %d parameters, the catalogue declares %d", name, params, len(declaration.Params))
		}
		if target == 0 {
			t.Fatalf("ambient %q has no target", name)
		}
		resolution, body, _, held := p.Static().References().Get(target)
		if !held || resolution != staticrefs.Declaration || keyspace.TermFamily(body) != keyspace.FamilyTypeInterface {
			t.Fatalf("ambient %q target = %v/%v/%v, want a declaration reference to its nominal body", name, resolution, body, held)
		}
	}
	if !found {
		t.Fatalf("the lowered Program declares no ambient %q", ambient.Channel)
	}
}

// TestAmbientTypesAreDeclaredOnlyWhenNamed keeps the ambient namespace out of
// a Program that does not use it: being in scope is not being declared, so a
// source annotating nothing ambient carries no ambient row.
func TestAmbientTypesAreDeclaredOnlyWhenNamed(t *testing.T) {
	p := parseBindLower(t, `type Job = { id: string }

local function dispatch(job: Job)
	return job
end

return dispatch
`)
	interfaces := p.Static().Declarations().Interfaces()
	for index := 0; index < interfaces.Count(); index++ {
		iface, ok := interfaces.At(index)
		if !ok {
			t.Fatalf("interface %d is absent", index)
		}
		_, key, _, ifaceOK := interfaces.Get(iface)
		if !ifaceOK {
			t.Fatalf("interface %d has no row", index)
		}
		name := declarationName(t, p, key)
		if _, ambientName := ambient.Lookup(name); ambientName {
			t.Fatalf("a source naming no ambient type declared the ambient %q", name)
		}
	}
}

// TestAuthoredDeclarationShadowsTheAmbientName holds the scope rule: the
// ambient namespace encloses the chunk, so an authored declaration of the same
// name is the one an annotation resolves to.
func TestAuthoredDeclarationShadowsTheAmbientName(t *testing.T) {
	p := parseBindLower(t, `type Channel = { id: string }

local function dispatch(out: Channel)
	return out
end

return dispatch
`)
	aliases := p.Static().Declarations().Aliases()
	shadow := keyspace.Term(0)
	for index := 0; index < aliases.Count(); index++ {
		alias, ok := aliases.At(index)
		if !ok {
			t.Fatalf("alias %d is absent", index)
		}
		_, _, key, _, aliasOK := aliases.Get(alias)
		if !aliasOK {
			t.Fatalf("alias %d has no row", index)
		}
		if declarationName(t, p, key) == ambient.Channel {
			params, paramsOK := aliases.ParamCount(alias)
			if !paramsOK || params != 0 {
				t.Fatalf("the authored %q alias binds %d parameters", ambient.Channel, params)
			}
			shadow = alias
		}
	}
	if shadow == 0 {
		t.Fatalf("the authored %q alias is absent", ambient.Channel)
	}
	interfaces := p.Static().Declarations().Interfaces()
	for index := 0; index < interfaces.Count(); index++ {
		iface, ok := interfaces.At(index)
		if !ok {
			t.Fatalf("interface %d is absent", index)
		}
		_, key, _, ifaceOK := interfaces.Get(iface)
		if !ifaceOK {
			t.Fatalf("interface %d has no row", index)
		}
		if name := declarationName(t, p, key); name == ambient.Channel {
			t.Fatalf("the shadowed ambient %q was declared anyway", name)
		}
	}
}
