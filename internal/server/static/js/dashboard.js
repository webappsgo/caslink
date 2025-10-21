/**
 * Dashboard JavaScript for Caslink URL Shortener
 * Handles dashboard interactions, URL management, and real-time updates
 */

// Dashboard state management
const Dashboard = {
    urls: [],
    filteredUrls: [],
    currentPage: 1,
    itemsPerPage: 20,
    sortBy: 'created_at',
    sortOrder: 'desc',
    filterStatus: 'all',
    searchQuery: '',
    selectedUrls: new Set(),

    // Initialize dashboard
    init() {
        this.bindEvents();
        this.loadUrls();
        this.setupRealTimeUpdates();
        this.loadStats();
    },

    // Bind event listeners
    bindEvents() {
        // Search functionality
        const searchInput = document.getElementById('url-search');
        if (searchInput) {
            searchInput.addEventListener('input', window.Caslink.utils.debounce((e) => {
                this.searchQuery = e.target.value.toLowerCase();
                this.filterAndSort();
            }, 300));
        }

        // Filter buttons
        document.querySelectorAll('.filter-button').forEach(btn => {
            btn.addEventListener('click', (e) => {
                this.filterStatus = e.currentTarget.dataset.filter || 'all';
                document.querySelectorAll('.filter-button').forEach(b => b.classList.remove('active'));
                e.currentTarget.classList.add('active');
                this.filterAndSort();
            });
        });

        // Sort dropdown
        const sortButton = document.getElementById('sort-button');
        if (sortButton) {
            sortButton.addEventListener('click', () => {
                this.toggleSortMenu();
            });
        }

        document.querySelectorAll('.sort-option').forEach(option => {
            option.addEventListener('click', (e) => {
                const sortValue = e.currentTarget.dataset.sort;
                if (sortValue) {
                    const [field, order] = sortValue.split(':');
                    this.sortBy = field;
                    this.sortOrder = order || 'desc';
                    this.filterAndSort();
                }
            });
        });

        // Bulk action buttons
        document.getElementById('bulk-delete')?.addEventListener('click', () => {
            this.bulkDelete();
        });

        document.getElementById('bulk-export')?.addEventListener('click', () => {
            this.bulkExport();
        });

        document.getElementById('bulk-cancel')?.addEventListener('click', () => {
            this.clearSelection();
        });

        // Refresh button
        document.getElementById('refresh-urls')?.addEventListener('click', () => {
            this.loadUrls(true);
        });

        // Create new URL button
        document.getElementById('create-url')?.addEventListener('click', () => {
            window.location.href = '/';
        });
    },

    // Load URLs from API
    async loadUrls(showLoading = false) {
        if (showLoading) {
            this.showLoading();
        }

        try {
            const response = await window.Caslink.API.get('/urls');
            this.urls = response.urls || [];
            this.filterAndSort();
            this.renderUrls();
        } catch (error) {
            console.error('Failed to load URLs:', error);
            window.Caslink.utils.showFlashMessage('error', 'Failed to load URLs');
        } finally {
            if (showLoading) {
                this.hideLoading();
            }
        }
    },

    // Load dashboard statistics
    async loadStats() {
        try {
            const response = await window.Caslink.API.get('/stats');
            this.renderStats(response);
        } catch (error) {
            console.error('Failed to load stats:', error);
        }
    },

    // Filter and sort URLs
    filterAndSort() {
        let filtered = [...this.urls];

        // Apply search filter
        if (this.searchQuery) {
            filtered = filtered.filter(url =>
                url.short_code.toLowerCase().includes(this.searchQuery) ||
                url.original_url.toLowerCase().includes(this.searchQuery) ||
                (url.title && url.title.toLowerCase().includes(this.searchQuery))
            );
        }

        // Apply status filter
        if (this.filterStatus !== 'all') {
            filtered = filtered.filter(url => {
                switch (this.filterStatus) {
                    case 'active':
                        return url.active && (!url.expires_at || new Date(url.expires_at) > new Date());
                    case 'expired':
                        return url.expires_at && new Date(url.expires_at) <= new Date();
                    case 'inactive':
                        return !url.active;
                    default:
                        return true;
                }
            });
        }

        // Apply sorting
        filtered.sort((a, b) => {
            let aVal = a[this.sortBy];
            let bVal = b[this.sortBy];

            // Handle date sorting
            if (this.sortBy.includes('_at')) {
                aVal = new Date(aVal || 0).getTime();
                bVal = new Date(bVal || 0).getTime();
            }

            // Handle numeric sorting
            if (this.sortBy === 'clicks' || this.sortBy === 'unique_clicks') {
                aVal = parseInt(aVal) || 0;
                bVal = parseInt(bVal) || 0;
            }

            if (this.sortOrder === 'asc') {
                return aVal > bVal ? 1 : -1;
            } else {
                return aVal < bVal ? 1 : -1;
            }
        });

        this.filteredUrls = filtered;
        this.currentPage = 1;
        this.renderUrls();
    },

    // Render URLs list
    renderUrls() {
        const container = document.getElementById('url-list-body');
        if (!container) return;

        if (this.filteredUrls.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-state-icon">
                        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
                            <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
                        </svg>
                    </div>
                    <h3 class="empty-state-title">No URLs found</h3>
                    <p class="empty-state-description">
                        ${this.searchQuery || this.filterStatus !== 'all' ? 'Try adjusting your filters' : 'Create your first short URL to get started'}
                    </p>
                    ${this.searchQuery || this.filterStatus !== 'all' ? '' : '<a href="/" class="empty-state-action">Create URL</a>'}
                </div>
            `;
            return;
        }

        const start = (this.currentPage - 1) * this.itemsPerPage;
        const end = start + this.itemsPerPage;
        const pageUrls = this.filteredUrls.slice(start, end);

        container.innerHTML = pageUrls.map(url => this.renderUrlItem(url)).join('');

        // Bind event listeners for URL items
        this.bindUrlItemEvents();

        // Render pagination
        this.renderPagination();
    },

    // Render individual URL item
    renderUrlItem(url) {
        const shortUrl = `${window.location.origin}/${url.short_code}`;
        const isExpired = url.expires_at && new Date(url.expires_at) <= new Date();
        const isSelected = this.selectedUrls.has(url.id);

        return `
            <div class="url-item" data-url-id="${url.id}">
                <div class="url-short">
                    <input type="checkbox"
                           class="url-checkbox"
                           data-url-id="${url.id}"
                           ${isSelected ? 'checked' : ''}>
                    <a href="${shortUrl}"
                       class="url-short-code"
                       target="_blank"
                       rel="noopener noreferrer">
                        ${window.Caslink.utils.escapeHtml(url.short_code)}
                    </a>
                    <button class="url-copy-btn"
                            data-url="${shortUrl}"
                            data-tooltip="Copy to clipboard">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
                        </svg>
                    </button>
                </div>

                <div class="url-original">
                    <a href="${window.Caslink.utils.escapeHtml(url.original_url)}"
                       target="_blank"
                       rel="noopener noreferrer"
                       title="${window.Caslink.utils.escapeHtml(url.original_url)}">
                        ${window.Caslink.utils.escapeHtml(url.title || url.original_url)}
                    </a>
                    ${isExpired ? '<span class="text-error" style="margin-left: 8px;">(Expired)</span>' : ''}
                    ${!url.active ? '<span class="text-warning" style="margin-left: 8px;">(Inactive)</span>' : ''}
                </div>

                <div class="url-stats">
                    <svg class="url-stats-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
                    </svg>
                    ${window.Caslink.utils.formatNumber(url.clicks || 0)}
                </div>

                <div class="url-created">
                    ${window.Caslink.utils.formatDate(url.created_at)}
                </div>

                <div class="url-actions">
                    <button class="url-action-btn"
                            data-action="analytics"
                            data-url-id="${url.id}"
                            data-tooltip="Analytics">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <line x1="18" y1="20" x2="18" y2="10"/>
                            <line x1="12" y1="20" x2="12" y2="4"/>
                            <line x1="6" y1="20" x2="6" y2="14"/>
                        </svg>
                    </button>
                    <button class="url-action-btn"
                            data-action="qr"
                            data-url-id="${url.id}"
                            data-tooltip="QR Code">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <rect x="3" y="3" width="18" height="18" rx="2"/>
                            <rect x="7" y="7" width="3" height="3"/>
                            <rect x="14" y="7" width="3" height="3"/>
                            <rect x="7" y="14" width="3" height="3"/>
                            <rect x="14" y="14" width="3" height="3"/>
                        </svg>
                    </button>
                    <button class="url-action-btn"
                            data-action="edit"
                            data-url-id="${url.id}"
                            data-tooltip="Edit">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
                        </svg>
                    </button>
                    <button class="url-action-btn danger"
                            data-action="delete"
                            data-url-id="${url.id}"
                            data-tooltip="Delete">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="3 6 5 6 21 6"/>
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                        </svg>
                    </button>
                </div>
            </div>
        `;
    },

    // Bind events for URL items
    bindUrlItemEvents() {
        // Copy buttons
        document.querySelectorAll('.url-copy-btn').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                const url = e.currentTarget.dataset.url;
                const success = await window.Caslink.utils.copyToClipboard(url);
                if (success) {
                    window.Caslink.utils.showFlashMessage('success', 'URL copied to clipboard!');
                }
            });
        });

        // Checkboxes
        document.querySelectorAll('.url-checkbox').forEach(checkbox => {
            checkbox.addEventListener('change', (e) => {
                const urlId = e.currentTarget.dataset.urlId;
                if (e.currentTarget.checked) {
                    this.selectedUrls.add(urlId);
                } else {
                    this.selectedUrls.delete(urlId);
                }
                this.updateBulkActionsBar();
            });
        });

        // Action buttons
        document.querySelectorAll('.url-action-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const action = e.currentTarget.dataset.action;
                const urlId = e.currentTarget.dataset.urlId;
                this.handleUrlAction(action, urlId);
            });
        });
    },

    // Handle URL actions
    async handleUrlAction(action, urlId) {
        const url = this.urls.find(u => u.id === urlId);
        if (!url) return;

        switch (action) {
            case 'analytics':
                window.location.href = `/analytics/${url.short_code}`;
                break;
            case 'qr':
                window.location.href = `/qr/${url.short_code}`;
                break;
            case 'edit':
                window.location.href = `/url/${url.short_code}/edit`;
                break;
            case 'delete':
                await this.deleteUrl(urlId);
                break;
        }
    },

    // Delete single URL
    async deleteUrl(urlId) {
        if (!confirm('Are you sure you want to delete this URL? This action cannot be undone.')) {
            return;
        }

        try {
            await window.Caslink.API.delete(`/urls/${urlId}`);
            window.Caslink.utils.showFlashMessage('success', 'URL deleted successfully');
            this.loadUrls();
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to delete URL');
        }
    },

    // Bulk delete
    async bulkDelete() {
        if (this.selectedUrls.size === 0) return;

        if (!confirm(`Are you sure you want to delete ${this.selectedUrls.size} URL(s)? This action cannot be undone.`)) {
            return;
        }

        try {
            const promises = Array.from(this.selectedUrls).map(id =>
                window.Caslink.API.delete(`/urls/${id}`)
            );
            await Promise.all(promises);

            window.Caslink.utils.showFlashMessage('success', `${this.selectedUrls.size} URL(s) deleted successfully`);
            this.clearSelection();
            this.loadUrls();
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to delete URLs');
        }
    },

    // Bulk export
    async bulkExport() {
        if (this.selectedUrls.size === 0) return;

        const selectedUrlsData = this.urls.filter(u => this.selectedUrls.has(u.id));
        const csv = this.convertToCSV(selectedUrlsData);

        const blob = new Blob([csv], { type: 'text/csv' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `urls_${Date.now()}.csv`;
        a.click();
        URL.revokeObjectURL(url);

        window.Caslink.utils.showFlashMessage('success', 'URLs exported successfully');
    },

    // Convert URLs to CSV
    convertToCSV(urls) {
        const headers = ['Short Code', 'Original URL', 'Title', 'Clicks', 'Created At'];
        const rows = urls.map(url => [
            url.short_code,
            url.original_url,
            url.title || '',
            url.clicks || 0,
            url.created_at
        ]);

        return [
            headers.join(','),
            ...rows.map(row => row.map(cell => `"${cell}"`).join(','))
        ].join('\n');
    },

    // Clear selection
    clearSelection() {
        this.selectedUrls.clear();
        document.querySelectorAll('.url-checkbox').forEach(cb => cb.checked = false);
        this.updateBulkActionsBar();
    },

    // Update bulk actions bar
    updateBulkActionsBar() {
        const bar = document.getElementById('bulk-actions-bar');
        if (!bar) return;

        if (this.selectedUrls.size > 0) {
            bar.classList.add('visible');
            const info = bar.querySelector('.bulk-actions-info');
            if (info) {
                info.textContent = `${this.selectedUrls.size} URL(s) selected`;
            }
        } else {
            bar.classList.remove('visible');
        }
    },

    // Render statistics
    renderStats(stats) {
        if (stats.total_urls !== undefined) {
            const el = document.getElementById('stat-total-urls');
            if (el) el.textContent = window.Caslink.utils.formatNumber(stats.total_urls);
        }

        if (stats.total_clicks !== undefined) {
            const el = document.getElementById('stat-total-clicks');
            if (el) el.textContent = window.Caslink.utils.formatNumber(stats.total_clicks);
        }

        if (stats.unique_visitors !== undefined) {
            const el = document.getElementById('stat-unique-visitors');
            if (el) el.textContent = window.Caslink.utils.formatNumber(stats.unique_visitors);
        }

        if (stats.click_rate !== undefined) {
            const el = document.getElementById('stat-click-rate');
            if (el) el.textContent = stats.click_rate.toFixed(1) + '%';
        }
    },

    // Render pagination
    renderPagination() {
        const container = document.getElementById('pagination');
        if (!container) return;

        const totalPages = Math.ceil(this.filteredUrls.length / this.itemsPerPage);
        if (totalPages <= 1) {
            container.innerHTML = '';
            return;
        }

        let html = '<div class="pagination">';

        // Previous button
        html += `
            <button class="pagination-button"
                    ${this.currentPage === 1 ? 'disabled' : ''}
                    onclick="Dashboard.goToPage(${this.currentPage - 1})">
                Previous
            </button>
        `;

        // Page numbers
        for (let i = 1; i <= totalPages; i++) {
            if (i === 1 || i === totalPages || (i >= this.currentPage - 2 && i <= this.currentPage + 2)) {
                html += `
                    <button class="pagination-button ${i === this.currentPage ? 'active' : ''}"
                            onclick="Dashboard.goToPage(${i})">
                        ${i}
                    </button>
                `;
            } else if (i === this.currentPage - 3 || i === this.currentPage + 3) {
                html += '<span class="pagination-info">...</span>';
            }
        }

        // Next button
        html += `
            <button class="pagination-button"
                    ${this.currentPage === totalPages ? 'disabled' : ''}
                    onclick="Dashboard.goToPage(${this.currentPage + 1})">
                Next
            </button>
        `;

        html += '</div>';
        container.innerHTML = html;
    },

    // Go to specific page
    goToPage(page) {
        this.currentPage = page;
        this.renderUrls();
        window.scrollTo({ top: 0, behavior: 'smooth' });
    },

    // Toggle sort menu
    toggleSortMenu() {
        const menu = document.querySelector('.sort-dropdown .dropdown-menu');
        if (menu) {
            menu.classList.toggle('visible');
        }
    },

    // Setup real-time updates
    setupRealTimeUpdates() {
        // Poll for updates every 30 seconds
        setInterval(() => {
            this.loadUrls(false);
            this.loadStats();
        }, 30000);
    },

    // Show loading state
    showLoading() {
        const container = document.getElementById('url-list-body');
        if (container) {
            container.innerHTML = `
                <div class="analytics-loading">
                    <div class="analytics-loading-spinner"></div>
                    <div class="analytics-loading-text">Loading URLs...</div>
                </div>
            `;
        }
    },

    // Hide loading state
    hideLoading() {
        // Loading will be replaced by renderUrls()
    }
};

// Initialize dashboard when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    if (document.getElementById('dashboard-container')) {
        Dashboard.init();
    }
});

// Export for global access
window.Dashboard = Dashboard;
