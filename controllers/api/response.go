package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	log "github.com/gophish/gophish/logger"
)

// JSONResponse attempts to set the status code, c, and marshal the given interface, d, into a response that
// is written to the given ResponseWriter.
func JSONResponse(w http.ResponseWriter, d interface{}, c int) {
	dj, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		http.Error(w, "Error creating JSON response", http.StatusInternalServerError)
		log.Error(err)
	}
	w.Header().Set("Content-Type", "application/json")
	// Every API response is per-user data behind auth; don't let browsers or
	// intermediate caches store it (ref gophish/gophish#2022 - stale data
	// shown after switching accounts in the same browser).
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(c)
	
	_, _ = fmt.Fprintf(w, "%s", dj)
}
