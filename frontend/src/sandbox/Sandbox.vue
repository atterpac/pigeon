<script setup lang="ts">
import { ref } from 'vue'

// Sandbox — TokyoNight Night shell as backdrop, exploring SETTINGS MODAL
// layouts. Toggle swaps the modal pattern.
type OptionId = 'sidebar' | 'tabs' | 'scroll' | 'cards' | 'palette' | 'fullscreen'
const options: Array<{ id: OptionId; label: string }> = [
  { id: 'sidebar', label: 'Sidebar tabs' },
  { id: 'tabs', label: 'Top tabs' },
  { id: 'scroll', label: 'Single scroll' },
  { id: 'cards', label: 'Category grid' },
  { id: 'palette', label: ': command' },
  { id: 'fullscreen', label: 'Full page' },
]
const active = ref<OptionId>('sidebar')
const dims = (id: OptionId) => id !== 'fullscreen'

const cats = [
  { icon: '⚙', name: 'General' },
  { icon: '👤', name: 'Accounts' },
  { icon: '🎨', name: 'Appearance' },
  { icon: '⌨', name: 'Keybindings' },
  { icon: '🔔', name: 'Notifications' },
  { icon: '🔒', name: 'Privacy' },
]
// Appearance panel content, reused across layouts
const appearance = {
  toggles: [
    { name: 'Compact density', desc: 'Tighter list rows and padding', on: true },
    { name: 'Show relative line numbers', desc: 'Vim-style gutter in the message list', on: true },
    { name: 'Animated transitions', desc: 'Fade and slide between views', on: false },
    { name: 'Render remote images', desc: 'Load images in HTML emails automatically', on: false },
  ],
  selects: [
    { name: 'Theme', value: 'TokyoNight Night ▾' },
    { name: 'Font', value: 'JetBrains Mono ▾' },
    { name: 'Accent', value: '#7aa2f7 ▾' },
  ],
}
// flat searchable settings list (for the : command variant)
const allSettings = [
  { k: 'appearance.theme', v: 'tokyonight-night' },
  { k: 'appearance.density', v: 'compact' },
  { k: 'list.relativenumber', v: 'true' },
  { k: 'editor.vimMode', v: 'true' },
  { k: 'notify.sound', v: 'false' },
  { k: 'privacy.remoteImages', v: 'false' },
]
</script>

<template>
  <div class="sandbox theme-night">
    <div class="stage">
      <!-- backdrop shell (simplified) -->
      <div class="layout triple">
        <aside class="nav">
          <button class="account"><span class="avatar sm">M</span>
            <span class="acctcol"><b>Michael</b><small>michael@getgalaxy.io</small></span><span class="chev">⌄</span></button>
          <button class="composebtn">✎ Compose</button>
          <div class="group"><p class="grouphead">Folders</p>
            <a v-for="(c, i) in ['Inbox','Starred','Drafts','Sent','Archive']" :key="c" class="navitem" :class="{ active: i === 0 }">
              <span class="navlabel">{{ c }}</span></a></div>
        </aside>
        <section class="list"><header class="listhead"><b>Inbox</b><span>12 unread</span></header><div class="filler" /></section>
        <section class="read"><div class="filler" /></section>
      </div>

      <!-- ===== SETTINGS MODAL ===== -->
      <div class="overlay" :class="{ dim: dims(active) }">
        <div class="modal" :class="active">

          <!-- 1. SIDEBAR TABS -->
          <template v-if="active === 'sidebar'">
            <aside class="set-nav">
              <p class="set-title">Settings</p>
              <a v-for="(c, i) in cats" :key="c.name" class="set-catitem" :class="{ active: i === 2 }">
                <span class="set-catico">{{ c.icon }}</span>{{ c.name }}</a>
            </aside>
            <div class="set-body">
              <header class="set-head"><h2>Appearance</h2><span class="set-close">esc</span></header>
              <div class="set-scroll">
                <p class="set-section">Display</p>
                <label v-for="t in appearance.toggles" :key="t.name" class="set-row">
                  <div><b>{{ t.name }}</b><small>{{ t.desc }}</small></div>
                  <span class="toggle" :class="{ on: t.on }"><i /></span>
                </label>
                <p class="set-section">Theme</p>
                <label v-for="s in appearance.selects" :key="s.name" class="set-row">
                  <div><b>{{ s.name }}</b></div><span class="select">{{ s.value }}</span>
                </label>
              </div>
            </div>
          </template>

          <!-- 2. TOP TABS -->
          <template v-else-if="active === 'tabs'">
            <header class="set-head withtabs"><h2>Settings</h2><span class="set-close">esc</span></header>
            <nav class="set-tabs">
              <a v-for="(c, i) in cats" :key="c.name" :class="{ active: i === 2 }">{{ c.name }}</a>
            </nav>
            <div class="set-scroll">
              <p class="set-section">Display</p>
              <label v-for="t in appearance.toggles" :key="t.name" class="set-row">
                <div><b>{{ t.name }}</b><small>{{ t.desc }}</small></div>
                <span class="toggle" :class="{ on: t.on }"><i /></span>
              </label>
              <p class="set-section">Theme</p>
              <label v-for="s in appearance.selects" :key="s.name" class="set-row">
                <div><b>{{ s.name }}</b></div><span class="select">{{ s.value }}</span>
              </label>
            </div>
          </template>

          <!-- 3. SINGLE SCROLL -->
          <template v-else-if="active === 'scroll'">
            <header class="set-head"><h2>Settings</h2><span class="set-close">esc</span></header>
            <div class="set-scroll">
              <template v-for="c in cats" :key="c.name">
                <p class="set-section big"><span class="set-catico">{{ c.icon }}</span>{{ c.name }}</p>
                <label v-for="t in appearance.toggles.slice(0, 2)" :key="c.name + t.name" class="set-row">
                  <div><b>{{ t.name }}</b><small>{{ t.desc }}</small></div>
                  <span class="toggle" :class="{ on: t.on }"><i /></span>
                </label>
              </template>
            </div>
          </template>

          <!-- 4. CATEGORY GRID -->
          <template v-else-if="active === 'cards'">
            <header class="set-head"><h2>Settings</h2><span class="set-close">esc</span></header>
            <div class="set-grid">
              <button v-for="c in cats" :key="c.name" class="set-tile">
                <span class="set-tileico">{{ c.icon }}</span>
                <b>{{ c.name }}</b>
                <small>Configure {{ c.name.toLowerCase() }} options</small>
              </button>
            </div>
          </template>

          <!-- 5. : COMMAND / SEARCH -->
          <template v-else-if="active === 'palette'">
            <div class="set-search"><span class="set-sigil">:set</span>
              <span class="set-query">relativenumber</span><span class="set-caret" /></div>
            <div class="set-scroll tight">
              <div v-for="s in allSettings" :key="s.k" class="set-cmd" :class="{ active: s.k === 'list.relativenumber' }">
                <span class="set-key">{{ s.k }}</span>
                <span class="set-val" :class="{ bool: s.v === 'true' || s.v === 'false' }">{{ s.v }}</span>
              </div>
            </div>
            <footer class="set-cmdfoot"><kbd>↵</kbd> toggle · <kbd>tab</kbd> complete · <kbd>esc</kbd> close</footer>
          </template>

          <!-- 6. FULL PAGE -->
          <template v-else>
            <aside class="set-nav wide">
              <p class="set-title">Settings</p>
              <a v-for="(c, i) in cats" :key="c.name" class="set-catitem" :class="{ active: i === 2 }">
                <span class="set-catico">{{ c.icon }}</span>{{ c.name }}</a>
              <div class="set-navfoot"><span class="set-close">⌘, to close</span></div>
            </aside>
            <div class="set-body">
              <header class="set-head"><h2>Appearance</h2></header>
              <div class="set-scroll wide">
                <p class="set-section">Display</p>
                <label v-for="t in appearance.toggles" :key="t.name" class="set-row">
                  <div><b>{{ t.name }}</b><small>{{ t.desc }}</small></div>
                  <span class="toggle" :class="{ on: t.on }"><i /></span>
                </label>
                <p class="set-section">Theme</p>
                <div class="set-themegrid">
                  <div v-for="th in ['Night','Storm','Moon','Day']" :key="th" class="set-swatch" :class="{ active: th === 'Night' }">{{ th }}</div>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <nav class="toggle-bar" aria-label="Settings modal toggle">
      <button v-for="option in options" :key="option.id" type="button"
        :class="{ active: active === option.id }" @click="active = option.id">
        {{ option.label }}
      </button>
    </nav>
  </div>
</template>

<style scoped>
.sandbox{
  --bg:#16161e;--surface:#1a1b26;--surface-2:#20212e;--border:#202230;--border-2:#2a2e42;
  --text:#c0caf5;--text-dim:#a9b1d6;--text-mut:#565f89;--head:#d5d9f0;
  --accent:#7aa2f7;--accent-ink:#16161e;--accent-soft:rgba(122,162,247,.16);--accent-line:rgba(122,162,247,.45);
  --green:#9ece6a;--orange:#ff9e64;--star:#e0af68;--read-bg:#1a1b26;--read-text:#a9b1d6;--grid:rgba(122,162,247,.06);
  position:relative;min-height:0;height:100%;overflow:hidden;background-color:#0e0e13;
  background-image:linear-gradient(var(--grid) 1px,transparent 1px),linear-gradient(90deg,var(--grid) 1px,transparent 1px);
  background-size:24px 24px;
}
.stage{position:relative;height:100%;padding:20px 20px 84px}

/* backdrop shell */
.triple{height:100%;display:grid;grid-template-columns:236px 320px 1fr;overflow:hidden;background:var(--bg);border:1px solid var(--border-2);border-radius:14px;color:var(--text);font:14px "Hanken Grotesk",Inter,sans-serif}
.nav{padding:16px 12px;border-right:1px solid var(--border)}
.list{border-right:1px solid var(--border);display:grid;grid-template-rows:auto 1fr;background:var(--surface)}
.read{background:var(--read-bg)}
.filler{background:repeating-linear-gradient(transparent,transparent 40px,var(--border) 40px,var(--border) 41px)}
.account{display:grid;grid-template-columns:34px 1fr 12px;gap:10px;align-items:center;width:100%;padding:9px;border-radius:11px;background:var(--surface-2);border:1px solid var(--border-2);color:var(--text);text-align:left}
.acctcol{display:grid;gap:1px;min-width:0}.acctcol b{font-size:13px}.acctcol small{color:var(--text-mut);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.chev{color:var(--text-mut)}
.composebtn{width:100%;padding:10px;margin:12px 0 6px;border-radius:11px;border:0;background:var(--accent-soft);color:var(--accent);font:600 13px "Hanken Grotesk",Inter,sans-serif}
.grouphead{margin:16px 12px 6px;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace;letter-spacing:.08em;text-transform:uppercase}
.navitem{display:block;padding:9px 12px;border-radius:9px;margin:2px 0;color:var(--text-dim);font-size:13.5px}
.navitem.active{background:var(--accent-soft);color:var(--accent);font-weight:600}
.avatar.sm{width:34px;height:34px;display:grid;place-items:center;border-radius:10px;background:var(--accent-soft);color:var(--accent);font:600 13px "Hanken Grotesk",Inter,sans-serif}
.listhead{display:flex;justify-content:space-between;padding:16px 18px;border-bottom:1px solid var(--border);color:var(--text-mut);font:12px "JetBrains Mono",ui-monospace,monospace}
.listhead b{color:var(--text);font:600 15px "Hanken Grotesk",Inter,sans-serif}
kbd{padding:1px 6px;border-radius:5px;background:var(--surface-2);border:1px solid var(--border-2);color:var(--text-dim);font:11px "JetBrains Mono",ui-monospace,monospace}

/* overlay + modal frame */
.overlay{position:absolute;inset:20px 20px 84px;z-index:10;display:flex;align-items:center;justify-content:center;border-radius:14px;pointer-events:none}
.overlay.dim{background:rgba(6,6,9,.64);backdrop-filter:blur(2px)}
.modal{pointer-events:auto;display:flex;flex-direction:column;background:var(--surface);border:1px solid var(--border-2);color:var(--text);box-shadow:0 30px 80px rgba(0,0,0,.6);overflow:hidden;border-radius:14px}
.modal.sidebar,.modal.tabs{width:min(720px,92%);height:min(520px,84%)}
.modal.sidebar{flex-direction:row}
.modal.scroll{width:min(560px,90%);height:min(560px,84%)}
.modal.cards{width:min(640px,90%)}
.modal.palette{width:min(600px,90%);max-height:80%}
.modal.fullscreen{flex-direction:row;width:100%;height:100%;border-radius:14px}

/* settings sub-nav */
.set-nav{width:200px;flex:none;padding:16px 12px;border-right:1px solid var(--border);background:var(--bg);display:flex;flex-direction:column}
.set-nav.wide{width:240px}
.set-title{margin:6px 10px 16px;color:var(--head);font:600 15px "Hanken Grotesk",Inter,sans-serif}
.set-catitem{display:flex;align-items:center;gap:11px;padding:9px 11px;border-radius:9px;margin:2px 0;color:var(--text-dim);font-size:13.5px;cursor:pointer}
.set-catitem:hover{background:var(--surface-2)}
.set-catitem.active{background:var(--accent-soft);color:var(--accent);font-weight:600}
.set-catico{font-size:13px}
.set-navfoot{margin-top:auto;padding:10px;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace}

/* settings body */
.set-body{flex:1;display:flex;flex-direction:column;min-width:0}
.set-head{display:flex;align-items:center;justify-content:space-between;padding:18px 22px;border-bottom:1px solid var(--border)}
.set-head h2{margin:0;color:var(--head);font:600 17px "Hanken Grotesk",Inter,sans-serif}
.set-close{color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace;border:1px solid var(--border-2);border-radius:6px;padding:3px 8px}
.set-scroll{flex:1;overflow:auto;padding:14px 22px}
.set-scroll.wide{padding:18px 30px}
.set-scroll.tight{padding:6px}
.set-section{margin:14px 0 8px;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace;letter-spacing:.08em;text-transform:uppercase}
.set-section:first-child{margin-top:0}
.set-section.big{display:flex;align-items:center;gap:9px;margin:22px 0 8px;color:var(--text-dim);font:600 13px "JetBrains Mono",ui-monospace,monospace;text-transform:none;letter-spacing:0;padding-top:14px;border-top:1px solid var(--border)}
.set-row{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:12px 0;border-bottom:1px solid var(--border);cursor:pointer}
.set-row b{display:block;color:var(--text);font-size:13.5px;font-weight:500}
.set-row small{display:block;margin-top:3px;color:var(--text-mut);font-size:12px}
/* toggle */
.toggle{flex:none;width:38px;height:22px;border-radius:999px;background:var(--surface-2);border:1px solid var(--border-2);position:relative;transition:background .15s}
.toggle i{position:absolute;top:2px;left:2px;width:16px;height:16px;border-radius:50%;background:var(--text-mut);transition:.15s}
.toggle.on{background:var(--accent-soft);border-color:var(--accent-line)}
.toggle.on i{left:18px;background:var(--accent)}
.select{flex:none;padding:6px 12px;border-radius:8px;background:var(--surface-2);border:1px solid var(--border-2);color:var(--text-dim);font:12px "JetBrains Mono",ui-monospace,monospace}

/* tabs variant */
.set-head.withtabs{border-bottom:0;padding-bottom:0}
.set-tabs{display:flex;gap:4px;padding:8px 22px 0;border-bottom:1px solid var(--border);overflow:auto}
.set-tabs a{padding:9px 13px;color:var(--text-dim);font-size:13px;border-bottom:2px solid transparent;white-space:nowrap;cursor:pointer}
.set-tabs a.active{color:var(--accent);border-bottom-color:var(--accent)}

/* cards variant */
.set-grid{display:grid;grid-template-columns:repeat(3,1fr);gap:12px;padding:22px}
.set-tile{display:flex;flex-direction:column;gap:6px;padding:18px;border-radius:13px;background:var(--bg);border:1px solid var(--border-2);text-align:left;cursor:pointer}
.set-tile:hover{border-color:var(--accent-line);background:var(--surface-2)}
.set-tileico{font-size:22px}
.set-tile b{color:var(--text);font-size:14px}
.set-tile small{color:var(--text-mut);font-size:11.5px;line-height:1.4}

/* : command variant */
.set-search{display:flex;align-items:center;gap:10px;padding:16px 18px;border-bottom:1px solid var(--border-2);background:var(--bg);font:14px "JetBrains Mono",ui-monospace,monospace}
.set-sigil{color:var(--green);font-weight:700}
.set-query{color:var(--text)}
.set-caret{width:8px;height:16px;background:var(--text);animation:blink 1s steps(2) infinite}
@keyframes blink{50%{opacity:0}}
.set-cmd{display:flex;justify-content:space-between;align-items:center;gap:16px;padding:9px 12px;border-radius:7px;font:12.5px "JetBrains Mono",ui-monospace,monospace}
.set-cmd.active{background:var(--accent-soft)}
.set-key{color:var(--text-dim)}
.set-cmd.active .set-key{color:var(--accent)}
.set-val{color:var(--orange)}
.set-val.bool{color:var(--green)}
.set-cmdfoot{padding:10px 18px;border-top:1px solid var(--border);color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace}

/* fullscreen theme swatches */
.set-themegrid{display:grid;grid-template-columns:repeat(4,1fr);gap:10px}
.set-swatch{height:64px;display:grid;place-items:end center;padding-bottom:8px;border-radius:11px;background:var(--surface-2);border:1px solid var(--border-2);color:var(--text-dim);font:12px "JetBrains Mono",ui-monospace,monospace;cursor:pointer}
.set-swatch.active{border-color:var(--accent);color:var(--accent);box-shadow:inset 0 0 0 1px var(--accent-line)}

/* ---- toggle bar ---- */
.toggle-bar{position:fixed;left:50%;bottom:24px;transform:translateX(-50%);z-index:20;display:flex;gap:4px;padding:5px;border-radius:12px;background:rgba(11,11,16,.92);border:1px solid #23232e;backdrop-filter:blur(8px);box-shadow:0 8px 30px rgba(0,0,0,.5)}
.toggle-bar button{color:#8a8a96;background:transparent;border:0;border-radius:8px;padding:8px 14px;font:12px "JetBrains Mono",ui-monospace,monospace;white-space:nowrap}
.toggle-bar button:hover{color:#c9c9d4}
.toggle-bar button.active{color:#fff;background:rgba(255,255,255,.12)}
</style>
