/**
 * Bulk Operations JavaScript for Caslink URL Shortener
 * Handles bulk import/export of URLs with file handling
 */

const Bulk = {
    importedData: [],
    validatedData: [],
    errors: [],

    // Initialize bulk operations
    init() {
        this.bindEvents();
    },

    // Bind event listeners
    bindEvents() {
        // File input
        const fileInput = document.getElementById('bulk-file-input');
        if (fileInput) {
            fileInput.addEventListener('change', (e) => {
                this.handleFileUpload(e.target.files[0]);
            });
        }

        // Drag and drop
        const dropZone = document.getElementById('drop-zone');
        if (dropZone) {
            dropZone.addEventListener('dragover', (e) => {
                e.preventDefault();
                dropZone.classList.add('dragover');
            });

            dropZone.addEventListener('dragleave', () => {
                dropZone.classList.remove('dragover');
            });

            dropZone.addEventListener('drop', (e) => {
                e.preventDefault();
                dropZone.classList.remove('dragover');
                this.handleFileUpload(e.dataTransfer.files[0]);
            });

            dropZone.addEventListener('click', () => {
                fileInput?.click();
            });
        }

        // Import button
        document.getElementById('import-button')?.addEventListener('click', () => {
            this.importUrls();
        });

        // Export buttons
        document.getElementById('export-csv')?.addEventListener('click', () => {
            this.exportUrls('csv');
        });

        document.getElementById('export-json')?.addEventListener('click', () => {
            this.exportUrls('json');
        });

        // Download template
        document.getElementById('download-template')?.addEventListener('click', () => {
            this.downloadTemplate();
        });
    },

    // Handle file upload
    handleFileUpload(file) {
        if (!file) return;

        const maxSize = 10 * 1024 * 1024; // 10MB
        if (file.size > maxSize) {
            window.Caslink.utils.showFlashMessage('error', 'File size exceeds 10MB limit');
            return;
        }

        const fileExtension = file.name.split('.').pop().toLowerCase();

        if (fileExtension === 'csv') {
            this.parseCSV(file);
        } else if (fileExtension === 'json') {
            this.parseJSON(file);
        } else {
            window.Caslink.utils.showFlashMessage('error', 'Unsupported file format. Please use CSV or JSON.');
        }
    },

    // Parse CSV file
    parseCSV(file) {
        const reader = new FileReader();

        reader.onload = (e) => {
            try {
                const csv = e.target.result;
                const lines = csv.split('\n');
                const headers = lines[0].split(',').map(h => h.trim().replace(/"/g, ''));

                this.importedData = [];

                for (let i = 1; i < lines.length; i++) {
                    if (!lines[i].trim()) continue;

                    const values = this.parseCSVLine(lines[i]);
                    const row = {};

                    headers.forEach((header, index) => {
                        row[header] = values[index] || '';
                    });

                    this.importedData.push(row);
                }

                this.validateData();
                this.renderPreview();
                window.Caslink.utils.showFlashMessage('success', `Parsed ${this.importedData.length} URLs from CSV`);
            } catch (error) {
                console.error('CSV parsing error:', error);
                window.Caslink.utils.showFlashMessage('error', 'Failed to parse CSV file');
            }
        };

        reader.readAsText(file);
    },

    // Parse CSV line (handles quoted values)
    parseCSVLine(line) {
        const values = [];
        let current = '';
        let inQuotes = false;

        for (let i = 0; i < line.length; i++) {
            const char = line[i];

            if (char === '"') {
                inQuotes = !inQuotes;
            } else if (char === ',' && !inQuotes) {
                values.push(current.trim());
                current = '';
            } else {
                current += char;
            }
        }

        values.push(current.trim());
        return values;
    },

    // Parse JSON file
    parseJSON(file) {
        const reader = new FileReader();

        reader.onload = (e) => {
            try {
                const json = JSON.parse(e.target.result);

                if (Array.isArray(json)) {
                    this.importedData = json;
                } else if (json.urls && Array.isArray(json.urls)) {
                    this.importedData = json.urls;
                } else {
                    throw new Error('Invalid JSON structure');
                }

                this.validateData();
                this.renderPreview();
                window.Caslink.utils.showFlashMessage('success', `Parsed ${this.importedData.length} URLs from JSON`);
            } catch (error) {
                console.error('JSON parsing error:', error);
                window.Caslink.utils.showFlashMessage('error', 'Failed to parse JSON file');
            }
        };

        reader.readAsText(file);
    },

    // Validate imported data
    validateData() {
        this.validatedData = [];
        this.errors = [];

        this.importedData.forEach((row, index) => {
            const errors = [];

            // Validate original URL
            if (!row.original_url && !row.url) {
                errors.push('Missing original URL');
            } else {
                const url = row.original_url || row.url;
                if (!window.Caslink.utils.isValidURL(url)) {
                    errors.push('Invalid URL format');
                }
            }

            // Validate custom code if provided
            if (row.short_code || row.custom_code) {
                const code = row.short_code || row.custom_code;
                if (!/^[a-zA-Z0-9_-]{3,50}$/.test(code)) {
                    errors.push('Invalid short code format');
                }
            }

            if (errors.length > 0) {
                this.errors.push({
                    line: index + 2, // +2 for header and 0-indexing
                    data: row,
                    errors: errors
                });
            } else {
                this.validatedData.push({
                    original_url: row.original_url || row.url,
                    short_code: row.short_code || row.custom_code || '',
                    title: row.title || '',
                    description: row.description || '',
                    tags: row.tags || '',
                    expires_at: row.expires_at || null
                });
            }
        });
    },

    // Render preview of imported data
    renderPreview() {
        const preview = document.getElementById('import-preview');
        if (!preview) return;

        preview.style.display = 'block';

        // Render summary
        const summary = document.getElementById('import-summary');
        if (summary) {
            summary.innerHTML = `
                <div class="admin-alert ${this.errors.length > 0 ? 'warning' : 'success'}">
                    <div class="admin-alert-content">
                        <div class="admin-alert-title">Import Summary</div>
                        <div class="admin-alert-message">
                            <strong>${this.validatedData.length}</strong> valid URLs ready to import
                            ${this.errors.length > 0 ? `<br><strong>${this.errors.length}</strong> errors found` : ''}
                        </div>
                    </div>
                </div>
            `;
        }

        // Render errors if any
        if (this.errors.length > 0) {
            const errorsContainer = document.getElementById('import-errors');
            if (errorsContainer) {
                errorsContainer.style.display = 'block';
                errorsContainer.innerHTML = `
                    <h4>Errors (${this.errors.length})</h4>
                    <div class="error-list">
                        ${this.errors.slice(0, 10).map(error => `
                            <div class="error-item">
                                <strong>Line ${error.line}:</strong>
                                ${error.errors.join(', ')}
                                <br>
                                <code>${JSON.stringify(error.data)}</code>
                            </div>
                        `).join('')}
                        ${this.errors.length > 10 ? `<p class="text-secondary">... and ${this.errors.length - 10} more errors</p>` : ''}
                    </div>
                `;
            }
        }

        // Render preview table
        const table = document.getElementById('preview-table');
        if (table && this.validatedData.length > 0) {
            table.innerHTML = `
                <table class="users-table">
                    <thead>
                        <tr>
                            <th>Original URL</th>
                            <th>Short Code</th>
                            <th>Title</th>
                            <th>Tags</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${this.validatedData.slice(0, 10).map(url => `
                            <tr>
                                <td>${window.Caslink.utils.escapeHtml(url.original_url)}</td>
                                <td>${url.short_code ? window.Caslink.utils.escapeHtml(url.short_code) : '<em>Auto-generate</em>'}</td>
                                <td>${window.Caslink.utils.escapeHtml(url.title)}</td>
                                <td>${window.Caslink.utils.escapeHtml(url.tags)}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
                ${this.validatedData.length > 10 ? `<p class="text-secondary text-center">... and ${this.validatedData.length - 10} more URLs</p>` : ''}
            `;
        }

        // Enable import button
        const importButton = document.getElementById('import-button');
        if (importButton) {
            importButton.disabled = this.validatedData.length === 0;
        }
    },

    // Import URLs to server
    async importUrls() {
        if (this.validatedData.length === 0) {
            window.Caslink.utils.showFlashMessage('error', 'No valid URLs to import');
            return;
        }

        const importButton = document.getElementById('import-button');
        if (importButton) {
            importButton.disabled = true;
            importButton.textContent = 'Importing...';
        }

        try {
            const response = await window.Caslink.API.post('/bulk/import', {
                urls: this.validatedData
            });

            window.Caslink.utils.showFlashMessage('success', `Successfully imported ${response.imported} URLs`);

            // Reset state
            this.importedData = [];
            this.validatedData = [];
            this.errors = [];

            // Hide preview
            const preview = document.getElementById('import-preview');
            if (preview) preview.style.display = 'none';

            // Redirect to dashboard
            setTimeout(() => {
                window.location.href = '/dashboard';
            }, 2000);
        } catch (error) {
            console.error('Import error:', error);
            window.Caslink.utils.showFlashMessage('error', error.message || 'Failed to import URLs');
        } finally {
            if (importButton) {
                importButton.disabled = false;
                importButton.textContent = 'Import URLs';
            }
        }
    },

    // Export URLs
    async exportUrls(format) {
        try {
            const response = await window.Caslink.API.get(`/bulk/export?format=${format}`);

            let content, filename, mimeType;

            if (format === 'csv') {
                content = this.convertToCSV(response.urls);
                filename = `caslink_urls_${Date.now()}.csv`;
                mimeType = 'text/csv';
            } else if (format === 'json') {
                content = JSON.stringify(response, null, 2);
                filename = `caslink_urls_${Date.now()}.json`;
                mimeType = 'application/json';
            }

            this.downloadFile(content, filename, mimeType);
            window.Caslink.utils.showFlashMessage('success', `Exported ${response.urls.length} URLs`);
        } catch (error) {
            console.error('Export error:', error);
            window.Caslink.utils.showFlashMessage('error', 'Failed to export URLs');
        }
    },

    // Convert data to CSV
    convertToCSV(urls) {
        const headers = ['short_code', 'original_url', 'title', 'description', 'tags', 'clicks', 'created_at', 'expires_at'];
        const rows = urls.map(url => headers.map(h => {
            const value = url[h] || '';
            return `"${String(value).replace(/"/g, '""')}"`;
        }));

        return [
            headers.join(','),
            ...rows.map(row => row.join(','))
        ].join('\n');
    },

    // Download file
    downloadFile(content, filename, mimeType) {
        const blob = new Blob([content], { type: mimeType });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    },

    // Download CSV template
    downloadTemplate() {
        const template = [
            'original_url,short_code,title,description,tags,expires_at',
            'https://example.com,example,Example Site,An example website,example;demo,',
            'https://github.com,,GitHub,Code hosting platform,dev;tools,2024-12-31T23:59:59Z'
        ].join('\n');

        this.downloadFile(template, 'caslink_template.csv', 'text/csv');
        window.Caslink.utils.showFlashMessage('success', 'Template downloaded');
    }
};

// Initialize bulk operations when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    if (document.getElementById('bulk-container')) {
        Bulk.init();
    }
});

// Export for global access
window.Bulk = Bulk;
