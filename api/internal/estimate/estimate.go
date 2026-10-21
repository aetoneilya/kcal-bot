// Package estimate turns a meal photo and/or a description into a Draft.
//
// Research on LLM calorie estimation is consistent: models name foods well and
// judge portion weight badly. So the model is asked for a per-component
// breakdown with weight and per-100 g density, never for one total — the
// weight is what a person corrects, and the total follows.
package estimate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
)

// MaxPhotos is how many photos one meal may carry: the plate, another angle,
// the packaging with its label.
const MaxPhotos = 3

// Photo is one uploaded image, already checked to be JPEG, PNG or WebP.
type Photo struct {
	Data []byte
	MIME string
}

// Input is one request: photos of one meal, a note, or both.
type Input struct {
	Photos []Photo
	Note   string
	// Usual portions from this person's own saved meals; may be empty.
	History []Portion
	// Set on a re-estimate: the question the model asked last time and the
	// person's answer to it.
	Question string
	Answer   string
}

func (in Input) Empty() bool { return len(in.Photos) == 0 && strings.TrimSpace(in.Note) == "" }

// Portion is how much of one food a person usually eats, from meals they saved.
type Portion struct {
	Name    string
	Grams   float64 // median of the saved weights
	Kcal100 float64 // from the most recent entry
	Times   int
}

// Portions folds saved items (newest first) into one usual portion per food,
// most frequent first, at most max. Names match case-insensitively. The median
// keeps one unusual meal from moving the usual portion.
func Portions(items []model.Item, max int) []Portion {
	type acc struct {
		Portion
		grams []float64
		first int
	}
	byName := map[string]*acc{}
	for n, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.Name))
		if key == "" || item.Grams <= 0 {
			continue
		}
		a, ok := byName[key]
		if !ok {
			a = &acc{Portion: Portion{Name: strings.TrimSpace(item.Name), Kcal100: item.Kcal100}, first: n}
			byName[key] = a
		}
		a.grams = append(a.grams, item.Grams)
	}

	out := make([]*acc, 0, len(byName))
	for _, a := range byName {
		a.Times = len(a.grams)
		a.Grams = median(a.grams)
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Times != out[j].Times {
			return out[i].Times > out[j].Times
		}
		return out[i].first < out[j].first
	})

	portions := make([]Portion, 0, min(len(out), max))
	for _, a := range out[:min(len(out), max)] {
		portions = append(portions, a.Portion)
	}
	return portions
}

func median(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return math.Round((sorted[mid-1] + sorted[mid]) / 2)
	}
	return sorted[mid]
}

// Estimator is one model backend.
type Estimator interface {
	Name() string
	Estimate(ctx context.Context, in Input) (Result, error)
	// Priced reports whether Result.Cost is real. Spend limits rely on it.
	Priced() bool
}

// Result is one estimate and what it cost.
type Result struct {
	Draft model.Draft
	// Cost is the call's price in USD; always 0 for an unpriced backend.
	Cost  float64
	Usage Usage
	// Raw is the model's answer text, kept for logging odd answers.
	Raw string
}

// ErrUnusable means the model answered, but not with something we can show:
// a refusal, a truncated answer, or JSON that does not match the schema.
var ErrUnusable = errors.New("the model returned no usable estimate")

const systemPrompt = `You estimate the nutrition of a meal for a food diary. You get photos of the meal, a short note from the person, or both.

Several photos always show one meal: the plate from another angle, a close-up, or the packaging and its label. Combine them and count every food once — a food seen in two photos is one item.

Break the meal into its distinct components, one item per component the person could weigh separately (e.g. "гречка", "куриное бедро", "огурец"). For each item give:
- name: short, in Russian, specific enough to look up (prefer "куриное бедро запечённое без кожи" over "мясо");
- grams: your best estimate of the edible weight on the plate, as served;
- kcal100, protein100, fat100, carbs100: per 100 g of that food as it is prepared here (cooked rice, not dry; fried, not raw);
- confidence: how sure you are about the weight — "high", "medium" or "low".

Portion size is where estimates go wrong, so reason about it from visible cues before settling on grams:
- scale: a standard dinner plate is 26–27 cm across, a side plate about 20 cm, a soup bowl about 15–18 cm, a tablespoon about 15 cm long, a fork about 19 cm; measure food against whichever of these is in the frame;
- depth: how thick a layer is, how high a mound rises, how full a bowl or glass is;
- count: how many pieces, slices or spoonfuls there are.
Large portions are usually underestimated — do not shrink them. A restaurant or café plate is usually larger than a home portion of the same dish.

When packaging with a nutrition label is visible, take the per-100 g values from the label instead of estimating them, and use the stated net weight when the whole pack is being eaten.

When the person's usual portions are listed, they come from meals they weighed or corrected themselves. If the same food appears in the photo and nothing visible says the amount is different, use their usual weight and their per-100 g values for it. A list entry is not evidence that the food is on this plate — never add an item because it is in the list.

Add hidden components as their own low-confidence items when the dish almost certainly contains them: cooking oil for fried or sautéed food, dressing on a salad, butter in porridge, sugar in a drink. Do not invent garnish you cannot see or infer.

When the note states a weight, an ingredient or a cooking method, it overrides what the photo suggests. If the note says something is not there, leave it out. The same goes for an answer to a question you asked earlier.

title: a short Russian name for the whole meal, e.g. "Гречка с курицей".

question: one short question in Russian only when a detail you cannot see would change the total by more than about 20% and neither the note nor an earlier answer settles it (e.g. "Жарили на масле?"). Otherwise an empty string.

options: when you ask a question, two or three short Russian answers the person can tap, each a few words that settle it (e.g. "Да, на масле", "Без масла"). An empty list when there is no question.

If the image is not food, return no items and put a short explanation in question with no options.`

// userText is the text part sent alongside the photo.
func userText(in Input) string {
	var b strings.Builder
	note := strings.TrimSpace(in.Note)
	photos := "the photo"
	if len(in.Photos) > 1 {
		photos = fmt.Sprintf("the %d photos (one meal)", len(in.Photos))
	}
	switch {
	case len(in.Photos) == 0:
		b.WriteString("No photo. The person describes the meal:\n" + note)
	case note == "":
		b.WriteString("Estimate the meal in " + photos + ". The person added no note.")
	default:
		b.WriteString("Estimate the meal in " + photos + ". The person's note:\n" + note)
	}
	if in.Question != "" && in.Answer != "" {
		fmt.Fprintf(&b, "\n\nYou asked earlier: %s\nThe person answered: %s", in.Question, in.Answer)
	}
	if len(in.History) > 0 {
		b.WriteString("\n\nThis person's usual portions (name — usual weight, kcal per 100 g, times saved):")
		for _, p := range in.History {
			fmt.Fprintf(&b, "\n- %s — %g г, %g ккал/100 г, ×%d", p.Name, p.Grams, p.Kcal100, p.Times)
		}
	}
	return b.String()
}

// draftSchemaJSON is the JSON Schema both backends constrain output to.
// Structured outputs need additionalProperties: false and every property
// required.
//
// Property order matters: the model writes fields in schema order, so each
// item is named and weighed before its numbers, and the meal is titled after
// its items. It is kept as text because a Go map would marshal its keys
// alphabetically — "carbs100" before "name" — and the model, made to commit
// to numbers before saying what the food is, answered with empty zeros.
const draftSchemaJSON = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["items", "title", "question", "options"],
  "properties": {
    "items": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["name", "grams", "confidence", "kcal100", "protein100", "fat100", "carbs100"],
        "properties": {
          "name": {"type": "string"},
          "grams": {"type": "number"},
          "confidence": {"type": "string", "enum": ["high", "medium", "low"]},
          "kcal100": {"type": "number"},
          "protein100": {"type": "number"},
          "fat100": {"type": "number"},
          "carbs100": {"type": "number"}
        }
      }
    },
    "title": {"type": "string"},
    "question": {"type": "string"},
    "options": {"type": "array", "items": {"type": "string"}}
  }
}`

// draftSchema is draftSchemaJSON with its order intact, for request bodies.
var draftSchema = json.RawMessage(draftSchemaJSON)

// decode parses a model's JSON answer and normalizes it for the editor.
func decode(raw string) (model.Draft, error) {
	var draft model.Draft
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &draft); err != nil {
		return model.Draft{}, fmt.Errorf("%w: %v", ErrUnusable, err)
	}
	return draft.Normalize(), nil
}
