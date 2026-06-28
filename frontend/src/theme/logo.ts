import { getTheme } from './themes'

export function pigeonLogoForTheme(themeId: string): string {
  return getTheme(themeId).dark ? '/pigeon_light.svg' : '/pigeon_dark.svg'
}
