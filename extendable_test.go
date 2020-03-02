package ctxutil

import (
	"context"
	"testing"
	"time"
)

func TestExtendable_NoExtensions(t *testing.T) {
	ctx, _, _ := WithExtension(context.WithTimeout(context.Background(), 100*time.Second))

	assertValid(t, ctx)
	assertNotDone(t, ctx)
}
