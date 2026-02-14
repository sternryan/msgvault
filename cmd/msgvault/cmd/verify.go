package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/gmail"
	"github.com/wesm/msgvault/internal/mime"
	"github.com/wesm/msgvault/internal/oauth"
	"github.com/wesm/msgvault/internal/store"
)

var (
	verifySampleSize int
	verifyLocalOnly  bool
)

var verifyCmd = &cobra.Command{
	Use:   "verify [email]",
	Short: "Verify archive integrity against Gmail",
	Long: `Verify the local archive by comparing message counts with Gmail
and sampling messages to ensure raw MIME data is intact.

This command:
1. Compares local message count with Gmail's reported total
2. Checks how many messages have raw MIME data stored
3. Samples random messages and verifies their MIME can be decompressed

Use --local-only to verify the local database without Gmail API access:
- PRAGMA integrity_check
- Message and raw MIME counts
- FTS5 consistency check
- Attachment file verification
- MIME sample verification

Examples:
  msgvault verify you@gmail.com
  msgvault verify you@gmail.com --sample 200
  msgvault verify --local-only`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if verifyLocalOnly {
			return runLocalVerify(cmd)
		}
		if len(args) == 0 {
			return fmt.Errorf("email address required (or use --local-only)")
		}
		return runRemoteVerify(cmd, args[0])
	},
}

// runLocalVerify performs local-only verification without Gmail API access.
func runLocalVerify(cmd *cobra.Command) error {
	// Open database
	dbPath := cfg.DatabaseDSN()
	s, err := store.Open(dbPath, store.WithPassphrase(passphrase))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer s.Close()

	fmt.Printf("Verifying local database: %s\n\n", dbPath)

	// 1. PRAGMA integrity_check
	var integrityResult string
	if err := s.DB().QueryRow("PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	if integrityResult != "ok" {
		fmt.Printf("WARNING: Database integrity check failed: %s\n", integrityResult)
	} else {
		fmt.Println("Database integrity: OK")
	}

	// 2. Message count
	var messageCount int64
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM messages").Scan(&messageCount); err != nil {
		return fmt.Errorf("count messages: %w", err)
	}
	fmt.Printf("Messages:            %10d\n", messageCount)

	// 3. Raw MIME count
	var rawCount int64
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM message_raw").Scan(&rawCount); err != nil {
		return fmt.Errorf("count raw MIME: %w", err)
	}
	rawPct := float64(0)
	if messageCount > 0 {
		rawPct = float64(rawCount) / float64(messageCount) * 100
	}
	fmt.Printf("With raw MIME:       %10d (%.1f%%)\n", rawCount, rawPct)

	// 4. FTS5 consistency check
	var ftsCount, bodiesCount int64
	ftsErr := s.DB().QueryRow("SELECT COUNT(*) FROM messages_fts").Scan(&ftsCount)
	bodiesErr := s.DB().QueryRow("SELECT COUNT(*) FROM message_bodies").Scan(&bodiesCount)
	if ftsErr == nil && bodiesErr == nil {
		if ftsCount != bodiesCount {
			fmt.Printf("WARNING: FTS5 index count (%d) doesn't match message bodies count (%d)\n", ftsCount, bodiesCount)
		} else {
			fmt.Printf("FTS5 index:          %10d (consistent with message_bodies)\n", ftsCount)
		}
	} else {
		fmt.Println("FTS5 index:          not available")
	}

	// 5. Attachment verification
	attachDir := cfg.AttachmentsDir()
	var attachCount int64
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM attachments").Scan(&attachCount); err != nil {
		fmt.Printf("Attachments:         error counting: %v\n", err)
	} else {
		fmt.Printf("Attachments:         %10d\n", attachCount)

		if attachCount > 0 {
			// Check that attachment files exist on disk
			rows, err := s.DB().Query("SELECT content_hash, storage_path, size FROM attachments WHERE content_hash IS NOT NULL")
			if err != nil {
				fmt.Printf("  WARNING: could not query attachments: %v\n", err)
			} else {
				var checked, missing, sizeMismatch int
				for rows.Next() {
					var hash, storagePath string
					var dbSize sql.NullInt64
					if err := rows.Scan(&hash, &storagePath, &dbSize); err != nil {
						continue
					}
					fullPath := filepath.Join(attachDir, storagePath)
					info, err := os.Stat(fullPath)
					if os.IsNotExist(err) {
						missing++
						if missing <= 3 {
							fmt.Printf("  MISSING: %s\n", storagePath)
						}
					} else if err == nil && dbSize.Valid && info.Size() != dbSize.Int64 {
						sizeMismatch++
					}
					checked++
				}
				rows.Close()

				if missing > 0 {
					fmt.Printf("  WARNING: %d of %d attachment files missing on disk\n", missing, checked)
				} else if checked > 0 {
					fmt.Printf("  Attachment files:  %10d checked, all present\n", checked)
				}
				if sizeMismatch > 0 {
					fmt.Printf("  WARNING: %d attachments have size mismatches\n", sizeMismatch)
				}
			}
		}
	}

	fmt.Println()

	// 6. MIME sample verification (same logic as remote verify)
	if messageCount > 0 && verifySampleSize > 0 {
		// Get all source IDs to sample across all accounts
		sourceRows, err := s.DB().Query("SELECT id FROM sources")
		if err != nil {
			return fmt.Errorf("query sources: %w", err)
		}
		var sourceIDs []int64
		for sourceRows.Next() {
			var id int64
			if err := sourceRows.Scan(&id); err != nil {
				sourceRows.Close()
				return fmt.Errorf("scan source: %w", err)
			}
			sourceIDs = append(sourceIDs, id)
		}
		sourceRows.Close()

		if len(sourceIDs) > 0 {
			actualSampleSize := verifySampleSize
			if int64(actualSampleSize) > messageCount {
				actualSampleSize = int(messageCount)
			}

			// Sample from the first source (or distribute across sources)
			sampleIDs, err := s.GetRandomMessageIDs(sourceIDs[0], actualSampleSize)
			if err != nil {
				return fmt.Errorf("get sample IDs: %w", err)
			}

			fmt.Printf("Sampling %d messages for MIME verification...\n", len(sampleIDs))

			verified := 0
			var verifyErrors []string

			for _, msgID := range sampleIDs {
				rawData, err := s.GetMessageRaw(msgID)
				if err != nil {
					if err == sql.ErrNoRows {
						verifyErrors = append(verifyErrors, fmt.Sprintf("msg %d: missing raw MIME", msgID))
					} else {
						verifyErrors = append(verifyErrors, fmt.Sprintf("msg %d: db error (%v)", msgID, err))
					}
					continue
				}

				_, err = mime.Parse(rawData)
				if err != nil {
					verifyErrors = append(verifyErrors, fmt.Sprintf("msg %d: corrupt MIME (%v)", msgID, err))
					continue
				}

				verified++
			}

			if len(verifyErrors) > 0 {
				fmt.Printf("Sample verified:     %10d of %d\n", verified, len(sampleIDs))
				fmt.Printf("Sample errors:       %10d\n", len(verifyErrors))
				for i, e := range verifyErrors {
					if i >= 5 {
						fmt.Printf("  ... and %d more\n", len(verifyErrors)-5)
						break
					}
					fmt.Printf("  - %s\n", e)
				}
			} else {
				fmt.Printf("Sample verified:     %10d (all OK)\n", verified)
			}
		}
	}

	fmt.Println()
	fmt.Println("Local verification complete.")
	return nil
}

// runRemoteVerify performs verification against Gmail API.
func runRemoteVerify(cmd *cobra.Command, email string) error {
	// Validate config
	if cfg.OAuth.ClientSecrets == "" {
		return errOAuthNotConfigured()
	}

	// Open database
	dbPath := cfg.DatabaseDSN()
	s, err := store.Open(dbPath, store.WithPassphrase(passphrase))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer s.Close()

	// Create OAuth manager and get token source
	oauthMgr, err := oauth.NewManager(cfg.OAuth.ClientSecrets, cfg.TokensDir(), logger)
	if err != nil {
		return wrapOAuthError(fmt.Errorf("create oauth manager: %w", err))
	}

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nInterrupted.")
		cancel()
	}()

	tokenSource, err := oauthMgr.TokenSource(ctx, email)
	if err != nil {
		return fmt.Errorf("get token source: %w (run 'add-account' first)", err)
	}

	// Create Gmail client (no rate limiter needed for single call)
	client := gmail.NewClient(tokenSource, gmail.WithLogger(logger))
	defer client.Close()

	// Get Gmail profile
	profile, err := client.GetProfile(ctx)
	if err != nil {
		return fmt.Errorf("get Gmail profile: %w", err)
	}

	fmt.Printf("Verifying archive for %s...\n\n", profile.EmailAddress)

	// Get source from database
	source, err := s.GetSourceByIdentifier(profile.EmailAddress)
	if err != nil {
		return fmt.Errorf("get source: %w", err)
	}
	if source == nil {
		fmt.Printf("Account %s not found in database.\n", profile.EmailAddress)
		fmt.Println("Run 'sync-full' first to populate the archive.")
		return nil
	}

	// Count local messages
	archiveCount, err := s.CountMessagesForSource(source.ID)
	if err != nil {
		return fmt.Errorf("count messages: %w", err)
	}

	withRaw, err := s.CountMessagesWithRaw(source.ID)
	if err != nil {
		return fmt.Errorf("count messages with raw: %w", err)
	}

	// Print summary
	gmailTotal := profile.MessagesTotal
	fmt.Printf("Gmail messages:      %10d\n", gmailTotal)
	fmt.Printf("Archived messages:   %10d\n", archiveCount)
	diff := gmailTotal - archiveCount
	if diff > 0 {
		fmt.Printf("Missing:             %10d\n", diff)
	} else if diff < 0 {
		fmt.Printf("Extra in archive:    %10d\n", -diff)
	} else {
		fmt.Printf("Difference:          %10d\n", diff)
	}
	fmt.Println()

	rawPct := float64(0)
	if archiveCount > 0 {
		rawPct = float64(withRaw) / float64(archiveCount) * 100
	}
	fmt.Printf("With raw MIME:       %10d (%.1f%%)\n", withRaw, rawPct)
	fmt.Println()

	// Sample verification
	if archiveCount > 0 && verifySampleSize > 0 {
		actualSampleSize := verifySampleSize
		if int64(actualSampleSize) > archiveCount {
			actualSampleSize = int(archiveCount)
		}

		sampleIDs, err := s.GetRandomMessageIDs(source.ID, actualSampleSize)
		if err != nil {
			return fmt.Errorf("get sample IDs: %w", err)
		}

		fmt.Printf("Sampling %d messages...\n", len(sampleIDs))

		verified := 0
		var errors []string

		for _, msgID := range sampleIDs {
			// Check context cancellation
			if ctx.Err() != nil {
				fmt.Println("\nVerification interrupted.")
				break
			}

			// Get raw MIME
			rawData, err := s.GetMessageRaw(msgID)
			if err != nil {
				if err == sql.ErrNoRows {
					errors = append(errors, fmt.Sprintf("msg %d: missing raw MIME", msgID))
				} else {
					errors = append(errors, fmt.Sprintf("msg %d: db error (%v)", msgID, err))
				}
				continue
			}

			// Verify it can be parsed as MIME
			_, err = mime.Parse(rawData)
			if err != nil {
				errors = append(errors, fmt.Sprintf("msg %d: corrupt MIME (%v)", msgID, err))
				continue
			}

			verified++
		}

		if len(errors) > 0 {
			fmt.Printf("Sample verified:     %10d of %d\n", verified, len(sampleIDs))
			fmt.Printf("Sample errors:       %10d\n", len(errors))
			for i, err := range errors {
				if i >= 5 {
					fmt.Printf("  ... and %d more\n", len(errors)-5)
					break
				}
				fmt.Printf("  - %s\n", err)
			}
		} else {
			fmt.Printf("Sample verified:     %10d (all OK)\n", verified)
		}
	}

	fmt.Println()
	fmt.Println("Verification complete.")

	return nil
}

func init() {
	verifyCmd.Flags().IntVar(&verifySampleSize, "sample", 100, "Number of messages to sample for MIME verification")
	verifyCmd.Flags().BoolVar(&verifyLocalOnly, "local-only", false, "Verify local database only (skip Gmail API)")
	rootCmd.AddCommand(verifyCmd)
}
