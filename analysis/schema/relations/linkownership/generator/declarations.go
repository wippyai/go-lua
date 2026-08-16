package generator

// This file is the cold declaration/use join for the Link ownership ledger.
// The declaration inventory is deliberately object based only at the join
// boundary.  Object identity is never part of a declaration's stable ID: the
// ID is derived from source-relative, canonical facts and is therefore
// reproducible across independent go/types loads.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

var (
	ErrDeclarationInventory = errors.New("link ownership: declaration inventory failed")
	ErrUseTargetJoin        = errors.New("link ownership: typed use target did not join exactly one declaration")
)

// DeclarationInfo is one canonical production declaration fact.  OwnerType
// is empty for package-scope declarations.  For fields and interface methods
// it is the declared named root type, while SyntheticPath identifies the
// anonymous structural surface below that root (empty means the named root).
// Path is the exact TypeShapes path and is retained as evidence, not used to
// infer ownership.
type DeclarationInfo struct {
	FactID            string
	PackagePath       string
	Kind              string
	OwnerType         string
	SyntheticPath     string
	Surface           string
	Path              string
	Name              string
	Type              string
	Signature         string
	AliasRHS          string
	AliasTargetDeclID string
	SourceFile        string
	Line              int
	Column            int
	Exported          bool
}

// declarationInventory contains the canonical ordered facts and the private
// exact join indexes used by typed Uses, Selections, and Instances.  The
// indexes are cold scanner state and do not become a runtime authority.
type declarationInventory struct {
	Declarations []DeclarationInfo
	byObject     map[types.Object][]string
	byKey        map[string][]string
	byID         map[string]DeclarationInfo
}

// inventoryDeclarations inventories all live packages in the Link family.
// TypeShapes supplies the admitted structural field set; this function only
// resolves each already-admitted shape fact to its source object and records
// source position.  It never follows foreign named types or invents a second
// structural ownership relation.
func inventoryDeclarations(root string, family []*packages.Package, shapes []TypeShapeInfo) (declarationInventory, error) {
	inv := declarationInventory{
		byObject: make(map[types.Object][]string),
		byKey:    make(map[string][]string),
		byID:     make(map[string]DeclarationInfo),
	}
	shapeByType := make(map[string]TypeShapeInfo, len(shapes))
	for _, shape := range shapes {
		key := shape.PackagePath + "\x00" + shape.Name
		if _, exists := shapeByType[key]; exists {
			return inv, fmt.Errorf("%w: duplicate type shape %s.%s", ErrDeclarationInventory, shape.PackagePath, shape.Name)
		}
		shapeByType[key] = shape
	}

	paths := make([]*packages.Package, len(family))
	copy(paths, family)
	sort.Slice(paths, func(i, j int) bool { return paths[i].PkgPath < paths[j].PkgPath })
	for _, pkg := range paths {
		if pkg == nil || pkg.Types == nil || pkg.TypesInfo == nil {
			return inv, fmt.Errorf("%w: incomplete package", ErrDeclarationInventory)
		}
		if err := inventoryPackageScope(root, pkg, &inv); err != nil {
			return inv, err
		}
		if err := inventoryDeclaredMethods(root, pkg, &inv); err != nil {
			return inv, err
		}
		if err := inventoryShapeMembers(root, pkg, shapeByType, &inv); err != nil {
			return inv, err
		}
	}
	if err := finalizeAliasCommitments(root, paths, &inv); err != nil {
		return inv, err
	}
	if len(inv.Declarations) == 0 {
		return inv, fmt.Errorf("%w: no declarations", ErrDeclarationInventory)
	}
	sort.Slice(inv.Declarations, func(i, j int) bool {
		return declarationLess(inv.Declarations[i], inv.Declarations[j])
	})
	return inv, nil
}

func inventoryPackageScope(root string, pkg *packages.Package, inv *declarationInventory) error {
	names := append([]string(nil), pkg.Types.Scope().Names()...)
	sort.Strings(names)
	for _, name := range names {
		object := pkg.Types.Scope().Lookup(name)
		if object == nil {
			return fmt.Errorf("%w: nil package-scope object %s.%s", ErrDeclarationInventory, pkg.PkgPath, name)
		}
		kind := ""
		switch value := object.(type) {
		case *types.TypeName:
			if value.IsAlias() {
				kind = "alias"
			} else {
				kind = "type"
			}
		case *types.Func:
			// Methods are not package-scope objects.  A function with no
			// receiver is the only function admitted in this pass.
			if sig, ok := value.Type().(*types.Signature); !ok || sig.Recv() != nil {
				continue
			}
			kind = "func"
		case *types.Var:
			kind = "var"
		case *types.Const:
			kind = "const"
		default:
			return fmt.Errorf("%w: unsupported package-scope object %T in %s", ErrDeclarationInventory, object, pkg.PkgPath)
		}
		info, err := declarationInfo(root, pkg, object, kind, "", "", "", "")
		if err != nil {
			return err
		}
		if err := addDeclaration(inv, object, info); err != nil {
			return err
		}
	}
	return nil
}

func inventoryDeclaredMethods(root string, pkg *packages.Package, inv *declarationInventory) error {
	// Defs is the authoritative typed declaration set.  Syntax traversal is
	// intentionally not used to rediscover methods or to include locals.
	objects := make([]*types.Func, 0)
	seen := make(map[*types.Func]struct{})
	for _, object := range pkg.TypesInfo.Defs {
		fn, ok := object.(*types.Func)
		if !ok || fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != pkg.PkgPath {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Recv() == nil {
			continue
		}
		// Abstract interface methods are declared through the TypeShapes
		// reference surface. Keeping them out of the concrete method plane
		// prevents one go/types Func from acquiring multiple declaration IDs
		// when an interface is embedded or selected externally.
		if receiverUnderlyingIsInterface(sig.Recv().Type()) {
			continue
		}
		if _, exists := seen[fn]; exists {
			continue
		}
		seen[fn] = struct{}{}
		objects = append(objects, fn)
	}
	sort.Slice(objects, func(i, j int) bool {
		return objectPositionKey(root, pkg, objects[i]) < objectPositionKey(root, pkg, objects[j])
	})
	for _, fn := range objects {
		sig := fn.Type().(*types.Signature)
		owner := receiverNamedName(sig.Recv().Type())
		if owner == "" {
			return fmt.Errorf("%w: method %s has no named receiver", ErrDeclarationInventory, fn.Name())
		}
		info, err := declarationInfo(root, pkg, fn, "method", owner, "", "", "")
		if err != nil {
			return err
		}
		if err := addDeclaration(inv, fn, info); err != nil {
			return err
		}
	}
	return nil
}

func receiverUnderlyingIsInterface(t types.Type) bool {
	for t != nil {
		t = types.Unalias(t)
		if pointer, ok := t.(*types.Pointer); ok {
			t = pointer.Elem()
			continue
		}
		if named, ok := t.(*types.Named); ok {
			_, isInterface := named.Underlying().(*types.Interface)
			return isInterface
		}
		_, isInterface := t.(*types.Interface)
		return isInterface
	}
	return false
}

func inventoryShapeMembers(root string, pkg *packages.Package, shapes map[string]TypeShapeInfo, inv *declarationInventory) error {
	names := append([]string(nil), pkg.Types.Scope().Names()...)
	sort.Strings(names)
	for _, name := range names {
		object, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName)
		if !ok || object.IsAlias() {
			continue
		}
		named, ok := object.Type().(*types.Named)
		if !ok {
			return fmt.Errorf("%w: non-named declared type %s.%s", ErrDeclarationInventory, pkg.PkgPath, name)
		}
		shape, exists := shapes[pkg.PkgPath+"\x00"+name]
		if !exists {
			return fmt.Errorf("%w: missing TypeShapes entry for %s.%s", ErrDeclarationInventory, pkg.PkgPath, name)
		}
		rootSurface := typeSurfaceID(pkg.PkgPath, name)
		_ = named // TypeShapes is the sole structural authority; the named object supplies identity.
		objectsAtPosition := declarationObjectsByPosition(pkg)
		surfaceRoots := make(map[string]SurfaceRootFact, len(shape.Facts.SurfaceRoots))
		for _, rootFact := range shape.Facts.SurfaceRoots {
			if rootFact.Owner != pkg.PkgPath || rootFact.Surface == "" || rootFact.Surface == rootSurface || rootFact.Position == token.NoPos {
				return fmt.Errorf("%w: malformed synthetic surface root for %s.%s: %+v", ErrDeclarationInventory, pkg.PkgPath, name, rootFact)
			}
			if syntheticPathFromSurface(rootSurface, rootFact.Surface) != rootFact.Path {
				return fmt.Errorf("%w: synthetic surface path mismatch for %s.%s: %+v", ErrDeclarationInventory, pkg.PkgPath, name, rootFact)
			}
			if _, duplicate := surfaceRoots[rootFact.Surface]; duplicate {
				return fmt.Errorf("%w: duplicate synthetic surface root %s", ErrDeclarationInventory, rootFact.Surface)
			}
			surfaceRoots[rootFact.Surface] = rootFact
		}

		fieldFacts := make(map[string]FieldFact, len(shape.Facts.Fields))
		for _, fact := range shape.Facts.Fields {
			if fact.Owner != pkg.PkgPath || fact.Surface == "" || fact.Path == "" || fact.Name == "" || fact.Type == "" {
				return fmt.Errorf("%w: malformed field shape %s.%s: %+v", ErrDeclarationInventory, pkg.PkgPath, name, fact)
			}
			key := memberKey(fact.Surface, fact.Path, fact.Name)
			if _, duplicate := fieldFacts[key]; duplicate {
				return fmt.Errorf("%w: duplicate field shape %s", ErrDeclarationInventory, key)
			}
			if fact.Surface != rootSurface {
				if _, ok := surfaceRoots[fact.Surface]; !ok {
					return fmt.Errorf("%w: field shape %s references undeclared synthetic surface %s", ErrDeclarationInventory, key, fact.Surface)
				}
			}
			fieldFacts[key] = fact
		}
		for key, fact := range fieldFacts {
			field, err := resolveFieldObject(pkg, objectsAtPosition, fact)
			if err != nil {
				return fmt.Errorf("%w: field shape %s: %v", ErrDeclarationInventory, key, err)
			}
			synthetic := syntheticPathFromSurface(rootSurface, fact.Surface)
			info, err := declarationInfo(root, pkg, field, "field", name, synthetic, fact.Surface, fact.Path)
			if err != nil {
				return err
			}
			if err := addDeclaration(inv, field, info); err != nil {
				return err
			}
		}

		// Explicit methods of named interfaces are structural declarations,
		// not the complete promoted method set. Anonymous interface methods
		// have no declaration owner and remain typed structure evidence.
		methodFacts := make(map[string]ReferenceFact)
		for _, fact := range shape.Facts.References {
			if fact.Kind == "method" {
				methodFacts[memberKey(fact.Surface, fact.Path, methodNameFromPath(fact.Path))] = fact
			}
		}
		for key, fact := range methodFacts {
			method, err := resolveMethodObject(pkg, objectsAtPosition, fact)
			if err != nil {
				// A complete interface shape may retain a promoted method
				// declared by an external package. It is not a family
				// declaration and therefore has no inventory row here.
				if isExternalReference(root, pkg, fact.Position) {
					continue
				}
				return fmt.Errorf("%w: interface method shape %s: %v", ErrDeclarationInventory, key, err)
			}
			nameAtPath := methodNameFromPath(fact.Path)
			if nameAtPath == "" {
				nameAtPath = method.Name()
			}
			// TypeShapes currently records interface method references.  Do
			// not require an exact reference for an explicitly declared
			// method when a future walker chooses a richer method kind, but
			// reject a structural method whose declared shape disagrees.
			if fact.Type != canonicalType(method.Type()) {
				return fmt.Errorf("%w: interface method type mismatch %s", ErrDeclarationInventory, key)
			}
			ownerType := name
			synthetic := syntheticPathFromSurface(rootSurface, fact.Surface)
			info, err := declarationInfo(root, pkg, method, "interface-method", ownerType, synthetic, fact.Surface, fact.Path)
			if err != nil {
				return err
			}
			info.Name = nameAtPath
			info.FactID = declarationFactID(info)
			if err := addDeclaration(inv, method, info); err != nil {
				return err
			}
		}
	}
	return nil
}

func declarationObjectsByPosition(pkg *packages.Package) map[token.Pos][]types.Object {
	result := make(map[token.Pos][]types.Object)
	if pkg == nil || pkg.TypesInfo == nil {
		return result
	}
	for id, object := range pkg.TypesInfo.Defs {
		if id == nil || object == nil || id.Pos() == token.NoPos {
			continue
		}
		result[id.Pos()] = append(result[id.Pos()], object)
	}
	for _, name := range pkg.Types.Scope().Names() {
		object := pkg.Types.Scope().Lookup(name)
		if object == nil || object.Pos() == token.NoPos {
			continue
		}
		result[object.Pos()] = appendUniqueObject(result[object.Pos()], object)
	}
	for position, objects := range result {
		sort.Slice(objects, func(i, j int) bool {
			if objects[i].Name() != objects[j].Name() {
				return objects[i].Name() < objects[j].Name()
			}
			return fmt.Sprintf("%T", objects[i]) < fmt.Sprintf("%T", objects[j])
		})
		result[position] = objects
	}
	return result
}

func appendUniqueObject(objects []types.Object, object types.Object) []types.Object {
	for _, previous := range objects {
		if previous == object {
			return objects
		}
	}
	return append(objects, object)
}

func resolveFieldObject(pkg *packages.Package, byPosition map[token.Pos][]types.Object, fact FieldFact) (*types.Var, error) {
	candidates := make([]*types.Var, 0, 1)
	for _, object := range byPosition[fact.Position] {
		field, ok := object.(*types.Var)
		if !ok || field.Name() != fact.Name || field.Pkg() == nil || field.Type() == nil {
			continue
		}
		if canonicalType(field.Type()) == fact.Type {
			candidates = append(candidates, field)
		}
	}
	if len(candidates) != 1 {
		return nil, fmt.Errorf("field %s at position %d resolved to %d declarations", fact.Name, fact.Position, len(candidates))
	}
	return candidates[0], nil
}

func resolveMethodObject(pkg *packages.Package, byPosition map[token.Pos][]types.Object, fact ReferenceFact) (*types.Func, error) {
	candidates := make([]*types.Func, 0, 1)
	name := methodNameFromPath(fact.Path)
	for _, object := range byPosition[fact.Position] {
		method, ok := object.(*types.Func)
		if !ok || method.Name() != name || method.Pkg() == nil || method.Type() == nil {
			continue
		}
		if canonicalType(method.Type()) == fact.Type {
			candidates = append(candidates, method)
		}
	}
	if len(candidates) != 1 {
		return nil, fmt.Errorf("method %s at position %d resolved to %d declarations", name, fact.Position, len(candidates))
	}
	return candidates[0], nil
}

func isExternalReference(root string, pkg *packages.Package, position token.Pos) bool {
	if pkg == nil || pkg.Fset == nil || position == token.NoPos {
		return false
	}
	location := pkg.Fset.PositionFor(position, true)
	_, inside := repoRelative(root, location.Filename)
	return !inside
}

func declarationInfo(root string, pkg *packages.Package, object types.Object, kind, owner, synthetic, surface, path string) (DeclarationInfo, error) {
	if object == nil || object.Pkg() == nil {
		return DeclarationInfo{}, fmt.Errorf("%w: declaration object is nil/local", ErrDeclarationInventory)
	}
	location := pkg.Fset.PositionFor(object.Pos(), true)
	source, ok := repoRelative(root, location.Filename)
	if !ok || source == "" || location.Line <= 0 || location.Column <= 0 {
		return DeclarationInfo{}, fmt.Errorf("%w: declaration %s.%s has invalid source position", ErrDeclarationInventory, object.Pkg().Path(), object.Name())
	}
	qualifier := packageQualifier(pkg.Types)
	typ := ""
	signature := ""
	switch value := object.(type) {
	case *types.Func:
		signature = types.TypeString(value.Type(), qualifier)
		typ = signature
	default:
		typ = types.TypeString(object.Type(), qualifier)
	}
	info := DeclarationInfo{
		PackagePath: pkg.PkgPath, Kind: kind, OwnerType: owner, SyntheticPath: synthetic,
		Surface: surface, Path: path, Name: object.Name(), Type: typ, Signature: signature,
		SourceFile: source, Line: location.Line, Column: location.Column, Exported: object.Exported(),
	}
	if kind == "alias" {
		info.AliasRHS = canonicalAliasRHS(object.Type())
	}
	info.FactID = declarationFactID(info)
	return info, nil
}

func addDeclaration(inv *declarationInventory, object types.Object, info DeclarationInfo) error {
	if info.FactID == "" || object == nil {
		return fmt.Errorf("%w: empty declaration fact", ErrDeclarationInventory)
	}
	if previous, exists := inv.byID[info.FactID]; exists {
		if !sameDeclaration(previous, info) {
			return fmt.Errorf("%w: fact ID collision %s", ErrDeclarationInventory, info.FactID)
		}
		// The same object may be reached once through a named interface
		// surface and once through its shape reference.  Preserve one fact.
		return nil
	}
	inv.byID[info.FactID] = info
	inv.Declarations = append(inv.Declarations, info)
	inv.byObject[object] = append(inv.byObject[object], info.FactID)
	key := declarationJoinKey(info.PackagePath, object.Name(), info.Type, info.SourceFile, info.Line, info.Column)
	inv.byKey[key] = append(inv.byKey[key], info.FactID)
	return nil
}

func finalizeAliasCommitments(root string, family []*packages.Package, inv *declarationInventory) error {
	oldToNew := make(map[string]string)
	for _, pkg := range family {
		if pkg == nil || pkg.Types == nil {
			return fmt.Errorf("%w: incomplete alias package", ErrDeclarationInventory)
		}
		for _, name := range pkg.Types.Scope().Names() {
			object, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok || !object.IsAlias() {
				continue
			}
			ids := inv.byObject[object]
			if len(ids) != 1 {
				return fmt.Errorf("%w: alias %s.%s has %d declaration rows", ErrDeclarationInventory, pkg.PkgPath, name, len(ids))
			}
			oldID := ids[0]
			info, ok := inv.byID[oldID]
			if !ok {
				return fmt.Errorf("%w: alias %s.%s declaration row is missing", ErrDeclarationInventory, pkg.PkgPath, name)
			}
			targetID, err := aliasTerminalTargetID(object.Type(), *inv)
			if err != nil {
				return fmt.Errorf("%w: alias %s.%s: %v", ErrDeclarationInventory, pkg.PkgPath, name, err)
			}
			info.AliasTargetDeclID = targetID
			newID := declarationFactID(info)
			// The commitment is part of the canonical declaration identity. Keep
			// the row's ID synchronized before touching any index: otherwise the
			// declaration slice and byObject/byKey indexes retain the pre-
			// commitment ID even though byID has been re-keyed.
			info.FactID = newID
			if previous, exists := inv.byID[newID]; exists && newID != oldID && !sameDeclaration(previous, info) {
				return fmt.Errorf("%w: alias fact ID collision %s", ErrDeclarationInventory, newID)
			}
			if newID != oldID {
				oldToNew[oldID] = newID
				delete(inv.byID, oldID)
			}
			inv.byID[newID] = info
		}
	}
	if len(oldToNew) == 0 {
		return nil
	}
	for index := range inv.Declarations {
		if newID, ok := oldToNew[inv.Declarations[index].FactID]; ok {
			info := inv.byID[newID]
			inv.Declarations[index] = info
		}
	}
	for object, ids := range inv.byObject {
		for index, id := range ids {
			if newID, ok := oldToNew[id]; ok {
				ids[index] = newID
			}
		}
		inv.byObject[object] = ids
	}
	for key, ids := range inv.byKey {
		for index, id := range ids {
			if newID, ok := oldToNew[id]; ok {
				ids[index] = newID
			}
		}
		inv.byKey[key] = ids
	}
	return nil
}

func canonicalAliasRHS(t types.Type) string {
	if alias, ok := t.(*types.Alias); ok {
		return canonicalType(alias.Rhs())
	}
	return canonicalType(types.Unalias(t))
}

func aliasTerminalTargetID(t types.Type, inv declarationInventory) (string, error) {
	for t != nil {
		switch value := t.(type) {
		case *types.Alias:
			t = value.Rhs()
			continue
		case *types.Named:
			object := value.Obj()
			if object == nil || object.Pkg() == nil {
				return "", nil
			}
			ids := inv.byObject[object]
			if len(ids) == 0 {
				return "", nil
			}
			if len(ids) != 1 {
				return "", fmt.Errorf("terminal type %s has %d declaration rows", object.Name(), len(ids))
			}
			return ids[0], nil
		default:
			return "", nil
		}
	}
	return "", nil
}

func sameDeclaration(left, right DeclarationInfo) bool {
	left.FactID = right.FactID
	return left == right
}

func declarationFactID(info DeclarationInfo) string {
	hash := sha256.New()
	writeCanonicalPart := func(value string) {
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{'\n'})
	}
	writeCanonicalPart("link-declaration-v1")
	writeCanonicalPart(info.PackagePath)
	writeCanonicalPart(info.Kind)
	writeCanonicalPart(info.OwnerType)
	writeCanonicalPart(info.SyntheticPath)
	writeCanonicalPart(info.Name)
	writeCanonicalPart(info.Type)
	writeCanonicalPart(info.Signature)
	writeCanonicalPart(info.AliasRHS)
	writeCanonicalPart(info.AliasTargetDeclID)
	writeCanonicalPart(info.SourceFile)
	writeCanonicalPart(strconv.Itoa(info.Line))
	writeCanonicalPart(strconv.Itoa(info.Column))
	return "decl-v1-" + hex.EncodeToString(hash.Sum(nil))
}

func declarationLess(a, b DeclarationInfo) bool {
	return compare(a.PackagePath, b.PackagePath, a.Kind, b.Kind, a.OwnerType, b.OwnerType,
		a.SyntheticPath, b.SyntheticPath, a.Path, b.Path, a.Name, b.Name, a.Type, b.Type,
		a.Signature, b.Signature, a.SourceFile, b.SourceFile,
		strconv.Itoa(a.Line), strconv.Itoa(b.Line), strconv.Itoa(a.Column), strconv.Itoa(b.Column),
		a.FactID, b.FactID) < 0
}

func objectPositionKey(root string, pkg *packages.Package, object types.Object) string {
	if object == nil {
		return ""
	}
	pos := pkg.Fset.PositionFor(object.Pos(), true)
	rel, _ := repoRelative(root, filepath.Clean(pos.Filename))
	return fmt.Sprintf("%s\x00%08d\x00%08d\x00%s", rel, pos.Line, pos.Column, object.Name())
}

func declarationJoinKey(packagePath, name, typ, source string, line, column int) string {
	return strings.Join([]string{packagePath, name, typ, source, strconv.Itoa(line), strconv.Itoa(column)}, "\x00")
}

func declarationForObject(root string, pkg *packages.Package, object types.Object, inv declarationInventory) (DeclarationInfo, error) {
	if object == nil || object.Pkg() == nil {
		return DeclarationInfo{}, fmt.Errorf("%w: local/non-package object", ErrUseTargetJoin)
	}
	if ids := inv.byObject[object]; len(ids) != 0 {
		if len(ids) != 1 {
			return DeclarationInfo{}, fmt.Errorf("%w: object %s has %d declaration facts", ErrUseTargetJoin, object.Name(), len(ids))
		}
		return inv.byID[ids[0]], nil
	}
	if pkg == nil || pkg.Fset == nil {
		return DeclarationInfo{}, fmt.Errorf("%w: missing package file set for %s.%s", ErrUseTargetJoin, object.Pkg().Path(), object.Name())
	}
	location := pkg.Fset.PositionFor(object.Pos(), true)
	source, ok := repoRelative(root, location.Filename)
	if !ok || source == "" || location.Line <= 0 || location.Column <= 0 {
		return DeclarationInfo{}, fmt.Errorf("%w: invalid source position for %s.%s", ErrUseTargetJoin, object.Pkg().Path(), object.Name())
	}
	qualifier := packageQualifier(pkg.Types)
	typ := types.TypeString(object.Type(), qualifier)
	key := declarationJoinKey(object.Pkg().Path(), object.Name(), typ, source, location.Line, location.Column)
	ids := inv.byKey[key]
	if len(ids) != 1 {
		return DeclarationInfo{}, fmt.Errorf("%w: %s.%s (%T type=%q at %s:%d:%d) resolved to %d declaration facts", ErrUseTargetJoin, object.Pkg().Path(), object.Name(), object, typ, source, location.Line, location.Column, len(ids))
	}
	return inv.byID[ids[0]], nil
}

func memberKey(surface, path, name string) string {
	return surface + "\x00" + path + "\x00" + name
}

func keySurface(key string) string {
	parts := strings.Split(key, "\x00")
	if len(parts) < 1 {
		return ""
	}
	return parts[0]
}

func keyPath(key string) string {
	parts := strings.Split(key, "\x00")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func methodNameFromPath(path string) string {
	if path == "" {
		return ""
	}
	last := path
	if index := strings.LastIndex(last, "."); index >= 0 {
		last = last[index+1:]
	}
	if index := strings.LastIndex(last, "}"); index >= 0 {
		last = last[index+1:]
	}
	return last
}

func syntheticPathFromSurface(rootSurface, surface string) string {
	if surface == rootSurface {
		return ""
	}
	prefix := rootSurface + "#"
	if strings.HasPrefix(surface, prefix) {
		return strings.TrimPrefix(surface, prefix)
	}
	return surface
}

func receiverNamedName(t types.Type) string {
	for t != nil {
		t = types.Unalias(t)
		if pointer, ok := t.(*types.Pointer); ok {
			t = pointer.Elem()
			continue
		}
		if named, ok := t.(*types.Named); ok && named.Obj() != nil {
			return named.Obj().Name()
		}
		break
	}
	return ""
}
