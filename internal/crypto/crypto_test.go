package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptToken_RoundTrip(t *testing.T) {
	plaintext := []byte(`{"access_token":"ya29.abc","token_type":"Bearer"}`)
	passphrase := "test-passphrase-123"

	encrypted, err := EncryptToken(plaintext, passphrase)
	if err != nil {
		t.Fatalf("EncryptToken failed: %v", err)
	}

	if bytes.Equal(encrypted, plaintext) {
		t.Fatal("encrypted data should differ from plaintext")
	}

	decrypted, err := DecryptToken(encrypted, passphrase)
	if err != nil {
		t.Fatalf("DecryptToken failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("round-trip mismatch:\n  got:  %s\n  want: %s", decrypted, plaintext)
	}
}

func TestDecryptToken_WrongPassphrase(t *testing.T) {
	plaintext := []byte(`{"access_token":"ya29.abc"}`)

	encrypted, err := EncryptToken(plaintext, "correct-passphrase")
	if err != nil {
		t.Fatalf("EncryptToken failed: %v", err)
	}

	_, err = DecryptToken(encrypted, "wrong-passphrase")
	if err == nil {
		t.Fatal("expected error when decrypting with wrong passphrase")
	}
}

func TestIsEncryptedToken(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"encrypted token", []byte{tokenVersion, 0x01, 0x02}, true},
		{"plain JSON", []byte(`{"access_token":"ya29"}`), false},
		{"empty", []byte{}, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEncryptedToken(tt.data)
			if got != tt.want {
				t.Errorf("IsEncryptedToken(%v) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestEncryptToken_DifferentPassphraseDifferentOutput(t *testing.T) {
	plaintext := []byte(`{"token":"value"}`)

	enc1, err := EncryptToken(plaintext, "passphrase-one")
	if err != nil {
		t.Fatalf("EncryptToken #1 failed: %v", err)
	}

	enc2, err := EncryptToken(plaintext, "passphrase-two")
	if err != nil {
		t.Fatalf("EncryptToken #2 failed: %v", err)
	}

	if bytes.Equal(enc1, enc2) {
		t.Fatal("different passphrases should produce different ciphertext")
	}
}

func TestGetPassphrase_FromEnv(t *testing.T) {
	t.Setenv("MSGVAULT_PASSPHRASE", "env-passphrase-42")

	pass, err := GetPassphrase("")
	if err != nil {
		t.Fatalf("GetPassphrase failed: %v", err)
	}

	if pass != "env-passphrase-42" {
		t.Fatalf("got %q, want %q", pass, "env-passphrase-42")
	}
}
