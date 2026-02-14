package crypto

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// GetPassphrase returns the encryption passphrase.
// Checks MSGVAULT_PASSPHRASE env var first, then prompts interactively.
func GetPassphrase(prompt string) (string, error) {
	if p := os.Getenv("MSGVAULT_PASSPHRASE"); p != "" {
		return p, nil
	}

	if prompt == "" {
		prompt = "Enter passphrase"
	}

	fmt.Fprintf(os.Stderr, "%s: ", prompt)
	passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // newline after password input
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}

	pass := string(passBytes)
	if pass == "" {
		return "", fmt.Errorf("passphrase cannot be empty")
	}
	return pass, nil
}

// ConfirmPassphrase prompts for a passphrase twice and verifies they match.
func ConfirmPassphrase() (string, error) {
	pass1, err := GetPassphrase("Enter new passphrase")
	if err != nil {
		return "", err
	}

	pass2, err := GetPassphrase("Confirm passphrase")
	if err != nil {
		return "", err
	}

	if pass1 != pass2 {
		return "", fmt.Errorf("passphrases do not match")
	}

	return pass1, nil
}
