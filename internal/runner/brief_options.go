package runner

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

type briefOptions struct {
	MaxDeliveryItems int
	Warning          string
}

func resolveBriefOptions(runtimeConfig map[string]string) briefOptions {
	value, warning := resolveMaxDeliveryItems(runtimeConfig)
	return briefOptions{MaxDeliveryItems: value, Warning: warning}
}

func resolveMaxDeliveryItems(runtimeConfig map[string]string) (int, string) {
	raw, ok := runtimeConfig[sqlite.RuntimeConfigMaxDeliveryItems]
	if !ok || strings.TrimSpace(raw) == "" {
		return sqlite.DefaultMaxDeliveryItems, ""
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || validateMaxDeliveryItems(value) != nil {
		return sqlite.DefaultMaxDeliveryItems, fmt.Sprintf("`%s` config value %q is invalid; using default %d", sqlite.RuntimeConfigMaxDeliveryItems, raw, sqlite.DefaultMaxDeliveryItems)
	}
	return value, ""
}

func validateMaxDeliveryItems(value int) error {
	if value <= 0 {
		return fmt.Errorf("%s must be between 1 and %d", sqlite.RuntimeConfigMaxDeliveryItems, sqlite.MaxDeliveryItemsUpperBound)
	}
	if value > sqlite.MaxDeliveryItemsUpperBound {
		return fmt.Errorf("%s must be between 1 and %d", sqlite.RuntimeConfigMaxDeliveryItems, sqlite.MaxDeliveryItemsUpperBound)
	}
	return nil
}
