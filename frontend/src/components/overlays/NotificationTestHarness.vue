<script setup lang="ts">
// drives the wails notifications service directly. desktop only —
// actions/replies won't fire in the browser preview.
import { onMounted, onUnmounted, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import {
  NotificationCategory,
  NotificationOptions,
  NotificationService,
} from '../../bindings/github.com/wailsapp/wails/v3/pkg/services/notifications'

const notifLog = ref<Array<{ id: number; text: string }>>([])
const notifAuth = ref<boolean | null>(null)

let logSeq = 0
function logNotif(text: string) {
  logSeq += 1
  notifLog.value = [{ id: logSeq, text }, ...notifLog.value].slice(0, 12)
}

let notifSeq = 0
function notifId() {
  notifSeq += 1
  return `test-${notifSeq}-${Math.floor(performance.now())}`
}

async function requestAuth() {
  try {
    notifAuth.value = await NotificationService.RequestNotificationAuthorization()
    logNotif(`RequestAuthorization → ${notifAuth.value}`)
  } catch (err) {
    logNotif(`RequestAuthorization error: ${String(err)}`)
  }
}

async function checkAuth() {
  try {
    notifAuth.value = await NotificationService.CheckNotificationAuthorization()
    logNotif(`CheckAuthorization → ${notifAuth.value}`)
  } catch (err) {
    logNotif(`CheckAuthorization error: ${String(err)}`)
  }
}

async function sendBasic() {
  const id = notifId()
  try {
    await NotificationService.SendNotification(
      new NotificationOptions({
        id,
        title: 'New message',
        subtitle: 'inbox',
        body: 'This is a basic test notification.',
      }),
    )
    logNotif(`SendNotification(${id}) sent`)
  } catch (err) {
    logNotif(`SendNotification error: ${String(err)}`)
  }
}

async function sendWithActions() {
  const categoryId = 'test-actions'
  const id = notifId()
  try {
    await NotificationService.RegisterNotificationCategory(
      new NotificationCategory({
        id: categoryId,
        actions: [
          { id: 'archive', title: 'Archive' },
          { id: 'reply', title: 'Reply' },
        ],
        hasReplyField: true,
        replyPlaceholder: 'Type a reply…',
        replyButtonTitle: 'Send',
      }),
    )
    await NotificationService.SendNotificationWithActions(
      new NotificationOptions({
        id,
        title: 'Message with actions',
        subtitle: 'inbox',
        body: 'Tap an action button (desktop only).',
        categoryId,
      }),
    )
    logNotif(`SendNotificationWithActions(${id}) sent`)
  } catch (err) {
    logNotif(`SendNotificationWithActions error: ${String(err)}`)
  }
}

async function clearAll() {
  try {
    await NotificationService.RemoveAllDeliveredNotifications()
    await NotificationService.RemoveAllPendingNotifications()
    logNotif('Cleared delivered + pending notifications')
  } catch (err) {
    logNotif(`Clear error: ${String(err)}`)
  }
}

let notifOff: (() => void) | undefined
let mailOff: (() => void) | undefined
onMounted(() => {
  notifOff = Events.On('notification:action', (ev: { data?: unknown }) => {
    logNotif(`action response: ${JSON.stringify(ev?.data)}`)
  })
  // sync loop emits this on each poll that pulls new mail
  mailOff = Events.On('mail:new', (ev: { data?: unknown }) => {
    logNotif(`poll → new mail: ${JSON.stringify(ev?.data)}`)
  })
})
onUnmounted(() => {
  notifOff?.()
  mailOff?.()
})
</script>

<template>
  <p class="set-section">Test harness</p>
  <p class="set-note">
    Drives the Wails notifications service directly. Authorization:
    <b>{{ notifAuth === null ? 'unknown' : notifAuth ? 'granted' : 'denied' }}</b
    >. Desktop only — actions/replies won't fire in the browser preview.
  </p>
  <div class="notif-actions">
    <button class="set-btn" type="button" @click="requestAuth">Request authorization</button>
    <button class="set-btn" type="button" @click="checkAuth">Check authorization</button>
    <button class="set-btn" type="button" @click="sendBasic">Send basic</button>
    <button class="set-btn" type="button" @click="sendWithActions">Send with actions</button>
    <button class="set-btn" type="button" @click="clearAll">Clear all</button>
  </div>
  <p class="set-section">Log</p>
  <ul class="notif-log">
    <li v-if="!notifLog.length" class="set-note">No events yet.</li>
    <li v-for="entry in notifLog" :key="entry.id">{{ entry.text }}</li>
  </ul>
</template>
