package spotify

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultClientID and DefaultClientSecret are fallback credentials for out-of-the-box Spotify login.
const (
	DefaultClientID     = "4fb7c85e66b8403a8ffdf424372dbf8e"
	DefaultClientSecret = "4ace292096f241afb76e03e6aa9054c6"
)

// Credentials stores Spotify access tokens and user tier.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Product      string    `json:"product"` // "premium" or "free"
}

// ClientConfig stores developer app credentials for Spotify.
type ClientConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// AuthManager manages OAuth2 PKCE authorization flows and token refresh.
type AuthManager struct {
	dataPath     string
	clientID     string
	clientSecret string
	client       *http.Client
	tokenURL     string
	apiBase      string

	mu           sync.Mutex
	verifier     string
	state        string
	redirectURI  string
	loginServer  *http.Server
	authComplete chan struct{}
}

// NewAuthManager initializes a new Spotify authentication manager.
func NewAuthManager(dataPath string, clientID, clientSecret string) *AuthManager {
	am := &AuthManager{
		dataPath:     dataPath,
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		client:       &http.Client{Timeout: 15 * time.Second},
		tokenURL:     "https://accounts.spotify.com/api/token",
		apiBase:      "https://api.spotify.com/v1",
	}
	if am.clientID == "" {
		am.loadClientConfig()
	}
	if am.clientID == "" {
		am.clientID = DefaultClientID
		am.clientSecret = DefaultClientSecret
	}
	return am
}

func (a *AuthManager) credsFile() string {
	return filepath.Join(a.dataPath, "spotify.json")
}

func (a *AuthManager) clientConfigFile() string {
	return filepath.Join(a.dataPath, "spotify_client.json")
}

func (a *AuthManager) loadClientConfig() {
	b, err := os.ReadFile(a.clientConfigFile())
	if err != nil {
		return
	}
	var sc ClientConfig
	if err := json.Unmarshal(b, &sc); err == nil && sc.ClientID != "" {
		a.clientID = sc.ClientID
		if sc.ClientSecret != "" {
			a.clientSecret = sc.ClientSecret
		}
	}
}

func (a *AuthManager) saveClientConfig(clientID, clientSecret string) error {
	sc := ClientConfig{ClientID: clientID, ClientSecret: clientSecret}
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.clientConfigFile(), b, 0o600)
}

// Save atomically writes credentials to disk.
func (a *AuthManager) Save(c Credentials) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.saveLocked(c)
}

func (a *AuthManager) saveLocked(c Credentials) error {
	if err := os.MkdirAll(a.dataPath, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.credsFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.credsFile())
}

// Credentials returns stored credentials or an error if not found.
func (a *AuthManager) Credentials() (Credentials, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	b, err := os.ReadFile(a.credsFile())
	if err != nil {
		return Credentials{}, err
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

// ProactiveToken returns a valid access token, proactively refreshing if expiring soon.
func (a *AuthManager) ProactiveToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	b, err := os.ReadFile(a.credsFile())
	if err != nil {
		return "", err
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return "", err
	}

	// Return token directly if still valid for over 5 minutes.
	if time.Until(c.ExpiresAt) > 5*time.Minute && c.AccessToken != "" {
		return c.AccessToken, nil
	}

	if c.RefreshToken == "" {
		return "", errors.New("no refresh token available")
	}

	// Refresh token via accounts.spotify.com
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.RefreshToken},
		"client_id":     {a.clientID},
	}
	if a.clientSecret != "" {
		form.Set("client_secret", a.clientSecret)
	}

	req, err := http.NewRequest(http.MethodPost, a.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if a.clientSecret != "" {
		req.SetBasicAuth(a.clientID, a.clientSecret)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	c.AccessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		c.RefreshToken = tr.RefreshToken
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600
	}
	c.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)

	if err := a.saveLocked(c); err != nil {
		return "", err
	}
	return c.AccessToken, nil
}

// localIP discovers the device's local network IP address.
func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					return ipNet.IP.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

// StartLoginFlow initializes the local login web portal and returns its URL for QR display.
func (a *AuthManager) StartLoginFlow() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Stop previous server if active.
	if a.loginServer != nil {
		_ = a.loginServer.Close()
		a.loginServer = nil
	}

	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", err
	}
	a.verifier = base64.RawURLEncoding.EncodeToString(verifierBytes)

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	a.state = base64.RawURLEncoding.EncodeToString(stateBytes)

	listener, err := net.Listen("tcp", "0.0.0.0:8888")
	if err != nil {
		listener, err = net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			return "", err
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port
	ip := localIP()
	portalURL := fmt.Sprintf("http://%s:%d/", ip, port)

	mux := http.NewServeMux()

	// GET / : Web configuration and login portal
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		a.mu.Lock()
		curID := a.clientID
		curSec := a.clientSecret
		a.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginPortalHTML(ip, port, curID, curSec)))
	})

	// POST /api/config : Save Client ID and Client Secret
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cid := strings.TrimSpace(payload.ClientID)
		csec := strings.TrimSpace(payload.ClientSecret)
		if cid == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Client ID không được để trống"})
			return
		}
		a.mu.Lock()
		a.clientID = cid
		a.clientSecret = csec
		_ = a.saveClientConfig(cid, csec)
		a.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	// GET /auth : Redirect to Spotify OAuth authorization
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		cid := strings.TrimSpace(r.URL.Query().Get("client_id"))
		csec := strings.TrimSpace(r.URL.Query().Get("client_secret"))
		if cid == "" {
			a.mu.Lock()
			cid = a.clientID
			a.mu.Unlock()
		}
		if cid == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("<h3>Thiếu Client ID / Missing Client ID</h3><p>Vui lòng nhập Spotify Client ID của bạn. <a href='/'>Quay lại</a></p>"))
			return
		}

		a.mu.Lock()
		a.clientID = cid
		if csec != "" || r.URL.Query().Has("client_secret") {
			a.clientSecret = csec
		}
		_ = a.saveClientConfig(cid, a.clientSecret)

		hash := sha256.Sum256([]byte(a.verifier))
		challenge := base64.RawURLEncoding.EncodeToString(hash[:])

		// Spotify dashboard strictly requires https:// or http://127.0.0.1 / http://localhost redirect URIs.
		chosenRedirect := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
		a.redirectURI = chosenRedirect
		state := a.state
		a.mu.Unlock()

		scopes := "user-read-private user-read-email user-library-read user-library-modify playlist-read-private playlist-read-collaborative playlist-modify-public playlist-modify-private"
		spotifyAuthURL := fmt.Sprintf(
			"https://accounts.spotify.com/authorize?response_type=code&client_id=%s&scope=%s&redirect_uri=%s&state=%s&code_challenge_method=S256&code_challenge=%s",
			url.QueryEscape(cid),
			url.QueryEscape(scopes),
			url.QueryEscape(chosenRedirect),
			url.QueryEscape(state),
			url.QueryEscape(challenge),
		)
		http.Redirect(w, r, spotifyAuthURL, http.StatusFound)
	})

	// GET /callback : Handle authorization callback from Spotify
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		st := r.URL.Query().Get("state")
		a.mu.Lock()
		expectedState := a.state
		redURI := a.redirectURI
		a.mu.Unlock()

		if st != expectedState || code == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("<h3>Lỗi xác thực / Invalid OAuth state or code</h3><p><a href='/'>Quay lại</a></p>"))
			return
		}

		if err := a.exchangeCode(code, redURI); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "<h3>Lỗi cấp token / Token exchange failed:</h3><p>%s</p><p><a href='/'>Thử lại</a></p>", err.Error())
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginSuccessHTML))
	})

	// POST /api/exchange : Manual code exchange endpoint when phone redirects to 127.0.0.1
	mux.HandleFunc("/api/exchange", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Code         string `json:"code"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		raw := strings.TrimSpace(payload.Code)
		if raw == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Vui lòng nhập link hoặc mã xác nhận"})
			return
		}

		if cid := strings.TrimSpace(payload.ClientID); cid != "" {
			a.mu.Lock()
			a.clientID = cid
			if payload.ClientSecret != "" {
				a.clientSecret = strings.TrimSpace(payload.ClientSecret)
			}
			_ = a.saveClientConfig(a.clientID, a.clientSecret)
			a.mu.Unlock()
		}

		if strings.Contains(raw, "accounts.spotify.com/authorize") || strings.Contains(raw, "accounts.spotify.com/login") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": "Đây là link BẮT ĐẦU ủy quyền (Authorize), chưa có mã code! Hãy bấm nút '1. Mở trang Spotify' bên trên để mở link này, đăng nhập và bấm 'Đồng ý' (Agree). Sau đó copy link ở trang lỗi tiếp theo (có dạng http://127.0.0.1:8888/callback?code=...) rồi dán vào đây.",
			})
			return
		}

		code := raw
		redURI := ""
		if u, err := url.Parse(raw); err == nil {
			if qCode := u.Query().Get("code"); qCode != "" {
				code = qCode
			}
			if u.Host != "" && u.Scheme != "" {
				redURI = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, u.Path)
			}
		}

		a.mu.Lock()
		if redURI == "" {
			redURI = a.redirectURI
		}
		if redURI == "" {
			redURI = fmt.Sprintf("http://127.0.0.1:%d/callback", port)
		}
		a.mu.Unlock()

		if err := a.exchangeCode(code, redURI); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	a.loginServer = &http.Server{Handler: mux}
	go func() {
		_ = a.loginServer.Serve(listener)
	}()

	return portalURL, nil
}

func (a *AuthManager) exchangeCode(code, redirectURI string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	urisToTry := []string{redirectURI}
	common := []string{
		"http://127.0.0.1:8888/callback",
		"http://127.0.0.1/callback",
		"http://localhost:8888/callback",
		"http://localhost/callback",
	}
	for _, c := range common {
		if c != redirectURI {
			urisToTry = append(urisToTry, c)
		}
	}

	var lastErr error
	for _, red := range urisToTry {
		err := a.doExchangeLocked(code, red)
		if err == nil {
			return nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "redirect_uri") && !strings.Contains(err.Error(), "Invalid redirect URI") {
			return err
		}
	}
	return lastErr
}

func (a *AuthManager) doExchangeLocked(code, redirectURI string) error {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {a.clientID},
		"code_verifier": {a.verifier},
	}
	if a.clientSecret != "" {
		form.Set("client_secret", a.clientSecret)
	}

	req, err := http.NewRequest(http.MethodPost, a.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if a.clientSecret != "" {
		req.SetBasicAuth(a.clientID, a.clientSecret)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return err
	}

	product := "free"
	meReq, err := http.NewRequest(http.MethodGet, a.apiBase+"/me", nil)
	if err == nil {
		meReq.Header.Set("Authorization", "Bearer "+tr.AccessToken)
		if meResp, err := a.client.Do(meReq); err == nil && meResp.StatusCode == http.StatusOK {
			var me struct {
				Product string `json:"product"`
			}
			_ = json.NewDecoder(meResp.Body).Decode(&me)
			meResp.Body.Close()
			if me.Product != "" {
				product = me.Product
			}
		}
	}

	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600
	}
	c := Credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		Product:      product,
	}

	if err := a.saveLocked(c); err != nil {
		return err
	}

	go func() {
		time.Sleep(2 * time.Second)
		a.mu.Lock()
		if a.loginServer != nil {
			_ = a.loginServer.Close()
			a.loginServer = nil
		}
		a.mu.Unlock()
	}()

	return nil
}

func loginPortalHTML(ip string, port int, curClientID, curClientSecret string) string {
	escapedID := html.EscapeString(curClientID)
	escapedSecret := html.EscapeString(curClientSecret)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="vi">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Audiofilus · Đăng nhập Spotify</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Press+Start+2P&family=VT323&display=swap" rel="stylesheet">
  <style>
    * {
      box-sizing: border-box;
      image-rendering: pixelated;
    }
    body {
      background-color: #0d0b17;
      background-image:
        radial-gradient(#1a162b 15%%, transparent 16%%),
        linear-gradient(rgba(18, 16, 28, 0.4) 50%%, rgba(0, 0, 0, 0.6) 50%%);
      background-size: 16px 16px, 100%% 4px;
      color: #f1ecff;
      font-family: 'VT323', 'Courier New', monospace;
      font-size: 19px;
      margin: 0;
      padding: 16px;
      line-height: 1.4;
    }
    .container {
      max-width: 540px;
      margin: 0 auto;
      padding-bottom: 30px;
    }
    .header-box {
      text-align: center;
      padding: 16px 12px;
      background: #141122;
      border: 3px solid #000;
      box-shadow: inset 2px 2px 0 #2c2547, inset -2px -2px 0 #08070e, 4px 4px 0 #000;
      margin-bottom: 20px;
    }
    .logo-text {
      font-family: 'Press Start 2P', monospace;
      font-size: 13px;
      color: #1ed760;
      text-shadow: 2px 2px 0 #000;
      letter-spacing: 1px;
      margin: 0 0 8px 0;
    }
    .sub-text {
      font-size: 17px;
      color: #8f87ad;
      margin: 0;
      letter-spacing: 0.5px;
    }
    .pixel-card {
      background: #181528;
      border: 3px solid #000;
      box-shadow: inset 2px 2px 0 #2f274c, inset -2px -2px 0 #0a0812, 4px 4px 0 #000;
      padding: 18px 16px;
      margin-bottom: 18px;
    }
    .card-title {
      font-family: 'Press Start 2P', monospace;
      font-size: 11px;
      color: #4fe3c1;
      text-shadow: 2px 2px 0 #000;
      margin: 0 0 12px 0;
      display: flex;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
    }
    .step-tag {
      background: #ff5fa2;
      color: #000;
      font-family: 'Press Start 2P', monospace;
      font-size: 9px;
      padding: 3px 6px;
      border: 2px solid #000;
      box-shadow: 2px 2px 0 #000;
    }
    .step-tag.green {
      background: #1ed760;
      color: #000;
    }
    .step-tag.cyan {
      background: #4fe3c1;
      color: #000;
    }
    .step-tag.yellow {
      background: #f2c418;
      color: #000;
    }
    p {
      margin: 6px 0 10px 0;
      color: #c9c3e0;
      font-size: 18px;
    }
    label {
      display: block;
      font-family: 'Press Start 2P', monospace;
      font-size: 9px;
      color: #8f87ad;
      margin-top: 12px;
      margin-bottom: 4px;
    }
    .pixel-input {
      width: 100%%;
      background: #090710;
      border: 2px solid #554c79;
      color: #1ed760;
      font-family: 'VT323', 'Courier New', monospace;
      font-size: 20px;
      padding: 8px 10px;
      margin-bottom: 6px;
      box-shadow: inset 3px 3px 0 #000;
      outline: none;
    }
    .pixel-input:focus {
      border-color: #1ed760;
      box-shadow: inset 3px 3px 0 #000, 0 0 0 1px #1ed760;
      color: #fff;
    }
    .pixel-btn {
      font-family: 'Press Start 2P', monospace;
      font-size: 11px;
      padding: 13px 12px;
      border: 3px solid #000;
      cursor: pointer;
      text-decoration: none;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      text-align: center;
      background: #1ed760;
      color: #000;
      box-shadow: 0 4px 0 #0f7a36, 4px 4px 0 #000;
      transition: transform 0.05s ease, box-shadow 0.05s ease;
      user-select: none;
      width: 100%%;
      margin-top: 10px;
      line-height: 1.2;
    }
    .pixel-btn:hover {
      background: #25e869;
    }
    .pixel-btn:active {
      transform: translate(2px, 3px);
      box-shadow: 0 1px 0 #0f7a36, 2px 2px 0 #000;
    }
    .pixel-btn:disabled {
      opacity: 0.5;
      cursor: not-allowed;
      transform: none;
      box-shadow: 0 4px 0 #000;
    }
    .btn-row {
      display: flex;
      gap: 8px;
      margin-top: 8px;
      flex-wrap: wrap;
    }
    .pixel-btn-small {
      font-family: 'Press Start 2P', monospace;
      font-size: 8px;
      padding: 7px 9px;
      border: 2px solid #000;
      cursor: pointer;
      background: #312a4f;
      color: #f1ecff;
      box-shadow: 0 3px 0 #1b172e, 2px 2px 0 #000;
      transition: transform 0.05s ease;
      line-height: 1.2;
    }
    .pixel-btn-small:hover {
      background: #42396b;
    }
    .pixel-btn-small:active {
      transform: translate(1px, 2px);
      box-shadow: 0 1px 0 #1b172e, 1px 1px 0 #000;
    }
    .code-box {
      background: #090710;
      border: 2px solid #332d4b;
      padding: 8px 10px;
      font-size: 16px;
      color: #4fe3c1;
      word-break: break-all;
      margin: 6px 0;
      box-shadow: inset 2px 2px 0 #000;
    }
    .msg-box {
      display: none;
      padding: 10px 12px;
      border: 3px solid #000;
      font-family: 'Press Start 2P', monospace;
      font-size: 9px;
      line-height: 1.5;
      margin-top: 12px;
      box-shadow: 3px 3px 0 #000;
    }
    .msg-box.ok {
      display: block;
      background: #11381e;
      color: #2ecc71;
      border-color: #2ecc71;
    }
    .msg-box.err {
      display: block;
      background: #3d1416;
      color: #ff6b6b;
      border-color: #ff6b6b;
    }
    .msg-box.info {
      display: block;
      background: #162a3d;
      color: #4fc3f7;
      border-color: #4fc3f7;
    }
    .divider {
      text-align: center;
      color: #554c79;
      font-size: 14px;
      margin: 10px 0;
      letter-spacing: 3px;
    }
    a {
      color: #1ed760;
      text-decoration: underline;
    }
    a:hover {
      color: #4fe3c1;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="header-box">
      <div class="logo-text">♫ AUDIOFILUS · SPOTIFY ♫</div>
      <div class="sub-text">[ CỔNG LIÊN KẾT TÀI KHOẢN RETRO ]</div>
    </div>

    <!-- BƯỚC 1: CẤU HÌNH API KEYS -->
    <div class="pixel-card">
      <div class="card-title">
        <span class="step-tag">BƯỚC 1</span>
        <span>CẤU HÌNH SPOTIFY API KEY</span>
      </div>
      <p>Nhập Client ID từ tài khoản <b>Spotify Developer</b> của bạn để kết nối riêng biệt và ổn định nhất.</p>

      <form id="authForm" method="GET" action="/auth" target="_blank" onsubmit="return handleAuthSubmit(event)">
        <label for="clientId">SPOTIFY CLIENT ID (* BẮT BUỘC):</label>
        <input type="text" id="clientId" name="client_id" class="pixel-input" placeholder="Dán Client ID (32 ký tự) vào đây..." value="%s" autocomplete="off" spellcheck="false" required>

        <label for="clientSecret">SPOTIFY CLIENT SECRET (TÙY CHỌN):</label>
        <input type="text" id="clientSecret" name="client_secret" class="pixel-input" placeholder="Dán Client Secret vào đây..." value="%s" autocomplete="off" spellcheck="false">

        <div class="btn-row">
          <button type="button" class="pixel-btn-small" onclick="saveKeyConfig()">💾 LƯU CẤU HÌNH</button>
          <button type="button" class="pixel-btn-small" onclick="fillDefaultKeys()">⚡ DÙNG KEY MẪU</button>
          <button type="button" class="pixel-btn-small" onclick="clearKeys()">✕ XÓA TRỐNG</button>
        </div>
        <div id="keySaveStatus" class="msg-box"></div>

        <div class="divider">░░▒▒▓▓██▓▓▒▒░░</div>

        <!-- BƯỚC 2: MỞ TRANG ĐĂNG NHẬP -->
        <div class="card-title" style="margin-top: 14px;">
          <span class="step-tag cyan">BƯỚC 2</span>
          <span>MỞ TRANG CẤP QUYỀN SPOTIFY</span>
        </div>
        <p>Bấm nút bên dưới để mở Spotify trên tab mới, đăng nhập và nhấn <b>"Đồng ý" (Agree)</b>.</p>
        <button type="submit" id="authBtn" class="pixel-btn">▶ 1. MỞ TRANG SPOTIFY (TAB MỚI) ◀</button>
      </form>
    </div>

    <!-- BƯỚC 3: XÁC NHẬN MÃ CODE -->
    <div class="pixel-card">
      <div class="card-title">
        <span class="step-tag green">BƯỚC 3</span>
        <span>XÁC NHẬN MÃ CODE HOÀN TẤT</span>
      </div>
      <p>Sau khi bấm "Đồng ý" trên tab Spotify, tab đó sẽ chuyển hướng đến trang báo lỗi kết nối (vì thiết bị không mở cổng %d).<br>
      👉 Hãy <b>sao chép toàn bộ đường link</b> trên thanh địa chỉ của tab đó rồi dán vào đây:</p>
      
      <input type="text" id="manualCode" class="pixel-input" placeholder="Dán link http://127.0.0.1:%d/callback?code=... hoặc mã code..." autocomplete="off" spellcheck="false">
      
      <button type="button" id="submitBtn" class="pixel-btn" style="background:#4fe3c1; box-shadow: 0 4px 0 #1b9075, 4px 4px 0 #000;" onclick="submitManualCode()">★ 2. XÁC NHẬN MÃ & KẾT NỐI ★</button>
      
      <div id="statusBox" class="msg-box"></div>
    </div>

    <!-- HƯỚNG DẪN SPOTIFY DEVELOPER -->
    <div class="pixel-card" style="border-color:#3d3460;">
      <div class="card-title" style="color:#f2c418;">
        <span class="step-tag yellow">HƯỚNG DẪN</span>
        <span>CÁCH TẠO SPOTIFY API KEY</span>
      </div>
      <p style="font-size:16px;">1. Mở <a href="https://developer.spotify.com/dashboard" target="_blank">developer.spotify.com/dashboard</a> và đăng nhập tài khoản Spotify.<br>
      2. Bấm <b>"Create App"</b>, đặt tên tùy thích (vd: <i>Audiofilus</i>).<br>
      3. Ở mục <b>Redirect URIs</b>, bắt buộc thêm đủ 2 đường link sau:<br>
      <div class="code-box">http://127.0.0.1:%d/callback</div>
      <div class="code-box">http://127.0.0.1/callback</div>
      4. Chọn <b>Web API</b> & <b>Web Playback SDK</b> rồi bấm <b>Save</b>.<br>
      5. Vào mục <b>Settings</b> của App vừa tạo, copy <b>Client ID</b> và <b>Client Secret</b> dán vào Bước 1 ở trên.</p>
    </div>
  </div>

  <script>
    const DEFAULT_CID = "%s";
    const DEFAULT_SEC = "%s";

    function fillDefaultKeys() {
      document.getElementById('clientId').value = DEFAULT_CID;
      document.getElementById('clientSecret').value = DEFAULT_SEC;
      showToast('keySaveStatus', 'ok', '✓ ĐÃ ĐIỀN KEY MẪU MẶC ĐỊNH!');
    }

    function clearKeys() {
      document.getElementById('clientId').value = '';
      document.getElementById('clientSecret').value = '';
      showToast('keySaveStatus', 'info', 'ℹ ĐÃ XÓA TRỐNG! HÃY DÁN KEY CỦA BẠN.');
    }

    function showToast(elemId, type, text) {
      const box = document.getElementById(elemId);
      box.className = 'msg-box ' + type;
      box.innerHTML = text;
      box.style.display = 'block';
    }

    async function saveKeyConfig() {
      const cid = document.getElementById('clientId').value.trim();
      const csec = document.getElementById('clientSecret').value.trim();
      if (!cid) {
        showToast('keySaveStatus', 'err', '❌ VUI LÒNG NHẬP CLIENT ID!');
        return false;
      }
      try {
        const res = await fetch('/api/config', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({client_id: cid, client_secret: csec})
        });
        const data = await res.json();
        if (data.ok) {
          showToast('keySaveStatus', 'ok', '✓ ĐÃ LƯU CẤU HÌNH KEY THÀNH CÔNG!');
          return true;
        } else {
          showToast('keySaveStatus', 'err', '❌ LỖI: ' + (data.error || 'KHÔNG THỂ LƯU'));
          return false;
        }
      } catch (e) {
        showToast('keySaveStatus', 'ok', '✓ ĐÃ LƯU KEY VÀO BỘ NHỚ TẠM');
        return true;
      }
    }

    async function handleAuthSubmit(e) {
      const cid = document.getElementById('clientId').value.trim();
      if (!cid) {
        e.preventDefault();
        showToast('keySaveStatus', 'err', '❌ BẠN PHẢI NHẬP CLIENT ID ĐỂ MỞ SPOTIFY!');
        return false;
      }
      saveKeyConfig();
      showToast('statusBox', 'info', '⏳ ĐÃ MỞ TAB SPOTIFY! HÃY ĐĂNG NHẬP, BẤM "AGREE", SAU ĐÓ COPY LINK LỖI VÀ DÁN VÀO Ô BƯỚC 3.');
      return true;
    }

    async function submitManualCode() {
      const val = document.getElementById('manualCode').value.trim();
      const cid = document.getElementById('clientId').value.trim();
      const csec = document.getElementById('clientSecret').value.trim();
      const box = document.getElementById('statusBox');
      const btn = document.getElementById('submitBtn');

      if (!val) {
        showToast('statusBox', 'err', '❌ VUI LÒNG DÁN LINK HOẶC MÃ CODE VÀO Ô TRÊN!');
        return;
      }
      showToast('statusBox', 'info', '⏳ ĐANG XÁC THỰC VỚI SPOTIFY, VUI LÒNG CHỜ...');
      btn.disabled = true;

      try {
        const res = await fetch('/api/exchange', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            code: val,
            client_id: cid,
            client_secret: csec
          })
        });
        const data = await res.json();
        if (data.ok) {
          showToast('statusBox', 'ok', '✓ ĐĂNG NHẬP THÀNH CÔNG!<br><br>BẠN CÓ THỂ ĐÓNG TRANG NÀY VÀ QUAY LẠI MÁY AUDIOFILUS ĐỂ THƯỞNG THỨC ÂM NHẠC.');
        } else {
          showToast('statusBox', 'err', '❌ ' + (data.error || 'MÃ XÁC THỰC KHÔNG HỢP LỆ'));
        }
      } catch (err) {
        showToast('statusBox', 'err', '❌ LỖI KẾT NỐI: ' + err.message);
      } finally {
        btn.disabled = false;
      }
    }
  </script>
</body>
</html>`,
		escapedID,
		escapedSecret,
		port,
		port,
		port,
		DefaultClientID,
		DefaultClientSecret,
	)
}

const loginSuccessHTML = `<!DOCTYPE html>
<html lang="vi">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Audiofilus · Đăng nhập thành công</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Press+Start+2P&family=VT323&display=swap" rel="stylesheet">
  <style>
    * {
      box-sizing: border-box;
      image-rendering: pixelated;
    }
    body {
      background-color: #0d0b17;
      background-image:
        radial-gradient(#1a162b 15%, transparent 16%),
        linear-gradient(rgba(18, 16, 28, 0.4) 50%, rgba(0, 0, 0, 0.6) 50%);
      background-size: 16px 16px, 100% 4px;
      color: #f1ecff;
      font-family: 'VT323', 'Courier New', monospace;
      padding: 40px 16px;
      margin: 0;
      text-align: center;
    }
    .box {
      max-width: 480px;
      margin: 30px auto;
      background: #181528;
      padding: 30px 20px;
      border: 4px solid #000;
      box-shadow: inset 3px 3px 0 #31274f, inset -3px -3px 0 #0a0812, 6px 6px 0 #000;
    }
    .badge {
      background: #1ed760;
      color: #000;
      font-family: 'Press Start 2P', monospace;
      font-size: 10px;
      padding: 6px 10px;
      border: 2px solid #000;
      box-shadow: 3px 3px 0 #000;
      display: inline-block;
      margin-bottom: 20px;
    }
    .icon {
      font-size: 48px;
      color: #1ed760;
      margin-bottom: 10px;
      text-shadow: 3px 3px 0 #000;
    }
    h2 {
      font-family: 'Press Start 2P', monospace;
      font-size: 13px;
      color: #4fe3c1;
      text-shadow: 2px 2px 0 #000;
      margin: 10px 0 16px 0;
      line-height: 1.5;
    }
    p {
      color: #c9c3e0;
      font-size: 20px;
      line-height: 1.4;
      margin: 12px 0;
    }
    .divider {
      color: #554c79;
      font-size: 14px;
      margin: 16px 0;
      letter-spacing: 4px;
    }
    .hint {
      font-family: 'Press Start 2P', monospace;
      font-size: 9px;
      color: #f2c418;
      background: #25203d;
      padding: 8px;
      border: 2px solid #000;
      margin-top: 20px;
      box-shadow: 2px 2px 0 #000;
    }
  </style>
</head>
<body>
  <div class="box">
    <div class="badge">★ SPOTIFY CONNECTED ★</div>
    <div class="icon">✓</div>
    <h2>ĐĂNG NHẬP THÀNH CÔNG!</h2>
    <div class="divider">░░▒▒▓▓████▓▓▒▒░░</div>
    <p>Tài khoản Spotify đã được liên kết với máy <b>Audiofilus</b> của bạn.</p>
    <p>Bây giờ bạn có thể đóng trang web này và quay lại máy chơi game để thưởng thức âm nhạc.</p>
    <div class="hint">[ ♫ SẴN SÀNG PHÁT NHẠC TRÊN AUDIOFILUS ♫ ]</div>
  </div>
</body>
</html>`
