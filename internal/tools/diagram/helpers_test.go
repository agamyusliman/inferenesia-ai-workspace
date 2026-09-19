package diagram

import (
	"context"
	"encoding/json"
	"testing"
)

func testCtx() context.Context {
	return context.Background()
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
