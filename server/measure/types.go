package measure

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"time"
)

// agentProtocolTimeout bounds how long the backend waits for an agent to
// a connection and reply to a measurement request.
const agentProtocolTimeout = 12 * time.Second

// agentConnectionTimeout bounds how long the backend waits for the initial
// TCP connection to the agent.
const agentConnectionTimeout = 5 * time.Second

// agentUserTestTimeout specifies the time limit for user-initiated agent tests.
const agentUserTestTimeout = 8 * time.Second

// maxAgentConcurrency caps how many agents are contacted in parallel when a
// measurement is dispatched, so a single ping request cannot exhaust resources.
const maxAgentConcurrency = 64

// maxMeasurementDuration caps the total measurement duration for a single ping request.
const maxMeasurementDuration = 4 * time.Minute

// maxHealthCheckDuration caps the total duration of agent health checks.
const maxHealthCheckDuration = 10 * time.Minute

// minHealthyAgents specifies the minimum healthy agents for the health check to be successful.
const minHealthyAgents = 16

// agentHealthCheckInterval is how often the backend probes agents not probed within this duration
// with a "version" command to refresh their last_seen timestamp.
const agentHealthCheckInterval = 24 * time.Hour

// agentMaxDownDuration is how long an agent may go without a successful response
// before it is considered inactive.
const agentMaxDownDuration = 30 * 24 * time.Hour

// maxAgentResponseBytes caps how much data is ever read back from an
// agent connection to avoid denial of service.
const maxAgentResponseBytes = 5 * 1024

// --------------------------------------------------------------------------------------

// pingResult holds the measurement reported by a single agent.
type pingResult struct {
	ASN            string   `json:"asn"`
	Node           string   `json:"node,omitempty"`
	Latency        *float64 `json:"latency,omitempty"`
	Jitter         *float64 `json:"jitter,omitempty"`
	MinRTT         *float64 `json:"min_rtt,omitempty"`
	MaxRTT         *float64 `json:"max_rtt,omitempty"`
	Sent           int      `json:"sent"`
	Recv           int      `json:"recv"`
	Reachable      bool     `json:"reachable"`
	AgentResponded bool     `json:"-"`
}

// agentInfo describes a measurement agent (node). It is both the on-disk record
// persisted to SQLite and the structure mapped for public query responses. It
// carries the HMAC secret (HMACKey).
//
// UUID is an opaque random identifier, not a formatted RFC 4122
type agentInfo struct {
	UUID       string     `json:"uuid"`
	ASN        string     `json:"-"`
	ID         string     `json:"id"` // The user defined node name. Must be unique per ASN.
	Endpoint   string     `json:"endpoint"`
	HMACKey    string     `json:"hmac_key"`
	Version    *string    `json:"version"`
	AddedAt    *time.Time `json:"added_at"`
	LastSeen   *time.Time `json:"last_seen"`
	LastProbed *time.Time `json:"last_probed"`
	// Disabled agents are retained but never sent ping requests.
	Disabled bool `json:"disabled"`
}

// MeasurementStore keeps active agent registrations in an SQLite database.
// It is safe for concurrent use.
type MeasurementStore struct {
	db                     *sql.DB
	activeMeasurementCount atomic.Int32

	statistics agentStatistics
}

type agentStatistics struct {
	Active     int       `json:"active"`
	Registered int       `json:"registered"`
	UpdateTime time.Time `json:"update_time"`
	sync.Mutex
}
