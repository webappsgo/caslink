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

  // ---- Confirm-before-submit forms -----------------------------------
  // Progressive enhancement only: without JS the form just submits
  // directly (server still enforces the action), per AI.md PART 16.

  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('form[data-confirm]').forEach(function (form) {
      form.addEventListener('submit', function (e) {
        if (!window.confirm(form.getAttribute('data-confirm'))) {
          e.preventDefault();
        }
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

  // ---- Theme selection buttons (users/settings) ----------------------
  // Replaces the former inline onclick handlers; applies the theme live
  // with no page reload (seamless switching per AI.md PART 16).

  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('[data-theme-set]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        applyTheme(btn.getAttribute('data-theme-set'));
      });
    });
  });

  // ---- Language selector auto-submit ---------------------------------
  // Replaces the former inline onchange="this.form.submit()".

  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('[data-autosubmit]').forEach(function (el) {
      el.addEventListener('change', function () {
        if (el.form) el.form.submit();
      });
    });
  });

  // ---- Org slug auto-generation (orgs/new) ---------------------------
  // Replaces the former inline autoSlug() script block.

  document.addEventListener('DOMContentLoaded', function () {
    var source = document.querySelector('[data-slug-source]');
    var slug = document.getElementById('slug');
    if (!source || !slug) return;
    source.addEventListener('input', function () {
      if (!slug.dataset.edited) {
        slug.value = source.value.toLowerCase()
          .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40);
      }
    });
    slug.addEventListener('input', function () { slug.dataset.edited = 'true'; });
  });

  // ---- Short-link create form (dashboard) ----------------------------
  // Replaces the former dashboard inline-js block. CSRF is injected by the
  // window.fetch patch above, so no explicit header is needed here.

  document.addEventListener('DOMContentLoaded', function () {
    var form = document.getElementById('create-form');
    if (!form) return;
    var result = document.getElementById('create-result');
    var shortLink = document.getElementById('short-link');
    var copyBtn = document.getElementById('copy-btn');

    function showToast(msg, type) {
      var t = document.createElement('div');
      t.className = 'alert alert-' + (type || 'info');
      t.style.cssText = 'position:fixed;top:16px;right:16px;z-index:9999;max-width:400px';
      t.textContent = msg;
      document.body.appendChild(t);
      setTimeout(function () { t.remove(); }, 5000);
    }

    form.addEventListener('submit', function (e) {
      e.preventDefault();
      var data = { long_url: form.long_url.value };
      if (form.custom_code.value) data.custom_code = form.custom_code.value;
      fetch('/api/v1/urls', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify(data)
      }).then(function (r) { return r.json(); }).then(function (j) {
        if (j.ok && j.data && j.data.short_url) {
          shortLink.href = j.data.short_url;
          shortLink.textContent = j.data.short_url;
          copyBtn.setAttribute('data-copy-text', j.data.short_url);
          result.classList.remove('hidden');
          form.reset();
        } else {
          showToast(j.message || j.error || 'Failed to create link', 'danger');
        }
      }).catch(function () { showToast('Network error — check your connection', 'danger'); });
    });
  });

  // ---- Recovery keys (users/security/recovery-keys) ------------------
  // Keys are passed as a non-executable JSON island; replaces the former
  // inline <script> that embedded {{.KeysJSON}} directly.

  document.addEventListener('DOMContentLoaded', function () {
    var dataEl = document.getElementById('recovery-keys-data');
    var downloadBtn = document.getElementById('download-keys-btn');
    if (!dataEl || !downloadBtn) return;
    var keys;
    try { keys = JSON.parse(dataEl.textContent); } catch (e) { return; }
    var keysText = keys.map(function (k, i) { return (i + 1) + '. ' + k; }).join('\n');

    downloadBtn.addEventListener('click', function () {
      var blob = new Blob([keysText], { type: 'text/plain' });
      var url = URL.createObjectURL(blob);
      var a = document.createElement('a');
      a.href = url;
      a.download = 'caslink-recovery-keys.txt';
      a.click();
      URL.revokeObjectURL(url);
    });

    var copyBtn = document.getElementById('copy-keys-btn');
    if (copyBtn) {
      copyBtn.addEventListener('click', function () {
        navigator.clipboard.writeText(keysText).then(function () {
          copyBtn.textContent = 'Copied!';
          setTimeout(function () { copyBtn.textContent = 'Copy All Keys'; }, 2000);
        });
      });
    }

    var confirmed = document.getElementById('confirmed');
    var continueBtn = document.getElementById('continue-btn');
    if (confirmed && continueBtn) {
      confirmed.addEventListener('change', function () {
        continueBtn.disabled = !confirmed.checked;
      });
    }
  });

  // ---- Passkey registration (users/security/passkeys) ----------------
  // Replaces the former inline WebAuthn <script>. Uses fixed endpoints and
  // no template data, so it lives entirely in this external file.

  document.addEventListener('DOMContentLoaded', function () {
    var registerBtn = document.getElementById('register-btn');
    if (!registerBtn) return;

    function showStatus(type, msg) {
      var el = document.getElementById('register-status');
      el.className = 'mt-3 alert alert-' + (type === 'error' ? 'danger' : 'success');
      el.textContent = msg;
      el.classList.remove('hidden');
    }

    if (!window.PublicKeyCredential) {
      registerBtn.disabled = true;
      showStatus('error', 'Your browser does not support passkeys. Try Chrome, Safari 16+, Firefox 119+, or Edge.');
      return;
    }

    function bufferDecode(value) {
      return Uint8Array.from(atob(value.replace(/-/g, '+').replace(/_/g, '/')), function (c) { return c.charCodeAt(0); });
    }

    function bufferEncode(value) {
      return btoa(String.fromCharCode.apply(null, new Uint8Array(value)))
        .replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
    }

    registerBtn.addEventListener('click', function () {
      var name = document.getElementById('passkey-name').value.trim() || 'Passkey';
      var btn = this;
      btn.disabled = true;
      btn.textContent = 'Waiting for authenticator…';

      fetch('/users/passkeys/begin-register', {
        method: 'POST',
        credentials: 'same-origin'
      })
      .then(function (r) { return r.json(); })
      .then(function (body) {
        if (!body.ok) throw new Error(body.message || body.error);
        var opts = body.data;
        opts.publicKey.challenge = bufferDecode(opts.publicKey.challenge);
        opts.publicKey.user.id = bufferDecode(opts.publicKey.user.id);
        if (opts.publicKey.excludeCredentials) {
          opts.publicKey.excludeCredentials = opts.publicKey.excludeCredentials.map(function (c) {
            return { id: bufferDecode(c.id), type: c.type, transports: c.transports };
          });
        }
        return navigator.credentials.create(opts);
      })
      .then(function (cred) {
        var body = {
          id: cred.id,
          rawId: bufferEncode(cred.rawId),
          type: cred.type,
          response: {
            attestationObject: bufferEncode(cred.response.attestationObject),
            clientDataJSON: bufferEncode(cred.response.clientDataJSON)
          }
        };
        return fetch('/users/passkeys/finish-register?name=' + encodeURIComponent(name), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify(body)
        });
      })
      .then(function (r) { return r.json(); })
      .then(function (body) {
        if (!body.ok) throw new Error(body.message || body.error);
        showStatus('success', 'Passkey registered successfully. Refreshing…');
        setTimeout(function () { window.location.reload(); }, 1200);
      })
      .catch(function (err) {
        showStatus('error', err.message || 'Registration failed. Please try again.');
        btn.disabled = false;
        btn.textContent = 'Register Passkey';
      });
    });
  });

})();
