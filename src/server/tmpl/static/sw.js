// Caslink service worker — minimal offline shell caching
const CACHE = 'caslink-v1';
const PRECACHE = ['/static/css/app.css', '/static/js/app.js'];

self.addEventListener('install', function(e) {
  e.waitUntil(
    caches.open(CACHE).then(function(c) { return c.addAll(PRECACHE); })
  );
  self.skipWaiting();
});

self.addEventListener('activate', function(e) {
  e.waitUntil(
    caches.keys().then(function(keys) {
      return Promise.all(keys.filter(function(k) { return k !== CACHE; }).map(function(k) {
        return caches.delete(k);
      }));
    })
  );
  self.clients.claim();
});

self.addEventListener('fetch', function(e) {
  // Only cache GET requests for static assets.
  if (e.request.method !== 'GET') return;
  var url = new URL(e.request.url);
  if (!url.pathname.startsWith('/static/')) return;

  e.respondWith(
    caches.match(e.request).then(function(cached) {
      return cached || fetch(e.request).then(function(r) {
        var clone = r.clone();
        caches.open(CACHE).then(function(c) { c.put(e.request, clone); });
        return r;
      });
    })
  );
});
