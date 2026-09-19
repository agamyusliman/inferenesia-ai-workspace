import { useEffect, useState, type ReactNode } from 'react'
import type { Workspace } from '../../lib/api'
import { ChatPanel } from '../chat/ChatPanel'
import { EditorTabs } from '../editor/EditorTabs'
import { defaultViewMode, type EditorViewMode } from '../editor/editorMode'
import { UndoToolbar } from '../undo/UndoToolbar'
import { WorkspaceChatPlaceholder } from '../workspaces/WorkspaceChatPlaceholder'
import {
  SplitEditorArea,
  type EditorColumn,
} from './SplitEditorArea'

import { VerticalSplitter } from './VerticalSplitter'
import { HorizontalSplitter } from './HorizontalSplitter'
import type { ChatDock } from './useLayoutConfig'

type Props = {
  selectedFile?: string
  chatContext?: string
  undoRefreshKey?: number
  chatWidth: number
  chatHeight: number
  chatDock: ChatDock
  chatVisible?: boolean
  onChatWidthChange: (w: number) => void
  onChatHeightChange: (h: number) => void
  onDiffPrevChange?: () => void
  onDiffNextChange?: () => void
  canDiffPrevChange?: boolean
  canDiffNextChange?: boolean
  explorerDock: 'left' | 'right'
  tabs: EditorColumn[]
  activeTabId: string | null
  onActivateTab: (id: string) => void
  onCloseTab: (id: string) => void
  primary: EditorColumn | null
  secondary: EditorColumn | null
  onPrimaryChange: (content: string) => void
  onSecondaryChange: (content: string) => void
  onPrimaryViewState?: (state: unknown) => void
  onSecondaryViewState?: (state: unknown) => void
  onCloseSecondary: () => void
  onFocusColumn?: (which: 'primary' | 'secondary') => void
  onSaveActive?: () => void
  onFileChanged?: (path?: string) => void
  onAfterUndo?: () => void
  workspaceId?: string
  onPlanModeChange?: (active: boolean) => void
  /** PlanProposed / TodoUpdated → parent switches to Todos panel (VAL-PLAN-003/005). */
  onPlanProposed?: () => void
  onTodoUpdated?: () => void
  onPlanDecided?: (kind: 'approve' | 'reject') => void
  /** MissionStatus stream → refresh mission panel (VAL-MISSION-006). */
  onMissionStatus?: () => void
  /** TaskSpawned/TaskDone stream → refresh task panel (VAL-ORCH-010). */
  onTaskEvent?: () => void
  /** BrowserStatus stream → refresh browser panel (VAL-BRW-008). */
  onBrowserStatus?: () => void
  onOpenWebPreview?: (seed: import('../web-preview/webPreview').WebPreviewSeed) => void
  htmlSyncNonce?: number
  needsSession?: boolean
  onEnsureSession?: (firstUserMessage?: string) => Promise<string | undefined>
  onCreateNewSession?: () => Promise<string | undefined>
  onMaybeTitleSession?: (workspaceId: string, firstUserMessage: string) => void
  needsFolderWorkspace?: boolean
  workspaces?: Workspace[]
  workspaceBusy?: boolean
  onOpenWorkspacePath?: (path: string) => void
  onSwitchWorkspace?: (id: string) => void
  todoRefreshKey?: number
  missionRefreshKey?: number
  taskRefreshKey?: number
  browserRefreshKey?: number
}

export function MainPane({
  selectedFile,
  chatContext,
  undoRefreshKey,
  chatWidth,
  chatHeight,
  chatDock,
  chatVisible = true,
  onChatWidthChange,
  onChatHeightChange,
  onDiffPrevChange,
  onDiffNextChange,
  canDiffPrevChange,
  canDiffNextChange,
  explorerDock,
  tabs,
  activeTabId,
  onActivateTab,
  onCloseTab,
  primary,
  secondary,
  onPrimaryChange,
  onSecondaryChange,
  onPrimaryViewState,
  onSecondaryViewState,
  onCloseSecondary,
  onFocusColumn,
  onSaveActive,
  onFileChanged,
  onAfterUndo,
  workspaceId,
  onPlanModeChange,
  onPlanProposed,
  onTodoUpdated,
  onPlanDecided,
  onMissionStatus,
  onTaskEvent,
  onBrowserStatus,
  onOpenWebPreview,
  htmlSyncNonce = 0,
  needsSession = false,
  onEnsureSession,
  onCreateNewSession,
  onMaybeTitleSession,
  needsFolderWorkspace = false,
  workspaces = [],
  workspaceBusy = false,
  onOpenWorkspacePath,
  onSwitchWorkspace,
  todoRefreshKey = 0,
  missionRefreshKey = 0,
  taskRefreshKey = 0,
  browserRefreshKey = 0,
}: Props) {
  /** Hide editor column entirely when nothing is open (no blank placeholder). */
  const showEditor = tabs.length > 0 || !!secondary
  const [primaryMode, setPrimaryMode] = useState<EditorViewMode>('edit')

  useEffect(() => {
    if (!primary) {
      setPrimaryMode('edit')
      return
    }
    setPrimaryMode(primary.preferredMode || defaultViewMode(primary.path))
  }, [primary?.path, primary?.preferredMode, activeTabId])

  const showChat = chatVisible
  const chatPanel = showChat ? (
    <div
      data-testid="chat-column"
      data-chat-dock={chatDock}
      data-chat-width={chatDock === 'bottom' ? undefined : chatWidth}
      data-chat-height={chatDock === 'bottom' ? chatHeight : undefined}
      data-editor-open={showEditor ? 'true' : 'false'}
      data-needs-folder={needsFolderWorkspace ? 'true' : 'false'}
      style={
        showEditor
          ? chatDock === 'bottom'
            ? {
                height: chatHeight,
                flex: 'none',
                width: '100%',
                minHeight: 100,
                maxHeight: '70%',
              }
            : {
                width: chatWidth,
                maxWidth: '50vw',
                flex: '0 0 auto',
                minWidth: 0,
              }
          : chatDock === 'bottom'
            ? { flex: '1 1 auto', width: '100%', minHeight: 0 }
            : { flex: '1 1 auto', minWidth: 0, width: 'auto' }
      }
      className={`flex min-h-0 min-w-0 flex-col overflow-hidden ${
        showEditor
          ? chatDock === 'bottom'
            ? 'shrink-0 border-t border-shell-border'
            : chatDock === 'left'
              ? 'shrink-0 border-r border-shell-border'
              : 'shrink-0 border-l border-shell-border'
          : 'flex-1'
      }`}
    >
      {needsFolderWorkspace ? (
        <WorkspaceChatPlaceholder
          workspaces={workspaces}
          busy={workspaceBusy}
          onOpenPath={(path) => onOpenWorkspacePath?.(path)}
          onSwitch={(id) => onSwitchWorkspace?.(id)}
        />
      ) : (
        <ChatPanel
          workspaceId={workspaceId}
          chatContext={chatContext}
          needsSession={needsSession}
          onEnsureSession={onEnsureSession}
          onCreateNewSession={onCreateNewSession}
          onMaybeTitleSession={onMaybeTitleSession}
          onFileChanged={onFileChanged}
          onUndoStackChanged={() => onFileChanged?.(undefined)}
          onPlanModeChange={onPlanModeChange}
          onPlanProposed={() => onPlanProposed?.()}
          onTodoUpdated={onTodoUpdated}
          onPlanDecided={(kind) => onPlanDecided?.(kind)}
          onMissionStatus={onMissionStatus}
          onTaskEvent={onTaskEvent}
          onBrowserStatus={onBrowserStatus}
          onOpenWebPreview={onOpenWebPreview}
          htmlSyncNonce={htmlSyncNonce}
          todoRefreshKey={todoRefreshKey}
          missionRefreshKey={missionRefreshKey}
          taskRefreshKey={taskRefreshKey}
          browserRefreshKey={browserRefreshKey}
        />
      )}
    </div>
  ) : null

  const editor = showEditor ? (
    <section
      data-testid="editor-section"
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
    >
      {tabs.length > 0 && (
        <EditorTabs
          tabs={tabs}
          activeId={activeTabId}
          onActivate={onActivateTab}
          onClose={onCloseTab}
          onSave={onSaveActive}
          viewMode={primaryMode}
          onViewModeChange={setPrimaryMode}
          onPrevChange={onDiffPrevChange}
          onNextChange={onDiffNextChange}
          canPrevChange={canDiffPrevChange}
          canNextChange={canDiffNextChange}
        />
      )}
      <div className="min-h-0 min-w-0 flex-1 overflow-hidden">
        <SplitEditorArea
          primary={primary}
          secondary={secondary}
          onPrimaryChange={onPrimaryChange}
          onSecondaryChange={onSecondaryChange}
          onPrimaryViewState={onPrimaryViewState}
          onSecondaryViewState={onSecondaryViewState}
          onCloseSecondary={onCloseSecondary}
          onFocusColumn={onFocusColumn}
          primaryMode={primaryMode}
          onPrimaryModeChange={setPrimaryMode}
        />
      </div>
    </section>
  ) : null

  let body: ReactNode
  if (!showEditor) {
    body = (
      <div
        data-testid="main-body"
        data-layout={showChat ? 'chat-only' : 'empty'}
        className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
      >
        {chatPanel || (
          <div className="flex flex-1 items-center justify-center text-[11px] text-shell-muted">
            Chat hidden — use the Chat button to show it.
          </div>
        )}
      </div>
    )
  } else if (!showChat) {
    body = (
      <div
        data-testid="main-body"
        data-layout="editor-only"
        className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
      >
        {editor}
      </div>
    )
  } else if (chatDock === 'bottom') {
    body = (
      <div
        data-testid="main-body"
        data-layout="editor-top-chat-bottom"
        className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
      >
        {editor}
        <HorizontalSplitter
          testId="splitter-chat"
          value={chatHeight}
          onChange={onChatHeightChange}
          growSide="bottom"
          minOpposite={120}
          aria-label="Resize chat panel height"
        />
        {chatPanel}
      </div>
    )
  } else if (chatDock === 'left') {
    body = (
      <div
        data-testid="main-body"
        data-layout="chat-left-editor-right"
        className="flex min-h-0 min-w-0 flex-1 flex-row overflow-hidden"
      >
        {chatPanel}
        <VerticalSplitter
          testId="splitter-chat"
          value={chatWidth}
          onChange={onChatWidthChange}
          growSide="left"
          minOpposite={160}
          aria-label="Resize chat panel"
        />
        {editor}
      </div>
    )
  } else {
    body = (
      <div
        data-testid="main-body"
        data-layout="editor-left-chat-right"
        className="flex min-h-0 min-w-0 flex-1 flex-row overflow-hidden"
      >
        {editor}
        <VerticalSplitter
          testId="splitter-chat"
          value={chatWidth}
          onChange={onChatWidthChange}
          growSide="right"
          minOpposite={160}
          aria-label="Resize chat panel"
        />
        {chatPanel}
      </div>
    )
  }

  return (
    <main
      data-testid="main-pane"
      data-explorer-dock={explorerDock}
      data-chat-dock={chatDock}
      data-editor-open={showEditor ? 'true' : 'false'}
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      {showEditor && (
        <UndoToolbar
          selectedFile={selectedFile}
          refreshKey={undoRefreshKey}
          onAfterAction={onAfterUndo}
        />
      )}

      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">{body}</div>
      </div>
    </main>
  )
}
