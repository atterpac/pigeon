# Mail UI Redesign — Component Extraction & Migration Plan

Goal: graduate the sandbox design (`src/sandbox/Sandbox.vue`) into the real app by
**extracting `App.vue` into components** and restyling onto a TokyoNight token layer.
Keep ALL existing logic (mail client, onboarding, keyboard handling, vim editor,
markdown preview). This is a structural + visual refactor, not a rewrite.

Decisions locked (from sandbox sessions):
- Layout: **triple pane** (sidebar | list | reading), reading pane persistent.
- Sidebar: **flat** style — account switcher + Compose + grouped Folders/Labels + dot-number unread counts.
- Compose: **centered modal overlay** as the DEFAULT.
- Theme: **TokyoNight** as the first shipped pack, Night the default. 4 variants now.
- Terminal/vim: **vim modeline** statusbar, relative line numbers in list, `/` search, `:` ex-command, which-key, `?` cheatsheet.
- Settings: **sidebar-tabs** modal.

Cross-cutting requirements (added):
- **R1 — Many themes, extensible.** Themes must be a *registry/data*, not hardcoded classes.
  Adding a theme = adding one entry. Plan for dozens (community/custom packs), grouped by pack.
- **R2 — Variants are user-selectable, not thrown away.** The sandbox explorations (compose
  modal vs docked vs side-panel vs full sheet; sidebar styles; nav layouts; settings-modal shapes)
  become **preference options surfaced in Settings**, defaulting to the locked choices above.
  So the design system is "sensible default + switchable view presets," not a single frozen UI.

Reference implementations for all visual treatments live in git history of
`src/sandbox/Sandbox.vue` (each session overwrote it). Current `Sandbox.vue` = settings modal.
Pull token values + markup from there.

---

## 1. Target file structure

```
src/
  App.vue                 # thin: phase routing (starting/onboarding/mail), owns MailClient + global state
  theme/
    tokens.css            # :root + .theme-{night,storm,moon,day} CSS variables (TokyoNight)
    themes.ts             # ThemeId type + theme list metadata for the switcher
  composables/
    useMailShell.ts       # extracted state + actions from App.vue (client, mailboxes, conversations, thread, draft…)
    useKeybindings.ts     # global keydown → actions (j/k/gg/G, /, :, dd, *, u, c, esc, leader)
    useSettings.ts        # reactive settings object + localStorage persistence (theme, density, vimMode, relativenumber…)
  components/
    shell/
      MailShell.vue       # triple-pane grid; hosts Sidebar + List + Reading + Modeline + modals
      Sidebar.vue         # account block, Compose btn, grouped Folders/Labels w/ dot counts
      MessageList.vue     # list pane: header + rows w/ relative-number gutter, star, cursorline
      ReadingPane.vue     # thread header + messages (keeps iframe html rendering) + reply panel
      Modeline.vue        # vim statusline (mode block, buffer, branch, ln:col, %)
      CommandLine.vue     # `/` search + `:` ex-command input strip above modeline
    overlays/
      ComposeModal.vue    # centered modal wrapping existing compose form (To/Subject/editor/toolbar/attachments)
      SettingsModal.vue   # sidebar-tabs settings (General/Accounts/Appearance/Keybindings/Notifications/Privacy)
      Cheatsheet.vue      # `?` keybinding overlay (DONE, phase 4)
      # WhichKey leader popup — dropped, not needed (cheatsheet covers discovery)
    editor/
      MarkdownEditor.vue  # the textarea + line gutter + terminal caret + INSERT/NORMAL + applyFormat + preview
  onboarding/
    OnboardingView.vue    # extract the appPhase==='onboarding' template/form from App.vue
```

Keep `mail/`, `onboarding/client.ts`, `mail/types.ts` as-is.

---

## 2. State ownership (where App.vue logic moves)

Extract from current `App.vue` `<script setup>` into `composables/useMailShell.ts`
(return refs + functions; App/MailShell consume it). Map of current symbols:

- Refs to move: `client, account, configuredAccounts, screen, activeMailbox, activeCategory,
  selectedIndex, selectedThread, mailboxes, labels, conversations, searchResults,
  threadMessages, query, replyMode, replyExpanded, draft, status`.
- Computeds: `filteredConversations, activeList, selectedConversation, unreadCount,
  todayConversations, earlierConversations, categoryCounts, mode, statusHints`.
- Actions: `initializeApp, bootMailbox, submitOnboarding, refreshShell, openMailbox,
  warmMailbox, selectCategory, openThread, prepareReply, archiveThread, snoozeThread,
  toggleStar, toggleRead, compose, sendDraft, discardDraft, runSearch, openSearch, moveSelection`.
- Editor-only (move to MarkdownEditor.vue): `editorMode, preview, currentLine, caretStyle,
  lineNumbers, renderedPreview, queueSave, applyFormat, attachMock, updateCaret,
  handleEditorKeydown, replaceBody, moveEditorLine, onEditorFocus/Blur, newDraft, charCanvas, saveTimer`.
- Helpers (small, keep in a `mail/format.ts`): `renderMarkdown, renderInlineMarkdown,
  renderEmailHtml, escapeHtml, initials, participantLine, formatDate, isToday, labelFor,
  parseAddresses, errorMessage, accountFromConfigured, materializeRecipients`.

`screen` type currently `'inbox' | 'thread' | 'compose' | 'search'`. New model:
- reading pane is **always** rendered when a thread is selected (not a screen).
- `compose` and `settings` become **overlay booleans** (`composeOpen`, `settingsOpen`), not screens.
- keep `search` as a mode of the command line (`/`), not a full screen.
  Suggest: `selection mode` enum `'list' | 'thread'` + `command: null | {kind:'search'|'ex', text}`.

---

## 3. Component contracts (props / emits)

- **Sidebar.vue** — props: `account, mailboxes, labels, activeMailbox`. emits:
  `open-mailbox(id)`, `open-label(label)`, `compose`. (folders=`mailboxes`, dot-count = `mailbox.unread`.)
- **MessageList.vue** — props: `conversations, selectedIndex, relativeNumbers:boolean, headerTitle, unreadCount`.
  emits: `select(index)`, `open(id)`, `toggle-star(conversation)`. Gutter number = relative to selectedIndex.
- **ReadingPane.vue** — props: `thread:Conversation|null, messages:ThreadMessage[], replyMode, replyExpanded, draft`.
  emits: `archive, snooze, toggle-star, toggle-read, prepare-reply(mode), back`. Hosts MarkdownEditor for the reply.
- **Modeline.vue** — props: `mode:string, mailboxPath, flags, branch, position:{ln,col}, pct`.
- **CommandLine.vue** — props: `kind:'search'|'ex', text`. emits: `submit, cancel, input`. Drives `query`/ex parsing.
- **ComposeModal.vue** — props: `draft, open`. emits: `send, discard, save, attach, close`. Wraps MarkdownEditor.
- **SettingsModal.vue** — props: `open, settings, layout` (its own variant). emits: `update(key,value), close`.
  Categories incl. **Appearance** (theme picker over `THEMES`) and **Layout/Views** (the R2 preset switches).
- **MarkdownEditor.vue** — `v-model:body`, props: `preview`. emits format/caret internally; expose `focus()`.

> Presentation components take their variant as a prop (`ComposeModal :variant="settings.compose"`,
> `Sidebar :style-variant="settings.sidebarStyle" :nav-layout="settings.navLayout"`,
> `MessageList :relative-numbers="settings.relativenumber"`). App/MailShell reads `useSettings()` and passes down.

---

## 4. Theme token layer (phase 1) — built for MANY themes (R1)

Token contract (the fixed surface every theme must define). Names from sandbox:
`--bg --surface --surface-2 --border --border-2 --text --text-dim --text-mut --head
--accent --accent-ink --accent-soft --accent-line --star --read-bg --read-text --grid`
plus modeline syntax accents: `--green --orange --red --purple --cyan`.

**Do NOT hardcode `.theme-x` classes.** Instead make themes data:

```ts
// theme/themes.ts
export type ThemeTokens = Record<string, string>   // the var contract above
export interface Theme { id: string; name: string; pack: string; dark: boolean; tokens: ThemeTokens }
export const THEMES: Theme[] = [
  { id:'tokyonight-night', name:'Night', pack:'TokyoNight', dark:true, tokens:{ '--bg':'#16161e', ... } },
  { id:'tokyonight-storm', ... }, { id:'tokyonight-moon', ... }, { id:'tokyonight-day', dark:false, ... },
  // future packs: Catppuccin, Gruvbox, Rosé Pine, Nord, Dracula, Solarized, custom… one entry each
]
```

Apply by writing tokens to a style element / `:root` inline at runtime from the active theme id
(no per-theme CSS class needed): `applyTheme(theme){ for (k,v of theme.tokens) root.style.setProperty(k,v) }`.
This means **adding a theme is a pure data add** — no new CSS, no template change. Settings → Appearance
renders the picker by iterating `THEMES` grouped by `pack`; supports an arbitrary count, search, and
later user-defined packs (validate a tokens object → same contract).

Derived/computed tokens (rgba softs like `--accent-soft`) can be generated from a base accent in
`applyTheme` so a minimal theme only needs ~8 core colors, lowering the bar to add one.

Migration mechanic: replace every hardcoded hex in `App.vue` `<style>` with the matching var.
`#8b7cf6` → `--accent`; `#0e0e13/#0b0b10/#0a0a0e` → `--bg/--surface`; `#e8e8ef` → `--text`;
`#5a5a66` → `--text-mut`; etc. Remove the `accent` JS const + `:style="{ '--accent': accent }"`.
Keep JetBrains Mono / Hanken Grotesk fonts.

TokyoNight token values live in the themes version of `Sandbox.vue` (git history) — copy them into `THEMES`.

---

## 5. Phase order (each a reviewable commit)

1. **Token layer**: add `tokens.css`, swap App.vue hex → vars, default Night. No structural change.
2. **Extract shell + restyle**: split App.vue mail phase into MailShell + Sidebar + MessageList + ReadingPane
   (+ extract OnboardingView). Reading pane persistent. Flat sidebar with account block + grouped nav + dot counts.
3. **Compose modal**: ComposeModal.vue wrapping the existing compose form/editor; `compose()` opens overlay
   instead of `screen='compose'`. Keep `⌘↵` send, esc close, draft autosave.
4. **Vim layer**: Modeline.vue replaces `.statusbar`; CommandLine.vue for `/` (reuses `runSearch`) and `:` ex
   (`:archive`,`:snooze`,`:label x`,`:w`,`:q`). Add relative-number gutter (gated by `settings.relativenumber`).
   Extend `handleGlobalKeydown` → useKeybindings (`gg/G`, counts, `dd`, leader/which-key, `?`).
5. **Settings**: SettingsModal.vue (sidebar tabs) + useSettings persistence; Appearance tab drives themes
   live (iterates `THEMES`); a **Layout/Views** tab exposes the view presets (R2).

## 4b. View presets — variants as settings (R2)

The sandbox variants are not dead ends; they become persisted preferences with the locked choice as default.
Model them as a settings object, consumed via `useSettings()`, each rendered by a `:class`/`v-if` switch
on the relevant component (the components already prove these variants render from one markup + a variant key):

```ts
// useSettings() shape (persisted to localStorage)
interface Settings {
  theme: string                                   // THEMES id, default 'tokyonight-night'
  compose: 'centered'|'docked'|'side'|'fullscreen'|'minimal'|'split'   // default 'centered'
  sidebarStyle: 'flat'|'cards'|'compact'|'outline'|'header'|'airy'     // default 'flat'
  navLayout: 'grouped'|'plain'|'icons'|'counts'|'rail'|'accounts'      // default 'grouped'
  settingsLayout: 'sidebar'|'tabs'|'scroll'|'cards'|'palette'|'fullscreen' // default 'sidebar'
  density: 'comfortable'|'compact'
  vimMode: boolean
  relativenumber: boolean
}
```

Implementation note: keep each variant's CSS under a stable class (`.compose-centered`, `.sidebar-flat`,
`.nav-grouped`, …) exactly as in the sandbox, and bind the class from settings. ComposeModal/Sidebar/
MessageList/SettingsModal each accept their variant as a prop so they stay presentation-only. Ship the
locked defaults first; the other variants are already built in sandbox history and can be ported incrementally —
the switch points just need to exist from the start so adding one later is a data/CSS add, not a refactor.

---

## 6. Risks / watch-outs

- **iframe email HTML** (`renderEmailHtml`, sandboxed iframe) must survive the ReadingPane extraction unchanged.
- **Editor caret math** (`updateCaret`, canvas measure, `caretStyle`) is fiddly — move as a unit into MarkdownEditor, don't rewrite.
- **Keyboard focus**: global keydown must still ignore INPUT/TEXTAREA except the editor (current `inEditor` guard). Centralize in useKeybindings.
- **Onboarding** path unchanged functionally; just visually re-tokened.
- **Search**: today it's a screen with its own input; becoming the `/` command line. Preserve `from:github is:unread` query syntax; `:` ex-commands are separate.
- **Categories tabs** (primary/promotions/…) currently in inbox list header — decide whether they stay in MessageList header or move; keep for now.
- Don't break the Sandbox entry (App.vue `screen==='sandbox'` branch + topbar button) until redesign lands; can drop it at the end.

---

## 7. Execution kickoff (for fresh context)

Start at Phase 1. Read `src/App.vue` (full), pull TokyoNight token values from the themes
version in `git log -p src/sandbox/Sandbox.vue`, create `theme/tokens.css`, and do the hex→var
swap. Verify build (`cd frontend && npm run build` or `vite`), confirm visual parity in Night, then commit and proceed to Phase 2.
