package measure

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"net/netip"
	"regexp"
	"strings"
	"unicode"
)

// randomHexSecret returns a random hex encoded secret with the desired length
func randomHexSecret(length int) string {
	b := make([]byte, length)
	if _, err := cryptorand.Read(b); err != nil {
		log.Fatalln(fmt.Errorf("failed to read random bytes: %w", err))
	}
	return hex.EncodeToString(b)
}

// limitDescription limits the description to at most one line and 50 characters
func limitDescription(s string) string {
	if before, _, found := strings.Cut(s, "\n"); found {
		s = before
	}

	// Hard grapheme cluster limit
	if len(s) > 100 {
		s = s[:100]
	}

	runes := []rune(s)
	if len(runes) > 50 {
		return string(runes[:50]) + "..."
	}
	return string(runes)
}

// jsonFloat extracts an optional float64 field from a decoded JSON map
func jsonFloat(m map[string]any, key string) *float64 {
	v, ok := m[key].(float64)
	if !ok || v == 0 {
		return nil
	}
	return new(v)
}

// rounded rounds a float64 pointer to at most 3 decimal places.
// nil input returns nil.
func rounded(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := math.Round(*v*1000) / 1000
	return &r
}

// boolToInt maps a bool to the integer representation used by SQLite.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isPrivateOrLocalIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsPrivate() || isLocalIP(ip)
}

func isLocalIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() || ip.IsUnspecified()
}

func containsWhitespace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

var validVersionRegexp = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}$`)

func isValidVersionString(s string) bool {
	return validVersionRegexp.MatchString(s)
}
