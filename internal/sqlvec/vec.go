// Package sqlvec registers the sqlite-vec extension with go-sqlcipher's SQLite
// build by compiling sqlite-vec.c against go-sqlcipher's sqlite3.h (3.33.0).
//
// This package must be imported with a blank identifier in any binary or test
// that uses vec0 virtual tables:
//
//	import _ "github.com/wesm/msgvault/internal/sqlvec"
package sqlvec

// #cgo CFLAGS: -DSQLITE_CORE -I.
// #cgo linux LDFLAGS: -lm
// #include "sqlite-vec.h"
//
import "C"
import "unsafe"

func init() {
	// Register sqlite-vec as an auto-extension so every new connection
	// created by go-sqlcipher automatically has vec0 available.
	C.sqlite3_auto_extension((*[0]byte)(unsafe.Pointer(C.sqlite3_vec_init)))
}
