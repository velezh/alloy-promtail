// Package scrapeconfig keeps the promtail target config used by loki.source.gelf in alloy-promtail.
package scrapeconfig

import (
	"github.com/prometheus/common/model"
)

// GelfTargetConfig describes a scrape config that read GELF messages on UDP.
type GelfTargetConfig struct {
	// ListenAddress is the address to listen on UDP for gelf messages. (Default to `:12201`)
	ListenAddress string `yaml:"listen_address"`

	// Labels optionally holds labels to associate with each record read from gelf messages.
	Labels model.LabelSet `yaml:"labels"`

	// UseIncomingTimestamp sets the timestamp to the incoming gelf messages
	// timestamp if it's set.
	UseIncomingTimestamp bool `yaml:"use_incoming_timestamp"`
}
