package handler

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"
)

const maxJSONBodyBytes = 1 << 20 // 1MB

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func isValidEmail(v string) bool {
	if len(v) > 320 || strings.TrimSpace(v) == "" {
		return false
	}
	_, err := mail.ParseAddress(v)
	return err == nil
}
