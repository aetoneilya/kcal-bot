// Package model holds the types shared by the store, the estimators and the
// HTTP layer.
//
// Nutrition is stored per 100 g, never as a line total. The weight is the
// number people correct, and keeping density separate means a corrected weight
// recomputes the line without asking the model again.
package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
}

// Confidence is the model's own read of how sure it is about a line's weight.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Item is one component of a meal: what it is, how much of it, and how dense.
type Item struct {
	Name       string     `json:"name"`
	Grams      float64    `json:"grams"`
	Kcal100    float64    `json:"kcal100"`
	Protein100 float64    `json:"protein100"`
	Fat100     float64    `json:"fat100"`
	Carbs100   float64    `json:"carbs100"`
	Confidence Confidence `json:"confidence"`
}

func (i Item) Kcal() float64 { return i.Grams * i.Kcal100 / 100 }

// Draft is what an estimator returns: a proposal the user edits before saving.
type Draft struct {
	Title string `json:"title"`
	Items []Item `json:"items"`
	// Question is a single clarifying question when something unseen (oil,
	// sauce, sugar) would move the total noticeably. Empty when there is none.
	Question string `json:"question"`
	// Options are up to MaxOptions short ready answers to Question, shown as
	// buttons. Empty when there is no question.
	Options []string `json:"options"`
}

// Meal is a saved, user-confirmed record.
type Meal struct {
	ID        int64     `json:"id"`
	EatenAt   time.Time `json:"eatenAt"`
	Title     string    `json:"title"`
	Note      string    `json:"note"`
	Items     []Item    `json:"items"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Limits that keep a typo (5000 instead of 500) or a confused model from
// writing nonsense into someone's diary.
const (
	MaxItems     = 40
	MaxNameRunes = 80
	MaxTitleRune = 80
	MaxNoteRunes = 500
	MaxGrams     = 5000
	MaxKcal100   = 900 // pure fat
	MaxMacro100  = 100

	MaxQuestionRunes = 200
	MaxOptions       = 3
	MaxOptionRunes   = 40
	// An answer is one of the options or a short phrase typed in their place.
	MaxAnswerRunes = 200
)

var ErrInvalid = errors.New("invalid input")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// ValidateItems rejects what a person submitted. Unlike Normalize it never
// fixes anything: a saved meal must be exactly what the user saw.
func ValidateItems(items []Item) error {
	if len(items) == 0 {
		return invalid("a meal needs at least one item")
	}
	if len(items) > MaxItems {
		return invalid("at most %d items per meal", MaxItems)
	}
	for n, item := range items {
		name := strings.TrimSpace(item.Name)
		switch {
		case name == "":
			return invalid("item %d has no name", n+1)
		case len([]rune(name)) > MaxNameRunes:
			return invalid("item %d: name is longer than %d characters", n+1, MaxNameRunes)
		case !inRange(item.Grams, 0, MaxGrams):
			return invalid("item %d: grams must be between 0 and %d", n+1, MaxGrams)
		case !inRange(item.Kcal100, 0, MaxKcal100):
			return invalid("item %d: kcal per 100 g must be between 0 and %d", n+1, MaxKcal100)
		case !inRange(item.Protein100, 0, MaxMacro100), !inRange(item.Fat100, 0, MaxMacro100), !inRange(item.Carbs100, 0, MaxMacro100):
			return invalid("item %d: macros per 100 g must be between 0 and %d", n+1, MaxMacro100)
		}
		switch item.Confidence {
		case ConfidenceHigh, ConfidenceMedium, ConfidenceLow, "":
		default:
			return invalid("item %d: unknown confidence %q", n+1, item.Confidence)
		}
	}
	return nil
}

// Normalize makes a model's answer safe to show: clamps out-of-range numbers,
// drops nameless lines and rounds to what a person can read. A model is
// allowed to be wrong; it is not allowed to break the editor.
func (d Draft) Normalize() Draft {
	out := Draft{
		Title:    truncate(strings.TrimSpace(d.Title), MaxTitleRune),
		Question: truncate(strings.TrimSpace(d.Question), MaxQuestionRunes),
		Items:    make([]Item, 0, len(d.Items)),
		Options:  []string{},
	}
	if out.Question != "" {
		for _, option := range d.Options {
			option = truncate(strings.TrimSpace(option), MaxOptionRunes)
			if option != "" && len(out.Options) < MaxOptions {
				out.Options = append(out.Options, option)
			}
		}
	}
	for _, item := range d.Items {
		name := truncate(strings.TrimSpace(item.Name), MaxNameRunes)
		if name == "" {
			continue
		}
		confidence := item.Confidence
		if confidence != ConfidenceHigh && confidence != ConfidenceMedium {
			confidence = ConfidenceLow
		}
		out.Items = append(out.Items, Item{
			Name:       name,
			Grams:      math.Round(clamp(item.Grams, 0, MaxGrams)),
			Kcal100:    math.Round(clamp(item.Kcal100, 0, MaxKcal100)),
			Protein100: round1(clamp(item.Protein100, 0, MaxMacro100)),
			Fat100:     round1(clamp(item.Fat100, 0, MaxMacro100)),
			Carbs100:   round1(clamp(item.Carbs100, 0, MaxMacro100)),
			Confidence: confidence,
		})
		if len(out.Items) == MaxItems {
			break
		}
	}
	return out
}

func ValidateText(field, value string, max int) error {
	if len([]rune(value)) > max {
		return invalid("%s is longer than %d characters", field, max)
	}
	return nil
}

func inRange(v, lo, hi float64) bool { return !math.IsNaN(v) && v >= lo && v <= hi }

func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) || v < lo {
		return lo
	}
	return math.Min(v, hi)
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
