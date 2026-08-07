package measure

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// sendAgentRequest opens a TCP connection to an agent, sends an HMAC-signed
// request, and returns the verified JSON response. The agent's shared
// secret (HMACKey) authenticates both directions.
// Returned: (Unmarshalled JSON payload, success boolean)
func sendAgentRequest(ctx context.Context, peer agentInfo, payload map[string]any) (map[string]any, bool) {
	if peer.Endpoint == "" || peer.HMACKey == "" {
		return nil, false
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}

	if len(body) > 65535 {
		return nil, false
	}

	reqTs := time.Now().Unix()
	tsBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBuf, uint64(reqTs))

	ncBuf := make([]byte, 32)
	if _, err = cryptorand.Read(ncBuf); err != nil {
		return nil, false
	}

	key, err := hex.DecodeString(peer.HMACKey)
	if err != nil {
		return nil, false
	}

	if len(key) != sha256.Size {
		return nil, false
	}

	mac := hmac.New(sha256.New, key)

	var requestMarker = [1]byte{0x00}
	if peer.Version != nil {
		// New MAC format
		if cmp, err := compareVersions(*peer.Version, "1.2.0"); err == nil && cmp >= 0 {
			mac.Write(requestMarker[:])
		}
	}
	mac.Write(tsBuf)
	mac.Write(ncBuf)
	mac.Write(body)
	sig := mac.Sum(nil)

	lengthBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBuf, uint16(len(body)))

	dialer := &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			if ip, err := netip.ParseAddr(host); err == nil && isLocalIP(ip) {
				return fmt.Errorf("connections to local addresses are not allowed")
			}
			return nil
		},
	}

	dialCtx, cancel := context.WithTimeout(ctx, agentConnectionTimeout)
	defer cancel()

	conn, err := dialer.DialContext(dialCtx, "tcp", peer.Endpoint)
	if err != nil {
		return nil, false
	}
	defer func() {
		_ = conn.Close()
	}()

	// Unblock in-flight Read/Write immediately on cancel or overall deadline.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if err = conn.SetDeadline(time.Now().Add(agentProtocolTimeout)); err != nil {
		log.Println("failed to set connection deadline", err)
		return nil, false
	}

	wr := bufio.NewWriter(conn)

	if _, err := wr.Write(sig); err != nil {
		return nil, false
	}
	if _, err := wr.Write(tsBuf); err != nil {
		return nil, false
	}
	if _, err := wr.Write(ncBuf); err != nil {
		return nil, false
	}
	if _, err := wr.Write(lengthBuf); err != nil {
		return nil, false
	}
	if _, err := wr.Write(body); err != nil {
		return nil, false
	}

	if err := wr.Flush(); err != nil {
		return nil, false
	}

	limitReader := io.LimitReader(conn, maxAgentResponseBytes)
	rr := bufio.NewReader(limitReader)

	respSig := make([]byte, 32)
	if _, err := io.ReadFull(rr, respSig); err != nil {
		return nil, false
	}

	respTsBuf := make([]byte, 8)
	if _, err := io.ReadFull(rr, respTsBuf); err != nil {
		return nil, false
	}
	respTs := int64(binary.BigEndian.Uint64(respTsBuf))

	respNcBuf := make([]byte, 32)
	if _, err := io.ReadFull(rr, respNcBuf); err != nil {
		return nil, false
	}

	respLengthBuf := make([]byte, 2)
	if _, err := io.ReadFull(rr, respLengthBuf); err != nil {
		return nil, false
	}
	respPayloadLen := binary.BigEndian.Uint16(respLengthBuf)

	respPayload := make([]byte, respPayloadLen)
	if _, err := io.ReadFull(rr, respPayload); err != nil {
		return nil, false
	}

	usesLegacyResponseMac := false
	var responseMarker = [1]byte{0x01}

	mac.Reset()
	mac.Write(responseMarker[:])
	mac.Write(respTsBuf)
	mac.Write(respNcBuf)
	mac.Write(respPayload)
	if !hmac.Equal(respSig, mac.Sum(nil)) {
		// --- LEGACY (agent version < v1.1.0) ---
		mac.Reset()
		mac.Write(respTsBuf)
		mac.Write(respNcBuf)
		mac.Write(respPayload)
		if !hmac.Equal(respSig, mac.Sum(nil)) {
			return nil, false
		}
		usesLegacyResponseMac = true
		// --------------------------------------
	}

	if respTs != reqTs {
		return nil, false
	}
	if !bytes.Equal(respNcBuf, ncBuf) {
		return nil, false
	}

	var resp map[string]any
	if err := json.Unmarshal(respPayload, &resp); err != nil {
		return nil, false
	}

	if usesLegacyResponseMac {
		versionStr, ok := resp["version"].(string)
		if !ok {
			return nil, false
		}

		// Disallow legacy MAC for versions >= 1.1.0
		if cmp, err := compareVersions(versionStr, "1.1.0"); err != nil || cmp >= 0 {
			return nil, false
		}

		// Disallow "command" field for versions >= 1.0.4
		if cmp, err := compareVersions(versionStr, "1.0.4"); err != nil {
			return nil, false
		} else if cmp >= 0 {
			if _, exists := resp["command"]; exists {
				return nil, false
			}
		}
	}

	return resp, true
}

// compareVersions compares two dot-separated numeric version strings.
// It returns:
//
//	-1 if v1 <  v2
//	 0 if v1 == v2
//	 1 if v1 >  v2
func compareVersions(v1, v2 string) (int, error) {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := max(len(parts2), len(parts1))

	for i := range maxLen {
		var n1, n2 int
		var err error

		if i < len(parts1) {
			n1, err = strconv.Atoi(parts1[i])
			if err != nil {
				return 0, fmt.Errorf("invalid version segment %q in %q", parts1[i], v1)
			}
		}
		if i < len(parts2) {
			n2, err = strconv.Atoi(parts2[i])
			if err != nil {
				return 0, fmt.Errorf("invalid version segment %q in %q", parts2[i], v2)
			}
		}

		if n1 < n2 {
			return -1, nil
		}
		if n1 > n2 {
			return 1, nil
		}
	}

	return 0, nil
}
