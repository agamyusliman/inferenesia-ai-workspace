import { platformMessageVars } from '../../lib/platform'
import type { LocaleId } from './locale'
import {
  catalogFor,
  en,
  isActionKey,
  type MessageKey,
} from './messages'

export function translate(
  locale: LocaleId,
  key: MessageKey,
  vars?: Record<string, string | number>,
): string {
  let raw: string
  if (isActionKey(key)) {
    raw = en[key]
  } else {
    const catalog = catalogFor(locale)
    raw = catalog[key] ?? en[key] ?? String(key)
  }
  const merged: Record<string, string | number> = {
    ...platformMessageVars(),
    ...(vars || {}),
  }
  return raw.replace(/\{(\w+)\}/g, (match, name: string) => {
    if (Object.prototype.hasOwnProperty.call(merged, name)) {
      return String(merged[name])
    }
    return match
  })
}
