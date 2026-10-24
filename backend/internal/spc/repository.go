package spc

import (
	"context"
	"errors"
)

// ErrChartExists, aynı adla bir grafik zaten varsa döner.
var ErrChartExists = errors.New("bu adla bir kontrol grafiği zaten var")

// ChartRepository, grafiklerin tanımını ve sonuçlarını okuma/oluşturma işlemleridir.
type ChartRepository interface {
	CreateChart(ctx context.Context, name string, chartType ChartType, subgroupSize int, createdBy int64) (Chart, error)
	GetChart(ctx context.Context, id int64) (Chart, error)
	ListCharts(ctx context.Context) ([]Chart, error)
	// LatestLimits, en son limit sürümünü döner; hiç yoksa nil döner.
	LatestLimits(ctx context.Context, chartID int64) (*LimitSet, error)
	// RecentPoints noktaları eskiden yeniye sıralı döner.
	RecentPoints(ctx context.Context, chartID int64, n int) ([]Point, error)
	// RecentAlarms alarmları yeniden eskiye sıralı döner.
	RecentAlarms(ctx context.Context, chartID int64, n int) ([]Alarm, error)
}
