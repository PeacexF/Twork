package web

import "net/http"

// registers the dashboard's JSON API and static asset handler
func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stats", s.handleGetStats)

	mux.HandleFunc("GET /api/chats", s.handleListChats)
	mux.HandleFunc("POST /api/chats", s.handleAddChat)
	mux.HandleFunc("DELETE /api/chats/{id}", s.handleDeleteChat)
	mux.HandleFunc("POST /api/chats/{id}/pause", s.handlePauseChat)
	mux.HandleFunc("POST /api/chats/{id}/resume", s.handleResumeChat)
	mux.HandleFunc("PATCH /api/chats/{id}/broadcast", s.handleSetChatResume)

	mux.HandleFunc("GET /api/resume", s.handleGetResumeText)
	mux.HandleFunc("PUT /api/resume", s.handleSetResumeText)

	mux.HandleFunc("GET /api/compliance", s.handleGetCompliance)
	mux.HandleFunc("PUT /api/compliance", s.handleSetCompliance)

	mux.Handle("/", staticHandler())
}
