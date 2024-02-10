package webhook_sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/processor/models"
)

type NestWebhookMessage struct {
	Type    string      `json:"type"`
	Message NestWebhook `json:"message"`
}

type webhookDestination struct {
	logger     *logrus.Logger
	config     WebhookConfig
	areaNames  []geo.AreaName
	httpClient *http.Client
}

func (dest *webhookDestination) sendMessages(messages []NestWebhookMessage) error {
	var buf bytes.Buffer

	messages = dest.filterMessages(messages)
	if len(messages) == 0 {
		return nil
	}

	dest.logger.Infof("PoracleSender: sending %d nest webhook(s)", len(messages))

	encoder := json.NewEncoder(&buf)
	err := encoder.Encode(messages)
	if err != nil {
		return fmt.Errorf("couldn't json encode webhook: %w", err)
	}

	url := dest.config.Url

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return fmt.Errorf("couldn't create request for porable webhook to '%s': %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range dest.config.HeadersAsMap() {
		req.Header.Set(k, v)
	}

	resp, err := dest.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make porable webhook request to '%s': %w", url, err)
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		dest.logger.Warnf("PoracleSender: received http response: %d", resp.StatusCode)
	}

	return nil
}

func (dest *webhookDestination) filterMessages(messages []NestWebhookMessage) []NestWebhookMessage {
	if len(dest.areaNames) == 0 {
		return messages
	}
	filteredMessages := make([]NestWebhookMessage, 0)
	for _, message := range messages {
		if !geo.AreaMatchWithWildcards(message.Message.AreaName, dest.areaNames) {
			continue
		}
		filteredMessages = append(filteredMessages, message)
	}
	return filteredMessages
}

type NestWebhook struct {
	NestId       int64   `json:"nest_id"`
	Name         string  `json:"name"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	PokemonId    int     `json:"pokemon_id"`
	Form         int     `json:"form"`
	Type         int     `json:"type"` // always 0?
	PokemonCount uint64  `json:"pokemon_count"`
	PokemonAvg   float64 `json:"pokemon_avg"`
	PokemonRatio float64 `json:"pokemon_ratio"`
	PolyPath     string  `json:"poly_path"`  // json encoded path. poracle json parses this.
	ResetTime    int64   `json:"reset_time"` // used as discover time epoch

	//PolyType     int         `json:"poly_type"` // 1 if park, else 0? I don't see this in poracle tho
	//CurrentTime     int         `json:"current_time"`
	//NestSubmittedBy string      `json:"nest_submitted_by,omitempty"`

	AreaName geo.AreaName `json:"-"`
}

type nestWebhookQueue struct {
	messages []NestWebhookMessage
}

func (q *nestWebhookQueue) AddMessage(message NestWebhookMessage) {
	q.messages = append(q.messages, message)
}

type PoracleSender struct {
	logger *logrus.Logger

	flushInterval time.Duration

	mutex        sync.Mutex
	nestQueue    *nestWebhookQueue
	destinations []*webhookDestination
}

func (sender *PoracleSender) popNestMessagesFromQueue() []NestWebhookMessage {
	q := sender.nestQueue
	sender.mutex.Lock()
	messages := q.messages[:]
	q.messages = q.messages[:0]
	sender.mutex.Unlock()
	return messages
}

func (sender *PoracleSender) AddNestWebhook(nest *models.Nest, ni *models.NestingPokemonInfo) {
	center := nest.Center
	polyPathJson, _ := json.Marshal(geo.PathFromGeometry(nest.Geometry.Geometry()))
	webhook := NestWebhook{
		NestId:       nest.Id,
		Name:         nest.Name,
		Lat:          center.Lat(),
		Lon:          center.Lon(),
		PokemonId:    ni.PokemonKey.PokemonId,
		Form:         ni.PokemonKey.FormId,
		PokemonCount: ni.NestCount,
		PokemonAvg:   ni.NestHourlyCount,
		PokemonRatio: ni.NestPct(),
		ResetTime:    ni.DetectedAt.Unix(),
		PolyPath:     string(polyPathJson),
		AreaName:     geo.AreaName{Parent: "", Name: nest.AreaName.ValueOrZero()},
	}

	whMessage := NestWebhookMessage{
		Type:    "nest",
		Message: webhook,
	}
	sender.mutex.Lock()
	sender.nestQueue.AddMessage(whMessage)
	sender.mutex.Unlock()
}

// Flush will send the collected webhooks. This is meant to be used after
// the web server has been shut down and before the program exits.
func (sender *PoracleSender) Flush() {
	var wg sync.WaitGroup

	messages := sender.popNestMessagesFromQueue()
	for _, destination := range sender.destinations {
		wg.Add(1)
		go func(destination *webhookDestination) {
			defer wg.Done()
			destination.sendMessages(messages)
		}(destination)
	}
	wg.Wait()
}

// Run will monitor the webhooks collection and send in bulk every 1s.
// This blocks until `ctx` is cancelled.
func (sender *PoracleSender) Run(ctx context.Context) error {
	ticker := time.NewTicker(sender.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sender.logger.Infof("PoracleSender: asked to shut down. Flushing webhooks...")
			sender.Flush()
			sender.logger.Infof("PoracleSender: done flushing webhooks...")
			return nil
		case <-ticker.C:
			go sender.Flush()
		}
	}
}

func NewPoracleSender(logger *logrus.Logger, webhooks WebhooksConfig, settings SettingsConfig) (*PoracleSender, error) {
	flushInterval := settings.FlushInterval()
	if flushInterval <= 0 {
		flushInterval = time.Second
	}

	destinations := make([]*webhookDestination, len(webhooks))
	for idx, webhookCfg := range webhooks {
		destinations[idx] = &webhookDestination{
			logger:     logger,
			config:     webhookCfg,
			areaNames:  webhookCfg.AreaNames(),
			httpClient: &http.Client{},
		}
	}

	logger.Infof("PoracleSender: Added %d nest webhook destination(s)", len(destinations))

	sender := &PoracleSender{
		logger:        logger,
		flushInterval: flushInterval,
		destinations:  destinations,
		nestQueue:     &nestWebhookQueue{},
	}

	return sender, nil
}
