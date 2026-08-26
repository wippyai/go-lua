// Package database owns the immutable W2 relation-state publication root.
//
// Store columns and arrangement indexes are separate child packages because
// the index consumes Store.Version. Database is the one owner above both: its
// Bootstrap door accepts only a sealed Mounted witness and Geometry, builds
// all physical columns/store/indexes internally, and captures one complete
// root. Prepared/Commit is the only later publication path; a committed store
// root can never be submitted as a fresh database root.
package database
