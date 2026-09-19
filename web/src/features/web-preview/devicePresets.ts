export type DeviceId = 'mobile' | 'tablet' | 'laptop' | 'desktop'

export type ScreenOrientation = 'portrait' | 'landscape'

export type DevicePreset = {
  id: DeviceId
  label: string
  width: number
  height: number
  chrome: 'phone' | 'tablet' | 'laptop' | 'flat'
  natural: ScreenOrientation
}

export const DEVICE_PRESETS: readonly DevicePreset[] = [
  {
    id: 'mobile',
    label: 'Mobile',
    width: 390,
    height: 844,
    chrome: 'phone',
    natural: 'portrait',
  },
  {
    id: 'tablet',
    label: 'Tablet',
    width: 768,
    height: 1024,
    chrome: 'tablet',
    natural: 'portrait',
  },
  {
    id: 'laptop',
    label: 'Laptop',
    width: 1280,
    height: 800,
    chrome: 'laptop',
    natural: 'landscape',
  },
  {
    id: 'desktop',
    label: 'Desktop',
    width: 1440,
    height: 900,
    chrome: 'flat',
    natural: 'landscape',
  },
] as const

export function chromeBezel(chrome: DevicePreset['chrome']): number {
  switch (chrome) {
    case 'phone':
      return 12
    case 'tablet':
      return 14
    case 'laptop':
      return 10
    default:
      return 1
  }
}

export function getDevicePreset(id: DeviceId): DevicePreset {
  return DEVICE_PRESETS.find((d) => d.id === id) ?? DEVICE_PRESETS[0]
}

export function frameSize(
  device: Pick<DevicePreset, 'width' | 'height' | 'natural'>,
  orientation: ScreenOrientation,
): { width: number; height: number } {
  const portrait = {
    width: Math.min(device.width, device.height),
    height: Math.max(device.width, device.height),
  }
  const landscape = {
    width: Math.max(device.width, device.height),
    height: Math.min(device.width, device.height),
  }
  return orientation === 'landscape' ? landscape : portrait
}

export function outerFrameSize(
  frame: { width: number; height: number },
  chrome: DevicePreset['chrome'],
): { width: number; height: number; bezel: number } {
  const bezel = chromeBezel(chrome)
  return {
    width: frame.width + bezel * 2,
    height: frame.height + bezel * 2,
    bezel,
  }
}

export function toggleOrientation(
  current: ScreenOrientation,
): ScreenOrientation {
  return current === 'portrait' ? 'landscape' : 'portrait'
}

export function defaultOrientation(deviceId: DeviceId): ScreenOrientation {
  return getDevicePreset(deviceId).natural
}

export function scaleToFit(
  frameW: number,
  frameH: number,
  availW: number,
  availH: number,
  padding = 32,
): number {
  const aw = Math.max(0, availW - padding * 2)
  const ah = Math.max(0, availH - padding * 2)
  if (frameW <= 0 || frameH <= 0 || aw <= 0 || ah <= 0) return 1
  const s = Math.min(aw / frameW, ah / frameH, 1)
  return Math.round(s * 1000) / 1000
}

export function frameChromeClass(chrome: DevicePreset['chrome']): string {
  switch (chrome) {
    case 'phone':
      return 'rounded-[2.25rem] bg-shell-border shadow-2xl ring-1 ring-black/20'
    case 'tablet':
      return 'rounded-[1.75rem] bg-shell-border shadow-2xl ring-1 ring-black/15'
    case 'laptop':
      return 'rounded-xl bg-shell-border shadow-2xl ring-1 ring-black/10'
    default:
      return 'rounded-lg bg-shell-border shadow-xl ring-1 ring-shell-border'
  }
}
