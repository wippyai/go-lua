package modulecomposition

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func resolvedImportID(row ResolvedImport) identity.ContentID {
	if !row.link.Available() || !row.sourceModuleKey.Available() || !row.targetModuleKey.Available() ||
		!row.artifactID.Available() || !row.programID.Available() || !row.importID.Available() || !row.requestID.Available() ||
		!row.valueID.Available() || row.requestKey == 0 {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID(
		"analysis/schema/module-composition/resolved-import/v1",
		idPart(row.link), idPart(row.sourceModuleKey), idPart(row.artifactID), idPart(row.programID),
		idPart(row.importID), idPart(row.requestID), idPart(row.valueID), keyPart(uint32(row.requestKey)), idPart(row.targetModuleKey),
	)
	return id
}

func cacheIngressID(row CacheIngress) identity.ContentID {
	if !row.link.Available() || !row.importID.Available() || !row.requestID.Available() || !row.sourceModuleKey.Available() || !row.targetModuleKey.Available() ||
		!row.fromRootID.Available() || !row.toRootID.Available() || !row.actorID.Available() || !row.representativeInstanceID.Available() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID(
		"analysis/schema/module-composition/cache-ingress/v1",
		idPart(row.link), idPart(row.importID), idPart(row.requestID), idPart(row.sourceModuleKey), idPart(row.targetModuleKey),
		idPart(row.fromRootID), idPart(row.toRootID), idPart(row.actorID), idPart(row.representativeInstanceID),
		idPart(row.fromContextID), idPart(row.toContextID),
	)
	return id
}

func moduleCallTransitionID(row ModuleCallTransition) identity.ContentID {
	if !row.link.Available() || !row.cacheIngressID.Available() || !row.sourceModuleKey.Available() || !row.sourcePointID.Available() ||
		!row.artifactID.Available() || !row.programID.Available() || !row.importID.Available() ||
		!row.callID.Available() || !row.generationID.Available() || !row.transitionID.Available() || !row.fromContextID.Available() ||
		!row.toContextID.Available() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID(
		"analysis/schema/module-composition/module-call-transition/v2",
		idPart(row.link), idPart(row.cacheIngressID), idPart(row.sourceModuleKey),
		idPart(row.sourcePointID), idPart(row.artifactID), idPart(row.programID), idPart(row.importID), idPart(row.callID),
		idPart(row.generationID), idPart(row.transitionID), idPart(row.fromContextID), idPart(row.toContextID),
	)
	return id
}

func initGenerationID(row InitGeneration) identity.ContentID {
	if !row.link.Available() || !row.ingressID.Available() || !row.moduleKey.Available() || !row.artifactID.Available() || !row.programID.Available() || !row.bodyID.Available() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID("analysis/schema/module-composition/init-generation/v1", idPart(row.link), idPart(row.ingressID), idPart(row.moduleKey), idPart(row.artifactID), idPart(row.programID), idPart(row.bodyID))
	return id
}

func initOutcomeID(row InitOutcome) identity.ContentID {
	if !row.link.Available() || !row.generationID.Available() || !row.outcomeID.Available() || !admittedOutcome(row.kind) {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID("analysis/schema/module-composition/init-outcome/v1", idPart(row.link), idPart(row.generationID), idPart(row.outcomeID), kindPart(row.kind), ordinalPart(row.ordinal))
	return id
}

func initTerminalID(row InitTerminal) identity.ContentID {
	if !row.link.Available() || !row.generationID.Available() || !row.outcomeID.Available() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID("analysis/schema/module-composition/init-terminal/v1", idPart(row.link), idPart(row.generationID), idPart(row.outcomeID))
	return id
}

func moduleExportCallableOriginKeyID(transitionID, allocationID identity.ContentID) identity.ContentID {
	if !transitionID.Available() || !allocationID.Available() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID(
		"analysis/schema/module-composition/module-export-callable-origin-key/v1",
		idPart(transitionID), idPart(allocationID),
	)
	return id
}

func moduleExportCallableOriginID(row ModuleExportCallableOrigin) identity.ContentID {
	if !row.link.Available() || !row.transitionID.Available() || !row.fromContextID.Available() || !row.toContextID.Available() ||
		!row.generationID.Available() || !row.outcomeID.Available() || !row.entryID.Available() || !row.exportID.Available() ||
		!row.functionID.Available() || !row.allocationID.Available() || !row.bodyID.Available() || !row.bodyContextID.Available() ||
		!row.formalID.Available() || !row.moduleKey.Available() || !row.artifactID.Available() || !row.programID.Available() || !row.kind.valid() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID(
		"analysis/schema/module-composition/module-export-callable-origin/v1",
		idPart(row.link), idPart(row.transitionID), idPart(row.fromContextID), idPart(row.toContextID),
		idPart(row.generationID), idPart(row.outcomeID), idPart(row.entryID), idPart(row.exportID),
		idPart(row.functionID), idPart(row.allocationID), idPart(row.bodyID), idPart(row.bodyContextID),
		idPart(row.formalID), idPart(row.moduleKey), idPart(row.artifactID), idPart(row.programID),
		kindPartModuleExportCallableOrigin(row.kind),
	)
	return id
}

func idPart(id identity.ContentID) []byte { return id[:] }

func keyPart(key uint32) []byte {
	var part [4]byte
	binary.BigEndian.PutUint32(part[:], key)
	return part[:]
}

func kindPart(kind programschema.OutcomeKind) []byte { return []byte{byte(kind)} }

func ordinalPart(ordinal uint32) []byte {
	var part [4]byte
	binary.BigEndian.PutUint32(part[:], ordinal)
	return part[:]
}

func kindPartModuleExportCallableOrigin(kind ModuleExportCallableOriginKind) []byte {
	return []byte{byte(kind)}
}
