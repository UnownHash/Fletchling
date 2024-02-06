package nest_loader

import (
	"context"

	"github.com/UnownHash/Fletchling/processor/models"
)

type NestLoader interface {
	LoadNests(ctx context.Context) ([]*models.Nest, error)
}
