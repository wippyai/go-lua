package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/keyspace"
)

// BeginInterface returns the predeclared interface identity at its authored
// source turn. The active Body sequence is the sole order authority.
func (w *Writer) BeginInterface(def *ast.InterfaceDefStmt) (keyspace.Term, error) {
	iface, decl, err := w.interfaceDecl(def)
	if err != nil {
		return 0, err
	}
	_ = decl
	return iface, nil
}

// Interface returns the exact predeclared Program host for one bound source
// interface declaration.
func (w *Writer) Interface(def *ast.InterfaceDefStmt) (keyspace.Term, bool) {
	term, _, err := w.interfaceDecl(def)
	return term, err == nil
}

// FinishInterface consumes the one exact ordered member range. Extends remain
// TypeRefs; fields reuse TypeField; methods retain their names while pointing
// at TypeFunction. No split field/method projection survives this boundary.
func (w *Writer) FinishInterface(def *ast.InterfaceDefStmt, childMark, memberMark int) error {
	iface, decl, err := w.interfaceDecl(def)
	if err != nil {
		return err
	}
	if w == nil || def == nil {
		return fmt.Errorf("programlower: invalid interface declaration")
	}
	count := len(def.Extends)
	if childMark < 0 || childMark > len(w.children) || len(w.children)-childMark != count {
		return fmt.Errorf("programlower: incomplete interface declaration children")
	}
	if memberMark < 0 || memberMark > len(w.interfaceMembers) || len(w.interfaceMembers)-memberMark != len(def.Members) {
		return fmt.Errorf("programlower: incomplete interface member scratch")
	}
	children, err := w.rangeTerms(childMark, count)
	if err != nil {
		return err
	}
	if !w.static.Declarations().InterfaceExtends(iface, children) ||
		!w.static.Declarations().InterfaceMembers(iface, w.interfaceMembers[memberMark:]) {
		w.interfaceMembers = w.interfaceMembers[:memberMark]
		return fmt.Errorf("programlower: could not finalize interface %q", decl.Name)
	}
	w.interfaceMembers = w.interfaceMembers[:memberMark]
	return nil
}

func (w *Writer) interfaceDecl(def *ast.InterfaceDefStmt) (keyspace.Term, bind.TypeDecl, error) {
	if w == nil || w.binding == nil || def == nil {
		return 0, bind.TypeDecl{}, fmt.Errorf("programlower: invalid interface declaration")
	}
	decl, ok := w.binding.InterfaceDef(def)
	if !ok || decl.Kind != bind.TypeDeclInterface || decl.ID == 0 || decl.Name != def.Name {
		return 0, bind.TypeDecl{}, fmt.Errorf("programlower: missing interface binding")
	}
	term, ok := w.Host(decl)
	if !ok {
		return 0, bind.TypeDecl{}, fmt.Errorf("programlower: interface %q was not predeclared", decl.Name)
	}
	return term, decl, nil
}
