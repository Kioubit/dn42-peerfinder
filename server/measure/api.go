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

		rc := http.NewResponseController(w)
		lastWrite := time.Now()

		writeFrame := func(frame string) error {
			if err := rc.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return err
			}

			if _, err := io.WriteString(w, frame); err != nil {
				return err
			}

			if err := rc.Flush(); err != nil {
				return err
			}

			// Don't leave a deadline running while the stream is idle
			if err := rc.SetWriteDeadline(time.Time{}); err != nil {
				return err
			}

			lastWrite = time.Now()
			return nil
		}

		send := func(event string, v any) error {
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}

			return writeFrame(fmt.Sprintf("event: %s\ndata: %s\n\n", event, b))
		}

		sendError := func(msg string) error {
			return send("error", map[string]any{"message": msg})
		}

		finish := func() {
			err := ctx.Err()
			switch {
			case err == nil:
				_ = send("done", map[string]any{})
			case errors.Is(err, context.DeadlineExceeded) && r.Context().Err() == nil:
				_ = sendError("request timed out")
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		if err := writeFrame(": connected\n\n"); err != nil {
			if errors.Is(err, http.ErrNotSupported) {
				log.Printf("ERROR: SSE ResponseWriter %T lacks deadline/flush support. Check middleware: %v",
					w, err)
			}
			return
		}

		ip := r.URL.Query().Get("ip")
		if ip == "" {
			_ = sendError("ip query parameter is required")
			return
		}

		addr, err := netip.ParseAddr(ip)
		if err != nil {
			_ = sendError("invalid ip address")
			return
		}

		if isNotPubliclyRoutable(addr, false) {
			_ = sendError("only publicly routable addresses are allowed")
			return
		}

		currentMeasurementCount := s.activeMeasurementCount.Add(1)
		defer func() {
			s.activeMeasurementCount.Add(-1)
		}()
		if int(currentMeasurementCount) > config.Global.MaxOpenRequests {
			_ = sendError("too many simultaneous requests")
			return
		}

		if !limiter.RateLimitOK(session.ASN) {
			_ = sendError("rate limit exceeded: max ping requests for this ASN have been exceeded for this period")
			return
		}

		log.Printf("Running ping query with ip: %s for %s\n", ip, session.ASN)

		rows, err := s.db.QueryContext(ctx, `SELECT uuid, asn, id, endpoint, hmac_key, version FROM peers
                                                  WHERE endpoint != '' AND disabled = 0`)
		if err != nil {
			log.Println("database query error:", err)
			_ = sendError("database error")
			return
		}
		var agents []agentInfo
		for rows.Next() {
			var peer agentInfo
			if err := rows.Scan(&peer.UUID, &peer.ASN, &peer.ID, &peer.Endpoint, &peer.HMACKey, &peer.Version); err != nil {
				log.Println("measurement: streaming scan failed:", err)
				_ = rows.Close()
				_ = sendError("database error")
				return
			}
			agents = append(agents, peer)
		}
		if rows.Err() != nil {
			log.Println("measurement: rows iteration failed:", rows.Err())
			_ = rows.Close()
			_ = sendError("database error")
			return
		}
		_ = rows.Close()

		rand.Shuffle(len(agents), func(i, j int) {
			agents[i], agents[j] = agents[j], agents[i]
		})

		total := len(agents)

		if err = send("start", map[string]any{"total": total}); err != nil {
			return
		}
		if total == 0 {
			finish()
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
		emitResult := func(res pingResult) error {
			if !res.AgentResponded {
				return nil
			}
			if res.ASN != "" && !sentMeta[res.ASN] {
				if m := s.networkMeta(res.ASN); m != nil {
					sentMeta[res.ASN] = true
					if err := send("meta", map[string]map[string]any{res.ASN: m}); err != nil {
						return err
					}
				}
			}
			return send("result", res)
		}

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-ctx.Done():
				finish()
				return
			case <-heartbeat.C:
				if time.Since(lastWrite) < 10*time.Second {
					continue
				}
				if err := writeFrame(": keep-alive\n\n"); err != nil {
					return
				}
			case res, ok := <-resultsCh:
				if !ok {
					finish()
					return
				}
				if err := emitResult(res); err != nil {
					return
				}
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
