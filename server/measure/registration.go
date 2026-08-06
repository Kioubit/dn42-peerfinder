package measure

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/netip"
	"peerfinder/config"
	"peerfinder/directory/directoryTypes"
	"strconv"
	"time"
	"unicode"

	"github.com/mattn/go-sqlite3"
)

// listAgents returns the agent list for a target ASN.
func (s *MeasurementStore) listAgents(asn string) []agentInfo {
	rows, err := s.db.Query(`
        SELECT uuid, asn, id, endpoint, hmac_key, version, added_at, last_seen, last_probed, disabled 
        FROM peers 
        WHERE asn = ? ORDER BY added_at`, asn)
	if err != nil {
		log.Println("measurement: ListAgents query failed:", err)
		return []agentInfo{}
	}
	defer func() {
		_ = rows.Close()
	}()

	out := make([]agentInfo, 0)
	for rows.Next() {
		var p agentInfo
		var addedAt, lastSeen, lastProbed sql.NullInt64
		var version sql.NullString
		var disabled int
		err := rows.Scan(&p.UUID, &p.ASN, &p.ID, &p.Endpoint, &p.HMACKey, &version, &addedAt, &lastSeen, &lastProbed, &disabled)
		if err != nil {
			log.Println("measurement: ListAgents scan failed:", err)
			continue
		}
		p.Disabled = disabled != 0
		if addedAt.Valid {
			p.AddedAt = new(time.Unix(addedAt.Int64, 0))
		}
		if lastSeen.Valid {
			p.LastSeen = new(time.Unix(lastSeen.Int64, 0))
		}
		if lastProbed.Valid {
			p.LastProbed = new(time.Unix(lastProbed.Int64, 0))
		}
		if version.Valid {
			p.Version = &version.String
		}
		out = append(out, p)
	}
	return out
}

// registerAgent registers a new agent with a server-generated UUID.
func (s *MeasurementStore) registerAgent(ctx context.Context, asn string, name directoryTypes.YamlServerID, endpoint string) (*agentInfo, error) {
	var errInternal = errors.New("internal server error")

	if err := name.Validate(); err != nil {
		return nil, fmt.Errorf("invalid agent name: %w", err)
	}

	if err := validateAgentEndpoint(endpoint); err != nil {
		return nil, fmt.Errorf("invalid agent endpoint: %w", err)
	}

	limit := config.Global.MaxAgentsPerASN
	if limit <= 0 {
		limit = math.MaxInt64
	}

	addedAt := time.Now().UTC()
	agent := &agentInfo{
		ASN:      asn,
		ID:       string(name),
		Endpoint: endpoint,
		HMACKey:  randomHexSecret(32),
		AddedAt:  &addedAt,
	}

	const maxUUIDRetries = 3
	for attempt := 1; attempt <= maxUUIDRetries; attempt++ {
		agent.UUID = randomHexSecret(32)

		res, err := s.db.ExecContext(ctx, `
        INSERT INTO peers (uuid, asn, id, endpoint, hmac_key, added_at)
        SELECT ?, ?, ?, ?, ?, ?
        WHERE (SELECT COUNT(*) FROM peers WHERE asn = ?) < ?
        ON CONFLICT(asn, id) DO NOTHING`,
			agent.UUID, agent.ASN, agent.ID, agent.Endpoint, agent.HMACKey,
			addedAt.Unix(), agent.ASN, limit,
		)
		if err != nil {
			if sqliteErr, ok := errors.AsType[sqlite3.Error](err); ok {
				// The (asn, id) conflict is suppressed by the upsert clause, so the
				// only unique violation that can surface here is the uuid PK.
				if errors.Is(sqliteErr.ExtendedCode, sqlite3.ErrConstraintUnique) {
					continue
				}
			}
			log.Printf("registerAgent: insert failed: %v", err)
			return nil, errInternal
		}

		n, err := res.RowsAffected()
		if err != nil {
			log.Printf("registerAgent: RowsAffected failed: %v", err)
			return nil, errInternal
		}
		if n == 1 {
			return agent, nil
		}

		// Nothing inserted, and not an uuid conflict: either the name is taken
		// or the per-ASN limit was hit. One diagnostic query disambiguates.
		var nameExists bool
		err = s.db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM peers WHERE asn = ? AND id = ?)",
			agent.ASN, agent.ID,
		).Scan(&nameExists)
		if err != nil {
			log.Printf("registerAgent: diagnostic query failed: %v", err)
			return nil, errInternal
		}
		if nameExists {
			return nil, fmt.Errorf("node named %q is already registered for %s", name, asn)
		}
		return nil, fmt.Errorf("ASN %s already has the maximum of %d registered agents", asn, config.Global.MaxAgentsPerASN)
	}

	log.Println("registerAgent: failed to insert agent:", errInternal)
	return nil, errInternal
}

// deleteAgent removes an agent matching the provided ASN and UUID.
func (s *MeasurementStore) deleteAgent(asn, uuid string) error {
	res, err := s.db.Exec("DELETE FROM peers WHERE uuid = ? AND asn = ?", uuid, asn)
	if err != nil {
		log.Println(fmt.Errorf("failed deleting agent: %w", err))
		return errors.New("internal server error")
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Println(fmt.Errorf("database transaction error: %w", err))
		return errors.New("internal server error")
	}
	if rows == 0 {
		return fmt.Errorf("agent not found")
	}
	return nil
}

// editAgent renames an existing agent while asserting ID index constraints.
func (s *MeasurementStore) editAgent(asn, uuid string, name directoryTypes.YamlServerID, endpoint string, disabled bool) error {
	if err := name.Validate(); err != nil {
		return fmt.Errorf("invalid agent name: %w", err)
	}

	if err := validateAgentEndpoint(endpoint); err != nil {
		return fmt.Errorf("invalid agent endpoint: %w", err)
	}

	var nameExists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM peers WHERE asn = ? AND id = ? AND uuid != ?)", asn, name, uuid).Scan(&nameExists)
	if err != nil {
		log.Println(fmt.Errorf("database query error: %w", err))
		return errors.New("internal server error")
	}
	if nameExists {
		return fmt.Errorf("a node named %q is already registered for %s", name, asn)
	}

	res, err := s.db.Exec("UPDATE peers SET id = ?, endpoint = ?, added_at = ?, disabled = ? WHERE uuid = ? AND asn = ?", string(name), endpoint, time.Now().Unix(), boolToInt(disabled), uuid, asn)
	if err != nil {
		log.Println(fmt.Errorf("failed to edit agent: %w", err))
		return errors.New("internal server error")
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Println(fmt.Errorf("database transaction error: %w", err))
		return errors.New("internal server error")
	}
	if rows == 0 {
		return fmt.Errorf("agent not found")
	}
	return nil
}

func validateAgentEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("missing")
	}

	if containsWhitespace(endpoint) {
		return fmt.Errorf("must not contain any whitespace")
	}

	if len(endpoint) > 100 {
		return fmt.Errorf("too long")
	}

	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("must be in host:port form")
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port")
	}
	if portInt < 10 || portInt > math.MaxUint16 {
		return fmt.Errorf("invalid port number")
	}

	if host == "" {
		return fmt.Errorf("missing host")
	}

	// is IP
	if ip, err := netip.ParseAddr(host); err == nil {
		if isLocalIP(ip) {
			return fmt.Errorf("must be a routable address")
		}
		return nil
	}

	for _, c := range host {
		alnum := 'a' <= c && c <= 'z' || '0' <= c && c <= '9'
		if !alnum && c != '-' && c != '.' {
			return fmt.Errorf("invalid character")
		}
	}

	return nil
}

func containsWhitespace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// agentStatistics returns registration statistics
func (s *MeasurementStore) agentStatistics() ([]byte, error) {
	s.statistics.Lock()
	defer s.statistics.Unlock()

	if time.Since(s.statistics.UpdateTime) > 12*time.Hour {
		var active, registered int
		row := s.db.QueryRow("SELECT count(*) FROM peers WHERE disabled = 0 AND last_seen >= last_probed")
		if err := row.Scan(&active); err != nil {
			return nil, err
		}

		row = s.db.QueryRow("SELECT count(*) FROM peers WHERE disabled = 0")
		if err := row.Scan(&registered); err != nil {
			return nil, err
		}

		s.statistics.Active = active
		s.statistics.Registered = registered
		s.statistics.UpdateTime = time.Now().Truncate(time.Second)
	}

	result, err := json.Marshal(&s.statistics)
	if err != nil {
		return nil, err
	}
	return result, nil
}
