/**
 * Admin Interface JavaScript for Caslink URL Shortener
 * Handles admin panel functionality, user management, and server configuration
 */

const Admin = {
    currentSection: 'dashboard',

    // Initialize admin interface
    init() {
        this.bindEvents();
        this.loadDashboardData();
        this.setupAutoRefresh();
    },

    // Bind event listeners
    bindEvents() {
        // Settings form submission
        document.getElementById('settings-form')?.addEventListener('submit', (e) => {
            e.preventDefault();
            this.saveSettings();
        });

        // Database connection test
        document.getElementById('test-connection')?.addEventListener('click', () => {
            this.testDatabaseConnection();
        });

        // Run migrations
        document.getElementById('run-migrations')?.addEventListener('click', () => {
            this.runMigrations();
        });

        // User management actions
        document.querySelectorAll('.user-action-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const action = e.currentTarget.dataset.action;
                const userId = e.currentTarget.dataset.userId;
                this.handleUserAction(action, userId);
            });
        });

        // Backup database
        document.getElementById('backup-database')?.addEventListener('click', () => {
            this.backupDatabase();
        });

        // Clear cache
        document.getElementById('clear-cache')?.addEventListener('click', () => {
            this.clearCache();
        });

        // Export logs
        document.getElementById('export-logs')?.addEventListener('click', () => {
            this.exportLogs();
        });

        // Toggle switches
        document.querySelectorAll('.toggle-input').forEach(toggle => {
            toggle.addEventListener('change', (e) => {
                const setting = e.currentTarget.dataset.setting;
                this.updateSetting(setting, e.currentTarget.checked);
            });
        });

        // Code copy buttons
        document.querySelectorAll('.code-copy-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const code = e.currentTarget.previousElementSibling.textContent;
                window.Caslink.utils.copyToClipboard(code);
                window.Caslink.utils.showFlashMessage('success', 'Copied to clipboard');
            });
        });
    },

    // Load dashboard data
    async loadDashboardData() {
        try {
            const data = await window.Caslink.API.get('/admin/dashboard');

            // Update system stats
            this.updateSystemStats(data.stats);

            // Update activity log
            this.updateActivityLog(data.activity);

            // Update health status
            this.updateHealthStatus(data.health);
        } catch (error) {
            console.error('Failed to load dashboard data:', error);
            window.Caslink.utils.showFlashMessage('error', 'Failed to load dashboard data');
        }
    },

    // Update system stats
    updateSystemStats(stats) {
        if (!stats) return;

        const statElements = {
            total_users: 'stat-total-users',
            total_urls: 'stat-total-urls',
            total_clicks: 'stat-total-clicks',
            storage_used: 'stat-storage-used',
            uptime: 'stat-uptime',
            memory_usage: 'stat-memory-usage'
        };

        Object.entries(statElements).forEach(([key, id]) => {
            const el = document.getElementById(id);
            if (el && stats[key] !== undefined) {
                let value = stats[key];

                if (key === 'storage_used') {
                    value = window.Caslink.utils.formatFileSize(value);
                } else if (key === 'memory_usage') {
                    value = value.toFixed(1) + '%';
                } else if (key === 'uptime') {
                    value = this.formatUptime(value);
                } else {
                    value = window.Caslink.utils.formatNumber(value);
                }

                el.textContent = value;
            }
        });
    },

    // Format uptime
    formatUptime(seconds) {
        const days = Math.floor(seconds / 86400);
        const hours = Math.floor((seconds % 86400) / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);

        if (days > 0) {
            return `${days}d ${hours}h`;
        } else if (hours > 0) {
            return `${hours}h ${minutes}m`;
        } else {
            return `${minutes}m`;
        }
    },

    // Update activity log
    updateActivityLog(activities) {
        const container = document.getElementById('activity-log');
        if (!container || !activities) return;

        container.innerHTML = activities.map(activity => `
            <div class="activity-item">
                <div class="activity-icon ${activity.type || 'info'}">
                    ${this.getActivityIcon(activity.type)}
                </div>
                <div class="activity-content">
                    <div class="activity-message">${window.Caslink.utils.escapeHtml(activity.message)}</div>
                    <div class="activity-meta">
                        <span>${window.Caslink.utils.formatDate(activity.timestamp)}</span>
                        ${activity.user ? `<span>by ${window.Caslink.utils.escapeHtml(activity.user)}</span>` : ''}
                        ${activity.ip ? `<span>from ${activity.ip}</span>` : ''}
                    </div>
                </div>
            </div>
        `).join('');
    },

    // Get activity icon
    getActivityIcon(type) {
        const icons = {
            success: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>',
            error: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>',
            warning: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
            info: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>'
        };
        return icons[type] || icons.info;
    },

    // Update health status
    updateHealthStatus(health) {
        const container = document.getElementById('health-status');
        if (!container || !health) return;

        const statusClass = health.status === 'healthy' ? 'success' : health.status === 'degraded' ? 'warning' : 'error';

        container.innerHTML = `
            <div class="admin-alert ${statusClass}">
                <div class="admin-alert-icon">
                    ${this.getActivityIcon(statusClass)}
                </div>
                <div class="admin-alert-content">
                    <div class="admin-alert-title">System Status: ${health.status.toUpperCase()}</div>
                    <div class="admin-alert-message">
                        ${health.message || 'All systems operational'}
                    </div>
                </div>
            </div>
        `;
    },

    // Save settings
    async saveSettings() {
        const form = document.getElementById('settings-form');
        if (!form) return;

        const formData = new FormData(form);
        const settings = {};

        for (const [key, value] of formData.entries()) {
            settings[key] = value;
        }

        try {
            await window.Caslink.API.post('/admin/settings', settings);
            window.Caslink.utils.showFlashMessage('success', 'Settings saved successfully');
        } catch (error) {
            console.error('Failed to save settings:', error);
            window.Caslink.utils.showFlashMessage('error', 'Failed to save settings');
        }
    },

    // Test database connection
    async testDatabaseConnection() {
        const button = document.getElementById('test-connection');
        const statusEl = document.getElementById('connection-status');

        if (button) {
            button.disabled = true;
            button.textContent = 'Testing...';
        }

        if (statusEl) {
            statusEl.innerHTML = `
                <div class="connection-indicator testing"></div>
                <span class="connection-message">Testing connection...</span>
            `;
        }

        try {
            const result = await window.Caslink.API.post('/admin/database/test');

            if (statusEl) {
                const indicator = result.success ? 'success' : 'error';
                statusEl.innerHTML = `
                    <div class="connection-indicator ${indicator}"></div>
                    <span class="connection-message">${result.message}</span>
                `;
            }

            if (result.details) {
                const detailsEl = document.getElementById('connection-details');
                if (detailsEl) {
                    detailsEl.textContent = JSON.stringify(result.details, null, 2);
                }
            }

            window.Caslink.utils.showFlashMessage(
                result.success ? 'success' : 'error',
                result.message
            );
        } catch (error) {
            if (statusEl) {
                statusEl.innerHTML = `
                    <div class="connection-indicator error"></div>
                    <span class="connection-message">Connection failed</span>
                `;
            }
            window.Caslink.utils.showFlashMessage('error', 'Database connection test failed');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = 'Test Connection';
            }
        }
    },

    // Run migrations
    async runMigrations() {
        if (!confirm('Are you sure you want to run pending migrations? This will modify the database schema.')) {
            return;
        }

        const button = document.getElementById('run-migrations');
        if (button) {
            button.disabled = true;
            button.textContent = 'Running...';
        }

        try {
            const result = await window.Caslink.API.post('/admin/migrations/run');

            window.Caslink.utils.showFlashMessage('success', `Successfully ran ${result.count} migration(s)`);

            // Reload migration status
            this.loadMigrationStatus();
        } catch (error) {
            console.error('Migration error:', error);
            window.Caslink.utils.showFlashMessage('error', 'Failed to run migrations');
        } finally {
            if (button) {
                button.disabled = false;
                button.textContent = 'Run Migrations';
            }
        }
    },

    // Load migration status
    async loadMigrationStatus() {
        try {
            const migrations = await window.Caslink.API.get('/admin/migrations');
            this.renderMigrations(migrations);
        } catch (error) {
            console.error('Failed to load migrations:', error);
        }
    },

    // Render migrations list
    renderMigrations(migrations) {
        const container = document.getElementById('migration-list');
        if (!container || !migrations) return;

        container.innerHTML = migrations.map(migration => `
            <li class="migration-item">
                <div class="migration-info">
                    <div class="migration-name">${migration.name}</div>
                    <div class="migration-description">${migration.description}</div>
                </div>
                <div class="migration-status">
                    <span class="migration-badge ${migration.applied ? 'applied' : 'pending'}">
                        ${migration.applied ? 'Applied' : 'Pending'}
                    </span>
                    ${migration.applied_at ? `<span class="text-secondary">${window.Caslink.utils.formatDate(migration.applied_at)}</span>` : ''}
                </div>
            </li>
        `).join('');
    },

    // Handle user action
    async handleUserAction(action, userId) {
        const user = await this.getUser(userId);
        if (!user) return;

        switch (action) {
            case 'edit':
                this.editUser(user);
                break;
            case 'delete':
                this.deleteUser(userId);
                break;
            case 'toggle-admin':
                this.toggleUserAdmin(userId);
                break;
            case 'reset-password':
                this.resetUserPassword(userId);
                break;
        }
    },

    // Get user details
    async getUser(userId) {
        try {
            return await window.Caslink.API.get(`/admin/users/${userId}`);
        } catch (error) {
            console.error('Failed to get user:', error);
            return null;
        }
    },

    // Delete user
    async deleteUser(userId) {
        if (!confirm('Are you sure you want to delete this user? This action cannot be undone.')) {
            return;
        }

        try {
            await window.Caslink.API.delete(`/admin/users/${userId}`);
            window.Caslink.utils.showFlashMessage('success', 'User deleted successfully');
            // Reload user list
            window.location.reload();
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to delete user');
        }
    },

    // Toggle user admin status
    async toggleUserAdmin(userId) {
        try {
            await window.Caslink.API.post(`/admin/users/${userId}/toggle-admin`);
            window.Caslink.utils.showFlashMessage('success', 'User role updated');
            window.location.reload();
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to update user role');
        }
    },

    // Reset user password
    async resetUserPassword(userId) {
        const newPassword = prompt('Enter new password for user:');
        if (!newPassword) return;

        try {
            await window.Caslink.API.post(`/admin/users/${userId}/reset-password`, {
                password: newPassword
            });
            window.Caslink.utils.showFlashMessage('success', 'Password reset successfully');
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to reset password');
        }
    },

    // Backup database
    async backupDatabase() {
        try {
            window.location.href = '/admin/database/backup';
            window.Caslink.utils.showFlashMessage('success', 'Database backup started');
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to backup database');
        }
    },

    // Clear cache
    async clearCache() {
        if (!confirm('Are you sure you want to clear the cache?')) {
            return;
        }

        try {
            await window.Caslink.API.post('/admin/cache/clear');
            window.Caslink.utils.showFlashMessage('success', 'Cache cleared successfully');
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to clear cache');
        }
    },

    // Export logs
    async exportLogs() {
        try {
            window.location.href = '/admin/logs/export';
            window.Caslink.utils.showFlashMessage('success', 'Log export started');
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to export logs');
        }
    },

    // Update individual setting
    async updateSetting(setting, value) {
        try {
            await window.Caslink.API.post('/admin/settings/update', {
                key: setting,
                value: value
            });
            window.Caslink.utils.showFlashMessage('success', 'Setting updated');
        } catch (error) {
            window.Caslink.utils.showFlashMessage('error', 'Failed to update setting');
        }
    },

    // Setup auto-refresh for dashboard
    setupAutoRefresh() {
        // Refresh dashboard data every 30 seconds
        setInterval(() => {
            if (this.currentSection === 'dashboard') {
                this.loadDashboardData();
            }
        }, 30000);
    }
};

// Initialize admin interface when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    if (document.getElementById('admin-container')) {
        Admin.init();
    }
});

// Export for global access
window.Admin = Admin;
