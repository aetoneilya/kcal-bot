package model

import (
	"math"
	"strings"
	"testing"
)

func TestNormalizeMakesModelOutputSafe(t *testing.T) {
	draft := Draft{
		Title: "  Завтрак  ",
		Items: []Item{
			{Name: "овсянка", Grams: 251.6, Kcal100: 88.4, Protein100: 3.04, Fat100: 1.66, Carbs100: 15.01, Confidence: ConfidenceHigh},
			{Name: "   "},
			{Name: "масло", Grams: -5, Kcal100: 1500, Fat100: math.NaN(), Confidence: "very sure"},
		},
	}
	got := draft.Normalize()
	if got.Title != "Завтрак" {
		t.Fatalf("title not trimmed: %q", got.Title)
	}
	if len(got.Items) != 2 {
		t.Fatalf("nameless items must be dropped, got %+v", got.Items)
	}
	oats := got.Items[0]
	if oats.Grams != 252 || oats.Kcal100 != 88 || oats.Protein100 != 3 || oats.Fat100 != 1.7 {
		t.Fatalf("values not rounded for display: %+v", oats)
	}
	butter := got.Items[1]
	if butter.Grams != 0 || butter.Kcal100 != MaxKcal100 || butter.Fat100 != 0 || butter.Confidence != ConfidenceLow {
		t.Fatalf("out-of-range values not clamped: %+v", butter)
	}
	if err := ValidateItems(got.Items); err != nil {
		t.Fatalf("a normalized draft must pass validation: %v", err)
	}
}

func TestNormalizeCapsItemsAndNames(t *testing.T) {
	var draft Draft
	for i := 0; i < MaxItems+5; i++ {
		draft.Items = append(draft.Items, Item{Name: strings.Repeat("я", MaxNameRunes+10), Grams: 1})
	}
	got := draft.Normalize()
	if len(got.Items) != MaxItems || len([]rune(got.Items[0].Name)) != MaxNameRunes {
		t.Fatalf("caps not applied: %d items, name %d runes", len(got.Items), len([]rune(got.Items[0].Name)))
	}
}

func TestNormalizeKeepsOptionsOnlyWithAQuestion(t *testing.T) {
	asked := Draft{Question: "Жарили на масле?", Options: []string{" Да ", "", "Нет", "Не знаю", "Четвёртый"}}.Normalize()
	if len(asked.Options) != MaxOptions || asked.Options[0] != "Да" || asked.Options[1] != "Нет" {
		t.Fatalf("options not trimmed and capped: %q", asked.Options)
	}
	if silent := (Draft{Options: []string{"Да"}}).Normalize(); silent.Options == nil || len(silent.Options) != 0 {
		t.Fatalf("options without a question must be an empty list, got %#v", silent.Options)
	}
}

func TestItemKcal(t *testing.T) {
	if got := (Item{Grams: 150, Kcal100: 200}).Kcal(); got != 300 {
		t.Fatalf("150 g at 200 kcal/100 g is 300 kcal, got %v", got)
	}
}
