package matching

import (
	"context"

	"github.com/simonhull/kitsune/internal/remote"
)

// Background marks ctx's requests as work done ahead, such as the queue asking
// for the next albums: they wait behind every request a person is waiting on.
// It is [remote.Background], which every gate reads.
func Background(ctx context.Context) context.Context { return remote.Background(ctx) }
