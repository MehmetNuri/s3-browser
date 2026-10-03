import { SetLanguage } from './api'
import { en, type Dict } from './locales/en'
import { tr } from './locales/tr'
import { de } from './locales/de'
import { fr } from './locales/fr'
import { es } from './locales/es'
import { pt } from './locales/pt'
import { ru } from './locales/ru'
import { ar } from './locales/ar'
import { zh } from './locales/zh'

export type Lang = 'en' | 'tr' | 'de' | 'fr' | 'es' | 'pt' | 'ru' | 'ar' | 'zh'
// Native names so a user can find their language without knowing the current one.
export const languages: { id: Lang; label: string; dir: 'ltr' | 'rtl' }[] = [
  { id: 'en', label: 'English', dir: 'ltr' },
  { id: 'tr', label: 'Türkçe', dir: 'ltr' },
  { id: 'de', label: 'Deutsch', dir: 'ltr' },
  { id: 'fr', label: 'Français', dir: 'ltr' },
  { id: 'es', label: 'Español', dir: 'ltr' },
  { id: 'pt', label: 'Português', dir: 'ltr' },
  { id: 'ru', label: 'Русский', dir: 'ltr' },
  { id: 'ar', label: 'العربية', dir: 'rtl' },
  { id: 'zh', label: '中文', dir: 'ltr' },
]

const dicts: Record<Lang, Dict> = { en, tr, de, fr, es, pt, ru, ar, zh }
export type Key = keyof Dict
export type { Dict }

const isLang = (value: string | null | undefined): value is Lang => !!value && value in dicts

function initialLang(): Lang {
  try {
    const saved = localStorage.getItem('lang')
    if (isLang(saved)) return saved
  } catch {}
  // The first matching browser language, by its primary tag ("pt-BR" → "pt").
  for (const tag of navigator.languages ?? [navigator.language]) {
    const primary = tag?.toLowerCase().split('-')[0]
    if (isLang(primary)) return primary
  }
  return 'en'
}

export const i18n = $state({ lang: initialLang() })

export const direction = (lang: Lang) => languages.find((l) => l.id === lang)?.dir ?? 'ltr'

// t reads i18n.lang, so templates using it re-render when the language changes.
export function t(key: Key, params?: Record<string, string | number>): string {
  let s: string = dicts[i18n.lang][key] ?? en[key] ?? key
  if (params) for (const [k, v] of Object.entries(params)) s = s.replaceAll(`{${k}}`, String(v))
  return s
}

export function setLang(lang: Lang) {
  i18n.lang = lang
  try { localStorage.setItem('lang', lang) } catch {}
  syncBackend()
}

// syncBackend tells Go which language to use for its messages and sets the
// document language and writing direction (Arabic is right-to-left).
export function syncBackend() {
  document.documentElement.lang = i18n.lang
  document.documentElement.dir = direction(i18n.lang)
  SetLanguage(i18n.lang).catch(() => {})
}
