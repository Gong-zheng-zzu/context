package vectorstore

import "testing"

func TestQdrantConfigUsesAuthoritativeVectorDBCollection(t *testing.T) {
	t.Setenv("QDRANT_URL", "http://qdrant.example")
	t.Setenv("VECTOR_DB_COLLECTION", "nursing_records")
	t.Setenv("QDRANT_COLLECTION", "legacy_collection")
	t.Setenv("EMBEDDING_API_URL", "http://embedding.example")
	t.Setenv("EMBEDDING_MODEL", "nomic-embed-text")

	config, err := loadQdrantConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.DefaultCollection != "nursing_records" || config.DatabaseConfig.Collection != "nursing_records" {
		t.Fatalf("configured collection = %q / %q, want nursing_records", config.DefaultCollection, config.DatabaseConfig.Collection)
	}
}
