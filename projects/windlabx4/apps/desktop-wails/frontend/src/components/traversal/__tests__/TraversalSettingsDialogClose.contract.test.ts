import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('traversal settings dialog close contract', () => {
  it.each(['TraversalSettings.vue', 'dual/DualTraversalSettings.vue'])(
    '%s closes the controlled dialog when UiDialog hides',
    (relativePath) => {
      const source = readFileSync(resolve(__dirname, '..', relativePath), 'utf8')
      const dialogTag = source.slice(source.indexOf('<UiDialog'), source.indexOf('>', source.indexOf('<UiDialog')) + 1)

      expect(dialogTag).toContain('@update:show="emit(\'close\')"')
      expect(dialogTag).not.toContain('@close=')
    },
  )
})
