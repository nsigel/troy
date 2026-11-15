// Package api is Troy's HTTP API for the web app at URL. A student signs in by texting
// SignInPhrase to Troy and opening the link Troy texts back. Links and web sessions are
// Short links are stored per user; web sessions are HMAC-signed.
package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"troy/agent"
)

// SignInPhrase is the text that gets a sign-in link back, ignoring case, surrounding
// whitespace and trailing punctuation.
const SignInPhrase = "Sign in to Troy"

const (
	signInLinkLifetime = 10 * time.Minute
	webSessionLifetime = 30 * 24 * time.Hour
	webSessionCookie   = "troy_web_session"
)

type API struct {
	Key             []byte // from LoadKey
	URL             string // public base URL, e.g. https://troy.usc.sh
	Users           string // $TROY_HOME/users, one directory per phone number
	TroyPhoneNumber string // where students text SignInPhrase
	// Notify tells a user's agent about something they did on the web.
	Notify func(phoneNumber, notice string)
	// USCChanged runs after a USC account is connected or disconnected (turning on or off the
	// built-in grade alerts) and returns a line for the agent's notice, if any.
	USCChanged func(phoneNumber string, connected bool) string
	// Say calls deliver with the agent of a user's web chat, starting it if needed, while
	// nothing can close its inbox. The agent reports its turns to Stream(phoneNumber, chatID).
	Say func(phoneNumber, chatID, text string, deliver func(*agent.Agent) bool) error

	mu      sync.Mutex // serializes replacing a user's usc/ directory
	chats   hub
	closing chan struct{} // closed by CloseStreams
}

func (a *API) Register(mux *http.ServeMux) {
	a.closing = make(chan struct{})
	mux.HandleFunc("GET /api/sign-in", a.signInInstructions)
	mux.HandleFunc("GET /sign-in/{token}", a.signIn) // existing links
	mux.HandleFunc("GET /s/{token}", a.signIn)
	mux.HandleFunc("DELETE /api/web-session", a.signOut)
	mux.HandleFunc("GET /api/me", a.signedIn(a.me))
	mux.HandleFunc("GET /api/connections", a.signedIn(a.listConnections))
	mux.HandleFunc("PUT /api/connections/usc-account", a.signedIn(a.connectUSCAccount))
	mux.HandleFunc("DELETE /api/connections/usc-account", a.signedIn(a.disconnectUSCAccount))
	mux.HandleFunc("GET /api/chats", a.signedIn(a.listChats))
	mux.HandleFunc("POST /api/chats", a.signedIn(a.createChat))
	mux.HandleFunc("DELETE /api/chats/{id}", a.signedIn(a.deleteChat))
	mux.HandleFunc("POST /api/chats/{id}/messages", a.signedIn(a.sendChatMessage))
	mux.HandleFunc("GET /api/chats/{id}/events", a.signedIn(a.chatEvents))
}

// LoadKey reads the signing key at path, creating a random one (mode 0600) the first time.
func LoadKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return key, err
	}
	key = make([]byte, 32)
	rand.Read(key)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		return nil, err
	}
	return key, f.Close()
}

// SignInNotice supplies the assistant with a short link and delivery instructions.
func (a *API) SignInNotice(phoneNumber, text string) (string, bool) {
	text = strings.TrimSpace(strings.TrimRightFunc(strings.TrimSpace(text), unicode.IsPunct))
	if !strings.EqualFold(text, SignInPhrase) {
		return "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	raw := make([]byte, 16)
	rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	// Store the signed identity, so expiry and identity remain authenticated on disk.
	path := filepath.Join(a.Users, phoneNumber, "web_link")
	err := os.WriteFile(path, []byte(token+" "+a.sign("sign_in_link", phoneNumber, signInLinkLifetime)), 0o600)
	if err != nil {
		return "The user's request for a sign-in link failed. Briefly ask them to try again.", true
	}
	return "The user requested web sign-in. Send this exact URL with at most three introductory words and no blank lines. It expires in 10 minutes: " + a.URL + "/s/" + token, true
}

func (a *API) shortLink(token string) (string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 16 {
		return "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	users, err := os.ReadDir(a.Users)
	if err != nil {
		return "", false
	}
	for _, user := range users {
		if !user.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(a.Users, user.Name(), "web_link"))
		if err != nil {
			continue
		}
		code, signed, _ := strings.Cut(string(data), " ")
		if hmac.Equal([]byte(code), []byte(token)) {
			return a.verify("sign_in_link", signed)
		}
	}
	return "", false
}

func (a *API) signInInstructions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"troy_phone_number": a.TroyPhoneNumber, "sign_in_phrase": SignInPhrase})
}

// signIn is the link Troy texts. It is not consumed when opened, so link previews that
// fetch it don't burn it; it signs in any browser that opens it until it expires.
func (a *API) signIn(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	phoneNumber, ok := a.shortLink(r.PathValue("token"))
	if strings.HasPrefix(r.URL.Path, "/sign-in/") {
		phoneNumber, ok = a.verify("sign_in_link", r.PathValue("token"))
	}
	if !ok || !a.exists(phoneNumber) {
		http.Error(w, "This sign-in link has expired. Text \""+SignInPhrase+"\" to Troy for a new one.", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: webSessionCookie, Value: a.sign("web_session", phoneNumber, webSessionLifetime),
		Path: "/", MaxAge: int(webSessionLifetime.Seconds()), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *API) signOut(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: webSessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) signedIn(h func(w http.ResponseWriter, r *http.Request, phoneNumber string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var phoneNumber string
		ok := false
		if c, err := r.Cookie(webSessionCookie); err == nil {
			phoneNumber, ok = a.verify("web_session", c.Value)
		}
		if !ok || !a.exists(phoneNumber) {
			writeError(w, http.StatusUnauthorized, "not_signed_in", "Sign in first.")
			return
		}
		h(w, r, phoneNumber)
	}
}

func (a *API) me(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	writeJSON(w, http.StatusOK, map[string]any{"phone_number": phoneNumber, "connections": a.connections(phoneNumber)})
}

func (a *API) listConnections(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	writeJSON(w, http.StatusOK, a.connections(phoneNumber))
}

func (a *API) exists(phoneNumber string) bool {
	_, err := os.Stat(filepath.Join(a.Users, phoneNumber))
	return err == nil
}

// A token is base64url("<phone number> <expiry unix>") "." base64url(HMAC-SHA256 of kind
// and that, truncated to 128 bits). The kind keeps a link from working as a session.
func (a *API) sign(kind, phoneNumber string, lifetime time.Duration) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(phoneNumber + " " + strconv.FormatInt(time.Now().Add(lifetime).Unix(), 10)))
	return payload + "." + a.mac(kind, payload)
}

func (a *API) verify(kind, token string) (phoneNumber string, ok bool) {
	payload, mac, _ := strings.Cut(token, ".")
	if !hmac.Equal([]byte(mac), []byte(a.mac(kind, payload))) {
		return "", false
	}
	raw, _ := base64.RawURLEncoding.DecodeString(payload)
	phoneNumber, expiry, _ := strings.Cut(string(raw), " ")
	unix, err := strconv.ParseInt(expiry, 10, 64)
	return phoneNumber, err == nil && time.Now().Unix() < unix
}

func (a *API) mac(kind, payload string) string {
	h := hmac.New(sha256.New, a.Key)
	h.Write([]byte(kind + "\n" + payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:16])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes {"error": {"code": "<stable_snake_case>", "message": "<for people>"}}.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
