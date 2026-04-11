package microsoft

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wesm/msgvault/internal/fileutil"
	"golang.org/x/oauth2"
)

const (
	DefaultTenant   = "common"
	ScopeIMAP       = "https://outlook.office365.com/IMAP.AccessAsUser.All"
	callbackPath    = "/callback/microsoft"
	graphMeEndpoint = "https://graph.microsoft.com/v1.0/me"
)

var Scopes = []string{
	ScopeIMAP,
	"offline_access",
	"openid",
	"email",
	"User.Read", // required for MS Graph /me to validate email
}

type TokenMismatchError struct {
	Expected string
	Actual   string
}

func (e *TokenMismatchError) Error() string {
	return fmt.Sprintf("token mismatch: expected %s but authorized as %s", e.Expected, e.Actual)
}

type Manager struct {
	clientID         string
	tenantID         string
	tokensDir        string
	logger           *slog.Logger
	graphURL         string // override for testing
	tokenURLOverride string // override for testing

	browserFlowFn func(ctx context.Context, email string) (*oauth2.Token, error)
}

func NewManager(clientID, tenantID, tokensDir string, logger *slog.Logger) *Manager {
	if tenantID == "" {
		tenantID = DefaultTenant
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		clientID:  clientID,
		tenantID:  tenantID,
		tokensDir: tokensDir,
		logger:    logger,
	}
}

func (m *Manager) oauthConfig(redirectURL string) *oauth2.Config {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", m.tenantID)
	if m.tokenURLOverride != "" {
		tokenURL = m.tokenURLOverride
	}
	return &oauth2.Config{
		ClientID: m.clientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", m.tenantID),
			TokenURL: tokenURL,
		},
		RedirectURL: redirectURL,
		Scopes:      Scopes,
	}
}

// oauthConfigNoRedirect returns a config without redirect URL (for token refresh, device flow).
func (m *Manager) oauthConfigNoRedirect() *oauth2.Config {
	return m.oauthConfig("")
}

func (m *Manager) Authorize(ctx context.Context, email string, headless bool) error {
	var token *oauth2.Token
	var err error

	if headless {
		token, err = m.deviceCodeFlow(ctx, email)
	} else if m.browserFlowFn != nil {
		token, err = m.browserFlowFn(ctx, email)
	} else {
		token, err = m.browserFlow(ctx, email)
	}
	if err != nil {
		return err
	}
	if _, err := m.resolveTokenEmail(ctx, email, token); err != nil {
		return err
	}
	return m.saveToken(email, token, Scopes)
}

// TokenSource returns a function that provides fresh access tokens.
// Suitable for passing to imap.WithTokenSource.
func (m *Manager) TokenSource(ctx context.Context, email string) (func(context.Context) (string, error), error) {
	tf, err := m.loadTokenFile(email)
	if err != nil {
		return nil, fmt.Errorf("no valid token for %s: %w", email, err)
	}

	cfg := m.oauthConfigNoRedirect()
	ts := cfg.TokenSource(ctx, &tf.Token)

	return func(callCtx context.Context) (string, error) {
		tok, err := ts.Token()
		if err != nil {
			return "", fmt.Errorf("refresh Microsoft token (re-run 'add-o365 %s' if expired): %w", email, err)
		}
		if tok.AccessToken != tf.AccessToken {
			if saveErr := m.saveToken(email, tok, tf.Scopes); saveErr != nil {
				m.logger.Warn("failed to save refreshed token", "email", email, "error", saveErr)
			}
			tf.Token = *tok
		}
		return tok.AccessToken, nil
	}, nil
}

func (m *Manager) browserFlow(ctx context.Context, email string) (*oauth2.Token, error) {
	// Bind to a dynamic port (OS-assigned)
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("start OAuth callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	redirectURL := fmt.Sprintf("http://localhost:%d%s", port, callbackPath)
	cfg := m.oauthConfig(redirectURL)

	// PKCE (required by Azure AD for public clients)
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	challengeHash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeHash[:])

	// CSRF state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("generate state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("state mismatch: possible CSRF attack")
			fmt.Fprintf(w, "Error: state mismatch")
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			errChan <- fmt.Errorf("microsoft OAuth error: %s: %s", errMsg, desc)
			fmt.Fprintf(w, "Error: %s", desc)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			fmt.Fprintf(w, "Error: no authorization code received")
			return
		}
		codeChan <- code
		fmt.Fprintf(w, "Authorization successful! You can close this window.")
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			errChan <- err
		}
	}()
	defer func() { _ = server.Shutdown(ctx) }()

	authURL := cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("login_hint", email),
	)

	fmt.Printf("Opening browser for Microsoft authorization...\n")
	fmt.Printf("If browser doesn't open, visit:\n%s\n\n", authURL)
	if err := openBrowser(authURL); err != nil {
		m.logger.Warn("failed to open browser", "error", err)
	}

	select {
	case code := <-codeChan:
		return cfg.Exchange(ctx, code,
			oauth2.SetAuthURLParam("code_verifier", verifier),
		)
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// deviceCodeFlow runs the Azure AD device authorization grant for headless/SSH environments.
func (m *Manager) deviceCodeFlow(ctx context.Context, email string) (*oauth2.Token, error) {
	deviceURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/devicecode", m.tenantID)
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", m.tenantID)

	// Request device code
	resp, err := http.PostForm(deviceURL, url.Values{
		"client_id": {m.clientID},
		"scope":     {strings.Join(Scopes, " ")},
	})
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var deviceResp struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Message         string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}

	fmt.Printf("\n%s\n\n", deviceResp.Message)

	// Poll for token
	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		tokenResp, err := http.PostForm(tokenURL, url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {m.clientID},
			"device_code": {deviceResp.DeviceCode},
		})
		if err != nil {
			continue
		}

		body, _ := io.ReadAll(tokenResp.Body)
		_ = tokenResp.Body.Close()

		if tokenResp.StatusCode == http.StatusOK {
			var token oauth2.Token
			if err := json.Unmarshal(body, &token); err != nil {
				return nil, fmt.Errorf("parse token response: %w", err)
			}
			return &token, nil
		}

		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &errResp)

		switch errResp.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		default:
			return nil, fmt.Errorf("device code auth failed: %s", string(body))
		}
	}

	return nil, fmt.Errorf("device code authorization timed out")
}

const resolveTimeout = 10 * time.Second

func (m *Manager) resolveTokenEmail(ctx context.Context, email string, token *oauth2.Token) (string, error) {
	valCtx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	cfg := m.oauthConfigNoRedirect()
	ts := cfg.TokenSource(valCtx, token)
	client := oauth2.NewClient(valCtx, ts)

	graphURL := m.graphURL
	if graphURL == "" {
		graphURL = graphMeEndpoint
	}
	req, err := http.NewRequestWithContext(valCtx, "GET", graphURL, nil)
	if err != nil {
		return "", fmt.Errorf("create graph request: %w", err)
	}

	httpResp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("verify Microsoft account: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return "", fmt.Errorf("MS Graph returned HTTP %d: %s", httpResp.StatusCode, string(body))
	}

	var profile struct {
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&profile); err != nil {
		return "", fmt.Errorf("parse MS Graph profile: %w", err)
	}

	actual := profile.Mail
	if actual == "" {
		actual = profile.UserPrincipalName
	}
	if !strings.EqualFold(actual, email) {
		return "", &TokenMismatchError{Expected: email, Actual: actual}
	}

	return actual, nil
}

// --- Token storage ---

type tokenFile struct {
	oauth2.Token
	Scopes []string `json:"scopes,omitempty"`
}

func (m *Manager) TokenPath(email string) string {
	safe := sanitizeEmail(email)
	return filepath.Join(m.tokensDir, "microsoft_"+safe+".json")
}

func (m *Manager) saveToken(email string, token *oauth2.Token, scopes []string) error {
	if err := fileutil.SecureMkdirAll(m.tokensDir, 0700); err != nil {
		return err
	}

	tf := tokenFile{Token: *token, Scopes: scopes}
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}

	path := m.TokenPath(email)
	tmpFile, err := os.CreateTemp(m.tokensDir, ".ms-token-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp token file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp token file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp token file: %w", err)
	}
	if err := fileutil.SecureChmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp token file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp token file: %w", err)
	}
	return nil
}

func (m *Manager) loadTokenFile(email string) (*tokenFile, error) {
	path := m.TokenPath(email)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tf tokenFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, err
	}
	return &tf, nil
}

func (m *Manager) HasToken(email string) bool {
	_, err := os.Stat(m.TokenPath(email))
	return err == nil
}

func (m *Manager) DeleteToken(email string) error {
	err := os.Remove(m.TokenPath(email))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func sanitizeEmail(email string) string {
	safe := strings.ReplaceAll(email, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	safe = strings.ReplaceAll(safe, "..", "_..")
	return safe
}

func openBrowser(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("refused to open URL with scheme %q", parsed.Scheme)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}
