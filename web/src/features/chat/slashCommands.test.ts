import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  applySlashInsert,
  filterSlashCommands,
  matchTrailingSlash,
  SLASH_COMMANDS,
} from './slashCommands'

describe('slash commands catalog', () => {
  it('includes tools and config shortcuts', () => {
    assert.ok(SLASH_COMMANDS.some((c) => c.name === 'todo_list'))
    assert.ok(SLASH_COMMANDS.some((c) => c.name === 'plan' && c.action === 'plan'))
    assert.ok(SLASH_COMMANDS.some((c) => c.name === 'read_file'))
  })

  it('filters by prefix and description', () => {
    const hits = filterSlashCommands('todo')
    assert.ok(hits.some((c) => c.name.startsWith('todo')))
    assert.ok(hits.every((c) => /todo|plan|goal/i.test(`${c.name}${c.description}`)))
  })

  it('matches trailing slash query', () => {
    assert.deepEqual(matchTrailingSlash('/todo'), { query: 'todo', start: 0 })
    assert.deepEqual(matchTrailingSlash('hi /git'), { query: 'git', start: 3 })
    assert.equal(matchTrailingSlash('no slash'), null)
    assert.equal(matchTrailingSlash('email@x.com'), null)
  })

  it('replaces trailing slash token on insert', () => {
    assert.equal(
      applySlashInsert('/todo_list', 'Use tool `todo_list`.'),
      'Use tool `todo_list`.',
    )
    assert.equal(
      applySlashInsert('please /tod', 'Use tool `todo_list`.'),
      'please Use tool `todo_list`.',
    )
  })
})
