package equation

// GraphBuildPhase is the one internal phase witness returned by Topology.Graph
// and compileTopology. It is deliberately a value-only construction result:
// no builder, graph, domain callback, or diagnostic text crosses the compiler
// boundary. The engine maps these phases to its detached SolveReport enum.
type GraphBuildPhase uint8

const (
	GraphBuildPhaseNone GraphBuildPhase = iota
	GraphBuildPhaseAcceptedValidation
	GraphBuildPhaseTemplateExpansion
	GraphBuildPhaseRevision
	GraphBuildPhaseCompileValidation
	GraphBuildPhaseCompilePoints
	GraphBuildPhaseCompileCatalog
	GraphBuildPhaseCompileInstances
	GraphBuildPhaseCompileGroups
	GraphBuildPhaseCompileEnvironment
	GraphBuildPhaseCompileFactor
	GraphBuildPhaseCompileQuery
	GraphBuildPhaseCompileAssemble
)
