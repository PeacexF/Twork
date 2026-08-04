// Package web serves the optional local dashboard: a more comfortable place
// than the bot's menus to manage monitored chats and edit the resume
// broadcasting settings (global text, per-chat overrides, compliance
// limits). Off by default -- see config.WebConfig.
package web

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/PeacexF/Twork/internal/bot"
	"github.com/PeacexF/Twork/internal/broadcaster"
	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/storage"
)

// Server hosts the dashboard's JSON API and static assets
type Server struct {
	store  *storage.Store
	source bot.ChatSource
	sender broadcaster.Sender // nil when the active source can't send (e.g. RSSHub)

	httpServer *http.Server
}

// builds a Server; sender may be nil if the active chat source can't send
func New(store *storage.Store, source bot.ChatSource, sender broadcaster.Sender, cfg config.WebConfig) *Server {
	s := &Server{store: store, source: source, sender: sender}

	mux := http.NewServeMux()
	s.routes(mux)

	s.httpServer = &http.Server{
		Addr:    cfg.Addr,
		Handler: basicAuth(cfg.Username, cfg.Password, mux),
	}
	return s
}

// runs the dashboard's HTTP server until ctx is cancelled. Binds the port
// synchronously before returning, so a conflict (e.g. another process
// already on cfg.Addr) surfaces immediately as an error instead of only
// showing up asynchronously once the caller happens to notice.
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("binding %s: %w", s.httpServer.Addr, err)
	}
	log.Printf("twork: web dashboard listening on %s", s.httpServer.Addr)

	errCh := make(chan error, 1)
	go func() { errCh <- s.httpServer.Serve(listener) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		<-errCh
		return ctx.Err()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return ctx.Err()
	}
}
