package api

import (
	"net/http"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
)

// Placeholders. Each method moves to its own `handlers_*.go` file as
// it is implemented; this file shrinks to zero by the end of Adım 7.

func (s *Server) BlastRadius(w http.ResponseWriter, _ *http.Request) {
	writeNotImplemented(w, "BlastRadius")
}

func (s *Server) ListIncidents(w http.ResponseWriter, _ *http.Request, _ gen.ListIncidentsParams) {
	writeNotImplemented(w, "ListIncidents")
}

func (s *Server) GetIncident(w http.ResponseWriter, _ *http.Request, _ string) {
	writeNotImplemented(w, "GetIncident")
}

func (s *Server) AddRemediation(w http.ResponseWriter, _ *http.Request, _ string) {
	writeNotImplemented(w, "AddRemediation")
}

func (s *Server) TransitionIncident(w http.ResponseWriter, _ *http.Request, _ string) {
	writeNotImplemented(w, "TransitionIncident")
}


