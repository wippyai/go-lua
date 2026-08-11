// Package accessgeometry owns Flow's derived table/access geometry.
//
// The package consumes only committed Source and authored Flow views plus the
// candidate projection.  It retains normalized Source-owned key handles and
// the compact candidate indexed-access relation; it does not retain an owner graph,
// source payload, or a second key authority.
package accessgeometry
