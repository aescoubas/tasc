package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/aescoubas/tasc/internal/store"
	"github.com/google/uuid"
)

type actorKey struct{}

func (s *Server) requestStore(r *http.Request) store.Store {
	if actor, ok := r.Context().Value(actorKey{}).(string); ok {
		return s.store.ForActor(actor)
	}
	return s.store
}

// A separate URL namespace makes older servers reject scoped writes.
func (s *Server) serveActor(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/actors/"), "/")
	if !ok || (actor != store.GuestActor && uuid.Validate(actor) != nil) {
		http.Error(w, "invalid agent run", http.StatusBadRequest)
		return
	}
	req := r.WithContext(context.WithValue(r.Context(), actorKey{}, actor))
	http.StripPrefix("/actors/"+actor, s.mux).ServeHTTP(w, req)
}

func (s *Server) handleAssignments(w http.ResponseWriter, r *http.Request) {
	st := s.requestStore(r)
	if r.Method == http.MethodGet {
		rows, err := st.Assignments()
		if err != nil {
			respondMutationError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, rows)
		return
	}
	var change store.AssignmentChange
	if err := json.NewDecoder(r.Body).Decode(&change); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rows, err := st.ChangeAssignments(change)
	if errors.Is(err, store.ErrLeaseExpired) {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusOK, rows)
}
