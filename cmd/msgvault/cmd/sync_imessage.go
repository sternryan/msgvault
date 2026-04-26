// MERGE TODO: This file used the HEAD-side imessage connector API
// (imessage.WithMyAddress, imessage.NewClient(path, address, ...) etc).
// During the upstream merge we adopted upstream's (more battle-tested)
// imessage package, which has a different surface — including the new
// streamtyped parser. Upstream wires iMessage through
// cmd/msgvault/cmd/import_imessage.go instead of a sync command.
//
// Decide between:
//   1. Delete this file and rely on upstream's import_imessage.go.
//   2. Reimplement sync-imessage on top of the upstream imessage package
//      and add the missing gmail.API methods or a thin shim.
//
// Until that decision is made the file is build-tag-disabled so the
// rest of the binary compiles.

//go:build merge_todo_sync_imessage

package cmd
