package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/brightspace"
	"github.com/nsigel/usc-cli/site"
)

// A connection is an outside account Troy can use for a user. Each kind adds its own
// fields (a USC account its net_id and first_name) and is listed whether connected or not.
type connection struct {
	Kind      string `json:"kind"`   // usc_account
	Status    string `json:"status"` // connected or not_connected
	NetID     string `json:"net_id,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

func (a *API) connections(phoneNumber string) []connection {
	return []connection{a.uscAccount(phoneNumber)}
}

// uscDir holds a user's USC sign-in in usc-cli's credentials.json and session.json, and who
// they are on Brightspace in student.json (uscFiles).
// It is outside workspace/, and mounted into the agent's container for its usc command.
func (a *API) uscDir(phoneNumber string) string {
	return filepath.Join(a.Users, phoneNumber, "usc")
}

var uscFiles = []string{"credentials.json", "session.json", "student.json"}

func (a *API) uscAccount(phoneNumber string) connection {
	c := connection{Kind: "usc_account", Status: "not_connected"}
	if credentials, err := auth.LoadCredentials(filepath.Join(a.uscDir(phoneNumber), "credentials.json")); err == nil && credentials.Username != "" {
		c.Status, c.NetID = "connected", credentials.Username
		var student brightspace.User
		if data, err := os.ReadFile(filepath.Join(a.uscDir(phoneNumber), "student.json")); err == nil && json.Unmarshal(data, &student) == nil {
			c.FirstName = student.FirstName
		}
	}
	return c
}

// connectUSCAccount signs in to USC with the credentials and keeps them only if that works.
func (a *API) connectUSCAccount(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	var body struct {
		NetID         string `json:"net_id"`
		Password      string `json:"password"`
		DuoBypassCode string `json:"duo_bypass_code"`
	}
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	credentials := auth.Credentials{Username: strings.TrimSpace(body.NetID), Password: body.Password, BypassCode: strings.TrimSpace(body.DuoBypassCode)}
	if err != nil || !credentials.Complete() {
		writeError(w, http.StatusBadRequest, "invalid_request", "net_id, password, and duo_bypass_code are required.")
		return
	}
	// Sign in fresh into a new directory, then move its files in, so failure leaves any old
	// connection. The usc directory itself stays: the student's container has it mounted.
	staging, err := os.MkdirTemp(filepath.Join(a.Users, phoneNumber), ".usc-")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Something broke on our side.")
		return
	}
	defer os.RemoveAll(staging)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	session, err := auth.Open(ctx, site.Brightspace, auth.Options{Credentials: credentials, SessionFile: filepath.Join(staging, "session.json")})
	switch {
	case errors.Is(err, auth.ErrPasswordRejected):
		writeError(w, http.StatusUnprocessableEntity, "password_rejected", "USC rejected the NetID or password.")
		return
	case errors.Is(err, auth.ErrBypassRejected):
		writeError(w, http.StatusUnprocessableEntity, "duo_bypass_code_rejected", "Duo rejected the bypass code. Get a new one and try again.")
		return
	case err != nil:
		log.Printf("usc sign-in for %s failed: %v", phoneNumber, err)
		writeError(w, http.StatusBadGateway, "usc_unavailable", "Couldn't sign in to USC right now. Try again in a bit.")
		return
	}
	// Who they are is nice to have, not worth failing the connection over.
	student, err := brightspace.New(session).WhoAmI(ctx)
	if err != nil {
		log.Printf("brightspace whoami for %s failed: %v", phoneNumber, err)
	}
	data, _ := json.Marshal(student)
	err = os.WriteFile(filepath.Join(staging, "student.json"), data, 0o600)
	if err == nil {
		err = auth.SaveCredentials(filepath.Join(staging, "credentials.json"), credentials)
	}
	if err == nil {
		a.mu.Lock()
		err = os.MkdirAll(a.uscDir(phoneNumber), 0o700)
		for _, name := range uscFiles {
			if err == nil {
				err = os.Rename(filepath.Join(staging, name), filepath.Join(a.uscDir(phoneNumber), name))
			}
		}
		a.mu.Unlock()
	}
	if err != nil {
		log.Printf("saving usc account for %s: %v", phoneNumber, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Something broke on our side.")
		return
	}
	who := "NetID " + credentials.Username
	if name := strings.TrimSpace(student.FirstName + " " + student.LastName); name != "" {
		who = name + ", " + who
	}
	// The notice waits for USCChanged, which fetches grades, so the agent sees the result.
	go func() {
		a.Notify(phoneNumber, strings.TrimSpace("The user just connected their USC account ("+who+
			") on the Troy website. You can now use it through the usc command. "+
			"Send a brief text confirming their USC account is connected and offer to check what is due this week. "+
			"Ask before checking; do not fetch or list deadlines yet. "+a.USCChanged(phoneNumber, true)))
	}()
	writeJSON(w, http.StatusOK, a.uscAccount(phoneNumber))
}

func (a *API) disconnectUSCAccount(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	a.mu.Lock()
	existed := a.uscAccount(phoneNumber).Status == "connected"
	var err error
	for _, name := range uscFiles {
		if e := os.Remove(filepath.Join(a.uscDir(phoneNumber), name)); e != nil && !errors.Is(e, fs.ErrNotExist) {
			err = e
		}
	}
	a.mu.Unlock()
	if err != nil {
		log.Printf("removing usc account for %s: %v", phoneNumber, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Something broke on our side.")
		return
	}
	a.USCChanged(phoneNumber, false)
	if existed {
		a.Notify(phoneNumber, "The user disconnected their USC account on the Troy website. Troy no longer has their USC credentials.")
	}
	w.WriteHeader(http.StatusNoContent)
}
