package server

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func jsonError(w http.ResponseWriter, errStr string, code int) {
	w.WriteHeader(code)
	jsonOutput(w, map[string]string{"error": errStr})
}

func jsonOutput(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}

	w.Write(append(b, '\n'))
}

func (h *handler) getAuthHandler(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		Token string `json:"token"`
	}

	if h.Params.Passphrase == "" || h.checkRequestAuthorized(r) {
		k, err := h.sharedKey()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp.Token = fmt.Sprintf("%x", k)
	}

	jsonOutput(w, resp)
}

func (h *handler) postAuthHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error(err)
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if h.Params.Passphrase != req.Passphrase {
		log.Info("Client provided password was incorrect")
		jsonError(w, "Incorrect passphrase", http.StatusOK)
		return
	}

	log.Debug("Client successfully authenticated")
	k, err := h.sharedKey()
	if err != nil {
		log.WithError(err).Error("Generating shared key failed")
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOutput(w, struct {
		Token string `json:"token"`
	}{
		fmt.Sprintf("%x", k),
	})
}

func (h *handler) sharedKey() ([]byte, error) {
	var b []byte

	switch {
	case h.Params.APIKey != nil:
		b = h.Params.APIKey
	case h.Params.Passphrase != "":
		hash := sha1.Sum([]byte(h.Params.Passphrase))
		b = hash[0:16]
	default:
		b = make([]byte, 16)
		for i := range b {
			b[i] = byte(rand.Intn(256))
		}
	}

	return b, nil
}

func (h *handler) checkAPIKey(s string) (result bool) {
	k, err := h.sharedKey()
	if err != nil {
		return false
	}
	if s == fmt.Sprintf("%x", k) {
		return true
	}
	log.Printf("Incorrect api key %q, expected %x", s, k)
	return false
}

func (h *handler) checkRequestAuthorized(r *http.Request) bool {
	if auth := r.Header.Get("Authorization"); auth != "" {
		log.WithField("method", "header").Debug("Authenticating request")
		return h.checkAPIKey(strings.TrimPrefix(auth, "apitoken "))
	} else if apiKey := r.URL.Query().Get("apikey"); apiKey != "" {
		log.WithField("method", "query-string").Debug("Authenticating request")
		return h.checkAPIKey(apiKey)
	}
	log.Debug("Failed to authenticate request")
	return false
}
