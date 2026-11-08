package spotify

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

// Credentials almacena las credenciales de acceso a Spotify.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Product      string    `json:"product"` // "premium" o "free"
}

// AuthManager gestiona el flujo de autorización OAuth2 con PKCE y el refresco de tokens.
type AuthManager struct {
	dataPath string
	clientID string
	client   *http.Client
	tokenURL string
	apiBase  string

	mu           sync.Mutex
	verifier     string
	state        string
	loginServer  *http.Server
	authComplete chan struct{}
}

// NewAuthManager inicializa un nuevo gestor de autenticación de Spotify.
func NewAuthManager(dataPath string, clientID string) *AuthManager {
	if clientID == "" {
		// Client ID público predeterminado para clientes de escritorio/móviles abiertos de Spotify
		clientID = "d87a41951f2845c48737b8bf67b8a74e"
	}
	return &AuthManager{
		dataPath: dataPath,
		clientID: clientID,
		client:   &http.Client{Timeout: 15 * time.Second},
		tokenURL: "https://accounts.spotify.com/api/token",
		apiBase:  "https://api.spotify.com/v1",
	}
}

func (a *AuthManager) credsFile() string {
	return filepath.Join(a.dataPath, "spotify.json")
}

// Save guarda las credenciales en el archivo de datos de forma atómica.
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

// Credentials devuelve las credenciales almacenadas o un error si no existen.
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

// ProactiveToken comprueba el token y lo refresca proactivamente si caduca en menos de 5 minutos.
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

	// Si el token aún es válido por más de 5 minutos, devolverlo directamente.
	if time.Until(c.ExpiresAt) > 5*time.Minute && c.AccessToken != "" {
		return c.AccessToken, nil
	}

	if c.RefreshToken == "" {
		return "", errors.New("no refresh token available")
	}

	// Refrescar el token con accounts.spotify.com
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.RefreshToken},
		"client_id":     {a.clientID},
	}

	req, err := http.NewRequest(http.MethodPost, a.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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

// StartLoginFlow inicia el servidor local de callback y devuelve la URL para el código QR.
func (a *AuthManager) StartLoginFlow() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Detener servidor previo si estaba activo.
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

	hash := sha256.Sum256([]byte(a.verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	// Enlazar a un puerto local efímero.
	listener, err := net.Listen("tcp", "0.0.0.0:8888")
	if err != nil {
		// Fallback a puerto aleatorio si el 8888 estuviese ocupado.
		listener, err = net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			return "", err
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	scopes := "user-read-private user-read-email user-library-read user-library-modify playlist-read-private playlist-read-collaborative"
	authURL := fmt.Sprintf(
		"https://accounts.spotify.com/authorize?response_type=code&client_id=%s&scope=%s&redirect_uri=%s&state=%s&code_challenge_method=S256&code_challenge=%s",
		url.QueryEscape(a.clientID),
		url.QueryEscape(scopes),
		url.QueryEscape(redirectURI),
		url.QueryEscape(a.state),
		url.QueryEscape(challenge),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		if state != a.state || code == "" {
			http.Error(w, "Invalid OAuth state or authorization code", http.StatusBadRequest)
			return
		}

		go func() {
			if err := a.exchangeCode(code, redirectURI); err == nil {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("<!DOCTYPE html><html><body style='font-family:sans-serif;text-align:center;padding:50px;'><h2>Spotify Login Successful!</h2><p>You can return to Audiofilus now.</p></body></html>"))
			} else {
				http.Error(w, "Token exchange failed: "+err.Error(), http.StatusInternalServerError)
			}
			time.Sleep(1 * time.Second)
			a.mu.Lock()
			if a.loginServer != nil {
				_ = a.loginServer.Close()
				a.loginServer = nil
			}
			a.mu.Unlock()
		}()
	})

	a.loginServer = &http.Server{Handler: mux}
	go func() {
		_ = a.loginServer.Serve(listener)
	}()

	return authURL, nil
}

func (a *AuthManager) exchangeCode(code, redirectURI string) error {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {a.clientID},
		"code_verifier": {a.verifier},
	}

	req, err := http.NewRequest(http.MethodPost, a.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
	// Consultar perfil para detectar si la cuenta es Premium
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

	creds := Credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		Product:      product,
	}
	return a.Save(creds)
}
