package transformer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

const GlobalBoundarySchema = "go-lua.transformer.global-boundary/v1"
const globalContentIdentitySchema = "go-lua.transformer.global-content/v1"

// GlobalRootClass separates immutable artifact-backed roots from roots whose
// value can change without an exact dependency invalidation edge.
type GlobalRootClass uint8

const (
	GlobalRootImmutableStdlib GlobalRootClass = iota + 1
	GlobalRootRuntimeModule
	GlobalRootImportedAlias
	GlobalRootMutableUnknown
)

func (c GlobalRootClass) valid() bool {
	return c >= GlobalRootImmutableStdlib && c <= GlobalRootMutableUnknown
}

// GlobalBoundaryCompleteness is an explicit closed-set proof. Only Complete
// catalogs may instantiate RootGlobal bindings.
type GlobalBoundaryCompleteness uint8

const (
	GlobalBoundaryUnknown GlobalBoundaryCompleteness = iota
	GlobalBoundaryIncomplete
	GlobalBoundaryComplete
)

// GlobalContentID is an exact content identity of the stdlib/runtime/module
// artifact supplying an immutable global. It is never a path, version counter,
// source span, or process-local ordinal.
type GlobalContentID [sha256.Size]byte

func (id GlobalContentID) zero() bool { return id == GlobalContentID{} }

// DeriveGlobalContentID derives the immutable identity of one global artifact
// binding from the complete body-boundary environment that gave the name its
// meaning. Class and name are domain-separated components, not a SHA(name)
// surrogate. Mutable roots cannot claim immutable content identity.
func DeriveGlobalContentID(environment [sha256.Size]byte, class GlobalRootClass, stableName string) (GlobalContentID, error) {
	if environment == [sha256.Size]byte{} {
		return GlobalContentID{}, errors.New("transformer: global content identity has no boundary environment")
	}
	if !class.valid() || class == GlobalRootMutableUnknown {
		return GlobalContentID{}, fmt.Errorf("transformer: global content identity has invalid immutable class %d", class)
	}
	if stableName == "" || strings.TrimSpace(stableName) != stableName {
		return GlobalContentID{}, errors.New("transformer: global content identity has invalid stable name")
	}
	var canonical bytes.Buffer
	if err := writeGlobalString(&canonical, globalContentIdentitySchema); err != nil {
		return GlobalContentID{}, err
	}
	canonical.Write(environment[:])
	canonical.WriteByte(byte(class))
	if err := writeGlobalString(&canonical, stableName); err != nil {
		return GlobalContentID{}, err
	}
	return sha256.Sum256(canonical.Bytes()), nil
}

// GlobalRootDescriptor classifies one referenced global root. StableName is
// diagnostic/canonical identity (e.g. stdlib name or imported alias path);
// semantic reuse additionally requires ContentID equality.
type GlobalRootDescriptor struct {
	Symbol     symbol.ID
	Class      GlobalRootClass
	StableName string
	ContentID  GlobalContentID
}

// GlobalBoundary is an immutable, canonical referenced-global census.
type GlobalBoundary struct {
	completeness GlobalBoundaryCompleteness
	roots        []GlobalRootDescriptor
	index        map[symbol.ID]uint32
	dependencies []GlobalContentID
	canonical    []byte
	digest       GlobalContentID
}

// SealGlobalBoundary validates and seals the exact referenced-root set.
// MutableUnknown roots are retained as explicit negative evidence and make
// instantiation fail closed; omitting them from a Complete set is unsound.
func SealGlobalBoundary(completeness GlobalBoundaryCompleteness, input []GlobalRootDescriptor) (GlobalBoundary, error) {
	if completeness > GlobalBoundaryComplete {
		return GlobalBoundary{}, fmt.Errorf("transformer: global boundary has invalid completeness %d", completeness)
	}
	roots := append([]GlobalRootDescriptor(nil), input...)
	sort.Slice(roots, func(left, right int) bool { return roots[left].Symbol < roots[right].Symbol })
	index := make(map[symbol.ID]uint32, len(roots))
	dependencySet := make(map[GlobalContentID]struct{})
	for position, root := range roots {
		if root.Symbol == 0 || !root.Class.valid() || root.StableName == "" || strings.TrimSpace(root.StableName) != root.StableName {
			return GlobalBoundary{}, fmt.Errorf("transformer: invalid global boundary root at index %d", position)
		}
		if _, duplicate := index[root.Symbol]; duplicate {
			return GlobalBoundary{}, fmt.Errorf("transformer: duplicate global boundary symbol %d", root.Symbol)
		}
		index[root.Symbol] = uint32(position)
		if root.Class == GlobalRootMutableUnknown {
			if !root.ContentID.zero() {
				return GlobalBoundary{}, fmt.Errorf("transformer: mutable global %d cannot claim artifact content identity", root.Symbol)
			}
			continue
		}
		if root.ContentID.zero() {
			return GlobalBoundary{}, fmt.Errorf("transformer: immutable global %d has no dependency content identity", root.Symbol)
		}
		dependencySet[root.ContentID] = struct{}{}
	}
	dependencies := make([]GlobalContentID, 0, len(dependencySet))
	for dependency := range dependencySet {
		dependencies = append(dependencies, dependency)
	}
	sort.Slice(dependencies, func(left, right int) bool {
		return bytes.Compare(dependencies[left][:], dependencies[right][:]) < 0
	})
	boundary := GlobalBoundary{completeness: completeness, roots: roots, index: index, dependencies: dependencies}
	canonical, err := encodeGlobalBoundary(boundary)
	if err != nil {
		return GlobalBoundary{}, err
	}
	boundary.canonical = canonical
	boundary.digest = sha256.Sum256(boundary.canonical)
	return boundary, nil
}

func (b GlobalBoundary) Completeness() GlobalBoundaryCompleteness { return b.completeness }
func (b GlobalBoundary) ContentID() GlobalContentID               { return b.digest }
func (b GlobalBoundary) CanonicalBytes() []byte                   { return append([]byte(nil), b.canonical...) }
func (b GlobalBoundary) Roots() []GlobalRootDescriptor {
	return append([]GlobalRootDescriptor(nil), b.roots...)
}
func (b GlobalBoundary) Dependencies() []GlobalContentID {
	return append([]GlobalContentID(nil), b.dependencies...)
}

func (b GlobalBoundary) RootIndex(target symbol.ID) (uint32, bool) {
	index, ok := b.index[target]
	return index, ok
}

// GlobalRootBinding is the project-owned value for one exact descriptor
// content identity. Paths are optional for value-only uses; when supplied they
// must be the same unversioned canonical root.
type GlobalRootBinding struct {
	Symbol    symbol.ID
	ContentID GlobalContentID
	Value     product.Value
	Path      pathdom.Path
	HasPath   bool
}

// GlobalBoundaryBindings is a detached dense RootGlobal binding vector.
type GlobalBoundaryBindings struct {
	values []product.Value
	paths  []pathdom.Path
}

func (b GlobalBoundaryBindings) Values() []product.Value {
	return append([]product.Value(nil), b.values...)
}
func (b GlobalBoundaryBindings) Paths() []pathdom.Path {
	out := make([]pathdom.Path, len(b.paths))
	for index, path := range b.paths {
		out[index] = path.Clone()
	}
	return out
}

// InstantiateGlobalBoundary validates content identities and emits exact dense
// vectors. Unknown/incomplete sets, mutable roots, omissions, duplicates,
// stale content, and foreign paths all fail without a partial result.
func InstantiateGlobalBoundary(boundary GlobalBoundary, input []GlobalRootBinding) (GlobalBoundaryBindings, error) {
	if boundary.completeness != GlobalBoundaryComplete || len(boundary.canonical) == 0 {
		return GlobalBoundaryBindings{}, errors.New("transformer: global boundary referenced-root set is not complete")
	}
	if len(input) != len(boundary.roots) {
		return GlobalBoundaryBindings{}, fmt.Errorf("transformer: got %d global bindings, want %d", len(input), len(boundary.roots))
	}
	bySymbol := make(map[symbol.ID]GlobalRootBinding, len(input))
	for _, binding := range input {
		if binding.Symbol == 0 {
			return GlobalBoundaryBindings{}, errors.New("transformer: zero global binding symbol")
		}
		if _, duplicate := bySymbol[binding.Symbol]; duplicate {
			return GlobalBoundaryBindings{}, fmt.Errorf("transformer: duplicate global binding symbol %d", binding.Symbol)
		}
		bySymbol[binding.Symbol] = binding
	}
	out := GlobalBoundaryBindings{values: make([]product.Value, len(boundary.roots)), paths: make([]pathdom.Path, len(boundary.roots))}
	for index, descriptor := range boundary.roots {
		if descriptor.Class == GlobalRootMutableUnknown {
			return GlobalBoundaryBindings{}, fmt.Errorf("transformer: global %d is mutable or unknown", descriptor.Symbol)
		}
		binding, ok := bySymbol[descriptor.Symbol]
		if !ok {
			return GlobalBoundaryBindings{}, fmt.Errorf("transformer: global %d has no exact binding", descriptor.Symbol)
		}
		if binding.ContentID != descriptor.ContentID {
			return GlobalBoundaryBindings{}, fmt.Errorf("transformer: global %d dependency content identity changed", descriptor.Symbol)
		}
		if binding.HasPath {
			if binding.Path.Symbol != descriptor.Symbol || binding.Path.Root != descriptor.StableName || binding.Path.Version != 0 || len(binding.Path.Segments) != 0 {
				return GlobalBoundaryBindings{}, fmt.Errorf("transformer: global %d path is not its canonical unversioned root", descriptor.Symbol)
			}
			out.paths[index] = binding.Path.Clone()
		}
		out.values[index] = binding.Value
	}
	return out, nil
}

func encodeGlobalBoundary(boundary GlobalBoundary) ([]byte, error) {
	var out bytes.Buffer
	if err := writeGlobalString(&out, GlobalBoundarySchema); err != nil {
		return nil, err
	}
	out.WriteByte(byte(boundary.completeness))
	if err := writeGlobalCount(&out, len(boundary.roots)); err != nil {
		return nil, err
	}
	for _, root := range boundary.roots {
		writeGlobalUint64(&out, uint64(root.Symbol))
		out.WriteByte(byte(root.Class))
		if err := writeGlobalString(&out, root.StableName); err != nil {
			return nil, err
		}
		out.Write(root.ContentID[:])
	}
	if err := writeGlobalCount(&out, len(boundary.dependencies)); err != nil {
		return nil, err
	}
	for _, dependency := range boundary.dependencies {
		out.Write(dependency[:])
	}
	return out.Bytes(), nil
}

func writeGlobalString(out *bytes.Buffer, value string) error {
	if err := writeGlobalCount(out, len(value)); err != nil {
		return err
	}
	out.WriteString(value)
	return nil
}

func writeGlobalCount(out *bytes.Buffer, value int) error {
	if value < 0 || uint64(value) > uint64(^uint32(0)) {
		return errors.New("transformer: global boundary canonical length exceeds uint32")
	}
	writeGlobalUint32(out, uint32(value))
	return nil
}

func writeGlobalUint32(out *bytes.Buffer, value uint32) {
	var buffer [4]byte
	binary.BigEndian.PutUint32(buffer[:], value)
	out.Write(buffer[:])
}

func writeGlobalUint64(out *bytes.Buffer, value uint64) {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], value)
	out.Write(buffer[:])
}
