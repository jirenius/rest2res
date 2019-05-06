package service

import res "github.com/jirenius/go-res"

// Config holds server configuration
type Config struct {
	ServiceName string        `json:"serviceName"`
	Endpoints   []EndpointCfg `json:"endpoints"`
}

type EndpointCfg struct {
	URL          string `json:"url"`
	RefreshTime  int    `json:"refreshTime"`
	RefreshCount int    `json:"refreshCount"`
	Timeout      int    `json:"timeout"`
	Access       res.AccessHandler
	ResourceCfg
}

type ResourceCfg struct {
	Type      string        `json:"type,omitempty"`
	Pattern   string        `json:"pattern,omitempty"`
	Path      string        `json:"path,omitempty"`
	IDProp    string        `json:"idProp,omitempty"`
	Resources []ResourceCfg `json:"resources,omitempty"`
}

// SetDefault sets the default values
func (c *Config) SetDefault() {
	if c.ServiceName == "" {
		c.ServiceName = "rest2res"
	}
	if c.Endpoints == nil {
		c.Endpoints = []EndpointCfg{}
	}
	for i := range c.Endpoints {
		ep := &c.Endpoints[i]
		if ep.RefreshTime == 0 {
			ep.RefreshTime = 5000
		}
		if ep.RefreshCount == 0 {
			ep.RefreshCount = 12
		}
	}
}
