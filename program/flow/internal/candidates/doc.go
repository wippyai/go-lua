// Package candidates seals the small, typed operator/control-candidate projection
// used by later Flow assembly.
//
// Candidates are existing authored Terms only.  The projection has no
// synthetic rows, generic candidate registry, branch/handler selection, or
// persistence representation.  Executability is the sole reachability gate;
// the authored operator/access relation supplies the candidate family. A
// GenericLoop bucket contains executable authored GenericFor rows whose
// header Values relation has at least one fixed member.
package candidates
