package generator

// MethodExposure and SurfaceInfo are scanner projections. They do not add a
// declaration authority: every target is an existing DeclarationInfo fact and
// every surface is an existing TypeShapes root/surface-root fact.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"sort"
	"strconv"

	"golang.org/x/tools/go/packages"
)

var (
	ErrMethodExposure = errors.New("link ownership: method exposure projection failed")
	ErrSurfaceInfo    = errors.New("link ownership: surface projection failed")
)

// MethodExposure is one effective method-set member for one package-scope
// declared type or alias. TargetDeclID always points at exactly one concrete
// or interface-method declaration. Disposition is one of declared, promoted,
// embedded, or aliased.
type MethodExposure struct {
	FactID       string
	PackagePath  string
	RootType     string
	Surface      string
	Set          string
	Name         string
	Signature    string
	TargetDeclID string
	Disposition  string
}

// SurfaceInfo is a projection of one named TypeShapes root or one explicit
// SurfaceRootFact. Empty anonymous structs are retained: their field creates a
// nonempty SurfaceRootFact even when the surface has no FieldFacts of its own.
type SurfaceInfo struct {
	FactID        string
	PackagePath   string
	RootType      string
	Surface       string
	ParentSurface string
	Path          string
	Kind          string
	Type          string
	SourceFile    string
	Line          int
	Column        int
	OriginDeclID  string
}

func methodExposureProjection(root string, family []*packages.Package, inv declarationInventory) ([]MethodExposure, error) {
	familyPaths := make(map[string]struct{}, len(family))
	ordered := append([]*packages.Package(nil), family...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].PkgPath < ordered[j].PkgPath })
	for _, pkg := range ordered {
		if pkg == nil || pkg.Types == nil {
			return nil, fmt.Errorf("%w: incomplete package", ErrMethodExposure)
		}
		familyPaths[pkg.PkgPath] = struct{}{}
	}
	result := make([]MethodExposure, 0)
	for _, pkg := range ordered {
		names := append([]string(nil), pkg.Types.Scope().Names()...)
		sort.Strings(names)
		for _, name := range names {
			typeName, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok {
				continue
			}
			subject := types.Unalias(typeName.Type())
			valueSet := types.NewMethodSet(subject)
			pointerSet := types.NewMethodSet(types.NewPointer(subject))
			for _, member := range []struct {
				name string
				set  *types.MethodSet
			}{
				{name: "value", set: valueSet},
				{name: "pointer", set: pointerSet},
			} {
				for index := 0; index < member.set.Len(); index++ {
					fn, ok := member.set.At(index).Obj().(*types.Func)
					if !ok || fn == nil || fn.Pkg() == nil {
						continue
					}
					if _, familyOwned := familyPaths[fn.Pkg().Path()]; !familyOwned {
						// The declaration authority is the live Link family.
						// Foreign methods are not Link declarations and do not
						// become an unjoinable projection row.
						continue
					}
					declaration, err := declarationForObject(root, pkg, fn, inv)
					if err != nil {
						return nil, fmt.Errorf("%w: %s.%s %s-set %s: %v", ErrMethodExposure, pkg.PkgPath, name, member.name, fn.Name(), err)
					}
					exposure := MethodExposure{
						PackagePath:  pkg.PkgPath,
						RootType:     name,
						Surface:      typeSurfaceID(pkg.PkgPath, name),
						Set:          member.name,
						Name:         fn.Name(),
						Signature:    declaration.Signature,
						TargetDeclID: declaration.FactID,
						Disposition:  methodDisposition(typeName, subject, fn),
					}
					exposure.FactID = methodExposureFactID(exposure)
					result = append(result, exposure)
				}
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return methodExposureLess(result[i], result[j]) })
	return result, nil
}

func methodDisposition(typeName *types.TypeName, subject types.Type, method *types.Func) string {
	if typeName != nil && typeName.IsAlias() {
		return "aliased"
	}
	if named, ok := subject.(*types.Named); ok {
		if iface, isInterface := named.Underlying().(*types.Interface); isInterface {
			iface.Complete()
			for index := 0; index < iface.NumExplicitMethods(); index++ {
				if sameMethodObject(iface.ExplicitMethod(index), method) {
					return "declared"
				}
			}
			return "embedded"
		}
	}
	if sig, ok := method.Type().(*types.Signature); ok && sig.Recv() != nil && receiverNamedName(sig.Recv().Type()) == typeName.Name() {
		return "declared"
	}
	return "promoted"
}

func sameMethodObject(left, right *types.Func) bool {
	if left == nil || right == nil {
		return false
	}
	if left == right {
		return true
	}
	return left.Pkg() != nil && right.Pkg() != nil && left.Pkg().Path() == right.Pkg().Path() && left.Name() == right.Name() && left.Pos() == right.Pos()
}

func methodExposureFactID(exposure MethodExposure) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-method-exposure-v1")
	writeProjectionPart(hash, exposure.PackagePath)
	writeProjectionPart(hash, exposure.RootType)
	writeProjectionPart(hash, exposure.Surface)
	writeProjectionPart(hash, exposure.Set)
	writeProjectionPart(hash, exposure.Name)
	writeProjectionPart(hash, exposure.Signature)
	writeProjectionPart(hash, exposure.TargetDeclID)
	writeProjectionPart(hash, exposure.Disposition)
	return "exposure-v1-" + hex.EncodeToString(hash.Sum(nil))
}

func methodExposureLess(left, right MethodExposure) bool {
	return compare(left.PackagePath, right.PackagePath, left.RootType, right.RootType,
		left.Surface, right.Surface, left.Set, right.Set, left.Name, right.Name,
		left.Signature, right.Signature, left.TargetDeclID, right.TargetDeclID,
		left.Disposition, right.Disposition, left.FactID, right.FactID) < 0
}

func surfaceProjection(root string, family []*packages.Package, shapes []TypeShapeInfo, inv declarationInventory) ([]SurfaceInfo, error) {
	byPackage := make(map[string]*packages.Package, len(family))
	for _, pkg := range family {
		if pkg == nil || pkg.Types == nil || pkg.Fset == nil {
			return nil, fmt.Errorf("%w: incomplete package", ErrSurfaceInfo)
		}
		byPackage[pkg.PkgPath] = pkg
	}
	result := make([]SurfaceInfo, 0)
	seen := make(map[string]struct{})
	for _, shape := range shapes {
		pkg := byPackage[shape.PackagePath]
		if pkg == nil {
			return nil, fmt.Errorf("%w: shape package %s is not loaded", ErrSurfaceInfo, shape.PackagePath)
		}
		rootSurface := typeSurfaceID(shape.PackagePath, shape.Name)
		if shape.Facts.Surface != rootSurface || shape.Facts.Owner != shape.PackagePath {
			return nil, fmt.Errorf("%w: root shape context drift %s.%s", ErrSurfaceInfo, shape.PackagePath, shape.Name)
		}
		declaration, err := rootTypeDeclaration(inv, shape.PackagePath, shape.Name)
		if err != nil {
			return nil, err
		}
		kind := "named-root"
		if declaration.Kind == "alias" {
			kind = "alias-root"
		}
		rootInfo := SurfaceInfo{
			PackagePath: shape.PackagePath, RootType: shape.Name, Surface: rootSurface,
			Kind: kind, Type: declaration.Type, SourceFile: declaration.SourceFile,
			Line: declaration.Line, Column: declaration.Column, OriginDeclID: declaration.FactID,
		}
		if err := addSurfaceInfo(&result, seen, rootInfo); err != nil {
			return nil, err
		}
		for _, rootFact := range shape.Facts.SurfaceRoots {
			if rootFact.Owner != shape.PackagePath || rootFact.Surface == rootSurface || rootFact.Position == token.NoPos {
				return nil, fmt.Errorf("%w: malformed surface root %s.%s: %+v", ErrSurfaceInfo, shape.PackagePath, shape.Name, rootFact)
			}
			location := pkg.Fset.PositionFor(rootFact.Position, true)
			source, ok := repoRelative(root, location.Filename)
			if !ok || source == "" || location.Line <= 0 || location.Column <= 0 {
				return nil, fmt.Errorf("%w: surface root %s has invalid source position", ErrSurfaceInfo, rootFact.Surface)
			}
			origin, err := surfaceOriginDeclaration(root, pkg, inv, shape, rootFact)
			if err != nil {
				return nil, err
			}
			info := SurfaceInfo{
				PackagePath: shape.PackagePath, RootType: shape.Name, Surface: rootFact.Surface,
				ParentSurface: rootFact.ParentSurface, Path: rootFact.Path, Kind: rootFact.Kind,
				Type: rootFact.Type, SourceFile: source, Line: location.Line, Column: location.Column,
				OriginDeclID: origin.FactID,
			}
			if err := addSurfaceInfo(&result, seen, info); err != nil {
				return nil, err
			}
		}
	}
	expected := len(shapes)
	for _, shape := range shapes {
		expected += len(shape.Facts.SurfaceRoots)
	}
	if len(result) != expected {
		return nil, fmt.Errorf("%w: surface 1:1 coverage mismatch: got=%d want=%d", ErrSurfaceInfo, len(result), expected)
	}
	sort.Slice(result, func(i, j int) bool { return surfaceInfoLess(result[i], result[j]) })
	return result, nil
}

func rootTypeDeclaration(inv declarationInventory, packagePath, name string) (DeclarationInfo, error) {
	var matches []DeclarationInfo
	for _, declaration := range inv.Declarations {
		if declaration.PackagePath == packagePath && declaration.OwnerType == "" && declaration.Name == name && (declaration.Kind == "type" || declaration.Kind == "alias") {
			matches = append(matches, declaration)
		}
	}
	if len(matches) != 1 {
		all := make([]string, 0)
		for _, declaration := range inv.Declarations {
			if declaration.PackagePath == packagePath {
				all = append(all, declaration.Kind+":"+declaration.Name+":"+declaration.OwnerType)
			}
		}
		sort.Strings(all)
		return DeclarationInfo{}, fmt.Errorf("%w: root declaration %s.%s resolved to %d facts all=%v", ErrSurfaceInfo, packagePath, name, len(matches), all)
	}
	return matches[0], nil
}

func surfaceOriginDeclaration(root string, pkg *packages.Package, inv declarationInventory, shape TypeShapeInfo, rootFact SurfaceRootFact) (DeclarationInfo, error) {
	if pkg == nil || pkg.Fset == nil || rootFact.Position == token.NoPos {
		return DeclarationInfo{}, fmt.Errorf("%w: surface root %s has no source position", ErrSurfaceInfo, rootFact.Surface)
	}
	location := pkg.Fset.PositionFor(rootFact.Position, true)
	source, ok := repoRelative(root, location.Filename)
	if !ok || source == "" || location.Line <= 0 || location.Column <= 0 {
		return DeclarationInfo{}, fmt.Errorf("%w: surface root %s has invalid source coordinate", ErrSurfaceInfo, rootFact.Surface)
	}
	var matches []DeclarationInfo
	for _, declaration := range inv.Declarations {
		// The root fact's path is the structural path at which the anonymous
		// representation was opened.  Containers extend that path (for
		// example Items[]), while the source declaration remains the field
		// Items.  Source coordinate is therefore the canonical origin join;
		// exact structural-path equality would reject valid container roots.
		if declaration.PackagePath != shape.PackagePath || declaration.Kind != "field" || declaration.OwnerType != shape.Name || declaration.SourceFile != source || declaration.Line != location.Line || declaration.Column != location.Column {
			continue
		}
		matches = append(matches, declaration)
	}
	if len(matches) != 1 {
		return DeclarationInfo{}, fmt.Errorf("%w: surface root %s origin resolved to %d field facts", ErrSurfaceInfo, rootFact.Surface, len(matches))
	}
	return matches[0], nil
}

func addSurfaceInfo(result *[]SurfaceInfo, seen map[string]struct{}, info SurfaceInfo) error {
	if info.PackagePath == "" || info.RootType == "" || info.Surface == "" || info.Kind == "" || info.Type == "" || info.OriginDeclID == "" {
		return fmt.Errorf("%w: incomplete surface fact: %+v", ErrSurfaceInfo, info)
	}
	key := info.PackagePath + "\x00" + info.Surface
	if _, exists := seen[key]; exists {
		return fmt.Errorf("%w: duplicate surface %s", ErrSurfaceInfo, key)
	}
	info.FactID = surfaceInfoFactID(info)
	seen[key] = struct{}{}
	*result = append(*result, info)
	return nil
}

func surfaceInfoFactID(info SurfaceInfo) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-surface-v1")
	writeProjectionPart(hash, info.PackagePath)
	writeProjectionPart(hash, info.RootType)
	writeProjectionPart(hash, info.Surface)
	writeProjectionPart(hash, info.ParentSurface)
	writeProjectionPart(hash, info.Path)
	writeProjectionPart(hash, info.Kind)
	writeProjectionPart(hash, info.Type)
	writeProjectionPart(hash, info.SourceFile)
	writeProjectionPart(hash, strconv.Itoa(info.Line))
	writeProjectionPart(hash, strconv.Itoa(info.Column))
	writeProjectionPart(hash, info.OriginDeclID)
	return "surface-v1-" + hex.EncodeToString(hash.Sum(nil))
}

func surfaceInfoLess(left, right SurfaceInfo) bool {
	return compare(left.PackagePath, right.PackagePath, left.RootType, right.RootType,
		left.Surface, right.Surface, left.ParentSurface, right.ParentSurface,
		left.Path, right.Path, left.Kind, right.Kind, left.Type, right.Type,
		left.SourceFile, right.SourceFile, strconv.Itoa(left.Line), strconv.Itoa(right.Line),
		strconv.Itoa(left.Column), strconv.Itoa(right.Column), left.FactID, right.FactID) < 0
}

func writeProjectionPart(hash interface{ Write([]byte) (int, error) }, value string) {
	_, _ = fmt.Fprintf(hash, "%d:", len(value))
	_, _ = hash.Write([]byte(value))
	_, _ = hash.Write([]byte{'\n'})
}
