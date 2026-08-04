package web

import (
	"net/http"

	"github.com/PeacexF/Twork/internal/config"
)

type complianceResponse struct {
	MinDelaySeconds int `json:"min_delay_seconds"`
	MaxPerHour      int `json:"max_per_hour"`
}

func (s *Server) handleGetCompliance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	minDelay, err := s.store.GetResumeMinDelaySeconds(ctx, config.DefaultResumeMinDelaySeconds)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	maxPerHour, err := s.store.GetResumeMaxPerHour(ctx, config.DefaultResumeMaxPerHour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, complianceResponse{MinDelaySeconds: minDelay, MaxPerHour: maxPerHour})
}

// updates the compliance limits. Values below the hardcoded recommended
// defaults are still accepted -- overriding them is a deliberate, if
// discouraged, choice the operator can make -- but never negative ones.
func (s *Server) handleSetCompliance(w http.ResponseWriter, r *http.Request) {
	var req complianceResponse
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.MinDelaySeconds < 0 || req.MaxPerHour < 0 {
		writeError(w, http.StatusBadRequest, "values must not be negative")
		return
	}
	ctx := r.Context()
	if err := s.store.SetResumeMinDelaySeconds(ctx, req.MinDelaySeconds); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.SetResumeMaxPerHour(ctx, req.MaxPerHour); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
