package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/guaidao2/lingeyebot/internal/config"
	"github.com/guaidao2/lingeyebot/internal/server"
	"github.com/guaidao2/lingeyebot/internal/skill"
	"github.com/guaidao2/lingeyebot/internal/store"
)

const version = "1.0.3"

func main() {
	var addr string
	flag.StringVar(&addr, "addr", "", "listen address (default from config)")
	flag.StringVar(&addr, "listen", "", "listen address (alias for -addr)")
	flag.StringVar(&addr, "l", "", "listen address (short alias)")
	dataDir := flag.String("data", "", "data directory (default ./data)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
		cfg.Workspace = filepath.Join(*dataDir, "workspace")
		oldSkills := filepath.Join(cfg.Workspace, "skills")
		newSkills := filepath.Join(*dataDir, "skills")
		skillsRoot := newSkills
		if _, err := os.Stat(newSkills); os.IsNotExist(err) {
			if _, oldErr := os.Stat(oldSkills); oldErr == nil {
				if err := os.Rename(oldSkills, newSkills); err != nil {
					skillsRoot = oldSkills
				}
			}
		}
		cfg.Skills = []string{skillsRoot}
	}
	if addr != "" {
		cfg.Listen = addr
	}
	if err := os.MkdirAll(cfg.Workspace, 0o755); err != nil {
		log.Fatalf("workspace: %v", err)
	}
	for _, root := range cfg.Skills {
		_ = os.MkdirAll(root, 0o755)
	}

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	skills := skill.New(cfg.Skills)
	srv, err := server.New(cfg, st, skills, version)
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	log.Printf("config=%s data=%s workspace=%s", config.Path(), cfg.DataDir, cfg.Workspace)
	if _, ok := cfg.Active(); !ok {
		log.Printf("尚未配置 LLM 端点：在 Web 设置里填写 provider / base URL / API Key / model")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
	log.Printf("stopped")
}
