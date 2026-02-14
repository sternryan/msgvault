// Package backup provides backup and restore operations for msgvault data.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/mutecomm/go-sqlcipher/v4"
	"github.com/wesm/msgvault/internal/config"
	"github.com/wesm/msgvault/internal/store"
)

// BackupManifest contains metadata about a backup.
type BackupManifest struct {
	Version             int       `json:"version"`
	CreatedAt           time.Time `json:"created_at"`
	Hostname            string    `json:"hostname,omitempty"`
	DatabaseHash        string    `json:"database_hash"`
	MessageCount        int64     `json:"message_count"`
	DatabaseSize        int64     `json:"database_size"`
	IncludesTokens      bool      `json:"includes_tokens"`
	IncludesDeletions   bool      `json:"includes_deletions"`
	IncludesAttachments bool      `json:"includes_attachments"`
}

// BackupOptions configures a backup operation.
type BackupOptions struct {
	OutputDir        string // Target directory (default: ~/.msgvault/backups/backup-{timestamp}/)
	Tar              bool   // Create .tar.gz instead of directory
	Incremental      bool   // Only copy new attachments
	IncludeTokens    bool   // Include OAuth tokens
	IncludeDeletions bool   // Include deletion manifests
}

// DefaultBackupOptions returns the default backup options.
func DefaultBackupOptions() *BackupOptions {
	return &BackupOptions{
		IncludeTokens:    true,
		IncludeDeletions: true,
	}
}

// Backup performs a full backup of the msgvault data.
// Uses SQLite's online backup API for atomic, consistent DB snapshots.
// Returns the path to the backup.
func Backup(ctx context.Context, st *store.Store, cfg *config.Config, opts *BackupOptions) (string, error) {
	if opts == nil {
		opts = DefaultBackupOptions()
	}

	// Determine output directory
	outputDir := opts.OutputDir
	if outputDir == "" {
		ts := time.Now().Format("20060102-150405")
		outputDir = filepath.Join(cfg.BackupsDir(), "backup-"+ts)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	// Backup database using SQLite backup API
	dbDest := filepath.Join(outputDir, "msgvault.db")
	if err := backupDatabase(st.DB(), dbDest, st.Passphrase()); err != nil {
		return "", fmt.Errorf("backup database: %w", err)
	}

	// Compute SHA-256 of backed-up database
	dbHash, err := hashFile(dbDest)
	if err != nil {
		return "", fmt.Errorf("hash database: %w", err)
	}

	// Get database size
	var dbSize int64
	if info, err := os.Stat(dbDest); err == nil {
		dbSize = info.Size()
	}

	// Get message count from backup DB
	msgCount, err := countMessages(dbDest, st.Passphrase())
	if err != nil {
		return "", fmt.Errorf("count messages: %w", err)
	}

	// Build manifest
	hostname, _ := os.Hostname()
	manifest := &BackupManifest{
		Version:      1,
		CreatedAt:    time.Now(),
		Hostname:     hostname,
		DatabaseHash: dbHash,
		MessageCount: msgCount,
		DatabaseSize: dbSize,
	}

	// Copy tokens
	if opts.IncludeTokens {
		tokensDir := cfg.TokensDir()
		if dirExists(tokensDir) {
			dest := filepath.Join(outputDir, "tokens")
			if err := copyDir(tokensDir, dest); err != nil {
				return "", fmt.Errorf("copy tokens: %w", err)
			}
			manifest.IncludesTokens = true
		}
	}

	// Copy deletions
	if opts.IncludeDeletions {
		deletionsDir := cfg.DeletionsDir()
		if dirExists(deletionsDir) {
			dest := filepath.Join(outputDir, "deletions")
			if err := copyDir(deletionsDir, dest); err != nil {
				return "", fmt.Errorf("copy deletions: %w", err)
			}
			manifest.IncludesDeletions = true
		}
	}

	// Copy attachments (content-addressed, supports incremental)
	attachDir := cfg.AttachmentsDir()
	if dirExists(attachDir) {
		dest := filepath.Join(outputDir, "attachments")
		if opts.Incremental {
			if err := copyDirIncremental(attachDir, dest); err != nil {
				return "", fmt.Errorf("copy attachments: %w", err)
			}
		} else {
			if err := copyDir(attachDir, dest); err != nil {
				return "", fmt.Errorf("copy attachments: %w", err)
			}
		}
		manifest.IncludesAttachments = true
	}

	// Write manifest
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}

	// Create tar.gz if requested
	if opts.Tar {
		tarPath := outputDir + ".tar.gz"
		if err := createTarGz(tarPath, outputDir); err != nil {
			return "", fmt.Errorf("create tar.gz: %w", err)
		}
		// Remove the directory after successful tar creation
		if err := os.RemoveAll(outputDir); err != nil {
			return "", fmt.Errorf("remove backup dir after tar: %w", err)
		}
		return tarPath, nil
	}

	return outputDir, nil
}

// backupDatabase uses SQLite's online backup API for an atomic snapshot.
func backupDatabase(srcDB *sql.DB, destPath string, passphrase string) error {
	destDSN := destPath
	if passphrase != "" {
		destDSN = destPath + "?_pragma_key=" + url.QueryEscape(passphrase)
	}
	destDB, err := sql.Open("sqlite3", destDSN)
	if err != nil {
		return fmt.Errorf("open dest db: %w", err)
	}
	defer destDB.Close()

	// Ensure destination is initialized
	if err := destDB.Ping(); err != nil {
		return fmt.Errorf("ping dest db: %w", err)
	}

	srcConn, err := srcDB.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("get src conn: %w", err)
	}
	defer srcConn.Close()

	destConn, err := destDB.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("get dest conn: %w", err)
	}
	defer destConn.Close()

	var backupErr error
	err = srcConn.Raw(func(srcRaw interface{}) error {
		return destConn.Raw(func(destRaw interface{}) error {
			srcSQLite, ok := srcRaw.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("source is not *sqlite3.SQLiteConn")
			}
			destSQLite, ok := destRaw.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("dest is not *sqlite3.SQLiteConn")
			}

			backup, err := destSQLite.Backup("main", srcSQLite, "main")
			if err != nil {
				return fmt.Errorf("init backup: %w", err)
			}

			done, err := backup.Step(-1)
			if err != nil {
				backup.Finish()
				return fmt.Errorf("backup step: %w", err)
			}
			if !done {
				backup.Finish()
				return fmt.Errorf("backup not complete after step(-1)")
			}

			backupErr = backup.Finish()
			return backupErr
		})
	})

	if err != nil {
		return err
	}
	return backupErr
}

// hashFile computes the SHA-256 hash of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// countMessages opens the backup DB and counts messages.
func countMessages(dbPath string, passphrase string) (int64, error) {
	dsn := dbPath + "?mode=ro"
	if passphrase != "" {
		dsn = dbPath + "?mode=ro&_pragma_key=" + url.QueryEscape(passphrase)
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var count int64
	err = db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	if err != nil {
		// Table may not exist in an empty DB
		return 0, nil
	}
	return count, nil
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

// copyDirIncremental copies files that don't already exist in the destination.
// Useful for content-addressed attachments where filename = hash.
func copyDirIncremental(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Skip if destination file already exists (content-addressed)
		if _, err := os.Stat(destPath); err == nil {
			return nil
		}

		return copyFile(path, destPath)
	})
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcF.Close()

	srcInfo, err := srcF.Stat()
	if err != nil {
		return err
	}

	dstF, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstF.Close()

	_, err = io.Copy(dstF, srcF)
	return err
}

// createTarGz creates a tar.gz archive from a directory.
func createTarGz(tarPath, srcDir string) error {
	f, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	baseDir := filepath.Base(srcDir)

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.Join(baseDir, rel)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}
