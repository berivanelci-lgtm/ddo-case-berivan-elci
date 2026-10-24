package spc

// ChartType, kontrol grafiğinin türünü belirtir.
type ChartType string

const (
	// ChartIndividual: her ölçüm tek başına bir nokta (I-MR).
	ChartIndividual ChartType = "individual"
	// ChartXbarR: ölçümler alt gruplar halinde gelir, grafik grup ortalamasını izler.
	ChartXbarR ChartType = "xbar_r"
)

// Valid, grafik türünün tanımlı türlerden biri olup olmadığını söyler.
func (t ChartType) Valid() bool {
	return t == ChartIndividual || t == ChartXbarR
}

// Chart, bir kontrol grafiğinin tanımıdır.
type Chart struct {
	ID           int64
	Name         string
	Type         ChartType
	SubgroupSize int // bireysel grafikte 1
}

// Point, grafikteki bir noktadır. Statistic, grafikte çizilen değerdir
// (bireysel grafikte ölçümün kendisi, X̄-R grafiğinde grup ortalaması).
type Point struct {
	Seq       int64
	Subgroup  []float64
	Statistic float64
}

// LimitSet, bir grafiğin sürümlenmiş kontrol limitleridir.
type LimitSet struct {
	Version int
	Mean    Limits
	Range   *RangeLimits // sadece X̄-R grafiğinde dolu
}

// Alarm, bir kural ihlalinin kaydıdır.
type Alarm struct {
	ChartID      int64
	PointSeq     int64
	Rule         Rule
	Value        float64
	LimitVersion int
}
