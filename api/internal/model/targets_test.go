package model

import (
	"testing"
	"time"
)

// The same cases as web/src/lib/targets.test.ts.
func TestDailyTargetMatchesTheMiniApp(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	base := Profile{Sex: SexMale, BirthYear: 1996, HeightCm: 180, WeightKg: 80, GoalWeightKg: 80, Activity: ActivityModerate, PaceKgWeek: 0.5}
	with := func(f func(*Profile)) Profile { p := base; f(&p); return p }

	cases := []struct {
		name string
		p    Profile
		want float64
	}{
		{"hold", base, 2760},
		{"lose 0.5 kg a week", with(func(p *Profile) { p.GoalWeightKg = 74 }), 2210},
		{"deficit capped", with(func(p *Profile) { p.GoalWeightKg = 70; p.PaceKgWeek = 1 }), 2070},
		{"floor", Profile{Sex: SexFemale, BirthYear: 1996, HeightCm: 150, WeightKg: 48, GoalWeightKg: 44, Activity: ActivitySedentary, PaceKgWeek: 1}, 1200},
		{"gain", with(func(p *Profile) { p.GoalWeightKg = 86; p.PaceKgWeek = 0.25 }), 3030},
	}
	for _, c := range cases {
		if got := DailyTarget(c.p, now); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWaterGoalFollowsEnergySpent(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	woman := Profile{Sex: SexFemale, BirthYear: 1998, HeightCm: 165, WeightKg: 60, Activity: ActivityModerate}
	man := Profile{Sex: SexMale, BirthYear: 1996, HeightCm: 180, WeightKg: 80, Activity: ActivityModerate}
	sitting, athlete := woman, man
	sitting.Activity = ActivitySedentary
	athlete.Activity = ActivityVeryActive
	big := man
	big.WeightKg, big.Activity = 140, ActivityVeryActive
	for _, c := range []struct {
		name string
		p    *Profile
		want int
	}{
		{"no profile", nil, 2000},
		{"woman, moderate", &woman, 1750}, // EFSA: 1.6 l from drinks
		{"man, moderate", &man, 2250},     // EFSA: 2.0 l from drinks
		{"woman, sitting", &sitting, 1500},
		{"man, very active", &athlete, 2750},
		{"capped", &big, 3500},
	} {
		if got := WaterGoalMl(c.p, now); got != c.want {
			t.Errorf("%s: WaterGoalMl = %d, want %d", c.name, got, c.want)
		}
	}
}
