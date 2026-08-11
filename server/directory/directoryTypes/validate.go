package directoryTypes

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/biter777/countries"
)

func (n *YamlNetwork) Validate() error {
	// Name
	if n.Name == "" {
		return errors.New("network name is empty")
	}
	if len(n.Name) > 64 {
		return errors.New("network name is too long")
	}
	if !isPrintable(n.Name) {
		return errors.New("network name contains non-printable characters")
	}

	// Description
	if n.Description != "" {
		if len(n.Description) > 300 {
			return errors.New("description is too long")
		}
		if !isPrintable(n.Description) {
			return errors.New("description contains non-printable characters")
		}
	}

	// URL
	if n.URL != "" {
		if len(n.URL) > 128 {
			return errors.New("URL is too long")
		}
		if !isPrintableASCII(n.URL) {
			return errors.New("URL contains non-printable ASCII characters")
		}
		if containsWhitespace(n.URL) {
			return errors.New("URL contains whitespace characters")
		}

		parsedURL, err := url.Parse(n.URL)
		if err != nil {
			return errors.New("URL is invalid")
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return errors.New("URL is invalid")
		}
	}

	// Servers
	if err := n.Servers.validate(); err != nil {
		return fmt.Errorf("servers list is invalid: %w", err)
	}

	// Tags
	if err := n.Tags.validate(); err != nil {
		return fmt.Errorf("tags list is invalid: %w", err)
	}
	return nil
}

func (s *YamlServers) validate() error {
	if len(*s) == 0 {
		return errors.New("list is empty")
	}

	if len(*s) > 100 {
		return errors.New("too many servers")
	}

	seenIDs := make(map[YamlServerID]struct{})
	for _, server := range *s {
		// ID
		if err := server.ID.Validate(); err != nil {
			return fmt.Errorf("invalid server ID '%s': %w", server.ID, err)
		}

		if _, ok := seenIDs[server.ID]; ok {
			return fmt.Errorf("duplicate server ID '%s'", server.ID)
		}
		seenIDs[server.ID] = struct{}{}

		// Server
		if err := server.validate(); err != nil {
			return fmt.Errorf("invalid server with ID '%s': %w", server.ID, err)
		}
	}
	return nil
}

func (t YamlServerID) Validate() error {
	if t == "" {
		return errors.New("empty")
	}

	if len(t) > 32 {
		return errors.New("value is too long")
	}

	if !allowedAscii(t, true, true, true, true, []rune{'_', '-', ',', '@', '.', ' '}) {
		return errors.New("invalid character")
	}
	return nil
}

func (s *YamlServer) validate() error {
	// Address
	if s.Address != "" {
		if len(s.Address) > 80 {
			return errors.New("address is too long")
		}

		if strings.Contains(s.Address, ":") {
			v6, err := netip.ParseAddr(s.Address)
			if err != nil {
				return fmt.Errorf("invalid address")
			}
			if v6.IsLoopback() || v6.IsUnspecified() || v6.IsMulticast() || v6.IsLinkLocalUnicast() {
				return fmt.Errorf("IPv6 address type is not permitted")
			}
		} else {
			if !allowedAscii(s.Address, true, true, false, true, []rune{'-', '.'}) {
				return errors.New("address contains invalid characters")
			}
			if !strings.Contains(s.Address, ".") || strings.HasSuffix(s.Address, ".") ||
				strings.HasPrefix(s.Address, ".") {
				return errors.New("invalid address")
			}

			if v4, err := netip.ParseAddr(s.Address); err == nil {
				if v4.IsLoopback() || v4.IsUnspecified() || v4.IsMulticast() || v4.IsLinkLocalUnicast() {
					return fmt.Errorf("IPv4 address type is not permitted")
				}
			}
		}
	}

	// Country code
	if s.CountryCode == "" {
		return errors.New("country code is missing")
	}
	if len(s.CountryCode) != 2 {
		return errors.New("country code must be two characters long")
	}
	if !allowedAscii(s.CountryCode, true, false, true, false, nil) {
		return errors.New("country code must use valid upper case characters")
	}
	if !validateCountryAlpha2(s.CountryCode) {
		return errors.New("invalid country code")
	}

	// City
	if s.City != "" {
		if len(s.City) != 3 {
			return errors.New("city code must be three characters long")
		}
		if !allowedAscii(s.City, true, true, true, false, nil) {
			return errors.New("city code must use valid upper case characters")
		}
	}
	return nil
}

func (t YamlTags) validate() error {
	if len(t) > 64 {
		return errors.New("too many tags")
	}

	seen := make(map[YamlTag]struct{}, len(t))

	for _, tag := range t {
		if err := tag.validate(); err != nil {
			return fmt.Errorf("tag error: %w", err)
		}

		if _, exists := seen[tag]; exists {
			return fmt.Errorf("duplicate tag: %q", tag)
		}

		seen[tag] = struct{}{}
	}

	return nil
}

func (t YamlTag) validate() error {
	if slices.Contains(validTags, t) {
		return nil
	}
	return errors.New("not in allowed set")
}

// --- Validation helper function ---

func allowedAscii[T ~string](s T, letters, numbers, uppercase, lowercase bool, special []rune) bool {
	for _, r := range s {
		switch {
		case uppercase && r >= 'A' && r <= 'Z':
			if !letters {
				return false
			}
		case lowercase && r >= 'a' && r <= 'z':
			if !letters {
				return false
			}
		case r >= '0' && r <= '9':
			if !numbers {
				return false
			}
		default:
			if special == nil {
				return false
			}
			// Check if the rune is in the allowed special characters
			if !slices.Contains(special, r) {
				return false
			}
		}
	}
	return true
}

func containsWhitespace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func validateCountryAlpha2(code string) bool {
	country := countries.ByName(code)
	return country.IsValid() && country.Alpha2() == code
}

func isPrintable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] > 126 {
			return false
		}
	}
	return true
}
