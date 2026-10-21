package estimate

// Usage is the tokens one model call consumed. Output includes reasoning.
type Usage struct {
	Input, CacheWrite, CacheRead, Output int64
}

// Price is a model's list price in USD per million tokens.
type Price struct {
	Input, CacheWrite, CacheRead, Output float64
}

// Cost is what u costs at p, in USD.
func (p Price) Cost(u Usage) float64 {
	return (float64(u.Input)*p.Input +
		float64(u.CacheWrite)*p.CacheWrite +
		float64(u.CacheRead)*p.CacheRead +
		float64(u.Output)*p.Output) / 1e6
}

// claudePrices are Anthropic's list prices, from
// platform.claude.com/docs/en/about-claude/pricing (checked 2026-10-21).
// Cache writes use the 5-minute rate. A model missing here is unpriced: the
// app still works, but per-user spend limits cannot be enforced for it.
var claudePrices = map[string]Price{
	"claude-sonnet-5-5": {Input: 2, CacheWrite: 2.5, CacheRead: 0.10, Output: 10},
	"claude-opus-5-5":   {Input: 4, CacheWrite: 5, CacheRead: 0.20, Output: 20},
	"claude-haiku-5-5":  {Input: 0.10, CacheWrite: 0.125, CacheRead: 0.01, Output: 0.50},
}
