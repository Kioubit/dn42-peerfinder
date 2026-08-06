package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"peerfinder-db/config"
	"peerfinder-db/directory"
	"peerfinder-db/kauth"
	"peerfinder-db/measure"
	"peerfinder-db/rateLimiter"
	"strings"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.Llongfile)

	flag.StringVar(&config.Global.MyDomain, "domain", "peerfinder.dn42.dev", "domain name")
	flag.BoolVar(&config.Global.IsDevelopment, "development", false, "development mode")
	flag.StringVar(&config.Global.DataDirectory, "data", "./data/node-directory/servers/", "data directory")
	flag.StringVar(&config.Global.ZipPath, "zip", "./data/archive.zip", "zip path")
	flag.StringVar(&config.Global.MeasurementDir, "measurements", "./data/measurements/", "measurement DB store directory")
	flag.IntVar(&config.Global.MaxOpenRequests, "max-open-requests", 2, "maximum number of concurrently open ping requests")
	flag.IntVar(&config.Global.MaxAgentsPerASN, "max-agents-per-asn", 80, "maximum number of agents an ASN can register")
	flag.Parse()

	nlc := directory.NewNetworkListCache()
	// Warm the network list cache
	go nlc.Get()

	mux := http.NewServeMux()
	// -- Static web handler --
	mux.Handle("GET /", staticContentHandler())

	// -- Directory frontend endpoints --
	mux.HandleFunc("GET /api/directory/list", nlc.GetListHandler)
	mux.HandleFunc("GET /api/directory/countries", nlc.GetCountriesHandler)
	mux.HandleFunc("GET /api/directory/map_data", nlc.GetMapDataHandler)
	mux.HandleFunc("GET /api/directory/network/{asn}", directory.GetASNHandler)
	mux.HandleFunc("GET /api/directory/network/{asn}/servers", directory.GetServersHandler)
	mux.HandleFunc("GET /api/directory/download_script", nlc.DownloadLocalFinderScript)

	// Authenticated
	mux.Handle("POST /api/directory/self", kauth.WithAuth(rateLimiter.WithRateLimiter(nlc.EditHandler, 3*time.Hour, 100)))
	mux.Handle("DELETE /api/directory/self", kauth.WithAuth(rateLimiter.WithRateLimiter(nlc.DeleteHandler, 3*time.Hour, 100)))
	mux.Handle("GET /api/directory/self", kauth.WithAuth(rateLimiter.WithRateLimiter(directory.GetSelfHandler, 3*time.Hour, 200)))

	// -- Ping agent frontend endpoints --
	store, err := measure.NewMeasurementStore(config.Global.MeasurementDir)
	if err != nil {
		log.Fatalln("Failed to initialize measurement store:", err)
	}
	go store.RunAgentHealthChecks()
	go store.StartCleanupRateLimitWorker()

	mux.HandleFunc("GET /api/agents/statistics", store.AgentStatisticsHandler)

	// Authenticated
	mux.Handle("GET /api/agents/ping", kauth.WithAuth(store.PingStreamHandler))
	mux.Handle("GET /api/agents/self", kauth.WithAuth(rateLimiter.WithRateLimiter(store.ListAgentsHandler, 3*time.Hour, 200)))
	mux.Handle("POST /api/agents/register", kauth.WithAuth(rateLimiter.WithRateLimiter(store.RegisterAgentHandler, 3*time.Hour, 150)))
	mux.Handle("DELETE /api/agents/{uuid}", kauth.WithAuth(rateLimiter.WithRateLimiter(store.DeleteAgentHandler, 3*time.Hour, 150)))
	mux.Handle("POST /api/agents/{uuid}/edit", kauth.WithAuth(rateLimiter.WithRateLimiter(store.EditAgentHandler, 3*time.Hour, 100)))
	mux.Handle("GET /api/agents/{uuid}/test", kauth.WithAuth(rateLimiter.WithRateLimiter(store.TestAgentHandler, 2*time.Hour, 50)))

	var mainHandler http.Handler = mux
	if config.Global.IsDevelopment {
		fmt.Println("Development mode active")
		mainHandler = corsMiddleware(mainHandler)
	}

	s1 := &http.Server{
		Addr:              ":8001",
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       1 * time.Minute,
		Handler:           mainHandler,
	}

	go func() {
		if err = s1.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalln(err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s1.Shutdown(shutdownContext)
	_ = store.Close()
}

//go:embed www/*
var www embed.FS

func staticContentHandler() http.Handler {
	fSys := fs.FS(www)
	html, _ := fs.Sub(fSys, "www")
	fileServer := http.FileServer(http.FS(html))

	withETag := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		maxAge := "86400"
		if r.URL.Path == "/" {
			maxAge = "900"
		}
		w.Header().Set("ETag", config.StaticEtag)
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%s must-revalidate", maxAge))

		if match := r.Header.Get("If-None-Match"); strings.TrimPrefix(match, "W/") == config.StaticEtag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	return withETag
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Permissive CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		// Handle preflight requests (OPTIONS method)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}
