package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"typstpad/internal/api"
	"typstpad/internal/auth"
	"typstpad/internal/blob"
	"typstpad/internal/collab"
	"typstpad/internal/compile"
	"typstpad/internal/config"
	"typstpad/internal/seed"
	"typstpad/internal/store"
	"typstpad/internal/versions"
	"typstpad/web"
)

func main() {
	root := &cobra.Command{
		Use:           "typstpad",
		Short:         "TypstPad — self-hosted collaborative Typst editor",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(serveCmd(), migrateCmd())
	addClientCommands(root)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the TypstPad server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			st, err := store.New(ctx, cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.Migrate(ctx); err != nil {
				return fmt.Errorf("migrate: %w", err)
			}

			bl, err := blob.New(filepath.Join(cfg.DataDir, "assets"))
			if err != nil {
				return err
			}
			cc := collab.New(cfg.CollabURL, cfg.CollabSecret)
			comp, err := compile.New(cfg.TypstBin,
				filepath.Join(cfg.DataDir, "work"),
				filepath.Join(cfg.DataDir, "typst-cache"),
				time.Duration(cfg.CompileTimeout)*time.Second)
			if err != nil {
				return err
			}

			hub := api.NewHub()
			snap := versions.NewSnapshotter(st, bl, cc, func(projectID, typ string) {
				hub.Publish(projectID, api.Event{Type: typ})
			})
			go snap.Run(ctx)

			srv := &api.Server{
				Cfg:      cfg,
				Store:    st,
				Auth:     &auth.Auth{Store: st, DevHTTP: cfg.DevHTTP},
				Blob:     bl,
				Hub:      hub,
				Collab:   cc,
				Compiler: comp,
				Versions: snap,
				SPA:      web.Dist(),
				OnDocStored: func(projectID string) {
					snap.MarkDirty(projectID)
				},
				OnFirstUser: func() {
					if err := seed.Templates(context.Background(), st); err != nil {
						slog.Error("template seeding failed", "err", err)
					}
				},
			}
			if err := srv.SetupOIDC(ctx); err != nil {
				return fmt.Errorf("oidc: %w", err)
			}
			// Seed templates on startup too (covers restarts after first user).
			if err := seed.Templates(ctx, st); err != nil {
				slog.Error("template seeding failed", "err", err)
			}

			httpSrv := &http.Server{
				Addr:              cfg.Addr,
				Handler:           srv.Router(),
				ReadHeaderTimeout: 10 * time.Second,
			}
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = httpSrv.Shutdown(shutdownCtx)
			}()
			slog.Info("typstpad listening", "addr", cfg.Addr)
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return err
			}
			return nil
		},
	}
}

func migrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations and exit",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			st, err := store.New(ctx, cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer st.Close()
			return st.Migrate(ctx)
		},
	}
}
