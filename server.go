package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/google/subcommands"
)

func SlogMiddleware() wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(sess ssh.Session) {
			ct := time.Now()
			hpk := sess.PublicKey() != nil
			pty, _, _ := sess.Pty()
			slog.Info(
				"connect",
				"user", sess.User(),
				"remote-addr", sess.RemoteAddr().String(),
				"public-key", hpk,
				"command", sess.Command(),
				"term", pty.Term,
				"width", pty.Window.Width,
				"height", pty.Window.Height,
				"client-version", sess.Context().ClientVersion(),
			)
			next(sess)
			slog.Info(
				"disconnect",
				"user", sess.User(),
				"remote-addr", sess.RemoteAddr().String(),
				"duration", time.Since(ct),
			)
		}
	}
}

func fetchReviews(ctx context.Context, client *GitHubPRClient, projects []*Project) error {
	for _, p := range projects {
		for _, w := range p.worktrees {
			hasReview, err := w.review(ctx, client)
			if err != nil {
				return err
			}
			if hasReview {
				slog.Info("got review", "repo", p.Title(), "branch", w.Name)
			}
		}
	}
	return nil
}

func applyChanges(ctx context.Context, projects []*Project) error {
	for _, p := range projects {
		for _, w := range p.worktrees {
			switch {
			case w.Status.IsComplete:
			case len(w.Status.ApplyRequires) == 1 && slices.Contains(w.Status.ApplyRequires, "tasks"):
				if _, err := ocCommand(ctx, w.Path, w.Model, "opsx-apply"); err != nil {
					return err
				}
				if err := push(ctx, w.Path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func pullMain(ctx context.Context, projects []*Project) error {
	for _, p := range projects {
		if err := pull(ctx, p.repoPath); err != nil {
			return err
		}
	}
	return nil
}

type Server struct {
	host      string
	port      int
	password  string
	workspace string
	interval  time.Duration
}

func (s *Server) passkey() string {
	if s.password != "" {
		return s.password
	}
	return os.Getenv("TCR_PASSKEY")
}

func (s *Server) Start(ctx context.Context) error {
	if err := bootstrapWorkspace(s.workspace); err != nil {
		return err
	}
	options := []ssh.Option{
		wish.WithAddress(s.host + ":" + strconv.Itoa(s.port)),
		ssh.AllocatePty(),
		wish.WithMiddleware(
			bubbletea.Middleware(NewTeaHandler(s.workspace)),
			activeterm.Middleware(),
			SlogMiddleware(),
		),
	}
	if key := s.passkey(); key != "" {
		options = append(options, wish.WithPasswordAuth(func(ctx ssh.Context, password string) bool {
			return password == s.password
		}))
	}
	server, err := wish.NewServer(options...)
	if err != nil {
		return err
	}
	done := make(chan error)
	go func() {
		slog.Info(fmt.Sprintf("start listening on %s:%d", s.host, s.port))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			done <- err
		} else {
			done <- nil
		}
	}()
	go func() {
		client := NewGitHubPRClient("")
		for range time.Tick(s.interval) {
			tCtx, cancel := context.WithTimeout(ctx, s.interval)
			projects, err := LoadWorkspace(tCtx, s.workspace)
			if err != nil {
				slog.Error(err.Error())
			}
			if err := pullMain(tCtx, projects); err != nil {
				slog.Error(err.Error())
			}
			if err := fetchReviews(tCtx, client, projects); err != nil {
				slog.Error(err.Error())
			}
			if err := applyChanges(tCtx, projects); err != nil {
				slog.Error(err.Error())
			}
			cancel()
		}
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		done <- server.Shutdown(shutdownCtx)
	case err := <-done:
		return err
	}
	return nil
}

func (*Server) Name() string     { return "server" }
func (*Server) Synopsis() string { return "start tcr server" }
func (*Server) Usage() string    { return "" }

func (s *Server) SetFlags(f *flag.FlagSet) {
	f.StringVar(&s.host, "host", "127.0.0.1", "server host IP address")
	f.IntVar(&s.port, "port", 2222, "server port number to run on")
	f.StringVar(&s.password, "passkey", "", "passkey for server (empty for no auth)")
	f.DurationVar(&s.interval, "interval", 15*time.Minute, "review refresh interval")
	home, _ := os.UserHomeDir()
	ws := filepath.Join(home, ".local", "share", "tcr")
	f.StringVar(&s.workspace, "workspace", ws, "dir for git worktree")
}

func (s *Server) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if err := s.Start(ctx); err != nil {
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
