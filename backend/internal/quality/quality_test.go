package quality

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/spc"
)

var today = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

func chartID(id int64) *int64 { return &id }

func validForm() Form {
	due := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	return Form{
		ID: 1, Code: " QF-1 ", Name: " Flanş kontrolü ", GaugeCode: "KUMPAS-01", CalibrationDue: &due,
		Items: []Item{
			{Characteristic: " Çap ", Unit: "mm", Nominal: 10, LSL: 9.5, USL: 10.5, ChartID: chartID(7)},
			{Characteristic: "Boy", Unit: "mm", Nominal: 50, LSL: 49, USL: 51},
		},
	}
}

func TestValidateForm_CleansAndNumbers(t *testing.T) {
	f, err := ValidateForm(validForm())
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if f.Code != "QF-1" || f.Name != "Flanş kontrolü" || f.Items[0].Characteristic != "Çap" ||
		f.Items[0].Position != 1 || f.Items[1].Position != 2 {
		t.Fatalf("beklenmeyen form: %+v", f)
	}
}

func TestValidateForm_Rejects(t *testing.T) {
	cases := map[string]func(*Form){
		"kod yok":           func(f *Form) { f.Code = " " },
		"karakteristik yok": func(f *Form) { f.Items = nil },
		"alt üstten büyük":  func(f *Form) { f.Items[0].LSL, f.Items[0].USL = 11, 9 },
		"nominal dışarıda":  func(f *Form) { f.Items[0].Nominal = 12 },
		"NaN sınır":         func(f *Form) { f.Items[1].USL = math.NaN() },
		"sıfır grafik":      func(f *Form) { f.Items[0].ChartID = chartID(0) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := validForm()
			mutate(&f)
			if _, err := ValidateForm(f); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ErrInvalid beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestEvaluate_MarksOutOfSpec(t *testing.T) {
	values, all, err := Evaluate(validForm(), []float64{10.1, 51.5}, today)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if all || !values[0].InSpec || values[1].InSpec {
		t.Fatalf("çap içeride, boy dışarıda olmalı: %+v", values)
	}
}

func TestEvaluate_LimitsAreInclusive(t *testing.T) {
	_, all, err := Evaluate(validForm(), []float64{9.5, 51}, today)
	if err != nil || !all {
		t.Fatalf("sınır değerleri tolerans içinde sayılmalı: %v %v", all, err)
	}
}

func TestEvaluate_Calibration(t *testing.T) {
	f := validForm()
	due := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	f.CalibrationDue = &due

	if _, _, err := Evaluate(f, []float64{10, 50}, today); err != nil {
		t.Errorf("bitiş günü ölçüm kabul edilmeli: %v", err)
	}
	if _, _, err := Evaluate(f, []float64{10, 50}, today.Add(24*time.Hour)); !errors.Is(err, ErrCalibrationExpired) {
		t.Errorf("ertesi gün ErrCalibrationExpired beklenirdi, gelen %v", err)
	}
}

func TestEvaluate_WrongCount(t *testing.T) {
	if _, _, err := Evaluate(validForm(), []float64{10}, today); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ErrInvalid beklenirdi, gelen %v", err)
	}
}

// --- Servis ---

type fakeRepo struct {
	form  Form
	saved []Inspection
}

func (f *fakeRepo) CreateForm(_ context.Context, form Form, _ int64) (Form, error) { return form, nil }
func (f *fakeRepo) ListForms(context.Context) ([]Form, error)                      { return []Form{f.form}, nil }
func (f *fakeRepo) GetForm(_ context.Context, id int64) (Form, error) {
	if id != f.form.ID {
		return Form{}, ErrNotFound
	}
	return f.form, nil
}
func (f *fakeRepo) SaveInspection(_ context.Context, in Inspection) (Inspection, error) {
	in.ID = int64(len(f.saved) + 1)
	f.saved = append(f.saved, in)
	return in, nil
}
func (f *fakeRepo) RecentInspections(context.Context, int64, int) ([]Inspection, error) {
	return f.saved, nil
}

type fakeRecorder struct {
	err   error
	calls map[int64][]float64
}

func (f *fakeRecorder) AddMeasurement(_ context.Context, chartID int64, subgroup []float64) (spc.Result, error) {
	if f.err != nil {
		return spc.Result{}, f.err
	}
	f.calls[chartID] = append(f.calls[chartID], subgroup...)
	return spc.Result{Seq: int64(len(f.calls[chartID]))}, nil
}

func newService(t *testing.T, recorder *fakeRecorder) (*Service, *fakeRepo) {
	t.Helper()
	form, err := ValidateForm(validForm())
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepo{form: form}
	svc := NewService(repo, recorder)
	svc.now = func() time.Time { return today }
	return svc, repo
}

func TestInspect_SavesAndForwardsLinkedValuesToSPC(t *testing.T) {
	recorder := &fakeRecorder{calls: map[int64][]float64{}}
	svc, repo := newService(t, recorder)

	saved, outcomes, err := svc.Inspect(context.Background(), 1, []float64{10.1, 51.5}, nil, 5)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if saved.AllInSpec || len(repo.saved) != 1 || repo.saved[0].InspectedBy != 5 {
		t.Fatalf("muayene kaydedilmeli: %+v", saved)
	}
	if len(outcomes) != 1 || outcomes[0].ChartID != 7 || outcomes[0].Seq != 1 || recorder.calls[7][0] != 10.1 {
		t.Fatalf("yalnızca çap değeri 7 numaralı grafiğe gitmeli: %+v %+v", outcomes, recorder.calls)
	}
}

func TestInspect_SPCErrorDoesNotUndoInspection(t *testing.T) {
	svc, repo := newService(t, &fakeRecorder{err: errors.New("grafik kilitli"), calls: map[int64][]float64{}})

	_, outcomes, err := svc.Inspect(context.Background(), 1, []float64{10, 50}, nil, 5)
	if err != nil {
		t.Fatalf("SPC hatası isteği düşürmemeli: %v", err)
	}
	if len(repo.saved) != 1 || outcomes[0].Error == "" {
		t.Fatalf("muayene kalmalı ve hata sonuçta görünmeli: %+v", outcomes)
	}
}

func TestInspect_RejectsBeforeSaving(t *testing.T) {
	svc, repo := newService(t, &fakeRecorder{calls: map[int64][]float64{}})
	bad := int64(-1)

	if _, _, err := svc.Inspect(context.Background(), 1, []float64{10}, nil, 5); !errors.Is(err, ErrInvalid) {
		t.Errorf("eksik değer ErrInvalid vermeli: %v", err)
	}
	if _, _, err := svc.Inspect(context.Background(), 1, []float64{10, 50}, &bad, 5); !errors.Is(err, ErrInvalid) {
		t.Errorf("negatif iş emri ErrInvalid vermeli: %v", err)
	}
	if _, _, err := svc.Inspect(context.Background(), 99, []float64{10, 50}, nil, 5); !errors.Is(err, ErrNotFound) {
		t.Errorf("olmayan form ErrNotFound vermeli: %v", err)
	}
	if len(repo.saved) != 0 {
		t.Error("geçersiz muayene kaydedilmemeli")
	}
}
