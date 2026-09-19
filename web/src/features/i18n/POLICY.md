# i18n policy — mixed language (en / id)

## Rule

**Only a narrow set of tight chrome action labels stay English in all locales.**  
These sit in dense toolbars where long Indonesian would stretch the shell.

**Everything else is localized** when locale is `id` — including most buttons, menus, panels, settings copy, palette command labels, tooltips, empty states, and confirm dialogs.

## Forced-English action keys (chrome only)

These keys always resolve to English via `isActionKey` / `ACTION_MESSAGE_KEYS`, even when UI locale is `id`:

| Key | EN label |
|-----|----------|
| `action.save` | Save |
| `action.undo` | Undo |
| `action.redo` | Redo |
| `action.close` | Close |
| `action.cancel` | Cancel |
| `action.copy` | Copy |
| `action.send` | Send |
| `action.stop` | Stop |

**Not forced English** (translate in the `id` catalog):

- `action.saveAll` → Simpan semua
- `action.apply` → Terapkan
- `action.generate` → Buat
- `action.delete` → Hapus
- `action.open` → Buka
- `action.new` → Baru
- `action.refresh` → Segarkan
- `action.test` → Uji
- `action.add` → Tambah
- `action.edit` → Ubah
- `action.canvas` → Playground
- `action.source` → Sumber
- Product terms that stay EN for consistency but are **not** in the force list: `action.revertAgent` (“Revert agent”), `action.restoreHead` (“Restore to HEAD”) — same string in en + id catalogs.

## Surfaces

| Surface | Localized? |
|---------|------------|
| Sidebar tooltips / aria | Yes (ID) |
| Settings (headers, help, buttons, forms, Token Saver descriptions) | Yes (ID) |
| Docs panel chrome (title, search, All, categories) | Yes (ID) |
| Docs article titles, summaries, bodies | Yes — Markdown under `web/src/features/docs/articles/{en,id}/` |
| Status bar labels / tooltips | Yes (ID) |
| Undo toolbar chrome buttons | EN only for Undo/Redo; product labels Revert agent / Restore to HEAD stay EN strings |
| Undo hint line | Short ID ok (`undo.hint`) |
| Command palette labels | Yes (ID) — Save/Close-style **palette** keys may still say Save when they map to chrome |
| Explorer / git / workflow / terminal / layout / panels | Yes (ID) |
| Product names (Inferenesia, Live Blocks, Token Saver, RTK, Headroom, Stage, Commit) | Unchanged short product terms OK |
| Familiar short labels (Apply/Generate may stay EN in catalog as Terapkan/Buat — either is fine) | Per catalog |

## Storage

- Key: `inferenesia-locale` (legacy read: `yura-ai-locale`)
- Values: `en` \| `id` (default `en`)
- Also sets `document.documentElement.lang`
