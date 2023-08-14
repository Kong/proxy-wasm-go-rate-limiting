package config

import (
	"github.com/pquerna/ffjson/ffjson"
)

// -----------------------------------------------------------------------------
// Instance Config
// -----------------------------------------------------------------------------
//go:generate ffjson -noencoder $GOFILE

type Config struct {
	// Accepted hits per second
	Second int64 `json:"second"`

	// Accepted hits per minute
	Minute int64 `json:"minute"`

	// Accepted hits per hour
	Hour int64 `json:"hour"`

	// Accepted hits per day
	Day int64 `json:"day"`

	// Accepted hits per month
	Month int64 `json:"month"`

	// Accepted hits per year
	Year int64 `json:"year"`

	// Criteria to limit by
	LimitBy string `json:"limit_by" jsonschema:"enum=ip,enum=header,enum=path,default=ip"` // TODO consumer, credential, service

	// Header name to use when limiting by header
	HeaderName string `json:"header_name,omitempty" jsonschema:"pattern=^[A-Za-z0-9_]+$"`

	// Path to use when limiting by path
	Path string `json:"path" jsonschema:"pattern=^/[A-Za-z0-9_.~/%:@!$&'()*+,;=-]*$"` // TODO path validation is more complex (proper percent-encoding, no empty path segments)

	// Policy to adopt for counters
	Policy string `json:"policy" jsonschema:"enum=local,default=local"` // TODO cluster, redis

	// If counter cannot be determined, accept (true) or reject (false) request
	FaultTolerant bool `json:"fault_tolerant" jsonschema:"default=true"`

	// If enabled, does not return rate limit counter information in response headers
	HideClientHeaders bool `json:"hide_client_headers" jsonschema:"default=false"`
}

func Load(data []byte, conf *Config) error {
	// set defaults
	conf.Second = -1
	conf.Minute = -1
	conf.Hour = -1
	conf.Day = -1
	conf.Month = -1
	conf.Year = -1
	conf.LimitBy = "ip"
	conf.Policy = "local"
	conf.FaultTolerant = true
	conf.HideClientHeaders = false

	// load configuration
	err := ffjson.Unmarshal(data, conf)
	if err != nil {
		return err
	}

	return nil
}
