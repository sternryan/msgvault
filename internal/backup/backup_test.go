package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mutecomm/go-sqlcipher/v4"
	"github.com/wesm/msgvault/internal/config"
	"github.com/wesm/msgvault/internal/store"
)

// setupTestStore creates a store in a temp directory with a messages table and sample data.
func setupTestStore(t *testing.T, dataDir string) *store.Store {
	t.Helper()

	dbPath := filepath.Join(dataDir, "msgvault.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	if err := s.InitSchema(); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	return s
}

// setupTestConfig creates a config pointing to the given data directory.
func setupTestConfig(t *testing.T, dataDir string) *config.Config {
	t.Helper()
	return &config.Config{
		HomeDir: dataDir,
		Data: config.DataConfig{
			DataDir: dataDir,
		},
	}
}

func TestBackupCreatesValidDatabase(t *testing.T) {
	dataDir := t.TempDir()
	cfg := setupTestConfig(t, dataDir)
	s := setupTestStore(t, dataDir)
	defer s.Close()

	backupDir := t.TempDir()
	opts := DefaultBackupOptions()
	opts.OutputDir = filepath.Join(backupDir, "test-backup")

	path, err := Backup(context.Background(), s, cfg, opts)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Verify backup database exists and is valid
	dbPath := filepath.Join(path, "msgvault.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("backup db not found: %v", err)
	}

	// Open and verify integrity
	if err := integrityCheck(dbPath, ""); err != nil {
		t.Fatalf("integrity check failed: %v", err)
	}
}

func TestManifestJSONRoundTrip(t *testing.T) {
	manifest := &BackupManifest{
		Version:             1,
		CreatedAt:           time.Now().Truncate(time.Second),
		Hostname:            "test-host",
		DatabaseHash:        "abc123def456",
		MessageCount:        42,
		DatabaseSize:        1024,
		IncludesTokens:      true,
		IncludesDeletions:   false,
		IncludesAttachments: true,
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded BackupManifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Version != manifest.Version {
		t.Errorf("version: got %d, want %d", decoded.Version, manifest.Version)
	}
	if decoded.Hostname != manifest.Hostname {
		t.Errorf("hostname: got %q, want %q", decoded.Hostname, manifest.Hostname)
	}
	if decoded.DatabaseHash != manifest.DatabaseHash {
		t.Errorf("hash: got %q, want %q", decoded.DatabaseHash, manifest.DatabaseHash)
	}
	if decoded.MessageCount != manifest.MessageCount {
		t.Errorf("count: got %d, want %d", decoded.MessageCount, manifest.MessageCount)
	}
	if decoded.DatabaseSize != manifest.DatabaseSize {
		t.Errorf("size: got %d, want %d", decoded.DatabaseSize, manifest.DatabaseSize)
	}
	if decoded.IncludesTokens != manifest.IncludesTokens {
		t.Errorf("tokens: got %v, want %v", decoded.IncludesTokens, manifest.IncludesTokens)
	}
	if decoded.IncludesDeletions != manifest.IncludesDeletions {
		t.Errorf("deletions: got %v, want %v", decoded.IncludesDeletions, manifest.IncludesDeletions)
	}
	if decoded.IncludesAttachments != manifest.IncludesAttachments {
		t.Errorf("attachments: got %v, want %v", decoded.IncludesAttachments, manifest.IncludesAttachments)
	}
	if !decoded.CreatedAt.Equal(manifest.CreatedAt) {
		t.Errorf("created_at: got %v, want %v", decoded.CreatedAt, manifest.CreatedAt)
	}
}

func TestSHA256Verification(t *testing.T) {
	dataDir := t.TempDir()
	cfg := setupTestConfig(t, dataDir)
	s := setupTestStore(t, dataDir)
	defer s.Close()

	backupDir := t.TempDir()
	opts := DefaultBackupOptions()
	opts.OutputDir = filepath.Join(backupDir, "test-backup")

	path, err := Backup(context.Background(), s, cfg, opts)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	// Verify hash matches
	dbPath := filepath.Join(path, "msgvault.db")
	actualHash, err := hashFile(dbPath)
	if err != nil {
		t.Fatalf("hash file: %v", err)
	}

	if actualHash != manifest.DatabaseHash {
		t.Errorf("hash mismatch: got %q, want %q", actualHash, manifest.DatabaseHash)
	}

	// Corrupt the file and verify hash doesn't match
	f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("open for corruption: %v", err)
	}
	f.Write([]byte("corrupted"))
	f.Close()

	corruptedHash, err := hashFile(dbPath)
	if err != nil {
		t.Fatalf("hash corrupted file: %v", err)
	}

	if corruptedHash == manifest.DatabaseHash {
		t.Error("hash should not match after corruption")
	}
}

func TestRestoreVerifyOnly(t *testing.T) {
	dataDir := t.TempDir()
	cfg := setupTestConfig(t, dataDir)
	s := setupTestStore(t, dataDir)
	defer s.Close()

	backupDir := t.TempDir()
	opts := DefaultBackupOptions()
	opts.OutputDir = filepath.Join(backupDir, "test-backup")

	path, err := Backup(context.Background(), s, cfg, opts)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Close the store so we can use a fresh config for restore
	s.Close()

	// Verify-only should succeed without modifying anything
	restoreDir := t.TempDir()
	restoreCfg := setupTestConfig(t, restoreDir)

	err = Restore(context.Background(), path, restoreCfg, &RestoreOptions{VerifyOnly: true})
	if err != nil {
		t.Fatalf("verify-only restore: %v", err)
	}

	// Database should NOT have been copied
	restoredDB := filepath.Join(restoreDir, "msgvault.db")
	if _, err := os.Stat(restoredDB); !os.IsNotExist(err) {
		t.Error("verify-only should not copy database")
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	// Setup: create store, insert data
	dataDir := t.TempDir()
	cfg := setupTestConfig(t, dataDir)
	s := setupTestStore(t, dataDir)

	// Create some test tokens and attachments
	tokensDir := filepath.Join(dataDir, "tokens")
	os.MkdirAll(tokensDir, 0755)
	os.WriteFile(filepath.Join(tokensDir, "test@gmail.com.json"), []byte(`{"token":"test"}`), 0600)

	attachDir := filepath.Join(dataDir, "attachments")
	os.MkdirAll(attachDir, 0755)
	os.WriteFile(filepath.Join(attachDir, "abc123"), []byte("attachment data"), 0644)

	deletionsDir := filepath.Join(dataDir, "deletions")
	os.MkdirAll(filepath.Join(deletionsDir, "pending"), 0755)
	os.WriteFile(filepath.Join(deletionsDir, "pending", "test.json"), []byte(`{"id":"test"}`), 0644)

	// Backup
	backupDir := t.TempDir()
	opts := DefaultBackupOptions()
	opts.OutputDir = filepath.Join(backupDir, "test-backup")

	backupPath, err := Backup(context.Background(), s, cfg, opts)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	s.Close()

	// Verify manifest
	manifest, err := LoadManifest(backupPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.Version != 1 {
		t.Errorf("manifest version: got %d, want 1", manifest.Version)
	}
	if !manifest.IncludesTokens {
		t.Error("manifest should include tokens")
	}
	if !manifest.IncludesDeletions {
		t.Error("manifest should include deletions")
	}
	if !manifest.IncludesAttachments {
		t.Error("manifest should include attachments")
	}

	// Restore to new directory
	restoreDir := t.TempDir()
	restoreCfg := setupTestConfig(t, restoreDir)

	err = Restore(context.Background(), backupPath, restoreCfg, &RestoreOptions{Force: true})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	// Verify restored database
	restoredDBPath := filepath.Join(restoreDir, "msgvault.db")
	restoredDB, err := sql.Open("sqlite3", restoredDBPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer restoredDB.Close()

	var result string
	if err := restoredDB.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		t.Fatalf("integrity check: %v", err)
	}
	if result != "ok" {
		t.Errorf("integrity check: %s", result)
	}

	// Verify token was restored
	tokenData, err := os.ReadFile(filepath.Join(restoreDir, "tokens", "test@gmail.com.json"))
	if err != nil {
		t.Fatalf("read restored token: %v", err)
	}
	if string(tokenData) != `{"token":"test"}` {
		t.Errorf("token data: got %q", string(tokenData))
	}

	// Verify attachment was restored
	attachData, err := os.ReadFile(filepath.Join(restoreDir, "attachments", "abc123"))
	if err != nil {
		t.Fatalf("read restored attachment: %v", err)
	}
	if string(attachData) != "attachment data" {
		t.Errorf("attachment data: got %q", string(attachData))
	}

	// Verify deletion manifest was restored
	delData, err := os.ReadFile(filepath.Join(restoreDir, "deletions", "pending", "test.json"))
	if err != nil {
		t.Fatalf("read restored deletion: %v", err)
	}
	if string(delData) != `{"id":"test"}` {
		t.Errorf("deletion data: got %q", string(delData))
	}
}

func TestBackupTarGz(t *testing.T) {
	dataDir := t.TempDir()
	cfg := setupTestConfig(t, dataDir)
	s := setupTestStore(t, dataDir)
	defer s.Close()

	backupDir := t.TempDir()
	opts := DefaultBackupOptions()
	opts.OutputDir = filepath.Join(backupDir, "test-backup")
	opts.Tar = true

	path, err := Backup(context.Background(), s, cfg, opts)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Should be a .tar.gz file
	if filepath.Ext(path) != ".gz" {
		t.Errorf("expected .tar.gz path, got %s", path)
	}

	// Directory should have been removed
	if _, err := os.Stat(filepath.Join(backupDir, "test-backup")); !os.IsNotExist(err) {
		t.Error("backup directory should have been removed after tar")
	}

	// Restore from tar.gz should work
	restoreDir := t.TempDir()
	restoreCfg := setupTestConfig(t, restoreDir)

	err = Restore(context.Background(), path, restoreCfg, &RestoreOptions{VerifyOnly: true})
	if err != nil {
		t.Fatalf("verify tar.gz backup: %v", err)
	}
}

func TestRestoreRefusesWithoutForce(t *testing.T) {
	dataDir := t.TempDir()
	cfg := setupTestConfig(t, dataDir)
	s := setupTestStore(t, dataDir)

	backupDir := t.TempDir()
	opts := DefaultBackupOptions()
	opts.OutputDir = filepath.Join(backupDir, "test-backup")

	path, err := Backup(context.Background(), s, cfg, opts)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	s.Close()

	// Try to restore to same location without --force
	err = Restore(context.Background(), path, cfg, &RestoreOptions{})
	if err == nil {
		t.Fatal("expected error when restoring over existing data without --force")
	}
}

func TestVerifyMessagesForDeletion_AllPresent(t *testing.T) {
	dataDir := t.TempDir()
	s := setupTestStore(t, dataDir)
	defer s.Close()

	// Create source and conversation
	source, err := s.GetOrCreateSource("gmail", "test@example.com")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	convID, err := s.EnsureConversation(source.ID, "thread-1", "Thread 1")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	// Insert messages with raw MIME
	rawMIME := []byte("From: test@example.com\r\nSubject: Test\r\n\r\nBody")
	gmailIDs := []string{"gmail-1", "gmail-2", "gmail-3"}
	for _, gid := range gmailIDs {
		msgID, err := s.UpsertMessage(&store.Message{
			ConversationID:  convID,
			SourceID:        source.ID,
			SourceMessageID: gid,
			MessageType:     "email",
			SizeEstimate:    100,
		})
		if err != nil {
			t.Fatalf("upsert message %s: %v", gid, err)
		}
		if err := s.UpsertMessageRaw(msgID, rawMIME); err != nil {
			t.Fatalf("upsert raw %s: %v", gid, err)
		}
	}

	result, err := VerifyMessagesForDeletion(context.Background(), s, gmailIDs)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if result.HasIssues() {
		t.Errorf("expected no issues, got: %s", result.Summary())
	}
	if result.Verified != 3 {
		t.Errorf("verified = %d, want 3", result.Verified)
	}
}

func TestVerifyMessagesForDeletion_MissingRaw(t *testing.T) {
	dataDir := t.TempDir()
	s := setupTestStore(t, dataDir)
	defer s.Close()

	source, err := s.GetOrCreateSource("gmail", "test@example.com")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	convID, err := s.EnsureConversation(source.ID, "thread-1", "Thread 1")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	// Insert message WITHOUT raw MIME
	_, err = s.UpsertMessage(&store.Message{
		ConversationID:  convID,
		SourceID:        source.ID,
		SourceMessageID: "gmail-no-raw",
		MessageType:     "email",
		SizeEstimate:    100,
	})
	if err != nil {
		t.Fatalf("upsert message: %v", err)
	}

	result, err := VerifyMessagesForDeletion(context.Background(), s, []string{"gmail-no-raw"})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !result.HasIssues() {
		t.Error("expected issues for missing raw MIME")
	}
	if len(result.MissingRaw) != 1 {
		t.Errorf("MissingRaw = %d, want 1", len(result.MissingRaw))
	}
	if result.MissingRaw[0] != "gmail-no-raw" {
		t.Errorf("MissingRaw[0] = %q, want %q", result.MissingRaw[0], "gmail-no-raw")
	}
	if result.Verified != 0 {
		t.Errorf("verified = %d, want 0", result.Verified)
	}
}

func TestVerifyMessagesForDeletion_MissingMeta(t *testing.T) {
	dataDir := t.TempDir()
	s := setupTestStore(t, dataDir)
	defer s.Close()

	// Verify gmail IDs that don't exist in DB at all
	result, err := VerifyMessagesForDeletion(context.Background(), s, []string{"nonexistent-1", "nonexistent-2"})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if !result.HasIssues() {
		t.Error("expected issues for missing messages")
	}
	if len(result.MissingMeta) != 2 {
		t.Errorf("MissingMeta = %d, want 2", len(result.MissingMeta))
	}
	if result.Verified != 0 {
		t.Errorf("verified = %d, want 0", result.Verified)
	}
}

func TestVerifyMessagesForDeletion_Empty(t *testing.T) {
	dataDir := t.TempDir()
	s := setupTestStore(t, dataDir)
	defer s.Close()

	result, err := VerifyMessagesForDeletion(context.Background(), s, []string{})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if result.HasIssues() {
		t.Error("expected no issues for empty input")
	}
	if result.Verified != 0 {
		t.Errorf("verified = %d, want 0", result.Verified)
	}
}

func TestIncrementalAttachmentBackup(t *testing.T) {
	dataDir := t.TempDir()
	cfg := setupTestConfig(t, dataDir)
	s := setupTestStore(t, dataDir)

	// Create attachments
	attachDir := filepath.Join(dataDir, "attachments")
	os.MkdirAll(attachDir, 0755)
	os.WriteFile(filepath.Join(attachDir, "file1"), []byte("data1"), 0644)
	os.WriteFile(filepath.Join(attachDir, "file2"), []byte("data2"), 0644)

	// First backup
	backupDir := t.TempDir()
	opts := DefaultBackupOptions()
	opts.OutputDir = filepath.Join(backupDir, "backup1")

	path1, err := Backup(context.Background(), s, cfg, opts)
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}

	// Add new attachment
	os.WriteFile(filepath.Join(attachDir, "file3"), []byte("data3"), 0644)

	// Incremental backup to same location
	opts2 := DefaultBackupOptions()
	opts2.OutputDir = filepath.Join(backupDir, "backup2")
	opts2.Incremental = true

	path2, err := Backup(context.Background(), s, cfg, opts2)
	if err != nil {
		t.Fatalf("incremental backup: %v", err)
	}
	s.Close()

	// All three files should exist in backup2
	for _, name := range []string{"file1", "file2", "file3"} {
		p := filepath.Join(path2, "attachments", name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing attachment %s in incremental backup", name)
		}
	}

	// Verify both backups are valid
	for _, p := range []string{path1, path2} {
		if err := Restore(context.Background(), p, setupTestConfig(t, t.TempDir()), &RestoreOptions{VerifyOnly: true}); err != nil {
			t.Errorf("verify backup %s: %v", p, err)
		}
	}
}
