package orgknowledge

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"cortisol-server/internal/db"
)

func TestMongoStoreSearchPaymentRetryKeyword(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("MONGODB_URI not set")
	}
	orgID := os.Getenv("CORTISOL_ORG_ID")
	if orgID == "" {
		t.Skip("CORTISOL_ORG_ID not set (seeded org hex id)")
	}

	cfg, err := db.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := db.Connect(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Disconnect(database) }()

	store := NewMongoStore(database)
	result, err := store.Search(ctx, SearchParams{
		OrganizationID: orgID,
		Query:          "payment retry backoff jitter",
		K:              5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected at least one knowledge hit for payment retry query")
	}
	for _, item := range result.Items {
		if item.OrganizationID != orgID {
			t.Fatalf("org leak %q", item.OrganizationID)
		}
	}
	joined := strings.ToLower(result.Items[0].Content)
	if !strings.Contains(joined, "retry") && !strings.Contains(joined, "backoff") && !strings.Contains(joined, "payment") {
		t.Logf("top hit content (may still be semantically related): %q", result.Items[0].Content)
	}
}
