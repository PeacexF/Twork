package web

import (
	"net/http"
	"strconv"

	"github.com/PeacexF/Twork/internal/bot"
	"github.com/PeacexF/Twork/internal/models"
)

type chatResponse struct {
	TelegramID            int64  `json:"telegram_id"`
	Title                 string `json:"title"`
	Username              string `json:"username"`
	Tag                   string `json:"tag"`
	Kind                  string `json:"kind"`
	Paused                bool   `json:"paused"`
	ResumeEnabled         bool   `json:"resume_enabled"`
	ResumeIntervalSeconds int    `json:"resume_interval_seconds"`
	ResumeText            string `json:"resume_text"`
}

func toChatResponse(c models.Chat) chatResponse {
	return chatResponse{
		TelegramID:            c.TelegramID,
		Title:                 c.Title,
		Username:              c.Username,
		Tag:                   c.Tag,
		Kind:                  string(c.Kind),
		Paused:                c.Paused,
		ResumeEnabled:         c.ResumeEnabled,
		ResumeIntervalSeconds: c.ResumeIntervalSeconds,
		ResumeText:            c.ResumeText,
	}
}

func (s *Server) handleListChats(w http.ResponseWriter, r *http.Request) {
	chats, err := s.store.ListChats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]chatResponse, len(chats))
	for i, c := range chats {
		out[i] = toChatResponse(c)
	}
	writeJSON(w, http.StatusOK, out)
}

type addChatRequest struct {
	// a @username, invite link, or folder link -- same free-form input the
	// bot's "chat:add" prompt accepts
	Input string `json:"input"`
}

// adds one or more chats from a pasted username/invite/folder link, reusing
// the exact same parsing the bot's add-chat prompt uses
func (s *Server) handleAddChat(w http.ResponseWriter, r *http.Request) {
	var req addChatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	parsed, err := bot.ParseChatInput(req.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	switch parsed.Kind {
	case bot.ChatInputKindUsername:
		chat, err := s.source.AddByUsername(ctx, parsed.Value)
		s.respondAddedChat(w, chat, err)
	case bot.ChatInputKindInvite:
		chat, err := s.source.AddByInviteLink(ctx, parsed.Value)
		s.respondAddedChat(w, chat, err)
	case bot.ChatInputKindFolder:
		chats, err := s.source.AddFolder(ctx, parsed.Value)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		out := make([]chatResponse, len(chats))
		for i, c := range chats {
			out[i] = toChatResponse(*c)
		}
		writeJSON(w, http.StatusCreated, out)
	default:
		writeError(w, http.StatusBadRequest, "couldn't tell what that was -- send a @username or a t.me link")
	}
}

func (s *Server) respondAddedChat(w http.ResponseWriter, chat *models.Chat, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toChatResponse(*chat))
}

func (s *Server) handleDeleteChat(w http.ResponseWriter, r *http.Request) {
	id, ok := pathChatID(w, r)
	if !ok {
		return
	}
	if err := s.source.Remove(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePauseChat(w http.ResponseWriter, r *http.Request) {
	id, ok := pathChatID(w, r)
	if !ok {
		return
	}
	if err := s.source.Pause(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResumeChat(w http.ResponseWriter, r *http.Request) {
	id, ok := pathChatID(w, r)
	if !ok {
		return
	}
	if err := s.source.Resume(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type chatBroadcastRequest struct {
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"interval_seconds"`
	Text            string `json:"text"`
}

// updates a chat's resume broadcasting config. storage.SetChatResumeConfig
// is the single choke point that refuses to enable this on anything but a
// group -- its error (e.g. "not a group") is surfaced as a 400 here, not a
// 500, since it's a rejected request, not a server fault.
func (s *Server) handleSetChatResume(w http.ResponseWriter, r *http.Request) {
	id, ok := pathChatID(w, r)
	if !ok {
		return
	}
	var req chatBroadcastRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.SetChatResumeConfig(r.Context(), id, req.Enabled, req.IntervalSeconds, req.Text); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// extracts and parses the {id} path value shared by all /api/chats/{id}/* routes
func pathChatID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat id")
		return 0, false
	}
	return id, true
}
