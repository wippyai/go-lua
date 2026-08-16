// Package recurrence derives the source-control recurrence annotation used by
// Flow assembly.
//
// This package is deliberately not a scheduler and does not build a second
// graph. Sourcecontrol owns topology and reachability; recurrence only
// computes the cyclic components of that already-sealed topology and projects
// exact reset ranges onto the sourcecontrol Arc denominator. Its published
// result contains no coordinates, SCC numbering, sourcecontrol graph, or
// synthetic Mu terms.
package recurrence
