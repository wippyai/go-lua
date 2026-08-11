package gorewrite

import (
	"fmt"
	"go/ast"
	"go/token"
)

// RelocateNamedFields moves the named fields of owner to child, adds the
// owner.child field, and updates keyed Owner literals. It rejects literals
// where grouping the moved values would alter left-to-right evaluation.
func RelocateNamedFields(file *ast.File, fset *token.FileSet, relocation FieldRelocation) error {
	if err := CheckNamedFieldRelocationLiterals(file, fset, relocation); err != nil {
		return err
	}
	owner := namedStruct(file, relocation.Owner)
	child := namedStruct(file, relocation.Child)
	if child == nil {
		return fmt.Errorf("missing child struct %s", relocation.Child)
	}
	moved, err := removeNamedFields(owner, relocation.Fields)
	if err != nil {
		return err
	}
	child.Fields.List = append(child.Fields.List, moved...)
	if err := addChildField(owner, relocation.ChildField, relocation.Child); err != nil {
		return err
	}
	var first error
	ast.Inspect(file, func(node ast.Node) bool {
		if first != nil {
			return false
		}
		literal, ok := node.(*ast.CompositeLit)
		if !ok || !isNamedType(literal.Type, relocation.Owner) {
			return true
		}
		if err := rewriteOwnerLiteral(literal, relocation); err != nil {
			first = fmt.Errorf("%s: %w", fset.Position(literal.Pos()), err)
			return false
		}
		return true
	})
	return first
}

// CheckNamedFieldRelocationLiterals validates the source-local part of a
// named-field extraction without mutating syntax. It is shared by the
// architectural compiler and the renderer so an Intent cannot be accepted
// for a literal layout that RelocateNamedFields would later reject.
func CheckNamedFieldRelocationLiterals(file *ast.File, fset *token.FileSet, relocation FieldRelocation) error {
	if file == nil || fset == nil {
		return fmt.Errorf("field relocation requires source syntax and file set")
	}
	if err := relocation.validate(); err != nil {
		return err
	}
	if err := rejectAuthorityHazards(file, fset, nil); err != nil {
		return err
	}
	if namedStruct(file, relocation.Owner) == nil {
		return fmt.Errorf("missing owner struct %s", relocation.Owner)
	}
	var first error
	ast.Inspect(file, func(node ast.Node) bool {
		if first != nil {
			return false
		}
		literal, ok := node.(*ast.CompositeLit)
		if !ok || !isNamedType(literal.Type, relocation.Owner) {
			return true
		}
		if _, err := ownerLiteralPlan(literal, relocation); err != nil {
			first = fmt.Errorf("%s: %w", fset.Position(literal.Pos()), err)
			return false
		}
		return true
	})
	return first
}

func namedStruct(file *ast.File, name string) *ast.StructType {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != name {
				continue
			}
			structure, _ := typeSpec.Type.(*ast.StructType)
			return structure
		}
	}
	return nil
}

func removeNamedFields(owner *ast.StructType, wanted map[string]struct{}) ([]*ast.Field, error) {
	var moved []*ast.Field
	remaining := make([]*ast.Field, 0, len(owner.Fields.List))
	found := make(map[string]bool, len(wanted))
	for _, field := range owner.Fields.List {
		if len(field.Names) != 1 {
			remaining = append(remaining, field)
			continue
		}
		name := field.Names[0].Name
		if _, take := wanted[name]; !take {
			remaining = append(remaining, field)
			continue
		}
		found[name] = true
		moved = append(moved, field)
	}
	for name := range wanted {
		if !found[name] {
			return nil, fmt.Errorf("owner has no uniquely named field %s", name)
		}
	}
	owner.Fields.List = remaining
	return moved, nil
}

func addChildField(owner *ast.StructType, fieldName, child string) error {
	for _, field := range owner.Fields.List {
		for _, name := range field.Names {
			if name.Name == fieldName {
				return fmt.Errorf("owner already has child field %s", fieldName)
			}
		}
	}
	owner.Fields.List = append(owner.Fields.List, &ast.Field{Names: []*ast.Ident{ast.NewIdent(fieldName)}, Type: ast.NewIdent(child)})
	return nil
}

func isNamedType(expr ast.Expr, name string) bool {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name == name
	case *ast.IndexExpr:
		return isNamedType(x.X, name)
	case *ast.IndexListExpr:
		return isNamedType(x.X, name)
	default:
		return false
	}
}

func rewriteOwnerLiteral(literal *ast.CompositeLit, relocation FieldRelocation) error {
	plan, err := ownerLiteralPlan(literal, relocation)
	if err != nil {
		return err
	}
	if plan.firstMoved < 0 {
		return nil
	}
	childElements := make([]ast.Expr, 0, plan.lastMoved-plan.firstMoved+1)
	childElements = append(childElements, literal.Elts[plan.firstMoved:plan.lastMoved+1]...)
	child := &ast.KeyValueExpr{
		Key:   ast.NewIdent(relocation.ChildField),
		Value: &ast.CompositeLit{Type: ast.NewIdent(relocation.Child), Elts: childElements},
	}
	updated := make([]ast.Expr, 0, len(literal.Elts)-(plan.lastMoved-plan.firstMoved))
	updated = append(updated, literal.Elts[:plan.firstMoved]...)
	updated = append(updated, child)
	updated = append(updated, literal.Elts[plan.lastMoved+1:]...)
	literal.Elts = updated
	return nil
}

type literalPlan struct {
	firstMoved int
	lastMoved  int
}

func ownerLiteralPlan(literal *ast.CompositeLit, relocation FieldRelocation) (literalPlan, error) {
	if len(literal.Elts) == 0 {
		return literalPlan{firstMoved: -1, lastMoved: -1}, nil
	}
	plan := literalPlan{firstMoved: -1, lastMoved: -1}
	for index, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return literalPlan{}, fmt.Errorf("unkeyed %s literal cannot be safely relocated", relocation.Owner)
		}
		key, ok := keyValue.Key.(*ast.Ident)
		if !ok {
			return literalPlan{}, fmt.Errorf("non-identifier key in %s literal", relocation.Owner)
		}
		if key.Name == relocation.ChildField {
			return literalPlan{}, fmt.Errorf("%s literal already initializes %s; merge is intentionally unsupported", relocation.Owner, relocation.ChildField)
		}
		if _, moved := relocation.Fields[key.Name]; moved {
			if plan.firstMoved < 0 {
				plan.firstMoved = index
			}
			plan.lastMoved = index
		}
	}
	if plan.firstMoved < 0 {
		return plan, nil
	}
	for index := plan.firstMoved; index <= plan.lastMoved; index++ {
		keyValue := literal.Elts[index].(*ast.KeyValueExpr)
		key := keyValue.Key.(*ast.Ident)
		if _, moved := relocation.Fields[key.Name]; !moved {
			return literalPlan{}, fmt.Errorf("moved fields are interleaved with %s in composite literal", key.Name)
		}
	}
	return plan, nil
}
