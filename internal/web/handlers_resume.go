package web

import "net/http"

type resumeTextResponse struct {
	Text string `json:"text"`
}

// the global resume text -- the default used by any chat without its own override
func (s *Server) handleGetResumeText(w http.ResponseWriter, r *http.Request) {
	text, err := s.store.GetResumeGlobalText(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resumeTextResponse{Text: text})
}

func (s *Server) handleSetResumeText(w http.ResponseWriter, r *http.Request) {
	var req resumeTextResponse
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.SetResumeGlobalText(r.Context(), req.Text); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
