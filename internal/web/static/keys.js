// keys.js — keyboard shortcut handler (stub, full implementation in Plan 04)
(function() {
    document.addEventListener('keydown', function(e) {
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') {
            if (e.key === 'Escape') { e.target.blur(); }
            return;
        }
        // Stub: full implementation added in Plan 04
    });
})();
