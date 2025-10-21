/**
 * Analytics JavaScript for Caslink URL Shortener
 * Handles analytics data visualization using Chart.js
 */

const Analytics = {
    charts: {},
    currentUrl: null,
    dateRange: {
        start: null,
        end: null,
        preset: '7d'
    },

    // Initialize analytics
    init(urlId) {
        this.currentUrl = urlId;
        this.setupDateRange();
        this.bindEvents();
        this.loadAnalytics();
    },

    // Setup initial date range
    setupDateRange() {
        const end = new Date();
        const start = new Date();
        start.setDate(start.getDate() - 7); // Default to last 7 days

        this.dateRange.start = start;
        this.dateRange.end = end;

        // Set date inputs
        const startInput = document.getElementById('date-start');
        const endInput = document.getElementById('date-end');

        if (startInput) startInput.valueAsDate = start;
        if (endInput) endInput.valueAsDate = end;
    },

    // Bind event listeners
    bindEvents() {
        // Date preset buttons
        document.querySelectorAll('.date-preset-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const preset = e.currentTarget.dataset.preset;
                this.setDatePreset(preset);
            });
        });

        // Custom date range
        document.getElementById('date-apply')?.addEventListener('click', () => {
            this.applyCustomDateRange();
        });

        // Export buttons
        document.querySelectorAll('.export-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const format = e.currentTarget.dataset.format;
                this.exportData(format);
            });
        });

        // Refresh button
        document.getElementById('refresh-analytics')?.addEventListener('click', () => {
            this.loadAnalytics(true);
        });
    },

    // Set date preset
    setDatePreset(preset) {
        document.querySelectorAll('.date-preset-btn').forEach(b => b.classList.remove('active'));
        document.querySelector(`[data-preset="${preset}"]`)?.classList.add('active');

        const end = new Date();
        const start = new Date();

        switch (preset) {
            case '24h':
                start.setHours(start.getHours() - 24);
                break;
            case '7d':
                start.setDate(start.getDate() - 7);
                break;
            case '30d':
                start.setDate(start.getDate() - 30);
                break;
            case '90d':
                start.setDate(start.getDate() - 90);
                break;
            case 'all':
                start.setFullYear(2000); // Far back enough
                break;
        }

        this.dateRange.start = start;
        this.dateRange.end = end;
        this.dateRange.preset = preset;

        this.loadAnalytics();
    },

    // Apply custom date range
    applyCustomDateRange() {
        const startInput = document.getElementById('date-start');
        const endInput = document.getElementById('date-end');

        if (startInput && endInput) {
            this.dateRange.start = new Date(startInput.value);
            this.dateRange.end = new Date(endInput.value);
            this.dateRange.preset = 'custom';

            document.querySelectorAll('.date-preset-btn').forEach(b => b.classList.remove('active'));

            this.loadAnalytics();
        }
    },

    // Load analytics data
    async loadAnalytics(showLoading = false) {
        if (showLoading) {
            this.showLoading();
        }

        try {
            const endpoint = this.currentUrl
                ? `/analytics/${this.currentUrl}?start=${this.dateRange.start.toISOString()}&end=${this.dateRange.end.toISOString()}`
                : `/analytics/overview?start=${this.dateRange.start.toISOString()}&end=${this.dateRange.end.toISOString()}`;

            const data = await window.Caslink.API.get(endpoint);

            this.renderMetrics(data.metrics);
            this.renderTimelineChart(data.timeline);
            this.renderDeviceBreakdown(data.devices);
            this.renderTopCountries(data.countries);
            this.renderTopBrowsers(data.browsers);
            this.renderTopReferrers(data.referrers);

            if (data.realtime) {
                this.startRealTimeFeed(data.realtime);
            }
        } catch (error) {
            console.error('Failed to load analytics:', error);
            window.Caslink.utils.showFlashMessage('error', 'Failed to load analytics data');
        } finally {
            if (showLoading) {
                this.hideLoading();
            }
        }
    },

    // Render metrics cards
    renderMetrics(metrics) {
        if (!metrics) return;

        const metricElements = {
            total_clicks: 'metric-total-clicks',
            unique_clicks: 'metric-unique-clicks',
            avg_daily: 'metric-avg-daily',
            click_rate: 'metric-click-rate'
        };

        Object.entries(metricElements).forEach(([key, id]) => {
            const el = document.getElementById(id);
            if (el && metrics[key] !== undefined) {
                const value = key === 'click_rate'
                    ? metrics[key].toFixed(1) + '%'
                    : window.Caslink.utils.formatNumber(metrics[key]);
                el.textContent = value;
            }
        });

        // Render trend indicators
        if (metrics.trends) {
            this.renderTrends(metrics.trends);
        }
    },

    // Render trend indicators
    renderTrends(trends) {
        Object.entries(trends).forEach(([metric, trend]) => {
            const el = document.getElementById(`trend-${metric}`);
            if (!el) return;

            const icon = trend.change >= 0 ? '↑' : '↓';
            const className = trend.change >= 0 ? 'up' : 'down';

            el.className = `metric-trend ${className}`;
            el.innerHTML = `
                <span>${icon} ${Math.abs(trend.change).toFixed(1)}%</span>
                <span>vs previous period</span>
            `;
        });
    },

    // Render timeline chart
    renderTimelineChart(timelineData) {
        const canvas = document.getElementById('timeline-chart');
        if (!canvas || !timelineData) return;

        // Destroy existing chart
        if (this.charts.timeline) {
            this.charts.timeline.destroy();
        }

        const ctx = canvas.getContext('2d');

        this.charts.timeline = new Chart(ctx, {
            type: 'line',
            data: {
                labels: timelineData.labels,
                datasets: [
                    {
                        label: 'Total Clicks',
                        data: timelineData.clicks,
                        borderColor: 'rgb(59, 130, 246)',
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        fill: true,
                        tension: 0.4
                    },
                    {
                        label: 'Unique Clicks',
                        data: timelineData.unique_clicks,
                        borderColor: 'rgb(16, 185, 129)',
                        backgroundColor: 'rgba(16, 185, 129, 0.1)',
                        fill: true,
                        tension: 0.4
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        display: true,
                        position: 'bottom'
                    },
                    tooltip: {
                        mode: 'index',
                        intersect: false
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true,
                        ticks: {
                            precision: 0
                        }
                    }
                },
                interaction: {
                    mode: 'nearest',
                    axis: 'x',
                    intersect: false
                }
            }
        });
    },

    // Render device breakdown
    renderDeviceBreakdown(devices) {
        const container = document.getElementById('device-breakdown');
        if (!container || !devices) return;

        const total = devices.reduce((sum, d) => sum + d.count, 0);

        container.innerHTML = devices.map(device => `
            <div class="device-card">
                <div class="device-icon">
                    ${this.getDeviceIcon(device.type)}
                </div>
                <div class="device-name">${device.type}</div>
                <div class="device-percentage">${((device.count / total) * 100).toFixed(1)}%</div>
            </div>
        `).join('');
    },

    // Get device icon SVG
    getDeviceIcon(type) {
        const icons = {
            'desktop': '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>',
            'mobile': '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="2" width="14" height="20" rx="2"/><line x1="12" y1="18" x2="12.01" y2="18"/></svg>',
            'tablet': '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="2" width="16" height="20" rx="2"/><line x1="12" y1="18" x2="12.01" y2="18"/></svg>',
            'other': '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>'
        };
        return icons[type.toLowerCase()] || icons.other;
    },

    // Render top countries list
    renderTopCountries(countries) {
        this.renderTopList('top-countries', countries, (country, index) => `
            <li class="top-list-item">
                <div class="top-list-item-info">
                    <span class="top-list-item-rank ${index < 3 ? `top-${index + 1}` : ''}">${index + 1}</span>
                    <div class="top-list-item-icon">
                        ${country.code ? `<img src="/flags/${country.code.toLowerCase()}.svg" alt="${country.name}" style="width: 20px;">` : '🌍'}
                    </div>
                    <span class="top-list-item-label">${country.name}</span>
                </div>
                <span class="top-list-item-count">${window.Caslink.utils.formatNumber(country.count)}</span>
            </li>
        `);
    },

    // Render top browsers list
    renderTopBrowsers(browsers) {
        this.renderTopList('top-browsers', browsers, (browser, index) => `
            <li class="top-list-item">
                <div class="top-list-item-info">
                    <span class="top-list-item-rank ${index < 3 ? `top-${index + 1}` : ''}">${index + 1}</span>
                    <div class="top-list-item-icon">
                        ${this.getBrowserIcon(browser.name)}
                    </div>
                    <span class="top-list-item-label">${browser.name}</span>
                </div>
                <span class="top-list-item-count">${window.Caslink.utils.formatNumber(browser.count)}</span>
            </li>
        `);
    },

    // Render top referrers list
    renderTopReferrers(referrers) {
        this.renderTopList('top-referrers', referrers, (referrer, index) => `
            <li class="top-list-item">
                <div class="top-list-item-info">
                    <span class="top-list-item-rank ${index < 3 ? `top-${index + 1}` : ''}">${index + 1}</span>
                    <div class="top-list-item-icon">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
                        </svg>
                    </div>
                    <span class="top-list-item-label">${referrer.domain || 'Direct'}</span>
                </div>
                <span class="top-list-item-count">${window.Caslink.utils.formatNumber(referrer.count)}</span>
            </li>
        `);
    },

    // Generic top list renderer
    renderTopList(containerId, items, renderFn) {
        const container = document.getElementById(containerId);
        if (!container || !items) return;

        if (items.length === 0) {
            container.innerHTML = '<p class="text-secondary text-center">No data available</p>';
            return;
        }

        container.innerHTML = items.slice(0, 10).map((item, index) => renderFn(item, index)).join('');
    },

    // Get browser icon
    getBrowserIcon(browser) {
        return '🌐'; // Simplified, could use actual browser icons
    },

    // Start real-time feed
    startRealTimeFeed(initialData) {
        const container = document.getElementById('realtime-items');
        if (!container) return;

        // Render initial data
        if (initialData && initialData.length > 0) {
            this.renderRealTimeItems(initialData);
        }

        // Poll for new clicks every 5 seconds
        setInterval(async () => {
            try {
                const data = await window.Caslink.API.get(`/analytics/${this.currentUrl}/realtime`);
                if (data.clicks && data.clicks.length > 0) {
                    this.addRealTimeItems(data.clicks);
                }
            } catch (error) {
                console.error('Failed to fetch real-time data:', error);
            }
        }, 5000);
    },

    // Render real-time items
    renderRealTimeItems(items) {
        const container = document.getElementById('realtime-items');
        if (!container) return;

        container.innerHTML = items.map(click => this.renderRealTimeItem(click)).join('');
    },

    // Add new real-time items
    addRealTimeItems(items) {
        const container = document.getElementById('realtime-items');
        if (!container) return;

        items.forEach(click => {
            const item = document.createElement('div');
            item.innerHTML = this.renderRealTimeItem(click);
            container.insertBefore(item.firstChild, container.firstChild);

            // Remove old items if more than 50
            while (container.children.length > 50) {
                container.removeChild(container.lastChild);
            }
        });
    },

    // Render single real-time item
    renderRealTimeItem(click) {
        const timeAgo = this.getTimeAgo(new Date(click.clicked_at));

        return `
            <div class="realtime-item">
                <div class="realtime-item-time">${timeAgo}</div>
                <div class="realtime-item-details">
                    <div class="realtime-item-url">${click.short_code}</div>
                    <div class="realtime-item-location">
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/>
                            <circle cx="12" cy="10" r="3"/>
                        </svg>
                        ${click.city ? `${click.city}, ${click.country}` : click.country || 'Unknown'}
                        •
                        ${click.browser || 'Unknown'} on ${click.device || 'Unknown'}
                    </div>
                </div>
            </div>
        `;
    },

    // Get time ago string
    getTimeAgo(date) {
        const seconds = Math.floor((new Date() - date) / 1000);

        if (seconds < 60) return 'Just now';
        if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
        if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
        return `${Math.floor(seconds / 86400)}d ago`;
    },

    // Export data
    async exportData(format) {
        try {
            const endpoint = this.currentUrl
                ? `/analytics/${this.currentUrl}/export?format=${format}&start=${this.dateRange.start.toISOString()}&end=${this.dateRange.end.toISOString()}`
                : `/analytics/export?format=${format}&start=${this.dateRange.start.toISOString()}&end=${this.dateRange.end.toISOString()}`;

            // Trigger download
            window.location.href = endpoint;

            window.Caslink.utils.showFlashMessage('success', 'Export started');
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to export data');
        }
    },

    // Show loading state
    showLoading() {
        const container = document.querySelector('.analytics-container');
        if (container) {
            const overlay = document.createElement('div');
            overlay.className = 'analytics-loading';
            overlay.innerHTML = `
                <div class="analytics-loading-spinner"></div>
                <div class="analytics-loading-text">Loading analytics...</div>
            `;
            container.appendChild(overlay);
        }
    },

    // Hide loading state
    hideLoading() {
        const overlay = document.querySelector('.analytics-loading');
        if (overlay) {
            overlay.remove();
        }
    }
};

// Initialize analytics when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    const analyticsContainer = document.getElementById('analytics-container');
    if (analyticsContainer) {
        const urlId = analyticsContainer.dataset.urlId;
        Analytics.init(urlId);
    }
});

// Export for global access
window.Analytics = Analytics;
