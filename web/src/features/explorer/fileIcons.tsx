import type { ReactNode } from 'react'
import {
  Braces,
  File,
  FileCode,
  FileJson,
  FileText,
  FileType,
  Folder,
  FolderOpen,
  GitBranch,
  Image,
  Settings,
  Terminal,
} from 'lucide-react'

const ICON = 15

/** VS Code–style icons by extension / folder state (lucide-react). */
export function fileIcon(name: string, isDir: boolean, isOpen: boolean): ReactNode {
  if (isDir) {
    return isOpen ? (
      <FolderOpen size={ICON} className="shrink-0 text-amber-400/90" aria-hidden />
    ) : (
      <Folder size={ICON} className="shrink-0 text-amber-400/90" aria-hidden />
    )
  }

  const lower = name.toLowerCase()
  const ext = lower.includes('.') ? lower.slice(lower.lastIndexOf('.') + 1) : ''

  if (lower === '.gitignore' || lower === '.gitattributes') {
    return <GitBranch size={ICON} className="shrink-0 text-orange-400" aria-hidden />
  }
  if (lower === 'dockerfile' || lower.endsWith('.dockerfile')) {
    return <Settings size={ICON} className="shrink-0 text-sky-400" aria-hidden />
  }
  if (lower === 'package.json' || lower === 'package-lock.json' || ext === 'json' || ext === 'jsonc') {
    return <FileJson size={ICON} className="shrink-0 text-yellow-400" aria-hidden />
  }
  if (ext === 'md' || ext === 'mdx' || ext === 'txt' || ext === 'rst') {
    return <FileText size={ICON} className="shrink-0 text-sky-300" aria-hidden />
  }
  if (ext === 'ts' || ext === 'tsx' || ext === 'mts' || ext === 'cts') {
    return <FileCode size={ICON} className="shrink-0 text-blue-400" aria-hidden />
  }
  if (ext === 'js' || ext === 'jsx' || ext === 'mjs' || ext === 'cjs') {
    return <FileCode size={ICON} className="shrink-0 text-yellow-300" aria-hidden />
  }
  if (ext === 'go') {
    return <FileCode size={ICON} className="shrink-0 text-cyan-400" aria-hidden />
  }
  if (ext === 'py') {
    return <FileCode size={ICON} className="shrink-0 text-emerald-400" aria-hidden />
  }
  if (ext === 'rs') {
    return <FileCode size={ICON} className="shrink-0 text-orange-300" aria-hidden />
  }
  if (ext === 'css' || ext === 'scss' || ext === 'less') {
    return <Braces size={ICON} className="shrink-0 text-pink-400" aria-hidden />
  }
  if (ext === 'html' || ext === 'htm' || ext === 'svg') {
    return <FileType size={ICON} className="shrink-0 text-orange-400" aria-hidden />
  }
  if (ext === 'png' || ext === 'jpg' || ext === 'jpeg' || ext === 'gif' || ext === 'webp' || ext === 'ico') {
    return <Image size={ICON} className="shrink-0 text-purple-400" aria-hidden />
  }
  if (ext === 'sh' || ext === 'bash' || ext === 'zsh' || ext === 'fish') {
    return <Terminal size={ICON} className="shrink-0 text-green-400" aria-hidden />
  }
  if (ext === 'yml' || ext === 'yaml' || ext === 'toml' || ext === 'env' || lower.startsWith('.env')) {
    return <Settings size={ICON} className="shrink-0 text-shell-muted" aria-hidden />
  }

  return <File size={ICON} className="shrink-0 text-shell-muted" aria-hidden />
}
