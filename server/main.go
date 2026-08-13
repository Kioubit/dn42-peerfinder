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
	"peerfinder/config"
	"peerfinder/directory"
	"peerfinder/kauth"
	"peerfinder/measure"
	"peerfinder/rateLimiter"
	"strings"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.Llongfile)

	flag.StringVar(&config.Global.MyDomain, "domain", "peerfinder.dn42.dev", "domain name")
	flag.StringVar(&config.Global.ServersDirectory, "servers-dir", "../data/node-directory/servers/",
		"path to the servers directory of the node-directory containing yml files")
	flag.StringVar(&config.Global.LocalFinderZipPath, "local-finder-zip-path", "../data/archive.zip",
		"path to the zip file containing the local finder script")
	flag.StringVar(&config.Global.MeasurementDir, "measurement-dir", "../data/measurements/",
		"measurement DB store directory")
	flag.IntVar(&config.Global.MaxOpenRequests, "max-open-requests", 2,
		"maximum number of concurrently open ping requests")
	flag.IntVar(&config.Global.MaxAgentsPerASN, "max-agents-per-asn", 80,
		"maximum number of agents an ASN can register")
	flag.Parse()

	for _, p := range []string{config.Global.ServersDirectory, config.Global.LocalFinderZipPath, config.Global.MeasurementDir} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			log.Fatalf("path does not exist: %s", p)
		}
	}

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

	mux.HandleFunc("GET /api/agents/statistics", store.AgentStatisticsHandler)

	// Authenticated
	mux.Handle("GET /api/agents/ping", kauth.WithAuth(store.PingStreamHandler()))
	mux.Handle("GET /api/agents/self", kauth.WithAuth(rateLimiter.WithRateLimiter(store.ListAgentsHandler, 3*time.Hour, 200)))
	mux.Handle("POST /api/agents/register", kauth.WithAuth(rateLimiter.WithRateLimiter(store.RegisterAgentHandler, 3*time.Hour, 150)))
	mux.Handle("DELETE /api/agents/{uuid}", kauth.WithAuth(rateLimiter.WithRateLimiter(store.DeleteAgentHandler, 3*time.Hour, 150)))
	mux.Handle("POST /api/agents/{uuid}/edit", kauth.WithAuth(rateLimiter.WithRateLimiter(store.EditAgentHandler, 3*time.Hour, 100)))
	mux.Handle("GET /api/agents/{uuid}/test", kauth.WithAuth(rateLimiter.WithRateLimiter(store.TestAgentHandler, 2*time.Hour, 50)))

	s1 := &http.Server{
		Addr:              ":8001",
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       1 * time.Minute,
		Handler:           mux,
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
