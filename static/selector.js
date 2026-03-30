// Injected selector script for visual web scraper
(function () {
    'use strict';

    const HIGHLIGHT_CLASS = 'ws-highlight-element';
    const SELECTED_CLASS = 'ws-selected-element';

    // Container tags that are likely list/group containers
    const CONTAINER_TAGS = new Set([
        'ul', 'ol', 'dl', 'table', 'tbody', 'thead', 'tfoot',
        'nav', 'section', 'article', 'main', 'aside', 'header', 'footer',
        'div', 'form', 'fieldset', 'details', 'figure'
    ]);

    // Leaf tags that are NOT containers
    const LEAF_TAGS = new Set([
        'a', 'img', 'span', 'p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
        'input', 'button', 'label', 'textarea', 'select', 'option',
        'strong', 'em', 'b', 'i', 'u', 'code', 'pre', 'time',
        'br', 'hr', 'svg', 'canvas', 'video', 'audio', 'source'
    ]);

    // Add highlight styles
    const style = document.createElement('style');
    style.textContent = `
        .${HIGHLIGHT_CLASS} {
            outline: 3px solid #8b5cf6 !important;
            outline-offset: 2px !important;
            cursor: crosshair !important;
            background-color: rgba(139, 92, 246, 0.08) !important;
            transition: outline-color 0.15s, background-color 0.15s !important;
        }
        .${HIGHLIGHT_CLASS}::after {
            content: attr(data-ws-tag) !important;
            position: absolute !important;
            top: -22px !important;
            left: 0 !important;
            background: #8b5cf6 !important;
            color: #fff !important;
            font-size: 10px !important;
            font-family: monospace !important;
            padding: 1px 6px !important;
            border-radius: 4px !important;
            z-index: 999999 !important;
            pointer-events: none !important;
            white-space: nowrap !important;
            line-height: 16px !important;
        }
        .${SELECTED_CLASS} {
            outline: 3px solid #22c55e !important;
            outline-offset: 2px !important;
            background-color: rgba(34, 197, 94, 0.1) !important;
        }
        * {
            cursor: crosshair !important;
        }
        html, body {
            overflow: auto !important;
            height: auto !important;
            min-height: 100% !important;
            position: static !important;
        }
    `;
    document.head.appendChild(style);

    let highlightedElement = null;

    function isSafeClass(cls) {
        if (!cls || cls.length === 0) return false;
        if (cls.startsWith('ws-')) return false;
        if (/[:\\\\/\[\]()!@#$%^&*+={}|<>?,`~"']/.test(cls)) return false;
        if (/^\d/.test(cls)) return false;
        if (/^(sm|md|lg|xl|2xl|hover|focus|active|group|peer|first|last|odd|even|dark):/.test(cls)) return false;
        return true;
    }

    function cssEscape(str) {
        if (typeof CSS !== 'undefined' && CSS.escape) {
            return CSS.escape(str);
        }
        return str.replace(/([^\w-])/g, '\\$1')
            .replace(/^(\d)/, '\\3$1 ');
    }

    function generateSelector(element) {
        const path = [];
        let current = element;
        let depth = 0;
        const MAX_DEPTH = 6;

        while (current && current.nodeType === Node.ELEMENT_NODE && depth < MAX_DEPTH) {
            const tag = current.nodeName.toLowerCase();
            if (tag === 'html' || tag === 'body') break;

            let segment = tag;

            if (current.id) {
                segment = tag + '[id="' + current.id.replace(/"/g, '\\"') + '"]';
                path.unshift(segment);
                break;
            }

            let nth = 0;
            let sameTagCount = 0;
            if (current.parentElement) {
                const children = current.parentElement.children;
                for (let i = 0; i < children.length; i++) {
                    if (children[i].nodeName === current.nodeName) {
                        sameTagCount++;
                        if (children[i] === current) {
                            nth = sameTagCount;
                        }
                    }
                }
            }

            if (current.className && typeof current.className === 'string') {
                const safeClasses = current.className.split(/\s+/)
                    .filter(isSafeClass)
                    .slice(0, 2);
                if (safeClasses.length > 0) {
                    segment += '.' + safeClasses.join('.');
                }
            }

            if (sameTagCount > 1 && nth > 0) {
                segment += ':nth-of-type(' + nth + ')';
            }

            path.unshift(segment);
            current = current.parentElement;
            depth++;
        }

        const selector = path.join(' > ');

        try {
            const match = document.querySelector(selector);
            if (match === element) {
                return selector;
            }
        } catch (e) { }

        return buildPositionalSelector(element);
    }

    function buildPositionalSelector(element) {
        const path = [];
        let current = element;

        while (current && current.nodeType === Node.ELEMENT_NODE) {
            const tag = current.nodeName.toLowerCase();
            if (tag === 'html' || tag === 'body') break;

            if (current.id) {
                path.unshift(tag + '[id="' + current.id.replace(/"/g, '\\"') + '"]');
                break;
            }

            let nth = 0;
            let count = 0;
            if (current.parentElement) {
                const children = current.parentElement.children;
                for (let i = 0; i < children.length; i++) {
                    if (children[i].nodeName === current.nodeName) {
                        count++;
                        if (children[i] === current) {
                            nth = count;
                        }
                    }
                }
            }

            let segment = tag;
            if (count > 1) {
                segment += ':nth-of-type(' + nth + ')';
            }

            path.unshift(segment);
            current = current.parentElement;
        }

        return path.join(' > ');
    }

    function countMatches(selector) {
        try {
            return document.querySelectorAll(selector).length;
        } catch (e) {
            return 0;
        }
    }

    // Detect if element is a container with meaningful children
    function isContainer(el) {
        const tag = el.tagName.toLowerCase();
        if (LEAF_TAGS.has(tag)) return false;
        const childElements = el.children;
        return childElements.length >= 1;
    }

    // Gather child element info for container picker (recursive with depth limit)
    function getChildrenInfo(containerEl, depth) {
        if (depth === undefined) depth = 0;
        if (depth >= 4) return []; // max drill-down depth

        const containerSelector = generateSelector(containerEl);
        const children = Array.from(containerEl.children);
        const items = [];

        const seen = new Set();
        children.slice(0, 20).forEach(child => {
            const tag = child.tagName.toLowerCase();
            const text = (child.innerText || child.textContent || '').trim().substring(0, 80);
            const hasHref = child.hasAttribute('href') || child.querySelector('a[href]') !== null;
            const hasSrc = child.hasAttribute('src') || child.querySelector('img[src]') !== null;
            const childSelector = generateSelector(child);

            const siblingSelector = containerSelector + ' > ' + tag;
            const siblingCount = countMatches(siblingSelector);

            const key = tag + ':' + (siblingCount > 1 ? 'multi' : child.className);
            if (seen.has(key) && seen.size > 6) return;
            seen.add(key);

            const childIsContainer = isContainer(child);

            items.push({
                tag: tag,
                text: text,
                selector: childSelector,
                siblingSelector: siblingSelector,
                siblingCount: siblingCount,
                hasHref: hasHref,
                hasSrc: hasSrc,
                hrefValue: child.hasAttribute('href') ? child.getAttribute('href') :
                    (child.querySelector('a[href]') ? child.querySelector('a[href]').getAttribute('href') : ''),
                srcValue: child.hasAttribute('src') ? child.getAttribute('src') :
                    (child.querySelector('img[src]') ? child.querySelector('img[src]').getAttribute('src') : ''),
                isContainer: childIsContainer,
                childrenInfo: childIsContainer ? getChildrenInfo(child, depth + 1) : []
            });
        });

        return items;
    }


    // Mouseover handler
    function onMouseOver(e) {
        if (highlightedElement && highlightedElement !== e.target) {
            highlightedElement.classList.remove(HIGHLIGHT_CLASS);
            delete highlightedElement.dataset.wsTag;
        }
        highlightedElement = e.target;
        const tag = e.target.tagName.toLowerCase();
        const childCount = e.target.children.length;
        let info = tag;
        if (e.target.hasAttribute('href')) info += ' [href]';
        if (e.target.hasAttribute('src')) info += ' [src]';
        if (isContainer(e.target)) info += ' (' + childCount + ')';
        e.target.dataset.wsTag = info;
        e.target.classList.add(HIGHLIGHT_CLASS);
    }

    // Mouseout handler
    function onMouseOut(e) {
        if (e.target.classList) {
            e.target.classList.remove(HIGHLIGHT_CLASS);
            delete e.target.dataset.wsTag;
        }
        if (highlightedElement === e.target) {
            highlightedElement = null;
        }
    }

    // Click handler
    function onClick(e) {
        e.preventDefault();
        e.stopPropagation();

        const target = e.target;
        const selector = generateSelector(target);
        const text = (target.innerText || target.textContent || '').trim().substring(0, 200);
        const tagName = target.tagName.toLowerCase();
        const hasHref = target.hasAttribute('href');
        const hasSrc = target.hasAttribute('src');
        const matchCount = countMatches(selector);

        // Flash green to confirm selection
        target.classList.add(SELECTED_CLASS);
        setTimeout(() => target.classList.remove(SELECTED_CLASS), 1500);

        // Check if this is a container element
        const container = isContainer(target);
        let childrenInfo = [];
        if (container) {
            childrenInfo = getChildrenInfo(target);
        }

        // Send message to parent
        window.parent.postMessage({
            type: 'ELEMENT_SELECTED',
            selector: selector,
            text: text,
            tagName: tagName,
            hasHref: hasHref,
            hasSrc: hasSrc,
            hrefValue: hasHref ? (target.getAttribute('href') || '') : '',
            srcValue: hasSrc ? (target.getAttribute('src') || '') : '',
            matchCount: matchCount,
            isContainer: container,
            childrenInfo: childrenInfo
        }, '*');

        console.log('[SnapCrawl] Selected:', selector, 'tag:', tagName, 'container:', container, 'children:', childrenInfo.length);
    }

    // Attach event listeners
    function attachListeners() {
        document.querySelectorAll('*').forEach(el => {
            el.addEventListener('mouseover', onMouseOver);
            el.addEventListener('mouseout', onMouseOut);
            el.addEventListener('click', onClick);
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', attachListeners);
    } else {
        attachListeners();
    }

    const observer = new MutationObserver((mutations) => {
        mutations.forEach((mutation) => {
            mutation.addedNodes.forEach((node) => {
                if (node.nodeType === Node.ELEMENT_NODE) {
                    node.addEventListener('mouseover', onMouseOver);
                    node.addEventListener('mouseout', onMouseOut);
                    node.addEventListener('click', onClick);

                    node.querySelectorAll('*').forEach(el => {
                        el.addEventListener('mouseover', onMouseOver);
                        el.addEventListener('mouseout', onMouseOut);
                        el.addEventListener('click', onClick);
                    });
                }
            });
        });
    });

    observer.observe(document.body, {
        childList: true,
        subtree: true
    });

    // ── Panel Hover Highlight ──
    const PANEL_HIGHLIGHT_CLASS = 'ws-panel-highlight';
    const panelStyle = document.createElement('style');
    panelStyle.textContent = `
        .${PANEL_HIGHLIGHT_CLASS} {
            outline: 3px solid #f59e0b !important;
            outline-offset: 2px !important;
            background-color: rgba(245, 158, 11, 0.12) !important;
            transition: outline-color 0.2s, background-color 0.2s !important;
        }
    `;
    document.head.appendChild(panelStyle);

    function clearPanelHighlights() {
        document.querySelectorAll('.' + PANEL_HIGHLIGHT_CLASS).forEach(function (el) {
            el.classList.remove(PANEL_HIGHLIGHT_CLASS);
        });
    }

    // ── DOM Tree Builder ──
    let nodeIdCounter = 0;
    
    function buildDomTree(element, maxDepth, currentDepth) {
        maxDepth = maxDepth || 8;
        currentDepth = currentDepth || 0;
        
        if (!element || element.nodeType !== Node.ELEMENT_NODE) return null;
        if (currentDepth >= maxDepth) return null;
        
        var tag = element.tagName.toLowerCase();
        if (tag === 'script' || tag === 'style' || tag === 'noscript' || tag === 'iframe') return null;
        
        var nodeId = 'node_' + (nodeIdCounter++);
        var node = {
            id: nodeId,
            tag: tag,
            selector: generateSelector(element),
            children: []
        };
        
        if (element.id) {
            node.id = element.id;
        }
        
        if (element.className && typeof element.className === 'string') {
            node.classes = element.className.split(/\s+/).filter(function(c) { 
                return c && !c.startsWith('ws-'); 
            });
        }
        
        var text = (element.innerText || element.textContent || '').trim();
        if (text && text.length > 0 && element.children.length === 0) {
            node.text = text.substring(0, 100);
        }
        
        // Build children
        var children = Array.from(element.children);
        children.forEach(function(child) {
            var childNode = buildDomTree(child, maxDepth, currentDepth + 1);
            if (childNode) {
                node.children.push(childNode);
            }
        });
        
        return node;
    }

    function sendDomTree() {
        nodeIdCounter = 0;
        var tree = buildDomTree(document.body, 10, 0);
        if (tree) {
            window.parent.postMessage({
                type: 'DOM_TREE_DATA',
                tree: tree
            }, '*');
        }
    }

    function selectElementBySelector(selector) {
        try {
            var el = document.querySelector(selector);
            if (el) {
                // Simulate a click on the element
                el.scrollIntoView({ behavior: 'smooth', block: 'center' });
                el.classList.add(SELECTED_CLASS);
                setTimeout(function() { el.classList.remove(SELECTED_CLASS); }, 1500);
                
                // Trigger the selection message
                var text = (el.innerText || el.textContent || '').trim().substring(0, 200);
                var tagName = el.tagName.toLowerCase();
                var hasHref = el.hasAttribute('href');
                var hasSrc = el.hasAttribute('src');
                var matchCount = countMatches(selector);
                var container = isContainer(el);
                var childrenInfo = container ? getChildrenInfo(el) : [];
                
                window.parent.postMessage({
                    type: 'ELEMENT_SELECTED',
                    selector: selector,
                    text: text,
                    tagName: tagName,
                    hasHref: hasHref,
                    hasSrc: hasSrc,
                    hrefValue: hasHref ? (el.getAttribute('href') || '') : '',
                    srcValue: hasSrc ? (el.getAttribute('src') || '') : '',
                    matchCount: matchCount,
                    isContainer: container,
                    childrenInfo: childrenInfo
                }, '*');
            }
        } catch (e) {}
    }

    function scrollToSelector(selector) {
        try {
            var el = document.querySelector(selector);
            if (el) {
                el.scrollIntoView({ behavior: 'smooth', block: 'center' });
                // Also add a temporary highlight effect
                el.style.transition = 'outline 0.3s';
                var originalOutline = el.style.outline;
                el.style.outline = '3px solid #f59e0b';
                setTimeout(function() {
                    el.style.outline = originalOutline;
                }, 1000);
            }
        } catch (e) {}
    }

    window.addEventListener('message', function (e) {
        if (!e.data || !e.data.type) return;

        if (e.data.type === 'HIGHLIGHT_SELECTOR') {
            clearPanelHighlights();
            try {
                var els = document.querySelectorAll(e.data.selector);
                els.forEach(function (el) {
                    el.classList.add(PANEL_HIGHLIGHT_CLASS);
                });
                // Scroll to first match
                if (els.length > 0) {
                    els[0].scrollIntoView({ behavior: 'smooth', block: 'center' });
                }
            } catch (err) { }
        }

        if (e.data.type === 'CLEAR_HIGHLIGHT') {
            clearPanelHighlights();
        }
        
        if (e.data.type === 'GET_DOM_TREE') {
            sendDomTree();
        }
        
        if (e.data.type === 'SELECT_ELEMENT_BY_SELECTOR') {
            selectElementBySelector(e.data.selector);
        }
        
        if (e.data.type === 'SCROLL_TO_SELECTOR') {
            scrollToSelector(e.data.selector);
        }
    });

    // Send initial tree after a short delay
    setTimeout(sendDomTree, 500);

    console.log('[SnapCrawl] Selector script loaded');
})();
