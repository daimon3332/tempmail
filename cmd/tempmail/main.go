package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tempmail/internal/auth"
	"tempmail/internal/config"
	"tempmail/internal/db"
	"tempmail/internal/importer"
	"tempmail/internal/inbound"
	"tempmail/internal/mailer"
	"tempmail/internal/roles"
	"tempmail/internal/server"
	"tempmail/web"
)

var version = "dev"

func main() {
	importPath := flag.String("import", "", "import a D1 SQL dump into the database and exit")
	merge := flag.Bool("merge", false, "with -import: drop ids and merge into an existing database")
	flag.Parse()

	cfg := config.Load()
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	if *importPath != "" {
		if err := runImport(database, *importPath, *merge); err != nil {
			log.Fatalf("import: %v", err)
		}
		return
	}

	signer := auth.New(cfg.JWTSecret)
	m := mailer.New(cfg)
	rs := roles.New(cfg, database)

	var webFS fs.FS
	if sub, err := fs.Sub(web.Dist, "dist"); err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			webFS = sub
		}
	}
	app := server.New(cfg, database, signer, m, rs, webFS)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server.StartScheduler(ctx, app, time.Hour)

	httpSrv := &http.Server{Addr: cfg.HTTPAddr, Handler: app.Handler(), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		log.Printf("tempmail %s http on %s domains=%v", version, cfg.HTTPAddr, cfg.Domains)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http: %v", err)
		}
	}()

	smtpSrv := inbound.New(cfg, database, m, app)
	app.SetIngest(smtpSrv.Ingest)
	go func() {
		log.Printf("smtp on %s hostname=%s", cfg.SMTPAddr, cfg.SMTPHostname)
		if err := smtpSrv.ListenAndServe(); err != nil {
			log.Fatalf("smtp: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down")
	cancel()
	shutdownCtx, c2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer c2()
	httpSrv.Shutdown(shutdownCtx)
	smtpSrv.Close()
}

func runImport(database *db.DB, path string, merge bool) error {
	sqlText, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ctx := context.Background()
	before := counts(ctx, database)
	mode := importer.Primary
	if merge {
		mode = importer.Merge
	}
	st, err := importer.Run(ctx, database, string(sqlText), mode)
	if err != nil {
		return err
	}
	after := counts(ctx, database)
	fmt.Printf("statements executed=%d skipped=%d\n", st.Executed, st.Skipped)
	for _, t := range []string{"address", "raw_mails", "users", "users_address", "user_roles", "sendbox", "settings", "address_sender"} {
		fmt.Printf("%-16s %8d -> %8d\n", t, before[t], after[t])
	}
	return nil
}

func counts(ctx context.Context, database *db.DB) map[string]int64 {
	out := map[string]int64{}
	for _, t := range []string{"address", "raw_mails", "users", "users_address", "user_roles", "sendbox", "settings", "address_sender"} {
		out[t], _ = database.Count(ctx, "SELECT COUNT(*) FROM "+t)
	}
	return out
}
