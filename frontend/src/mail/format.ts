// Pure presentation helpers shared across the mail shell components.
// No reactive state — safe to import anywhere.
import type { Conversation, Label } from './types'

export function escapeHtml(value: string) {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export function initials(address: { name: string; addr: string }) {
  return (address.name || address.addr).split(/\s+/).slice(0, 2).map((part) => part[0]).join('').toUpperCase()
}

// Deterministic per-sender identity tint. Hashes the address to one of the
// theme's named hue tokens so avatars stay colorful and scannable while still
// inheriting whatever theme is active. Returns inline styles for a tinted chip.
const AVATAR_HUES = ['--accent', '--green', '--orange', '--purple', '--cyan', '--red', '--star']
export function avatarStyle(address: { name: string; addr: string }) {
  const key = (address.addr || address.name).toLowerCase()
  let hash = 0
  for (let i = 0; i < key.length; i++) hash = (hash * 31 + key.charCodeAt(i)) >>> 0
  const hue = AVATAR_HUES[hash % AVATAR_HUES.length]
  return {
    color: `var(${hue})`,
    background: `color-mix(in oklab, var(${hue}) 18%, transparent)`,
    borderColor: `color-mix(in oklab, var(${hue}) 38%, transparent)`,
  }
}

export function participantLine(conversation: Conversation | null) {
  return conversation?.participants.map((p) => p.name || p.addr).join(', ') ?? ''
}

export function isToday(value: string) {
  return new Date(value).toDateString() === new Date().toDateString()
}

export function formatDate(value: string) {
  const date = new Date(value)
  return isToday(value)
    ? date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
    : date.toLocaleDateString([], { weekday: 'short' })
}

export function labelFor(conversation: Conversation | null, labels: Label[]) {
  return labels.find((label) => conversation?.labelIds.includes(label.id))
}

export function parseAddresses(input: string) {
  return input.split(',').map((value) => value.trim()).filter(Boolean).map((value) => {
    const match = value.match(/^(.*)<(.+)>$/)
    return match ? { name: match[1]?.trim() ?? '', addr: match[2]?.trim() ?? '' } : { name: '', addr: value }
  })
}

export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'Unknown error')
}

// Remove the email's own scripts/handlers before display: marketing JS is a
// source of variable render cost (and an XSS surface). Our injected link
// handler below still runs under sandbox="allow-scripts".
function stripActiveContent(html: string) {
  return html
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
    .replace(/\son\w+\s*=\s*"[^"]*"/gi, '')
    .replace(/\son\w+\s*=\s*'[^']*'/gi, '')
    .replace(/javascript:/gi, '')
}

export function renderEmailHtml(html: string) {
  return `<!doctype html><html><head><base target="_blank"><meta name="referrer" content="no-referrer"><style>html,body{margin:0;padding:0;background:#fff;color:#111}body{overflow-wrap:anywhere;overflow:hidden}img{max-width:100%;height:auto}</style></head><body>${stripActiveContent(html)}<script>
document.addEventListener('click', function(event) {
  var link = event.target && event.target.closest ? event.target.closest('a[href]') : null;
  if (!link) return;
  event.preventDefault();
  event.stopPropagation();
  window.parent.postMessage({ type: 'email-link-open', href: link.href }, '*');
}, true);
// Report content height so the parent can auto-size the iframe (no inner scroll).
function reportHeight() {
  var h = Math.max(
    document.body.scrollHeight, document.documentElement.scrollHeight,
    document.body.offsetHeight, document.documentElement.offsetHeight
  );
  window.parent.postMessage({ type: 'email-frame-height', height: h }, '*');
}
window.addEventListener('load', reportHeight);
window.addEventListener('resize', reportHeight);
if (window.ResizeObserver) { new ResizeObserver(reportHeight).observe(document.body); }
Array.prototype.forEach.call(document.images, function(img) {
  if (!img.complete) img.addEventListener('load', reportHeight);
});
reportHeight();
// In-thread find: the parent can't read this sandboxed document, so it asks us
// to highlight matches and reports back counts + match positions.
(function() {
  var marks = [];
  function clearMarks() {
    marks.forEach(function(m) {
      if (!m.parentNode) return;
      m.parentNode.replaceChild(document.createTextNode(m.textContent), m);
    });
    marks = [];
    if (document.body) document.body.normalize();
  }
  function ensureStyle() {
    if (document.getElementById('ef-find-style')) return;
    var st = document.createElement('style');
    st.id = 'ef-find-style';
    st.textContent = 'mark.ef-find{background:#fde68a;color:#111;border-radius:2px}mark.ef-find.ef-active{background:#f59e0b;box-shadow:0 0 0 2px #f59e0b}';
    (document.head || document.documentElement).appendChild(st);
  }
  function highlight(query) {
    clearMarks();
    var q = query.toLowerCase();
    if (!q) return;
    var walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, {
      acceptNode: function(node) {
        if (!node.nodeValue || node.nodeValue.toLowerCase().indexOf(q) === -1) return NodeFilter.FILTER_SKIP;
        var tag = node.parentNode && node.parentNode.nodeName;
        if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'MARK') return NodeFilter.FILTER_REJECT;
        return NodeFilter.FILTER_ACCEPT;
      }
    });
    var targets = [], n;
    while ((n = walker.nextNode())) targets.push(n);
    targets.forEach(function(node) {
      var text = node.nodeValue, lower = text.toLowerCase();
      var frag = document.createDocumentFragment(), last = 0, pos;
      while ((pos = lower.indexOf(q, last)) !== -1) {
        if (pos > last) frag.appendChild(document.createTextNode(text.slice(last, pos)));
        var mark = document.createElement('mark');
        mark.className = 'ef-find';
        mark.textContent = text.slice(pos, pos + q.length);
        frag.appendChild(mark);
        marks.push(mark);
        last = pos + q.length;
      }
      if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
      if (node.parentNode) node.parentNode.replaceChild(frag, node);
    });
  }
  window.addEventListener('message', function(ev) {
    var d = ev.data || {};
    if (d.type === 'email-find') {
      ensureStyle();
      highlight(String(d.query || ''));
      var tops = marks.map(function(m) { return m.getBoundingClientRect().top + (window.scrollY || 0); });
      window.parent.postMessage({ type: 'email-find-result', count: marks.length, tops: tops }, '*');
    } else if (d.type === 'email-find-activate') {
      marks.forEach(function(m) { m.classList.remove('ef-active'); });
      if (d.index != null && d.index >= 0 && marks[d.index]) marks[d.index].classList.add('ef-active');
    } else if (d.type === 'email-find-clear') {
      clearMarks();
    }
  });
})();
</script></body></html>`
}

export function renderInlineMarkdown(line: string) {
  return escapeHtml(line)
    .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/_([^_\n]+)_/g, '<em>$1</em>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
}

export function renderMarkdown(markdown: string) {
  if (!markdown.trim()) return '<div class="preview-empty">Nothing to preview yet.</div>'
  let inFence = false
  return markdown.split('\n').map((line) => {
    if (line.trim().startsWith('```')) { inFence = !inFence; return '<div class="preview-line"><code>```</code></div>' }
    const rendered = inFence ? `<code>${escapeHtml(line) || '&nbsp;'}</code>` : renderInlineMarkdown(line)
    return `<div class="preview-line">${rendered || '&nbsp;'}</div>`
  }).join('')
}
