package estimate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
)

func TestPortionsFoldRepeatsIntoMedianByFrequency(t *testing.T) {
	items := []model.Item{ // newest first, as the store returns them
		{Name: "Овсянка на молоке", Grams: 260, Kcal100: 95},
		{Name: "банан", Grams: 120, Kcal100: 89},
		{Name: "овсянка на молоке", Grams: 240, Kcal100: 102},
		{Name: "овсянка на молоке", Grams: 600, Kcal100: 102}, // one unusual day
		{Name: "  ", Grams: 100},
		{Name: "вода", Grams: 0},
	}
	got := Portions(items, 10)
	if len(got) != 2 {
		t.Fatalf("want oatmeal and banana, got %+v", got)
	}
	oats := got[0]
	if oats.Name != "Овсянка на молоке" || oats.Times != 3 || oats.Grams != 260 || oats.Kcal100 != 95 {
		t.Fatalf("oatmeal: want the median weight and the newest entry's density, got %+v", oats)
	}
	if got[1].Name != "банан" || got[1].Times != 1 {
		t.Fatalf("banana second, got %+v", got[1])
	}
	if capped := Portions(items, 1); len(capped) != 1 || capped[0].Times != 3 {
		t.Fatalf("the cap keeps the most frequent, got %+v", capped)
	}
}

func TestUserTextCarriesAnswerAndHistory(t *testing.T) {
	text := userText(Input{
		Photo:    []byte{1},
		Question: "Жарили на масле?",
		Answer:   "Без масла",
		History:  []Portion{{Name: "гречка", Grams: 180, Kcal100: 110, Times: 4}},
	})
	for _, want := range []string{"no note", "You asked earlier: Жарили на масле?", "answered: Без масла", "- гречка — 180 г, 110 ккал/100 г, ×4"} {
		if !strings.Contains(text, want) {
			t.Errorf("user text lacks %q:\n%s", want, text)
		}
	}
	if plain := userText(Input{Note: "яблоко"}); strings.Contains(plain, "usual portions") || strings.Contains(plain, "asked earlier") {
		t.Errorf("no history or answer must add nothing:\n%s", plain)
	}
}

func TestSchemaKeepsItsOrder(t *testing.T) {
	var parsed map[string]any
	if err := json.Unmarshal(draftSchema, &parsed); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	raw := string(draftSchema)
	// Each item must be named and weighed before its numbers.
	if !(strings.Index(raw, `"name"`) < strings.Index(raw, `"grams"`) && strings.Index(raw, `"grams"`) < strings.Index(raw, `"kcal100"`)) {
		t.Fatalf("item properties out of order:\n%s", raw)
	}
	compact, _ := json.Marshal(draftSchema)
	if strings.Index(string(compact), `"name"`) > strings.Index(string(compact), `"carbs100"`) {
		t.Fatalf("marshalling must keep the order, got %s", compact)
	}
}
