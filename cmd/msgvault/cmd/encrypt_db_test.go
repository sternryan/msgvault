package cmd

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

func TestExportDatabaseEncrypt(t *testing.T) {
	// Create a plain database with some data
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "plain.db")

	srcDB, err := sql.Open("sqlite3", srcPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = srcDB.Exec(`
		CREATE TABLE messages (id INTEGER PRIMARY KEY, text TEXT);
		INSERT INTO messages (text) VALUES ('hello'), ('world');
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Export to encrypted
	encPath := filepath.Join(dir, "encrypted.db")
	if err := exportDatabase(srcDB, encPath, "testpass"); err != nil {
		t.Fatal(err)
	}
	srcDB.Close()

	// Verify encrypted DB can be opened with passphrase
	encDB, err := sql.Open("sqlite3", encPath+"?_pragma_key=testpass")
	if err != nil {
		t.Fatal(err)
	}

	var count int
	if err := encDB.QueryRow("SELECT count(*) FROM messages").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 messages, got %d", count)
	}
	encDB.Close()

	// Verify encrypted DB cannot be opened without passphrase
	badDB, err := sql.Open("sqlite3", encPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := badDB.QueryRow("SELECT count(*) FROM messages").Scan(&count); err == nil {
		t.Error("expected error opening encrypted DB without passphrase")
	}
	badDB.Close()
}

func TestExportDatabaseDecrypt(t *testing.T) {
	// Create an encrypted database via export
	dir := t.TempDir()
	plainPath := filepath.Join(dir, "plain.db")

	plainDB, err := sql.Open("sqlite3", plainPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = plainDB.Exec(`
		CREATE TABLE messages (id INTEGER PRIMARY KEY, text TEXT);
		INSERT INTO messages (text) VALUES ('encrypted data');
	`)
	if err != nil {
		t.Fatal(err)
	}

	encPath := filepath.Join(dir, "encrypted.db")
	if err := exportDatabase(plainDB, encPath, "mypass"); err != nil {
		t.Fatal(err)
	}
	plainDB.Close()

	// Open encrypted DB and export back to plaintext
	encDB, err := sql.Open("sqlite3", encPath+"?_pragma_key=mypass")
	if err != nil {
		t.Fatal(err)
	}

	decPath := filepath.Join(dir, "decrypted.db")
	if err := exportDatabase(encDB, decPath, ""); err != nil {
		t.Fatal(err)
	}
	encDB.Close()

	// Verify decrypted DB is readable without passphrase
	decDB, err := sql.Open("sqlite3", decPath)
	if err != nil {
		t.Fatal(err)
	}
	defer decDB.Close()

	var text string
	if err := decDB.QueryRow("SELECT text FROM messages WHERE id = 1").Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "encrypted data" {
		t.Errorf("expected 'encrypted data', got %q", text)
	}
}

func TestVerifyEncryptedDB(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")

	srcDB, err := sql.Open("sqlite3", srcPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = srcDB.Exec(`
		CREATE TABLE messages (id INTEGER PRIMARY KEY, text TEXT);
		INSERT INTO messages (text) VALUES ('test');
	`)
	if err != nil {
		t.Fatal(err)
	}

	encPath := filepath.Join(dir, "enc.db")
	if err := exportDatabase(srcDB, encPath, "pass123"); err != nil {
		t.Fatal(err)
	}
	srcDB.Close()

	// Should succeed with correct passphrase
	if err := verifyEncryptedDB(encPath, "pass123"); err != nil {
		t.Errorf("verify should succeed: %v", err)
	}

	// Should fail with wrong passphrase
	if err := verifyEncryptedDB(encPath, "wrongpass"); err == nil {
		t.Error("verify should fail with wrong passphrase")
	}
}

func TestVerifyPlainDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE messages (id INTEGER PRIMARY KEY, text TEXT);
		INSERT INTO messages (text) VALUES ('hello');
	`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := verifyPlainDB(dbPath); err != nil {
		t.Errorf("verify plain DB should succeed: %v", err)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	// Full round-trip: create plain DB → encrypt → decrypt → verify data
	dir := t.TempDir()

	// Step 1: Create plain DB with realistic data
	plainPath := filepath.Join(dir, "original.db")
	db, err := sql.Open("sqlite3", plainPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE messages (id INTEGER PRIMARY KEY, subject TEXT, body TEXT);
		CREATE TABLE labels (id TEXT PRIMARY KEY, name TEXT);
		INSERT INTO messages (subject, body) VALUES
			('Hello World', 'This is a test message body'),
			('Important', 'Very important content here'),
			('Meeting Notes', 'Discussed quarterly goals');
		INSERT INTO labels (id, name) VALUES ('INBOX', 'Inbox'), ('SENT', 'Sent');
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Step 2: Encrypt
	encPath := filepath.Join(dir, "encrypted.db")
	if err := exportDatabase(db, encPath, "roundtrip-pass"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Step 3: Verify encrypted
	if err := verifyEncryptedDB(encPath, "roundtrip-pass"); err != nil {
		t.Fatalf("encrypted verification failed: %v", err)
	}

	// Step 4: Decrypt
	encDB, err := sql.Open("sqlite3", encPath+"?_pragma_key=roundtrip-pass")
	if err != nil {
		t.Fatal(err)
	}
	decPath := filepath.Join(dir, "decrypted.db")
	if err := exportDatabase(encDB, decPath, ""); err != nil {
		t.Fatal(err)
	}
	encDB.Close()

	// Step 5: Verify decrypted
	if err := verifyPlainDB(decPath); err != nil {
		t.Fatalf("decrypted verification failed: %v", err)
	}

	// Step 6: Compare data
	decDB, err := sql.Open("sqlite3", decPath)
	if err != nil {
		t.Fatal(err)
	}
	defer decDB.Close()

	var msgCount, labelCount int
	decDB.QueryRow("SELECT count(*) FROM messages").Scan(&msgCount)
	decDB.QueryRow("SELECT count(*) FROM labels").Scan(&labelCount)

	if msgCount != 3 {
		t.Errorf("expected 3 messages, got %d", msgCount)
	}
	if labelCount != 2 {
		t.Errorf("expected 2 labels, got %d", labelCount)
	}

	// Verify specific content survived round-trip
	var subject string
	if err := decDB.QueryRow("SELECT subject FROM messages WHERE id = 2").Scan(&subject); err != nil {
		t.Fatal(err)
	}
	if subject != "Important" {
		t.Errorf("expected subject 'Important', got %q", subject)
	}
}

func TestChangePassphrase(t *testing.T) {
	// Test PRAGMA rekey functionality
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")

	// Create plain DB
	srcDB, err := sql.Open("sqlite3", srcPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = srcDB.Exec(`
		CREATE TABLE messages (id INTEGER PRIMARY KEY, text TEXT);
		INSERT INTO messages (text) VALUES ('secret data');
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt it
	encPath := filepath.Join(dir, "enc.db")
	if err := exportDatabase(srcDB, encPath, "oldpass"); err != nil {
		t.Fatal(err)
	}
	srcDB.Close()

	// Open with old passphrase and rekey
	db, err := sql.Open("sqlite3", encPath+"?_pragma_key=oldpass")
	if err != nil {
		t.Fatal(err)
	}

	// Verify access
	var count int
	if err := db.QueryRow("SELECT count(*) FROM messages").Scan(&count); err != nil {
		t.Fatalf("should open with old pass: %v", err)
	}

	// Rekey
	if _, err := db.Exec("PRAGMA rekey = 'newpass'"); err != nil {
		t.Fatalf("rekey failed: %v", err)
	}
	db.Close()

	// Should NOT open with old passphrase
	badDB, err := sql.Open("sqlite3", encPath+"?_pragma_key=oldpass")
	if err != nil {
		t.Fatal(err)
	}
	if err := badDB.QueryRow("SELECT count(*) FROM messages").Scan(&count); err == nil {
		t.Error("should not open with old passphrase after rekey")
	}
	badDB.Close()

	// Should open with new passphrase
	newDB, err := sql.Open("sqlite3", encPath+"?_pragma_key=newpass")
	if err != nil {
		t.Fatal(err)
	}
	defer newDB.Close()

	var text string
	if err := newDB.QueryRow("SELECT text FROM messages WHERE id = 1").Scan(&text); err != nil {
		t.Fatalf("should open with new pass: %v", err)
	}
	if text != "secret data" {
		t.Errorf("expected 'secret data', got %q", text)
	}
}
