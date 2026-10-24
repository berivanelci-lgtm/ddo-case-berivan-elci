// Package spc, istatistiksel proses kontrolü (SPC) hesaplarını içerir.
package spc

import (
	"errors"
	"fmt"
	"math"
)

// BaselineSize, kontrol limitlerinin hesaplandığı ilk alt grup sayısıdır.
const BaselineSize = 25

// d2MovingRange, iki ardışık ölçümün hareketli aralığı için d2 sabitidir.
const d2MovingRange = 1.128

var (
	// ErrNotEnoughData, limit hesabı için yeterli ölçüm olmadığında döner.
	ErrNotEnoughData = errors.New("limit hesabı için yeterli veri yok")
	// ErrInvalidSubgroups, alt gruplar tutarsız ya da desteklenmeyen büyüklükteyse döner.
	ErrInvalidSubgroups = errors.New("alt gruplar geçersiz")
)

// Limits, bir kontrol grafiğinin merkez çizgisini ve limitlerini tutar.
// Sigma, grafikte çizilen istatistiğin (tek ölçüm ya da grup ortalaması) standart sapmasıdır.
type Limits struct {
	Center float64
	Sigma  float64
	UCL    float64
	LCL    float64
}

// RangeLimits, R grafiğinin merkez çizgisini ve limitlerini tutar.
type RangeLimits struct {
	Center float64
	UCL    float64
	LCL    float64
}

// XbarR, X̄-R grafiğinin iki parçasını birlikte tutar.
type XbarR struct {
	Mean  Limits
	Range RangeLimits
}

// subgroupConstants, alt grup büyüklüğüne göre standart SPC sabitleridir.
type subgroupConstants struct {
	a2, d3, d4 float64
}

var constantsBySize = map[int]subgroupConstants{
	2:  {a2: 1.880, d3: 0, d4: 3.267},
	3:  {a2: 1.023, d3: 0, d4: 2.574},
	4:  {a2: 0.729, d3: 0, d4: 2.282},
	5:  {a2: 0.577, d3: 0, d4: 2.114},
	6:  {a2: 0.483, d3: 0, d4: 2.004},
	7:  {a2: 0.419, d3: 0.076, d4: 1.924},
	8:  {a2: 0.373, d3: 0.136, d4: 1.864},
	9:  {a2: 0.337, d3: 0.184, d4: 1.816},
	10: {a2: 0.308, d3: 0.223, d4: 1.777},
}

func newLimits(center, sigma float64) Limits {
	return Limits{
		Center: center,
		Sigma:  sigma,
		UCL:    center + 3*sigma,
		LCL:    center - 3*sigma,
	}
}

// ComputeIndividual, bireysel (I-MR) grafik limitlerini hesaplar.
// Sigma, ardışık ölçümler arasındaki ortalama farktan (hareketli aralık) tahmin edilir.
func ComputeIndividual(values []float64) (Limits, error) {
	if len(values) < 2 {
		return Limits{}, fmt.Errorf("%w: en az 2 ölçüm gerekli, gelen %d", ErrNotEnoughData, len(values))
	}

	var movingRangeSum float64
	for i := 1; i < len(values); i++ {
		movingRangeSum += math.Abs(values[i] - values[i-1])
	}
	movingRangeMean := movingRangeSum / float64(len(values)-1)

	return newLimits(mean(values), movingRangeMean/d2MovingRange), nil
}

// ComputeXbarR, alt gruplardan X̄-R grafik limitlerini hesaplar.
// Tüm alt gruplar aynı büyüklükte olmalı ve büyüklük 2 ile 10 arasında olmalıdır.
func ComputeXbarR(subgroups [][]float64) (XbarR, error) {
	if len(subgroups) == 0 {
		return XbarR{}, fmt.Errorf("%w: en az 1 alt grup gerekli", ErrNotEnoughData)
	}

	size := len(subgroups[0])
	constants, supported := constantsBySize[size]
	if !supported {
		return XbarR{}, fmt.Errorf("%w: alt grup büyüklüğü 2 ile 10 arasında olmalı, gelen %d", ErrInvalidSubgroups, size)
	}

	var meanSum, rangeSum float64
	for i, group := range subgroups {
		if len(group) != size {
			return XbarR{}, fmt.Errorf("%w: %d. alt grubun büyüklüğü %d, beklenen %d", ErrInvalidSubgroups, i+1, len(group), size)
		}
		meanSum += mean(group)
		rangeSum += spread(group)
	}

	count := float64(len(subgroups))
	grandMean := meanSum / count
	rangeMean := rangeSum / count

	return XbarR{
		Mean: newLimits(grandMean, constants.a2*rangeMean/3),
		Range: RangeLimits{
			Center: rangeMean,
			UCL:    constants.d4 * rangeMean,
			LCL:    constants.d3 * rangeMean,
		},
	}, nil
}

func mean(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// spread, bir grubun aralığını (en büyük eksi en küçük) döner.
func spread(values []float64) float64 {
	lowest, highest := values[0], values[0]
	for _, v := range values[1:] {
		lowest = math.Min(lowest, v)
		highest = math.Max(highest, v)
	}
	return highest - lowest
}
