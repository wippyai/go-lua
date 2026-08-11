// Package render compiles a reviewed flash-cut intent into an exact in-memory
// post-state. It has no filesystem, process, lock, or test capability: those
// boundaries belong to semantic collection, transaction, and verification.
//
// Rendering is fail-closed. Every source selection is a go/types object from
// the pre-cut semantic Workspace, every output belongs to Footprint.Write, and
// unsupported syntax is rejected before a VirtualFile is returned.
package render
