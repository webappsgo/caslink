/**
 * Main JavaScript file for Caslink URL Shortener
 * Handles core functionality, navigation, and utility functions
 */

// Global configuration
window.Caslink = {
    baseURL: window.location.origin,
    apiURL: '/api/v1',
    version: '1.0.0',
    config: {
        flashMessageDuration: 5000,
        debounceDelay: 500,
        animationDuration: 300,
        tooltipDelay: 1000
    }
};

// Initialize application when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    initializeNavigation();
    initializeFlashMessages();
    initializeTooltips();
    initializeThemeToggle();
    initializeFormValidation();
    initializeKeyboardShortcuts();
});

/**
 * Navigation functionality
 */
function initializeNavigation() {
    const navToggle = document.getElementById('nav-toggle');
    const navMenu = document.getElementById('nav-menu');
    const dropdowns = document.querySelectorAll('.dropdown');

    // Mobile navigation toggle
    if (navToggle && navMenu) {
        navToggle.addEventListener('click', function() {
            navMenu.classList.toggle('active');

            // Animate hamburger menu
            const lines = navToggle.querySelectorAll('.nav-toggle-line');
            lines.forEach((line, index) => {
                if (navMenu.classList.contains('active')) {
                    if (index === 0) line.style.transform = 'rotate(45deg) translate(5px, 5px)';
                    if (index === 1) line.style.opacity = '0';
                    if (index === 2) line.style.transform = 'rotate(-45deg) translate(7px, -6px)';
                } else {
                    line.style.transform = '';
                    line.style.opacity = '';
                }
            });
        });

        // Close mobile menu when clicking outside
        document.addEventListener('click', function(e) {
            if (!navToggle.contains(e.target) && !navMenu.contains(e.target)) {
                navMenu.classList.remove('active');
                // Reset hamburger animation
                const lines = navToggle.querySelectorAll('.nav-toggle-line');
                lines.forEach(line => {
                    line.style.transform = '';
                    line.style.opacity = '';
                });
            }
        });
    }

    // Desktop dropdown menus
    dropdowns.forEach(dropdown => {
        const toggle = dropdown.querySelector('.dropdown-toggle');
        const menu = dropdown.querySelector('.dropdown-menu');

        if (toggle && menu) {
            let timeoutId;

            dropdown.addEventListener('mouseenter', function() {
                clearTimeout(timeoutId);
                menu.style.opacity = '1';
                menu.style.visibility = 'visible';
                menu.style.transform = 'translateY(0)';
            });

            dropdown.addEventListener('mouseleave', function() {
                timeoutId = setTimeout(() => {
                    menu.style.opacity = '0';
                    menu.style.visibility = 'hidden';
                    menu.style.transform = 'translateY(-10px)';
                }, 100);
            });

            // Keyboard navigation
            toggle.addEventListener('keydown', function(e) {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    menu.style.opacity = menu.style.opacity === '1' ? '0' : '1';
                    menu.style.visibility = menu.style.visibility === 'visible' ? 'hidden' : 'visible';
                }
            });
        }
    });
}

/**
 * Flash message functionality
 */
function initializeFlashMessages() {
    const flashMessages = document.querySelectorAll('.flash[data-auto-dismiss="true"]');

    flashMessages.forEach(flash => {
        setTimeout(() => {
            dismissFlashMessage(flash);
        }, window.Caslink.config.flashMessageDuration);
    });

    // Add click handlers for close buttons
    document.addEventListener('click', function(e) {
        if (e.target.matches('.flash-close, .flash-close *')) {
            const flash = e.target.closest('.flash');
            if (flash) {
                dismissFlashMessage(flash);
            }
        }
    });
}

function dismissFlashMessage(flash) {
    flash.style.transform = 'translateX(100%)';
    flash.style.opacity = '0';
    setTimeout(() => {
        if (flash.parentNode) {
            flash.parentNode.removeChild(flash);
        }
    }, window.Caslink.config.animationDuration);
}

/**
 * Create and show flash message
 */
function showFlashMessage(type, message, autoDismiss = true) {
    const flashContainer = document.querySelector('.flash-messages') || createFlashContainer();

    const flash = document.createElement('div');
    flash.className = `flash flash-${type}`;
    flash.setAttribute('data-auto-dismiss', autoDismiss);

    const iconPaths = {
        success: 'M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z',
        error: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z',
        warning: 'M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z',
        info: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z'
    };

    flash.innerHTML = `
        <div class="flash-content">
            <svg class="flash-icon" width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="${iconPaths[type] || iconPaths.info}"/>
            </svg>
            <span class="flash-message">${escapeHtml(message)}</span>
        </div>
        <button class="flash-close">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
            </svg>
        </button>
    `;

    flashContainer.appendChild(flash);

    // Auto-dismiss if enabled
    if (autoDismiss) {
        setTimeout(() => {
            dismissFlashMessage(flash);
        }, window.Caslink.config.flashMessageDuration);
    }

    return flash;
}

function createFlashContainer() {
    const container = document.createElement('div');
    container.className = 'flash-messages';
    document.querySelector('.main-content').prepend(container);
    return container;
}

/**
 * Tooltip functionality
 */
function initializeTooltips() {
    const tooltipElements = document.querySelectorAll('[data-tooltip]');

    tooltipElements.forEach(element => {
        let tooltipTimeout;
        let tooltip;

        element.addEventListener('mouseenter', function() {
            tooltipTimeout = setTimeout(() => {
                tooltip = createTooltip(this.getAttribute('data-tooltip'));
                document.body.appendChild(tooltip);
                positionTooltip(tooltip, this);
            }, window.Caslink.config.tooltipDelay);
        });

        element.addEventListener('mouseleave', function() {
            clearTimeout(tooltipTimeout);
            if (tooltip) {
                tooltip.remove();
                tooltip = null;
            }
        });

        element.addEventListener('mousemove', function(e) {
            if (tooltip) {
                positionTooltip(tooltip, this, e);
            }
        });
    });
}

function createTooltip(text) {
    const tooltip = document.createElement('div');
    tooltip.className = 'tooltip';
    tooltip.textContent = text;
    tooltip.style.cssText = `
        position: absolute;
        background-color: var(--text-color);
        color: var(--bg-color);
        padding: var(--spacing-xs) var(--spacing-sm);
        border-radius: var(--radius-sm);
        font-size: 0.75rem;
        white-space: nowrap;
        z-index: 1004;
        opacity: 0;
        transition: opacity 0.2s ease;
        pointer-events: none;
    `;

    setTimeout(() => {
        tooltip.style.opacity = '1';
    }, 10);

    return tooltip;
}

function positionTooltip(tooltip, element, mouseEvent = null) {
    const rect = element.getBoundingClientRect();
    const tooltipRect = tooltip.getBoundingClientRect();

    let x, y;

    if (mouseEvent) {
        x = mouseEvent.clientX - tooltipRect.width / 2;
        y = mouseEvent.clientY - tooltipRect.height - 10;
    } else {
        x = rect.left + rect.width / 2 - tooltipRect.width / 2;
        y = rect.top - tooltipRect.height - 10;
    }

    // Keep tooltip within viewport
    if (x < 10) x = 10;
    if (x + tooltipRect.width > window.innerWidth - 10) {
        x = window.innerWidth - tooltipRect.width - 10;
    }
    if (y < 10) {
        y = rect.bottom + 10;
    }

    tooltip.style.left = x + 'px';
    tooltip.style.top = y + 'px';
}

/**
 * Theme toggle functionality
 */
function initializeThemeToggle() {
    // Check for saved theme preference or default to 'dark'
    const savedTheme = localStorage.getItem('caslink-theme') || 'dark';
    setTheme(savedTheme);

    // Create theme toggle button if it doesn't exist
    const existingToggle = document.getElementById('theme-toggle');
    if (!existingToggle) {
        createThemeToggle();
    }
}

function setTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('caslink-theme', theme);

    // Update theme toggle button if it exists
    const themeToggle = document.getElementById('theme-toggle');
    if (themeToggle) {
        updateThemeToggleIcon(themeToggle, theme);
    }
}

function createThemeToggle() {
    const navUser = document.querySelector('.nav-user');
    if (!navUser) return;

    const themeToggle = document.createElement('button');
    themeToggle.id = 'theme-toggle';
    themeToggle.className = 'action-btn';
    themeToggle.setAttribute('data-tooltip', 'Toggle theme');
    themeToggle.style.marginRight = 'var(--spacing-sm)';

    const currentTheme = document.documentElement.getAttribute('data-theme') || 'dark';
    updateThemeToggleIcon(themeToggle, currentTheme);

    themeToggle.addEventListener('click', function() {
        const currentTheme = document.documentElement.getAttribute('data-theme') || 'dark';
        const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
        setTheme(newTheme);
    });

    navUser.parentNode.insertBefore(themeToggle, navUser);
}

function updateThemeToggleIcon(button, theme) {
    const iconPath = theme === 'dark'
        ? 'M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z' // Sun icon for light mode
        : 'M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a8.959 8.959 0 008.354-5.646z'; // Moon icon for dark mode

    button.innerHTML = `
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="${iconPath}"/>
        </svg>
    `;
}

/**
 * Form validation
 */
function initializeFormValidation() {
    const forms = document.querySelectorAll('form[data-validate]');

    forms.forEach(form => {
        form.addEventListener('submit', function(e) {
            if (!validateForm(this)) {
                e.preventDefault();
            }
        });

        // Real-time validation
        const inputs = form.querySelectorAll('input, textarea, select');
        inputs.forEach(input => {
            input.addEventListener('blur', function() {
                validateField(this);
            });

            input.addEventListener('input', debounce(function() {
                if (this.classList.contains('error')) {
                    validateField(this);
                }
            }, window.Caslink.config.debounceDelay));
        });
    });
}

function validateForm(form) {
    const inputs = form.querySelectorAll('input[required], textarea[required], select[required]');
    let isValid = true;

    inputs.forEach(input => {
        if (!validateField(input)) {
            isValid = false;
        }
    });

    return isValid;
}

function validateField(field) {
    const value = field.value.trim();
    const type = field.type;
    const pattern = field.pattern;
    let isValid = true;
    let errorMessage = '';

    // Remove existing error state
    field.classList.remove('error');
    removeFieldError(field);

    // Required validation
    if (field.hasAttribute('required') && !value) {
        isValid = false;
        errorMessage = 'This field is required';
    }

    // Type-specific validation
    if (value && isValid) {
        switch (type) {
            case 'email':
                if (!isValidEmail(value)) {
                    isValid = false;
                    errorMessage = 'Please enter a valid email address';
                }
                break;
            case 'url':
                if (!isValidURL(value)) {
                    isValid = false;
                    errorMessage = 'Please enter a valid URL';
                }
                break;
            case 'password':
                const minLength = field.getAttribute('minlength') || 8;
                if (value.length < minLength) {
                    isValid = false;
                    errorMessage = `Password must be at least ${minLength} characters`;
                }
                break;
        }
    }

    // Pattern validation
    if (value && pattern && isValid) {
        const regex = new RegExp(pattern);
        if (!regex.test(value)) {
            isValid = false;
            errorMessage = field.getAttribute('title') || 'Invalid format';
        }
    }

    // Show error if invalid
    if (!isValid) {
        field.classList.add('error');
        showFieldError(field, errorMessage);
    }

    return isValid;
}

function showFieldError(field, message) {
    const errorElement = document.createElement('div');
    errorElement.className = 'field-error';
    errorElement.textContent = message;
    errorElement.style.cssText = `
        color: var(--error-color);
        font-size: 0.875rem;
        margin-top: var(--spacing-xs);
    `;

    field.parentNode.appendChild(errorElement);
}

function removeFieldError(field) {
    const errorElement = field.parentNode.querySelector('.field-error');
    if (errorElement) {
        errorElement.remove();
    }
}

/**
 * Keyboard shortcuts
 */
function initializeKeyboardShortcuts() {
    document.addEventListener('keydown', function(e) {
        // Ctrl/Cmd + K: Focus search/URL input
        if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
            e.preventDefault();
            const urlInput = document.getElementById('original_url') || document.querySelector('input[type="search"], input[type="url"]');
            if (urlInput) {
                urlInput.focus();
                urlInput.select();
            }
        }

        // Escape: Close modals, dropdowns, etc.
        if (e.key === 'Escape') {
            closeAllDropdowns();
            closeAllModals();
        }

        // Alt + T: Toggle theme
        if (e.altKey && e.key === 't') {
            e.preventDefault();
            const themeToggle = document.getElementById('theme-toggle');
            if (themeToggle) {
                themeToggle.click();
            }
        }
    });
}

function closeAllDropdowns() {
    const dropdownMenus = document.querySelectorAll('.dropdown-menu');
    dropdownMenus.forEach(menu => {
        menu.style.opacity = '0';
        menu.style.visibility = 'hidden';
        menu.style.transform = 'translateY(-10px)';
    });
}

function closeAllModals() {
    const modals = document.querySelectorAll('.modal');
    modals.forEach(modal => {
        modal.style.display = 'none';
    });
}

/**
 * Utility functions
 */

// Debounce function
function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

// Throttle function
function throttle(func, limit) {
    let inThrottle;
    return function() {
        const args = arguments;
        const context = this;
        if (!inThrottle) {
            func.apply(context, args);
            inThrottle = true;
            setTimeout(() => inThrottle = false, limit);
        }
    };
}

// HTML escape
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Email validation
function isValidEmail(email) {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
}

// URL validation
function isValidURL(string) {
    try {
        new URL(string);
        return true;
    } catch (_) {
        return false;
    }
}

// Copy to clipboard
async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
        return true;
    } catch (error) {
        // Fallback for older browsers
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        const success = document.execCommand('copy');
        document.body.removeChild(textarea);
        return success;
    }
}

// Format numbers with commas
function formatNumber(num) {
    return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

// Format file size
function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Format date
function formatDate(date, options = {}) {
    const defaultOptions = {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    };

    return new Intl.DateTimeFormat('en-US', { ...defaultOptions, ...options }).format(new Date(date));
}

// API helper functions
const API = {
    async request(endpoint, options = {}) {
        const url = `${window.Caslink.apiURL}${endpoint}`;
        const defaultOptions = {
            headers: {
                'Content-Type': 'application/json',
            },
        };

        const mergedOptions = { ...defaultOptions, ...options };

        try {
            const response = await fetch(url, mergedOptions);
            const data = await response.json();

            if (!response.ok) {
                throw new Error(data.error || `HTTP error! status: ${response.status}`);
            }

            return data;
        } catch (error) {
            console.error('API request failed:', error);
            throw error;
        }
    },

    async get(endpoint) {
        return this.request(endpoint, { method: 'GET' });
    },

    async post(endpoint, data) {
        return this.request(endpoint, {
            method: 'POST',
            body: JSON.stringify(data),
        });
    },

    async put(endpoint, data) {
        return this.request(endpoint, {
            method: 'PUT',
            body: JSON.stringify(data),
        });
    },

    async delete(endpoint) {
        return this.request(endpoint, { method: 'DELETE' });
    }
};

// Export API for global use
window.Caslink.API = API;
window.Caslink.utils = {
    debounce,
    throttle,
    escapeHtml,
    isValidEmail,
    isValidURL,
    copyToClipboard,
    formatNumber,
    formatFileSize,
    formatDate,
    showFlashMessage
};

// Service Worker registration (if available)
if ('serviceWorker' in navigator) {
    window.addEventListener('load', function() {
        navigator.serviceWorker.register('/sw.js')
            .then(function(registration) {
                console.log('ServiceWorker registration successful');
            })
            .catch(function(error) {
                console.log('ServiceWorker registration failed');
            });
    });
}