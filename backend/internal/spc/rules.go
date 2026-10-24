package spc

import (
	"errors"
	"fmt"
	"math"
)

// maxWindow, kuralların bakması gereken en uzun nokta dizisidir (8 ardışık nokta kuralı).
const maxWindow = 8

// ErrInvalidLimits, kural motoru geçersiz limitlerle kurulmaya çalışılırsa döner.
var ErrInvalidLimits = errors.New("kontrol limitleri geçersiz")

// Rule, bir kontrol grafiği kuralının kimliğidir.
type Rule string

const (
	// RuleBeyond3Sigma: tek nokta merkezden 3 sigmadan fazla uzakta (Western Electric 1 / Nelson 1).
	RuleBeyond3Sigma Rule = "beyond_3_sigma"
	// RuleSameSide8: 8 ardışık nokta merkez çizginin aynı tarafında.
	RuleSameSide8 Rule = "same_side_8"
	// RuleTrend6: 6 ardışık nokta sürekli artıyor ya da sürekli azalıyor (Nelson 3).
	RuleTrend6 Rule = "trend_6"
	// RuleTwoOfThreeBeyond2Sigma: ardışık 3 noktadan 2'si aynı tarafta 2 sigmadan uzakta (Western Electric 2).
	RuleTwoOfThreeBeyond2Sigma Rule = "two_of_three_beyond_2_sigma"
	// RuleFourOfFiveBeyond1Sigma: ardışık 5 noktadan 4'ü aynı tarafta 1 sigmadan uzakta (Western Electric 3).
	RuleFourOfFiveBeyond1Sigma Rule = "four_of_five_beyond_1_sigma"
)

// Violation, bir kuralın ihlal edildiği anı tanımlar.
type Violation struct {
	Rule  Rule
	Index int     // ihlali tetikleyen noktanın 0 tabanlı sırası
	Value float64 // ihlali tetikleyen noktanın değeri
}

// rulePredicate, son noktayla biten pencerede kuralın sağlanıp sağlanmadığını söyler.
type rulePredicate func(window []float64, l Limits) bool

type ruleSpec struct {
	rule Rule
	// perPoint true ise kural, uygun her nokta için ayrı alarm üretir.
	// false ise aynı ihlal sürdüğü sürece tek alarm üretilir.
	perPoint bool
	holds    rulePredicate
}

var ruleSpecs = []ruleSpec{
	{rule: RuleBeyond3Sigma, perPoint: true, holds: beyondThreeSigma},
	{rule: RuleSameSide8, holds: sameSideEight},
	{rule: RuleTrend6, holds: trendSix},
	{rule: RuleTwoOfThreeBeyond2Sigma, holds: twoOfThreeBeyondTwoSigma},
	{rule: RuleFourOfFiveBeyond1Sigma, holds: fourOfFiveBeyondOneSigma},
}

// Detector, ölçümler geldikçe kuralları çevrimiçi değerlendirir.
// Eşzamanlı kullanım için güvenli değildir; çağıran taraf kendi kilidini kullanmalıdır.
type Detector struct {
	limits Limits
	window []float64
	seen   int
	active map[Rule]bool
}

// NewDetector, verilen limitlerle yeni bir kural motoru oluşturur.
func NewDetector(limits Limits) (*Detector, error) {
	if limits.Sigma <= 0 || math.IsNaN(limits.Sigma) {
		return nil, fmt.Errorf("%w: sigma sıfırdan büyük olmalı, gelen %v", ErrInvalidLimits, limits.Sigma)
	}
	return &Detector{limits: limits, active: make(map[Rule]bool)}, nil
}

// Add, yeni bir noktayı değerlendirir ve bu nokta yüzünden başlayan ihlalleri döner.
func (d *Detector) Add(value float64) []Violation {
	d.window = append(d.window, value)
	if len(d.window) > maxWindow {
		d.window = d.window[len(d.window)-maxWindow:]
	}
	index := d.seen
	d.seen++

	var found []Violation
	for _, spec := range ruleSpecs {
		if !spec.holds(d.window, d.limits) {
			d.active[spec.rule] = false
			continue
		}
		if spec.perPoint || !d.active[spec.rule] {
			found = append(found, Violation{Rule: spec.rule, Index: index, Value: value})
		}
		d.active[spec.rule] = true
	}
	return found
}

func beyondThreeSigma(w []float64, l Limits) bool {
	above, below := countBeyond(w[len(w)-1:], l, 3)
	return above+below > 0
}

func sameSideEight(w []float64, l Limits) bool {
	if len(w) < 8 {
		return false
	}
	allAbove, allBelow := true, true
	for _, v := range w[len(w)-8:] {
		if v <= l.Center {
			allAbove = false
		}
		if v >= l.Center {
			allBelow = false
		}
	}
	return allAbove || allBelow
}

func trendSix(w []float64, _ Limits) bool {
	if len(w) < 6 {
		return false
	}
	last := w[len(w)-6:]
	rising, falling := true, true
	for i := 1; i < len(last); i++ {
		if last[i] <= last[i-1] {
			rising = false
		}
		if last[i] >= last[i-1] {
			falling = false
		}
	}
	return rising || falling
}

func twoOfThreeBeyondTwoSigma(w []float64, l Limits) bool {
	above, below := countBeyond(lastN(w, 3), l, 2)
	return above >= 2 || below >= 2
}

func fourOfFiveBeyondOneSigma(w []float64, l Limits) bool {
	above, below := countBeyond(lastN(w, 5), l, 1)
	return above >= 4 || below >= 4
}

// countBeyond, merkezden "sigmas" sigmadan fazla uzaktaki noktaları tarafına göre sayar.
func countBeyond(points []float64, l Limits, sigmas float64) (above, below int) {
	upper := l.Center + sigmas*l.Sigma
	lower := l.Center - sigmas*l.Sigma
	for _, v := range points {
		if v > upper {
			above++
		}
		if v < lower {
			below++
		}
	}
	return above, below
}

// lastN, dizinin son n elemanını döner; dizi daha kısaysa hepsini döner.
func lastN(w []float64, n int) []float64 {
	if len(w) < n {
		return w
	}
	return w[len(w)-n:]
}
