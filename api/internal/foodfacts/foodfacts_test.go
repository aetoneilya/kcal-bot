package foodfacts

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLookupReadsTheLabelAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("User-Agent") != "KalorieKitten/test" {
			t.Errorf("missing user agent")
		}
		switch {
		case strings.Contains(r.URL.Path, "3017620422003"):
			_, _ = w.Write([]byte(`{"status":1,"product":{"product_name":"Nutella","brands":"Nutella, Ferrero",
				"product_quantity":"400","serving_quantity":15,
				"nutriments":{"energy-kcal_100g":539,"proteins_100g":6.3,"fat_100g":30.9,"carbohydrates_100g":57.5}}}`))
		case strings.Contains(r.URL.Path, "4600000000001"):
			_, _ = w.Write([]byte(`{"status":1,"product":{"product_name_ru":"Кефир 2,5%","brands":"Простоквашино",
				"nutriments":{"energy-kj_100g":222,"proteins_100g":2.9,"fat_100g":2.5,"carbohydrates_100g":4}}}`))
		default:
			_, _ = w.Write([]byte(`{"status":0,"status_verbose":"product not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := &Client{BaseURL: srv.URL, UserAgent: "KalorieKitten/test"}
	ctx := context.Background()

	p, err := c.Lookup(ctx, "3017620422003")
	if err != nil || p.Name != "Nutella" || p.Kcal100 != 539 || p.PackageGrams != 400 || p.ServingGrams != 15 {
		t.Fatalf("nutella: %+v %v", p, err)
	}
	if _, err := c.Lookup(ctx, "3017620422003"); err != nil || calls != 1 {
		t.Fatalf("a repeat lookup must come from the cache, calls=%d", calls)
	}

	k, err := c.Lookup(ctx, "4600000000001")
	if err != nil || k.Name != "Кефир 2,5% Простоквашино" || k.Kcal100 < 53 || k.Kcal100 > 53.1 {
		t.Fatalf("kefir from kJ with the brand appended: %+v %v", k, err)
	}

	if _, err := c.Lookup(ctx, "0000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown code: %v", err)
	}
}
