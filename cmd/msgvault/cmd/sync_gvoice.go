// MERGE TODO: This file used the HEAD-side gvoice connector API.
// During the upstream merge we adopted upstream's (more battle-tested)
// gvoice package, which has a different surface (no BatchDeleteMessages,
// different client constructors). Upstream wires Voice through
// cmd/msgvault/cmd/import_gvoice.go instead of a sync command.
//
// Decide between:
//   1. Delete this file and rely on upstream's import_gvoice.go.
//   2. Reimplement sync-gvoice on top of the upstream gvoice package
//      and add the missing gmail.API methods or a thin shim.
//
// Until that decision is made the file is build-tag-disabled so the
// rest of the binary compiles.

//go:build merge_todo_sync_gvoice

package cmd
