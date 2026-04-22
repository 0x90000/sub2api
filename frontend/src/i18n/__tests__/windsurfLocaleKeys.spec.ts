import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('windsurf locale keys', () => {
  it('contains zh windsurf billing boundary copy', () => {
    expect(zh.admin.accounts.platforms.windsurf).toBe('Windsurf')
    expect(zh.admin.accounts.aiCreditsBalance).toBe('AI Credits')
    expect(zh.admin.accounts.windsurf.billingBoundaryHint).toContain('上游')
    expect(zh.admin.usage.billingBoundaryHint).toContain('本站')
  })

  it('contains en windsurf billing boundary copy', () => {
    expect(en.admin.accounts.platforms.windsurf).toBe('Windsurf')
    expect(en.admin.accounts.aiCreditsBalance).toBe('AI Credits')
    expect(en.admin.accounts.windsurf.billingBoundaryHint).toContain('upstream')
    expect(en.admin.usage.billingBoundaryHint).toContain('local sub2api billing')
  })
})
