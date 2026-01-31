package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
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

type Server struct {
	host     string
	port     int
	password string
}

func (s *Server) passkey() string {
	if s.password != "" {
		return s.password
	}
	return os.Getenv("TCR_PASSKEY")
}

func (s *Server) Start(ctx context.Context) error {
	options := []ssh.Option{
		wish.WithAddress(s.host + ":" + strconv.Itoa(s.port)),
		wish.WithMiddleware(
			func(next ssh.Handler) ssh.Handler {
				return func(sess ssh.Session) {
					sess.Write([]byte("hi"))
					next(sess)
				}
			},
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
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			done <- err
		} else {
			done <- nil
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
}

func (s *Server) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if err := s.Start(ctx); err != nil {
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
