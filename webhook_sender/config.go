package webhook_sender

import (
	"fmt"
	"strings"
	"time"

	"github.com/UnownHash/Fletchling/geo"
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
	Url string `koanf:"url"`
	// leaving this out for now. would be nice to have the parent
	// for the area. DB only has areaName. Before allowing this
	// setting, let's think about it. We'll put in a */* entry for
	// for now.
	Areas   []string `koanf:"-"`
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

func (cfg *WebhookConfig) AreaNames() []geo.AreaName {
	//return geo.AreaStringsToAreaNames(cfg.Areas)
	// see the struct:
	return []geo.AreaName{geo.AreaName{"", "*"}}
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
