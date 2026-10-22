package model

import (
	"math"
	"time"
)

// Expenditure is the energy a profile spends in a day, kcal: Mifflin–St
// Jeor resting energy times the activity factor.
func Expenditure(p Profile, now time.Time) float64 {
	age := float64(now.Year() - p.BirthYear)
	bmr := 10*p.WeightKg + 6.25*p.HeightCm - 5*age
	if p.Sex == SexMale {
		bmr += 5
	} else {
		bmr -= 161
	}
	return bmr * activityFactor[p.Activity]
}

// DailyTarget is the daily calorie target for a profile — the same formula
// as the Mini App's lib/targets.ts, which shows it live while the form is
// edited; the bot needs it for summaries. Keep the two in step: their tests
// pin the same numbers.
//
// Mifflin–St Jeor resting energy times an activity factor, shifted by the
// planned weekly change (~7700 kcal per kg), with the same guard rails: a
// deficit of at most a quarter, a surplus of at most 15%, and a floor of
// 1200 / 1500 kcal.
func DailyTarget(p Profile, now time.Time) float64 {
	floor := 1200.0
	if p.Sex == SexMale {
		floor = 1500
	}
	tdee := Expenditure(p, now)

	diff := p.GoalWeightKg - p.WeightKg
	planned := p.PaceKgWeek * 7700 / 7
	kcal := tdee
	switch {
	case math.Abs(diff) < 0.5:
	case diff < 0:
		kcal = tdee - math.Min(planned, tdee*0.25)
		if kcal < floor {
			kcal = math.Min(floor, tdee)
		}
	default:
		kcal = tdee + math.Min(planned, tdee*0.15)
	}
	return math.Round(kcal/10) * 10
}

var activityFactor = map[Activity]float64{
	ActivitySedentary:  1.2,
	ActivityLight:      1.375,
	ActivityModerate:   1.55,
	ActivityActive:     1.725,
	ActivityVeryActive: 1.9,
}
