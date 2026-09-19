package cli

import (
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// loadProviderRouter builds the multi-profile router from env + ~/.inferenesia/config.yaml.
// preferred is the CLI --profile value (empty uses default selection).
func loadProviderRouter(preferred string) (*provider.Router, error) {
	file, err := config.LoadFromHome()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	r := provider.Bootstrap(provider.BootstrapOptions{
		File:        file,
		PreferredID: strings.TrimSpace(preferred),
	})
	return r, nil
}
