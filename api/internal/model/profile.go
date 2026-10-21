package model

import (
	"math"
	"time"
)

// Profile is what a person tells about themselves so the app can work out a
// daily calorie target. Only inputs live here; the target is derived from
// them in the Mini App, where it updates live as the form is edited.
type Profile struct {
	Sex          Sex       `json:"sex"`
	BirthYear    int       `json:"birthYear"`
	HeightCm     float64   `json:"heightCm"`
	WeightKg     float64   `json:"weightKg"`
	GoalWeightKg float64   `json:"goalWeightKg"`
	Activity     Activity  `json:"activity"`
	PaceKgWeek   float64   `json:"paceKgWeek"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Sex string

const (
	SexMale   Sex = "male"
	SexFemale Sex = "female"
)

type Activity string

const (
	ActivitySedentary  Activity = "sedentary"
	ActivityLight      Activity = "light"
	ActivityModerate   Activity = "moderate"
	ActivityActive     Activity = "active"
	ActivityVeryActive Activity = "very_active"
)

// Bounds wide enough for any adult, narrow enough to catch a typo (1.80 for
// height, 750 for weight). The formulas the target uses are validated for
// adults, hence the age floor.
const (
	MinAge, MaxAge           = 16, 100
	MinHeightCm, MaxHeightCm = 120, 230
	MinWeightKg, MaxWeightKg = 30, 300
	MaxPaceKgWeek            = 1
)

// Validate rejects a profile the target formulas cannot use. now fixes the
// current year for the age check.
func (p Profile) Validate(now time.Time) error {
	age := now.Year() - p.BirthYear
	switch {
	case p.Sex != SexMale && p.Sex != SexFemale:
		return invalid("sex must be %q or %q", SexMale, SexFemale)
	case age < MinAge || age > MaxAge:
		return invalid("age must be between %d and %d", MinAge, MaxAge)
	case !inRange(p.HeightCm, MinHeightCm, MaxHeightCm):
		return invalid("height must be between %d and %d cm", MinHeightCm, MaxHeightCm)
	case !inRange(p.WeightKg, MinWeightKg, MaxWeightKg):
		return invalid("weight must be between %d and %d kg", MinWeightKg, MaxWeightKg)
	case !inRange(p.GoalWeightKg, MinWeightKg, MaxWeightKg):
		return invalid("goal weight must be between %d and %d kg", MinWeightKg, MaxWeightKg)
	case !inRange(p.PaceKgWeek, 0, MaxPaceKgWeek):
		return invalid("pace must be between 0 and %d kg a week", MaxPaceKgWeek)
	}
	switch p.Activity {
	case ActivitySedentary, ActivityLight, ActivityModerate, ActivityActive, ActivityVeryActive:
	default:
		return invalid("unknown activity %q", p.Activity)
	}
	return nil
}

// Weight is one weigh-in on a local calendar day.
type Weight struct {
	Day      string  `json:"day"` // YYYY-MM-DD, the phone's date
	WeightKg float64 `json:"weightKg"`
}

// ValidateWeight checks a weigh-in; the day must be a real date.
func ValidateWeight(w Weight) error {
	if _, err := time.Parse(time.DateOnly, w.Day); err != nil {
		return invalid("day must be YYYY-MM-DD")
	}
	if !inRange(w.WeightKg, MinWeightKg, MaxWeightKg) {
		return invalid("weight must be between %d and %d kg", MinWeightKg, MaxWeightKg)
	}
	return nil
}

// Normalize rounds the numbers to what the form shows, so a saved profile
// reads back exactly as entered.
func (p Profile) Normalize() Profile {
	p.HeightCm = math.Round(p.HeightCm)
	p.WeightKg = round1(p.WeightKg)
	p.GoalWeightKg = round1(p.GoalWeightKg)
	p.PaceKgWeek = math.Round(p.PaceKgWeek*100) / 100
	return p
}
