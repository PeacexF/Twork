package web

import "net/http"

type statsResponse struct {
	ChatsMonitored  int  `json:"chats_monitored"`
	MessagesIndexed int  `json:"messages_indexed"`
	TodayMatches    int  `json:"today_matches"`
	Bookmarks       int  `json:"bookmarks"`
	Ignored         int  `json:"ignored"`
	CanSend         bool `json:"can_send"` // false when the active chat source (e.g. RSSHub) can't post messages
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, statsResponse{
		ChatsMonitored:  stats.ChatsMonitored,
		MessagesIndexed: stats.MessagesIndexed,
		TodayMatches:    stats.TodayMatches,
		Bookmarks:       stats.Bookmarks,
		Ignored:         stats.Ignored,
		CanSend:         s.sender != nil,
	})
}
