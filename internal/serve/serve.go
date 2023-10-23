package serve

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// Start runs given services in parallel until either ctx.Done or any
// service exits, then it call cancel and wait until all services will
// exit.
//
// Returns error of first service which returned non-nil error, if any.
func Start(ctx context.Context, services ...func(context.Context) error) error {
	g, groupCtx := errgroup.WithContext(ctx)

	for i := range services {
		i := i

		g.Go(func() error { return services[i](groupCtx) })
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("got error executing %d services: %w", len(services), err)
	}

	return nil
}
