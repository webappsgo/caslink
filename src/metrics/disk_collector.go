package metrics

import "github.com/prometheus/client_golang/prometheus"

// diskCollector exposes AI.md PART 21's system disk usage metrics
// (system_disk_usage_percent/_used_bytes/_total_bytes, labeled by path) for
// a single directory, gated on server.metrics.include_system. Values are
// sampled fresh on every scrape via the platform-specific diskCapacity helper.
type diskCollector struct {
	path         string
	usagePercent *prometheus.Desc
	usedBytes    *prometheus.Desc
	totalBytes   *prometheus.Desc
}

func newDiskCollector(path string) *diskCollector {
	labels := []string{"path"}
	return &diskCollector{
		path: path,
		usagePercent: prometheus.NewDesc(
			"caslink_system_disk_usage_percent", "Disk usage percentage for the given path.", labels, nil),
		usedBytes: prometheus.NewDesc(
			"caslink_system_disk_used_bytes", "Disk space used in bytes for the given path.", labels, nil),
		totalBytes: prometheus.NewDesc(
			"caslink_system_disk_total_bytes", "Total disk space in bytes for the given path.", labels, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *diskCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.usagePercent
	ch <- c.usedBytes
	ch <- c.totalBytes
}

// Collect implements prometheus.Collector. It silently skips the scrape round
// if the underlying filesystem stat call fails (e.g. path not yet created),
// rather than fabricating a misleading value.
func (c *diskCollector) Collect(ch chan<- prometheus.Metric) {
	total, free, err := diskCapacity(c.path)
	if err != nil || total == 0 {
		return
	}
	used := total - free
	pct := float64(used) / float64(total) * 100

	ch <- prometheus.MustNewConstMetric(c.usagePercent, prometheus.GaugeValue, pct, c.path)
	ch <- prometheus.MustNewConstMetric(c.usedBytes, prometheus.GaugeValue, float64(used), c.path)
	ch <- prometheus.MustNewConstMetric(c.totalBytes, prometheus.GaugeValue, float64(total), c.path)
}
