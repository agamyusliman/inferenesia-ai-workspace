/**
 * Image generation toggle helpers (VAL-CHAT-025).
 * Pure logic: max 4 reference images; unsupported → clear degrade message.
 * Renderer never calls provider image APIs; generate_image goes to core.Service.
 */

/** Max reference images accepted with image-gen toggle (matches MaxChatImages). */
export const MAX_IMAGE_GEN_REFS = 4

/** User-facing banner when provider cannot run image generation. */
export const IMAGE_GEN_UNSUPPORTED_MESSAGE =
  'Image generation is not supported by the current provider or model. Turn off Image gen or switch to a gateway that supports /v1/images/generations (or responses/image_generation).'

/** Short composer hint when toggle is on. */
export const IMAGE_GEN_TOGGLE_HINT =
  'Image gen on · prompt + up to 4 refs → core gateway path'

export type ImageGenRef = {
  name?: string
  media_type?: string
  data_url: string
}

/**
 * Cap reference images to MAX_IMAGE_GEN_REFS (first N kept).
 */
export function capImageGenRefs<T>(refs: T[] | null | undefined, max = MAX_IMAGE_GEN_REFS): T[] {
  if (!refs || refs.length === 0) return []
  if (max <= 0) return []
  return refs.slice(0, max)
}

export function shouldRequestImageGen(
  toggleOn: boolean,
  prompt: string,
  once = false,
): boolean {
  if (!toggleOn && !once) return false
  const cleaned = stripImageSlashPrefix(prompt)
  return Boolean(cleaned && cleaned.trim().length > 0)
}

export function stripImageSlashPrefix(prompt: string): string {
  const t = (prompt || '').trim()
  if (!t) return ''
  const m = t.match(/^\/image(?:\s+|:\s*|\s*$)/i)
  if (!m) return t
  return t.slice(m[0].length).trim()
}

export function promptHasImageSlash(prompt: string): boolean {
  return /^\/image(?:\s|$|:)/i.test((prompt || '').trim())
}

/**
 * Detect unsupported / degrade responses from core (no crash).
 * Matches clear error strings from the Go image path.
 */
export function isImageGenUnsupported(error: string | null | undefined): boolean {
  if (!error) return false
  const lower = error.toLowerCase()
  return (
    lower.includes('image generation is not supported') ||
    lower.includes('image_generation unsupported') ||
    lower.includes('does not support image generation') ||
    lower.includes('/responses/image_generation') ||
    lower.includes('/v1/images/generations') ||
    (lower.includes('image gen') &&
      (lower.includes('unsupported') ||
        lower.includes('not supported') ||
        lower.includes('non-json')))
  )
}

/** Friendly bubble body when generation fails with unsupported. */
export function imageGenDegradeContent(error?: string | null): string {
  if (error && isImageGenUnsupported(error)) {
    return IMAGE_GEN_UNSUPPORTED_MESSAGE
  }
  if (error && error.trim()) {
    return `(image gen) ${error.trim()}`
  }
  return IMAGE_GEN_UNSUPPORTED_MESSAGE
}
