package config

var Global = config{}

type config struct {
	MyDomain           string
	ServersDirectory   string
	LocalFinderZipPath string
	MeasurementDir     string
	MaxOpenRequests    int
	MaxAgentsPerASN    int
}
