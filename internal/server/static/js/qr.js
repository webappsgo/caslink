/**
 * QR Code JavaScript for Caslink URL Shortener
 * Handles QR code generation and customization using QRCode.js
 */

const QRGenerator = {
    currentUrl: '',
    currentCode: null,
    settings: {
        size: 300,
        format: 'png',
        style: 'square',
        fgColor: '#000000',
        bgColor: '#ffffff',
        errorCorrection: 'M',
        margin: 2,
        logo: null
    },

    // Initialize QR generator
    init(url) {
        this.currentUrl = url;
        this.bindEvents();
        this.generateQR();
    },

    // Bind event listeners
    bindEvents() {
        // Size slider
        const sizeSlider = document.getElementById('qr-size');
        if (sizeSlider) {
            sizeSlider.addEventListener('input', (e) => {
                this.settings.size = parseInt(e.target.value);
                document.getElementById('qr-size-value').textContent = this.settings.size + 'px';
                this.generateQR();
            });
        }

        // Format selector
        document.querySelectorAll('input[name="qr-format"]').forEach(input => {
            input.addEventListener('change', (e) => {
                this.settings.format = e.target.value;
                this.generateQR();
            });
        });

        // Style selector
        document.querySelectorAll('input[name="qr-style"]').forEach(input => {
            input.addEventListener('change', (e) => {
                this.settings.style = e.target.value;
                this.generateQR();
            });
        });

        // Color pickers
        document.getElementById('qr-fg-color')?.addEventListener('input', (e) => {
            this.settings.fgColor = e.target.value;
            this.generateQR();
        });

        document.getElementById('qr-bg-color')?.addEventListener('input', (e) => {
            this.settings.bgColor = e.target.value;
            this.generateQR();
        });

        // Error correction level
        document.getElementById('qr-error-correction')?.addEventListener('change', (e) => {
            this.settings.errorCorrection = e.target.value;
            this.generateQR();
        });

        // Logo upload
        document.getElementById('qr-logo')?.addEventListener('change', (e) => {
            this.handleLogoUpload(e.target.files[0]);
        });

        // Remove logo
        document.getElementById('remove-logo')?.addEventListener('click', () => {
            this.settings.logo = null;
            document.getElementById('qr-logo').value = '';
            document.getElementById('logo-preview')?.remove();
            this.generateQR();
        });

        // Download button
        document.getElementById('download-qr')?.addEventListener('click', () => {
            this.downloadQR();
        });

        // Copy as data URL
        document.getElementById('copy-data-url')?.addEventListener('click', () => {
            this.copyDataURL();
        });

        // Print button
        document.getElementById('print-qr')?.addEventListener('click', () => {
            this.printQR();
        });

        // Reset button
        document.getElementById('reset-qr')?.addEventListener('click', () => {
            this.resetSettings();
        });
    },

    // Generate QR code
    async generateQR() {
        const container = document.getElementById('qr-preview');
        if (!container) return;

        // Clear previous QR code
        container.innerHTML = '';

        try {
            if (this.settings.format === 'svg') {
                await this.generateSVG(container);
            } else {
                await this.generateCanvas(container);
            }

            // Update file size estimate
            this.updateFileSizeEstimate();
        } catch (error) {
            console.error('QR generation error:', error);
            container.innerHTML = '<p class="text-error">Failed to generate QR code</p>';
        }
    },

    // Generate canvas-based QR code (PNG/JPG)
    async generateCanvas(container) {
        const canvas = document.createElement('canvas');
        container.appendChild(canvas);

        // Use QRCode library (assuming it's loaded)
        if (typeof QRCode !== 'undefined') {
            new QRCode(canvas, {
                text: this.currentUrl,
                width: this.settings.size,
                height: this.settings.size,
                colorDark: this.settings.fgColor,
                colorLight: this.settings.bgColor,
                correctLevel: QRCode.CorrectLevel[this.settings.errorCorrection]
            });
        } else {
            // Fallback: use simple canvas drawing
            this.drawQRManually(canvas);
        }

        // Apply style modifications
        if (this.settings.style === 'rounded') {
            this.applyRoundedStyle(canvas);
        } else if (this.settings.style === 'circle') {
            this.applyCircleStyle(canvas);
        }

        // Add logo if present
        if (this.settings.logo) {
            await this.addLogoToCanvas(canvas);
        }

        this.currentCode = canvas;
    },

    // Generate SVG QR code
    async generateSVG(container) {
        // This is a simplified version - would use a proper QR library in production
        const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg.setAttribute('width', this.settings.size);
        svg.setAttribute('height', this.settings.size);
        svg.setAttribute('viewBox', `0 0 ${this.settings.size} ${this.settings.size}`);

        // Background
        const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
        rect.setAttribute('width', '100%');
        rect.setAttribute('height', '100%');
        rect.setAttribute('fill', this.settings.bgColor);
        svg.appendChild(rect);

        // QR code pattern (simplified - would use proper QR algorithm)
        const moduleSize = this.settings.size / 33; // Standard QR code is 33x33 modules

        // Note: In production, use a proper QR code library for SVG generation
        const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        text.setAttribute('x', '50%');
        text.setAttribute('y', '50%');
        text.setAttribute('text-anchor', 'middle');
        text.setAttribute('dominant-baseline', 'middle');
        text.setAttribute('fill', this.settings.fgColor);
        text.setAttribute('font-size', '14');
        text.textContent = 'QR Code Preview';
        svg.appendChild(text);

        container.appendChild(svg);
        this.currentCode = svg;
    },

    // Draw QR code manually (fallback)
    drawQRManually(canvas) {
        const ctx = canvas.getContext('2d');
        canvas.width = this.settings.size;
        canvas.height = this.settings.size;

        // Background
        ctx.fillStyle = this.settings.bgColor;
        ctx.fillRect(0, 0, this.settings.size, this.settings.size);

        // Simplified QR pattern (for demo)
        ctx.fillStyle = this.settings.fgColor;
        const moduleSize = this.settings.size / 33;

        // Draw position markers
        this.drawPositionMarker(ctx, 0, 0, moduleSize);
        this.drawPositionMarker(ctx, 26 * moduleSize, 0, moduleSize);
        this.drawPositionMarker(ctx, 0, 26 * moduleSize, moduleSize);

        // Draw data pattern (simplified)
        for (let i = 0; i < 33; i++) {
            for (let j = 0; j < 33; j++) {
                if (Math.random() > 0.5 && !this.isPositionMarker(i, j)) {
                    ctx.fillRect(i * moduleSize, j * moduleSize, moduleSize, moduleSize);
                }
            }
        }
    },

    // Draw position marker
    drawPositionMarker(ctx, x, y, size) {
        ctx.fillRect(x, y, size * 7, size * 7);
        ctx.fillStyle = this.settings.bgColor;
        ctx.fillRect(x + size, y + size, size * 5, size * 5);
        ctx.fillStyle = this.settings.fgColor;
        ctx.fillRect(x + size * 2, y + size * 2, size * 3, size * 3);
    },

    // Check if position is in position marker
    isPositionMarker(i, j) {
        return (i < 9 && j < 9) || (i < 9 && j > 23) || (i > 23 && j < 9);
    },

    // Apply rounded style
    applyRoundedStyle(canvas) {
        // Would apply rounded corners to modules in production
        const ctx = canvas.getContext('2d');
        ctx.globalCompositeOperation = 'destination-out';
        // Simplified - would properly round each module
        ctx.globalCompositeOperation = 'source-over';
    },

    // Apply circle style
    applyCircleStyle(canvas) {
        // Would convert modules to circles in production
        const ctx = canvas.getContext('2d');
        // Simplified implementation
    },

    // Handle logo upload
    handleLogoUpload(file) {
        if (!file) return;

        if (!file.type.startsWith('image/')) {
            window.Caslink.utils.showFlashMessage('error', 'Please upload an image file');
            return;
        }

        const reader = new FileReader();
        reader.onload = (e) => {
            this.settings.logo = e.target.result;

            // Show logo preview
            const preview = document.createElement('div');
            preview.id = 'logo-preview';
            preview.innerHTML = `
                <img src="${e.target.result}" alt="Logo" style="max-width: 50px; max-height: 50px;">
            `;

            const container = document.getElementById('logo-container');
            if (container) {
                const existing = document.getElementById('logo-preview');
                if (existing) existing.remove();
                container.appendChild(preview);
            }

            this.generateQR();
        };

        reader.readAsDataURL(file);
    },

    // Add logo to canvas
    async addLogoToCanvas(canvas) {
        return new Promise((resolve) => {
            const img = new Image();
            img.onload = () => {
                const ctx = canvas.getContext('2d');
                const logoSize = this.settings.size / 5;
                const x = (this.settings.size - logoSize) / 2;
                const y = (this.settings.size - logoSize) / 2;

                // Draw white background for logo
                ctx.fillStyle = '#ffffff';
                ctx.fillRect(x - 10, y - 10, logoSize + 20, logoSize + 20);

                // Draw logo
                ctx.drawImage(img, x, y, logoSize, logoSize);
                resolve();
            };
            img.src = this.settings.logo;
        });
    },

    // Download QR code
    downloadQR() {
        if (!this.currentCode) return;

        let dataUrl, filename;

        if (this.settings.format === 'svg') {
            const svgData = new XMLSerializer().serializeToString(this.currentCode);
            const blob = new Blob([svgData], { type: 'image/svg+xml' });
            dataUrl = URL.createObjectURL(blob);
            filename = `qr_${Date.now()}.svg`;
        } else {
            dataUrl = this.currentCode.toDataURL(`image/${this.settings.format}`);
            filename = `qr_${Date.now()}.${this.settings.format}`;
        }

        const a = document.createElement('a');
        a.href = dataUrl;
        a.download = filename;
        a.click();

        if (this.settings.format === 'svg') {
            URL.revokeObjectURL(dataUrl);
        }

        window.Caslink.utils.showFlashMessage('success', 'QR code downloaded');
    },

    // Copy data URL to clipboard
    async copyDataURL() {
        if (!this.currentCode) return;

        let dataUrl;

        if (this.settings.format === 'svg') {
            const svgData = new XMLSerializer().serializeToString(this.currentCode);
            dataUrl = `data:image/svg+xml;base64,${btoa(svgData)}`;
        } else {
            dataUrl = this.currentCode.toDataURL(`image/${this.settings.format}`);
        }

        const success = await window.Caslink.utils.copyToClipboard(dataUrl);

        if (success) {
            window.Caslink.utils.showFlashMessage('success', 'Data URL copied to clipboard');
        } else {
            window.Caslink.utils.showFlashMessage('error', 'Failed to copy data URL');
        }
    },

    // Print QR code
    printQR() {
        if (!this.currentCode) return;

        const printWindow = window.open('', '_blank');
        const dataUrl = this.settings.format === 'svg'
            ? 'data:image/svg+xml;base64,' + btoa(new XMLSerializer().serializeToString(this.currentCode))
            : this.currentCode.toDataURL('image/png');

        printWindow.document.write(`
            <!DOCTYPE html>
            <html>
            <head>
                <title>QR Code - ${this.currentUrl}</title>
                <style>
                    body {
                        display: flex;
                        flex-direction: column;
                        align-items: center;
                        justify-content: center;
                        min-height: 100vh;
                        margin: 0;
                        font-family: Arial, sans-serif;
                    }
                    img {
                        max-width: 100%;
                        height: auto;
                    }
                    p {
                        margin-top: 20px;
                        text-align: center;
                        word-break: break-all;
                    }
                </style>
            </head>
            <body>
                <img src="${dataUrl}" alt="QR Code">
                <p>${this.currentUrl}</p>
            </body>
            </html>
        `);

        printWindow.document.close();
        printWindow.focus();

        setTimeout(() => {
            printWindow.print();
            printWindow.close();
        }, 250);
    },

    // Reset settings to defaults
    resetSettings() {
        this.settings = {
            size: 300,
            format: 'png',
            style: 'square',
            fgColor: '#000000',
            bgColor: '#ffffff',
            errorCorrection: 'M',
            margin: 2,
            logo: null
        };

        // Reset form inputs
        document.getElementById('qr-size').value = 300;
        document.getElementById('qr-size-value').textContent = '300px';
        document.querySelector('input[name="qr-format"][value="png"]').checked = true;
        document.querySelector('input[name="qr-style"][value="square"]').checked = true;
        document.getElementById('qr-fg-color').value = '#000000';
        document.getElementById('qr-bg-color').value = '#ffffff';
        document.getElementById('qr-error-correction').value = 'M';
        document.getElementById('qr-logo').value = '';
        document.getElementById('logo-preview')?.remove();

        this.generateQR();
        window.Caslink.utils.showFlashMessage('success', 'Settings reset to defaults');
    },

    // Update file size estimate
    updateFileSizeEstimate() {
        const estimate = document.getElementById('file-size-estimate');
        if (!estimate || !this.currentCode) return;

        let size = 0;

        if (this.settings.format === 'svg') {
            const svgData = new XMLSerializer().serializeToString(this.currentCode);
            size = new Blob([svgData]).size;
        } else {
            const dataUrl = this.currentCode.toDataURL(`image/${this.settings.format}`);
            size = Math.round((dataUrl.length - 22) * 3 / 4); // Approximate size from base64
        }

        estimate.textContent = window.Caslink.utils.formatFileSize(size);
    }
};

// Initialize QR generator when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    const qrContainer = document.getElementById('qr-container');
    if (qrContainer) {
        const url = qrContainer.dataset.url;
        QRGenerator.init(url);
    }
});

// Export for global access
window.QRGenerator = QRGenerator;
