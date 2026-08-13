package measure

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"peerfinder/config"
	"peerfinder/directory/directoryTypes"
	"peerfinder/kauth"
	"peerfinder/rateLimiter"
	"sync"
	"time"
)

// ListAgentsHandler implements GET /api/agents returning the authenticated
// ASN's registered agents.
func (s *MeasurementStore) ListAgentsHandler(w http.ResponseWriter, _ *http.Request, session *kauth.AuthenticationInfo) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	if err := json.NewEncoder(w).Encode(s.listAgents(session.ASN)); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
}

// RegisterAgentHandler creates a new agent for the authenticated ASN.
// It returns both the server-generated "hmac_key" and the "endpoint" once to the client.
func (s *MeasurementStore) RegisterAgentHandler(w http.ResponseWriter, r *http.Request, session *kauth.AuthenticationInfo) {
	var body struct {
		Name     string `json:"name"`
		Endpoint string `json:"endpoint"`
	}
	lr := http.MaxBytesReader(w, r.Body, 10000)
	defer func() { _ = lr.Close() }()

	if err := json.NewDecoder(lr).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	peer, err := s.registerAgent(context.Background(), session.ASN, directoryTypes.YamlServerID(body.Name), body.Endpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"endpoint": peer.Endpoint,
		"hmac_key": peer.HMACKey,
	})
}

// AgentStatisticsHandler implements GET /api/agents/statistics
func (s *MeasurementStore) AgentStatisticsHandler(w http.ResponseWriter, _ *http.Request) {
	statistics, err := s.agentStatistics()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=21600, must-revalidate")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(statistics)
}

// DeleteAgentHandler implements DELETE /api/agents/{uuid}
func (s *MeasurementStore) DeleteAgentHandler(w http.ResponseWriter, r *http.Request, session *kauth.AuthenticationInfo) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		http.Error(w, "missing agent id", http.StatusBadRequest)
		return
	}
	if err := s.deleteAgent(session.ASN, uuid); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// EditAgentHandler implements POST /api/agents/{uuid}/edit
func (s *MeasurementStore) EditAgentHandler(w http.ResponseWriter, r *http.Request, session *kauth.AuthenticationInfo) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		http.Error(w, "missing agent id", http.StatusBadRequest)
		return
	}
	var body struct {
		Name     string `json:"name"`
		Endpoint string `json:"endpoint"`
		Disabled bool   `json:"disabled"`
	}
	lr := http.MaxBytesReader(w, r.Body, 10000)
	defer func() { _ = lr.Close() }()
	if err := json.NewDecoder(lr).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.editAgent(session.ASN, uuid, directoryTypes.YamlServerID(body.Name), body.Endpoint, body.Disabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// PingStreamHandler streams results from database peers over Server-Sent Events.
func (s *MeasurementStore) PingStreamHandler() func(http.ResponseWriter, *http.Request, *kauth.AuthenticationInfo) {
	limiter := rateLimiter.NewRateLimiter[string](2*time.Hour, 1000, 10)
	return func(w http.ResponseWriter, r *http.Request, session *kauth.AuthenticationInfo) {
		ctx, cancel := context.WithTimeout(r.Context(), maxMeasurementDuration)
		defer cancel()

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		rc := http.NewResponseController(w)
		send := func(event string, v any) {
			b, err := json.Marshal(v)
			if err != nil {
				return
			}
			err = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err != nil {
				cancel()
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
				// client gone or too slow
				cancel()
				return
			}
			flusher.Flush()
		}

		sendError := func(msg string) {
			send("error", map[string]any{"message": msg})
		}

		ip := r.URL.Query().Get("ip")
		if ip == "" {
			sendError("ip query parameter is required")
			return
		}

		addr, err := netip.ParseAddr(ip)
		if err != nil {
			sendError("invalid ip address")
			return
		}

		if isNotPubliclyRoutable(addr, false) {
			sendError("only publicly routable addresses are allowed")
			return
		}

		currentMeasurementCount := s.activeMeasurementCount.Add(1)
		defer func() {
			s.activeMeasurementCount.Add(-1)
		}()
		if int(currentMeasurementCount) > config.Global.MaxOpenRequests {
			sendError("too many simultaneous requests")
			return
		}

		if !limiter.RateLimitOK(session.ASN) {
			sendError("rate limit exceeded: max ping requests for this ASN have been exceeded for this period")
			return
		}

		log.Printf("Running ping query with ip: %s for %s\n", ip, session.ASN)

		rows, err := s.db.QueryContext(ctx, "SELECT uuid, asn, id, endpoint, hmac_key, version FROM peers WHERE endpoint != '' AND disabled = 0")
		if err != nil {
			log.Println("database query error:", err)
			sendError("database error")
			return
		}
		var agents []agentInfo
		for rows.Next() {
			var peer agentInfo
			if err := rows.Scan(&peer.UUID, &peer.ASN, &peer.ID, &peer.Endpoint, &peer.HMACKey, &peer.Version); err != nil {
				log.Println("measurement: streaming scan failed:", err)
				break
			}
			agents = append(agents, peer)
		}
		if rows.Err() != nil {
			log.Println("measurement: rows iteration failed:", rows.Err())
			sendError("database error")
			_ = rows.Close()
			return
		}
		_ = rows.Close()

		rand.Shuffle(len(agents), func(i, j int) {
			agents[i], agents[j] = agents[j], agents[i]
		})

		total := len(agents)

		send("start", map[string]any{"total": total})
		if total == 0 {
			send("done", map[string]any{})
			return
		}

		resultsCh := make(chan pingResult, 100)

		go func() {
			var wg sync.WaitGroup
			defer func() {
				wg.Wait()
				close(resultsCh)
			}()
			sem := make(chan struct{}, maxAgentConcurrency)

			for _, a := range agents {
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				wg.Add(1)
				go func(a agentInfo) {
					defer wg.Done()
					defer func() { <-sem }()
					result := s.dispatchToAgent(ctx, a, addr)
					select {
					case resultsCh <- result:
					case <-ctx.Done():
					}
				}(a)
			}
		}()

		sentMeta := make(map[string]bool)
		emitResult := func(res pingResult) {
			if !res.AgentResponded {
				return
			}
			if res.ASN != "" && !sentMeta[res.ASN] {
				if m := s.networkMeta(res.ASN); m != nil {
					sentMeta[res.ASN] = true
					send("meta", map[string]map[string]any{res.ASN: m})
				}
			}
			send("result", res)
		}

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()

		lastWrite := time.Now()

		for {
			select {
			case <-ctx.Done():
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					sendError("request timed out")
				} else {
					sendError("request error")
				}
				return
			case <-heartbeat.C:
				if time.Since(lastWrite) < 10*time.Second {
					continue
				}
				_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case res, ok := <-resultsCh:
				if !ok {
					send("done", map[string]any{})
					return
				}
				emitResult(res)
				lastWrite = time.Now()
			}
		}
	}
}

func (s *MeasurementStore) TestAgentHandler(w http.ResponseWriter, r *http.Request, session *kauth.AuthenticationInfo) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		http.Error(w, "uuid query parameter is required", http.StatusBadRequest)
		return
	}
	row := s.db.QueryRow(`SELECT uuid, asn, id, endpoint, hmac_key, version FROM peers
                                         WHERE endpoint != '' AND uuid = ? AND asn = ?`, uuid, session.ASN)

	var peer agentInfo
	if err := row.Scan(&peer.UUID, &peer.ASN, &peer.ID, &peer.Endpoint, &peer.HMACKey, &peer.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		log.Println("measurement: error scanning agent:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), agentUserTestTimeout)
	defer cancel()

	ok, err := s.updateAgentMeta(ctx, peer)
	if err != nil {
		log.Println("measurement: updateAgentVersion error", err)
	}
	if !ok {
		http.Error(w, "agent test failed", http.StatusInternalServerError)
		return
	}
}
