package webhook_sender

import (
	"fmt"
	"strings"
	"time"

	"github.com/UnownHash/Fletchling/areas"
)

type SettingsConfig struct {
	FlushIntervalSeconds int `koanf:"flush_interval_seconds"`
}

func (cfg SettingsConfig) FlushInterval() time.Duration {
	return time.Second * time.Duration(cfg.FlushIntervalSeconds)
}

func (cfg SettingsConfig) Validate() error {
	if sec := cfg.FlushIntervalSeconds; sec < 1 {
		return fmt.Errorf("webhooks flush_interval_seconds should be at least 1, not %d", sec)
	}
	return nil
}

type WebhookConfig struct {
	Url     string   `koanf:"url"`
	Areas   []string `koanf:"areas"`
	Headers []string `koanf:"headers"`
}

func (cfg *WebhookConfig) HeadersAsMap() map[string]string {
	headerMap := make(map[string]string)
	for _, header := range cfg.Headers {
		split := strings.Split(header, ":")
		if len(split) == 2 {
			headerMap[split[0]] = split[1]
		}
	}
	return headerMap
}

func (cfg *WebhookConfig) AreaNames() []areas.AreaName {
	return areas.AreaStringsToAreaNames(cfg.Areas)
}

func (cfg *WebhookConfig) Validate() error {
	return nil
}

type WebhooksConfig []WebhookConfig

func (cfg WebhooksConfig) Validate() error {
	for _, webhookCfg := range cfg {
		if err := webhookCfg.Validate(); err != nil {
			return err
		}
	}
	return nil
}
