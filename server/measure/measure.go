package measure

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"math/rand/v2"
	"net/netip"
	"os"
	"path/filepath"
	"peerfinder/config"
	"peerfinder/directory"
	"peerfinder/directory/directoryTypes"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var dbSchema string

func NewMeasurementStore(dir string) (*MeasurementStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("measurement directory must be set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create measurement dir: %w", err)
	}

	dbPath := filepath.Join(dir, "peers.db")
	// If the database does not exist, create it with 0600 permissions
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		if err := os.WriteFile(dbPath, nil, 0o600); err != nil {
			return nil, fmt.Errorf("failed to create db file: %w", err)
		}
	}

	// Open connection using WAL mode and a robust busy_timeout
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if _, err := db.Exec(dbSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create table schema: %w", err)
	}

	s := &MeasurementStore{
		db:          db,
		pingHistory: make(map[string]pingRate),
	}
	return s, nil
}

// Close releases connection pool assets occupied by SQLite.
func (s *MeasurementStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// allowPing records and enforces the per-ASN ping-request rate limit (default 10 per hour).
func (s *MeasurementStore) allowPing(asn string) bool {
	s.pingHistoryMu.Lock()
	defer s.pingHistoryMu.Unlock()

	now := time.Now().UTC()
	const (
		maxTokens  = 10.0
		refillRate = 10.0 / 3600.0 // 10 tokens per 3600 seconds (1 hour)
	)

	lim, exists := s.pingHistory[asn]
	if !exists {
		// Allow request and initialize bucket with maxTokens minus this 1 request.
		s.pingHistory[asn] = pingRate{
			tokens: maxTokens - 1.0,
			last:   now,
		}
		return true
	}

	// Refill tokens based on physical duration elapsed
	elapsed := now.Sub(lim.last).Seconds()
	newTokens := lim.tokens + (elapsed * refillRate)
	if newTokens > maxTokens {
		newTokens = maxTokens
	}

	if newTokens < 1.0 {
		// Rate limit hit: persist the refilled state but deny access
		lim.tokens = newTokens
		lim.last = now
		s.pingHistory[asn] = lim
		return false
	}

	// Consume 1 token and save state
	lim.tokens = newTokens - 1.0
	lim.last = now
	s.pingHistory[asn] = lim
	return true
}

func (s *MeasurementStore) StartCleanupRateLimitWorker() {
	cleanup := func() {
		s.pingHistoryMu.Lock()
		defer s.pingHistoryMu.Unlock()

		now := time.Now().UTC()
		for asn, lim := range s.pingHistory {
			// If the entry has been idle for more than an hour, it's fully charged.
			// We can safely drop it. If they request again, they start at max tokens.
			if now.Sub(lim.last) >= time.Hour {
				delete(s.pingHistory, asn)
			}
		}
	}

	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	for {
		<-t.C
		cleanup()
	}
}

// markSeen refreshes last_seen, last_probed and optionally the client version
func (s *MeasurementStore) markSeen(peerID, version string) error {
	var err error
	if len(version) > 11 || !regexp.MustCompile(`^[0-9.]+$`).MatchString(version) {
		version = ""
	}
	now := time.Now().Unix()
	if version != "" {
		_, err = s.db.Exec("UPDATE peers SET last_seen = ?, last_probed = ?, version = ? WHERE uuid = ?",
			now, now, version, peerID)
	} else {
		_, err = s.db.Exec("UPDATE peers SET last_seen = ?, last_probed = ? WHERE uuid = ?", now, now, peerID)
	}
	if err != nil {
		return err
	}
	return nil
}

// markProbed refreshes last_probed. It should be used to update the DB on failure
func (s *MeasurementStore) markProbed(peerID string) error {
	now := time.Now().Unix()
	if _, err := s.db.Exec("UPDATE peers SET last_probed = ? WHERE uuid = ?", now, peerID); err != nil {
		return err
	}
	return nil
}

// dispatchToAgent pushes a "ping" measurement to an agent and refreshes seen status and version on success
func (s *MeasurementStore) dispatchToAgent(ctx context.Context, peer agentInfo, ip netip.Addr) pingResult {
	res := pingResult{}

	resp, ok := sendAgentRequest(ctx, peer, map[string]any{
		"command": "ping",
		"ip":      ip.String(),
	})
	if !ok {
		res.Reachable = false
		res.AgentResponded = false
		// Don't mark as probed if the failure was due to context cancellation
		if ctx.Err() == nil {
			if err := s.markProbed(peer.UUID); err != nil {
				log.Println("failed to mark probed:", err)
			}
		}
		return res
	}

	res.ASN = peer.ASN
	res.Node = peer.ID

	if reachable, ok := resp["reachable"].(bool); ok {
		res.Reachable = reachable
	}
	if v, ok := resp["sent"].(float64); ok {
		res.Sent = int(v)
	}
	if v, ok := resp["recv"].(float64); ok {
		res.Recv = int(v)
	}
	res.Latency = rounded(jsonFloat(resp, "latency"))
	res.Jitter = rounded(jsonFloat(resp, "jitter"))
	res.MinRTT = rounded(jsonFloat(resp, "min_rtt"))
	res.MaxRTT = rounded(jsonFloat(resp, "max_rtt"))
	res.Reachable = res.Reachable && res.Recv > 0 && res.Latency != nil
	res.AgentResponded = true

	version := ""
	if v, ok := resp["version"].(string); ok {
		version = v
	}
	if err := s.markSeen(peer.UUID, version); err != nil {
		log.Println("dispatchToAgent: markSeen update failed:", err)
	}
	return res
}

// updateAgentMeta sends a "version" command to an agent to confirm it is alive.
// It updates its version and last seen values and returns true on success.
func (s *MeasurementStore) updateAgentMeta(ctx context.Context, peer agentInfo) (bool, error) {
	resp, ok := sendAgentRequest(ctx, peer, map[string]any{
		"command": "version",
	})
	if !ok {
		// Don't mark as probed if the failure was due to context cancellation
		if ctx.Err() == nil {
			if err := s.markProbed(peer.UUID); err != nil {
				log.Println("failed to mark probed:", err)
			}
		}
		return false, nil
	}
	version, _ := resp["version"].(string)
	if err := s.markSeen(peer.UUID, version); err != nil {
		return true, err
	}
	return true, nil
}

// RunAgentHealthChecks periodically probes agents and disables any down longer than agentMaxDownDuration.
func (s *MeasurementStore) RunAgentHealthChecks() {
	time.Sleep(10 * time.Minute)
	ticker := time.NewTicker(agentHealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("agent health check started")
		if !s.agentHealthCheck() {
			log.Println("measurement: agent health check failed")
			continue
		}
		s.disableInactiveAgents()
		log.Println("agent health check completed")
	}
}

// agentHealthCheck probes all agents with a last_probed value smaller
// than the current time minus the agentHealthCheckInterval
func (s *MeasurementStore) agentHealthCheck() bool {
	ctx, cancel := context.WithTimeout(context.Background(), maxHealthCheckDuration)
	defer cancel()

	cutoff := time.Now().Add(-agentHealthCheckInterval).Unix()
	rows, err := s.db.Query(`SELECT uuid, asn, id, endpoint, hmac_key
		FROM peers WHERE endpoint != '' AND (last_probed < ? OR last_probed IS NULL) AND disabled = 0`, cutoff)
	if err != nil {
		log.Println("measurement: health check select failed:", err)
		return false
	}

	var agents []agentInfo
	for rows.Next() {
		var peer agentInfo
		if err := rows.Scan(&peer.UUID, &peer.ASN, &peer.ID, &peer.Endpoint, &peer.HMACKey); err != nil {
			log.Println("measurement: health check scan failed:", err)
			continue
		}
		agents = append(agents, peer)
	}

	if err := rows.Err(); err != nil {
		log.Println("measurement: health check rows failed:", err)
		_ = rows.Close()
		return false
	}

	_ = rows.Close()

	rand.Shuffle(len(agents), func(i, j int) {
		agents[i], agents[j] = agents[j], agents[i]
	})

	var anyError atomic.Bool

	sem := make(chan struct{}, maxAgentConcurrency)
	var wg sync.WaitGroup
	for _, peer := range agents {
		sem <- struct{}{}
		wg.Add(1)
		go func(peer agentInfo) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := s.updateAgentMeta(ctx, peer); err != nil {
				log.Println("measurement: health check error for agent", peer.UUID, err)
				anyError.Store(true)
				return
			}
		}(peer)
	}
	wg.Wait()

	if anyError.Load() {
		return false
	}

	var recent int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM peers
                WHERE endpoint != '' AND disabled = 0 AND last_seen >= ?`, cutoff).Scan(&recent)
	if err != nil {
		log.Println("measurement: health check recent-agent count failed:", err)
		return false
	}

	return recent >= minHealthyAgents
}

// disableInactiveAgents disables agents with a last_seen and added_at value earlier than the agentMaxDownDuration
// It also confirms last_probed is more recent than agentMaxDownDuration
func (s *MeasurementStore) disableInactiveAgents() {
	cutoff := time.Now().Add(-agentMaxDownDuration).Unix()
	res, err := s.db.Exec(`UPDATE peers SET disabled = 1
             WHERE disabled = 0 AND COALESCE(last_seen, added_at) < ? AND last_probed IS NOT NULL AND last_probed >= ?`,
		cutoff, cutoff)
	if err != nil {
		log.Println("deleteInactiveAgents: db update failed:", err)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Println("deleteInactiveAgents: deleteInactiveAgents cannot obtain rows affected", err)
		return
	}
	if rows > 0 {
		log.Printf("deleteInactiveAgents: disabled %d inactive agent(s)\n", rows)
	}
}

func (s *MeasurementStore) networkMeta(asn string) map[string]any {
	if asn == "" {
		return nil
	}
	c, _, err := directory.ReadYAMLFile(config.Global.DataDirectory, asn+".yml")
	if err != nil {
		return nil
	}

	type serverInfo struct {
		CountryCode string
		City        string
	}
	servers := make(map[directoryTypes.YamlServerID]serverInfo)
	for _, server := range c.Servers {
		servers[server.ID] = serverInfo{
			CountryCode: server.CountryCode,
			City:        server.City,
		}
	}

	return map[string]any{
		"network":     c.Name,
		"description": limitDescription(c.Description),
		"servers":     servers,
	}
}
