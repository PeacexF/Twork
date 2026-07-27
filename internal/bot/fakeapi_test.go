package bot

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// one call the bot made to the Telegram Bot API
type apiCall struct {
	method string
	params url.Values
}

// text returns the "text" parameter of the call, if any
func (c apiCall) text() string { return c.params.Get("text") }

// a stand-in Bot API server that records every call and answers each with a
// canned success, so handler code can be exercised without a real bot token
type fakeAPI struct {
	mu    sync.Mutex
	calls []apiCall
}

// records calls and replies with a plausible success payload
func (f *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	method := path.Base(r.URL.Path)
	f.mu.Lock()
	f.calls = append(f.calls, apiCall{method: method, params: r.Form})
	f.mu.Unlock()

	result := `{"message_id":100,"date":1,"chat":{"id":500,"type":"private"}}`
	if method == "getMe" {
		result = `{"id":1,"is_bot":true,"username":"twork_test_bot"}`
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"result":` + result + `}`))
}

// every call made so far, in order
func (f *fakeAPI) snapshot() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]apiCall(nil), f.calls...)
}

// the calls made to one API method
func (f *fakeAPI) callsTo(method string) []apiCall {
	var out []apiCall
	for _, c := range f.snapshot() {
		if c.method == method {
			out = append(out, c)
		}
	}
	return out
}

// the most recent call to a method, failing the test if there wasn't one
func (f *fakeAPI) lastCallTo(t *testing.T, method string) apiCall {
	t.Helper()
	calls := f.callsTo(method)
	if len(calls) == 0 {
		t.Fatalf("expected at least one %s call, got %+v", method, f.methodNames())
	}
	return calls[len(calls)-1]
}

// the ordered list of method names called, for failure messages
func (f *fakeAPI) methodNames() []string {
	calls := f.snapshot()
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.method)
	}
	return names
}

// forgets everything recorded so far
func (f *fakeAPI) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// asserts that the last rendered screen contains every given fragment
func (f *fakeAPI) assertScreenContains(t *testing.T, method string, wants ...string) {
	t.Helper()
	got := f.lastCallTo(t, method).text()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("%s text is missing %q:\n%s", method, want, got)
		}
	}
}

// builds a *tgbotapi.BotAPI wired to a fake Telegram server
func newFakeBotAPI(t *testing.T) (*tgbotapi.BotAPI, *fakeAPI) {
	t.Helper()

	fake := &fakeAPI{}
	srv := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(srv.Close)

	api, err := tgbotapi.NewBotAPIWithAPIEndpoint("test-token", srv.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("building the fake bot api: %v", err)
	}
	fake.reset() // drop the getMe handshake

	return api, fake
}
