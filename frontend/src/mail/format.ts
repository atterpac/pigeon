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

// Compact wake-time for snoozed rows: "in 40m", "in 3h", "tomorrow 9 AM",
// "Mon 9 AM", or "Jun 30" for anything further out.
export function formatWakeTime(value: string) {
  const date = new Date(value)
  const now = new Date()
  const diffMs = date.getTime() - now.getTime()
  if (diffMs <= 0) return 'now'
  const mins = Math.round(diffMs / 60000)
  if (mins < 60) return `in ${mins}m`
  const hours = Math.round(mins / 60)
  if (hours < 12) return `in ${hours}h`
  const time = date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
  if (date.toDateString() === now.toDateString()) return time
  if (date.toDateString() === tomorrow.toDateString()) return `tomorrow ${time}`
  const days = Math.round(diffMs / 86_400_000)
  if (days < 7) return `${date.toLocaleDateString([], { weekday: 'short' })} ${time}`
  return date.toLocaleDateString([], { month: 'short', day: 'numeric' })
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

// Human-readable byte size for attachment chips/rows (e.g. "12 KB", "3.4 MB").
export function formatBytes(bytes: number) {
  if (!bytes || bytes < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const exponent = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)))
  const value = bytes / 1024 ** exponent
  return `${exponent === 0 ? value : value.toFixed(value >= 10 ? 0 : 1)} ${units[exponent]}`
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

function isRemoteUrl(url: string) {
  return /^\s*(?:https?:)?\/\//i.test(url)
}

// Neutralize remote images *in the HTML string* before it ever reaches the
// iframe, so tracking pixels never fire on open. Original URLs are stashed in
// data-ef-* attributes so the iframe can restore them when the user clicks
// "Load images". cid:/data: (inline) images are left untouched.
function neutralizeRemoteImages(html: string): { html: string; count: number } {
  let count = 0
  // <img> src / srcset
  html = html.replace(/<img\b[^>]*>/gi, (tag) => {
    let changed = false
    const out = tag.replace(/\b(src|srcset)\s*=\s*(["'])(.*?)\2/gi, (whole, attr: string, quote: string, val: string) => {
      if (!isRemoteUrl(val)) return whole
      changed = true
      return `data-ef-${attr.toLowerCase()}=${quote}${val}${quote}`
    })
    if (changed) count++
    return out
  })
  // Inline-style background images: keep other declarations, swap the remote
  // url() for none, and stash the original so it can be restored.
  html = html.replace(/\bstyle\s*=\s*(["'])(.*?)\1/gi, (whole, quote: string, val: string) => {
    if (!/url\(\s*['"]?(?:https?:)?\/\//i.test(val)) return whole
    count++
    const stripped = val.replace(/url\(\s*['"]?(?:https?:)?\/\/[^)]*\)/gi, 'none')
    return `data-ef-style=${quote}${val}${quote} style=${quote}${stripped}${quote}`
  })
  return { html, count }
}

// Inline (cid:) image source. `content` is base64 of the image bytes.
export type InlineImage = { contentType: string; content: string }

// Rewrite cid: references in src/srcset/style url() to data: URLs using the
// message's inline parts, so embedded images render without any network fetch.
// cid is local, so this is allowed even when remote images are blocked.
export function inlineCidImages(html: string, images?: Record<string, InlineImage>): string {
  if (!images || !Object.keys(images).length) return html
  const resolve = (cid: string): string | null => {
    const key = decodeURIComponent(cid.trim()).replace(/^<|>$/g, '')
    const img = images[key]
    return img && img.content ? `data:${img.contentType || 'application/octet-stream'};base64,${img.content}` : null
  }
  return html
    .replace(/\b(src|srcset)\s*=\s*(["'])\s*cid:([^"']+)\2/gi, (whole, attr: string, quote: string, cid: string) => {
      const url = resolve(cid)
      return url ? `${attr}=${quote}${url}${quote}` : whole
    })
    .replace(/url\(\s*['"]?cid:([^)'"]+)['"]?\s*\)/gi, (whole, cid: string) => {
      const url = resolve(cid)
      return url ? `url("${url}")` : whole
    })
}

export function renderEmailHtml(html: string, blockImages = false, inlineImages?: Record<string, InlineImage>) {
  const safe = stripActiveContent(html)
  const { html: neutral, count: blocked } = blockImages ? neutralizeRemoteImages(safe) : { html: safe, count: 0 }
  const bodyHtml = inlineCidImages(neutral, inlineImages)
  return `<!doctype html><html><head><base target="_blank"><meta name="referrer" content="no-referrer"><style>html,body{margin:0;padding:0;background:#fff;color:#111}body{overflow-wrap:anywhere;overflow:hidden}img{max-width:100%;height:auto}</style></head><body>${bodyHtml}<script>
var EF_BLOCKED = ${blocked};
// Restore the real image sources stashed by neutralizeRemoteImages.
function efLoadImages() {
  Array.prototype.forEach.call(document.querySelectorAll('img[data-ef-src]'), function(el) { el.setAttribute('src', el.getAttribute('data-ef-src')); el.removeAttribute('data-ef-src'); });
  Array.prototype.forEach.call(document.querySelectorAll('img[data-ef-srcset]'), function(el) { el.setAttribute('srcset', el.getAttribute('data-ef-srcset')); el.removeAttribute('data-ef-srcset'); });
  Array.prototype.forEach.call(document.querySelectorAll('[data-ef-style]'), function(el) { el.setAttribute('style', el.getAttribute('data-ef-style')); el.removeAttribute('data-ef-style'); });
}
window.addEventListener('message', function(ev) {
  if ((ev.data || {}).type === 'email-load-images') { efLoadImages(); setTimeout(reportHeight, 0); }
});

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
if (EF_BLOCKED > 0) window.parent.postMessage({ type: 'email-images-blocked', count: EF_BLOCKED }, '*');
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
    // Images first so ![alt](src) isn't mistaken for a link. Allow cid:/data:/http(s).
    .replace(/!\[([^\]]*)\]\((cid:[^)\s]+|data:[^)\s]+|https?:\/\/[^)\s]+)\)/g, '<img src="$2" alt="$1" />')
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
