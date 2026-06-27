import { Browser } from '@wailsio/runtime'
import { onMounted, onUnmounted } from 'vue'

function isExternalURL(value: string) {
  try {
    const url = new URL(value)
    return ['http:', 'https:', 'mailto:'].includes(url.protocol)
  } catch {
    return false
  }
}

async function openExternalURL(value: string) {
  if (!isExternalURL(value)) return
  try {
    await Browser.OpenURL(value)
  } catch {
    window.open(value, '_blank', 'noopener,noreferrer')
  }
}

export function useExternalEmailLinks() {
  function handleEmailFrameMessage(event: MessageEvent) {
    const data = event.data as { type?: unknown; href?: unknown } | null
    if (!data || data.type !== 'email-link-open' || typeof data.href !== 'string') return
    void openExternalURL(data.href)
  }

  onMounted(() => window.addEventListener('message', handleEmailFrameMessage))
  onUnmounted(() => window.removeEventListener('message', handleEmailFrameMessage))
}
