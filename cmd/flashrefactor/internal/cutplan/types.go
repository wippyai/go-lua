package cutplan

// Version identifies the one-authority flash-cut schema. Version 3 removes
// authored residue lists and regex test selectors: both denominators are now
// generated from the reviewed ownership cut.
const Version = 3

// Intent is the only reviewed declaration.  It says what semantic authority
// moves and the complete mechanical closure; resolver and executor output is
// deliberately absent from this type and is committed only in Lock.
type Intent struct {
	Schema     int         `json:"schema"`
	Name       string      `json:"name"`
	Operations []Operation `json:"operations"`
}

// Operation is one ownership cut.  Its eight fields are intentionally the
// whole authored vocabulary: there is no second action language, implicit
// output list, compatibility switch, or executor command.
type Operation struct {
	ID        string       `json:"id"`
	After     []string     `json:"after,omitempty"`
	Authority Authority    `json:"authority"`
	Edits     []Edit       `json:"edits"`
	Bindings  []Binding    `json:"bindings,omitempty"`
	Imports   []Import     `json:"imports,omitempty"`
	Footprint Footprint    `json:"footprint"`
	Verify    Verification `json:"verify"`
}

// Authority records the old and sole post-cut semantic owner.  A bridge is
// not an authority: consumers route directly to To through exact Bindings.
type Authority struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Edit is a closed sum.  Exactly one payload matching Kind is present.
type Edit struct {
	Kind     EditKind  `json:"kind"`
	Relocate *Relocate `json:"relocate,omitempty"`
	Retire   *Retire   `json:"retire,omitempty"`
	Generate *Generate `json:"generate,omitempty"`
}

type EditKind string

const (
	EditRelocate EditKind = "relocate"
	EditRetire   EditKind = "retire"
	EditGenerate EditKind = "generate"
)

// Relocate maps resolved source declarations to resolved target declarations
// in Destination. Containment is optional only because a top-level move has no
// enclosing object; when supplied, it names the full extraction relation.
type Relocate struct {
	Source      string       `json:"source"`
	Destination Destination  `json:"destination"`
	Subjects    []Relocation `json:"subjects"`
	Containment *Containment `json:"containment,omitempty"`
}

// Destination binds the physical target file and its exact Go package clause.
// The package clause is authored, never inferred from a filename.
type Destination struct {
	Path    string `json:"path"`
	Package string `json:"package"`
}

// Relocation maps one old resolved declaration to exactly one new resolved
// declaration. Source-only symbols are deliberately not representable.
type Relocation struct {
	From SymbolRef `json:"from"`
	To   SymbolRef `json:"to"`
}

// Retire removes exact resolved objects from one source file.  Retiring a
// complete file still lists its objects, so the resolver can prove no hidden
// public surface was discarded.
type Retire struct {
	Source  string      `json:"source"`
	Symbols []SymbolRef `json:"symbols"`
}

// Generate writes Destination through one registered executor provider.
// Provider is a symbolic registry key, never a command line or shell text.
// Generic cutplan validates the key grammar; the executor's immutable provider
// registry validates that the named provider is available.
type Generate struct {
	Provider    Provider `json:"provider"`
	Inputs      []string `json:"inputs,omitempty"`
	Destination string   `json:"destination"`
}

type Provider string

// SymbolRef names one resolved Go object in a stable, unambiguous form:
//
//	import/path#package:Name
//	import/path#type:Receiver/field:Name
//	import/path#type:Receiver/method:Name
//
// Its source coordinates and all references are generated evidence in Lock.
type SymbolRef struct {
	Object string `json:"object"`
}

// Containment states an extraction exactly: Parent is the source owner, Child
// is the target container, and Through is the inserted target containment
// field. All three are resolved objects, so field ownership is never inferred.
type Containment struct {
	Parent  SymbolRef `json:"parent"`
	Child   SymbolRef `json:"child"`
	Through SymbolRef `json:"through"`
}

// Binding rewrites one exact consumer from an old resolved object to the new
// one.  Receiver contains only resolved direct field/view steps and never an
// arbitrary expression.
type Binding struct {
	Consumer string             `json:"consumer"`
	From     SymbolRef          `json:"from"`
	To       SymbolRef          `json:"to"`
	Form     BindingForm        `json:"form"`
	Receiver []ReceiverPathStep `json:"receiver,omitempty"`
}

type BindingForm string

const (
	BindingDirect          BindingForm = "direct"
	BindingField           BindingForm = "field"
	BindingMethodCall      BindingForm = "method-call"
	BindingPackageSelector BindingForm = "package-selector"
)

type ReceiverPathStep struct {
	Kind   ReceiverPathKind `json:"kind"`
	Object SymbolRef        `json:"object"`
}

type ReceiverPathKind string

const (
	ReceiverField      ReceiverPathKind = "field"
	ReceiverDirectView ReceiverPathKind = "direct-view"
)

// Import is an exact per-consumer import replacement.  Nil From means add;
// nil To means remove.  It is not an empty-marker convention: one endpoint is
// concrete and every moved object using the edge is named.
type Import struct {
	Consumer string      `json:"consumer"`
	From     *ImportRef  `json:"from,omitempty"`
	To       *ImportRef  `json:"to,omitempty"`
	Symbols  []SymbolRef `json:"symbols"`
}

type ImportRef struct {
	// Path is the canonical import path. Name is the imported package's
	// declared clause, never inferred from Path. Alias is the exact explicit
	// import spelling from Go syntax: it is empty for `import "path"` and is
	// never a derived effective identifier. Consumers use Alias when nonempty,
	// otherwise Name. Dot and blank aliases are outside this vocabulary.
	Path  string `json:"path"`
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

// Footprint is the exact read/write graph footprint.  Read paths receive byte
// fingerprints; writes receive output hashes or deletion evidence.  A write
// not in Read is required to be absent before the cut.
type Footprint struct {
	Read  []string `json:"read"`
	Write []string `json:"write"`
}

// Verification contains only bounded, structured checks.  The executor owns
// the bounded runner; no field carries a shell fragment.
type Verification struct {
	Laws  []Law  `json:"laws"`
	Gates []Gate `json:"gates"`
}

// Law names one exact top-level Go test. It is deliberately not a regex and
// has no runner/environment/limit controls.
type Law struct {
	ID      string `json:"id"`
	Package string `json:"package"`
	Test    string `json:"test"`
}

type Gate string

const (
	GateDiagnostics Gate = "diagnostics"
	GateImportDAG   Gate = "import-dag"
	GateResidue     Gate = "object-residue"
)

// HashPath binds one exact repository-relative path to SHA-256 bytes.
type HashPath struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// InputFingerprint is generated preflight state.  Files are exactly the read
// footprint; Absent is exactly Write minus Read.
type InputFingerprint struct {
	Files  []HashPath `json:"files"`
	Absent []string   `json:"absent"`
}

// ObjectEvidence is generated resolver evidence for one SymbolRef.
type ObjectEvidence struct {
	Object     SymbolRef  `json:"object"`
	Role       ObjectRole `json:"role"`
	Package    string     `json:"package"`
	Definition Position   `json:"definition"`
	References []Position `json:"references"`
}

// ObjectRole commits whether resolver evidence was obtained for a pre-cut or
// post-cut object. The two sets may not overlap in one flash cut.
type ObjectRole string

const (
	ObjectSource ObjectRole = "source"
	ObjectTarget ObjectRole = "target"
)

// ResolutionRequirement is the deterministic derived resolver denominator for
// an Intent. Empty Path/Package means that this object is still classified but
// not a relocation subject with a forced declaration location.
type ResolutionRequirement struct {
	Object  SymbolRef  `json:"object"`
	Role    ObjectRole `json:"role"`
	Path    string     `json:"path,omitempty"`
	Package string     `json:"package,omitempty"`
}

type Position struct {
	// PackageIDs is the complete sorted set of go/packages variants that
	// produced this one physical semantic site. It is generated resolver
	// evidence, never authored cut intent. It is empty only for non-semantic
	// positions such as loader diagnostics.
	PackageIDs []string `json:"package_ids,omitempty"`
	Path       string   `json:"path"`
	Offset     int      `json:"offset"`
	Line       int      `json:"line"`
	Column     int      `json:"column"`
	Role       SiteRole `json:"role,omitempty"`
}

// SiteRole classifies the AST form of a generated semantic site. It is not a
// user-facing language vocabulary: it only prevents declaration, ordinary
// use, selector, and import evidence from being silently conflated.
type SiteRole string

const (
	SiteDeclaration SiteRole = "declaration"
	SiteUse         SiteRole = "use"
	SiteSelector    SiteRole = "selector"
	SiteImport      SiteRole = "import"
)

// ResolutionEvidence is generated, never authored inside Intent.
type ResolutionEvidence struct {
	Objects   []ObjectEvidence   `json:"objects"`
	Providers []ProviderEvidence `json:"providers"`
}

// ReferenceSiteRoute is one exact resolved source site and the exact target
// site to which the renderer moved it. Sites include declarations as well as
// uses; a count is never accepted as substitute evidence.
type ReferenceSiteRoute struct {
	Source Position `json:"source"`
	Target Position `json:"target"`
}

// ReferenceRoute is generated post-render evidence for one declared
// relocation subject. It has no authored counterpart: its required From/To
// denominator is derived only from Intent and ResolutionRequirements.
type ReferenceRoute struct {
	From  SymbolRef            `json:"from"`
	To    SymbolRef            `json:"to"`
	Sites []ReferenceSiteRoute `json:"sites"`
}

// GateEvidence commits the canonical successful result of one requested
// structural gate. The gate implementation is bound by Toolchain; the result
// digest binds its normalized evidence without introducing gate-specific
// authored payloads.
type GateEvidence struct {
	Gate         Gate   `json:"gate"`
	ResultSHA256 string `json:"result_sha256"`
}

// ProviderEvidence is generated from the executor's immutable provider
// registry. It binds a provider key to its implementation identity without
// letting Intent carry execution details.
type ProviderEvidence struct {
	Name     Provider `json:"name"`
	Identity string   `json:"identity"`
}

// ExecutionEvidence is generated by dry run/apply and binds the exact diff.
type ExecutionEvidence struct {
	Touched    []string   `json:"touched"`
	Outputs    []HashPath `json:"outputs"`
	Deleted    []string   `json:"deleted"`
	DiffSHA256 string     `json:"diff_sha256"`
}

type Hazard struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Detail   string   `json:"detail"`
	Paths    []string `json:"paths"`
}

type Toolchain struct {
	HelperBuild        string `json:"helper_build"`
	HelperSHA256       string `json:"helper_sha256"`
	GoVersion          string `json:"go_version"`
	GoExecutableSHA256 string `json:"go_executable_sha256"`
	Resolver           string `json:"resolver"`
	BuildEnvSHA256     string `json:"build_env_sha256"`
	ModuleGraphSHA256  string `json:"module_graph_sha256"`
}

// LockEvidence separates generated input, resolution, and execution facts
// from reviewed intent.  It cannot provide a second authored move vocabulary.
type LockEvidence struct {
	Inputs     InputFingerprint   `json:"inputs"`
	Resolution ResolutionEvidence `json:"resolution"`
	Routes     []ReferenceRoute   `json:"routes"`
	Gates      []GateEvidence     `json:"gates"`
	Execution  ExecutionEvidence  `json:"execution"`
	Hazards    []Hazard           `json:"hazards"`
}

// Lock is the sole input an executor may apply.
type Lock struct {
	Schema       int          `json:"schema"`
	Intent       Intent       `json:"intent"`
	IntentSHA256 string       `json:"intent_sha256"`
	Toolchain    Toolchain    `json:"toolchain"`
	Evidence     LockEvidence `json:"evidence"`
}
