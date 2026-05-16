package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"jproxy/core-proxy/internal/app"
	"jproxy/core-proxy/internal/repository"
	"jproxy/core-proxy/internal/system"
)

func main() {
	dbPath := getenv("JPROXY_DB_PATH", "../database/jproxy.db")
	addr := getenv("CORE_PROXY_ADDR", ":8117")
	minCount := getenv("CORE_PROXY_MIN_COUNT", "6")
	webDistPath := getenv("WEB_DIST_PATH", "../web-dist")

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck(addr)
		return
	}

	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		log.Fatalf("init repository: %v", err)
	}
	defer func() {
		if err := repo.Close(); err != nil {
			log.Printf("close repository: %v", err)
		}
	}()

	if err := repo.EnsureSchema(context.Background()); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	service, err := app.NewService(repo, minCount)
	if err != nil {
		log.Fatalf("init service: %v", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	configService := system.NewConfigService(repo, client)
	syncService := system.NewSyncService(repo, client)
	scheduler := system.NewScheduler(syncService)
	defer scheduler.Stop()
	scheduler.Start()
	uiService := system.NewUIService(webDistPath)
	adminService := system.NewAdminService(repo, service, syncService)

	mux := http.NewServeMux()
	system.NewHTTPServer(service, configService, syncService).RegisterRoutes(mux)
	adminService.RegisterRoutes(mux)
	uiService.RegisterRoutes(mux)

	server := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("core-proxy listening on %s with db %s", addr, dbPath)
	log.Fatal(server.ListenAndServe())
}

func healthcheck(addr string) {
	url := "http://" + loopbackAddr(addr) + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("healthcheck failed: %v", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("healthcheck failed: %s", resp.Status)
		os.Exit(1)
	}
}

func loopbackAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	parts := strings.Split(addr, ":")
	if len(parts) > 1 {
		port := parts[len(parts)-1]
		if _, err := strconv.Atoi(port); err == nil {
			return "127.0.0.1:" + port
		}
	}
	return addr
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.String(), time.Since(start))
	})
}
