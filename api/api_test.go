package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShortLink(t *testing.T) {
	a := &API{Users: t.TempDir(), Key: []byte("test key"), URL: "https://example.com"}
	os.Mkdir(filepath.Join(a.Users, "student"), 0700)
	notice, ok := a.SignInNotice("student", SignInPhrase)
	if !ok {
		t.Fatal("request not recognized")
	}
	token := strings.Split(notice, "/s/")[1]
	if len(token) != 22 {
		t.Fatalf("code length %d", len(token))
	}
	// A new API instance resolves the persisted link; repeated previews don't consume it.
	b := &API{Users: a.Users, Key: a.Key}
	mux := http.NewServeMux()
	b.Register(mux)
	for range 2 {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/s/"+token, nil))
		if w.Code != 303 || len(w.Result().Cookies()) != 1 {
			t.Fatalf("sign in: %d", w.Code)
		}
	}
	if _, ok := b.shortLink(strings.Repeat("a", 22)); ok {
		t.Fatal("accepted unknown token")
	}
	os.WriteFile(filepath.Join(a.Users, "student", "web_link"), []byte(token+" "+a.sign("sign_in_link", "student", -time.Minute)), 0600)
	if _, ok := b.shortLink(token); ok {
		t.Fatal("accepted expired token")
	}
}
