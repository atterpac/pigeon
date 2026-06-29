<script setup lang="ts">
import { computed } from 'vue'
import {
  PhArchive,
  PhCheckCircle,
  PhClock,
  PhDotsThree,
  PhEnvelopeSimple,
  PhEnvelopeSimpleOpen,
  PhStar,
  PhTrash,
  PhX,
} from '@phosphor-icons/vue'
import { useMailShell } from '../../composables/useMailShell'

const s = useMailShell()

const selectedRows = computed(() => s.activeList.value.filter((conversation) => s.selectedIds.value.has(conversation.id)))
const hasUnread = computed(() => selectedRows.value.some((conversation) => conversation.unread))
const countLabel = computed(() => `${s.selectedCount.value} email${s.selectedCount.value === 1 ? '' : 's'} selected`)

function run(action: () => void | Promise<void>) {
  void action()
}
</script>

<template>
  <div v-if="s.visualMode.value && s.selectedCount.value > 0" class="selection-bar" role="toolbar" :aria-label="countLabel">
    <span class="selection-meta">
      <span class="kbadge">visual</span>
      <span class="selection-count">
        <PhCheckCircle :size="15" weight="fill" />
        {{ countLabel }}
      </span>
    </span>
    <span class="selection-actions">
      <button type="button" title="Archive selected" aria-label="Archive selected" @click="run(s.archiveSelection)">
        <kbd>e</kbd>
        <PhArchive :size="16" />
        <span>Archive</span>
      </button>
      <button type="button" title="Snooze selected" aria-label="Snooze selected" @click="run(() => s.snoozeSelection())">
        <kbd>s</kbd>
        <PhClock :size="16" />
        <span>Snooze</span>
      </button>
      <button type="button" :title="hasUnread ? 'Mark selected read' : 'Mark selected unread'" :aria-label="hasUnread ? 'Mark selected read' : 'Mark selected unread'" @click="run(s.toggleSelectionRead)">
        <kbd>u</kbd>
        <component :is="hasUnread ? PhEnvelopeSimpleOpen : PhEnvelopeSimple" :size="16" />
        <span>{{ hasUnread ? 'Read' : 'Unread' }}</span>
      </button>
      <button type="button" title="Star or unstar selected" aria-label="Star or unstar selected" @click="run(s.starSelection)">
        <kbd>*</kbd>
        <PhStar :size="16" />
        <span>Star</span>
      </button>
      <button type="button" class="danger" title="Delete selected" aria-label="Delete selected" @click="run(s.deleteSelection)">
        <kbd>#</kbd>
        <PhTrash :size="16" />
        <span>Delete</span>
      </button>
      <button type="button" title="More actions" aria-label="More actions" @click="s.openCommandMenu()">
        <kbd>↵</kbd>
        <PhDotsThree :size="18" weight="bold" />
        <span>More</span>
      </button>
      <button type="button" class="icon-only" title="Clear selection" aria-label="Clear selection" @click="s.exitVisual()">
        <kbd>esc</kbd>
        <PhX :size="16" />
      </button>
    </span>
  </div>
</template>

<style scoped>
.selection-bar {
  position: fixed;
  left: 50%;
  bottom: 76px;
  z-index: 70;
  display: flex;
  align-items: center;
  gap: 12px;
  width: min(calc(100vw - 32px), 820px);
  min-height: 46px;
  padding: 6px 8px 6px 10px;
  border: 1px solid var(--border-2);
  border-radius: 14px;
  background: color-mix(in oklab, var(--surface) 96%, transparent);
  box-shadow: var(--shadow-2), var(--top-hi);
  transform: translateX(-50%);
}

.selection-meta {
  flex: 1;
  min-width: 182px;
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.kbadge {
  flex: none;
  padding: 3px 9px;
  border-radius: 6px;
  background: var(--star, var(--orange));
  color: var(--accent-ink);
  font: 700 11px "JetBrains Mono", ui-monospace, monospace;
  text-transform: uppercase;
}

.selection-count {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  overflow: hidden;
  color: var(--text-mut);
  font: 11px "JetBrains Mono", ui-monospace, monospace;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.selection-count svg {
  flex: none;
  color: var(--accent);
}

.selection-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.selection-actions button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 32px;
  padding: 0 9px 0 7px;
  border: 1px solid transparent;
  border-radius: 9px;
  background: transparent;
  color: var(--text-dim);
  font: 12px "JetBrains Mono", ui-monospace, monospace;
  white-space: nowrap;
  transition: background var(--ease), border-color var(--ease), color var(--ease);
}

.selection-actions button:hover {
  border-color: var(--accent-line);
  background: var(--accent-soft);
  color: var(--text);
}

.selection-actions kbd {
  flex: none;
  min-width: 20px;
  display: inline-grid;
  place-items: center;
  height: 18px;
  padding: 0 5px;
  border: 1px solid var(--border-2);
  border-radius: 5px;
  background: var(--surface-3, var(--surface-2));
  color: var(--accent);
  font: 10px "JetBrains Mono", ui-monospace, monospace;
}

.selection-actions button.danger:hover {
  border-color: color-mix(in oklab, var(--red) 42%, transparent);
  background: color-mix(in oklab, var(--red) 16%, transparent);
  color: var(--red);
}

.selection-actions .icon-only {
  width: auto;
  padding: 0;
  color: var(--text-mut);
}

.selection-actions .icon-only kbd {
  min-width: 30px;
}

.selection-actions svg {
  flex: none;
}

@media (max-width: 720px) {
  .selection-bar {
    align-items: stretch;
    flex-direction: column;
    bottom: 68px;
    padding: 10px;
  }

  .selection-meta {
    min-width: 0;
  }

  .selection-actions {
    overflow-x: auto;
    padding-bottom: 1px;
  }
}

@media (max-width: 520px) {
  .selection-actions button span {
    display: none;
  }

  .selection-actions button {
    width: 34px;
    padding: 0;
  }
}
</style>
