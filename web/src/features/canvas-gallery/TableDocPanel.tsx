import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Braces,
  Columns3,
  Download,
  Eye,
  EyeOff,
  GripVertical,
  Plus,
  Rows3,
  Sheet,
  Trash2,
  Upload,
} from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'
import { DocAgentPanel } from './DocAgentPanel'
import { TableFormatToolbar, dragClasses } from './TableFormatToolbar'
import {
  DOC_PANEL_TOOL_BUTTON,
  DocPanelHeader,
  downloadDocFile,
  pickTextFile,
  useDocPanelNarrow,
} from './docPanelChrome'
import {
  activeSheet,
  addSheet,
  addTableColumn,
  addTableRow,
  getCellStyle,
  mergeAt,
  mergeCells,
  moveColumn,
  moveRow,
  parseTableWorkbook,
  removeSheet,
  removeTableColumn,
  removeTableRow,
  renameSheet,
  renameTableColumn,
  serializeTableDoc,
  serializeTableWorkbook,
  setActiveSheet,
  setCellStyles,
  setColumnWidth,
  setRowHeight,
  setTableCell,
  showAllColumns,
  tableFromCsv,
  tableToMarkdown,
  toCsv,
  toggleColumnHidden,
  toggleRowHidden,
  unmergeCells,
  updateSheetDoc,
  workbookFromDoc,
  TABLE_MAX_SHEETS,
  type TableCellStyle,
  type TableDoc,
  type TableWorkbook,
} from './tableDoc'
import { evaluateGrid, formatCellValue, isFormula } from './tableFormula'
import { downloadXlsx } from './xlsx'

type Props = {
  contextId?: string
  canvasKey: string
  canvasEntrySessionId?: string
  title: string
  scopeLabel: string
  content: string
  dirty: boolean
  saveFlash: boolean
  error?: string | null
  onContentChange: (next: string) => void
  onSave: (next: string) => void
  onApplyFromAgent: (next: string, opts: { canvasId: string }) => void
  onDismissError?: () => void
}

type Cursor = { row: number; col: number }
type DragState = { kind: 'row' | 'col'; from: number; over: number } | null

const DEFAULT_COL_WIDTH = 128
const DEFAULT_ROW_HEIGHT = 28

export function TableDocPanel({
  contextId,
  canvasKey,
  canvasEntrySessionId,
  title,
  scopeLabel,
  content,
  dirty,
  saveFlash,
  error,
  onContentChange,
  onSave,
  onApplyFromAgent,
  onDismissError,
}: Props) {
  const { t } = useLocale()
  const [localError, setLocalError] = useState<string | null>(null)
  const [cursor, setCursor] = useState<Cursor | null>(null)
  const [anchor, setAnchor] = useState<Cursor | null>(null)
  const [editing, setEditing] = useState<Cursor | null>(null)
  const [drag, setDrag] = useState<DragState>(null)
  const contentRef = useRef(content)
  contentRef.current = content
  const { ref: panelRef, narrow } = useDocPanelNarrow(560)

  const parsed = useMemo(() => parseTableWorkbook(content), [content])
  const workbook = parsed.workbook
  const sheet = activeSheet(workbook)
  const doc = sheet.doc
  const values = useMemo(() => evaluateGrid(doc), [doc])

  useEffect(() => {
    setLocalError(null)
    setCursor(null)
    setAnchor(null)
    setEditing(null)
  }, [canvasKey])

  const commitWorkbook = useCallback(
    (next: TableWorkbook) => onContentChange(serializeTableWorkbook(next)),
    [onContentChange],
  )

  const commit = useCallback(
    (next: TableDoc) => commitWorkbook(updateSheetDoc(workbook, sheet.id, next)),
    [commitWorkbook, sheet.id, workbook],
  )

  const onImport = useCallback(async () => {
    setLocalError(null)
    const picked = await pickTextFile('.csv,.tsv,.txt,text/csv,text/plain')
    if (!picked) return
    if (picked.error) {
      setLocalError(picked.error)
      return
    }
    const imported = tableFromCsv(picked.text)
    if (imported.error) {
      setLocalError(`${picked.name}: ${imported.error}`)
      return
    }
    if (imported.doc.rows.length === 0) {
      setLocalError(t('tableDoc.importEmpty', { name: picked.name }))
      return
    }
    onSave(serializeTableWorkbook(workbookFromDoc(imported.doc, picked.name)))
  }, [onSave, t])

  const selection = useMemo(() => {
    if (!cursor) return []
    const a = anchor || cursor
    const cells: Cursor[] = []
    for (let r = Math.min(a.row, cursor.row); r <= Math.max(a.row, cursor.row); r += 1) {
      for (let c = Math.min(a.col, cursor.col); c <= Math.max(a.col, cursor.col); c += 1) {
        cells.push({ row: r, col: c })
      }
    }
    return cells
  }, [anchor, cursor])

  const selectionBounds = useMemo(() => {
    if (!cursor) return null
    const a = anchor || cursor
    return {
      row: Math.min(a.row, cursor.row),
      col: Math.min(a.col, cursor.col),
      rowSpan: Math.abs(a.row - cursor.row) + 1,
      colSpan: Math.abs(a.col - cursor.col) + 1,
    }
  }, [anchor, cursor])

  const applyStyle = useCallback(
    (patch: TableCellStyle | null) => {
      if (selection.length === 0) return
      commit(setCellStyles(doc, selection, patch))
    },
    [commit, doc, selection],
  )

  const columnCount = doc.columns.length
  const rowCount = doc.rows.length
  const hiddenCols = useMemo(() => new Set(doc.hiddenColumns || []), [doc.hiddenColumns])
  const hiddenRows = useMemo(() => new Set(doc.hiddenRows || []), [doc.hiddenRows])
  const activeMerge = cursor ? mergeAt(doc, cursor.row, cursor.col) : undefined

  const onDropRow = useCallback(() => {
    if (drag?.kind === 'row' && drag.from !== drag.over) {
      commit(moveRow(doc, drag.from, drag.over))
    }
    setDrag(null)
  }, [commit, doc, drag])

  const onDropCol = useCallback(() => {
    if (drag?.kind === 'col' && drag.from !== drag.over) {
      commit(moveColumn(doc, drag.from, drag.over))
    }
    setDrag(null)
  }, [commit, doc, drag])

  const startResize = useCallback(
    (kind: 'col' | 'row', index: number, startPos: number, startSize: number) => {
      const move = (e: PointerEvent) => {
        const delta = (kind === 'col' ? e.clientX : e.clientY) - startPos
        const next = startSize + delta
        commit(
          kind === 'col'
            ? setColumnWidth(doc, index, next)
            : setRowHeight(doc, index, next),
        )
      }
      const up = () => {
        window.removeEventListener('pointermove', move)
        window.removeEventListener('pointerup', up)
      }
      window.addEventListener('pointermove', move)
      window.addEventListener('pointerup', up)
    },
    [commit, doc],
  )

  return (
    <div
      ref={panelRef}
      data-testid="table-doc-panel"
      data-narrow={narrow ? 'true' : 'false'}
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
    >
      <DocPanelHeader
        compact={narrow}
        title={title}
        scopeLabel={scopeLabel}
        dirty={dirty}
        saveFlash={saveFlash}
        statusText={t('tableDoc.statsSheets', {
          rows: rowCount,
          columns: columnCount,
          sheets: workbook.sheets.length,
        })}
        error={localError || parsed.error || error || null}
        onDismissError={() => {
          setLocalError(null)
          onDismissError?.()
        }}
        onSave={() => onSave(contentRef.current)}
        testId="table-doc"
        tools={
          <>
            <button
              type="button"
              data-testid="table-doc-add-row"
              onClick={() => commit(addTableRow(doc))}
              title={t('tableDoc.addRow')}
              aria-label={t('tableDoc.addRow')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Rows3 size={12} strokeWidth={2} aria-hidden />
              <Plus size={9} strokeWidth={3} aria-hidden />
            </button>
            <button
              type="button"
              data-testid="table-doc-add-column"
              onClick={() => commit(addTableColumn(doc))}
              title={t('tableDoc.addColumn')}
              aria-label={t('tableDoc.addColumn')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Columns3 size={12} strokeWidth={2} aria-hidden />
              <Plus size={9} strokeWidth={3} aria-hidden />
            </button>
            {hiddenCols.size > 0 || hiddenRows.size > 0 ? (
              <button
                type="button"
                data-testid="table-doc-show-all"
                onClick={() => {
                  let next = showAllColumns(doc)
                  for (const r of [...hiddenRows]) next = toggleRowHidden(next, r)
                  commit(next)
                }}
                title={t('tableDoc.showAll')}
                aria-label={t('tableDoc.showAll')}
                className={DOC_PANEL_TOOL_BUTTON}
              >
                <Eye size={12} strokeWidth={2} aria-hidden />
                {narrow ? null : (
                  <span>{hiddenCols.size + hiddenRows.size}</span>
                )}
              </button>
            ) : null}
            <button
              type="button"
              data-testid="table-doc-import"
              onClick={() => void onImport()}
              title={t('tableDoc.importCsv')}
              aria-label={t('tableDoc.importCsv')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Upload size={12} strokeWidth={2} aria-hidden />
            </button>
            <button
              type="button"
              data-testid="table-doc-export-xlsx"
              onClick={() => downloadXlsx(workbook.sheets, `${title || 'table'}.xlsx`)}
              title={t('tableDoc.exportXlsx')}
              aria-label={t('tableDoc.exportXlsx')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Sheet size={12} strokeWidth={2} aria-hidden />
              {narrow ? null : <span>XLSX</span>}
            </button>
            <button
              type="button"
              data-testid="table-doc-export-csv"
              onClick={() =>
                downloadDocFile(toCsv(doc), `${title || 'table'}.csv`, 'text/csv')
              }
              title={t('tableDoc.exportCsv')}
              aria-label={t('tableDoc.exportCsv')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Download size={12} strokeWidth={2} aria-hidden />
              {narrow ? null : <span>CSV</span>}
            </button>
            <button
              type="button"
              data-testid="table-doc-export-markdown"
              onClick={() =>
                downloadDocFile(
                  tableToMarkdown(doc),
                  `${title || 'table'}.md`,
                  'text/markdown',
                )
              }
              title={t('tableDoc.exportMarkdown')}
              aria-label={t('tableDoc.exportMarkdown')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Download size={12} strokeWidth={2} aria-hidden />
              {narrow ? null : <span>MD</span>}
            </button>
            <button
              type="button"
              data-testid="table-doc-export-json"
              onClick={() =>
                downloadDocFile(
                  serializeTableDoc(doc),
                  `${title || 'table'}.json`,
                  'application/json',
                )
              }
              title={t('tableDoc.exportJson')}
              aria-label={t('tableDoc.exportJson')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Braces size={12} strokeWidth={2} aria-hidden />
            </button>
          </>
        }
      />

      <TableFormatToolbar
        style={cursor ? getCellStyle(doc, cursor.row, cursor.col) : undefined}
        selectionSize={selection.length}
        onPatch={applyStyle}
        canMerge={Boolean(
          selectionBounds &&
            (selectionBounds.rowSpan > 1 || selectionBounds.colSpan > 1),
        )}
        canUnmerge={Boolean(activeMerge)}
        onMerge={() => {
          if (!selectionBounds) return
          commit(
            mergeCells(
              doc,
              selectionBounds.row,
              selectionBounds.col,
              selectionBounds.rowSpan,
              selectionBounds.colSpan,
            ),
          )
          setAnchor(null)
        }}
        onUnmerge={() => {
          if (!activeMerge) return
          commit(unmergeCells(doc, activeMerge.row, activeMerge.col))
        }}
      />

      <div
        data-testid="table-doc-grid-wrap"
        className="shell-scroll min-h-0 min-w-0 flex-1 overflow-auto bg-shell-bg"
      >
        <table
          data-testid="table-doc-grid"
          data-rows={rowCount}
          data-columns={columnCount}
          className="w-max min-w-full border-collapse text-[11px]"
        >
          <thead className="sticky top-0 z-10">
            <tr>
              <th
                scope="col"
                className="sticky left-0 z-20 w-10 border-b border-r border-shell-border bg-shell-panel px-1 py-1 text-[9px] font-medium text-shell-muted"
              >
                #
              </th>
              {doc.columns.map((col, ci) => {
                if (hiddenCols.has(ci)) {
                  return (
                    <th
                      key={col.id}
                      scope="col"
                      data-testid={`table-doc-column-hidden-${ci}`}
                      className="w-3 border-b border-r border-shell-border bg-shell-panel p-0"
                    >
                      <button
                        type="button"
                        onClick={() => commit(toggleColumnHidden(doc, ci))}
                        title={t('tableDoc.showColumn', { name: col.name })}
                        aria-label={t('tableDoc.showColumn', { name: col.name })}
                        className="flex h-7 w-3 items-center justify-center text-shell-accent"
                      >
                        <span className="text-[8px]">›</span>
                      </button>
                    </th>
                  )
                }
                return (
                  <th
                    key={col.id}
                    scope="col"
                    data-col={ci}
                    onDragOver={(e) => {
                      if (drag?.kind !== 'col') return
                      e.preventDefault()
                      if (drag.over !== ci) setDrag({ ...drag, over: ci })
                    }}
                    onDrop={onDropCol}
                    style={{ width: doc.widths?.[ci] || DEFAULT_COL_WIDTH }}
                    className={`relative border-b border-r border-shell-border bg-shell-panel p-0 text-left align-middle ${dragClasses(
                      {
                        dragging: drag?.kind === 'col' && drag.from === ci,
                        dropTarget: drag?.kind === 'col' && drag.over === ci,
                      },
                    )}`}
                  >
                    <div className="flex items-center gap-0.5 px-0.5">
                      <button
                        type="button"
                        draggable
                        data-testid={`table-doc-col-handle-${ci}`}
                        onDragStart={() => setDrag({ kind: 'col', from: ci, over: ci })}
                        onDragEnd={() => setDrag(null)}
                        title={t('tableDoc.dragColumn')}
                        aria-label={t('tableDoc.dragColumn')}
                        className="shrink-0 cursor-grab rounded p-0.5 text-shell-muted hover:text-shell-text active:cursor-grabbing"
                      >
                        <GripVertical size={9} strokeWidth={2} aria-hidden />
                      </button>
                      <input
                        data-testid={`table-doc-column-${ci}`}
                        value={col.name}
                        onChange={(e) =>
                          commit(renameTableColumn(doc, ci, e.target.value))
                        }
                        aria-label={t('tableDoc.columnName', { index: ci + 1 })}
                        className="h-7 min-w-0 flex-1 bg-transparent px-0.5 text-[11px] font-semibold text-shell-text outline-none focus:bg-shell-bg"
                      />
                      <button
                        type="button"
                        data-testid={`table-doc-hide-column-${ci}`}
                        onClick={() => commit(toggleColumnHidden(doc, ci))}
                        title={t('tableDoc.hideColumn')}
                        aria-label={t('tableDoc.hideColumn')}
                        className="shrink-0 rounded p-0.5 text-shell-muted transition hover:text-shell-text"
                      >
                        <EyeOff size={9} strokeWidth={2} aria-hidden />
                      </button>
                      <button
                        type="button"
                        data-testid={`table-doc-remove-column-${ci}`}
                        onClick={() => commit(removeTableColumn(doc, ci))}
                        disabled={columnCount <= 1}
                        title={t('tableDoc.removeColumn')}
                        aria-label={t('tableDoc.removeColumn')}
                        className="shrink-0 rounded p-0.5 text-shell-muted transition hover:text-red-400 disabled:opacity-30"
                      >
                        <Trash2 size={9} strokeWidth={2} aria-hidden />
                      </button>
                    </div>
                    <span
                      data-testid={`table-doc-col-resize-${ci}`}
                      role="separator"
                      aria-orientation="vertical"
                      onPointerDown={(e) => {
                        e.preventDefault()
                        startResize(
                          'col',
                          ci,
                          e.clientX,
                          doc.widths?.[ci] || DEFAULT_COL_WIDTH,
                        )
                      }}
                      className="absolute right-0 top-0 h-full w-1 cursor-col-resize hover:bg-shell-accent"
                    />
                  </th>
                )
              })}
              <th scope="col" className="border-b border-shell-border bg-shell-panel px-1">
                <button
                  type="button"
                  data-testid="table-doc-add-column-inline"
                  onClick={() => commit(addTableColumn(doc))}
                  title={t('tableDoc.addColumn')}
                  aria-label={t('tableDoc.addColumn')}
                  className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted transition hover:bg-shell-hover hover:text-shell-text"
                >
                  <Plus size={12} strokeWidth={2} aria-hidden />
                </button>
              </th>
            </tr>
          </thead>
          <tbody>
            {doc.rows.map((row, ri) => {
              if (hiddenRows.has(ri)) {
                return (
                  <tr key={ri} data-testid={`table-doc-row-hidden-${ri}`}>
                    <th
                      scope="row"
                      className="sticky left-0 z-10 border-b border-r border-shell-border bg-shell-panel p-0"
                    >
                      <button
                        type="button"
                        onClick={() => commit(toggleRowHidden(doc, ri))}
                        title={t('tableDoc.showRow', { index: ri + 1 })}
                        aria-label={t('tableDoc.showRow', { index: ri + 1 })}
                        className="flex h-3 w-full items-center justify-center text-[8px] text-shell-accent"
                      >
                        ⌄
                      </button>
                    </th>
                    <td colSpan={columnCount + 1} className="h-3 border-b border-shell-border" />
                  </tr>
                )
              }
              return (
                <tr
                  key={ri}
                  className="group"
                  onDragOver={(e) => {
                    if (drag?.kind !== 'row') return
                    e.preventDefault()
                    if (drag.over !== ri) setDrag({ ...drag, over: ri })
                  }}
                  onDrop={onDropRow}
                  style={{ height: doc.heights?.[ri] || undefined }}
                >
                  <th
                    scope="row"
                    className={`sticky left-0 z-10 border-b border-r border-shell-border bg-shell-panel p-0 text-center align-middle ${dragClasses(
                      {
                        dragging: drag?.kind === 'row' && drag.from === ri,
                        dropTarget: drag?.kind === 'row' && drag.over === ri,
                      },
                    )}`}
                  >
                    <div className="relative flex items-center justify-center gap-0.5 px-0.5">
                      <button
                        type="button"
                        draggable
                        data-testid={`table-doc-row-handle-${ri}`}
                        onDragStart={() => setDrag({ kind: 'row', from: ri, over: ri })}
                        onDragEnd={() => setDrag(null)}
                        title={t('tableDoc.dragRow')}
                        aria-label={t('tableDoc.dragRow')}
                        className="shrink-0 cursor-grab rounded text-shell-muted opacity-0 transition group-hover:opacity-100 active:cursor-grabbing"
                      >
                        <GripVertical size={9} strokeWidth={2} aria-hidden />
                      </button>
                      <span className="w-4 text-[9px] tabular-nums text-shell-muted">
                        {ri + 1}
                      </span>
                      <button
                        type="button"
                        data-testid={`table-doc-hide-row-${ri}`}
                        onClick={() => commit(toggleRowHidden(doc, ri))}
                        title={t('tableDoc.hideRow')}
                        aria-label={t('tableDoc.hideRow')}
                        className="rounded p-0.5 text-shell-muted opacity-0 transition group-hover:opacity-100 hover:text-shell-text"
                      >
                        <EyeOff size={9} strokeWidth={2} aria-hidden />
                      </button>
                      <button
                        type="button"
                        data-testid={`table-doc-remove-row-${ri}`}
                        onClick={() => commit(removeTableRow(doc, ri))}
                        title={t('tableDoc.removeRow')}
                        aria-label={t('tableDoc.removeRow')}
                        className="rounded p-0.5 text-shell-muted opacity-0 transition group-hover:opacity-100 hover:text-red-400"
                      >
                        <Trash2 size={9} strokeWidth={2} aria-hidden />
                      </button>
                      <span
                        data-testid={`table-doc-row-resize-${ri}`}
                        role="separator"
                        aria-orientation="horizontal"
                        onPointerDown={(e) => {
                          e.preventDefault()
                          startResize(
                            'row',
                            ri,
                            e.clientY,
                            doc.heights?.[ri] || DEFAULT_ROW_HEIGHT,
                          )
                        }}
                        className="absolute bottom-0 left-0 h-1 w-full cursor-row-resize hover:bg-shell-accent"
                      />
                    </div>
                  </th>
                  {row.map((cell, ci) => {
                    if (hiddenCols.has(ci)) {
                      return (
                        <td key={ci} className="w-3 border-b border-r border-shell-border" />
                      )
                    }
                    const merge = mergeAt(doc, ri, ci)
                    if (merge && (merge.row !== ri || merge.col !== ci)) return null

                    const inSelection = selection.some(
                      (s) => s.row === ri && s.col === ci,
                    )
                    const isEditing = editing?.row === ri && editing?.col === ci
                    const style = getCellStyle(doc, ri, ci)
                    const computed = values[ri]?.[ci]
                    const display = isEditing
                      ? cell
                      : isFormula(cell)
                        ? formatCellValue(computed ?? '')
                        : cell

                    return (
                      <td
                        key={ci}
                        rowSpan={merge?.rowSpan}
                        colSpan={merge?.colSpan}
                        className={`border-b border-r border-shell-border p-0 align-middle ${
                          inSelection ? 'bg-shell-active' : ''
                        }`}
                        style={{
                          backgroundColor: style?.fill ? `#${style.fill}` : undefined,
                        }}
                      >
                        <input
                          data-testid={`table-doc-cell-${ri}-${ci}`}
                          data-formula={isFormula(cell) ? 'true' : undefined}
                          value={display}
                          onChange={(e) =>
                            commit(setTableCell(doc, ri, ci, e.target.value))
                          }
                          onFocus={() => {
                            setCursor({ row: ri, col: ci })
                            setEditing({ row: ri, col: ci })
                          }}
                          onBlur={() => setEditing(null)}
                          onMouseDown={(e) => {
                            // Shift-click extends the selection from the anchor;
                            // a plain click starts a new one.
                            if (e.shiftKey) {
                              e.preventDefault()
                              setCursor({ row: ri, col: ci })
                            } else {
                              setAnchor({ row: ri, col: ci })
                            }
                          }}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' && ri === rowCount - 1) {
                              e.preventDefault()
                              commit(addTableRow(doc))
                            }
                            if (e.key === 'Escape') e.currentTarget.blur()
                          }}
                          aria-label={`${doc.columns[ci]?.name || ''} ${ri + 1}`}
                          style={{
                            fontWeight: style?.bold ? 700 : undefined,
                            fontStyle: style?.italic ? 'italic' : undefined,
                            color: style?.color ? `#${style.color}` : undefined,
                            textAlign: style?.align,
                            minWidth: doc.widths?.[ci] || DEFAULT_COL_WIDTH,
                            height: doc.heights?.[ri] || undefined,
                          }}
                          className="w-full bg-transparent px-1.5 py-1 text-[11px] text-shell-text outline-none focus:bg-shell-bg/70"
                        />
                      </td>
                    )
                  })}
                  <td className="border-b border-shell-border" />
                </tr>
              )
            })}
            <tr>
              <th scope="row" className="sticky left-0 z-10 bg-shell-panel px-1 py-1">
                <button
                  type="button"
                  data-testid="table-doc-add-row-inline"
                  onClick={() => commit(addTableRow(doc))}
                  title={t('tableDoc.addRow')}
                  aria-label={t('tableDoc.addRow')}
                  className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted transition hover:bg-shell-hover hover:text-shell-text"
                >
                  <Plus size={12} strokeWidth={2} aria-hidden />
                </button>
              </th>
              <td colSpan={columnCount + 1} className="px-2 text-[10px] text-shell-muted">
                {t('tableDoc.formulaHint')}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div
        data-testid="table-doc-sheet-tabs"
        role="tablist"
        aria-label={t('tableDoc.sheets')}
        className="flex shrink-0 items-center gap-0.5 overflow-x-auto border-t border-shell-border bg-shell-panel px-1 py-1"
      >
        {workbook.sheets.map((s) => {
          const isActive = s.id === sheet.id
          return (
            <div
              key={s.id}
              className={`group inline-flex shrink-0 items-center gap-0.5 rounded-md border px-1 transition ${
                isActive
                  ? 'border-shell-accent bg-shell-active'
                  : 'border-transparent hover:bg-shell-hover'
              }`}
            >
              <input
                data-testid={`table-doc-sheet-${s.id}`}
                role="tab"
                aria-selected={isActive}
                value={s.name}
                size={Math.max(6, Math.min(20, s.name.length || 6))}
                onFocus={() => {
                  if (!isActive) commitWorkbook(setActiveSheet(workbook, s.id))
                }}
                onChange={(e) =>
                  commitWorkbook(renameSheet(workbook, s.id, e.target.value))
                }
                aria-label={t('tableDoc.sheetName', { name: s.name })}
                className={`h-6 min-w-0 bg-transparent px-1 text-[11px] outline-none ${
                  isActive ? 'font-medium text-shell-accent' : 'text-shell-muted'
                }`}
              />
              <button
                type="button"
                data-testid={`table-doc-remove-sheet-${s.id}`}
                onClick={() => commitWorkbook(removeSheet(workbook, s.id))}
                disabled={workbook.sheets.length <= 1}
                title={t('tableDoc.removeSheet')}
                aria-label={t('tableDoc.removeSheet')}
                className="shrink-0 rounded p-0.5 text-shell-muted opacity-0 transition group-hover:opacity-100 hover:text-red-400 disabled:hidden"
              >
                <Trash2 size={9} strokeWidth={2} aria-hidden />
              </button>
            </div>
          )
        })}
        <button
          type="button"
          data-testid="table-doc-add-sheet"
          onClick={() => commitWorkbook(addSheet(workbook))}
          disabled={workbook.sheets.length >= TABLE_MAX_SHEETS}
          title={t('tableDoc.addSheet')}
          aria-label={t('tableDoc.addSheet')}
          className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-shell-muted transition hover:bg-shell-hover hover:text-shell-text disabled:opacity-30"
        >
          <Plus size={12} strokeWidth={2} aria-hidden />
        </button>
      </div>

      <DocAgentPanel
        kind="table"
        contextId={contextId}
        canvasKey={canvasKey}
        canvasEntrySessionId={canvasEntrySessionId}
        currentContent={content}
        onApply={onApplyFromAgent}
        placeholder={t('tableDoc.agentPlaceholder')}
      />
    </div>
  )
}
