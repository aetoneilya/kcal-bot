// Package foodfacts looks packaged food up by barcode in Open Food Facts,
// the open product database (world.openfoodfacts.org). A hit gives exact
// per-100 g values from the label without a model call.
package foodfacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrNotFound means the database has no usable entry for the code.
var ErrNotFound = errors.New("foodfacts: product not found")

type Product struct {
	Code       string
	Name       string
	Kcal100    float64
	Protein100 float64
	Fat100     float64
	Carbs100   float64
	// ServingGrams and PackageGrams are 0 when the label does not say.
	ServingGrams float64
	PackageGrams float64
}

type Client struct {
	HTTP    *http.Client
	BaseURL string // defaults to https://world.openfoodfacts.org
	// UserAgent identifies the app, as the database's terms ask.
	UserAgent string

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	product Product
	err     error
	at      time.Time
}

// Products rarely change; a day of caching spares the free service repeat
// lookups of the same yoghurt.
const cacheFor = 24 * time.Hour

func (c *Client) Lookup(ctx context.Context, code string) (Product, error) {
	c.mu.Lock()
	if hit, ok := c.cache[code]; ok && time.Since(hit.at) < cacheFor {
		c.mu.Unlock()
		return hit.product, hit.err
	}
	c.mu.Unlock()

	product, err := c.fetch(ctx, code)
	if err == nil || errors.Is(err, ErrNotFound) {
		c.mu.Lock()
		if c.cache == nil {
			c.cache = map[string]cached{}
		}
		c.cache[code] = cached{product: product, err: err, at: time.Now()}
		c.mu.Unlock()
	}
	return product, err
}

func (c *Client) fetch(ctx context.Context, code string) (Product, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://world.openfoodfacts.org"
	}
	url := base + "/api/v2/product/" + code +
		".json?fields=product_name,product_name_ru,brands,serving_quantity,product_quantity,nutriments"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Product{}, err
	}
	request.Header.Set("User-Agent", c.UserAgent)
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return Product{}, fmt.Errorf("foodfacts: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return Product{}, ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return Product{}, fmt.Errorf("foodfacts: status %d", response.StatusCode)
	}

	var body struct {
		Status  int `json:"status"`
		Product struct {
			Name            string   `json:"product_name"`
			NameRU          string   `json:"product_name_ru"`
			Brands          string   `json:"brands"`
			ServingQuantity flexible `json:"serving_quantity"`
			ProductQuantity flexible `json:"product_quantity"`
			Nutriments      struct {
				Kcal    *float64 `json:"energy-kcal_100g"`
				KJ      *float64 `json:"energy-kj_100g"`
				Protein float64  `json:"proteins_100g"`
				Fat     float64  `json:"fat_100g"`
				Carbs   float64  `json:"carbohydrates_100g"`
			} `json:"nutriments"`
		} `json:"product"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return Product{}, fmt.Errorf("foodfacts: decode: %w", err)
	}
	if body.Status != 1 {
		return Product{}, ErrNotFound
	}
	p := body.Product
	n := p.Nutriments
	var kcal float64
	switch {
	case n.Kcal != nil:
		kcal = *n.Kcal
	case n.KJ != nil:
		kcal = *n.KJ / 4.184
	default:
		// An entry without energy is no use for a calorie diary.
		return Product{}, ErrNotFound
	}
	return Product{
		Code:         code,
		Name:         name(p.NameRU, p.Name, p.Brands),
		Kcal100:      kcal,
		Protein100:   n.Protein,
		Fat100:       n.Fat,
		Carbs100:     n.Carbs,
		ServingGrams: float64(p.ServingQuantity),
		PackageGrams: float64(p.ProductQuantity),
	}, nil
}

// name prefers the Russian name and adds the first brand unless the name
// already carries it.
func name(ru, other, brands string) string {
	n := strings.TrimSpace(ru)
	if n == "" {
		n = strings.TrimSpace(other)
	}
	brand := strings.TrimSpace(strings.Split(brands, ",")[0])
	switch {
	case n == "":
		return brand
	case brand != "" && !strings.Contains(strings.ToLower(n), strings.ToLower(brand)):
		return n + " " + brand
	}
	return n
}

// flexible reads a number the database sends either as a number or a string.
type flexible float64

func (f *flexible) UnmarshalJSON(raw []byte) error {
	s := strings.Trim(string(raw), `"`)
	if s == "" || s == "null" {
		return nil
	}
	var v float64
	if _, err := fmt.Sscan(s, &v); err != nil {
		return nil // an unreadable quantity is just unknown
	}
	*f = flexible(v)
	return nil
}
