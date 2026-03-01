package validators

import (
	"context"

	"github.com/Grey-Magic/kunji/pkg/models"
)

type Validator interface {
	Name() string
	KeyPatterns() []string
	Validate(ctx context.Context, apiKey string) (*models.ValidationResult, error)
}
