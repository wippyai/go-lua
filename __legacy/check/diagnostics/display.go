package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

const (
	unknownSourceName = "assigned value"
	maxInlineTypeLen  = 96
)

func selectedChannelPathEvidence(channel string) string {
	return "branch chain checks channel " + codeName(channel)
}

func handledChannelCasesEvidence(cases string) string {
	return "handled cases: " + cases
}

func missingChannelCasesEvidence(cases string) string {
	return "missing cases: " + cases
}

func missingChannelDefaultEvidence() string {
	return "no default case handles the remaining channel cases"
}

func selectedDiscriminantPathEvidence(path string) string {
	return "branch chain checks discriminant " + codeName(path)
}

func possibleDiscriminantCasesEvidence(cases string) string {
	return "possible cases: " + cases
}

func handledDiscriminantCasesEvidence(cases string) string {
	return "handled cases: " + cases
}

func missingDiscriminantCasesEvidence(cases string) string {
	return "missing cases: " + cases
}

func missingDiscriminantDefaultEvidence() string {
	return "no default branch handles the remaining union cases"
}

func resultShapeUnionEvidence(receiver, discriminant string) string {
	return codeName(receiver) + " is a union discriminated by " + codeName(discriminant)
}

func resultShapeFieldCaseEvidence(readPath, requiredCase string) string {
	return codeName(readPath) + " exists only for " + codeName(requiredCase)
}

func resultShapeMissingProofEvidence(requiredCase string) string {
	return "no stable guard proves " + codeName(requiredCase) + " before this read"
}

func selectedOptionalPathEvidence(path string) string {
	return "branch checks optional " + codeName(path)
}

func optionalPossibleCasesEvidence(path string) string {
	return "possible cases: " + codeName(path+" ~= nil") + ", " + codeName(path+" == nil")
}

func optionalConsumedCaseEvidence(path string) string {
	return "consumed case: " + codeName(path+" ~= nil")
}

func optionalMissingCasesEvidence(cases string) string {
	return "missing cases: " + cases
}

func optionalMissingDefaultEvidence() string {
	return "no else branch handles the remaining optional case"
}

func dispatchLookupEvidence(table, discriminant string) string {
	return codeName(table) + " is indexed by discriminant " + codeName(discriminant)
}

func dispatchTableKeysEvidence(keys string) string {
	return "dispatch table provides keys: " + keys
}

func missingDispatchKeysEvidence(keys string) string {
	return "missing dispatch keys: " + keys
}

func registrationDispatchEvidence(registry, discriminant string) string {
	return codeName(registry) + " is dispatched with discriminant " + codeName(discriminant)
}

func registeredCasesEvidence(cases string) string {
	return "registered cases: " + cases
}

func missingRegistrationsEvidence(registrations string) string {
	return "missing registrations: " + registrations
}

func frozenAssignmentEvidence(containerName string) string {
	return fmt.Sprintf("this assignment mutates table %q", containerName)
}

func frozenCallMutationEvidence(containerName string) string {
	return fmt.Sprintf("this call mutates table %q", containerName)
}

func frozenIncomingStateEvidence(containerName string) string {
	return fmt.Sprintf("table %q is already frozen here", containerName)
}

func frozenAssignmentProofEvidence(containerName string) string {
	return fmt.Sprintf("table %q was frozen by this call before the assignment", containerName)
}

func frozenCallProofEvidence(containerName string) string {
	return fmt.Sprintf("table %q was frozen by this call before the mutating call", containerName)
}

func operandLabel(side string) string {
	if side == "" {
		return "operand"
	}
	return side + " operand"
}

type diagnosticDisplay struct{}

var display diagnosticDisplay

func argumentTypeMismatchHelpForEvidence(subject string, argName string, got typ.Type, evidence []diagnostic.Evidence) string {
	if evidenceNeedsValidationProof(got, evidence) {
		return display.ArgumentValidationProofHelp(argName)
	}
	if argName != "" && argName != unknownSourceName {
		return fmt.Sprintf("Pass `%s` as a value compatible with the parameter type, or change the callee signature if that argument is valid.", argName)
	}
	if subject != "" {
		return fmt.Sprintf("Pass a value for %s that satisfies the parameter type, or change the callee signature if that argument is valid.", subject)
	}
	return "Pass a value compatible with the parameter type, or change the callee signature if that argument is valid."
}

func evidenceNeedsValidationProof(got typ.Type, evidence []diagnostic.Evidence) bool {
	if typ.AbsentOrTopLike(got) {
		return true
	}
	for _, item := range evidence {
		if item.Kind == diagnostic.EvidencePrecisionBoundary && item.Reason == diagnostic.EvidenceReasonExplicitBoundaryValidation {
			return true
		}
		if item.Kind == diagnostic.EvidenceUserAssertion && item.Reason == diagnostic.EvidenceReasonUserAssertedAny {
			return true
		}
	}
	return false
}

func codeName(name string) string {
	if name == "" {
		return ""
	}
	return "`" + strings.ReplaceAll(name, "`", "\\`") + "`"
}

func typeDisplayOr(displayType, fallback string) string {
	displayType = strings.TrimSpace(displayType)
	if displayType != "" {
		return displayType
	}
	return fallback
}

func displayTypeWithoutNil(displayType string) string {
	displayType = strings.TrimSpace(displayType)
	if strings.HasSuffix(displayType, "?") {
		return strings.TrimSpace(strings.TrimSuffix(displayType, "?"))
	}
	return displayType
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
