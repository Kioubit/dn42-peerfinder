package config

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
)

var Global = config{}

type config struct {
	MyDomain           string
	ServersDirectory   string
	LocalFinderZipPath string
	MeasurementDir     string
	MaxOpenRequests    int
	MaxAgentsPerASN    int
}

var StaticEtag = ""

func init() {
	b := make([]byte, 12)
	_, _ = cryptorand.Read(b)
	StaticEtag = fmt.Sprintf(`"%s"`, base64.RawURLEncoding.EncodeToString(b))
}
