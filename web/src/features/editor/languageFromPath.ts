/**
 * Map a workspace-relative path to a Monaco language id (VS Code-like).
 */
export function languageFromPath(path: string): string {
  const base = path.split(/[/\\]/).pop() || path
  const lower = base.toLowerCase()

  if (lower === 'dockerfile' || lower.endsWith('.dockerfile')) return 'dockerfile'
  if (lower === 'makefile' || lower === 'gnumakefile') return 'makefile'
  if (lower === 'cmakelists.txt') return 'cmake'
  if (lower.startsWith('.env') || lower === '.env') return 'ini'
  if (lower === 'go.mod' || lower === 'go.sum') return 'go'
  if (lower === 'package.json' || lower === 'tsconfig.json' || lower.endsWith('.jsonc')) {
    return 'json'
  }

  const dot = lower.lastIndexOf('.')
  const ext = dot >= 0 ? lower.slice(dot + 1) : ''

  switch (ext) {
    case 'ts':
    case 'mts':
    case 'cts':
      return 'typescript'
    case 'tsx':
      return 'typescript'
    case 'js':
    case 'mjs':
    case 'cjs':
      return 'javascript'
    case 'jsx':
      return 'javascript'
    case 'json':
      return 'json'
    case 'md':
    case 'mdx':
      return 'markdown'
    case 'go':
      return 'go'
    case 'py':
    case 'pyi':
      return 'python'
    case 'rs':
      return 'rust'
    case 'java':
      return 'java'
    case 'kt':
    case 'kts':
      return 'kotlin'
    case 'c':
    case 'h':
      return 'c'
    case 'cpp':
    case 'cc':
    case 'cxx':
    case 'hpp':
    case 'hh':
      return 'cpp'
    case 'cs':
      return 'csharp'
    case 'rb':
      return 'ruby'
    case 'php':
      return 'php'
    case 'swift':
      return 'swift'
    case 'css':
      return 'css'
    case 'scss':
      return 'scss'
    case 'less':
      return 'less'
    case 'html':
    case 'htm':
      return 'html'
    case 'xml':
    case 'svg':
      return 'xml'
    case 'yml':
    case 'yaml':
      return 'yaml'
    case 'excalidraw':
      return 'json'
    case 'toml':
      return 'ini'
    case 'ini':
    case 'cfg':
    case 'conf':
      return 'ini'
    case 'sh':
    case 'bash':
    case 'zsh':
    case 'fish':
      return 'shell'
    case 'ps1':
      return 'powershell'
    case 'sql':
      return 'sql'
    case 'mmd':
    case 'mermaid':
      return 'markdown'
    case 'graphql':
    case 'gql':
      return 'graphql'
    case 'proto':
      return 'protobuf'
    case 'tf':
    case 'hcl':
      return 'hcl'
    case 'vue':
      return 'html'
    case 'svelte':
      return 'html'
    case 'r':
      return 'r'
    case 'lua':
      return 'lua'
    case 'dart':
      return 'dart'
    case 'txt':
    case 'log':
      return 'plaintext'
    default:
      return 'plaintext'
  }
}
