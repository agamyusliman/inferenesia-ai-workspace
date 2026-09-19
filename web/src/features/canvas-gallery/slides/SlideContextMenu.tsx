import { FormEvent, useEffect, useRef, useState } from 'react'
import {
  Image as ImageIcon,
  Languages,
  ListChecks,
  Maximize2,
  MessageSquareText,
  Minimize2,
  MoreVertical,
  SpellCheck,
  Sparkles,
  TrendingUp,
  Type,
  Wand2,
  X,
  Zap,
} from 'lucide-react'
import { sendSlideInstruction } from './slidesAgentApi'

type QuickAction = {
  id: string
  label: string
  icon: typeof Sparkles
  instruction: string
  group: 'layout' | 'writing' | 'image'
}

const QUICK_ACTIONS: QuickAction[] = [
  {
    id: 'new-layout',
    label: 'Coba tata letak baru',
    icon: Sparkles,
    instruction:
      'Redesign this slide with a completely different layout. Keep the same content but change the visual form (e.g. if cards, try timeline/stat/diagram/split instead). Make it visually distinct from the current version.',
    group: 'layout',
  },
  {
    id: 'improve-writing',
    label: 'Meningkatkan tulisan',
    icon: Type,
    instruction:
      'Improve the writing quality of this slide. Make the text more engaging, clear, and professional. Keep the same meaning but elevate the language.',
    group: 'writing',
  },
  {
    id: 'fix-grammar',
    label: 'Perbaiki ejaan & tata bahasa',
    icon: SpellCheck,
    instruction:
      'Fix any spelling, grammar, or punctuation errors in this slide. Keep the content and layout the same.',
    group: 'writing',
  },
  {
    id: 'translate',
    label: 'Menerjemahkan',
    icon: Languages,
    instruction:
      'Translate all text in this slide to English (if currently in Indonesian) or to Indonesian (if currently in English). Keep the layout and visual design the same.',
    group: 'writing',
  },
  {
    id: 'make-longer',
    label: 'Buat lebih lama',
    icon: Maximize2,
    instruction:
      'Expand the content of this slide. Add more detail, examples, or explanations to each point. Keep the same layout but fill it with richer content.',
    group: 'writing',
  },
  {
    id: 'make-shorter',
    label: 'Membuat lebih pendek',
    icon: Minimize2,
    instruction:
      'Condense the content of this slide. Make text more concise. Remove unnecessary words. Keep the key message but make it shorter and punchier.',
    group: 'writing',
  },
  {
    id: 'simplify',
    label: 'Menyederhanakan bahasa',
    icon: MessageSquareText,
    instruction:
      'Simplify the language in this slide. Use shorter words, shorter sentences. Make it easy to understand for a general audience. Keep the same content meaning.',
    group: 'writing',
  },
  {
    id: 'more-specific',
    label: 'Lebih spesifik',
    icon: ListChecks,
    instruction:
      'Make the content of this slide more specific and concrete. Add real examples, numbers, or specific details instead of vague statements.',
    group: 'writing',
  },
  {
    id: 'make-visual',
    label: 'Jadikan ini lebih visual',
    icon: Wand2,
    instruction:
      'Make this slide more visual. Replace text-heavy content with visual elements: icons, diagrams, charts, or visual metaphors. Reduce text, increase visual communication.',
    group: 'image',
  },
  {
    id: 'add-image',
    label: 'Menambahkan gambar',
    icon: ImageIcon,
    instruction:
      'Add an AI-generated image to this slide that reinforces the content. Place it in an appropriate position (side, background, or inline) without overlapping existing text.',
    group: 'image',
  },
  {
    id: 'add-chart',
    label: 'Menambahkan bagan',
    icon: TrendingUp,
    instruction:
      'Add a CSS-drawn chart or data visualization to this slide that represents the key data or metrics in the content. Use bar chart, progress bars, or stat blocks.',
    group: 'image',
  },
]

const GROUP_LABELS: Record<QuickAction['group'], string> = {
  layout: 'Tata letak',
  writing: 'Menulis',
  image: 'Gambar',
}

export function SlideContextMenu({
  canvasKey,
  slideId,
  busy,
  onClose,
}: {
  canvasKey: string
  slideId: string | null
  busy: boolean
  onClose: () => void
}) {
  const [input, setInput] = useState('')
  const rootRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!rootRef.current) return
    const onDoc = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        onClose()
      }
    }
    const onEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onEsc)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onEsc)
    }
  }, [onClose])

  const submitInstruction = (text: string) => {
    if (!text.trim() || busy) return
    sendSlideInstruction(canvasKey, { text: text.trim(), slideId: slideId || undefined })
    setInput('')
    onClose()
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    submitInstruction(input)
  }

  const groups: QuickAction['group'][] = ['layout', 'writing', 'image']

  return (
    <div
      ref={rootRef}
      data-testid="slide-context-menu"
      className="absolute right-2 top-2 z-30 w-72 rounded-lg border border-shell-border bg-shell-panel shadow-xl"
      role="dialog"
      aria-modal="false"
    >
      <div className="flex items-center justify-between border-b border-shell-border px-3 py-2">
        <span className="text-[11px] font-semibold text-shell-text">
          Edit slide ini
        </span>
        <button
          type="button"
          onClick={onClose}
          className="rounded p-0.5 text-shell-muted hover:bg-shell-border/30 hover:text-shell-text"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      <form onSubmit={onSubmit} className="border-b border-shell-border p-3">
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Bagaimana Anda ingin mengedit slide ini?"
          disabled={busy}
          rows={2}
          className="w-full resize-none rounded-md border border-shell-border bg-shell-bg px-2 py-1.5 text-[11px] text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent/50 disabled:opacity-50"
        />
        <div className="mt-2 flex items-center justify-end gap-2">
          <button
            type="submit"
            disabled={busy || !input.trim()}
            className="inline-flex items-center gap-1 rounded-md bg-shell-accent px-2.5 py-1 text-[10px] font-medium text-white hover:brightness-110 disabled:opacity-40"
          >
            <Zap className="h-3 w-3" />
            Kirim
          </button>
        </div>
      </form>

      <div className="max-h-64 overflow-y-auto p-2">
        {groups.map((group) => {
          const actions = QUICK_ACTIONS.filter((a) => a.group === group)
          if (!actions.length) return null
          return (
            <div key={group} className="mb-2 last:mb-0">
              <p className="px-1 py-1 text-[9px] font-semibold uppercase tracking-wide text-shell-muted">
                {GROUP_LABELS[group]}
              </p>
              <div className="flex flex-wrap gap-1">
                {actions.map((action) => {
                  const Icon = action.icon
                  return (
                    <button
                      key={action.id}
                      type="button"
                      disabled={busy}
                      onClick={() => submitInstruction(action.instruction)}
                      className="inline-flex items-center gap-1 rounded-md border border-shell-border bg-shell-bg px-2 py-1 text-[10px] text-shell-text hover:bg-shell-border/30 disabled:opacity-40"
                    >
                      <Icon className="h-3 w-3 shrink-0" />
                      {action.label}
                    </button>
                  )
                })}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function SlideMenuButton({
  onClick,
  busy,
}: {
  onClick: () => void
  busy: boolean
}) {
  return (
    <button
      type="button"
      data-testid="slide-menu-btn"
      onClick={(e) => {
        e.stopPropagation()
        onClick()
      }}
      disabled={busy}
      className="absolute right-1.5 top-1.5 z-20 inline-flex h-5 w-5 items-center justify-center rounded bg-black/40 text-white/70 backdrop-blur-sm hover:bg-black/60 hover:text-white disabled:opacity-30"
      title="Edit slide"
    >
      <MoreVertical className="h-3 w-3" />
    </button>
  )
}
