package app

import (
	"database/sql"

	"github.com/jmatelli/typecrawl/internal/quotes"
	"github.com/jmatelli/typecrawl/internal/storage"
)

// quotePoolSize is how many quotes we try to keep cached locally -- enough
// variety for a long test without a network call on every quotes-mode
// launch, just the first one (or ones after a fresh install).
const quotePoolSize = 200

// loadQuotePool returns the local quote cache, topping it up from the
// quotes API first if it's running low. Both the fetch and the cache write
// are best-effort: a network failure still leaves whatever's already
// cached usable, and a cache that's still empty after a failed fetch falls
// back to a small built-in quote list so quotes mode works fully offline.
func loadQuotePool(db *sql.DB) []quotes.Quote {
	return loadQuotePoolWith(db, quotes.Fetch)
}

// loadQuotePoolWith is loadQuotePool's logic with the network fetch
// injected, so tests can exercise the fetch-fails and fetch-succeeds paths
// without ever making a real HTTP call.
func loadQuotePoolWith(db *sql.DB, fetch func() ([]quotes.Quote, error)) []quotes.Quote {
	cached, _ := storage.CachedQuotes(db)
	if len(cached) >= quotePoolSize {
		return toQuotes(cached)
	}

	if fetched, err := fetch(); err == nil && len(fetched) > 0 {
		_ = storage.StoreQuotes(db, toCachedQuotes(fetched))
		if refreshed, err := storage.CachedQuotes(db); err == nil {
			cached = refreshed
		}
	}

	if len(cached) == 0 {
		return quotes.Fallback
	}
	return toQuotes(cached)
}

func toQuotes(cs []storage.CachedQuote) []quotes.Quote {
	out := make([]quotes.Quote, len(cs))
	for i, c := range cs {
		out[i] = quotes.Quote{ID: c.ID, Text: c.Text, Author: c.Author}
	}
	return out
}

func toCachedQuotes(qs []quotes.Quote) []storage.CachedQuote {
	out := make([]storage.CachedQuote, len(qs))
	for i, q := range qs {
		out[i] = storage.CachedQuote{ID: q.ID, Text: q.Text, Author: q.Author}
	}
	return out
}
