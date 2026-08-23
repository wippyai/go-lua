package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// predeclareAmbient materializes the ambient declarations this chunk names as
// ordinary declarations of the chunk Body.
//
// The ambient namespace is a fixed declaration of the language surface, so
// nothing downstream may treat its names specially: a reference to Channel is
// the same Declaration edge a reference to an authored alias is, and the type
// authority reads the resulting rows without knowing which of the two it came
// from. Only a named entry is materialized, so a chunk that annotates nothing
// ambient carries no ambient row and keeps its previous identity.
//
// An ambient type's body is the empty nominal interface carrying its own name.
// A parameterless entry is that interface; a parameterized entry is the alias
// binding its ordered parameters over the same interface, which is what makes
// an alias-with-parameters the generic the catalogue declares.
func (w *Writer) predeclareAmbient(body keyspace.Term) error {
	if w == nil || w.binding == nil {
		return fmt.Errorf("lualower: static writer is not initialized")
	}
	declarations := w.binding.AmbientTypes()
	if len(declarations) == 0 {
		return nil
	}
	// An ambient declaration is a declaration of the whole source, not of one
	// authored turn inside it, so it carries the source's own opening span.
	span, ok := coord.Build(w.sourceName, 1, 1, 1, 1)
	if !ok {
		return fmt.Errorf("lualower: could not span the ambient namespace of %q", w.sourceName)
	}
	for _, decl := range declarations {
		if decl.ID == 0 || decl.Kind != bind.TypeDeclAmbient || decl.Ambient.Name != decl.Name {
			return fmt.Errorf("lualower: invalid ambient type declaration %q", decl.Name)
		}
		if _, exists := w.terms[decl.ID]; exists {
			return fmt.Errorf("lualower: duplicate ambient type identity %q", decl.Name)
		}
		term, err := w.ambientType(span, body, decl)
		if err != nil {
			return err
		}
		if w.terms == nil {
			w.terms = make(map[bind.TypeDeclID]keyspace.Term)
		}
		w.terms[decl.ID] = term
	}
	return nil
}

// ambientType declares one catalogue entry and publishes every declaration it
// creates in the Body's source order.
func (w *Writer) ambientType(span source.Span, body keyspace.Term, decl bind.TypeDecl) (keyspace.Term, error) {
	nominal := w.static.Interface(span, span, body, decl.Name)
	if nominal == 0 {
		return 0, fmt.Errorf("lualower: could not declare ambient type %q", decl.Name)
	}
	if !w.static.InterfaceExtends(nominal, nil) || !w.static.InterfaceMembers(nominal, nil) {
		return 0, fmt.Errorf("lualower: could not complete ambient type %q", decl.Name)
	}
	if err := w.scopes.Append(nominal); err != nil {
		return 0, err
	}
	if len(decl.Ambient.Params) == 0 {
		return nominal, nil
	}
	alias := w.static.Alias(span, span, body, decl.Name)
	if alias == 0 {
		return 0, fmt.Errorf("lualower: could not declare ambient generic %q", decl.Name)
	}
	params := make([]keyspace.Term, 0, len(decl.Ambient.Params))
	for _, name := range decl.Ambient.Params {
		param := w.static.TypeParam(span, alias, name)
		if param == 0 || !w.static.TypeParamConstraint(param, 0) {
			return 0, fmt.Errorf("lualower: could not declare ambient type parameter %q of %q", name, decl.Name)
		}
		params = append(params, param)
	}
	// An alias target is a type node, and a declaration is not one: the body is
	// reached through the same resolved reference an authored alias would use.
	target := w.static.Declaration(span, []string{decl.Name}, 0, nominal)
	if target == 0 {
		return 0, fmt.Errorf("lualower: could not reference the ambient body of %q", decl.Name)
	}
	if !w.static.AliasParams(alias, params) || !w.static.AliasTarget(alias, target) {
		return 0, fmt.Errorf("lualower: could not complete ambient generic %q", decl.Name)
	}
	if err := w.scopes.Append(alias); err != nil {
		return 0, err
	}
	return alias, nil
}
