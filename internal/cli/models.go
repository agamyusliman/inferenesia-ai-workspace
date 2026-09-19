package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/spf13/cobra"
)

func newModelsCmd() *cobra.Command {
	var (
		profileFlag string
		showTarget  bool
	)
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List models from the configured provider",
		Long: `List models discovered live from the active OpenAI-compatible profile
(GET {base_url}/models). Default is temp-ai gateway (TEMP_AI_*) or BYOK when
that is the selected default.

TEMP_AI_MODEL is not required. Model ids are printed one per line.
Credentials are never written to the output. Use --show-target for the request host.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			router, err := loadProviderRouter(profileFlag)
			if err != nil {
				return err
			}
			client, err := router.GetProfile(profileFlag)
			if err != nil {
				return err
			}
			if showTarget {
				fmt.Fprintf(cmd.OutOrStdout(), "profile: %s\n", client.Name())
				fmt.Fprintf(cmd.OutOrStdout(), "target: %s\n", client.RequestHost())
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			// Bound discovery so bad base URLs fail promptly.
			ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()

			models, err := client.ListModels(ctx)
			if err != nil {
				// VAL-CROSS-009: wrap tempai gateway errors as brand-correct
				// "Inferenesia: gateway unavailable (temp-ai)" so the operator can
				// retry or switch profile; non-tempai profiles keep their
				// existing error text. Never prints the API key.
				return formatProviderErr(wrapGatewayErrIfTempAI(client.Name(), client.RequestHost(), err))
			}
			out := cmd.OutOrStdout()
			for _, m := range models {
				id := strings.TrimSpace(m.ID)
				if id == "" {
					continue
				}
				fmt.Fprintln(out, id)
			}
			if len(models) == 0 {
				return fmt.Errorf("%s: no models returned by provider", brand.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileFlag, "profile", "", "Provider profile id (tempai, byok, or config.yaml providers)")
	cmd.Flags().BoolVar(&showTarget, "show-target", false, "Print profile id and request host (no API key)")
	return cmd
}
