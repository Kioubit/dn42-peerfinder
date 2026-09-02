package directoryTypes

import (
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

// YamlNetwork represents the complete network configuration
type YamlNetwork struct {
	Name        string      `yaml:"name"`
	Mnt         string      `yaml:"mnt"`
	Description string      `yaml:"description,omitempty"`
	URL         string      `yaml:"url,omitempty"`
	Tags        YamlTags    `yaml:"tags,omitempty"`
	Servers     YamlServers `yaml:"servers" json:"Servers,omitempty"`
}

// YamlServers represents a mapping of server configurations
type YamlServers []YamlServer

// YamlServer represents a single server configuration
type YamlServer struct {
	ID          YamlServerID `yaml:"-"`
	Address     string       `yaml:"address,omitempty"`
	CountryCode string       `yaml:"country"`
	City        string       `yaml:"city,omitempty"`
	Tags        YamlTags     `yaml:"tags,omitempty"`
}

type YamlServerID string

type YamlTags []YamlTag
type YamlTag string

var validTags = []YamlTag{
	"automated-peering",
	"semi-automated-peering",
	"fast-reply",
	"testing",
	"e-mail",
	"irc",
	"telegram",
	"matrix",
	"xmpp",
	"signal",
	"wireguard",
	"openvpn",
	"gre",
	"ipsec",
	"tinc",
	"fastd",
	"stunnel",
	"v4-only",
	"v6-only",
	"NAT",
	"mp-bgp",
	"enh",
	"bfd",
	"selective-peering",
	"dynamic-ip",
}

// MarshalYAML Marshals YamlServers into an *ordered* YAML mapping
//
//goland:noinspection GoMixedReceiverTypes
func (s YamlServers) MarshalYAML() (any, error) {
	m := make(yaml.MapSlice, len(s))
	for i, srv := range s {
		m[i] = yaml.MapItem{
			Key:   string(srv.ID),
			Value: srv,
		}
	}
	return m, nil
}

// UnmarshalYAML Unmarshals a YAML mapping into *ordered* YamlServers
func (s *YamlServers) UnmarshalYAML(node ast.Node) error {
	m, ok := node.(*ast.MappingNode)
	if !ok {
		return fmt.Errorf("YamlServers: expected a mapping, got %T", node)
	}

	out := make(YamlServers, 0)
	mr := m.MapRange()
	for mr.Next() {
		// Decode the key
		var key string
		if err := yaml.NodeToValue(mr.Key(), &key); err != nil {
			return fmt.Errorf("YamlServers: error decoding key: %w", err)
		}

		// Decode this server's value from the AST
		var srv YamlServer
		if err := yaml.NodeToValue(mr.Value(), &srv); err != nil {
			return fmt.Errorf("YamlServers: error decoding server %q: %w", key, err)
		}
		srv.ID = YamlServerID(key)
		out = append(out, srv)
	}

	*s = out
	return nil
}
