package cutplan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// BuildLock binds reviewed intent to generated resolver and dry-run evidence.
// It cannot fill an omitted edit, route, footprint, test, gate, or object.
func BuildLock(intent Intent, toolchain Toolchain, evidence LockEvidence) (Lock, error) {
	if err := ValidateIntent(intent); err != nil {
		return Lock{}, err
	}
	if err := validateToolchain(toolchain); err != nil {
		return Lock{}, err
	}
	if err := ValidateEvidence(intent, evidence); err != nil {
		return Lock{}, err
	}
	digest, err := IntentDigest(intent)
	if err != nil {
		return Lock{}, err
	}
	canonical, err := CanonicalIntent(intent)
	if err != nil {
		return Lock{}, err
	}
	return Lock{
		Schema: Version, Intent: canonical, IntentSHA256: digest,
		Toolchain: toolchain, Evidence: canonicalEvidence(evidence),
	}, nil
}

// ValidateLock checks immutable lock shape. VerifyLock additionally compares
// exact input preconditions to the current checkout before any mutation.
func ValidateLock(lock Lock) error {
	if lock.Schema != Version {
		return fmt.Errorf("lock schema must be %d", Version)
	}
	if err := ValidateIntent(lock.Intent); err != nil {
		return fmt.Errorf("lock intent: %w", err)
	}
	digest, err := IntentDigest(lock.Intent)
	if err != nil {
		return err
	}
	if lock.IntentSHA256 != digest {
		return fmt.Errorf("lock intent digest mismatch")
	}
	if err := validateToolchain(lock.Toolchain); err != nil {
		return err
	}
	return ValidateEvidence(lock.Intent, lock.Evidence)
}

func VerifyLock(root string, lock Lock) error {
	if err := ValidateLock(lock); err != nil {
		return err
	}
	for _, input := range lock.Evidence.Inputs.Files {
		full, err := existingFile(root, input.Path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("locked input %s: %w", input.Path, err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != input.SHA256 {
			return fmt.Errorf("locked input changed: %s", input.Path)
		}
	}
	for _, absent := range lock.Evidence.Inputs.Absent {
		if err := absentDestination(root, absent); err != nil {
			return err
		}
	}
	return nil
}

// VerifyOutputs verifies a completed dry run or apply against the committed
// output bytes and deletion set.
func VerifyOutputs(root string, lock Lock) error {
	if err := ValidateLock(lock); err != nil {
		return err
	}
	for _, output := range lock.Evidence.Execution.Outputs {
		full, err := existingFile(root, output.Path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("locked output %s: %w", output.Path, err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != output.SHA256 {
			return fmt.Errorf("locked output changed: %s", output.Path)
		}
	}
	for _, deleted := range lock.Evidence.Execution.Deleted {
		if err := absentDestination(root, deleted); err != nil {
			return err
		}
	}
	return nil
}

func VerifyDiff(diff []byte, lock Lock) error {
	if err := ValidateLock(lock); err != nil {
		return err
	}
	digest := sha256.Sum256(diff)
	if hex.EncodeToString(digest[:]) != lock.Evidence.Execution.DiffSHA256 {
		return fmt.Errorf("locked diff changed")
	}
	return nil
}

// ValidateEvidence proves that the generated lock evidence is complete for
// the reviewed exact footprint.  It never derives missing evidence itself.
func ValidateEvidence(intent Intent, evidence LockEvidence) error {
	if err := ValidateInputs(evidence.Inputs); err != nil {
		return err
	}
	if !sameStrings(inputPaths(evidence.Inputs), ReadPaths(intent)) {
		return fmt.Errorf("input fingerprint paths do not equal intent read footprint")
	}
	if !sameStrings(evidence.Inputs.Absent, createdPaths(intent)) {
		return fmt.Errorf("absent paths do not equal write-minus-read footprint")
	}
	if err := validateResolution(intent, evidence.Resolution); err != nil {
		return err
	}
	if err := validateReferenceRoutes(intent, evidence.Resolution.Objects, evidence.Routes); err != nil {
		return err
	}
	if err := validateGateEvidence(intent, evidence.Gates); err != nil {
		return err
	}
	if err := validateHazards(evidence.Hazards); err != nil {
		return err
	}
	return validateExecution(intent, evidence.Execution)
}

func validateToolchain(toolchain Toolchain) error {
	if !safeToolVersion(toolchain.HelperBuild) ||
		!sha256Pattern.MatchString(toolchain.HelperSHA256) ||
		!safeToolVersion(toolchain.GoVersion) ||
		!sha256Pattern.MatchString(toolchain.GoExecutableSHA256) ||
		!safeToolVersion(toolchain.Resolver) ||
		!sha256Pattern.MatchString(toolchain.BuildEnvSHA256) ||
		!sha256Pattern.MatchString(toolchain.ModuleGraphSHA256) {
		return fmt.Errorf("toolchain must bind helper build/hash, Go version/executable hash, resolver, normalized build environment, and module graph")
	}
	return nil
}

func validateResolution(intent Intent, evidence ResolutionEvidence) error {
	requirements, err := ResolutionRequirements(intent)
	if err != nil {
		return err
	}
	wanted := make(map[string]ResolutionRequirement, len(requirements))
	for _, requirement := range requirements {
		wanted[requirement.Object.Object] = requirement
	}
	seen := map[string]bool{}
	for _, object := range evidence.Objects {
		if !safeSymbolRef(object.Object) || !safePackageClause(object.Package) {
			return fmt.Errorf("invalid resolved object")
		}
		want, declared := wanted[object.Object.Object]
		if !declared {
			return fmt.Errorf("resolution object not declared in intent: %s", object.Object.Object)
		}
		if object.Role != want.Role {
			return fmt.Errorf("resolution object has wrong %s/%s classification: %s", object.Role, want.Role, object.Object.Object)
		}
		if want.Path != "" && object.Definition.Path != want.Path {
			return fmt.Errorf("%s evidence does not match declared %s for %s", want.Role, roleLocation(want.Role), object.Object.Object)
		}
		if want.Package != "" && object.Package != want.Package {
			return fmt.Errorf("target evidence does not match declared destination for %s", object.Object.Object)
		}
		if seen[object.Object.Object] {
			return fmt.Errorf("duplicate object evidence: %s", object.Object.Object)
		}
		seen[object.Object.Object] = true
		if err := validateSemanticSite(object.Definition); err != nil {
			return fmt.Errorf("definition %s: %w", object.Object.Object, err)
		}
		if object.Definition.Role != SiteDeclaration {
			return fmt.Errorf("definition %s is not a declaration site", object.Object.Object)
		}
		positions := map[string]bool{}
		for _, reference := range object.References {
			if err := validateSemanticSite(reference); err != nil {
				return fmt.Errorf("reference %s: %w", object.Object.Object, err)
			}
			if reference.Role == SiteDeclaration {
				return fmt.Errorf("reference %s duplicates its declaration site", object.Object.Object)
			}
			key := positionKey(reference)
			if positions[key] {
				return fmt.Errorf("duplicate reference for %s", object.Object.Object)
			}
			positions[key] = true
		}
	}
	for object := range wanted {
		if !seen[object] {
			return fmt.Errorf("missing object evidence: %s", object)
		}
	}
	return validateProviders(intent, evidence.Providers)
}

func roleLocation(role ObjectRole) string {
	if role == ObjectTarget {
		return "destination"
	}
	return "source"
}

func validateProviders(intent Intent, providers []ProviderEvidence) error {
	wanted := map[Provider]bool{}
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			if edit.Kind == EditGenerate {
				wanted[edit.Generate.Provider] = true
			}
		}
	}
	seen := map[Provider]bool{}
	for _, provider := range providers {
		if !safeProvider(provider.Name) || !safeToolVersion(provider.Identity) {
			return fmt.Errorf("invalid provider evidence")
		}
		if !wanted[provider.Name] {
			return fmt.Errorf("provider evidence is not used by intent: %s", provider.Name)
		}
		if seen[provider.Name] {
			return fmt.Errorf("duplicate provider evidence: %s", provider.Name)
		}
		seen[provider.Name] = true
	}
	for provider := range wanted {
		if !seen[provider] {
			return fmt.Errorf("generator provider is not registered by executor: %s", provider)
		}
	}
	return nil
}

func validateReferenceRoutes(intent Intent, objects []ObjectEvidence, routes []ReferenceRoute) error {
	requirements, err := ReferenceRouteRequirements(intent)
	if err != nil {
		return err
	}
	wanted := make(map[string]ReferenceRoute, len(requirements))
	for _, requirement := range requirements {
		wanted[routeKey(requirement.From, requirement.To)] = requirement
	}
	evidence := make(map[string]ObjectEvidence, len(objects))
	for _, object := range objects {
		evidence[object.Object.Object] = object
	}
	seenRoutes := map[string]bool{}
	seenSource := map[string]bool{}
	seenTarget := map[string]bool{}
	for _, route := range routes {
		key := routeKey(route.From, route.To)
		if _, declared := wanted[key]; !declared {
			return fmt.Errorf("reference route is not a relocation subject: %s", key)
		}
		if seenRoutes[key] {
			return fmt.Errorf("duplicate reference route: %s", key)
		}
		seenRoutes[key] = true
		if len(route.Sites) == 0 {
			return fmt.Errorf("reference route has no sites: %s", key)
		}
		source, exists := evidence[route.From.Object]
		if !exists || source.Role != ObjectSource {
			return fmt.Errorf("reference route source has no source evidence: %s", route.From.Object)
		}
		target, exists := evidence[route.To.Object]
		if !exists || target.Role != ObjectTarget {
			return fmt.Errorf("reference route target has no target evidence: %s", route.To.Object)
		}
		expectedSource, err := evidenceSiteSet(source)
		if err != nil {
			return fmt.Errorf("route source %s: %w", route.From.Object, err)
		}
		expectedTarget, err := evidenceSiteSet(target)
		if err != nil {
			return fmt.Errorf("route target %s: %w", route.To.Object, err)
		}
		routedSource := map[string]bool{}
		routedTarget := map[string]bool{}
		for _, site := range route.Sites {
			if err := validateSemanticSite(site.Source); err != nil {
				return fmt.Errorf("reference route source position: %w", err)
			}
			if err := validateSemanticSite(site.Target); err != nil {
				return fmt.Errorf("reference route target position: %w", err)
			}
			sourceKey, targetKey := positionKey(site.Source), positionKey(site.Target)
			if !expectedSource[sourceKey] {
				return fmt.Errorf("reference route source site is not resolved to %s: %s", route.From.Object, sourceKey)
			}
			if !expectedTarget[targetKey] {
				return fmt.Errorf("reference route target site is not resolved to %s: %s", route.To.Object, targetKey)
			}
			if routedSource[sourceKey] || seenSource[sourceKey] {
				return fmt.Errorf("reference route source site is not unique: %s", sourceKey)
			}
			if routedTarget[targetKey] || seenTarget[targetKey] {
				return fmt.Errorf("reference route target site is not unique: %s", targetKey)
			}
			if site.Source.Role != site.Target.Role {
				return fmt.Errorf("reference route changes semantic site role: %s to %s", site.Source.Role, site.Target.Role)
			}
			routedSource[sourceKey], routedTarget[targetKey] = true, true
			seenSource[sourceKey], seenTarget[targetKey] = true, true
		}
		if !samePositionSets(routedSource, expectedSource) {
			return fmt.Errorf("reference route does not cover every source site for %s", route.From.Object)
		}
		if !samePositionSets(routedTarget, expectedTarget) {
			return fmt.Errorf("reference route does not cover every target site for %s", route.To.Object)
		}
	}
	for key := range wanted {
		if !seenRoutes[key] {
			return fmt.Errorf("missing reference route for relocation subject: %s", key)
		}
	}
	return nil
}

func evidenceSiteSet(object ObjectEvidence) (map[string]bool, error) {
	result := map[string]bool{}
	if err := validateSemanticSite(object.Definition); err != nil {
		return nil, fmt.Errorf("definition: %w", err)
	}
	result[positionKey(object.Definition)] = true
	for _, reference := range object.References {
		if err := validateSemanticSite(reference); err != nil {
			return nil, fmt.Errorf("reference: %w", err)
		}
		result[positionKey(reference)] = true
	}
	return result, nil
}

func samePositionSets(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}

func validateGateEvidence(intent Intent, gates []GateEvidence) error {
	required, err := GateRequirements(intent)
	if err != nil {
		return err
	}
	wanted := map[Gate]bool{}
	for _, gate := range required {
		wanted[gate] = true
	}
	seen := map[Gate]bool{}
	for _, evidence := range gates {
		if !knownGate(evidence.Gate) || !sha256Pattern.MatchString(evidence.ResultSHA256) {
			return fmt.Errorf("invalid gate evidence")
		}
		if !wanted[evidence.Gate] {
			return fmt.Errorf("gate evidence was not requested: %s", evidence.Gate)
		}
		if seen[evidence.Gate] {
			return fmt.Errorf("duplicate gate evidence: %s", evidence.Gate)
		}
		seen[evidence.Gate] = true
	}
	for _, gate := range required {
		if !seen[gate] {
			return fmt.Errorf("missing gate evidence: %s", gate)
		}
	}
	return nil
}

func validateHazards(hazards []Hazard) error {
	seen := map[string]bool{}
	for _, hazard := range hazards {
		if !safeLabel(hazard.Code) || (hazard.Severity != "warning" && hazard.Severity != "error") || hazard.Detail == "" || hasForbidden(hazard.Detail) {
			return fmt.Errorf("invalid hazard")
		}
		if err := exactPaths("hazard path", hazard.Paths); err != nil {
			return err
		}
		key := hazardKey(hazard)
		if seen[key] {
			return fmt.Errorf("duplicate hazard %s", hazard.Code)
		}
		seen[key] = true
		if hazard.Severity == "error" {
			return fmt.Errorf("blocking hazard %s: %s", hazard.Code, hazard.Detail)
		}
	}
	return nil
}

func validateExecution(intent Intent, execution ExecutionEvidence) error {
	if !sha256Pattern.MatchString(execution.DiffSHA256) {
		return fmt.Errorf("invalid execution diff digest")
	}
	if err := exactPaths("touched path", execution.Touched); err != nil {
		return err
	}
	if err := exactPaths("deleted path", execution.Deleted); err != nil {
		return err
	}
	if len(execution.Outputs) == 0 && len(execution.Deleted) == 0 {
		return fmt.Errorf("execution has no outputs or deletes")
	}
	seen := map[string]bool{}
	for _, output := range execution.Outputs {
		if !safePath(output.Path) || !sha256Pattern.MatchString(output.SHA256) {
			return fmt.Errorf("invalid output hash")
		}
		if seen[output.Path] {
			return fmt.Errorf("duplicate output hash %s", output.Path)
		}
		seen[output.Path] = true
	}
	executed := append(hashPaths(execution.Outputs), execution.Deleted...)
	if !sameStrings(execution.Touched, executed) {
		return fmt.Errorf("touched files do not equal hashed outputs plus deletes")
	}
	if !sameStrings(execution.Touched, WritePaths(intent)) {
		return fmt.Errorf("execution paths do not equal intent write footprint")
	}
	if !sameStrings(execution.Deleted, retiredPaths(intent)) {
		return fmt.Errorf("execution deletes do not equal retired paths")
	}
	return nil
}

func inputPaths(input InputFingerprint) []string {
	result := make([]string, 0, len(input.Files))
	for _, file := range input.Files {
		result = append(result, file.Path)
	}
	return result
}

func hashPaths(values []HashPath) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Path)
	}
	return result
}
