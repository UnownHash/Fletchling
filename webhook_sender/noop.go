package webhook_sender

import "github.com/UnownHash/Fletchling/processor/models"

type NoopSender struct{}

func (sender *NoopSender) AddNestWebhook(*models.Nest, *models.NestingPokemonInfo) {}

func NewNoopSender() *NoopSender {
	return &NoopSender{}
}
