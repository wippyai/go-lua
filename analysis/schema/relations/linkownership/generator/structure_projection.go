package generator

// This file is the cold, backend-neutral structure projection. It consumes
// TypeShapes and the declaration inventory; it does not walk syntax and does
// not introduce a second type grammar. WalkType remains the only recursive
// go/types traversal, including for declaration signatures that are not state
// roots.

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

	"golang.org/x/tools/go/packages"
)

var ErrStructureProjection = errors.New("link ownership: structure projection failed")

func declarationSurface(declaration DeclarationInfo) string {
	return "declaration:" + declaration.FactID
}

// StructureProjection is a typed set of semantic planes. DeclarationInfo and
// SurfaceInfo own package/root/type/source facts; these rows carry only their
// stable IDs plus the structural evidence that is not already owned there.
// Declaration-signature rows deliberately have an empty SurfaceID: a
// declaration identity is never polymorphically reused as a surface ID.
type StructureProjection struct {
	Fields           []StructureField
	Arrays           []StructureArray
	Slices           []StructureSlice
	Maps             []StructureMap
	Channels         []StructureChannel
	NamedReferences  []StructureNamedReference
	MethodReferences []StructureMethodReference
	OtherReferences  []StructureOtherReference
	Cycles           []StructureCycle
}

// StructureField references the exact field declaration and structural
// surface. DeclarationInfo owns the field's name, type, path, and provenance.
type StructureField struct {
	FactID        string
	DeclarationID string
	SurfaceID     string
	Embedded      bool
}

type StructureArray struct {
	FactID        string
	DeclarationID string
	SurfaceID     string
	Path          string
	Element       string
	Length        int64
}

type StructureSlice struct {
	FactID        string
	DeclarationID string
	SurfaceID     string
	Path          string
	Element       string
}

type StructureMap struct {
	FactID        string
	DeclarationID string
	SurfaceID     string
	Path          string
	Key           string
	Value         string
}

type StructureChannel struct {
	FactID        string
	DeclarationID string
	SurfaceID     string
	Path          string
	Element       string
	Direction     string
}

type StructureNamedReference struct {
	FactID            string
	DeclarationID     string
	SurfaceID         string
	Path              string
	TargetDeclID      string
	TargetPackagePath string
	TargetName        string
	Origin            bool
}

type StructureMethodReference struct {
	FactID            string
	DeclarationID     string
	SurfaceID         string
	Path              string
	TargetDeclID      string
	TargetPackagePath string
	TargetName        string
	MethodKey         string
	Type              string
	Receiver          string
}

// OtherReferenceDisposition is closed: a new walker disposition must be
// admitted here rather than smuggled through a generic Kind string.
type OtherReferenceDisposition uint8

const (
	OtherPointer OtherReferenceDisposition = iota + 1
	OtherAnonymousStruct
	OtherMember
	OtherSignature
	OtherTuple
	OtherTypeParameter
	OtherUnionTerm
	OtherUnionTildeTerm
	OtherInterfaceMethod
)

type StructureOtherReference struct {
	FactID        string
	DeclarationID string
	SurfaceID     string
	Path          string
	Disposition   OtherReferenceDisposition
	Type          string
}

type StructureCycle struct {
	FactID        string
	DeclarationID string
	SurfaceID     string
	Path          string
	Type          string
}

// structureProjection is the one-line integration boundary for scanner.go.
func structureProjection(root string, family []*packages.Package, shapes []TypeShapeInfo, inv declarationInventory, surfaces []SurfaceInfo) (StructureProjection, error) {
	return structureProjectionInternal(root, family, shapes, inv, surfaces, nil)
}

// validateStructureProjectionClosure proves that every retained raw shape
// fact crossed into exactly one typed relation before TypeShapeInfo is
// discarded. Declaration-signature rows have no SurfaceID and are therefore
// outside this shape-closure cardinality; structureProjection validates those
// while it walks their source objects.
func validateStructureProjectionClosure(shapes []TypeShapeInfo, surfaces []SurfaceInfo, projection StructureProjection) error {
	type cardinality struct {
		Surfaces, Fields, Arrays, Slices, Maps, Channels int
		Named, Methods, Other, Cycles                    int
	}
	var expected, actual cardinality
	expectedSurfaces := make(map[string]struct{})
	addExpectedSurface := func(packagePath, surface string) error {
		if packagePath == "" || surface == "" {
			return fmt.Errorf("%w: empty raw surface identity", ErrStructureProjection)
		}
		key := packagePath + "\x00" + surface
		if _, duplicate := expectedSurfaces[key]; duplicate {
			return fmt.Errorf("%w: duplicate raw surface %s", ErrStructureProjection, key)
		}
		expectedSurfaces[key] = struct{}{}
		expected.Surfaces++
		return nil
	}
	for _, shape := range shapes {
		if err := addExpectedSurface(shape.PackagePath, shape.Facts.Surface); err != nil {
			return err
		}
		for _, root := range shape.Facts.SurfaceRoots {
			if err := addExpectedSurface(shape.PackagePath, root.Surface); err != nil {
				return err
			}
		}
		expected.Fields += len(shape.Facts.Fields)
		expected.Cycles += len(shape.Facts.Cycles)
		for _, container := range shape.Facts.Containers {
			switch container.Kind {
			case "array":
				expected.Arrays++
			case "slice":
				expected.Slices++
			case "map":
				expected.Maps++
			case "chan":
				expected.Channels++
			default:
				return fmt.Errorf("%w: unknown raw container kind %q", ErrStructureProjection, container.Kind)
			}
		}
		for _, reference := range shape.Facts.References {
			switch reference.Kind {
			case "named", "named-origin":
				expected.Named++
			case "method", "method-reference":
				expected.Methods++
			default:
				if _, ok := otherDisposition(reference.Kind); !ok {
					return fmt.Errorf("%w: unadmitted raw reference disposition %q", ErrStructureProjection, reference.Kind)
				}
				expected.Other++
			}
		}
	}

	surfaceIDs := make(map[string]struct{}, len(surfaces))
	actualSurfaceKeys := make(map[string]struct{}, len(surfaces))
	for _, surface := range surfaces {
		key := surface.PackagePath + "\x00" + surface.Surface
		if surface.FactID == "" || key == "\x00" {
			return fmt.Errorf("%w: incomplete typed surface %+v", ErrStructureProjection, surface)
		}
		if _, duplicate := actualSurfaceKeys[key]; duplicate {
			return fmt.Errorf("%w: duplicate typed surface %s", ErrStructureProjection, key)
		}
		if _, duplicate := surfaceIDs[surface.FactID]; duplicate {
			return fmt.Errorf("%w: duplicate typed surface identity %s", ErrStructureProjection, surface.FactID)
		}
		actualSurfaceKeys[key] = struct{}{}
		surfaceIDs[surface.FactID] = struct{}{}
		actual.Surfaces++
	}
	for key := range expectedSurfaces {
		if _, exists := actualSurfaceKeys[key]; !exists {
			return fmt.Errorf("%w: raw surface %s has no typed projection", ErrStructureProjection, key)
		}
	}
	checkSurface := func(kind, surfaceID string) error {
		if surfaceID == "" {
			return nil
		}
		if _, exists := surfaceIDs[surfaceID]; !exists {
			return fmt.Errorf("%w: %s row references unknown surface %s", ErrStructureProjection, kind, surfaceID)
		}
		return nil
	}
	for _, row := range projection.Fields {
		if row.SurfaceID == "" {
			return fmt.Errorf("%w: field row escaped raw shape closure %+v", ErrStructureProjection, row)
		}
		if err := checkSurface("field", row.SurfaceID); err != nil {
			return err
		}
		actual.Fields++
	}
	for _, row := range projection.Arrays {
		if err := checkSurface("array", row.SurfaceID); err != nil {
			return err
		}
		if row.SurfaceID != "" {
			actual.Arrays++
		}
	}
	for _, row := range projection.Slices {
		if err := checkSurface("slice", row.SurfaceID); err != nil {
			return err
		}
		if row.SurfaceID != "" {
			actual.Slices++
		}
	}
	for _, row := range projection.Maps {
		if err := checkSurface("map", row.SurfaceID); err != nil {
			return err
		}
		if row.SurfaceID != "" {
			actual.Maps++
		}
	}
	for _, row := range projection.Channels {
		if err := checkSurface("channel", row.SurfaceID); err != nil {
			return err
		}
		if row.SurfaceID != "" {
			actual.Channels++
		}
	}
	for _, row := range projection.NamedReferences {
		if err := checkSurface("named reference", row.SurfaceID); err != nil {
			return err
		}
		if row.SurfaceID != "" {
			actual.Named++
		}
	}
	for _, row := range projection.MethodReferences {
		if err := checkSurface("method reference", row.SurfaceID); err != nil {
			return err
		}
		if row.SurfaceID != "" {
			actual.Methods++
		}
	}
	for _, row := range projection.OtherReferences {
		if err := checkSurface("other reference", row.SurfaceID); err != nil {
			return err
		}
		if row.SurfaceID != "" {
			actual.Other++
		}
	}
	for _, row := range projection.Cycles {
		if err := checkSurface("cycle", row.SurfaceID); err != nil {
			return err
		}
		if row.SurfaceID != "" {
			actual.Cycles++
		}
	}
	if expected != actual {
		return fmt.Errorf("%w: raw-to-typed closure mismatch expected=%+v actual=%+v", ErrStructureProjection, expected, actual)
	}
	return nil
}

// structureProjectionWithWitness is an internal verification boundary. The
// witness records deterministic lookup work without changing the integration
// API used by scanner.go.
func structureProjectionWithWitness(root string, family []*packages.Package, shapes []TypeShapeInfo, inv declarationInventory, surfaces []SurfaceInfo) (StructureProjection, structureLookupWitness, error) {
	var witness structureLookupWitness
	projection, err := structureProjectionInternal(root, family, shapes, inv, surfaces, &witness)
	return projection, witness, err
}

func structureProjectionInternal(root string, family []*packages.Package, shapes []TypeShapeInfo, inv declarationInventory, surfaces []SurfaceInfo, witnessOut *structureLookupWitness) (StructureProjection, error) {
	if root == "" {
		return StructureProjection{}, fmt.Errorf("%w: repository root is empty", ErrStructureProjection)
	}
	packagesByPath := make(map[string]*packages.Package, len(family))
	for _, pkg := range family {
		if pkg == nil || pkg.PkgPath == "" || pkg.Types == nil || pkg.Fset == nil {
			return StructureProjection{}, fmt.Errorf("%w: incomplete package", ErrStructureProjection)
		}
		if _, exists := packagesByPath[pkg.PkgPath]; exists {
			return StructureProjection{}, fmt.Errorf("%w: duplicate package %s", ErrStructureProjection, pkg.PkgPath)
		}
		packagesByPath[pkg.PkgPath] = pkg
	}
	if len(inv.Declarations) == 0 {
		return StructureProjection{}, fmt.Errorf("%w: declaration inventory is empty", ErrStructureProjection)
	}

	declarationsByID := make(map[string]DeclarationInfo, len(inv.Declarations))
	rootDeclarations := make(map[string]DeclarationInfo)
	for _, declaration := range inv.Declarations {
		if declaration.FactID == "" || declaration.FactID != declarationFactID(declaration) {
			return StructureProjection{}, fmt.Errorf("%w: noncanonical declaration %+v", ErrStructureProjection, declaration)
		}
		if _, exists := declarationsByID[declaration.FactID]; exists {
			return StructureProjection{}, fmt.Errorf("%w: duplicate declaration %s", ErrStructureProjection, declaration.FactID)
		}
		declarationsByID[declaration.FactID] = declaration
		if declaration.OwnerType == "" && (declaration.Kind == "type" || declaration.Kind == "alias") {
			key := declaration.PackagePath + "\x00" + declaration.Name
			if _, exists := rootDeclarations[key]; exists {
				return StructureProjection{}, fmt.Errorf("%w: duplicate root declaration %s", ErrStructureProjection, key)
			}
			rootDeclarations[key] = declaration
		}
	}

	surfaceByKey := make(map[string]SurfaceInfo, len(surfaces))
	for _, surface := range surfaces {
		if surface.FactID == "" || surface.FactID != surfaceInfoFactID(surface) {
			return StructureProjection{}, fmt.Errorf("%w: noncanonical surface %+v", ErrStructureProjection, surface)
		}
		key := surface.PackagePath + "\x00" + surface.Surface
		if _, exists := surfaceByKey[key]; exists {
			return StructureProjection{}, fmt.Errorf("%w: duplicate surface %s", ErrStructureProjection, key)
		}
		surfaceByKey[key] = surface
	}
	owners, err := buildStructureOwnerIndex(declarationsByID, surfaceByKey)
	if err != nil {
		return StructureProjection{}, err
	}

	methodTargets, err := methodDeclarationTargets(inv, declarationsByID)
	if err != nil {
		return StructureProjection{}, err
	}
	projector := structureProjector{
		root:             root,
		packages:         packagesByPath,
		declarations:     declarationsByID,
		rootDeclarations: rootDeclarations,
		surfaces:         surfaceByKey,
		owners:           owners,
		methodTargets:    methodTargets,
		seen:             make(map[string]struct{}),
	}
	orderedShapes := append([]TypeShapeInfo(nil), shapes...)
	sort.Slice(orderedShapes, func(i, j int) bool {
		return compare(orderedShapes[i].PackagePath, orderedShapes[j].PackagePath, orderedShapes[i].Name, orderedShapes[j].Name) < 0
	})
	for _, shape := range orderedShapes {
		if err := projector.projectShape(shape); err != nil {
			return StructureProjection{}, err
		}
	}
	if err := projector.projectDeclarationTypes(inv); err != nil {
		return StructureProjection{}, err
	}
	projector.sort()
	if witnessOut != nil {
		*witnessOut = projector.owners.witness
	}
	return projector.projection, nil
}

type structureProjector struct {
	root             string
	packages         map[string]*packages.Package
	declarations     map[string]DeclarationInfo
	rootDeclarations map[string]DeclarationInfo
	surfaces         map[string]SurfaceInfo
	owners           *structureOwnerIndex
	methodTargets    map[string]DeclarationInfo
	projection       StructureProjection
	seen             map[string]struct{}
}

// structureLookupWitness is internal lookup instrumentation. PrefixVisits
// counts observed exact-path map probes; unrelated declarations cannot
// multiply it for unchanged paths, and this is not a formal complexity claim.
type structureLookupWitness struct {
	OwnerQueries       int
	PrefixVisits       int
	MaxPrefixVisits    int
	ReferenceJoins     int
	IndexEntries       int
	DeclarationEntries int
}

type structureOwnerIndex struct {
	// Buckets are package/root/surface -> exact structural path -> owner.
	// Duplicate exact paths are rejected during construction.
	buckets map[string]map[string]DeclarationInfo
	origins map[string]DeclarationInfo
	witness structureLookupWitness
}

func buildStructureOwnerIndex(declarations map[string]DeclarationInfo, surfaces map[string]SurfaceInfo) (*structureOwnerIndex, error) {
	index := &structureOwnerIndex{
		buckets: make(map[string]map[string]DeclarationInfo),
		origins: make(map[string]DeclarationInfo),
	}
	index.witness.DeclarationEntries = len(declarations)
	ordered := make([]DeclarationInfo, 0, len(declarations))
	for _, declaration := range declarations {
		ordered = append(ordered, declaration)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return compare(ordered[i].PackagePath, ordered[j].PackagePath,
			ordered[i].OwnerType, ordered[j].OwnerType, ordered[i].Surface, ordered[j].Surface,
			ordered[i].Path, ordered[j].Path, ordered[i].FactID, ordered[j].FactID) < 0
	})
	for _, declaration := range ordered {
		if (declaration.Kind != "field" && declaration.Kind != "interface-method") || declaration.Path == "" {
			continue
		}
		bucketKey := structureOwnerBucketKey(declaration.PackagePath, declaration.OwnerType, declaration.Surface)
		bucket := index.buckets[bucketKey]
		if bucket == nil {
			bucket = make(map[string]DeclarationInfo)
			index.buckets[bucketKey] = bucket
		}
		if previous, exists := bucket[declaration.Path]; exists {
			return nil, fmt.Errorf("%w: ambiguous exact owner path package=%s root=%s surface=%s path=%s declarations=%s,%s", ErrStructureProjection, declaration.PackagePath, declaration.OwnerType, declaration.Surface, declaration.Path, previous.FactID, declaration.FactID)
		}
		bucket[declaration.Path] = declaration
		index.witness.IndexEntries++
	}
	surfaceKeys := make([]string, 0, len(surfaces))
	for key := range surfaces {
		surfaceKeys = append(surfaceKeys, key)
	}
	sort.Strings(surfaceKeys)
	for _, key := range surfaceKeys {
		surface := surfaces[key]
		if surface.OriginDeclID == "" {
			continue
		}
		origin, exists := declarations[surface.OriginDeclID]
		if !exists {
			return nil, fmt.Errorf("%w: surface %s origin declaration %s is absent", ErrStructureProjection, key, surface.OriginDeclID)
		}
		if _, duplicate := index.origins[key]; duplicate {
			return nil, fmt.Errorf("%w: ambiguous surface origin %s", ErrStructureProjection, key)
		}
		index.origins[key] = origin
	}
	return index, nil
}

func structureOwnerBucketKey(packagePath, rootType, surface string) string {
	return packagePath + "\x00" + rootType + "\x00" + surface
}

func (index *structureOwnerIndex) owner(packagePath, rootType, surface, path string) (DeclarationInfo, bool) {
	index.witness.OwnerQueries++
	bucket := index.buckets[structureOwnerBucketKey(packagePath, rootType, surface)]
	if bucket == nil || path == "" {
		return DeclarationInfo{}, false
	}
	probes := 1
	index.witness.PrefixVisits++
	if declaration, ok := bucket[path]; ok {
		if probes > index.witness.MaxPrefixVisits {
			index.witness.MaxPrefixVisits = probes
		}
		return declaration, true
	}
	for prefixEnd := len(path) - 1; prefixEnd > 0; prefixEnd-- {
		switch path[prefixEnd] {
		case '.', '[', '{', '(', '*', '<':
			probes++
			index.witness.PrefixVisits++
			if declaration, ok := bucket[path[:prefixEnd]]; ok {
				if probes > index.witness.MaxPrefixVisits {
					index.witness.MaxPrefixVisits = probes
				}
				return declaration, true
			}
		}
	}
	if probes > index.witness.MaxPrefixVisits {
		index.witness.MaxPrefixVisits = probes
	}
	return DeclarationInfo{}, false
}

func (p *structureProjector) projectShape(shape TypeShapeInfo) error {
	pkg := p.packages[shape.PackagePath]
	if pkg == nil {
		return fmt.Errorf("%w: shape package %s is not loaded", ErrStructureProjection, shape.PackagePath)
	}
	if _, ok := p.rootDeclarations[shape.PackagePath+"\x00"+shape.Name]; !ok {
		return fmt.Errorf("%w: shape %s.%s has no root declaration", ErrStructureProjection, shape.PackagePath, shape.Name)
	}
	rootSurface := typeSurfaceID(shape.PackagePath, shape.Name)
	if shape.Facts.Owner != shape.PackagePath || shape.Facts.Surface != rootSurface {
		return fmt.Errorf("%w: shape context drift for %s.%s", ErrStructureProjection, shape.PackagePath, shape.Name)
	}
	if _, ok := p.surfaces[shape.PackagePath+"\x00"+rootSurface]; !ok {
		return fmt.Errorf("%w: missing root surface for %s.%s", ErrStructureProjection, shape.PackagePath, shape.Name)
	}
	for _, fact := range shape.Facts.Fields {
		owner, surfaceID, err := p.shapeOwner(shape, fact.Surface, fact.Path)
		if err != nil {
			return err
		}
		if err := p.validatePosition(pkg, fact.Position); err != nil {
			return err
		}
		p.addField(StructureField{DeclarationID: owner.FactID, SurfaceID: surfaceID, Embedded: fact.Embedded})
	}
	for _, fact := range shape.Facts.Containers {
		owner, surfaceID, err := p.shapeOwner(shape, fact.Surface, fact.Path)
		if err != nil {
			return err
		}
		if err := p.addContainer(owner.FactID, surfaceID, fact); err != nil {
			return err
		}
	}
	for _, fact := range shape.Facts.References {
		owner, surfaceID, err := p.shapeOwner(shape, fact.Surface, fact.Path)
		if err != nil {
			return err
		}
		if err := p.validatePosition(pkg, fact.Position); err != nil {
			return err
		}
		if err := p.addReference(owner.FactID, surfaceID, fact); err != nil {
			return err
		}
	}
	for _, fact := range shape.Facts.Cycles {
		owner, surfaceID, err := p.shapeOwner(shape, fact.Surface, fact.Path)
		if err != nil {
			return err
		}
		p.addCycle(StructureCycle{DeclarationID: owner.FactID, SurfaceID: surfaceID, Path: fact.Path, Type: fact.Type})
	}
	// Surface roots are owned by SurfaceInfo. They produce no duplicate row,
	// but their nonzero provenance still must be valid and fail closed.
	for _, fact := range shape.Facts.SurfaceRoots {
		if _, ok := p.surfaces[shape.PackagePath+"\x00"+fact.Surface]; !ok {
			return fmt.Errorf("%w: missing surface %s", ErrStructureProjection, fact.Surface)
		}
		if err := p.validatePosition(pkg, fact.Position); err != nil {
			return err
		}
	}
	return nil
}

func (p *structureProjector) shapeOwner(shape TypeShapeInfo, surface, path string) (DeclarationInfo, string, error) {
	owner, err := p.ownerForPath(shape.PackagePath, shape.Name, surface, path)
	if err != nil {
		return DeclarationInfo{}, "", err
	}
	surfaceInfo, ok := p.surfaces[shape.PackagePath+"\x00"+surface]
	if !ok || surfaceInfo.FactID == "" {
		return DeclarationInfo{}, "", fmt.Errorf("%w: missing surface identity %s", ErrStructureProjection, surface)
	}
	return owner, surfaceInfo.FactID, nil
}

func (p *structureProjector) projectDeclarationTypes(inv declarationInventory) error {
	objects := make(map[string]types.Object)
	for object, declarationIDs := range inv.byObject {
		for _, id := range declarationIDs {
			objects[id] = object
		}
	}
	ids := make([]string, 0)
	for _, declaration := range inv.Declarations {
		switch declaration.Kind {
		case "func", "method", "var", "const":
			ids = append(ids, declaration.FactID)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		declaration, ok := p.declarations[id]
		if !ok {
			return fmt.Errorf("%w: declaration %s disappeared", ErrStructureProjection, id)
		}
		object := objects[id]
		if object == nil || object.Type() == nil {
			return fmt.Errorf("%w: declaration %s has no typed object", ErrStructureProjection, id)
		}
		pkg := p.packages[declaration.PackagePath]
		if pkg == nil {
			return fmt.Errorf("%w: declaration package %s is not loaded", ErrStructureProjection, declaration.PackagePath)
		}
		facts, err := WalkType(object.Type(), TypeWalkOptions{Owner: declaration.PackagePath, Surface: declarationSurface(declaration), Mode: WalkModeReference})
		if err != nil {
			return fmt.Errorf("%w: walk declaration %s: %v", ErrStructureProjection, id, err)
		}
		for _, fact := range facts.Containers {
			if err := p.addContainer(id, "", fact); err != nil {
				return err
			}
		}
		for _, fact := range facts.References {
			if err := p.validatePosition(pkg, fact.Position); err != nil {
				return err
			}
			if err := p.addReference(id, "", fact); err != nil {
				return err
			}
		}
		for _, fact := range facts.Cycles {
			p.addCycle(StructureCycle{DeclarationID: id, Path: fact.Path, Type: fact.Type})
		}
	}
	return nil
}

func (p *structureProjector) validatePosition(pkg *packages.Package, position token.Pos) error {
	if position == token.NoPos {
		return nil
	}
	if pkg == nil || pkg.Fset == nil {
		return fmt.Errorf("%w: nonzero position without FileSet: %d", ErrStructureProjection, position)
	}
	located := pkg.Fset.PositionFor(position, true)
	if located.Filename == "" || located.Line <= 0 || located.Column <= 0 {
		return fmt.Errorf("%w: invalid nonzero position %d", ErrStructureProjection, position)
	}
	if source, ok := repoRelative(p.root, filepath.Clean(located.Filename)); !ok || source == "" {
		return fmt.Errorf("%w: nonzero position escapes repository: %s", ErrStructureProjection, located.Filename)
	}
	return nil
}

func methodDeclarationTargets(inv declarationInventory, declarations map[string]DeclarationInfo) (map[string]DeclarationInfo, error) {
	result := make(map[string]DeclarationInfo)
	objectsByID := make(map[string]types.Object)
	for object, ids := range inv.byObject {
		for _, id := range ids {
			if previous, exists := objectsByID[id]; exists && previous != object {
				return nil, fmt.Errorf("%w: declaration %s has multiple source objects", ErrStructureProjection, id)
			}
			objectsByID[id] = object
		}
	}
	ids := make([]string, 0)
	for id, declaration := range declarations {
		if declaration.Kind == "interface-method" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		declaration := declarations[id]
		method, ok := objectsByID[id].(*types.Func)
		if !ok || method.Type() == nil {
			return nil, fmt.Errorf("%w: interface method %s has no typed source object", ErrStructureProjection, id)
		}
		receiver := declarationMethodReceiver(declaration)
		key := methodObjectKey(method, receiver, "")
		if key == "" || receiver == "" {
			return nil, fmt.Errorf("%w: interface method %s has no stable identity", ErrStructureProjection, declaration.FactID)
		}
		if previous, exists := result[key]; exists && previous.FactID != declaration.FactID {
			return nil, fmt.Errorf("%w: method object key collision %q declarations=%s,%s", ErrStructureProjection, key, previous.FactID, declaration.FactID)
		}
		result[key] = declaration
	}
	return result, nil
}

func declarationMethodReceiver(declaration DeclarationInfo) string {
	if declaration.PackagePath == "" || declaration.OwnerType == "" {
		return ""
	}
	return declaration.PackagePath + "." + declaration.OwnerType
}

func (p *structureProjector) addContainer(declarationID, surfaceID string, fact ContainerFact) error {
	if fact.Element == "" && fact.Kind != "map" {
		return fmt.Errorf("%w: container element is empty: %+v", ErrStructureProjection, fact)
	}
	if fact.Kind == "array" && fact.ArrayLen < 0 {
		return fmt.Errorf("%w: array length is negative: %+v", ErrStructureProjection, fact)
	}
	switch fact.Kind {
	case "array":
		p.addArray(StructureArray{DeclarationID: declarationID, SurfaceID: surfaceID, Path: fact.Path, Element: fact.Element, Length: fact.ArrayLen})
	case "slice":
		p.addSlice(StructureSlice{DeclarationID: declarationID, SurfaceID: surfaceID, Path: fact.Path, Element: fact.Element})
	case "map":
		if fact.Key == "" || fact.Value == "" {
			return fmt.Errorf("%w: map key/value is empty: %+v", ErrStructureProjection, fact)
		}
		p.addMap(StructureMap{DeclarationID: declarationID, SurfaceID: surfaceID, Path: fact.Path, Key: fact.Key, Value: fact.Value})
	case "chan":
		if fact.Direction == "" {
			return fmt.Errorf("%w: channel direction is empty: %+v", ErrStructureProjection, fact)
		}
		p.addChannel(StructureChannel{DeclarationID: declarationID, SurfaceID: surfaceID, Path: fact.Path, Element: fact.Element, Direction: fact.Direction})
	default:
		return fmt.Errorf("%w: unknown typed container kind %q", ErrStructureProjection, fact.Kind)
	}
	return nil
}

func (p *structureProjector) addReference(declarationID, surfaceID string, fact ReferenceFact) error {
	switch fact.Kind {
	case "named", "named-origin":
		if fact.NamedPackagePath == "" || fact.NamedName == "" {
			return fmt.Errorf("%w: named reference has no exact target: %+v", ErrStructureProjection, fact)
		}
		targetID := ""
		if target, ok := p.rootDeclarations[fact.NamedPackagePath+"\x00"+fact.NamedName]; ok {
			targetID = target.FactID
		}
		if targetID == "" {
			if _, internal := p.packages[fact.NamedPackagePath]; internal {
				return fmt.Errorf("%w: internal named target %s.%s has no declaration", ErrStructureProjection, fact.NamedPackagePath, fact.NamedName)
			}
		}
		p.addNamed(StructureNamedReference{
			DeclarationID: declarationID, SurfaceID: surfaceID, Path: fact.Path,
			TargetDeclID: targetID, TargetPackagePath: fact.NamedPackagePath, TargetName: fact.NamedName,
			Origin: fact.Kind == "named-origin",
		})
		if p.owners != nil {
			p.owners.witness.ReferenceJoins++
		}
		return nil
	case "method", "method-reference":
		if fact.MethodKey == "" || fact.MethodPackagePath == "" || fact.MethodName == "" || fact.Type == "" || fact.MethodReceiver == "" {
			return fmt.Errorf("%w: method reference has no exact object key: %+v", ErrStructureProjection, fact)
		}
		target, exists := p.methodTargets[fact.MethodKey]
		if exists && target.Kind != "interface-method" {
			return fmt.Errorf("%w: method reference target is not an interface declaration: %+v", ErrStructureProjection, target)
		}
		targetID := ""
		if exists {
			targetID = target.FactID
		}
		if !exists {
			if _, internal := p.packages[fact.MethodPackagePath]; internal {
				return fmt.Errorf("%w: internal method target %s.%s has no declaration", ErrStructureProjection, fact.MethodPackagePath, fact.MethodName)
			}
		}
		p.addMethod(StructureMethodReference{
			DeclarationID: declarationID, SurfaceID: surfaceID, Path: fact.Path,
			TargetDeclID: targetID, TargetPackagePath: fact.MethodPackagePath, TargetName: fact.MethodName,
			MethodKey: fact.MethodKey, Type: fact.Type, Receiver: fact.MethodReceiver,
		})
		if p.owners != nil {
			p.owners.witness.ReferenceJoins++
		}
		return nil
	default:
		disposition, ok := otherDisposition(fact.Kind)
		if !ok {
			return fmt.Errorf("%w: unadmitted reference disposition %q", ErrStructureProjection, fact.Kind)
		}
		if fact.Type == "" {
			return fmt.Errorf("%w: structural reference type is empty: %+v", ErrStructureProjection, fact)
		}
		if disposition == OtherInterfaceMethod && (fact.MethodKey == "" || fact.MethodSource == "" || fact.MethodReceiver != "") {
			return fmt.Errorf("%w: anonymous interface method has no exact source identity: %+v", ErrStructureProjection, fact)
		}
		p.addOther(StructureOtherReference{DeclarationID: declarationID, SurfaceID: surfaceID, Path: fact.Path, Disposition: disposition, Type: fact.Type})
		return nil
	}
}

func otherDisposition(kind string) (OtherReferenceDisposition, bool) {
	switch kind {
	case "pointer":
		return OtherPointer, true
	case "anonymous-struct":
		return OtherAnonymousStruct, true
	case "member":
		return OtherMember, true
	case "signature":
		return OtherSignature, true
	case "tuple":
		return OtherTuple, true
	case "type-param":
		return OtherTypeParameter, true
	case "union-term":
		return OtherUnionTerm, true
	case "union-tilde-term":
		return OtherUnionTildeTerm, true
	case "interface-method-local":
		return OtherInterfaceMethod, true
	default:
		return 0, false
	}
}

func (p *structureProjector) ownerForPath(packagePath, rootType, surface, path string) (DeclarationInfo, error) {
	if p.owners != nil {
		if owner, ok := p.owners.owner(packagePath, rootType, surface, path); ok {
			return owner, nil
		}
		if origin, ok := p.owners.origins[packagePath+"\x00"+surface]; ok {
			return origin, nil
		}
	}
	root, ok := p.rootDeclarations[packagePath+"\x00"+rootType]
	if !ok {
		return DeclarationInfo{}, fmt.Errorf("%w: no owner for %s.%s surface=%q path=%q", ErrStructureProjection, packagePath, rootType, surface, path)
	}
	return root, nil
}

func (p *structureProjector) addField(row StructureField) {
	row.FactID = structureFieldFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.Fields = append(p.projection.Fields, row)
}

func (p *structureProjector) addArray(row StructureArray) {
	row.FactID = structureArrayFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.Arrays = append(p.projection.Arrays, row)
}

func (p *structureProjector) addSlice(row StructureSlice) {
	row.FactID = structureSliceFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.Slices = append(p.projection.Slices, row)
}

func (p *structureProjector) addMap(row StructureMap) {
	row.FactID = structureMapFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.Maps = append(p.projection.Maps, row)
}

func (p *structureProjector) addChannel(row StructureChannel) {
	row.FactID = structureChannelFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.Channels = append(p.projection.Channels, row)
}

func (p *structureProjector) addNamed(row StructureNamedReference) {
	row.FactID = structureNamedReferenceFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.NamedReferences = append(p.projection.NamedReferences, row)
}

func (p *structureProjector) addMethod(row StructureMethodReference) {
	row.FactID = structureMethodReferenceFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.MethodReferences = append(p.projection.MethodReferences, row)
}

func (p *structureProjector) addOther(row StructureOtherReference) {
	row.FactID = structureOtherReferenceFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.OtherReferences = append(p.projection.OtherReferences, row)
}

func (p *structureProjector) addCycle(row StructureCycle) {
	row.FactID = structureCycleFactID(row)
	if _, exists := p.seen[row.FactID]; exists {
		return
	}
	p.seen[row.FactID] = struct{}{}
	p.projection.Cycles = append(p.projection.Cycles, row)
}

func (p *structureProjector) sort() {
	sort.Slice(p.projection.Fields, func(i, j int) bool {
		return compare(p.projection.Fields[i].DeclarationID, p.projection.Fields[j].DeclarationID, p.projection.Fields[i].SurfaceID, p.projection.Fields[j].SurfaceID, strconv.FormatBool(p.projection.Fields[i].Embedded), strconv.FormatBool(p.projection.Fields[j].Embedded), p.projection.Fields[i].FactID, p.projection.Fields[j].FactID) < 0
	})
	sort.Slice(p.projection.Arrays, func(i, j int) bool {
		return compare(p.projection.Arrays[i].DeclarationID, p.projection.Arrays[j].DeclarationID, p.projection.Arrays[i].SurfaceID, p.projection.Arrays[j].SurfaceID, p.projection.Arrays[i].Path, p.projection.Arrays[j].Path, p.projection.Arrays[i].Element, p.projection.Arrays[j].Element, strconv.FormatInt(p.projection.Arrays[i].Length, 10), strconv.FormatInt(p.projection.Arrays[j].Length, 10), p.projection.Arrays[i].FactID, p.projection.Arrays[j].FactID) < 0
	})
	sort.Slice(p.projection.Slices, func(i, j int) bool {
		return compare(p.projection.Slices[i].DeclarationID, p.projection.Slices[j].DeclarationID, p.projection.Slices[i].SurfaceID, p.projection.Slices[j].SurfaceID, p.projection.Slices[i].Path, p.projection.Slices[j].Path, p.projection.Slices[i].Element, p.projection.Slices[j].Element, p.projection.Slices[i].FactID, p.projection.Slices[j].FactID) < 0
	})
	sort.Slice(p.projection.Maps, func(i, j int) bool {
		return compare(p.projection.Maps[i].DeclarationID, p.projection.Maps[j].DeclarationID, p.projection.Maps[i].SurfaceID, p.projection.Maps[j].SurfaceID, p.projection.Maps[i].Path, p.projection.Maps[j].Path, p.projection.Maps[i].Key, p.projection.Maps[j].Key, p.projection.Maps[i].Value, p.projection.Maps[j].Value, p.projection.Maps[i].FactID, p.projection.Maps[j].FactID) < 0
	})
	sort.Slice(p.projection.Channels, func(i, j int) bool {
		return compare(p.projection.Channels[i].DeclarationID, p.projection.Channels[j].DeclarationID, p.projection.Channels[i].SurfaceID, p.projection.Channels[j].SurfaceID, p.projection.Channels[i].Path, p.projection.Channels[j].Path, p.projection.Channels[i].Element, p.projection.Channels[j].Element, p.projection.Channels[i].Direction, p.projection.Channels[j].Direction, p.projection.Channels[i].FactID, p.projection.Channels[j].FactID) < 0
	})
	sort.Slice(p.projection.NamedReferences, func(i, j int) bool {
		return compare(p.projection.NamedReferences[i].DeclarationID, p.projection.NamedReferences[j].DeclarationID, p.projection.NamedReferences[i].SurfaceID, p.projection.NamedReferences[j].SurfaceID, p.projection.NamedReferences[i].Path, p.projection.NamedReferences[j].Path, p.projection.NamedReferences[i].TargetDeclID, p.projection.NamedReferences[j].TargetDeclID, p.projection.NamedReferences[i].TargetPackagePath, p.projection.NamedReferences[j].TargetPackagePath, p.projection.NamedReferences[i].TargetName, p.projection.NamedReferences[j].TargetName, strconv.FormatBool(p.projection.NamedReferences[i].Origin), strconv.FormatBool(p.projection.NamedReferences[j].Origin), p.projection.NamedReferences[i].FactID, p.projection.NamedReferences[j].FactID) < 0
	})
	sort.Slice(p.projection.MethodReferences, func(i, j int) bool {
		return compare(p.projection.MethodReferences[i].DeclarationID, p.projection.MethodReferences[j].DeclarationID, p.projection.MethodReferences[i].SurfaceID, p.projection.MethodReferences[j].SurfaceID, p.projection.MethodReferences[i].Path, p.projection.MethodReferences[j].Path, p.projection.MethodReferences[i].TargetDeclID, p.projection.MethodReferences[j].TargetDeclID, p.projection.MethodReferences[i].TargetPackagePath, p.projection.MethodReferences[j].TargetPackagePath, p.projection.MethodReferences[i].TargetName, p.projection.MethodReferences[j].TargetName, p.projection.MethodReferences[i].MethodKey, p.projection.MethodReferences[j].MethodKey, p.projection.MethodReferences[i].Type, p.projection.MethodReferences[j].Type, p.projection.MethodReferences[i].Receiver, p.projection.MethodReferences[j].Receiver, p.projection.MethodReferences[i].FactID, p.projection.MethodReferences[j].FactID) < 0
	})
	sort.Slice(p.projection.OtherReferences, func(i, j int) bool {
		return compare(p.projection.OtherReferences[i].DeclarationID, p.projection.OtherReferences[j].DeclarationID, p.projection.OtherReferences[i].SurfaceID, p.projection.OtherReferences[j].SurfaceID, p.projection.OtherReferences[i].Path, p.projection.OtherReferences[j].Path, strconv.Itoa(int(p.projection.OtherReferences[i].Disposition)), strconv.Itoa(int(p.projection.OtherReferences[j].Disposition)), p.projection.OtherReferences[i].Type, p.projection.OtherReferences[j].Type, p.projection.OtherReferences[i].FactID, p.projection.OtherReferences[j].FactID) < 0
	})
	sort.Slice(p.projection.Cycles, func(i, j int) bool {
		return compare(p.projection.Cycles[i].DeclarationID, p.projection.Cycles[j].DeclarationID, p.projection.Cycles[i].SurfaceID, p.projection.Cycles[j].SurfaceID, p.projection.Cycles[i].Path, p.projection.Cycles[j].Path, p.projection.Cycles[i].Type, p.projection.Cycles[j].Type, p.projection.Cycles[i].FactID, p.projection.Cycles[j].FactID) < 0
	})
}

func structureFieldFactID(row StructureField) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-field-v2")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, strconv.FormatBool(row.Embedded))
	return "structure-field-v2-" + hex.EncodeToString(hash.Sum(nil))
}

func structureArrayFactID(row StructureArray) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-array-v2")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, row.Path)
	writeProjectionPart(hash, row.Element)
	writeProjectionPart(hash, strconv.FormatInt(row.Length, 10))
	return "structure-array-v2-" + hex.EncodeToString(hash.Sum(nil))
}

func structureSliceFactID(row StructureSlice) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-slice-v2")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, row.Path)
	writeProjectionPart(hash, row.Element)
	return "structure-slice-v2-" + hex.EncodeToString(hash.Sum(nil))
}

func structureMapFactID(row StructureMap) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-map-v2")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, row.Path)
	writeProjectionPart(hash, row.Key)
	writeProjectionPart(hash, row.Value)
	return "structure-map-v2-" + hex.EncodeToString(hash.Sum(nil))
}

func structureChannelFactID(row StructureChannel) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-channel-v2")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, row.Path)
	writeProjectionPart(hash, row.Element)
	writeProjectionPart(hash, row.Direction)
	return "structure-channel-v2-" + hex.EncodeToString(hash.Sum(nil))
}

func structureNamedReferenceFactID(row StructureNamedReference) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-named-reference-v2")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, row.Path)
	writeProjectionPart(hash, row.TargetDeclID)
	writeProjectionPart(hash, row.TargetPackagePath)
	writeProjectionPart(hash, row.TargetName)
	writeProjectionPart(hash, strconv.FormatBool(row.Origin))
	return "structure-named-reference-v2-" + hex.EncodeToString(hash.Sum(nil))
}

func structureMethodReferenceFactID(row StructureMethodReference) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-method-reference-v3")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, row.Path)
	writeProjectionPart(hash, row.TargetDeclID)
	writeProjectionPart(hash, row.TargetPackagePath)
	writeProjectionPart(hash, row.TargetName)
	writeProjectionPart(hash, row.MethodKey)
	writeProjectionPart(hash, row.Type)
	writeProjectionPart(hash, row.Receiver)
	return "structure-method-reference-v3-" + hex.EncodeToString(hash.Sum(nil))
}

func structureOtherReferenceFactID(row StructureOtherReference) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-other-reference-v3")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, row.Path)
	writeProjectionPart(hash, strconv.Itoa(int(row.Disposition)))
	writeProjectionPart(hash, row.Type)
	return "structure-other-reference-v3-" + hex.EncodeToString(hash.Sum(nil))
}

func structureCycleFactID(row StructureCycle) string {
	hash := sha256.New()
	writeProjectionPart(hash, "link-structure-cycle-v2")
	writeProjectionPart(hash, row.DeclarationID)
	writeProjectionPart(hash, row.SurfaceID)
	writeProjectionPart(hash, row.Path)
	writeProjectionPart(hash, row.Type)
	return "structure-cycle-v2-" + hex.EncodeToString(hash.Sum(nil))
}
