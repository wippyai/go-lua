// Package generator contains the cold, production-only Link ownership scanner.
package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	gomodule "golang.org/x/mod/module"
	"golang.org/x/tools/go/packages"
)

const (
	moduleImportPath = "github.com/wippyai/go-lua"
	linkImportPath   = moduleImportPath + "/program/link"
)

// BuildContext is the hermetic selection contract shared by every
// go/packages load. It intentionally contains values, not host paths, so the
// later gate can retain the same semantic context without importing ambient
// process state.
type BuildContext struct {
	GOWORK             string
	GOENV              string
	GOTOOLCHAIN        string
	GOOS               string
	GOARCH             string
	CGOEnabled         string
	GOFLAGS            string
	BuildTags          string
	GOEXPERIMENT       string
	GODEBUG            string
	GOAMD64            string
	GOARM64            string
	GOARM              string
	GO386              string
	GOMIPS             string
	GOMIPS64           string
	GOPPC64            string
	GORISCV64          string
	GOWASM             string
	GO111MODULE        string
	GOPROXY            string
	GOSUMDB            string
	Toolchain          string
	GoExecutableDigest string
	CCompilerDigest    string
	CXXCompilerDigest  string
	ARCompilerDigest   string

	// These paths are used only to construct the pinned go/packages
	// environment. They are deliberately excluded from key so fingerprints do
	// not become host-path identities.
	goExecutable string
	goRoot       string
	goPath       string
	goModCache   string
	goCache      string
	home         string
	path         string
	cc           string
	cxx          string
	ar           string
}

func (context BuildContext) environment() []string {
	// Keep this list intentionally explicit. Inheriting an arbitrary process
	// environment lets GOEXPERIMENT, GODEBUG, architecture tuning, compiler
	// flags, module proxies, or PATH silently change the selected program.
	env := []string{
		"HOME=" + context.home,
		"PATH=" + context.path,
		"GOROOT=" + context.goRoot,
		"GOPATH=" + context.goPath,
		"GOMODCACHE=" + context.goModCache,
		"GOCACHE=" + context.goCache,
		"GOWORK=" + context.GOWORK,
		"GOENV=" + context.GOENV,
		"GOTOOLCHAIN=" + context.GOTOOLCHAIN,
		"GOOS=" + context.GOOS,
		"GOARCH=" + context.GOARCH,
		"CGO_ENABLED=" + context.CGOEnabled,
		"GOFLAGS=" + context.GOFLAGS,
		"GOEXPERIMENT=" + context.GOEXPERIMENT,
		"GODEBUG=" + context.GODEBUG,
		"GOAMD64=" + context.GOAMD64,
		"GOARM64=" + context.GOARM64,
		"GOARM=" + context.GOARM,
		"GO386=" + context.GO386,
		"GOMIPS=" + context.GOMIPS,
		"GOMIPS64=" + context.GOMIPS64,
		"GOPPC64=" + context.GOPPC64,
		"GORISCV64=" + context.GORISCV64,
		"GOWASM=" + context.GOWASM,
		"GO111MODULE=" + context.GO111MODULE,
		"GOPROXY=" + context.GOPROXY,
		"GOSUMDB=" + context.GOSUMDB,
		"GONOSUMDB=",
		"GOPRIVATE=",
		"GONOPROXY=",
		"GOINSECURE=",
		"GOPACKAGESDRIVER=off",
		"CGO_CFLAGS=",
		"CGO_CPPFLAGS=",
		"CGO_CXXFLAGS=",
		"CGO_LDFLAGS=",
	}
	if context.CGOEnabled == "1" {
		env = append(env, "CC="+context.cc, "CXX="+context.cxx, "AR="+context.ar)
	}
	sort.Strings(env)
	return env
}

func (context BuildContext) buildFlags() []string {
	if context.BuildTags == "" {
		return nil
	}
	return []string{"-tags=" + context.BuildTags}
}

func (context BuildContext) key() string {
	return strings.Join([]string{
		"build-context-v2", context.GOWORK, context.GOENV, context.GOTOOLCHAIN,
		context.GOOS, context.GOARCH, context.CGOEnabled, context.GOFLAGS,
		context.BuildTags, context.GOEXPERIMENT, context.GODEBUG, context.GOAMD64,
		context.GOARM64, context.GOARM, context.GO386, context.GOMIPS,
		context.GOMIPS64, context.GOPPC64, context.GORISCV64, context.GOWASM,
		context.GO111MODULE, context.GOPROXY, context.GOSUMDB, context.Toolchain,
		context.GoExecutableDigest, context.CCompilerDigest, context.CXXCompilerDigest,
		context.ARCompilerDigest,
	}, "\x00")
}

func canonicalCGOEnabled() string { return "0" }

var probedGoEnvironmentNames = []string{
	"AR", "CC", "CXX", "GOCACHE", "GODEBUG", "GO386", "GOAMD64", "GOARM", "GOARM64",
	"GOEXPERIMENT", "GO111MODULE", "GOMIPS", "GOMIPS64", "GOMODCACHE", "GOPPC64",
	"GOPROXY", "GOPATH", "GORISCV64", "GOROOT", "GOSUMDB", "GOWASM", "GOVERSION",
}

func canonicalBuildContextChecked() (BuildContext, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: resolve go home: %v", ErrTypeCheck, err)
	}
	home, err = canonicalAbsolutePath(home)
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: canonicalize go home: %v", ErrTypeCheck, err)
	}
	goRoot, err := canonicalAbsolutePath(runtime.GOROOT())
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: canonicalize runtime GOROOT: %v", ErrTypeCheck, err)
	}
	// The loader must not let the caller's PATH choose a different Go binary.
	// Bind the executable to the running toolchain's canonical GOROOT/bin first,
	// then retain its digest and version as the path-free build identity.
	goExecutable := filepath.Join(goRoot, "bin", "go")
	if runtime.GOOS == "windows" {
		goExecutable += ".exe"
	}
	goExecutable, err = canonicalAbsolutePath(goExecutable)
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: resolve pinned go executable: %v", ErrTypeCheck, err)
	}
	// Build and module caches are operational scratch space, not semantic
	// identities. When the bounded runner supplies a private cache, honor that
	// explicit location so go/packages never reads stale host cache entries.
	probe := canonicalGoProbeEnvironment(goExecutable, home, goRoot, os.Getenv("GOCACHE"), os.Getenv("GOMODCACHE"))
	values, err := queryGoEnvironment(goExecutable, probe, probedGoEnvironmentNames)
	if err != nil {
		return BuildContext{}, err
	}
	toolchain := values["GOVERSION"]
	if toolchain == "" || toolchain != runtime.Version() {
		return BuildContext{}, fmt.Errorf("%w: go executable reports %q, running toolchain is %q", ErrTypeCheck, toolchain, runtime.Version())
	}
	goRoot, err = canonicalAbsolutePath(values["GOROOT"])
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: canonicalize GOROOT: %v", ErrTypeCheck, err)
	}
	if runtimeRoot, rootErr := canonicalAbsolutePath(runtime.GOROOT()); rootErr != nil || runtimeRoot != goRoot {
		return BuildContext{}, fmt.Errorf("%w: go executable GOROOT %q does not match running GOROOT %q", ErrTypeCheck, goRoot, runtime.GOROOT())
	}
	goPath := values["GOPATH"]
	goModCache := values["GOMODCACHE"]
	goCache := values["GOCACHE"]
	if goPath == "" || goModCache == "" || goCache == "" || goCache == "off" {
		return BuildContext{}, fmt.Errorf("%w: go executable did not provide stable GOPATH/cache roots", ErrTypeCheck)
	}
	goPath, err = canonicalAbsolutePath(filepath.SplitList(goPath)[0])
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: canonicalize GOPATH: %v", ErrTypeCheck, err)
	}
	goModCache, err = canonicalAbsolutePath(goModCache)
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: canonicalize GOMODCACHE: %v", ErrTypeCheck, err)
	}
	goCache, err = canonicalAbsolutePath(goCache)
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: canonicalize GOCACHE: %v", ErrTypeCheck, err)
	}
	cgo := canonicalCGOEnabled()
	context := BuildContext{
		GOWORK: "off", GOENV: "off", GOTOOLCHAIN: "local", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		CGOEnabled: cgo, GOFLAGS: "", BuildTags: "", GOEXPERIMENT: values["GOEXPERIMENT"], GODEBUG: values["GODEBUG"],
		GOAMD64: values["GOAMD64"], GOARM64: values["GOARM64"], GOARM: values["GOARM"], GO386: values["GO386"],
		GOMIPS: values["GOMIPS"], GOMIPS64: values["GOMIPS64"], GOPPC64: values["GOPPC64"], GORISCV64: values["GORISCV64"],
		GOWASM: values["GOWASM"], GO111MODULE: "on", GOPROXY: values["GOPROXY"], GOSUMDB: values["GOSUMDB"],
		Toolchain: toolchain, goExecutable: goExecutable, goRoot: goRoot, goPath: goPath, goModCache: goModCache,
		goCache: goCache, home: home,
	}
	context.GoExecutableDigest, err = fileDigest(goExecutable)
	if err != nil {
		return BuildContext{}, fmt.Errorf("%w: hash go executable: %v", ErrTypeCheck, err)
	}
	if context.CGOEnabled == "1" {
		context.cc, context.CCompilerDigest, err = resolveCompiler(values["CC"])
		if err != nil {
			return BuildContext{}, fmt.Errorf("%w: resolve C compiler: %v", ErrTypeCheck, err)
		}
		context.cxx, context.CXXCompilerDigest, err = resolveCompiler(values["CXX"])
		if err != nil {
			return BuildContext{}, fmt.Errorf("%w: resolve C++ compiler: %v", ErrTypeCheck, err)
		}
		context.ar, context.ARCompilerDigest, err = resolveCompiler(values["AR"])
		if err != nil {
			return BuildContext{}, fmt.Errorf("%w: resolve archiver: %v", ErrTypeCheck, err)
		}
	}
	dirs := []string{filepath.Dir(goExecutable)}
	for _, compiler := range []string{context.cc, context.cxx, context.ar} {
		if compiler != "" {
			dirs = append(dirs, filepath.Dir(compiler))
		}
	}
	context.path = canonicalPathList(dirs)
	if err := context.verify(); err != nil {
		return BuildContext{}, err
	}
	return context, nil
}

func canonicalGoProbeEnvironment(goExecutable, home, goRoot, goCache, goModCache string) []string {
	cgo := canonicalCGOEnabled()
	env := []string{
		"HOME=" + home, "PATH=" + filepath.Dir(goExecutable), "GOROOT=" + goRoot, "GOWORK=off", "GOENV=off",
		"GOTOOLCHAIN=local", "GOOS=" + runtime.GOOS, "GOARCH=" + runtime.GOARCH,
		"CGO_ENABLED=" + cgo, "GOFLAGS=", "GOEXPERIMENT=", "GODEBUG=",
	}
	if goCache != "" {
		env = append(env, "GOCACHE="+goCache)
	}
	if goModCache != "" {
		env = append(env, "GOMODCACHE="+goModCache)
	}
	sort.Strings(env)
	return env
}

func queryGoEnvironment(goExecutable string, environment []string, names []string) (map[string]string, error) {
	args := append([]string{"env", "-json"}, names...)
	command := exec.Command(goExecutable, args...)
	command.Env = append([]string(nil), environment...)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: query go environment: %v", ErrTypeCheck, err)
	}
	values := make(map[string]string)
	if err := json.Unmarshal(output, &values); err != nil {
		return nil, fmt.Errorf("%w: decode go environment: %v", ErrTypeCheck, err)
	}
	return values, nil
}

func resolveExecutable(name, pathValue string) (string, error) {
	if pathValue == "" {
		return "", fmt.Errorf("%s is not available on PATH", name)
	}
	for _, directory := range strings.Split(pathValue, string(os.PathListSeparator)) {
		if directory == "" {
			directory = "."
		}
		candidate := filepath.Join(directory, name)
		if runtime.GOOS == "windows" && filepath.Ext(candidate) == "" {
			candidate += ".exe"
		}
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return canonicalAbsolutePath(candidate)
	}
	return "", fmt.Errorf("%s is not available on PATH", name)
}

func canonicalAbsolutePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	physical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Clean(physical))
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func resolveCompiler(name string) (string, string, error) {
	if name == "" {
		return "", "", fmt.Errorf("compiler name is empty")
	}
	path := name
	if !filepath.IsAbs(path) {
		var err error
		path, err = resolveExecutable(name, os.Getenv("PATH"))
		if err != nil {
			return "", "", err
		}
	} else {
		var err error
		path, err = canonicalAbsolutePath(path)
		if err != nil {
			return "", "", err
		}
	}
	digest, err := fileDigest(path)
	if err != nil {
		return "", "", err
	}
	return path, digest, nil
}

func canonicalPathList(paths []string) string {
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path != "" {
			seen[path] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for path := range seen {
		result = append(result, path)
	}
	sort.Strings(result)
	return strings.Join(result, string(os.PathListSeparator))
}

func (context BuildContext) verify() error {
	if context.goExecutable == "" || context.GoExecutableDigest == "" {
		return fmt.Errorf("%w: go executable identity is incomplete", ErrTypeCheck)
	}
	current, err := fileDigest(context.goExecutable)
	if err != nil || current != context.GoExecutableDigest {
		return fmt.Errorf("%w: bound go executable changed", ErrTypeCheck)
	}
	bound, err := resolveExecutable("go", context.path)
	if err != nil || bound != context.goExecutable {
		return fmt.Errorf("%w: go PATH binding is ambiguous", ErrTypeCheck)
	}
	values, err := queryGoEnvironment(context.goExecutable, context.environment(), []string{"GOVERSION", "GOROOT"})
	if err != nil {
		return err
	}
	if values["GOVERSION"] != context.Toolchain {
		return fmt.Errorf("%w: bound go version changed from %q to %q", ErrTypeCheck, context.Toolchain, values["GOVERSION"])
	}
	root, err := canonicalAbsolutePath(values["GOROOT"])
	if err != nil || root != context.goRoot {
		return fmt.Errorf("%w: bound go GOROOT changed", ErrTypeCheck)
	}
	for _, pair := range [][2]string{{context.cc, context.CCompilerDigest}, {context.cxx, context.CXXCompilerDigest}, {context.ar, context.ARCompilerDigest}} {
		if pair[0] == "" {
			continue
		}
		current, err := fileDigest(pair[0])
		if err != nil || current != pair[1] {
			return fmt.Errorf("%w: bound compiler changed", ErrTypeCheck)
		}
	}
	return nil
}

var (
	ErrTypeCheck       = errors.New("link ownership: production package type check failed")
	ErrImportCycle     = errors.New("link ownership: production import graph contains a cycle")
	ErrManifestMissing = errors.New("link ownership: authored manifest is missing")
	ErrManifestStale   = errors.New("link ownership: generated output is stale")
	ErrInvalidMode     = errors.New("link ownership: invalid gate mode")
	ErrFinalBlocked    = errors.New("link ownership: final mode blocked")
)

type Mode string

const (
	ModeInventory Mode = "inventory"
	ModeFinal     Mode = "final"
)

// TypeShapeInfo is the scanner-owned structural description of one declared
// named type. PackagePath is the declaration owner in Go source; semantic
// ownership and the final public surface are assigned only by the authored
// ownership manifests. Facts never open a named child: every child has its own
// TypeShapeInfo and is joined through an exact ReferenceFact.
type TypeShapeInfo struct {
	PackagePath string
	Name        string
	Facts       TypeWalkFacts
}

type ImportEdge struct {
	From, To, SourceFile string
	Line, Column         int
}

type UseSite struct {
	PackagePath            string
	SourceFile             string
	Line, Column           int
	Symbol, Evidence, Type string
	TargetDeclID           string
	// Role is a closed typed context for the use.  It is part of the stable
	// fact identity; evidence text is explanatory provenance and cannot stand
	// in for this classification.
	Role UseRole
	// AliasChain is the ordered, canonical package/type-name chain crossed
	// before TargetDeclID. It contains aliases only; the terminal declaration
	// is represented by TargetDeclID. Defined wrapper types never acquire an
	// alias chain and therefore cannot be attributed to the Link family.
	AliasChain []string
	// FactID is a cold, reproducible identity for this exact typed-use fact.
	// It is deliberately not a runtime or analysis identity.
	FactID string
}

// UseRole is intentionally closed.  A new role must be added here and to the
// scanner/population laws before it can enter catalog.schema.
type UseRole string

const (
	CallCallee   UseRole = "call-callee"
	Reference    UseRole = "reference"
	TypeInstance UseRole = "type-instance"
)

func (role UseRole) valid() bool {
	switch role {
	case CallCallee, Reference, TypeInstance:
		return true
	default:
		return false
	}
}

type PackageInfo struct {
	Path, Name, Directory string
}

// ProductionSource is the single retained package-to-source relation. Path
// is the canonical physical source identity relative to the repository root.
// Logical symlink spellings are validated by the loader and never retained as
// a parallel source authority.
type ProductionSource struct {
	PackagePath string
	Path        string
}

type ModuleInfo struct {
	// Path/Version/Sum/GoMod identify the module requested by the build.
	Path, Version, Sum, GoMod string
	// Resolved* identify the module actually selected after replacement. Keep
	// both planes: distinct originals A=>B and C=>B must never collapse.
	ResolvedPath, ResolvedVersion, ResolvedSum, ResolvedGoMod string
	// ResolvedContentDigest commits local replacement inputs without leaking
	// cache-specific absolute paths into the module identity.
	ResolvedContentDigest string
}

type RootInventory struct {
	PackagePath string
	SourceDir   string
}

type SourceInventory struct {
	Packages          []PackageInfo
	ProductionSources []ProductionSource
}

// TypeInventory retains each typed semantic relation once. TypeShapeInfo is
// deliberately absent: raw walker shapes are scanner-local construction
// evidence whose complete projection is validated before this value exists.
type TypeInventory struct {
	Declarations []DeclarationInfo
	Exposures    []MethodExposure
	Surfaces     []SurfaceInfo
	Structure    StructureProjection
}

type DependencyInventory struct {
	ImportEdges []ImportEdge
	Modules     []ModuleInfo
}

type BuildInventory struct {
	SourceDigest [sha256.Size]byte
	Fingerprint  string
	Context      BuildContext
}

type ScanResult struct {
	Root           RootInventory
	Sources        SourceInventory
	Types          TypeInventory
	Dependencies   DependencyInventory
	Uses           []UseSite
	Build          BuildInventory
	ProductionOnly bool
}

// Scan loads compiled production packages, and obtains all ownership evidence
// from go/types. In particular, source files are CompiledGoFiles rather than a
// directory walk: files excluded by build constraints and files under
// __legacy are consequently absent from the inventory.
func Scan(root string) (ScanResult, error) {
	return scanFamily(root, linkImportPath)
}

// scanFamily is kept private so focused laws can exercise the loader against
// a tiny synthetic module without coupling production callers to scanner
// internals. The normal entry point always uses linkImportPath.
func scanFamily(root, familyPrefix string) (ScanResult, error) {
	root, err := canonicalRoot(root)
	if err != nil {
		return ScanResult{}, err
	}
	return scanFamilyAtCanonicalRoot(root, familyPrefix)
}

// scanFamilyAtCanonicalRoot is the single-root internal entry point. Gates
// that need to bind filesystem inputs to the same physical repository call it
// after canonicalRoot instead of canonicalizing a second spelling mid-run.
func scanFamilyAtCanonicalRoot(root, familyPrefix string) (ScanResult, error) {
	workspace, err := loadWorkspaceDetails(root, familyPrefix)
	if err != nil {
		return ScanResult{}, err
	}
	loaded := workspace.Selected
	byPath := make(map[string]*packages.Package, len(loaded))
	for _, pkg := range loaded {
		if pkg != nil && pkg.PkgPath != "" {
			byPath[pkg.PkgPath] = pkg
		}
	}
	linkPkg := byPath[familyPrefix]
	if linkPkg == nil {
		return ScanResult{}, fmt.Errorf("%w: %s is not loaded", ErrTypeCheck, familyPrefix)
	}
	family := familyPackages(byPath, familyPrefix)
	if len(family) == 0 {
		return ScanResult{}, fmt.Errorf("%w: link package family is empty", ErrTypeCheck)
	}
	productionSources, err := workspaceProductionSources(root, byPath)
	if err != nil {
		return ScanResult{}, err
	}
	sourceDir, err := productionSourceDirectory(productionSources, linkPkg.PkgPath)
	if err != nil {
		return ScanResult{}, err
	}
	directByPath, err := directImportInventory(byPath, root)
	if err != nil {
		return ScanResult{}, err
	}
	digest, err := productionManifestDigest(root, productionSources)
	if err != nil {
		return ScanResult{}, err
	}
	graph, err := workspaceImportGraph(byPath, root, directByPath)
	if err != nil {
		return ScanResult{}, err
	}
	if err := validateImportGraph(graph); err != nil {
		return ScanResult{}, err
	}
	importEdges, err := canonicalImportEdges(directByPath)
	if err != nil {
		return ScanResult{}, err
	}
	packagesInfo, err := familyInventory(family, productionSources)
	if err != nil {
		return ScanResult{}, err
	}
	typeShapes, err := familyTypeShapes(family)
	if err != nil {
		return ScanResult{}, err
	}
	if err := requireLinkShape(linkPkg, typeShapes); err != nil {
		return ScanResult{}, err
	}
	declarations, err := inventoryDeclarations(root, family, typeShapes)
	if err != nil {
		return ScanResult{}, err
	}
	exposures, err := methodExposureProjection(root, family, declarations)
	if err != nil {
		return ScanResult{}, err
	}
	surfaces, err := surfaceProjection(root, family, typeShapes, declarations)
	if err != nil {
		return ScanResult{}, err
	}
	structure, err := structureProjection(root, family, typeShapes, declarations, surfaces)
	if err != nil {
		return ScanResult{}, err
	}
	if err := validateStructureProjectionClosure(typeShapes, surfaces, structure); err != nil {
		return ScanResult{}, err
	}
	uses, err := typedUseInventory(root, byPath, familyPrefix, declarations)
	if err != nil {
		return ScanResult{}, err
	}
	modules, err := resolvedModules(root, workspace.Closure)
	if err != nil {
		return ScanResult{}, err
	}
	fingerprint, err := buildFingerprint(digest, modules, workspace.Context)
	if err != nil {
		return ScanResult{}, err
	}
	return ScanResult{
		Root: RootInventory{PackagePath: linkPkg.PkgPath, SourceDir: sourceDir},
		Sources: SourceInventory{
			Packages:          packagesInfo,
			ProductionSources: productionSources,
		},
		Types: TypeInventory{
			Declarations: declarations.Declarations,
			Exposures:    exposures,
			Surfaces:     surfaces,
			Structure:    structure,
		},
		Dependencies: DependencyInventory{ImportEdges: importEdges, Modules: modules},
		Uses:         uses,
		Build: BuildInventory{
			SourceDigest: digest,
			Fingerprint:  fingerprint,
			Context:      workspace.Context,
		},
		ProductionOnly: true,
	}, nil
}

// canonicalRoot establishes the physical repository boundary once. All
// selected-file and source-position identities below are physical paths
// relative to this root; a logical symlink path is never allowed to create a
// second source identity.
func canonicalRoot(root string) (string, error) {
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	physical, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return "", fmt.Errorf("%w: resolve repository root %s: %v", ErrTypeCheck, absolute, err)
	}
	return filepath.Abs(filepath.Clean(physical))
}

type productionFileSet struct {
	Files []productionFilePair
}

type productionFilePair struct {
	Logical  string
	Physical string
}

type workspaceLoad struct {
	Selected []*packages.Package
	Closure  map[string]*packages.Package
	Context  BuildContext
}

// loadWorkspace is intentionally a live go/packages load. Tests=false and
// NeedCompiledGoFiles make the build-selected production file set authoritative.
func loadWorkspace(root, familyPrefix string) ([]*packages.Package, error) {
	workspace, err := loadWorkspaceDetails(root, familyPrefix)
	if err != nil {
		return nil, err
	}
	return workspace.Selected, nil
}

func loadPackagesHermetic(context BuildContext, config *packages.Config, patterns ...string) ([]*packages.Package, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: nil go/packages config", ErrTypeCheck)
	}
	if err := context.verify(); err != nil {
		return nil, err
	}
	if err := verifyAmbientGoBinding(context); err != nil {
		return nil, err
	}
	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, err
	}
	if err := context.verify(); err != nil {
		return nil, err
	}
	if err := verifyAmbientGoBinding(context); err != nil {
		return nil, err
	}
	return loaded, nil
}

func verifyAmbientGoBinding(context BuildContext) error {
	ambient, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("%w: ambient go executable is unavailable: %v", ErrTypeCheck, err)
	}
	return verifyGoExecutablePath(context, ambient)
}

func verifyGoExecutablePath(context BuildContext, candidate string) error {
	ambient := candidate
	ambient, err := canonicalAbsolutePath(ambient)
	if err != nil {
		return fmt.Errorf("%w: resolve ambient go executable: %v", ErrTypeCheck, err)
	}
	if ambient != context.goExecutable {
		return fmt.Errorf("%w: ambient go executable %q is not verified runtime executable %q", ErrTypeCheck, ambient, context.goExecutable)
	}
	return nil
}

func loadWorkspaceDetails(root, familyPrefix string) (workspaceLoad, error) {
	var empty workspaceLoad
	context, err := canonicalBuildContextChecked()
	if err != nil {
		return empty, err
	}
	metadataConfig := &packages.Config{
		Dir: root,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedModule,
		Env:        context.environment(),
		BuildFlags: context.buildFlags(),
		Tests:      false,
	}
	loaded, err := loadPackagesHermetic(context, metadataConfig, "./...")
	if err != nil {
		return empty, fmt.Errorf("%w: %v", ErrTypeCheck, err)
	}
	indexed, err := indexPackagesChecked(loaded)
	if err != nil {
		return empty, err
	}
	if err := validateIndexedImports(indexed); err != nil {
		return empty, err
	}
	if len(indexed) == 0 {
		return empty, fmt.Errorf("%w: workspace has no production packages", ErrTypeCheck)
	}
	selected, err := reverseImportClosure(root, familyPrefix, indexed)
	if err != nil {
		return empty, err
	}
	if selected[familyPrefix] == nil {
		return empty, fmt.Errorf("%w: %s is not present in live metadata", ErrTypeCheck, familyPrefix)
	}
	paths := make([]string, 0, len(selected))
	for path := range selected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	typedConfig := &packages.Config{
		Dir: root,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedImports | packages.NeedModule,
		Env:        context.environment(),
		BuildFlags: context.buildFlags(),
		Tests:      false,
	}
	typedRoots, err := loadPackagesHermetic(context, typedConfig, paths...)
	if err != nil {
		return empty, fmt.Errorf("%w: %v", ErrTypeCheck, err)
	}
	typedByPath, err := indexPackagesChecked(typedRoots)
	if err != nil {
		return empty, err
	}
	if err := validateIndexedImports(typedByPath); err != nil {
		return empty, err
	}
	result := make([]*packages.Package, 0, len(paths))
	for _, path := range paths {
		pkg := typedByPath[path]
		if pkg == nil {
			return empty, fmt.Errorf("%w: selected package %q missing typed load", ErrTypeCheck, path)
		}
		if len(pkg.Errors) != 0 {
			return empty, fmt.Errorf("%w: %s: %s", ErrTypeCheck, path, formatPackageErrors(pkg.Errors))
		}
		if pkg.Types == nil || pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
			return empty, fmt.Errorf("%w: incomplete production package %q", ErrTypeCheck, path)
		}
		metadataSet, err := validateProductionFileSet(root, selected[path])
		if err != nil {
			return empty, err
		}
		typedSet, err := validateProductionFileSet(root, pkg)
		if err != nil {
			return empty, err
		}
		if !sameFileSets(metadataSet, typedSet) {
			return empty, fmt.Errorf("%w: metadata/typed compiled file mismatch for %s", ErrTypeCheck, path)
		}
		result = append(result, pkg)
	}
	return workspaceLoad{Selected: result, Closure: typedByPath, Context: context}, nil
}

// reverseImportClosure selects every in-root production package that can
// reach the Link family through the live import graph. Metadata loading with
// ./... gives us all workspace roots and their dependency closure; selection
// must then walk the graph backwards, not just inspect each package's direct
// imports once. This is what retains consumer -> bridge(alias) -> Link.
//
// Out-of-root dependencies are traversed as graph connectors but are never
// selected as scanner packages. An in-root package with even one out-of-root
// compiled file is rejected at the reverse boundary: silently treating it as
// an external connector would make the ownership evidence incomplete.
// Excluded in-repository packages are omitted leaves/connectors. They carry a
// taint while traversed backwards; only a non-excluded in-root production
// ancestor turns that omitted evidence into a rejection.
func reverseImportClosure(root, familyPrefix string, indexed map[string]*packages.Package) (map[string]*packages.Package, error) {
	if err := validateIndexedImports(indexed); err != nil {
		return nil, err
	}
	reverse := make(map[string][]string)
	for path, pkg := range indexed {
		if pkg == nil || pkg.PkgPath == "" {
			continue
		}
		for imported := range pkg.Imports {
			reverse[imported] = append(reverse[imported], path)
		}
	}
	for imported := range reverse {
		sort.Strings(reverse[imported])
	}

	selected := make(map[string]*packages.Package)
	type reverseState struct {
		path    string
		tainted bool
	}
	visited := make(map[reverseState]struct{})
	queue := make([]reverseState, 0)
	enqueue := func(path string, tainted bool) {
		state := reverseState{path: path, tainted: tainted}
		if _, exists := visited[state]; exists {
			return
		}
		visited[state] = struct{}{}
		queue = append(queue, state)
	}
	seedPaths := make([]string, 0)
	for path := range indexed {
		if path != familyPrefix && !strings.HasPrefix(path, familyPrefix+"/") {
			continue
		}
		seedPaths = append(seedPaths, path)
	}
	sort.Strings(seedPaths)
	for _, path := range seedPaths {
		pkg := indexed[path]
		if pkg == nil {
			return nil, fmt.Errorf("%w: nil Link-family package %q", ErrTypeCheck, path)
		}
		if excludedPackage(path, pkg) {
			return nil, fmt.Errorf("%w: excluded Link-family package %q lies on the selected root", ErrTypeCheck, path)
		}
		// Family roots are mandatory. Validate them immediately even when a
		// malformed package has no in-root file to make it eligible below.
		if _, err := validateProductionFiles(root, pkg); err != nil {
			return nil, err
		}
		selected[path] = pkg
		enqueue(path, false)
	}
	if selected[familyPrefix] == nil {
		return nil, fmt.Errorf("%w: %s is not present in live metadata", ErrTypeCheck, familyPrefix)
	}

	for head := 0; head < len(queue); head++ {
		state := queue[head]
		for _, importerPath := range reverse[state.path] {
			importer := indexed[importerPath]
			if importer == nil {
				return nil, fmt.Errorf("%w: reverse closure importer %q is missing indexed metadata", ErrTypeCheck, importerPath)
			}
			if excludedPackage(importerPath, importer) {
				// Excluded source is never selected, typed, or digested. It is
				// still traversed so a production ancestor cannot silently
				// depend on omitted ownership evidence.
				enqueue(importerPath, true)
				continue
			}
			if !hasInRootCompiledFile(root, importer) {
				if packageDirectoryInRoot(root, importer) {
					// An in-root module package with no usable compiled
					// source is a selected-source hole, not an external
					// connector that may be skipped.
					if _, err := validateProductionFiles(root, importer); err != nil {
						return nil, err
					}
					return nil, fmt.Errorf("%w: in-root importer %q has no in-root compiled source", ErrTypeCheck, importerPath)
				}
				// A dependency wholly outside the repository is a connector,
				// not a selected scanner package. Continue through it so an
				// in-root consumer above it is still discovered. An excluded
				// taint remains attached; module identity is committed by the
				// resolved-module plane separately.
				enqueue(importerPath, state.tainted)
				continue
			}
			if _, err := validateProductionFiles(root, importer); err != nil {
				return nil, err
			}
			if state.tainted {
				return nil, fmt.Errorf("%w: excluded importer path reaches production package %q on a reverse path to %s", ErrTypeCheck, importerPath, familyPrefix)
			}
			selected[importerPath] = importer
			enqueue(importerPath, false)
		}
	}
	return selected, nil
}

func validateIndexedImports(indexed map[string]*packages.Package) error {
	for path, pkg := range indexed {
		if pkg == nil {
			return fmt.Errorf("%w: nil indexed package %q", ErrTypeCheck, path)
		}
		if path == "" || pkg.PkgPath != path {
			return fmt.Errorf("%w: indexed package key %q does not match package path %q", ErrTypeCheck, path, pkg.PkgPath)
		}
		for importPath, imported := range pkg.Imports {
			if imported == nil {
				return fmt.Errorf("%w: nil imported package object %q from %s", ErrTypeCheck, importPath, path)
			}
			if importPath == "" || imported.PkgPath != importPath {
				return fmt.Errorf("%w: import metadata mismatch %q from %s", ErrTypeCheck, importPath, path)
			}
			if indexed[importPath] == nil {
				return fmt.Errorf("%w: imported package %q from %s is absent from indexed metadata", ErrTypeCheck, importPath, path)
			}
		}
	}
	return nil
}

// indexPackages retains the historical map-only helper used by the other
// cold projections. Invalid metadata is a hard failure rather than a silent
// nil/first-entry selection; the loader itself uses indexPackagesChecked to
// return the typed error to its caller.
func indexPackages(roots []*packages.Package) map[string]*packages.Package {
	indexed, err := indexPackagesChecked(roots)
	if err != nil {
		panic(err)
	}
	return indexed
}

func indexPackagesChecked(roots []*packages.Package) (map[string]*packages.Package, error) {
	indexed := make(map[string]*packages.Package)
	visited := make(map[*packages.Package]struct{})
	var visit func(*packages.Package) error
	visit = func(pkg *packages.Package) error {
		if pkg == nil {
			return fmt.Errorf("%w: nil imported package object", ErrTypeCheck)
		}
		if pkg.PkgPath == "" {
			return fmt.Errorf("%w: imported package has empty path", ErrTypeCheck)
		}
		if existing, ok := indexed[pkg.PkgPath]; ok {
			if !samePackageMetadata(existing, pkg) {
				return fmt.Errorf("%w: duplicate package metadata for %s", ErrTypeCheck, pkg.PkgPath)
			}
		} else {
			indexed[pkg.PkgPath] = pkg
		}
		if _, ok := visited[pkg]; ok {
			return nil
		}
		visited[pkg] = struct{}{}
		for importPath, imported := range pkg.Imports {
			if imported == nil {
				return fmt.Errorf("%w: nil imported package object %q from %s", ErrTypeCheck, importPath, pkg.PkgPath)
			}
			if importPath == "" || imported.PkgPath == "" || imported.PkgPath != importPath {
				return fmt.Errorf("%w: import metadata mismatch %q from %s", ErrTypeCheck, importPath, pkg.PkgPath)
			}
			if err := visit(imported); err != nil {
				return err
			}
		}
		return nil
	}
	for _, pkg := range roots {
		if err := visit(pkg); err != nil {
			return nil, err
		}
	}
	return indexed, nil
}

func samePackageMetadata(left, right *packages.Package) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.ID != right.ID || left.Name != right.Name || left.PkgPath != right.PkgPath || left.Dir != right.Dir ||
		!sameStringSet(left.GoFiles, right.GoFiles) || !sameStringSet(left.CompiledGoFiles, right.CompiledGoFiles) ||
		!sameStringSet(left.OtherFiles, right.OtherFiles) || !sameStringSet(left.IgnoredFiles, right.IgnoredFiles) ||
		!sameStringSet(left.EmbedFiles, right.EmbedFiles) || !sameStringSet(left.EmbedPatterns, right.EmbedPatterns) ||
		moduleMetadataKey(left.Module) != moduleMetadataKey(right.Module) {
		return false
	}
	if len(left.Imports) != len(right.Imports) {
		return false
	}
	for path, imported := range left.Imports {
		other := right.Imports[path]
		if imported == nil || other == nil || imported.PkgPath != other.PkgPath {
			return false
		}
	}
	return true
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for i := range leftCopy {
		if leftCopy[i] != rightCopy[i] {
			return false
		}
	}
	return true
}

func moduleMetadataKey(module *packages.Module) string {
	if module == nil {
		return ""
	}
	return strings.Join([]string{module.Path, module.Version, module.Dir, module.GoMod, module.GoVersion, strconv.FormatBool(module.Main), strconv.FormatBool(module.Indirect), moduleMetadataKey(module.Replace)}, "\x00")
}

func excludedPackage(path string, pkg *packages.Package) bool {
	if pkg == nil {
		return false
	}
	if path == moduleImportPath+"/__legacy" || strings.HasPrefix(path, moduleImportPath+"/__legacy/") ||
		path == moduleImportPath+"/analysis/test" || strings.HasPrefix(path, moduleImportPath+"/analysis/test/") ||
		path == moduleImportPath+"/program/testfixture" || strings.HasPrefix(path, moduleImportPath+"/program/testfixture/") {
		return true
	}
	for _, file := range pkg.CompiledGoFiles {
		slash := filepath.ToSlash(file)
		if strings.Contains(slash, "/__legacy/") || strings.HasSuffix(slash, "/__legacy") {
			return true
		}
	}
	return false
}

func hasInRootCompiledFile(root string, pkg *packages.Package) bool {
	if pkg == nil {
		return false
	}
	for _, file := range pkg.CompiledGoFiles {
		if _, ok := repoRelative(root, file); ok {
			return true
		}
	}
	return false
}

func packageDirectoryInRoot(root string, pkg *packages.Package) bool {
	if pkg == nil {
		return false
	}
	paths := make([]string, 0, 1+len(pkg.GoFiles))
	if pkg.Dir != "" {
		paths = append(paths, pkg.Dir)
	}
	paths = append(paths, pkg.GoFiles...)
	for _, path := range paths {
		physical, err := filepath.EvalSymlinks(filepath.Clean(path))
		if err != nil {
			continue
		}
		if !filepath.IsAbs(physical) {
			physical, err = filepath.Abs(physical)
			if err != nil {
				continue
			}
		}
		if info, err := os.Stat(physical); err == nil && !info.IsDir() {
			physical = filepath.Dir(physical)
		}
		rootPhysical, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err == nil {
			if _, ok := physicalRelative(rootPhysical, physical); ok {
				return true
			}
		}
	}
	return false
}

func familyPackages(byPath map[string]*packages.Package, prefix string) []*packages.Package {
	result := make([]*packages.Package, 0)
	for path, pkg := range byPath {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			result = append(result, pkg)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PkgPath < result[j].PkgPath })
	return result
}

func workspaceProductionSources(root string, byPath map[string]*packages.Package) ([]ProductionSource, error) {
	seen := make(map[string]string)
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		pkg := byPath[path]
		if pkg == nil {
			return nil, fmt.Errorf("%w: nil selected package %s", ErrTypeCheck, path)
		}
		if excludedPackage(path, pkg) {
			continue
		}
		files, err := validateProductionFiles(root, pkg)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			relative, ok := repoRelative(root, file)
			if !ok || relative == "" || filepath.IsAbs(relative) {
				return nil, fmt.Errorf("%w: selected source outside physical root %s", ErrTypeCheck, file)
			}
			if previous, duplicate := seen[relative]; duplicate {
				return nil, fmt.Errorf("%w: duplicate physical compiled source %s across selected packages %s and %s", ErrTypeCheck, relative, previous, path)
			}
			seen[relative] = path
		}
	}
	sources := make([]ProductionSource, 0, len(seen))
	for source, packagePath := range seen {
		sources = append(sources, ProductionSource{PackagePath: packagePath, Path: source})
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].PackagePath != sources[j].PackagePath {
			return sources[i].PackagePath < sources[j].PackagePath
		}
		return sources[i].Path < sources[j].Path
	})
	if len(sources) == 0 {
		return nil, fmt.Errorf("%w: no in-repository production files", ErrTypeCheck)
	}
	return sources, nil
}

func productionSourceDirectory(sources []ProductionSource, packagePath string) (string, error) {
	directory := ""
	found := false
	for _, source := range sources {
		if source.PackagePath != packagePath {
			continue
		}
		current := filepath.ToSlash(filepath.Dir(source.Path))
		if !found {
			directory, found = current, true
			continue
		}
		if directory != current {
			return "", fmt.Errorf("%w: package %s has production sources in multiple directories %s and %s", ErrTypeCheck, packagePath, directory, current)
		}
	}
	if !found {
		return "", fmt.Errorf("%w: package %s has no canonical production source", ErrTypeCheck, packagePath)
	}
	return directory, nil
}

func repoRelative(root, path string) (string, bool) {
	physicalRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", false
	}
	physicalPath, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", false
	}
	return physicalRelative(physicalRoot, physicalPath)
}

func physicalRelative(root, path string) (string, bool) {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func productionFiles(pkg *packages.Package, root string) ([]string, error) {
	return validateProductionFiles(root, pkg)
}

// validateProductionFiles is the single selected-package source boundary.
// CompiledGoFiles are the build-selected compiler inputs, so every one must
// be a production .go file and every one must resolve beneath the repository
// root. No filtering is allowed here: a mixed package is an evidence failure,
// not a reason to silently retain only the convenient files.
func validateProductionFiles(root string, pkg *packages.Package) ([]string, error) {
	set, err := validateProductionFileSet(root, pkg)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(set.Files))
	for _, pair := range set.Files {
		files = append(files, pair.Physical)
	}
	sort.Strings(files)
	return files, nil
}

func validateProductionFileSet(root string, pkg *packages.Package) (productionFileSet, error) {
	var empty productionFileSet
	if pkg == nil {
		return empty, fmt.Errorf("%w: nil production package", ErrTypeCheck)
	}
	if len(pkg.CompiledGoFiles) == 0 {
		return empty, fmt.Errorf("%w: no production source files in %s", ErrTypeCheck, pkg.PkgPath)
	}
	physicalRoot, err := canonicalRoot(root)
	if err != nil {
		return empty, err
	}
	set := productionFileSet{
		Files: make([]productionFilePair, 0, len(pkg.CompiledGoFiles)),
	}
	seenPhysical := make(map[string]string, len(pkg.CompiledGoFiles))
	for _, file := range pkg.CompiledGoFiles {
		if !isProductionGo(file) || isExcludedSourcePath(file) {
			return empty, fmt.Errorf("%w: non-production compiled file %s in %s", ErrTypeCheck, file, pkg.PkgPath)
		}
		logical := filepath.Clean(file)
		logicalAbsolute, err := filepath.Abs(logical)
		if err != nil {
			return empty, fmt.Errorf("%w: canonical logical compiled source %s in %s: %v", ErrTypeCheck, file, pkg.PkgPath, err)
		}
		logicalRel, logicalInside := physicalRelative(physicalRoot, logicalAbsolute)
		if !logicalInside {
			return empty, fmt.Errorf("%w: out-of-root logical compiled source %s in %s", ErrTypeCheck, file, pkg.PkgPath)
		}
		if isExcludedSourcePath(logicalRel) {
			return empty, fmt.Errorf("%w: excluded logical compiled file %s in %s", ErrTypeCheck, logicalRel, pkg.PkgPath)
		}
		physical, err := filepath.EvalSymlinks(logical)
		if err != nil {
			return empty, fmt.Errorf("%w: cannot resolve compiled source %s in %s: %v", ErrTypeCheck, file, pkg.PkgPath, err)
		}
		physical, err = filepath.Abs(filepath.Clean(physical))
		if err != nil {
			return empty, fmt.Errorf("%w: canonical compiled source %s in %s: %v", ErrTypeCheck, file, pkg.PkgPath, err)
		}
		physicalRel, ok := physicalRelative(physicalRoot, physical)
		if !ok {
			return empty, fmt.Errorf("%w: out-of-root production file %s in %s", ErrTypeCheck, file, pkg.PkgPath)
		}
		if isExcludedSourcePath(physicalRel) {
			return empty, fmt.Errorf("%w: excluded physical compiled file %s in %s", ErrTypeCheck, physicalRel, pkg.PkgPath)
		}
		info, err := os.Stat(physical)
		if err != nil {
			return empty, fmt.Errorf("%w: cannot stat compiled source %s in %s: %v", ErrTypeCheck, file, pkg.PkgPath, err)
		}
		if info.IsDir() {
			return empty, fmt.Errorf("%w: compiled source is a directory %s in %s", ErrTypeCheck, file, pkg.PkgPath)
		}
		if previous, duplicate := seenPhysical[physical]; duplicate {
			return empty, fmt.Errorf("%w: duplicate physical compiled source %s (also %s) in %s", ErrTypeCheck, physical, previous, pkg.PkgPath)
		}
		seenPhysical[physical] = logicalRel
		set.Files = append(set.Files, productionFilePair{Logical: logicalRel, Physical: physical})
	}
	sort.Slice(set.Files, func(i, j int) bool {
		if set.Files[i].Logical != set.Files[j].Logical {
			return set.Files[i].Logical < set.Files[j].Logical
		}
		return set.Files[i].Physical < set.Files[j].Physical
	})
	return set, nil
}

func sameFileSets(left, right productionFileSet) bool {
	if len(left.Files) != len(right.Files) {
		return false
	}
	for i := range left.Files {
		if left.Files[i] != right.Files[i] {
			return false
		}
	}
	return true
}

func familyInventory(family []*packages.Package, sources []ProductionSource) ([]PackageInfo, error) {
	result := make([]PackageInfo, 0, len(family))
	for _, pkg := range family {
		if pkg == nil || pkg.PkgPath == "" || pkg.Name == "" {
			return nil, fmt.Errorf("%w: incomplete family package metadata", ErrTypeCheck)
		}
		directory, err := productionSourceDirectory(sources, pkg.PkgPath)
		if err != nil {
			return nil, err
		}
		result = append(result, PackageInfo{Path: pkg.PkgPath, Name: pkg.Name, Directory: directory})
	}
	return result, nil
}

func familyTypeShapes(family []*packages.Package) ([]TypeShapeInfo, error) {
	result := make([]TypeShapeInfo, 0)
	for _, pkg := range family {
		if pkg == nil || pkg.Types == nil {
			return nil, fmt.Errorf("%w: family package has no type information", ErrTypeCheck)
		}
		for _, name := range pkg.Types.Scope().Names() {
			object, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok {
				continue
			}
			if object.IsAlias() {
				surface := typeSurfaceID(pkg.PkgPath, name)
				facts, err := WalkType(object.Type(), TypeWalkOptions{Owner: pkg.PkgPath, Surface: surface, Mode: WalkModeReference, OpenNamedRoot: false})
				if err != nil {
					return nil, fmt.Errorf("%w: walk alias %s.%s: %v", ErrTypeCheck, pkg.PkgPath, name, err)
				}
				if facts.Owner != pkg.PkgPath || facts.Surface != surface {
					return nil, fmt.Errorf("%w: alias structural surface context drift for %s.%s", ErrTypeCheck, pkg.PkgPath, name)
				}
				result = append(result, TypeShapeInfo{PackagePath: pkg.PkgPath, Name: name, Facts: facts})
				continue
			}
			if _, ok := object.Type().(*types.Named); !ok {
				return nil, fmt.Errorf("%w: declared type %s.%s is not named", ErrTypeCheck, pkg.PkgPath, name)
			}
			surface := typeSurfaceID(pkg.PkgPath, name)
			options := TypeWalkOptions{Owner: pkg.PkgPath, Surface: surface, Mode: WalkModeReference, OpenNamedRoot: true}
			if named, ok := object.Type().(*types.Named); ok {
				if _, stored := named.Underlying().(*types.Struct); stored {
					options.Mode = WalkModeState
					options.OpenNamedRoot = false
				}
			}
			facts, err := WalkType(object.Type(), options)
			if err != nil {
				return nil, fmt.Errorf("%w: walk %s.%s: %v", ErrTypeCheck, pkg.PkgPath, name, err)
			}
			if facts.Owner != pkg.PkgPath || facts.Surface != surface {
				return nil, fmt.Errorf("%w: structural surface context drift for %s.%s", ErrTypeCheck, pkg.PkgPath, name)
			}
			result = append(result, TypeShapeInfo{PackagePath: pkg.PkgPath, Name: name, Facts: facts})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PackagePath != result[j].PackagePath {
			return result[i].PackagePath < result[j].PackagePath
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func requireLinkShape(pkg *packages.Package, shapes []TypeShapeInfo) error {
	if pkg == nil || pkg.Types == nil {
		return fmt.Errorf("%w: Link package has no type scope", ErrTypeCheck)
	}
	object := pkg.Types.Scope().Lookup("Link")
	if object == nil {
		return fmt.Errorf("%w: Link type is absent", ErrTypeCheck)
	}
	named, ok := object.Type().(*types.Named)
	if !ok {
		return fmt.Errorf("%w: Link is not a named type", ErrTypeCheck)
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return fmt.Errorf("%w: Link is not a struct", ErrTypeCheck)
	}
	for _, shape := range shapes {
		if shape.PackagePath == pkg.PkgPath && shape.Name == "Link" {
			return nil
		}
	}
	return fmt.Errorf("%w: Link structural shape is absent", ErrTypeCheck)
}

func typeSurfaceID(packagePath, name string) string {
	return "type:" + packagePath + "." + name
}

func formatPackageErrors(errs []packages.Error) string {
	parts := make([]string, len(errs))
	for i, err := range errs {
		parts[i] = err.Error()
	}
	return strings.Join(parts, "; ")
}

func productionManifestDigest(root string, sources []ProductionSource) ([sha256.Size]byte, error) {
	hash := sha256.New()
	for index, source := range sources {
		if source.PackagePath == "" || source.Path == "" || filepath.IsAbs(source.Path) || filepath.ToSlash(filepath.Clean(source.Path)) != source.Path {
			return [sha256.Size]byte{}, fmt.Errorf("%w: noncanonical production source %+v", ErrTypeCheck, source)
		}
		if index > 0 {
			previous := sources[index-1]
			if previous.PackagePath > source.PackagePath || previous.PackagePath == source.PackagePath && previous.Path >= source.Path {
				return [sha256.Size]byte{}, fmt.Errorf("%w: production sources are not strictly ordered: previous=%+v current=%+v", ErrTypeCheck, previous, source)
			}
		}
		path := filepath.Join(root, filepath.FromSlash(source.Path))
		resolved, ok := repoRelative(root, path)
		if !ok || resolved != source.Path {
			return [sha256.Size]byte{}, fmt.Errorf("%w: digest source identity drift %s", ErrTypeCheck, source.Path)
		}
		data, err := readRegularFileAt(root, source.Path, false)
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		fmt.Fprintf(hash, "file\t%s\t%s\t%d\n", source.PackagePath, source.Path, len(data))
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{'\n'})
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := readRegularFileAt(root, name, name == "go.sum")
		if err != nil {
			if os.IsNotExist(err) && name == "go.sum" {
				continue
			}
			return [sha256.Size]byte{}, err
		}
		fmt.Fprintf(hash, "manifest\t%s\t%d\n", name, len(data))
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{'\n'})
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func packageQualifier(subject *types.Package) types.Qualifier {
	return func(pkg *types.Package) string {
		if pkg == nil || pkg == subject {
			return ""
		}
		return pkg.Path()
	}
}

func directImportInventory(byPath map[string]*packages.Package, root string) (map[string][]ImportEdge, error) {
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make(map[string][]ImportEdge, len(paths))
	for _, path := range paths {
		pkg := byPath[path]
		if pkg == nil {
			return nil, fmt.Errorf("%w: nil selected package %s", ErrTypeCheck, path)
		}
		if excludedPackage(path, pkg) {
			continue
		}
		edges, err := directImports(pkg, root)
		if err != nil {
			return nil, err
		}
		result[path] = edges
	}
	return result, nil
}

func canonicalImportEdges(byPath map[string][]ImportEdge) ([]ImportEdge, error) {
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]ImportEdge, 0)
	for _, path := range paths {
		for _, edge := range byPath[path] {
			if edge.From != path || edge.To == "" || edge.SourceFile == "" || edge.Line <= 0 || edge.Column <= 0 {
				return nil, fmt.Errorf("%w: malformed direct import edge %+v", ErrTypeCheck, edge)
			}
			result = append(result, edge)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.From != right.From {
			return left.From < right.From
		}
		if left.To != right.To {
			return left.To < right.To
		}
		if left.SourceFile != right.SourceFile {
			return left.SourceFile < right.SourceFile
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return left.Column < right.Column
	})
	for index := 1; index < len(result); index++ {
		if result[index-1] == result[index] {
			return nil, fmt.Errorf("%w: duplicate direct import edge %+v", ErrTypeCheck, result[index])
		}
	}
	return result, nil
}

func directImports(pkg *packages.Package, root string) ([]ImportEdge, error) {
	if pkg == nil || pkg.Fset == nil || len(pkg.Syntax) == 0 {
		return nil, fmt.Errorf("%w: missing syntax/position inventory for direct imports", ErrTypeCheck)
	}
	result := make([]ImportEdge, 0)
	seen := make(map[string]struct{})
	for _, file := range pkg.Syntax {
		if file == nil {
			return nil, fmt.Errorf("%w: nil syntax file in %s", ErrTypeCheck, pkg.PkgPath)
		}
		for _, spec := range file.Imports {
			if spec == nil || spec.Path == nil || spec.Pos() == token.NoPos {
				return nil, fmt.Errorf("%w: malformed import position in %s", ErrTypeCheck, pkg.PkgPath)
			}
			path := strings.Trim(spec.Path.Value, "\"")
			if path == "" {
				return nil, fmt.Errorf("%w: empty direct import path in %s", ErrTypeCheck, pkg.PkgPath)
			}
			pos := pkg.Fset.PositionFor(spec.Pos(), true)
			source, ok := repoRelative(root, pos.Filename)
			if !ok {
				return nil, fmt.Errorf("%w: import position outside physical root in %s:%d:%d", ErrTypeCheck, pos.Filename, pos.Line, pos.Column)
			}
			if !isProductionGo(source) || isExcludedSourcePath(source) {
				return nil, fmt.Errorf("%w: import position is not in a production source file %s", ErrTypeCheck, source)
			}
			if source == "" || pos.Line <= 0 || pos.Column <= 0 {
				return nil, fmt.Errorf("%w: incomplete import position in %s", ErrTypeCheck, pkg.PkgPath)
			}
			key := path + "\x00" + source
			if _, exists := seen[key]; exists {
				return nil, fmt.Errorf("%w: duplicate direct import evidence %s in %s", ErrTypeCheck, path, pkg.PkgPath)
			}
			seen[key] = struct{}{}
			result = append(result, ImportEdge{From: pkg.PkgPath, To: path, SourceFile: source, Line: pos.Line, Column: pos.Column})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].From != result[j].From {
			return result[i].From < result[j].From
		}
		if result[i].To != result[j].To {
			return result[i].To < result[j].To
		}
		if result[i].SourceFile != result[j].SourceFile {
			return result[i].SourceFile < result[j].SourceFile
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].Column < result[j].Column
	})
	return result, nil
}

func workspaceImportGraph(byPath map[string]*packages.Package, root string, directByPath map[string][]ImportEdge) (map[string][]string, error) {
	graph := make(map[string][]string, len(byPath))
	for path, pkg := range byPath {
		if pkg == nil {
			return nil, fmt.Errorf("%w: nil selected package %s", ErrTypeCheck, path)
		}
		if excludedPackage(path, pkg) {
			continue
		}
		if !hasInRootCompiledFile(root, pkg) {
			return nil, fmt.Errorf("%w: selected package %q has no in-root compiled source", ErrTypeCheck, path)
		}
		edges, ok := directByPath[path]
		if !ok {
			return nil, fmt.Errorf("%w: direct import inventory missing for %s", ErrTypeCheck, path)
		}
		declared := make(map[string]struct{}, len(edges))
		for _, edge := range edges {
			declared[edge.To] = struct{}{}
		}
		loaded := make(map[string]struct{}, len(pkg.Imports))
		for imported := range pkg.Imports {
			loaded[imported] = struct{}{}
		}
		// For typed selected roots, loader Imports and the compiled source's
		// direct import set must agree exactly. This prevents stale metadata or
		// test-only imports from entering the graph.
		if len(declared) != len(loaded) {
			return nil, fmt.Errorf("%w: import source/loader mismatch for %s", ErrTypeCheck, path)
		}
		for imported := range declared {
			if _, ok := loaded[imported]; !ok {
				return nil, fmt.Errorf("%w: source import %s missing loader edge in %s", ErrTypeCheck, imported, path)
			}
		}
		for imported := range loaded {
			if _, ok := declared[imported]; !ok {
				return nil, fmt.Errorf("%w: loader import %s missing source edge in %s", ErrTypeCheck, imported, path)
			}
		}
		paths := make([]string, 0, len(loaded))
		for imported := range loaded {
			paths = append(paths, imported)
		}
		sort.Strings(paths)
		graph[path] = paths
	}
	return graph, nil
}

func validateImportGraph(graph map[string][]string) error {
	state := make(map[string]uint8, len(graph))
	var visit func(string) error
	visit = func(path string) error {
		switch state[path] {
		case 1:
			return fmt.Errorf("%w: %s", ErrImportCycle, path)
		case 2:
			return nil
		}
		state[path] = 1
		for _, dep := range graph[path] {
			if _, ok := graph[dep]; ok {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		state[path] = 2
		return nil
	}
	paths := make([]string, 0, len(graph))
	for path := range graph {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := visit(path); err != nil {
			return err
		}
	}
	return nil
}

// typedUseInventory deliberately does not infer callers from import syntax.
// A package is a caller only when go/types records a concrete family object in
// Uses, Selections, or Instances. This excludes import-only and dead-source
// false positives while retaining method values, method expressions, interface
// selections, and generic instantiations.
func typedUseInventory(root string, byPath map[string]*packages.Package, familyPrefix string, declarations declarationInventory) ([]UseSite, error) {
	// Uses, Selections, and Instances are orthogonal typed observations. Keep
	// distinct canonical facts even when they share one token position; only
	// an exact duplicate fact is collapsed.
	useByFact := make(map[string]UseSite)
	addUse := func(use UseSite) error {
		key := useSiteFactKey(use)
		if _, exists := useByFact[key]; exists {
			return nil
		}
		useByFact[key] = use
		return nil
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		pkg := byPath[path]
		if excludedPackage(path, pkg) {
			return nil, fmt.Errorf("%w: excluded package entered typed-use inventory: %s", ErrTypeCheck, path)
		}
		if pkg == nil || pkg.TypesInfo == nil || pkg.Types == nil || pkg.Fset == nil || len(pkg.Syntax) == 0 {
			return nil, fmt.Errorf("%w: incomplete typed-use boundary for %s", ErrTypeCheck, path)
		}
		for _, file := range pkg.Syntax {
			if file == nil {
				return nil, fmt.Errorf("%w: nil typed syntax file for %s", ErrTypeCheck, path)
			}
		}
		spans := indexFunctionSpans(pkg)
		qualifier := packageQualifier(pkg.Types)
		for id, object := range pkg.TypesInfo.Uses {
			if id == nil || object == nil {
				return nil, fmt.Errorf("%w: malformed typed-use entry in %s", ErrTypeCheck, path)
			}
			// Local-object detection walks syntax only to distinguish a
			// family-shadowing declaration. Avoid paying that cost for every
			// ordinary standard-library/type use in the complete reverse
			// closure: only direct family objects and true aliases can reach
			// the Link declaration plane.
			if !couldReachFamily(object, familyPrefix) {
				continue
			}
			if isLocalFamilyObject(pkg, object, spans) {
				continue
			}
			decl, aliasChain, found, err := familyUseDeclaration(root, pkg, object, familyPrefix, declarations)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			evidence := useEvidence("use", aliasChain)
			use, err := makeUseSite(root, pkg, id.Pos(), objectSymbol(object), evidence, types.TypeString(object.Type(), qualifier), decl.FactID, aliasChain)
			if err != nil {
				return nil, err
			}
			use.Role = useRoleAtPosition(pkg, id.Pos(), Reference)
			use.FactID = useSiteFactID(use)
			if err := addUse(use); err != nil {
				return nil, err
			}
		}
		for selector, selection := range pkg.TypesInfo.Selections {
			if selector == nil || selection == nil || selection.Obj() == nil {
				return nil, fmt.Errorf("%w: malformed typed selection entry in %s", ErrTypeCheck, path)
			}
			object := selection.Obj()
			if !couldReachFamily(object, familyPrefix) {
				continue
			}
			if isLocalFamilyObject(pkg, object, spans) {
				continue
			}
			decl, aliasChain, found, err := familyUseDeclaration(root, pkg, object, familyPrefix, declarations)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			selectionAliases, selectionHasAlias, err := selectionAliasEvidence(pkg, selector, familyPrefix)
			if err != nil {
				return nil, err
			}
			if len(aliasChain) == 0 && selectionHasAlias {
				aliasChain = selectionAliases
			}
			evidence := selectionEvidence(selection.Kind())
			evidence = useEvidence(evidence, aliasChain)
			use, err := makeUseSite(root, pkg, selector.Sel.Pos(), objectSymbol(object), evidence, types.TypeString(object.Type(), qualifier), decl.FactID, aliasChain)
			if err != nil {
				return nil, err
			}
			use.Role = useRoleAtPosition(pkg, selector.Sel.Pos(), Reference)
			use.FactID = useSiteFactID(use)
			if err := addUse(use); err != nil {
				return nil, err
			}
		}
		for id, instance := range pkg.TypesInfo.Instances {
			if id == nil {
				return nil, fmt.Errorf("%w: malformed generic instance position in %s", ErrTypeCheck, path)
			}
			object := pkg.TypesInfo.Uses[id]
			if instance.Type == nil {
				return nil, fmt.Errorf("%w: nil generic instance type at %s", ErrTypeCheck, path)
			}
			if object == nil {
				return nil, fmt.Errorf("%w: generic instance has no target object at %s", ErrTypeCheck, path)
			}
			if !couldReachFamily(object, familyPrefix) {
				continue
			}
			if isLocalFamilyObject(pkg, object, spans) {
				continue
			}
			decl, aliasChain, found, err := familyUseDeclaration(root, pkg, object, familyPrefix, declarations)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			evidence := useEvidence("instance", aliasChain)
			use, err := makeUseSite(root, pkg, id.Pos(), objectSymbol(object), evidence, types.TypeString(instance.Type, qualifier), decl.FactID, aliasChain)
			if err != nil {
				return nil, err
			}
			use.Role = TypeInstance
			use.FactID = useSiteFactID(use)
			if err := addUse(use); err != nil {
				return nil, err
			}
		}
	}
	uses := make([]UseSite, 0, len(useByFact))
	for _, use := range useByFact {
		uses = append(uses, use)
	}
	sort.Slice(uses, func(i, j int) bool { return useSiteKey(uses[i]) < useSiteKey(uses[j]) })
	return uses, nil
}

func useEvidence(base string, aliasChain []string) string {
	if len(aliasChain) == 0 {
		return base
	}
	return "alias-chain/" + base
}

func sameAliasChain(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func selectionAliasEvidence(pkg *packages.Package, selector *ast.SelectorExpr, familyPrefix string) ([]string, bool, error) {
	if pkg == nil || pkg.TypesInfo == nil || selector == nil {
		return nil, false, nil
	}
	typed, ok := pkg.TypesInfo.Types[selector.X]
	if !ok || typed.Type == nil {
		return nil, false, nil
	}
	chain, found, err := externalAliasChainFromType(typed.Type, familyPrefix)
	return chain, found, err
}

func externalAliasChainFromType(typ types.Type, familyPrefix string) ([]string, bool, error) {
	switch value := typ.(type) {
	case *types.Alias:
		object := value.Obj()
		if object == nil {
			return nil, false, fmt.Errorf("%w: anonymous selection alias", ErrUseTargetJoin)
		}
		_, chain, found, err := resolveExternalAlias(object, familyPrefix)
		return chain, found, err
	default:
		return nil, false, nil
	}
}

func selectionEvidence(kind types.SelectionKind) string {
	switch kind {
	case types.MethodVal:
		return "method-value"
	case types.MethodExpr:
		return "method-expression"
	case types.FieldVal:
		return "field-selection"
	default:
		return "selection"
	}
}

func belongsToFamily(object types.Object, prefix string) bool {
	if object == nil || object.Pkg() == nil {
		return false
	}
	path := object.Pkg().Path()
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func couldReachFamily(object types.Object, prefix string) bool {
	if belongsToFamily(object, prefix) {
		return true
	}
	typeName, ok := object.(*types.TypeName)
	return ok && typeName != nil && typeName.IsAlias()
}

// familyUseDeclaration is the only typed-use target join. Direct family
// objects retain their exact declaration target. An object from another
// selected package is admitted only when it is a true Go alias chain whose
// terminal named object belongs to the Link family; a defined wrapper is a
// distinct named type and is intentionally not opened.
func familyUseDeclaration(root string, pkg *packages.Package, object types.Object, familyPrefix string, declarations declarationInventory) (DeclarationInfo, []string, bool, error) {
	if object == nil || object.Pkg() == nil {
		return DeclarationInfo{}, nil, false, nil
	}
	if belongsToFamily(object, familyPrefix) {
		declaration, err := declarationForTypedUseObject(root, pkg, object, declarations)
		if err != nil {
			return DeclarationInfo{}, nil, false, err
		}
		if declaration.FactID == "" {
			return DeclarationInfo{}, nil, false, fmt.Errorf("%w: family object %s.%s has no declaration fact", ErrUseTargetJoin, object.Pkg().Path(), object.Name())
		}
		return declaration, nil, true, nil
	}
	target, chain, found, err := resolveExternalAlias(object, familyPrefix)
	if err != nil {
		return DeclarationInfo{}, nil, false, err
	}
	if !found {
		return DeclarationInfo{}, nil, false, nil
	}
	declaration, err := declarationForTypedUseObject(root, pkg, target, declarations)
	if err != nil {
		return DeclarationInfo{}, nil, false, fmt.Errorf("%w: alias chain %v terminal %s.%s: %v", ErrUseTargetJoin, chain, target.Pkg().Path(), target.Name(), err)
	}
	if declaration.FactID == "" {
		return DeclarationInfo{}, nil, false, fmt.Errorf("%w: alias chain %v has no terminal declaration fact", ErrUseTargetJoin, chain)
	}
	return declaration, chain, true, nil
}

// declarationForTypedUseObject keeps the exact object join as the primary
// path. go/types creates a fresh *types.Func for an instantiated generic
// method/function; it retains the declaration source position but its
// instantiated signature no longer matches the generic declaration row. A
// unique package/name/source-position match is the only permitted fallback,
// so generic instantiation does not weaken the one-declaration invariant.
func declarationForTypedUseObject(root string, pkg *packages.Package, object types.Object, inv declarationInventory) (DeclarationInfo, error) {
	declaration, err := declarationForObject(root, pkg, object, inv)
	if err == nil || !errors.Is(err, ErrUseTargetJoin) {
		return declaration, err
	}
	if object == nil || object.Pkg() == nil || pkg == nil || pkg.Fset == nil {
		return DeclarationInfo{}, err
	}
	location := pkg.Fset.PositionFor(object.Pos(), true)
	source, ok := repoRelative(root, location.Filename)
	if !ok || source == "" || location.Line <= 0 || location.Column <= 0 {
		return DeclarationInfo{}, err
	}
	matches := make([]DeclarationInfo, 0, 1)
	for _, candidate := range inv.Declarations {
		if candidate.PackagePath == object.Pkg().Path() && candidate.Name == object.Name() &&
			candidate.SourceFile == source && candidate.Line == location.Line && candidate.Column == location.Column {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return DeclarationInfo{}, fmt.Errorf("%w: generic typed object %s.%s at %s:%d:%d matched %d declaration facts", ErrUseTargetJoin, object.Pkg().Path(), object.Name(), source, location.Line, location.Column, len(matches))
	}
	return DeclarationInfo{}, err
}

// resolveExternalAlias follows only a true alias chain. The RHS at the end
// of that chain must be one named Link declaration (a generic instantiation
// is still represented by its named Obj). Composite aliases such as `*Link`,
// `[]Link`, maps, or anonymous structs are not singular Link declarations and
// therefore produce no alias join. A defined wrapper remains a separate named
// type and is never opened.
func resolveExternalAlias(object types.Object, familyPrefix string) (types.Object, []string, bool, error) {
	typeName, ok := object.(*types.TypeName)
	if !ok || !typeName.IsAlias() {
		return nil, nil, false, nil
	}
	chain := []string{canonicalAliasObject(typeName)}
	seenAliases := make(map[string]struct{})
	typ := typeName.Type()
	for {
		if typ == nil {
			return nil, nil, false, fmt.Errorf("%w: alias chain %v has nil RHS", ErrUseTargetJoin, chain)
		}
		switch value := typ.(type) {
		case *types.Alias:
			aliasObject := value.Obj()
			if aliasObject == nil || aliasObject.Pkg() == nil {
				return nil, nil, false, fmt.Errorf("%w: alias chain %v has anonymous alias", ErrUseTargetJoin, chain)
			}
			aliasKey := canonicalAliasObject(aliasObject)
			if _, duplicate := seenAliases[aliasKey]; duplicate {
				return nil, nil, false, fmt.Errorf("%w: cyclic alias chain %v", ErrUseTargetJoin, chain)
			}
			seenAliases[aliasKey] = struct{}{}
			if aliasKey != chain[len(chain)-1] {
				chain = append(chain, aliasKey)
			}
			typ = value.Rhs()
		case *types.Named:
			if value.Obj() == nil || !belongsToFamily(value.Obj(), familyPrefix) {
				return nil, nil, false, nil
			}
			return value.Obj(), chain, true, nil
		default:
			// A composite RHS is retained in the selected source closure,
			// but it cannot be assigned one singular Link declaration ID.
			return nil, nil, false, nil
		}
	}
}

func canonicalAliasObject(object *types.TypeName) string {
	if object == nil || object.Pkg() == nil {
		return ""
	}
	return object.Pkg().Path() + "." + object.Name()
}

// A type parameter or a function-local named type can retain the declaring
// package on its go/types object even though it is not a package declaration.
// Such objects are intentionally absent from the declaration ledger and are
// excluded before the exact family-target join. Other missing family objects
// remain hard join errors.
type functionSpan struct {
	Start token.Pos
	End   token.Pos
}

func indexFunctionSpans(pkg *packages.Package) []functionSpan {
	if pkg == nil || pkg.Syntax == nil {
		return nil
	}
	spans := make([]functionSpan, 0)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncDecl:
				spans = append(spans, functionSpan{Start: value.Pos(), End: value.End()})
			case *ast.FuncLit:
				spans = append(spans, functionSpan{Start: value.Pos(), End: value.End()})
			}
			return true
		})
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End < spans[j].End
	})
	return spans
}

func isLocalFamilyObject(pkg *packages.Package, object types.Object, spans []functionSpan) bool {
	if object == nil || object.Pkg() == nil {
		return true
	}
	if _, ok := object.(*types.PkgName); ok {
		return true
	}
	if typeName, ok := object.(*types.TypeName); ok {
		// A local named type may shadow a package declaration with the same
		// name, so package-scope Lookup alone is not an ownership test. The
		// lexical parent is authoritative when available; the syntax-position
		// check covers go/types objects whose Parent is not retained by a
		// loader mode.
		if parent := typeName.Parent(); parent != nil && parent != object.Pkg().Scope() {
			return true
		}
		if typeName.Parent() == nil && positionInsideFunction(spans, object.Pos()) {
			return true
		}
		return object.Pkg().Scope().Lookup(object.Name()) == nil
	}
	switch object.(type) {
	case *types.Var, *types.Const:
		// Struct fields have no lexical scope in go/types; package values
		// are children of the package scope. Parameters and locals have a
		// distinct non-nil lexical scope and are therefore excluded.
		parent := object.Parent()
		if parent != nil && parent != object.Pkg().Scope() {
			return true
		}
		// A field of an anonymous struct declared inside a function also
		// has no Parent scope. This is the only remaining case requiring
		// a syntax-position check, and it is reached only for family or
		// alias candidates rather than every typed identifier.
		if parent == nil && positionInsideFunction(spans, object.Pos()) {
			return true
		}
	}
	return false
}

func positionInsideFunction(spans []functionSpan, position token.Pos) bool {
	if position == token.NoPos {
		return false
	}
	for _, span := range spans {
		if position < span.Start {
			break
		}
		if position <= span.End {
			return true
		}
	}
	return false
}

func makeUseSite(root string, pkg *packages.Package, position token.Pos, symbol, evidence, typ, targetDeclID string, aliasChain []string) (UseSite, error) {
	if pkg == nil || pkg.Fset == nil || position == token.NoPos || symbol == "" || evidence == "" || typ == "" || targetDeclID == "" {
		return UseSite{}, fmt.Errorf("%w: incomplete typed-use position for %s", ErrTypeCheck, symbol)
	}
	for _, alias := range aliasChain {
		if alias == "" {
			return UseSite{}, fmt.Errorf("%w: incomplete typed-use alias chain for %s", ErrTypeCheck, symbol)
		}
	}
	location := pkg.Fset.PositionFor(position, true)
	source, ok := repoRelative(root, location.Filename)
	if !ok || source == "" || location.Line <= 0 || location.Column <= 0 {
		return UseSite{}, fmt.Errorf("%w: typed-use position outside physical root for %s at %s:%d:%d", ErrTypeCheck, symbol, location.Filename, location.Line, location.Column)
	}
	if !isProductionGo(source) || isExcludedSourcePath(source) {
		return UseSite{}, fmt.Errorf("%w: typed-use position is not in a production source file %s", ErrTypeCheck, source)
	}
	chain := append([]string(nil), aliasChain...)
	role := Reference
	if evidence == "instance" || strings.HasSuffix(evidence, "/instance") {
		role = TypeInstance
	}
	use := UseSite{PackagePath: pkg.PkgPath, SourceFile: source, Line: location.Line, Column: location.Column, Symbol: symbol, Evidence: evidence, Type: typ, TargetDeclID: targetDeclID, AliasChain: chain, Role: role}
	use.FactID = useSiteFactID(use)
	return use, nil
}

// useRoleAtPosition classifies only the typed AST context needed for the
// closed UseRole vocabulary.  It does not parse source text or infer a query
// relation.  The caller/token identity remains the scanner's primary fact.
func useRoleAtPosition(pkg *packages.Package, position token.Pos, fallback UseRole) UseRole {
	if fallback == TypeInstance {
		return TypeInstance
	}
	if pkg == nil || position == token.NoPos {
		return fallback
	}
	role := fallback
	for _, file := range pkg.Syntax {
		if file == nil {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil || role == CallCallee {
				return role != CallCallee
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || call.Fun == nil || !calleeTokenAt(call.Fun, position) {
				return true
			}
			role = CallCallee
			return false
		})
		if role == CallCallee {
			return role
		}
	}
	return role
}

func calleeTokenAt(node ast.Expr, position token.Pos) bool {
	switch value := node.(type) {
	case *ast.Ident:
		return value.Pos() == position
	case *ast.SelectorExpr:
		return value.Sel != nil && value.Sel.Pos() == position
	case *ast.ParenExpr:
		return calleeTokenAt(value.X, position)
	case *ast.IndexExpr:
		return calleeTokenAt(value.X, position)
	case *ast.IndexListExpr:
		return calleeTokenAt(value.X, position)
	default:
		return false
	}
}

func useSiteFactID(use UseSite) string {
	hash := sha256.New()
	writeCanonicalPart := func(value string) {
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{'\n'})
	}
	writeCanonicalPart("link-use-v2")
	writeCanonicalPart(use.PackagePath)
	writeCanonicalPart(use.SourceFile)
	writeCanonicalPart(strconv.Itoa(use.Line))
	writeCanonicalPart(strconv.Itoa(use.Column))
	writeCanonicalPart(use.Symbol)
	writeCanonicalPart(use.TargetDeclID)
	writeCanonicalPart(string(use.Role))
	writeCanonicalPart(use.Evidence)
	writeCanonicalPart(use.Type)
	writeCanonicalPart(strconv.Itoa(len(use.AliasChain)))
	for _, alias := range use.AliasChain {
		writeCanonicalPart(alias)
	}
	return "use-v2-" + hex.EncodeToString(hash.Sum(nil))
}

func useSiteKey(use UseSite) string {
	return fmt.Sprintf("%s\x00%s\x00%08d\x00%08d\x00%s\x00%s\x00%s\x00%s", use.PackagePath, use.SourceFile, use.Line, use.Column, use.Symbol, use.Role, use.Evidence, use.FactID)
}

func useSiteFactKey(use UseSite) string {
	return useSiteKey(use)
}

func objectSymbol(object types.Object) string {
	if object == nil || object.Pkg() == nil {
		return ""
	}
	if fn, ok := object.(*types.Func); ok {
		if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
			return fn.Pkg().Path() + "." + receiverObjectName(sig.Recv().Type()) + "." + fn.Name()
		}
	}
	return object.Pkg().Path() + "." + object.Name()
}

func receiverObjectName(typ types.Type) string {
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	if named, ok := types.Unalias(typ).(*types.Named); ok {
		return named.Obj().Name()
	}
	return types.TypeString(typ, nil)
}

func resolvedModules(root string, byPath map[string]*packages.Package) ([]ModuleInfo, error) {
	physicalRoot, err := canonicalRoot(root)
	if err != nil {
		return nil, err
	}
	sums := moduleSums(physicalRoot)
	seen := make(map[string]ModuleInfo)
	resolutionByOriginal := make(map[string]string)
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		pkg := byPath[path]
		if pkg == nil {
			return nil, fmt.Errorf("%w: nil package in module closure %s", ErrTypeCheck, path)
		}
		if pkg.Module == nil {
			continue
		}
		original := pkg.Module
		// A package's original module path is a logical module identity, never a
		// filesystem locator. Validate it before any GoMod/file resolution so an
		// absolute or traversal-shaped value cannot become a committed module by
		// passing a later filesystem check.
		if err := validateLogicalModulePath(original.Path); err != nil {
			return nil, fmt.Errorf("%w: original module path %q: %v", ErrTypeCheck, original.Path, err)
		}
		resolved, err := packageTerminalModule(original)
		if err != nil {
			return nil, fmt.Errorf("%w: module %s replacement chain: %v", ErrTypeCheck, original.Path, err)
		}
		resolvedIsFilesystem := resolved.Version == "" && filesystemModulePath(resolved.Path)
		if !resolvedIsFilesystem {
			if err := validateLogicalModulePath(resolved.Path); err != nil {
				return nil, fmt.Errorf("%w: resolved module path %q: %v", ErrTypeCheck, resolved.Path, err)
			}
		}
		originalGoMod, _, _, err := canonicalModuleGoMod(physicalRoot, original)
		if err != nil {
			return nil, err
		}
		resolvedGoMod, resolvedDir, resolvedLocal, err := canonicalModuleGoMod(physicalRoot, resolved)
		if err != nil {
			return nil, err
		}
		resolvedPath := resolved.Path
		contentDigest := ""
		if original.Replace != nil && resolvedLocal {
			rel, ok := physicalRelative(physicalRoot, resolvedDir)
			if !ok {
				return nil, fmt.Errorf("%w: local replacement %s is outside the admitted root", ErrTypeCheck, resolved.Path)
			}
			resolvedPath = "local/" + rel
			contentDigest, err = localModuleContentDigest(physicalRoot, resolvedDir, resolved, byPath)
			if err != nil {
				return nil, err
			}
		} else if resolved.Version != "" {
			// A go.sum h1 authenticates the published module version, but it
			// does not by itself prove that the exact typed files consumed by
			// this load remain unchanged in a mutable cache. Commit those
			// selected bytes as a second, path-free provenance plane.
			contentDigest, err = moduleCompiledInputDigest(resolved, byPath)
			if err != nil {
				return nil, err
			}
		}
		if err := validateResolvedModulePath(resolvedPath, original.Replace != nil && resolvedLocal); err != nil {
			return nil, fmt.Errorf("%w: committed resolved module path %q: %v", ErrTypeCheck, resolvedPath, err)
		}
		if err := requireModuleProvenance(original, resolved, resolvedLocal, contentDigest, sums); err != nil {
			return nil, err
		}
		info := ModuleInfo{
			Path: original.Path, Version: original.Version, Sum: sums[original.Path+"\x00"+original.Version], GoMod: originalGoMod,
			ResolvedPath: resolvedPath, ResolvedVersion: resolved.Version, ResolvedSum: sums[resolved.Path+"\x00"+resolved.Version], ResolvedGoMod: resolvedGoMod,
			ResolvedContentDigest: contentDigest,
		}
		key := strings.Join([]string{info.Path, info.Version, info.Sum, info.GoMod, info.ResolvedPath, info.ResolvedVersion, info.ResolvedSum, info.ResolvedGoMod, info.ResolvedContentDigest}, "\x00")
		originalIdentity := replacementLogicalKey(original)
		resolvedIdentity := committedTerminalIdentity(resolvedPath, resolved.Version, contentDigest)
		if previous, exists := resolutionByOriginal[originalIdentity]; exists && previous != resolvedIdentity {
			return nil, fmt.Errorf("%w: ambiguous replacement for module %s@%s", ErrTypeCheck, original.Path, original.Version)
		}
		resolutionByOriginal[originalIdentity] = resolvedIdentity
		seen[key] = info
	}
	result := make([]ModuleInfo, 0, len(seen))
	for _, module := range seen {
		result = append(result, module)
	}
	sort.Slice(result, func(i, j int) bool { return moduleInfoLess(result[i], result[j]) })
	return result, nil
}

func canonicalModuleGoMod(root string, module *packages.Module) (identity, directory string, local bool, err error) {
	if module == nil {
		return "", "", false, nil
	}
	if module.Version == "" && !filesystemModulePath(module.Path) {
		if err := validateLogicalModulePath(module.Path); err != nil {
			return "", "", false, fmt.Errorf("%w: module path %q: %v", ErrTypeCheck, module.Path, err)
		}
	}
	goMod := module.GoMod
	if goMod == "" && module.Version == "" && module.Path != "" {
		goMod = filepath.Join(module.Path, "go.mod")
	}
	if goMod != "" {
		candidate := goMod
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, candidate)
		}
		physical, resolveErr := filepath.EvalSymlinks(filepath.Clean(candidate))
		if resolveErr == nil {
			physical, resolveErr = filepath.Abs(filepath.Clean(physical))
			if resolveErr != nil {
				return "", "", false, fmt.Errorf("%w: canonicalize module go.mod %s: %v", ErrTypeCheck, goMod, resolveErr)
			}
			rel, inside := physicalRelative(root, physical)
			if inside {
				return rel, filepath.Dir(physical), true, nil
			}
			if module.Version == "" || filesystemModulePath(module.Path) {
				return "", "", false, fmt.Errorf("%w: local module go.mod %s is outside the admitted root", ErrTypeCheck, goMod)
			}
		}
		if resolveErr != nil && module.Version == "" {
			return "", "", false, fmt.Errorf("%w: local module go.mod %s cannot be resolved: %v", ErrTypeCheck, goMod, resolveErr)
		}
	}
	if module.Version == "" {
		return "", "", false, fmt.Errorf("%w: local module %s has no content-committed go.mod", ErrTypeCheck, module.Path)
	}
	return "module:" + module.Path + "@" + module.Version + "/go.mod", "", false, nil
}

func filesystemModulePath(path string) bool {
	return filepath.IsAbs(path) || path == "." || path == ".." || strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || strings.HasPrefix(path, ".\\") || strings.HasPrefix(path, "..\\")
}

func validateLogicalModulePath(path string) error {
	if path == "" {
		return fmt.Errorf("empty module path")
	}
	if filesystemModulePath(path) {
		return fmt.Errorf("filesystem-shaped module path")
	}
	return gomodule.CheckPath(path)
}

func validateResolvedModulePath(path string, local bool) error {
	if !local {
		return validateLogicalModulePath(path)
	}
	// Local replacements are represented by a repository-relative marker, not
	// by the absolute filesystem Path supplied by go/packages. Keep this plane
	// path-free and reject traversal before it can enter the fingerprint.
	if !strings.HasPrefix(path, "local/") {
		return fmt.Errorf("local replacement path is not canonical")
	}
	rel := strings.TrimPrefix(path, "local/")
	if rel == "" || filepath.IsAbs(rel) || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "\\") {
		return fmt.Errorf("local replacement path is filesystem-shaped")
	}
	return nil
}

func requireModuleProvenance(original, resolved *packages.Module, resolvedLocal bool, contentDigest string, sums map[string]string) error {
	if original == nil || resolved == nil {
		return fmt.Errorf("%w: incomplete module provenance", ErrTypeCheck)
	}
	if original.Version != "" {
		originalSum := sums[original.Path+"\x00"+original.Version]
		// A local replacement's exact typed compiled-input digest is sufficient
		// to commit the bytes actually analyzed; the unreplaced request itself
		// still remains in the identity plane.
		if originalSum == "" && contentDigest == "" {
			return fmt.Errorf("%w: nonlocal module %s@%s has no go.sum checksum or committed typed input digest", ErrTypeCheck, original.Path, original.Version)
		}
	}
	if resolved.Version != "" && !resolvedLocal {
		if sums[resolved.Path+"\x00"+resolved.Version] == "" {
			return fmt.Errorf("%w: resolved nonlocal module %s@%s has no go.sum checksum", ErrTypeCheck, resolved.Path, resolved.Version)
		}
	}
	return nil
}

func localModuleContentDigest(root, moduleDir string, module *packages.Module, byPath map[string]*packages.Package) (string, error) {
	if _, inside := physicalRelative(root, moduleDir); !inside {
		return "", fmt.Errorf("%w: local replacement directory %s is outside the admitted root", ErrTypeCheck, moduleDir)
	}
	goMod := filepath.Join(moduleDir, "go.mod")
	if _, err := os.Stat(goMod); err != nil {
		return "", fmt.Errorf("%w: local replacement go.mod missing at %s: %v", ErrTypeCheck, goMod, err)
	}
	return moduleCompiledInputDigestAt(moduleDir, goMod, module, byPath, true)
}

func moduleCompiledInputDigest(module *packages.Module, byPath map[string]*packages.Package) (string, error) {
	if module == nil {
		return "", fmt.Errorf("%w: versioned module has no terminal identity", ErrTypeCheck)
	}
	goMod := module.GoMod
	if goMod == "" && module.Dir != "" {
		goMod = filepath.Join(module.Dir, "go.mod")
	}
	return moduleCompiledInputDigestAt(module.Dir, goMod, module, byPath, true)
}

func moduleCompiledInputDigestAt(moduleDir, goMod string, module *packages.Module, byPath map[string]*packages.Package, requireInput bool) (string, error) {
	if module == nil {
		return "", fmt.Errorf("%w: module has no terminal identity", ErrTypeCheck)
	}
	targetIdentity := terminalModuleIdentityKey(module)
	files := make(map[string]string)
	if goMod != "" {
		if physical, err := filepath.EvalSymlinks(filepath.Clean(goMod)); err == nil {
			physical, err = filepath.Abs(filepath.Clean(physical))
			if err != nil {
				return "", fmt.Errorf("%w: canonicalize module go.mod %s: %v", ErrTypeCheck, goMod, err)
			}
			files["go.mod"] = physical
		} else if requireInput && module.Version == "" {
			return "", fmt.Errorf("%w: module go.mod %s cannot be resolved: %v", ErrTypeCheck, goMod, err)
		}
	}
	matchedPackages := 0
	matchedFiles := 0
	for path, pkg := range byPath {
		if pkg == nil {
			continue
		}
		terminal, err := packageTerminalModule(pkg.Module)
		if err != nil {
			return "", fmt.Errorf("%w: package %s has invalid module chain: %v", ErrTypeCheck, path, err)
		}
		if terminal == nil || terminalModuleIdentityKey(terminal) != targetIdentity {
			continue
		}
		matchedPackages++
		for _, file := range pkg.CompiledGoFiles {
			physical, err := filepath.EvalSymlinks(filepath.Clean(file))
			if err != nil {
				return "", fmt.Errorf("%w: local replacement source %s cannot be resolved: %v", ErrTypeCheck, file, err)
			}
			physical, err = filepath.Abs(filepath.Clean(physical))
			if err != nil {
				return "", fmt.Errorf("%w: local replacement source %s cannot be canonicalized: %v", ErrTypeCheck, file, err)
			}
			fileIdentity := path + ":" + filepath.Base(physical)
			if moduleDir != "" {
				if rel, inside := physicalRelative(moduleDir, physical); inside {
					fileIdentity = rel
				} else {
					return "", fmt.Errorf("%w: module package %s source %s escapes its module root", ErrTypeCheck, path, file)
				}
			} else if pkg.Dir != "" {
				if packageDir, dirErr := filepath.EvalSymlinks(filepath.Clean(pkg.Dir)); dirErr == nil {
					if rel, inside := physicalRelative(packageDir, physical); inside {
						fileIdentity = path + ":" + rel
					}
				}
			}
			if previous, exists := files[fileIdentity]; exists && previous != physical {
				return "", fmt.Errorf("%w: module input identity %s maps to multiple files", ErrTypeCheck, fileIdentity)
			}
			files[fileIdentity] = physical
			matchedFiles++
		}
	}
	if matchedPackages == 0 || (requireInput && matchedFiles == 0) {
		return "", fmt.Errorf("%w: local replacement has no typed package closure for terminal module %s", ErrTypeCheck, module.Path)
	}
	keys := make([]string, 0, len(files))
	for rel := range files {
		keys = append(keys, rel)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, rel := range keys {
		data, err := os.ReadFile(files[rel])
		if err != nil {
			return "", fmt.Errorf("%w: read local replacement input %s: %v", ErrTypeCheck, files[rel], err)
		}
		fmt.Fprintf(hash, "file\t%s\t%d\n", rel, len(data))
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func packageTerminalModule(module *packages.Module) (*packages.Module, error) {
	seenPointers := make(map[*packages.Module]struct{})
	seenIdentities := make(map[string]struct{})
	for module != nil {
		if _, exists := seenPointers[module]; exists {
			return nil, fmt.Errorf("replacement cycle")
		}
		seenPointers[module] = struct{}{}
		identity := replacementLogicalKey(module)
		if module.Path == "" || identity == "" {
			return nil, fmt.Errorf("malformed replacement node")
		}
		if module.Replace != nil {
			if filesystemModulePath(module.Path) {
				return nil, fmt.Errorf("nonterminal replacement node has filesystem-shaped path")
			}
			if err := validateLogicalModulePath(module.Path); err != nil {
				return nil, fmt.Errorf("invalid replacement node path %q: %v", module.Path, err)
			}
		}
		if _, exists := seenIdentities[identity]; exists {
			return nil, fmt.Errorf("replacement cycle")
		}
		seenIdentities[identity] = struct{}{}
		if module.Replace == nil {
			return module, nil
		}
		module = module.Replace
	}
	return nil, fmt.Errorf("malformed nil replacement node")
}

func terminalModuleIdentityKey(module *packages.Module) string {
	if module == nil {
		return ""
	}
	return replacementLogicalKey(module)
}

func replacementLogicalKey(module *packages.Module) string {
	if module == nil {
		return ""
	}
	return strings.Join([]string{module.Path, module.Version}, "\x00")
}

func committedTerminalIdentity(path, version, contentDigest string) string {
	return strings.Join([]string{path, version, contentDigest}, "\x00")
}

func moduleInfoLess(left, right ModuleInfo) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Version != right.Version {
		return left.Version < right.Version
	}
	if left.Sum != right.Sum {
		return left.Sum < right.Sum
	}
	if left.GoMod != right.GoMod {
		return left.GoMod < right.GoMod
	}
	if left.ResolvedPath != right.ResolvedPath {
		return left.ResolvedPath < right.ResolvedPath
	}
	if left.ResolvedVersion != right.ResolvedVersion {
		return left.ResolvedVersion < right.ResolvedVersion
	}
	if left.ResolvedSum != right.ResolvedSum {
		return left.ResolvedSum < right.ResolvedSum
	}
	if left.ResolvedGoMod != right.ResolvedGoMod {
		return left.ResolvedGoMod < right.ResolvedGoMod
	}
	return left.ResolvedContentDigest < right.ResolvedContentDigest
}

func moduleSums(root string) map[string]string {
	result := make(map[string]string)
	data, err := readRegularFileAt(root, "go.sum", true)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || strings.HasSuffix(fields[1], "/go.mod") {
			continue
		}
		result[fields[0]+"\x00"+fields[1]] = fields[2]
	}
	return result
}

func buildFingerprint(digest [sha256.Size]byte, modules []ModuleInfo, context BuildContext) (string, error) {
	if err := context.verify(); err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write(digest[:])
	fmt.Fprintf(hash, "build-context\t%s\n", context.key())
	ordered := append([]ModuleInfo(nil), modules...)
	sort.Slice(ordered, func(i, j int) bool { return moduleInfoLess(ordered[i], ordered[j]) })
	for _, module := range ordered {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\n", module.Path, module.Version, module.Sum, module.GoMod, module.ResolvedPath, module.ResolvedVersion, module.ResolvedSum, module.ResolvedGoMod, module.ResolvedContentDigest)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func isProductionGo(path string) bool {
	return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}

func isExcludedSourcePath(path string) bool {
	slash := filepath.ToSlash(filepath.Clean(path))
	for _, root := range []string{"__legacy", "analysis/test", "program/testfixture"} {
		if slash == root || strings.HasPrefix(slash, root+"/") || strings.HasSuffix(slash, "/"+root) || strings.Contains(slash, "/"+root+"/") {
			return true
		}
	}
	return false
}
