package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// healthTimeout is how long health waits for the running Lys, under Docker's 5 s.
const healthTimeout = 4 * time.Second

// runHealth asks the running Lys's /health, on NOTIFICATION_PORT (2400 when unset).
func runHealth(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) > 0 {
		return usageError("health takes no arguments.")
	}
	port := os.Getenv("NOTIFICATION_PORT")
	if port == "" {
		port = "2400"
	}
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:"+port+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Lys doesn't answer on port %s: is it running? (%w)", port, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Lys answers but isn't healthy (status %d): a chat connection is down; see its logs", resp.StatusCode)
	}
	env.Success("Lys is healthy.")
	return nil
}
