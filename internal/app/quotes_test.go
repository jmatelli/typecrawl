package app

import (
	"errors"
	"testing"

	"github.com/jmatelli/typecrawl/internal/quotes"
	"github.com/jmatelli/typecrawl/internal/storage"
)

func manyCachedQuotes(n int) []storage.CachedQuote {
	out := make([]storage.CachedQuote, n)
	for i := range out {
		out[i] = storage.CachedQuote{ID: i + 1, Text: "Quote.", Author: "Someone"}
	}
	return out
}

func TestLoadQuotePoolUsesCacheWithoutFetchingWhenFull(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.StoreQuotes(db, manyCachedQuotes(quotePoolSize)); err != nil {
		t.Fatal(err)
	}

	fetchCalled := false
	fetch := func() ([]quotes.Quote, error) {
		fetchCalled = true
		return nil, nil
	}
	pool := loadQuotePoolWith(db, fetch)
	if fetchCalled {
		t.Fatal("expected a full cache to skip the network fetch entirely")
	}
	if len(pool) != quotePoolSize {
		t.Fatalf("expected %d quotes from the cache, got %d", quotePoolSize, len(pool))
	}
}

func TestLoadQuotePoolFetchesAndCachesWhenBelowThreshold(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	fetch := func() ([]quotes.Quote, error) {
		return []quotes.Quote{{ID: 1, Text: "Fetched quote.", Author: "Someone"}}, nil
	}
	pool := loadQuotePoolWith(db, fetch)
	if len(pool) != 1 || pool[0].Text != "Fetched quote." {
		t.Fatalf("expected the freshly fetched quote in the pool, got %+v", pool)
	}

	cached, err := storage.CachedQuotes(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 1 {
		t.Fatalf("expected the fetched quote to be persisted to the cache, got %+v", cached)
	}
}

func TestLoadQuotePoolFallsBackToBuiltInListWhenFetchFailsAndCacheEmpty(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	fetch := func() ([]quotes.Quote, error) { return nil, errors.New("offline") }
	pool := loadQuotePoolWith(db, fetch)
	if len(pool) != len(quotes.Fallback) {
		t.Fatalf("expected the built-in fallback list (%d quotes), got %d", len(quotes.Fallback), len(pool))
	}
}

func TestLoadQuotePoolKeepsPartialCacheWhenFetchFails(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.StoreQuotes(db, manyCachedQuotes(3)); err != nil {
		t.Fatal(err)
	}

	fetch := func() ([]quotes.Quote, error) { return nil, errors.New("offline") }
	pool := loadQuotePoolWith(db, fetch)
	if len(pool) != 3 {
		t.Fatalf("expected the existing partial cache (3 quotes) to still be usable, got %d", len(pool))
	}
}
