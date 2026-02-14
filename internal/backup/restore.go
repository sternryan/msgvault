package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/wesm/msgvault/internal/config"
)

// RestoreOptions configures a restore operation.
type RestoreOptions struct {
	VerifyOnly bool   // Just verify, don't restore
	Force      bool   // Overwrite existing data
	Passphrase string // Database encryption passphrase (for integrity check)
}

// Restore restores a backup to the msgvault data directory.
func Restore(ctx context.Context, backupPath string, cfg *config.Config, opts *RestoreOptions) error {
	if opts == nil {
		opts = &RestoreOptions{}
	}

	// Handle tar.gz: extract to temp dir first
	extractedDir := ""
	if strings.HasSuffix(backupPath, ".tar.gz") || strings.HasSuffix(backupPath, ".tgz") {
		tmpDir, err := os.MkdirTemp("", "msgvault-restore-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		if err := extractTarGz(backupPath, tmpDir); err != nil {
			return fmt.Errorf("extract tar.gz: %w", err)
		}

		// Find the backup directory inside the extracted path
		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			return fmt.Errorf("read extracted dir: %w", err)
		}
		if len(entries) == 1 && entries[0].IsDir() {
			extractedDir = filepath.Join(tmpDir, entries[0].Name())
		} else {
			extractedDir = tmpDir
		}
		backupPath = extractedDir
	}

	// Load and verify manifest
	manifest, err := LoadManifest(backupPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Verify database checksum
	dbPath := filepath.Join(backupPath, "msgvault.db")
	actualHash, err := hashFile(dbPath)
	if err != nil {
		return fmt.Errorf("hash backup database: %w", err)
	}
	if actualHash != manifest.DatabaseHash {
		return fmt.Errorf("database checksum mismatch: expected %s, got %s", manifest.DatabaseHash, actualHash)
	}

	// Run integrity check on backup database
	if err := integrityCheck(dbPath, opts.Passphrase); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}

	if opts.VerifyOnly {
		return nil
	}

	// Check for existing data
	destDB := cfg.DatabaseDSN()
	if !opts.Force {
		if _, err := os.Stat(destDB); err == nil {
			return fmt.Errorf("existing database found at %s; use --force to overwrite", destDB)
		}
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destDB), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Copy database
	if err := copyFile(dbPath, destDB); err != nil {
		return fmt.Errorf("restore database: %w", err)
	}

	// Copy tokens
	if manifest.IncludesTokens {
		tokensDir := filepath.Join(backupPath, "tokens")
		if dirExists(tokensDir) {
			if err := copyDir(tokensDir, cfg.TokensDir()); err != nil {
				return fmt.Errorf("restore tokens: %w", err)
			}
		}
	}

	// Copy deletions
	if manifest.IncludesDeletions {
		deletionsDir := filepath.Join(backupPath, "deletions")
		if dirExists(deletionsDir) {
			if err := copyDir(deletionsDir, cfg.DeletionsDir()); err != nil {
				return fmt.Errorf("restore deletions: %w", err)
			}
		}
	}

	// Copy attachments
	if manifest.IncludesAttachments {
		attachDir := filepath.Join(backupPath, "attachments")
		if dirExists(attachDir) {
			if err := copyDir(attachDir, cfg.AttachmentsDir()); err != nil {
				return fmt.Errorf("restore attachments: %w", err)
			}
		}
	}

	return nil
}

// LoadManifest loads a backup manifest from a directory.
func LoadManifest(backupDir string) (*BackupManifest, error) {
	manifestPath := filepath.Join(backupDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return &manifest, nil
}

// integrityCheck runs PRAGMA integrity_check on a database.
func integrityCheck(dbPath string, passphrase string) error {
	dsn := dbPath + "?mode=ro"
	if passphrase != "" {
		dsn = dbPath + "?mode=ro&_pragma_key=" + url.QueryEscape(passphrase)
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("run integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity check result: %s", result)
	}
	return nil
}

// extractTarGz extracts a tar.gz archive to the given directory.
func extractTarGz(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Sanitize the path to prevent directory traversal
		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(destDir) {
			return fmt.Errorf("invalid tar entry path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}
