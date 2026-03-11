# Deferred Items — Phase 09-polish

Pre-existing `go vet` failures (not caused by 09-01 changes):

1. `internal/export/attachments_test.go:88`: not enough arguments in call to Attachments
2. `internal/mbox/client_test.go:93`: undefined: writeTempMbox
3. `cmd/msgvault/cmd/validation_test.go:33`: TestEmailValidation redeclared

These existed before Phase 09-01 execution. Out of scope.
