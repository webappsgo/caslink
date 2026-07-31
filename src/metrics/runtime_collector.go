package metrics

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
)

// runtimeMetricsCollector exposes AI.md PART 21's spec-mandated, custom-named
// Go runtime metrics (go_goroutines, go_mem_alloc_bytes, go_mem_sys_bytes,
// go_gc_runs_total, go_gc_pause_total_seconds). These are distinct from the
// default collectors.NewGoCollector() metric names, so a dedicated collector
// is required. Values are read fresh on every scrape via runtime.ReadMemStats,
// so no background goroutine or periodic sampling is needed.
type runtimeMetricsCollector struct {
	goroutines   *prometheus.Desc
	memAlloc     *prometheus.Desc
	memSys       *prometheus.Desc
	gcRuns       *prometheus.Desc
	gcPauseTotal *prometheus.Desc
}

func newRuntimeMetricsCollector() *runtimeMetricsCollector {
	return &runtimeMetricsCollector{
		goroutines: prometheus.NewDesc(
			"caslink_go_goroutines", "Current number of goroutines.", nil, nil),
		memAlloc: prometheus.NewDesc(
			"caslink_go_mem_alloc_bytes", "Bytes of heap memory allocated and still in use.", nil, nil),
		memSys: prometheus.NewDesc(
			"caslink_go_mem_sys_bytes", "Total bytes of memory obtained from the OS.", nil, nil),
		gcRuns: prometheus.NewDesc(
			"caslink_go_gc_runs_total", "Total number of completed garbage collection cycles.", nil, nil),
		gcPauseTotal: prometheus.NewDesc(
			"caslink_go_gc_pause_total_seconds", "Cumulative time spent in garbage collection stop-the-world pauses.", nil, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *runtimeMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.goroutines
	ch <- c.memAlloc
	ch <- c.memSys
	ch <- c.gcRuns
	ch <- c.gcPauseTotal
}

// Collect implements prometheus.Collector.
func (c *runtimeMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	ch <- prometheus.MustNewConstMetric(c.goroutines, prometheus.GaugeValue, float64(runtime.NumGoroutine()))
	ch <- prometheus.MustNewConstMetric(c.memAlloc, prometheus.GaugeValue, float64(ms.HeapAlloc))
	ch <- prometheus.MustNewConstMetric(c.memSys, prometheus.GaugeValue, float64(ms.Sys))
	ch <- prometheus.MustNewConstMetric(c.gcRuns, prometheus.CounterValue, float64(ms.NumGC))
	ch <- prometheus.MustNewConstMetric(c.gcPauseTotal, prometheus.CounterValue, float64(ms.PauseTotalNs)/1e9)
}
