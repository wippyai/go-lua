// Package generate provides the capability-minimal boundary for registered
// flash-refactor generators.  It has no filesystem, process, shell, network,
// or workspace dependency: providers receive only the exact bytes declared by
// a cutplan Generate edit and its destination metadata.
//
// A provider is trusted implementation code, but the registry gives it no
// ambient repository capability.  Render is evaluated twice against deep
// copies; differing bytes, a panic, an unknown provider, or a malformed
// request fail closed before an executor can write anything.
package generate
