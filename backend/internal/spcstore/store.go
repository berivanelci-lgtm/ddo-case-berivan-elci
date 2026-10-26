// Package spcstore, SPC verilerini PostgreSQL'de saklar.
package spcstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/spc"
)

const uniqueViolation = "23505"

// Store, spc.Store ve spc.ChartRepository arayüzlerini PostgreSQL ile gerçekler.
type Store struct {
	pool *pgxpool.Pool
}

var (
	_ spc.Store           = (*Store)(nil)
	_ spc.ChartRepository = (*Store)(nil)
)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// querier, hem bağlantı havuzunun hem de açık bir işlemin ortak sorgu yöntemleridir.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// WithChartLock, grafiğin satırını kilitleyip fn'i tek bir veritabanı işlemi içinde çalıştırır.
// fn hata dönerse işlem geri alınır.
func (s *Store) WithChartLock(ctx context.Context, chartID int64, fn func(spc.ChartTx) error) error {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lockQuery = `
		SELECT id, name, chart_type, subgroup_size
		FROM charts
		WHERE id = $1
		FOR UPDATE`
	chart, err := scanChart(tx.QueryRow(ctx, lockQuery, chartID))
	if err != nil {
		return err
	}

	if err := fn(&chartTx{q: tx, chart: chart}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return nil
}

// CreateChart, yeni bir kontrol grafiği oluşturur.
func (s *Store) CreateChart(ctx context.Context, name string, chartType spc.ChartType, subgroupSize int, createdBy int64) (spc.Chart, error) {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return spc.Chart{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const query = `
		INSERT INTO charts (name, chart_type, subgroup_size, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id`

	chart := spc.Chart{Name: name, Type: chartType, SubgroupSize: subgroupSize}
	err = tx.QueryRow(ctx, query, name, string(chartType), subgroupSize, createdBy).Scan(&chart.ID)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return spc.Chart{}, spc.ErrChartExists
	}
	if err != nil {
		return spc.Chart{}, fmt.Errorf("grafik oluşturulamadı: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return spc.Chart{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return chart, nil
}

// GetChart, grafiği kimliğiyle getirir.
func (s *Store) GetChart(ctx context.Context, id int64) (spc.Chart, error) {
	const query = `SELECT id, name, chart_type, subgroup_size FROM charts WHERE id = $1`
	return scanChart(s.pool.QueryRow(ctx, query, id))
}

// ListCharts, tüm grafikleri kimlik sırasıyla getirir.
func (s *Store) ListCharts(ctx context.Context) ([]spc.Chart, error) {
	const query = `SELECT id, name, chart_type, subgroup_size FROM charts ORDER BY id`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("grafikler sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var charts []spc.Chart
	for rows.Next() {
		chart, err := scanChart(rows)
		if err != nil {
			return nil, err
		}
		charts = append(charts, chart)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("grafikler okunamadı: %w", err)
	}
	return charts, nil
}

// LatestLimits, grafiğin en son limit sürümünü döner; hiç yoksa nil döner.
func (s *Store) LatestLimits(ctx context.Context, chartID int64) (*spc.LimitSet, error) {
	return latestLimits(ctx, s.pool, chartID)
}

// RecentPoints, grafiğin son n noktasını eskiden yeniye sıralı döner.
func (s *Store) RecentPoints(ctx context.Context, chartID int64, n int) ([]spc.Point, error) {
	return recentPoints(ctx, s.pool, chartID, n)
}

// RecentAlarms, grafiğin son n alarmını yeniden eskiye sıralı döner.
func (s *Store) RecentAlarms(ctx context.Context, chartID int64, n int) ([]spc.Alarm, error) {
	const query = `
		SELECT point_seq, rule, point_value, limit_version
		FROM spc_alarms
		WHERE chart_id = $1
		ORDER BY id DESC
		LIMIT $2`

	rows, err := s.pool.Query(ctx, query, chartID, n)
	if err != nil {
		return nil, fmt.Errorf("alarmlar sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var alarms []spc.Alarm
	for rows.Next() {
		alarm := spc.Alarm{ChartID: chartID}
		var rule string
		if err := rows.Scan(&alarm.PointSeq, &rule, &alarm.Value, &alarm.LimitVersion); err != nil {
			return nil, fmt.Errorf("alarm okunamadı: %w", err)
		}
		alarm.Rule = spc.Rule(rule)
		alarms = append(alarms, alarm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alarmlar okunamadı: %w", err)
	}
	return alarms, nil
}

// chartTx, kilitlenmiş bir grafik üzerindeki işlemleri gerçekleştirir.
type chartTx struct {
	q     querier
	chart spc.Chart
}

var _ spc.ChartTx = (*chartTx)(nil)

func (t *chartTx) Chart() spc.Chart { return t.chart }

func (t *chartTx) PointCount(ctx context.Context) (int64, error) {
	var count int64
	err := t.q.QueryRow(ctx, `SELECT count(*) FROM spc_points WHERE chart_id = $1`, t.chart.ID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("nokta sayısı alınamadı: %w", err)
	}
	return count, nil
}

func (t *chartTx) FirstPoints(ctx context.Context, n int) ([]spc.Point, error) {
	const query = `
		SELECT seq, subgroup, statistic
		FROM spc_points
		WHERE chart_id = $1
		ORDER BY seq ASC
		LIMIT $2`
	return queryPoints(ctx, t.q, query, t.chart.ID, n)
}

func (t *chartTx) RecentPoints(ctx context.Context, n int) ([]spc.Point, error) {
	return recentPoints(ctx, t.q, t.chart.ID, n)
}

func (t *chartTx) CurrentLimits(ctx context.Context) (*spc.LimitSet, error) {
	return latestLimits(ctx, t.q, t.chart.ID)
}

func (t *chartTx) AddPoint(ctx context.Context, p spc.Point) error {
	const query = `
		INSERT INTO spc_points (chart_id, seq, subgroup, statistic)
		VALUES ($1, $2, $3, $4)`
	if _, err := t.q.Exec(ctx, query, t.chart.ID, p.Seq, p.Subgroup, p.Statistic); err != nil {
		return fmt.Errorf("nokta eklenemedi: %w", err)
	}
	return nil
}

func (t *chartTx) SaveLimits(ctx context.Context, set spc.LimitSet) error {
	const query = `
		INSERT INTO spc_limit_sets
			(chart_id, version, center, sigma, ucl, lcl, range_center, range_ucl, range_lcl)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	var rangeCenter, rangeUCL, rangeLCL *float64
	if set.Range != nil {
		rangeCenter, rangeUCL, rangeLCL = &set.Range.Center, &set.Range.UCL, &set.Range.LCL
	}

	_, err := t.q.Exec(ctx, query, t.chart.ID, set.Version,
		set.Mean.Center, set.Mean.Sigma, set.Mean.UCL, set.Mean.LCL,
		rangeCenter, rangeUCL, rangeLCL)
	if err != nil {
		return fmt.Errorf("limitler kaydedilemedi: %w", err)
	}
	return nil
}

// AddAlarms, alarmları ekler. Aynı noktada aynı kural için alarm zaten varsa
// veritabanı kısıtı sayesinde ikinci kez eklenmez.
func (t *chartTx) AddAlarms(ctx context.Context, alarms []spc.Alarm) error {
	const query = `
		INSERT INTO spc_alarms (chart_id, point_seq, rule, point_value, limit_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (chart_id, point_seq, rule) DO NOTHING`

	for _, alarm := range alarms {
		_, err := t.q.Exec(ctx, query, t.chart.ID, alarm.PointSeq, string(alarm.Rule), alarm.Value, alarm.LimitVersion)
		if err != nil {
			return fmt.Errorf("alarm eklenemedi: %w", err)
		}
	}
	return nil
}

// scanner, tek bir satırı okuyabilen pgx.Row ve pgx.Rows için ortak yöntemdir.
type scanner interface {
	Scan(dest ...any) error
}

func scanChart(row scanner) (spc.Chart, error) {
	var (
		chart     spc.Chart
		chartType string
	)
	err := row.Scan(&chart.ID, &chart.Name, &chartType, &chart.SubgroupSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return spc.Chart{}, spc.ErrChartNotFound
	}
	if err != nil {
		return spc.Chart{}, fmt.Errorf("grafik okunamadı: %w", err)
	}
	chart.Type = spc.ChartType(chartType)
	return chart, nil
}

func latestLimits(ctx context.Context, q querier, chartID int64) (*spc.LimitSet, error) {
	const query = `
		SELECT version, center, sigma, ucl, lcl, range_center, range_ucl, range_lcl
		FROM spc_limit_sets
		WHERE chart_id = $1
		ORDER BY version DESC
		LIMIT 1`

	var (
		set                             spc.LimitSet
		rangeCenter, rangeUCL, rangeLCL *float64
	)
	err := q.QueryRow(ctx, query, chartID).Scan(
		&set.Version, &set.Mean.Center, &set.Mean.Sigma, &set.Mean.UCL, &set.Mean.LCL,
		&rangeCenter, &rangeUCL, &rangeLCL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("limitler okunamadı: %w", err)
	}

	if rangeCenter != nil && rangeUCL != nil && rangeLCL != nil {
		set.Range = &spc.RangeLimits{Center: *rangeCenter, UCL: *rangeUCL, LCL: *rangeLCL}
	}
	return &set, nil
}

func recentPoints(ctx context.Context, q querier, chartID int64, n int) ([]spc.Point, error) {
	const query = `
		SELECT seq, subgroup, statistic
		FROM spc_points
		WHERE chart_id = $1
		ORDER BY seq DESC
		LIMIT $2`

	points, err := queryPoints(ctx, q, query, chartID, n)
	if err != nil {
		return nil, err
	}
	// Sorgu yeniden eskiye sıraladı; çağıranlar eskiden yeniye bekliyor.
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
	return points, nil
}

func queryPoints(ctx context.Context, q querier, query string, args ...any) ([]spc.Point, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("noktalar sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var points []spc.Point
	for rows.Next() {
		var p spc.Point
		if err := rows.Scan(&p.Seq, &p.Subgroup, &p.Statistic); err != nil {
			return nil, fmt.Errorf("nokta okunamadı: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("noktalar okunamadı: %w", err)
	}
	return points, nil
}
