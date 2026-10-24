package spc

import (
	"errors"
	"testing"
)

type expected struct {
	rule  Rule
	index int
}

// testLimits: merkez 10, sigma 1 (1σ: 9-11, 2σ: 8-12, 3σ: 7-13).
func testLimits() Limits {
	return Limits{Center: 10, Sigma: 1, UCL: 13, LCL: 7}
}

func newTestDetector(t *testing.T) *Detector {
	t.Helper()
	d, err := NewDetector(testLimits())
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return d
}

// feed, değerleri sırayla ekler ve üretilen tüm ihlalleri döner.
func feed(d *Detector, values ...float64) []Violation {
	var all []Violation
	for _, v := range values {
		all = append(all, d.Add(v)...)
	}
	return all
}

func repeat(value float64, times int) []float64 {
	values := make([]float64, times)
	for i := range values {
		values[i] = value
	}
	return values
}

func join(parts ...[]float64) []float64 {
	var all []float64
	for _, part := range parts {
		all = append(all, part...)
	}
	return all
}

func assertViolations(t *testing.T, got []Violation, want ...expected) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d ihlal beklenirdi, gelen %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i].Rule != want[i].rule || got[i].Index != want[i].index {
			t.Errorf("%d. ihlal: beklenen %s@%d, gelen %s@%d",
				i+1, want[i].rule, want[i].index, got[i].Rule, got[i].Index)
		}
	}
}

func TestNewDetector_RejectsInvalidSigma(t *testing.T) {
	for name, sigma := range map[string]float64{"sıfır": 0, "negatif": -1} {
		t.Run(name, func(t *testing.T) {
			_, err := NewDetector(Limits{Center: 10, Sigma: sigma})
			if !errors.Is(err, ErrInvalidLimits) {
				t.Fatalf("ErrInvalidLimits beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestBeyondThreeSigma_FiresAboveUCL(t *testing.T) {
	got := feed(newTestDetector(t), 10, 13.5)
	assertViolations(t, got, expected{RuleBeyond3Sigma, 1})
}

func TestBeyondThreeSigma_FiresBelowLCL(t *testing.T) {
	got := feed(newTestDetector(t), 10, 6.5)
	assertViolations(t, got, expected{RuleBeyond3Sigma, 1})
}

func TestBeyondThreeSigma_PointOnLimitDoesNotFire(t *testing.T) {
	got := feed(newTestDetector(t), 13, 7)
	assertViolations(t, got)
}

func TestBeyondThreeSigma_EachPointFiresSeparately(t *testing.T) {
	got := feed(newTestDetector(t), 13.5, 6.5)
	assertViolations(t, got, expected{RuleBeyond3Sigma, 0}, expected{RuleBeyond3Sigma, 1})
}

func TestSameSideEight_FiresOnEighthPoint(t *testing.T) {
	d := newTestDetector(t)

	assertViolations(t, feed(d, repeat(10.5, 7)...))
	assertViolations(t, d.Add(10.5), expected{RuleSameSide8, 7})
}

func TestSameSideEight_NoDuplicateWhileRunContinues(t *testing.T) {
	got := feed(newTestDetector(t), repeat(10.5, 12)...)
	assertViolations(t, got, expected{RuleSameSide8, 7})
}

func TestSameSideEight_FiresAgainAfterRunBreaks(t *testing.T) {
	values := join(repeat(10.5, 8), []float64{9.5}, repeat(10.5, 8))

	got := feed(newTestDetector(t), values...)

	assertViolations(t, got, expected{RuleSameSide8, 7}, expected{RuleSameSide8, 16})
}

func TestSameSideEight_AlternatingSidesDoNotFire(t *testing.T) {
	values := []float64{10.5, 9.5, 10.5, 9.5, 10.5, 9.5, 10.5, 9.5}
	assertViolations(t, feed(newTestDetector(t), values...))
}

func TestSameSideEight_PointOnCenterBreaksRun(t *testing.T) {
	values := join(repeat(10.5, 7), []float64{10.0, 10.5})
	assertViolations(t, feed(newTestDetector(t), values...))
}

func TestTrendSix_Rising(t *testing.T) {
	got := feed(newTestDetector(t), 9.5, 9.7, 9.9, 10.1, 10.3, 10.5)
	assertViolations(t, got, expected{RuleTrend6, 5})
}

func TestTrendSix_Falling(t *testing.T) {
	got := feed(newTestDetector(t), 10.5, 10.3, 10.1, 9.9, 9.7, 9.5)
	assertViolations(t, got, expected{RuleTrend6, 5})
}

func TestTrendSix_FivePointsAreNotEnough(t *testing.T) {
	got := feed(newTestDetector(t), 9.5, 9.7, 9.9, 10.1, 10.3)
	assertViolations(t, got)
}

func TestTrendSix_NoDuplicateWhileTrendContinues(t *testing.T) {
	got := feed(newTestDetector(t), 9.4, 9.6, 9.8, 10.0, 10.2, 10.4, 10.6)
	assertViolations(t, got, expected{RuleTrend6, 5})
}

func TestTrendSix_EqualValuesBreakTrend(t *testing.T) {
	got := feed(newTestDetector(t), 9.5, 9.7, 9.9, 9.9, 10.1, 10.3)
	assertViolations(t, got)
}

func TestTwoOfThree_FiresOnSecondBeyondPoint(t *testing.T) {
	got := feed(newTestDetector(t), 12.5, 10.0, 12.5)
	assertViolations(t, got, expected{RuleTwoOfThreeBeyond2Sigma, 2})
}

func TestTwoOfThree_OppositeSidesDoNotCount(t *testing.T) {
	got := feed(newTestDetector(t), 12.5, 7.5, 10.0)
	assertViolations(t, got)
}

func TestTwoOfThree_NoDuplicateThenFiresAgainAfterReset(t *testing.T) {
	got := feed(newTestDetector(t), 12.5, 12.5, 10.0, 10.0, 12.5, 12.5)
	assertViolations(t, got,
		expected{RuleTwoOfThreeBeyond2Sigma, 1},
		expected{RuleTwoOfThreeBeyond2Sigma, 5},
	)
}

func TestFourOfFive_Fires(t *testing.T) {
	got := feed(newTestDetector(t), 11.5, 11.5, 11.5, 10.0, 11.5)
	assertViolations(t, got, expected{RuleFourOfFiveBeyond1Sigma, 4})
}

func TestFourOfFive_OppositeSidesDoNotCount(t *testing.T) {
	got := feed(newTestDetector(t), 11.5, 8.5, 11.5, 8.5, 11.5)
	assertViolations(t, got)
}
