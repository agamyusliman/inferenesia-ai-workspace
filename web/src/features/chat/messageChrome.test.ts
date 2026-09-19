import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  codeFenceLanguage,
  extractMarkdownImages,
  hasFencedCode,
  isAssistantPlainInterim,
  isAssistantRole,
  isMermaidFence,
  isUserRole,
  messageCardClass,
  shouldRenderMarkdown,
  shouldShowActionBar,
  stripMarkdownImages,
  textForCopy,
} from './messageChrome'

describe('message roles & card chrome (VAL-CHAT-010)', () => {
  it('distinguishes user vs assistant roles', () => {
    assert.equal(isUserRole('user'), true)
    assert.equal(isAssistantRole('assistant'), true)
    assert.equal(isUserRole('assistant'), false)
    assert.equal(isAssistantRole('user'), false)
  })

  it('user card uses theme-aware bubble chrome', () => {
    const c = messageCardClass('user')
    assert.match(c, /shell-user-bubble/)
    assert.match(c, /rounded/)
  })

  it('assistant is flat stream (no full card chrome)', () => {
    const c = messageCardClass('assistant')
    assert.match(c, /shell-text/)
    assert.doesNotMatch(c, /rounded-lg/)
    assert.doesNotMatch(c, /border-shell-border/)
    assert.doesNotMatch(c, /shell-accent\/10/)
  })

  it('assistant plain interim is tighter than final', () => {
    const plain = messageCardClass('assistant', { plainInterim: true })
    const final = messageCardClass('assistant')
    assert.match(plain, /leading-snug/)
    assert.notEqual(plain, final)
  })

  it('user and assistant surfaces differ', () => {
    assert.notEqual(messageCardClass('user'), messageCardClass('assistant'))
  })
})

describe('action bar visibility (VAL-CHAT-011)', () => {
  it('shows after complete when not streaming', () => {
    assert.equal(shouldShowActionBar({ complete: true, streaming: false }), true)
  })
  it('hides while streaming even on hover', () => {
    assert.equal(shouldShowActionBar({ streaming: true, hovered: true }), false)
  })
  it('shows on focus when complete-ish', () => {
    assert.equal(shouldShowActionBar({ focused: true, complete: true }), true)
  })
  it('hides while streaming without hover/focus', () => {
    assert.equal(shouldShowActionBar({ streaming: true, complete: false }), false)
  })
  it('hides on empty incomplete idle', () => {
    assert.equal(shouldShowActionBar({ streaming: false, complete: false }), false)
  })
  it('hides plain interim assistant turns', () => {
    assert.equal(
      shouldShowActionBar({ complete: true, plainInterim: true, hovered: true }),
      false,
    )
  })
})

describe('assistant plain interim', () => {
  it('treats short progress and stopped as plain', () => {
    assert.equal(
      isAssistantPlainInterim({ content: 'Saya lanjut dari peta sebelumnya.' }),
      true,
    )
    assert.equal(isAssistantPlainInterim({ stopped: true, content: 'x' }), true)
    assert.equal(isAssistantPlainInterim({ error: true, content: 'err' }), true)
  })
  it('keeps tools/code/images as full chrome', () => {
    assert.equal(isAssistantPlainInterim({ hasTools: true, content: 'x' }), false)
    assert.equal(
      isAssistantPlainInterim({ content: 'see\n\n```ts\nconst x = 1\n```' }),
      false,
    )
    assert.equal(
      isAssistantPlainInterim({ content: '![a](https://example.com/a.png)' }),
      false,
    )
  })
})

describe('markdown helpers (VAL-CHAT-007/008/009)', () => {
  it('extracts markdown images for zoom wiring', () => {
    const imgs = extractMarkdownImages(
      'Hello ![a](https://example.com/a.png) and ![b](data:image/png;base64,abc)',
    )
    assert.equal(imgs.length, 2)
    assert.equal(imgs[0].alt, 'a')
    assert.equal(imgs[0].src, 'https://example.com/a.png')
    assert.equal(imgs[1].src, 'data:image/png;base64,abc')
  })

  it('extracts long base64 data-url images from generated markdown', () => {
    const b64 =
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
    const md = `Generated image for: logo\n\n![generated-1](data:image/png;base64,${b64})\n`
    const imgs = extractMarkdownImages(md)
    assert.equal(imgs.length, 1)
    assert.equal(imgs[0].alt, 'generated-1')
    assert.ok(imgs[0].src.startsWith('data:image/png;base64,'))
    assert.equal(stripMarkdownImages(md), 'Generated image for: logo')
    assert.equal(isAssistantPlainInterim({ content: md }), false)
  })

  it('detects fenced code blocks', () => {
    assert.equal(hasFencedCode('plain'), false)
    assert.equal(hasFencedCode('```ts\nconst x = 1\n```'), true)
  })

  it('parses code fence language from className', () => {
    assert.equal(codeFenceLanguage('language-typescript'), 'typescript')
    assert.equal(codeFenceLanguage('language-go'), 'go')
    assert.equal(codeFenceLanguage(undefined), '')
  })

  it('renders markdown for non-empty user/assistant content', () => {
    assert.equal(shouldRenderMarkdown('assistant', '# Hi'), true)
    assert.equal(shouldRenderMarkdown('user', 'hello'), true)
    assert.equal(shouldRenderMarkdown('assistant', ''), false)
    assert.equal(shouldRenderMarkdown('assistant', '   '), false)
    assert.equal(shouldRenderMarkdown('system', 'x'), false)
  })

  it('copy helper returns message text', () => {
    assert.equal(textForCopy('hello'), 'hello')
    assert.equal(textForCopy(null), '')
  })
})

describe('mermaid fence detection (VAL-DIAG-001)', () => {
  it('detects ```mermaid fence by language label', () => {
    assert.equal(isMermaidFence('mermaid', 'flowchart TD\n  A --> B'), true)
    assert.equal(isMermaidFence('mmd', 'erDiagram\n  USER ||--o{ POST : has'), true)
  })

  it('detects mermaid by diagram-type keyword when no language label', () => {
    assert.equal(isMermaidFence('', 'flowchart TD\n  A --> B'), true)
    assert.equal(isMermaidFence('', 'erDiagram\n  USER ||--o{ POST : has'), true)
    assert.equal(isMermaidFence('', 'sequenceDiagram\n  A->>B: Hi'), true)
  })

  it('does not misclassify non-mermaid fences', () => {
    assert.equal(isMermaidFence('typescript', 'const x = 1'), false)
    assert.equal(isMermaidFence('', 'not-a-diagram TD'), false)
    assert.equal(isMermaidFence('', ''), false)
  })
})
