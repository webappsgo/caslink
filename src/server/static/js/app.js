// Caslink — application JavaScript
// Runs without any bundler. No CDN dependencies.

(function () {
  'use strict';

  // ---- Theme management -----------------------------------------------

  const THEME_KEY = 'cl_theme';
  const THEME_ATTR = 'data-theme';

  function applyTheme(theme) {
    document.documentElement.setAttribute(THEME_ATTR, theme);
    // Persist non-auto choices in a cookie so the server can read it.
    if (theme !== 'auto') {
      document.cookie = 'theme=' + theme + '; path=/; max-age=31536000; SameSite=Lax';
    } else {
      document.cookie = 'theme=; path=/; max-age=0; SameSite=Lax';
    }
    localStorage.setItem(THEME_KEY, theme);
    // Update toggle button label if present.
    var btn = document.getElementById('theme-toggle');
    if (btn) {
      var icons = { dark: '☀️', light: '🌙', auto: '🖥️' };
      btn.textContent = icons[theme] || '🖥️';
      btn.title = 'Switch theme (current: ' + theme + ')';
    }
  }

  function cycleTheme() {
    var current = localStorage.getItem(THEME_KEY) || 'auto';
    var next = { auto: 'dark', dark: 'light', light: 'auto' }[current] || 'auto';
    applyTheme(next);
  }

  // Restore theme on page load.
  (function () {
    var saved = localStorage.getItem(THEME_KEY) || 'auto';
    applyTheme(saved);
  })();

  // Wire toggle button once DOM is ready.
  document.addEventListener('DOMContentLoaded', function () {
    var btn = document.getElementById('theme-toggle');
    if (btn) {
      btn.addEventListener('click', cycleTheme);
    }
  });

  // ---- CSRF injection for fetch() ------------------------------------

  function getCSRFToken() {
    // 1. Try <meta name="csrf-token"> (server-rendered)
    var meta = document.querySelector('meta[name="csrf-token"]');
    if (meta) return meta.getAttribute('content');
    // 2. Try cookie
    var match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
    return match ? decodeURIComponent(match[1]) : '';
  }

  // Patch global fetch to add CSRF header on same-origin state-mutating calls.
  var _origFetch = window.fetch;
  window.fetch = function (input, init) {
    init = init || {};
    var method = (init.method || 'GET').toUpperCase();
    var safeMethods = ['GET', 'HEAD', 'OPTIONS', 'TRACE'];

    if (safeMethods.indexOf(method) === -1) {
      // Only inject if not already set and not a Bearer-auth call.
      init.headers = init.headers || {};
      var headers = init.headers;
      var hasAuth = false;
      if (headers instanceof Headers) {
        hasAuth = headers.has('Authorization');
        if (!hasAuth && !headers.has('X-CSRF-Token')) {
          headers.set('X-CSRF-Token', getCSRFToken());
        }
      } else if (typeof headers === 'object') {
        hasAuth = !!(headers['Authorization'] || headers['authorization']);
        if (!hasAuth && !headers['X-CSRF-Token']) {
          headers['X-CSRF-Token'] = getCSRFToken();
        }
      }
    }
    return _origFetch.call(window, input, init);
  };

  // ---- Copy-to-clipboard buttons ------------------------------------

  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('[data-copy]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var target = btn.getAttribute('data-copy');
        var text = target
          ? (document.getElementById(target) || { textContent: target }).textContent
          : btn.getAttribute('data-copy-text') || '';

        if (!navigator.clipboard) {
          // Fallback for non-secure contexts.
          var ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.opacity = '0';
          document.body.appendChild(ta);
          ta.select();
          try { document.execCommand('copy'); } catch (_) {}
          document.body.removeChild(ta);
        } else {
          navigator.clipboard.writeText(text).catch(function () {});
        }

        var orig = btn.textContent;
        btn.textContent = 'Copied!';
        setTimeout(function () { btn.textContent = orig; }, 1500);
      });
    });
  });

  // ---- Flash message auto-dismiss -----------------------------------

  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('.alert[data-autohide]').forEach(function (el) {
      var delay = parseInt(el.getAttribute('data-autohide') || '4000', 10);
      setTimeout(function () {
        el.style.transition = 'opacity .4s';
        el.style.opacity = '0';
        setTimeout(function () { el.remove(); }, 400);
      }, delay);
    });
  });

  // ---- PWA install prompt -------------------------------------------

  var deferredInstallPrompt;
  window.addEventListener('beforeinstallprompt', function (e) {
    e.preventDefault();
    deferredInstallPrompt = e;
    var btn = document.getElementById('pwa-install');
    if (btn) {
      btn.style.display = 'inline-flex';
      btn.addEventListener('click', function () {
        deferredInstallPrompt.prompt();
        deferredInstallPrompt.userChoice.then(function () {
          deferredInstallPrompt = null;
          btn.style.display = 'none';
        });
      });
    }
  });

})();
