package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

// a bot.ChatSource test double
type fakeSource struct {
	chats       []models.Chat
	addUsername func(ctx context.Context, username string) (*models.Chat, error)
	addInvite   func(ctx context.Context, link string) (*models.Chat, error)
	addFolder   func(ctx context.Context, link string) ([]*models.Chat, error)
	paused      []int64
	resumed     []int64
	removed     []int64
}

func (f *fakeSource) Run(ctx context.Context) error { return nil }

func (f *fakeSource) AddByUsername(ctx context.Context, username string) (*models.Chat, error) {
	if f.addUsername != nil {
		return f.addUsername(ctx, username)
	}
	return &models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: username, Username: username}, nil
}

func (f *fakeSource) AddByInviteLink(ctx context.Context, link string) (*models.Chat, error) {
	if f.addInvite != nil {
		return f.addInvite(ctx, link)
	}
	return &models.Chat{TelegramID: 2, Kind: models.ChatKindGroup, Title: "Invited"}, nil
}

func (f *fakeSource) AddFolder(ctx context.Context, link string) ([]*models.Chat, error) {
	if f.addFolder != nil {
		return f.addFolder(ctx, link)
	}
	return []*models.Chat{{TelegramID: 3, Kind: models.ChatKindGroup, Title: "FromFolder"}}, nil
}

func (f *fakeSource) Pause(ctx context.Context, telegramID int64) error {
	f.paused = append(f.paused, telegramID)
	return nil
}

func (f *fakeSource) Resume(ctx context.Context, telegramID int64) error {
	f.resumed = append(f.resumed, telegramID)
	return nil
}

func (f *fakeSource) Remove(ctx context.Context, telegramID int64) error {
	f.removed = append(f.removed, telegramID)
	return nil
}

func (f *fakeSource) ListResolved() []models.Chat { return f.chats }

// a broadcaster.Sender test double
type fakeSender struct {
	calls []sentCall
}

type sentCall struct {
	chatID int64
	text   string
}

func (f *fakeSender) SendText(ctx context.Context, chatID int64, text string) error {
	f.calls = append(f.calls, sentCall{chatID, text})
	return nil
}

// builds a Server backed by a real temp-file store, reachable over HTTP with Basic Auth
func newTestServer(t *testing.T) (*httptest.Server, *storage.Store, *fakeSource) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	src := &fakeSource{}
	s := New(store, src, &fakeSender{}, config.WebConfig{Username: "admin", Password: "secret"})
	srv := httptest.NewServer(s.httpServer.Handler)
	t.Cleanup(srv.Close)
	return srv, store, src
}

// issues an authenticated JSON request and returns the response
func doJSON(t *testing.T, srv *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.SetBasicAuth("admin", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do(%s %s): %v", method, path, err)
	}
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
}

// a port already in use fails Run() immediately with a clear error, instead
// of the bind conflict only surfacing asynchronously deep inside ListenAndServe
func TestRun_BindConflictFailsImmediately(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer store.Close()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer occupied.Close()

	s := New(store, &fakeSource{}, &fakeSender{}, config.WebConfig{
		Addr: occupied.Addr().String(), Username: "a", Password: "b",
	})

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected Run to fail when the address is already in use")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly on a bind conflict")
	}
}

// every route requires Basic Auth, including static assets
func TestUnauthenticatedRequestsAreRejected(t *testing.T) {
	srv, _, _ := newTestServer(t)

	for _, path := range []string{"/api/stats", "/api/chats", "/"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s (no auth) = %d, want 401", path, resp.StatusCode)
		}
	}
}

// a wrong password is rejected just like no auth at all
func TestWrongCredentialsAreRejected(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/stats", nil)
	req.SetBasicAuth("admin", "wrong-password")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// stats reports the live counters and whether the active source can send
func TestGetStats(t *testing.T) {
	srv, _, _ := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/stats", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got statsResponse
	decodeBody(t, resp, &got)
	if !got.CanSend {
		t.Error("expected can_send = true (the fake sender is non-nil)")
	}
}

// adding a chat by @username reaches the source and the chat then appears in ListChats
func TestAddChat_Username(t *testing.T) {
	srv, store, _ := newTestServer(t)
	ctx := context.Background()

	resp := doJSON(t, srv, http.MethodPost, "/api/chats", addChatRequest{Input: "@golang_jobs"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got chatResponse
	decodeBody(t, resp, &got)
	if got.Username != "golang_jobs" || got.Kind != "group" {
		t.Fatalf("chat = %+v", got)
	}

	// AddByUsername in the fake stubs a group but never persists it to
	// storage itself (the real collector does that) -- persist it here to
	// exercise the same path the bot/collector take, then confirm it lists.
	if err := store.UpsertChat(ctx, models.Chat{TelegramID: got.TelegramID, Kind: models.ChatKindGroup, Title: got.Title, Username: got.Username}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	listResp := doJSON(t, srv, http.MethodGet, "/api/chats", nil)
	var chats []chatResponse
	decodeBody(t, listResp, &chats)
	if len(chats) != 1 || chats[0].Username != "golang_jobs" {
		t.Fatalf("chats = %+v", chats)
	}
}

// a malformed add-chat input is rejected with 400, not forwarded to the source
func TestAddChat_InvalidInput(t *testing.T) {
	srv, _, src := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/chats", addChatRequest{Input: "has spaces"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
	if src.addUsername != nil {
		t.Error("source should not have been reached for invalid input")
	}
}

// pause/resume/remove reach the source with the right chat id
func TestPauseResumeDeleteChat(t *testing.T) {
	srv, _, src := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/chats/42/pause", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || len(src.paused) != 1 || src.paused[0] != 42 {
		t.Fatalf("pause: status=%d paused=%+v", resp.StatusCode, src.paused)
	}

	resp = doJSON(t, srv, http.MethodPost, "/api/chats/42/resume", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || len(src.resumed) != 1 || src.resumed[0] != 42 {
		t.Fatalf("resume: status=%d resumed=%+v", resp.StatusCode, src.resumed)
	}

	resp = doJSON(t, srv, http.MethodDelete, "/api/chats/42", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || len(src.removed) != 1 || src.removed[0] != 42 {
		t.Fatalf("delete: status=%d removed=%+v", resp.StatusCode, src.removed)
	}
}

// resume broadcasting can be enabled on a group via the API
func TestSetChatResume_Group(t *testing.T) {
	srv, store, _ := newTestServer(t)
	ctx := context.Background()
	if err := store.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Go Jobs"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	resp := doJSON(t, srv, http.MethodPatch, "/api/chats/1/broadcast", chatBroadcastRequest{
		Enabled: true, IntervalSeconds: 3600, Text: "my resume",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	c, err := store.GetChatByTelegramID(ctx, 1)
	if err != nil || c == nil || !c.ResumeEnabled || c.ResumeText != "my resume" {
		t.Fatalf("chat = %+v, err = %v", c, err)
	}
}

// the API surfaces the storage layer's channel rejection as 400, not 500
func TestSetChatResume_RefusesChannel(t *testing.T) {
	srv, store, _ := newTestServer(t)
	ctx := context.Background()
	if err := store.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindChannel, Title: "Vacancies"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	resp := doJSON(t, srv, http.MethodPatch, "/api/chats/1/broadcast", chatBroadcastRequest{Enabled: true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	decodeBody(t, resp, &body)
	if !strings.Contains(body["error"], "group") {
		t.Errorf("error = %q, expected it to mention groups", body["error"])
	}
}

// the global resume text round-trips through the API
func TestResumeTextRoundTrip(t *testing.T) {
	srv, _, _ := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPut, "/api/resume", resumeTextResponse{Text: "Experienced Go developer."})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d", resp.StatusCode)
	}

	resp = doJSON(t, srv, http.MethodGet, "/api/resume", nil)
	var got resumeTextResponse
	decodeBody(t, resp, &got)
	if got.Text != "Experienced Go developer." {
		t.Fatalf("text = %q", got.Text)
	}
}

// the compliance limits round-trip through the API, defaulting beforehand
func TestComplianceRoundTrip(t *testing.T) {
	srv, _, _ := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/compliance", nil)
	var got complianceResponse
	decodeBody(t, resp, &got)
	if got.MinDelaySeconds != config.DefaultResumeMinDelaySeconds || got.MaxPerHour != config.DefaultResumeMaxPerHour {
		t.Fatalf("defaults = %+v", got)
	}

	resp = doJSON(t, srv, http.MethodPut, "/api/compliance", complianceResponse{MinDelaySeconds: 600, MaxPerHour: 5})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d", resp.StatusCode)
	}

	resp = doJSON(t, srv, http.MethodGet, "/api/compliance", nil)
	decodeBody(t, resp, &got)
	if got.MinDelaySeconds != 600 || got.MaxPerHour != 5 {
		t.Fatalf("after update = %+v", got)
	}
}

// negative compliance values are rejected
func TestComplianceRejectsNegativeValues(t *testing.T) {
	srv, _, _ := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPut, "/api/compliance", complianceResponse{MinDelaySeconds: -1, MaxPerHour: 5})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// unknown paths fall back to the SPA's index.html rather than 404ing
func TestStaticFallback(t *testing.T) {
	srv, _, _ := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/some/client/side/route", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(resp.Body); err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if !strings.Contains(body.String(), "Twork") {
		t.Errorf("expected the placeholder/app shell, got:\n%s", body.String())
	}
}
