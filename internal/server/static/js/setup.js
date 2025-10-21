/**
 * Setup Wizard JavaScript for Caslink URL Shortener
 * Handles first-run setup process with step-by-step guidance
 */

const Setup = {
    currentStep: 1,
    totalSteps: 3,
    setupData: {
        admin: {},
        firstUrl: {},
        config: {}
    },

    // Initialize setup wizard
    init() {
        this.detectCurrentStep();
        this.bindEvents();
        this.updateProgress();
        this.checkSetupStatus();
    },

    // Detect current step from URL or page
    detectCurrentStep() {
        const path = window.location.pathname;

        if (path.includes('/setup/admin')) {
            this.currentStep = 1;
        } else if (path.includes('/setup/first-url')) {
            this.currentStep = 2;
        } else if (path.includes('/setup/customize')) {
            this.currentStep = 3;
        }
    },

    // Bind event listeners
    bindEvents() {
        // Admin creation form
        const adminForm = document.getElementById('admin-form');
        if (adminForm) {
            adminForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.createAdmin();
            });
        }

        // First URL creation form
        const urlForm = document.getElementById('first-url-form');
        if (urlForm) {
            urlForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.createFirstUrl();
            });
        }

        // Customization form
        const customizeForm = document.getElementById('customize-form');
        if (customizeForm) {
            customizeForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.saveCustomization();
            });
        }

        // Skip customization button
        document.getElementById('skip-customize')?.addEventListener('click', () => {
            this.completeSetup();
        });

        // Password strength indicator
        const passwordInput = document.getElementById('password');
        if (passwordInput) {
            passwordInput.addEventListener('input', (e) => {
                this.updatePasswordStrength(e.target.value);
            });
        }

        // Custom code availability check
        const customCodeInput = document.getElementById('custom-code');
        if (customCodeInput) {
            customCodeInput.addEventListener('input', window.Caslink.utils.debounce((e) => {
                this.checkCodeAvailability(e.target.value);
            }, 500));
        }

        // Generate random code button
        document.getElementById('generate-code')?.addEventListener('click', () => {
            this.generateRandomCode();
        });
    },

    // Check setup status
    async checkSetupStatus() {
        try {
            const status = await window.Caslink.API.get('/api/v1/setup/status');

            if (status.completed) {
                // Setup already completed, redirect to home
                window.location.href = '/';
            } else if (status.current_step) {
                this.currentStep = status.current_step;
                this.updateProgress();
            }
        } catch (error) {
            console.error('Failed to check setup status:', error);
        }
    },

    // Update progress indicator
    updateProgress() {
        const progressBar = document.getElementById('setup-progress');
        if (progressBar) {
            const percentage = (this.currentStep / this.totalSteps) * 100;
            progressBar.style.width = `${percentage}%`;
        }

        const stepIndicator = document.getElementById('step-indicator');
        if (stepIndicator) {
            stepIndicator.textContent = `Step ${this.currentStep} of ${this.totalSteps}`;
        }

        // Update step markers
        document.querySelectorAll('.setup-step').forEach((step, index) => {
            if (index + 1 < this.currentStep) {
                step.classList.add('completed');
                step.classList.remove('active');
            } else if (index + 1 === this.currentStep) {
                step.classList.add('active');
                step.classList.remove('completed');
            } else {
                step.classList.remove('active', 'completed');
            }
        });
    },

    // Create admin account
    async createAdmin() {
        const form = document.getElementById('admin-form');
        const submitButton = form.querySelector('button[type="submit"]');

        const formData = new FormData(form);
        const adminData = {
            username: formData.get('username'),
            email: formData.get('email'),
            password: formData.get('password'),
            password_confirm: formData.get('password_confirm')
        };

        // Validation
        if (adminData.password !== adminData.password_confirm) {
            window.Caslink.utils.showFlashMessage('error', 'Passwords do not match');
            return;
        }

        if (adminData.password.length < 8) {
            window.Caslink.utils.showFlashMessage('error', 'Password must be at least 8 characters');
            return;
        }

        submitButton.disabled = true;
        submitButton.textContent = 'Creating account...';

        try {
            const response = await window.Caslink.API.post('/api/v1/setup/admin', adminData);

            this.setupData.admin = response.user;
            window.Caslink.utils.showFlashMessage('success', 'Admin account created successfully!');

            // Proceed to next step
            setTimeout(() => {
                window.location.href = '/setup/first-url';
            }, 1000);
        } catch (error) {
            console.error('Failed to create admin:', error);
            window.Caslink.utils.showFlashMessage('error', error.message || 'Failed to create admin account');
            submitButton.disabled = false;
            submitButton.textContent = 'Create Admin Account';
        }
    },

    // Create first URL
    async createFirstUrl() {
        const form = document.getElementById('first-url-form');
        const submitButton = form.querySelector('button[type="submit"]');

        const formData = new FormData(form);
        const urlData = {
            original_url: formData.get('original_url'),
            custom_code: formData.get('custom_code') || '',
            title: formData.get('title') || ''
        };

        // Validation
        if (!window.Caslink.utils.isValidURL(urlData.original_url)) {
            window.Caslink.utils.showFlashMessage('error', 'Please enter a valid URL');
            return;
        }

        submitButton.disabled = true;
        submitButton.textContent = 'Creating short URL...';

        try {
            const response = await window.Caslink.API.post('/api/v1/setup/first-url', urlData);

            this.setupData.firstUrl = response.url;

            // Show success message with the created URL
            const resultContainer = document.getElementById('url-result');
            if (resultContainer) {
                const shortUrl = `${window.location.origin}/${response.url.short_code}`;
                resultContainer.innerHTML = `
                    <div class="result-container">
                        <h3 class="result-title">Your First Short URL!</h3>
                        <div class="result-url-container">
                            <div class="result-url">${shortUrl}</div>
                            <div class="result-actions">
                                <button class="action-btn" onclick="window.Caslink.utils.copyToClipboard('${shortUrl}')">
                                    Copy
                                </button>
                                <a href="${shortUrl}" class="action-btn" target="_blank">
                                    Visit
                                </a>
                            </div>
                        </div>
                        <button class="submit-btn" onclick="window.location.href='/setup/customize'" style="margin-top: 20px;">
                            Continue to Customization
                        </button>
                    </div>
                `;
                resultContainer.style.display = 'block';
            }

            window.Caslink.utils.showFlashMessage('success', 'Short URL created successfully!');
        } catch (error) {
            console.error('Failed to create URL:', error);
            window.Caslink.utils.showFlashMessage('error', error.message || 'Failed to create short URL');
            submitButton.disabled = false;
            submitButton.textContent = 'Create Short URL';
        }
    },

    // Save customization settings
    async saveCustomization() {
        const form = document.getElementById('customize-form');
        const submitButton = form.querySelector('button[type="submit"]');

        const formData = new FormData(form);
        const configData = {
            brand_name: formData.get('brand_name'),
            enable_registration: formData.get('enable_registration') === 'on',
            enable_anonymous_urls: formData.get('enable_anonymous_urls') === 'on',
            default_theme: formData.get('default_theme')
        };

        submitButton.disabled = true;
        submitButton.textContent = 'Saving...';

        try {
            await window.Caslink.API.post('/api/v1/setup/config', configData);

            this.setupData.config = configData;
            window.Caslink.utils.showFlashMessage('success', 'Configuration saved successfully!');

            // Complete setup
            this.completeSetup();
        } catch (error) {
            console.error('Failed to save configuration:', error);
            window.Caslink.utils.showFlashMessage('error', 'Failed to save configuration');
            submitButton.disabled = false;
            submitButton.textContent = 'Complete Setup';
        }
    },

    // Complete setup and redirect
    completeSetup() {
        window.Caslink.utils.showFlashMessage('success', 'Setup completed! Redirecting to dashboard...');

        setTimeout(() => {
            window.location.href = '/dashboard';
        }, 2000);
    },

    // Update password strength indicator
    updatePasswordStrength(password) {
        const indicator = document.getElementById('password-strength');
        if (!indicator) return;

        let strength = 0;
        let feedback = '';

        if (password.length >= 8) strength++;
        if (password.length >= 12) strength++;
        if (/[a-z]/.test(password) && /[A-Z]/.test(password)) strength++;
        if (/\d/.test(password)) strength++;
        if (/[^a-zA-Z0-9]/.test(password)) strength++;

        const levels = ['Very Weak', 'Weak', 'Fair', 'Good', 'Strong'];
        const colors = ['#ef4444', '#f59e0b', '#eab308', '#10b981', '#059669'];

        feedback = levels[Math.min(strength, 4)];

        indicator.innerHTML = `
            <div style="display: flex; align-items: center; gap: 8px; margin-top: 4px;">
                <div style="flex: 1; height: 4px; background-color: var(--border-color); border-radius: 2px; overflow: hidden;">
                    <div style="width: ${strength * 20}%; height: 100%; background-color: ${colors[Math.min(strength, 4)]}; transition: all 0.3s ease;"></div>
                </div>
                <span style="font-size: 0.75rem; color: ${colors[Math.min(strength, 4)]}; font-weight: 600;">
                    ${feedback}
                </span>
            </div>
        `;
    },

    // Check custom code availability
    async checkCodeAvailability(code) {
        const indicator = document.getElementById('code-availability');
        if (!indicator || !code) {
            if (indicator) indicator.innerHTML = '';
            return;
        }

        // Validate format
        if (!/^[a-zA-Z0-9_-]{3,50}$/.test(code)) {
            indicator.innerHTML = '<span class="availability-text unavailable">Invalid format (3-50 characters, alphanumeric, dash, underscore)</span>';
            return;
        }

        try {
            const response = await window.Caslink.API.get(`/api/v1/check-availability/${code}`);

            if (response.available) {
                indicator.innerHTML = '<span class="availability-text available">✓ Available</span>';
            } else {
                indicator.innerHTML = '<span class="availability-text unavailable">✗ Already taken</span>';
            }
        } catch (error) {
            indicator.innerHTML = '<span class="availability-text unavailable">✗ Not available</span>';
        }
    },

    // Generate random code
    generateRandomCode() {
        const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
        let code = '';

        for (let i = 0; i < 8; i++) {
            code += chars.charAt(Math.floor(Math.random() * chars.length));
        }

        const input = document.getElementById('custom-code');
        if (input) {
            input.value = code;
            this.checkCodeAvailability(code);
        }
    }
};

// Initialize setup wizard when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    if (document.querySelector('.setup-container')) {
        Setup.init();
    }
});

// Export for global access
window.Setup = Setup;
