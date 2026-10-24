package spc

import (
	"errors"
	"math"
	"testing"
)

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s: beklenen %v, gelen %v", name, want, got)
	}
}

func TestComputeIndividual_KnownValues(t *testing.T) {
	// Ortalama 11; hareketli aralıklar 2,1,2,4 (ortalama 2.25); sigma 2.25/1.128.
	limits, err := ComputeIndividual([]float64{10, 12, 11, 13, 9})
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	assertClose(t, "merkez", limits.Center, 11)
	assertClose(t, "sigma", limits.Sigma, 1.9946808511)
	assertClose(t, "UCL", limits.UCL, 16.9840425532)
	assertClose(t, "LCL", limits.LCL, 5.0159574468)
}

func TestComputeIndividual_RequiresTwoValues(t *testing.T) {
	for _, values := range [][]float64{nil, {1}} {
		if _, err := ComputeIndividual(values); !errors.Is(err, ErrNotEnoughData) {
			t.Errorf("%v için ErrNotEnoughData beklenirdi, gelen %v", values, err)
		}
	}
}

func TestComputeIndividual_ConstantDataHasZeroWidth(t *testing.T) {
	limits, err := ComputeIndividual([]float64{5, 5, 5, 5})
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	assertClose(t, "merkez", limits.Center, 5)
	assertClose(t, "UCL", limits.UCL, 5)
	assertClose(t, "LCL", limits.LCL, 5)
}

func TestComputeXbarR_KnownValues(t *testing.T) {
	// Ortalamalar 11,12,10 (genel 11); aralıklar 2,2,2 (ortalama 2); n=2.
	result, err := ComputeXbarR([][]float64{{10, 12}, {11, 13}, {9, 11}})
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	assertClose(t, "X̄ merkez", result.Mean.Center, 11)
	assertClose(t, "X̄ UCL", result.Mean.UCL, 14.76)
	assertClose(t, "X̄ LCL", result.Mean.LCL, 7.24)
	assertClose(t, "X̄ sigma", result.Mean.Sigma, 1.2533333333)
	assertClose(t, "R merkez", result.Range.Center, 2)
	assertClose(t, "R UCL", result.Range.UCL, 6.534)
	assertClose(t, "R LCL", result.Range.LCL, 0)
}

func TestComputeXbarR_LowerRangeLimitForLargeSubgroups(t *testing.T) {
	// n=7 için D3=0.076, yani R grafiğinin alt limiti sıfırdan büyük olur.
	group := []float64{0, 10, 5, 5, 5, 5, 5}
	result, err := ComputeXbarR([][]float64{group, group, group})
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	assertClose(t, "R merkez", result.Range.Center, 10)
	assertClose(t, "R UCL", result.Range.UCL, 19.24)
	assertClose(t, "R LCL", result.Range.LCL, 0.76)
}

func TestComputeXbarR_RejectsUnequalSubgroupSizes(t *testing.T) {
	_, err := ComputeXbarR([][]float64{{1, 2}, {1, 2, 3}})
	if !errors.Is(err, ErrInvalidSubgroups) {
		t.Fatalf("ErrInvalidSubgroups beklenirdi, gelen %v", err)
	}
}

func TestComputeXbarR_RejectsUnsupportedSize(t *testing.T) {
	tooSmall := [][]float64{{1}, {2}}
	tooLarge := [][]float64{{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}}

	for name, subgroups := range map[string][][]float64{"tek ölçüm": tooSmall, "11 ölçüm": tooLarge} {
		if _, err := ComputeXbarR(subgroups); !errors.Is(err, ErrInvalidSubgroups) {
			t.Errorf("%s için ErrInvalidSubgroups beklenirdi, gelen %v", name, err)
		}
	}
}

func TestComputeXbarR_RejectsEmptyInput(t *testing.T) {
	if _, err := ComputeXbarR(nil); !errors.Is(err, ErrNotEnoughData) {
		t.Fatalf("ErrNotEnoughData beklenirdi, gelen %v", err)
	}
}
