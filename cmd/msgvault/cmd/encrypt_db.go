package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/crypto"
	"github.com/wesm/msgvault/internal/store"
)

var encryptDBCmd = &cobra.Command{
	Use:   "encrypt-db",
	Short: "Encrypt the msgvault database with SQLCipher",
	Long: `Encrypt an existing unencrypted msgvault database using SQLCipher.

Prompts for a new passphrase (or reads from MSGVAULT_PASSPHRASE env var),
creates an encrypted copy, verifies it, and swaps the files. The original
unencrypted database is kept as a .unencrypted backup.

After encryption, set [encryption] enabled = true in your config.toml,
or re-run with the MSGVAULT_PASSPHRASE environment variable.

Examples:
  msgvault encrypt-db
  MSGVAULT_PASSPHRASE=secret msgvault encrypt-db`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()

		// Check database exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return fmt.Errorf("database not found at %s", dbPath)
		}

		// Check if already encrypted
		if cfg.Encryption.Enabled {
			return fmt.Errorf("database encryption is already enabled in config; use change-passphrase to change the key")
		}

		// Get new passphrase
		newPass, err := crypto.ConfirmPassphrase()
		if err != nil {
			return fmt.Errorf("get passphrase: %w", err)
		}

		// Open the unencrypted database
		srcDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer srcDB.Close()

		// Verify source is readable
		if _, err := srcDB.Exec("SELECT count(*) FROM sqlite_master"); err != nil {
			return fmt.Errorf("database appears corrupted or already encrypted: %w", err)
		}

		fmt.Println("Encrypting database...")

		// Create encrypted copy using sqlcipher_export
		encPath := dbPath + ".encrypted"
		if err := exportDatabase(srcDB, encPath, newPass); err != nil {
			os.Remove(encPath)
			return fmt.Errorf("encrypt database: %w", err)
		}

		// Verify the encrypted copy
		fmt.Println("Verifying encrypted database...")
		if err := verifyEncryptedDB(encPath, newPass); err != nil {
			os.Remove(encPath)
			return fmt.Errorf("verification failed: %w", err)
		}

		// Swap files: current -> .unencrypted, encrypted -> current
		backupPath := dbPath + ".unencrypted"
		if err := os.Rename(dbPath, backupPath); err != nil {
			os.Remove(encPath)
			return fmt.Errorf("backup original: %w", err)
		}
		if err := os.Rename(encPath, dbPath); err != nil {
			// Try to restore
			os.Rename(backupPath, dbPath)
			return fmt.Errorf("swap encrypted: %w", err)
		}

		// Also move WAL/SHM files if they exist
		for _, suffix := range []string{"-wal", "-shm"} {
			walPath := dbPath + suffix
			if _, err := os.Stat(walPath); err == nil {
				os.Rename(walPath, backupPath+suffix)
			}
		}

		// Update config
		if err := updateEncryptionConfig(true); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update config.toml: %v\n", err)
			fmt.Println("Please manually add to config.toml:")
			fmt.Println("  [encryption]")
			fmt.Println("  enabled = true")
		}

		fmt.Println("\nDatabase encrypted successfully!")
		fmt.Printf("  Backup: %s\n", backupPath)
		fmt.Println("\nSet MSGVAULT_PASSPHRASE or you'll be prompted on each command.")
		return nil
	},
}

var decryptDBCmd = &cobra.Command{
	Use:   "decrypt-db",
	Short: "Decrypt the msgvault database",
	Long: `Decrypt an encrypted msgvault database back to plaintext.

Prompts for the current passphrase, creates a decrypted copy, verifies it,
and swaps the files. The encrypted database is kept as a .encrypted backup.

Examples:
  msgvault decrypt-db
  MSGVAULT_PASSPHRASE=secret msgvault decrypt-db`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()

		// Check database exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return fmt.Errorf("database not found at %s", dbPath)
		}

		// Get current passphrase
		currentPass := passphrase
		if currentPass == "" {
			var err error
			currentPass, err = crypto.GetPassphrase("Enter current passphrase")
			if err != nil {
				return fmt.Errorf("get passphrase: %w", err)
			}
		}

		// Verify we can open with this passphrase
		s, err := store.Open(dbPath, store.WithPassphrase(currentPass))
		if err != nil {
			return fmt.Errorf("cannot open database (wrong passphrase?): %w", err)
		}
		s.Close()

		fmt.Println("Decrypting database...")

		// Open encrypted database
		srcDB, err := sql.Open("sqlite3", dbPath+"?_pragma_key="+currentPass+"&_journal_mode=WAL&_busy_timeout=5000")
		if err != nil {
			return fmt.Errorf("open encrypted database: %w", err)
		}
		defer srcDB.Close()

		// Create decrypted copy
		decPath := dbPath + ".decrypted"
		if err := exportDatabase(srcDB, decPath, ""); err != nil {
			os.Remove(decPath)
			return fmt.Errorf("decrypt database: %w", err)
		}

		// Verify the decrypted copy
		fmt.Println("Verifying decrypted database...")
		if err := verifyPlainDB(decPath); err != nil {
			os.Remove(decPath)
			return fmt.Errorf("verification failed: %w", err)
		}

		// Swap files
		backupPath := dbPath + ".encrypted"
		if err := os.Rename(dbPath, backupPath); err != nil {
			os.Remove(decPath)
			return fmt.Errorf("backup encrypted: %w", err)
		}
		if err := os.Rename(decPath, dbPath); err != nil {
			os.Rename(backupPath, dbPath)
			return fmt.Errorf("swap decrypted: %w", err)
		}

		// Also move WAL/SHM files
		for _, suffix := range []string{"-wal", "-shm"} {
			walPath := dbPath + suffix
			if _, err := os.Stat(walPath); err == nil {
				os.Rename(walPath, backupPath+suffix)
			}
		}

		// Update config
		if err := updateEncryptionConfig(false); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update config.toml: %v\n", err)
			fmt.Println("Please manually set [encryption] enabled = false in config.toml")
		}

		fmt.Println("\nDatabase decrypted successfully!")
		fmt.Printf("  Backup: %s\n", backupPath)
		return nil
	},
}

var changePassphraseCmd = &cobra.Command{
	Use:   "change-passphrase",
	Short: "Change the database encryption passphrase",
	Long: `Change the encryption passphrase for an encrypted msgvault database.

Uses SQLCipher's PRAGMA rekey to re-encrypt the database with a new key.
This is an in-place operation that does not create a temporary copy.

Examples:
  msgvault change-passphrase
  MSGVAULT_PASSPHRASE=oldpass msgvault change-passphrase`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return fmt.Errorf("database not found at %s", dbPath)
		}

		// Get current passphrase
		currentPass := passphrase
		if currentPass == "" {
			var err error
			currentPass, err = crypto.GetPassphrase("Enter current passphrase")
			if err != nil {
				return fmt.Errorf("get passphrase: %w", err)
			}
		}

		// Verify current passphrase
		s, err := store.Open(dbPath, store.WithPassphrase(currentPass))
		if err != nil {
			return fmt.Errorf("cannot open database (wrong passphrase?): %w", err)
		}
		s.Close()

		// Get new passphrase
		newPass, err := crypto.ConfirmPassphrase()
		if err != nil {
			return fmt.Errorf("get new passphrase: %w", err)
		}

		if currentPass == newPass {
			return fmt.Errorf("new passphrase is the same as the current one")
		}

		fmt.Println("Changing passphrase...")

		// Open with current passphrase and rekey
		db, err := sql.Open("sqlite3", dbPath+"?_pragma_key="+currentPass+"&_journal_mode=WAL&_busy_timeout=5000")
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		// Verify access
		if _, err := db.Exec("SELECT count(*) FROM sqlite_master"); err != nil {
			return fmt.Errorf("verify access: %w", err)
		}

		// PRAGMA rekey changes the encryption key in-place
		if _, err := db.Exec(fmt.Sprintf("PRAGMA rekey = '%s'", newPass)); err != nil {
			return fmt.Errorf("rekey database: %w", err)
		}

		// Verify with new passphrase
		if _, err := db.Exec("SELECT count(*) FROM sqlite_master"); err != nil {
			return fmt.Errorf("verify after rekey: %w", err)
		}

		fmt.Println("Passphrase changed successfully!")
		fmt.Println("Remember to update MSGVAULT_PASSPHRASE if you use it.")
		return nil
	},
}

// exportDatabase copies all data from srcDB to a new database at destPath.
// If destPass is non-empty, the destination is encrypted with that passphrase.
// If destPass is empty, the destination is plaintext.
func exportDatabase(srcDB *sql.DB, destPath string, destPass string) error {
	// ATTACH the destination database
	attachQuery := fmt.Sprintf("ATTACH DATABASE '%s' AS export_db", destPath)
	if destPass != "" {
		attachQuery += fmt.Sprintf(" KEY '%s'", destPass)
	} else {
		attachQuery += " KEY ''"
	}

	if _, err := srcDB.Exec(attachQuery); err != nil {
		return fmt.Errorf("attach destination: %w", err)
	}
	defer srcDB.Exec("DETACH DATABASE export_db")

	// Use sqlcipher_export to copy all data
	if _, err := srcDB.Exec("SELECT sqlcipher_export('export_db')"); err != nil {
		return fmt.Errorf("sqlcipher_export: %w", err)
	}

	return nil
}

// verifyEncryptedDB opens an encrypted database and runs integrity checks.
func verifyEncryptedDB(dbPath, pass string) error {
	db, err := sql.Open("sqlite3", dbPath+"?_pragma_key="+pass+"&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	// Verify we can read
	var count int64
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master").Scan(&count); err != nil {
		return fmt.Errorf("read sqlite_master: %w", err)
	}

	// Run integrity check
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check failed: %s", result)
	}

	// Verify message count
	if err := db.QueryRow("SELECT count(*) FROM messages").Scan(&count); err != nil {
		// Table may not exist in empty DB
		return nil
	}
	fmt.Printf("  Verified: %d messages, integrity OK\n", count)

	return nil
}

// verifyPlainDB opens a plaintext database and runs integrity checks.
func verifyPlainDB(dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	var count int64
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master").Scan(&count); err != nil {
		return fmt.Errorf("read sqlite_master: %w", err)
	}

	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check failed: %s", result)
	}

	if err := db.QueryRow("SELECT count(*) FROM messages").Scan(&count); err != nil {
		return nil
	}
	fmt.Printf("  Verified: %d messages, integrity OK\n", count)

	return nil
}

// updateEncryptionConfig updates the [encryption] section in config.toml.
func updateEncryptionConfig(enabled bool) error {
	configPath := filepath.Join(cfg.HomeDir, "config.toml")

	// Read existing config
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		// Create new config with encryption section
		content := fmt.Sprintf("[encryption]\nenabled = %v\n", enabled)
		return os.WriteFile(configPath, []byte(content), 0644)
	}
	if err != nil {
		return err
	}

	content := string(data)

	// Check if [encryption] section exists
	if idx := findSection(content, "[encryption]"); idx >= 0 {
		// Update existing section
		content = updateSectionBool(content, "[encryption]", "enabled", enabled)
	} else {
		// Append new section
		if len(content) > 0 && content[len(content)-1] != '\n' {
			content += "\n"
		}
		content += fmt.Sprintf("\n[encryption]\nenabled = %v\n", enabled)
	}

	return os.WriteFile(configPath, []byte(content), 0644)
}

// findSection returns the index of a TOML section header, or -1.
func findSection(content, section string) int {
	for i := 0; i < len(content); {
		lineEnd := i
		for lineEnd < len(content) && content[lineEnd] != '\n' {
			lineEnd++
		}
		line := content[i:lineEnd]
		// Trim spaces
		trimmed := ""
		for _, c := range line {
			if c != ' ' && c != '\t' && c != '\r' {
				trimmed += string(c)
			}
		}
		if trimmed == section {
			return i
		}
		if lineEnd < len(content) {
			i = lineEnd + 1
		} else {
			break
		}
	}
	return -1
}

// updateSectionBool updates a boolean key in a TOML section.
func updateSectionBool(content, section, key string, value bool) string {
	idx := findSection(content, section)
	if idx < 0 {
		return content
	}

	// Find the key line after the section header
	lines := splitLines(content)
	inSection := false
	for i, line := range lines {
		trimmed := trimSpace(line)
		if trimmed == section {
			inSection = true
			continue
		}
		if inSection {
			if len(trimmed) > 0 && trimmed[0] == '[' {
				// Reached next section without finding key, insert before it
				newLine := fmt.Sprintf("%s = %v", key, value)
				lines = append(lines[:i], append([]string{newLine}, lines[i:]...)...)
				return joinLines(lines)
			}
			if hasPrefix(trimmed, key) {
				lines[i] = fmt.Sprintf("%s = %v", key, value)
				return joinLines(lines)
			}
		}
	}

	// Key not found, append to end of section
	lines = append(lines, fmt.Sprintf("%s = %v", key, value))
	return joinLines(lines)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	if len(result) > 0 && result[len(result)-1] != '\n' {
		result += "\n"
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func init() {
	rootCmd.AddCommand(encryptDBCmd)
	rootCmd.AddCommand(decryptDBCmd)
	rootCmd.AddCommand(changePassphraseCmd)
}
