package api

import (
	"net/http"
)

// Placeholders. Each method moves to its own `handlers_*.go` file as
// it is implemented; this file shrinks to zero by the end of Adım 7.

func (s *Server) AddRemediation(w http.ResponseWriter, _ *http.Request, _ string) {
	writeNotImplemented(w, "AddRemediation")
}


