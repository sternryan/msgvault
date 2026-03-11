// keys.js — full keyboard shortcut handler for msgvault web UI

// Iframe auto-resize: listen for postMessage from email body iframe(s)
// Handles both single iframe (message detail) and multiple iframes (thread view)
window.addEventListener('message', function(e) {
    if (e.data && e.data.type === 'msgvault-resize') {
        // Try to find the specific iframe that sent this message
        var frames = document.querySelectorAll('.email-iframe');
        var matched = false;
        for (var i = 0; i < frames.length; i++) {
            try {
                if (frames[i].contentWindow === e.source) {
                    frames[i].style.height = (e.data.height + 20) + 'px';
                    matched = true;
                    break;
                }
            } catch (ex) {
                // Cross-origin: can't access contentWindow, skip
            }
        }
        // Fallback: if no match (e.g. single iframe without class), try by ID
        if (!matched) {
            var frame = document.getElementById('email-body-frame');
            if (frame) {
                frame.style.height = (e.data.height + 20) + 'px';
            }
        }
    }
});

(function () {
    var currentRow = -1;

    document.addEventListener('keydown', function (e) {
        // Ignore if focused in input/textarea/select
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') {
            if (e.key === 'Escape') {
                e.target.blur();
                e.preventDefault();
            }
            return;
        }

        switch (e.key) {
            case 'j':
            case 'ArrowDown':
                moveRow(1);
                e.preventDefault();
                break;
            case 'k':
            case 'ArrowUp':
                moveRow(-1);
                e.preventDefault();
                break;
            case 'Enter':
                activateRow();
                e.preventDefault();
                break;
            case 'Escape':
            case 'Backspace':
                // If help overlay is active, close it instead of going back
                var helpOverlay = document.getElementById('help-overlay');
                if (helpOverlay && helpOverlay.classList.contains('active')) {
                    helpOverlay.classList.remove('active');
                    e.preventDefault();
                    break;
                }
                goBack();
                e.preventDefault();
                break;
            case 'Tab':
                cycleViewType();
                e.preventDefault();
                break;
            case 's':
                cycleSortField();
                break;
            case 'r':
                reverseSortDir();
                break;
            case 't':
                // On message detail page: navigate to thread view
                // On other pages: navigate to time view (existing behavior)
                if (window.location.pathname.match(/^\/messages\/\d+$/)) {
                    navigateToThread();
                } else {
                    navigateToTimeView();
                }
                break;
            case 'n':
                if (window.location.pathname.startsWith('/threads/')) {
                    navigateThreadMessage(1);
                    e.preventDefault();
                }
                break;
            case 'p':
                if (window.location.pathname.startsWith('/threads/')) {
                    navigateThreadMessage(-1);
                    e.preventDefault();
                }
                break;
            case 'a':
                focusAccountFilter();
                e.preventDefault();
                break;
            case '/':
                focusSearch();
                e.preventDefault();
                break;
            case '?':
                toggleHelp();
                e.preventDefault();
                break;
            case 'q':
                // No-op: prevents accidental tab close
                e.preventDefault();
                break;
        }
    });

    // Account filter: JavaScript URL manipulation on change.
    // This definitively handles account filter propagation by reading the current URL,
    // replacing/adding the sourceId param, and navigating via htmx.ajax to the new URL.
    // This approach correctly preserves all existing query params (groupBy, sortField, q, etc.)
    // across all pages, which HTMX's native hx-get approach cannot do without duplicating params.
    document.addEventListener('DOMContentLoaded', function () {
        setupAccountFilter();
        setupThreadHighlight();
    });
    // Re-setup after HTMX swaps (in case layout is re-rendered)
    document.body.addEventListener('htmx:afterSettle', function () {
        setupAccountFilter();
        setupThreadHighlight();
    });

    function setupAccountFilter() {
        var select = document.getElementById('account-filter');
        if (!select || select.dataset.bound) return;
        select.dataset.bound = 'true';
        select.addEventListener('change', function () {
            var url = new URL(window.location.href);
            var sourceId = select.value;
            if (sourceId) {
                url.searchParams.set('sourceId', sourceId);
            } else {
                url.searchParams.delete('sourceId');
            }
            var targetUrl = url.pathname + url.search;
            htmx.ajax('GET', targetUrl, {
                target: '#main-content',
                select: '#main-content',
                swap: 'outerHTML'
            }).then(function () {
                history.replaceState({}, '', targetUrl);
            });
        });
    }

    // Reset row focus on HTMX content swap
    document.body.addEventListener('htmx:afterSwap', function () {
        currentRow = -1;
    });

    function moveRow(delta) {
        var rows = document.querySelectorAll('[data-row]');
        if (!rows.length) return;
        if (currentRow >= 0 && currentRow < rows.length) {
            rows[currentRow].classList.remove('row-focused');
        }
        currentRow = Math.max(0, Math.min(rows.length - 1, currentRow + delta));
        rows[currentRow].classList.add('row-focused');
        rows[currentRow].scrollIntoView({ block: 'nearest' });
    }

    function activateRow() {
        var row = document.querySelector('.row-focused');
        if (!row) return;
        var href = row.dataset.href;
        if (href) {
            htmx.ajax('GET', href, {
                target: '#main-content',
                select: '#main-content',
                swap: 'outerHTML'
            }).then(function () {
                history.pushState({}, '', href);
            });
        }
    }

    function goBack() {
        window.history.back();
    }

    function cycleViewType() {
        // Only works on aggregate page — cycle through view type tabs
        var tabs = document.querySelectorAll('.view-tab');
        if (!tabs.length) return;
        var activeIdx = -1;
        tabs.forEach(function (tab, i) {
            if (tab.classList.contains('active')) activeIdx = i;
        });
        var nextIdx = (activeIdx + 1) % tabs.length;
        var nextTab = tabs[nextIdx];
        if (nextTab) {
            var href = nextTab.getAttribute('hx-get') || nextTab.getAttribute('href');
            if (href) {
                htmx.ajax('GET', href, {
                    target: '#main-content',
                    select: '#main-content',
                    swap: 'outerHTML'
                }).then(function () {
                    history.pushState({}, '', href);
                });
            }
        }
    }

    function cycleSortField() {
        // Find sort header links and cycle through them
        var sortLinks = document.querySelectorAll('.sort-header[data-sort-field]');
        if (!sortLinks.length) return;
        var activeField = document.querySelector('.sort-header.active');
        var activeIdx = -1;
        sortLinks.forEach(function (link, i) {
            if (link === activeField) activeIdx = i;
        });
        var nextIdx = (activeIdx + 1) % sortLinks.length;
        var nextLink = sortLinks[nextIdx];
        if (nextLink) {
            nextLink.click();
        }
    }

    function reverseSortDir() {
        // Find the active sort header and click its reverse link
        var activeSort = document.querySelector('.sort-header.active');
        if (activeSort) {
            activeSort.click();
        }
    }

    function navigateToTimeView() {
        // Navigate to Time aggregate view
        var timeTab = document.querySelector('.view-tab[data-view="time"]');
        if (timeTab) {
            timeTab.click();
        } else {
            htmx.ajax('GET', '/aggregate?groupBy=time', {
                target: '#main-content',
                select: '#main-content',
                swap: 'outerHTML'
            }).then(function () {
                history.pushState({}, '', '/aggregate?groupBy=time');
            });
        }
    }

    function focusAccountFilter() {
        var select = document.getElementById('account-filter');
        if (select) select.focus();
    }

    function focusSearch() {
        var input = document.querySelector('.search-input');
        if (input) {
            input.focus();
        } else {
            // Navigate to search page
            htmx.ajax('GET', '/search', {
                target: '#main-content',
                select: '#main-content',
                swap: 'outerHTML'
            }).then(function () {
                history.pushState({}, '', '/search');
                // Focus after render
                setTimeout(function () {
                    var si = document.querySelector('.search-input');
                    if (si) si.focus();
                }, 100);
            });
        }
    }

    function navigateToThread() {
        var link = document.getElementById('view-thread-link');
        if (link) {
            var href = link.getAttribute('href');
            if (href) {
                htmx.ajax('GET', href, {
                    target: '#main-content',
                    select: '#main-content',
                    swap: 'outerHTML'
                }).then(function() {
                    history.pushState({}, '', href);
                });
            }
        }
    }

    function navigateThreadMessage(delta) {
        var msgs = Array.from(document.querySelectorAll('.thread-message[data-msg-id]'));
        if (!msgs.length) return;

        var focusedIdx = msgs.findIndex(function(el) {
            return el.classList.contains('thread-focused');
        });

        // If no current focus, start from the open/latest message
        if (focusedIdx < 0) {
            focusedIdx = msgs.findIndex(function(el) { return el.open; });
            if (focusedIdx < 0) focusedIdx = msgs.length - 1;
        }

        // Remove current focus
        msgs[focusedIdx].classList.remove('thread-focused');

        // Calculate next with wrap-around
        var nextIdx = (focusedIdx + delta + msgs.length) % msgs.length;
        var nextMsg = msgs[nextIdx];

        nextMsg.classList.add('thread-focused');
        nextMsg.open = true;  // expand if collapsed
        nextMsg.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function setupThreadHighlight() {
        var container = document.getElementById('thread-container');
        if (!container) return;
        var highlightId = container.dataset.highlight;
        if (!highlightId || highlightId === '0') return;
        // Only run once per container load
        if (container.dataset.highlightApplied) return;
        container.dataset.highlightApplied = 'true';

        var el = document.getElementById('msg-' + highlightId);
        if (el) {
            el.open = true;
            el.classList.add('thread-focused');
            // Delay to let iframe height settle before scrolling
            setTimeout(function() {
                el.scrollIntoView({ behavior: 'smooth', block: 'start' });
            }, 150);
        }
    }

    function toggleHelp() {
        var overlay = document.getElementById('help-overlay');
        if (!overlay) {
            // Create help overlay dynamically
            overlay = document.createElement('div');
            overlay.id = 'help-overlay';
            overlay.className = 'help-overlay active';
            overlay.innerHTML =
                '<div class="help-content">' +
                '<h2>Keyboard Shortcuts</h2>' +
                '<table>' +
                '<tr><td><kbd>j</kbd> / <kbd>k</kbd></td><td>Navigate rows</td></tr>' +
                '<tr><td><kbd>Enter</kbd></td><td>Open / drill down</td></tr>' +
                '<tr><td><kbd>Esc</kbd></td><td>Go back</td></tr>' +
                '<tr><td><kbd>Tab</kbd></td><td>Cycle view types</td></tr>' +
                '<tr><td><kbd>s</kbd></td><td>Cycle sort field</td></tr>' +
                '<tr><td><kbd>r</kbd></td><td>Reverse sort</td></tr>' +
                '<tr><td><kbd>t</kbd></td><td>Time view / Thread (on message)</td></tr>' +
                '<tr><td><kbd>n</kbd></td><td>Next thread message</td></tr>' +
                '<tr><td><kbd>p</kbd></td><td>Previous thread message</td></tr>' +
                '<tr><td><kbd>a</kbd></td><td>Account filter</td></tr>' +
                '<tr><td><kbd>/</kbd></td><td>Search</td></tr>' +
                '<tr><td><kbd>?</kbd></td><td>Toggle help</td></tr>' +
                '<tr><td><kbd>q</kbd></td><td>No-op (browser tab safe)</td></tr>' +
                '</table>' +
                '<p class="help-dismiss">Press <kbd>?</kbd> or <kbd>Esc</kbd> to close</p>' +
                '</div>';
            overlay.addEventListener('click', function (ev) {
                if (ev.target === overlay) overlay.classList.remove('active');
            });
            document.body.appendChild(overlay);
        } else {
            overlay.classList.toggle('active');
        }
    }
})();
