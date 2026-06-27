// Shared registry for user-assignable folder icons. Used by the folder icon
// picker and the sidebar so both render from the same curated set.
import type { Component } from 'vue'
import {
  PhAirplaneTilt,
  PhArchive,
  PhBriefcase,
  PhBug,
  PhCalendarBlank,
  PhCode,
  PhCreditCard,
  PhCurrencyDollar,
  PhEnvelopeSimple,
  PhFire,
  PhFlag,
  PhFolderSimple,
  PhGift,
  PhGraduationCap,
  PhHeart,
  PhHouse,
  PhLightning,
  PhMegaphone,
  PhNote,
  PhPackage,
  PhPaintBrush,
  PhPalette,
  PhReceipt,
  PhRocket,
  PhShoppingCart,
  PhSparkle,
  PhStar,
  PhSuitcase,
  PhTag,
  PhTarget,
  PhTrophy,
  PhUsers,
  PhWrench,
} from '@phosphor-icons/vue'

export type FolderIconWeight = 'regular' | 'bold' | 'fill' | 'duotone'
export type FolderIconCat = 'all' | 'work' | 'money' | 'people' | 'fun' | 'system'

// Per-folder icon preference persisted in settings, keyed by mailbox id.
export type FolderIconPref = {
  icon: string
  weight: FolderIconWeight
  color: string // a CSS color (theme token) or '' to inherit the nav color
}

// Full result of the folder editor: rename + icon preference.
export type FolderEdit = FolderIconPref & { name: string }

export type FolderIconDef = { id: string; name: string; icon: Component; cats: FolderIconCat[] }

export const FOLDER_ICON_CATEGORIES: Array<{ id: FolderIconCat; label: string }> = [
  { id: 'all', label: 'All' },
  { id: 'work', label: 'Work' },
  { id: 'money', label: 'Money' },
  { id: 'people', label: 'People' },
  { id: 'fun', label: 'Fun' },
  { id: 'system', label: 'System' },
]

export const FOLDER_ICONS: FolderIconDef[] = [
  { id: 'folder', name: 'Folder', icon: PhFolderSimple, cats: ['system'] },
  { id: 'rocket', name: 'Rocket', icon: PhRocket, cats: ['work', 'fun'] },
  { id: 'briefcase', name: 'Briefcase', icon: PhBriefcase, cats: ['work'] },
  { id: 'suitcase', name: 'Suitcase', icon: PhSuitcase, cats: ['work'] },
  { id: 'target', name: 'Target', icon: PhTarget, cats: ['work'] },
  { id: 'megaphone', name: 'Megaphone', icon: PhMegaphone, cats: ['work'] },
  { id: 'code', name: 'Code', icon: PhCode, cats: ['work', 'system'] },
  { id: 'bug', name: 'Bug', icon: PhBug, cats: ['work', 'system'] },
  { id: 'wrench', name: 'Wrench', icon: PhWrench, cats: ['work', 'system'] },
  { id: 'package', name: 'Package', icon: PhPackage, cats: ['work'] },
  { id: 'note', name: 'Note', icon: PhNote, cats: ['work', 'system'] },
  { id: 'flag', name: 'Flag', icon: PhFlag, cats: ['work'] },
  { id: 'dollar', name: 'Dollar', icon: PhCurrencyDollar, cats: ['money'] },
  { id: 'receipt', name: 'Receipt', icon: PhReceipt, cats: ['money'] },
  { id: 'card', name: 'Card', icon: PhCreditCard, cats: ['money'] },
  { id: 'cart', name: 'Cart', icon: PhShoppingCart, cats: ['money'] },
  { id: 'users', name: 'People', icon: PhUsers, cats: ['people'] },
  { id: 'heart', name: 'Heart', icon: PhHeart, cats: ['people', 'fun'] },
  { id: 'gift', name: 'Gift', icon: PhGift, cats: ['people', 'fun'] },
  { id: 'grad', name: 'Graduation', icon: PhGraduationCap, cats: ['people'] },
  { id: 'house', name: 'House', icon: PhHouse, cats: ['people'] },
  { id: 'star', name: 'Star', icon: PhStar, cats: ['fun'] },
  { id: 'sparkle', name: 'Sparkle', icon: PhSparkle, cats: ['fun'] },
  { id: 'fire', name: 'Fire', icon: PhFire, cats: ['fun'] },
  { id: 'trophy', name: 'Trophy', icon: PhTrophy, cats: ['fun'] },
  { id: 'plane', name: 'Plane', icon: PhAirplaneTilt, cats: ['fun'] },
  { id: 'palette', name: 'Palette', icon: PhPalette, cats: ['fun'] },
  { id: 'brush', name: 'Paint Brush', icon: PhPaintBrush, cats: ['fun'] },
  { id: 'bolt', name: 'Lightning', icon: PhLightning, cats: ['system'] },
  { id: 'tag', name: 'Tag', icon: PhTag, cats: ['system'] },
  { id: 'archive', name: 'Archive', icon: PhArchive, cats: ['system'] },
  { id: 'calendar', name: 'Calendar', icon: PhCalendarBlank, cats: ['system'] },
  { id: 'envelope', name: 'Envelope', icon: PhEnvelopeSimple, cats: ['system'] },
]

export const FOLDER_ICON_WEIGHTS: FolderIconWeight[] = ['regular', 'bold', 'fill', 'duotone']

export const FOLDER_ICON_COLORS: Array<{ id: string; value: string }> = [
  { id: 'inherit', value: '' },
  { id: 'blue', value: 'var(--accent)' },
  { id: 'green', value: 'var(--green)' },
  { id: 'orange', value: 'var(--orange)' },
  { id: 'purple', value: 'var(--purple)' },
  { id: 'cyan', value: 'var(--cyan)' },
  { id: 'red', value: 'var(--red)' },
]

const byId = new Map(FOLDER_ICONS.map((d) => [d.id, d]))

export function folderIconComponent(id: string | undefined): Component | null {
  return id ? (byId.get(id)?.icon ?? null) : null
}

export function folderIconName(id: string | undefined): string {
  return (id && byId.get(id)?.name) || 'Folder'
}
