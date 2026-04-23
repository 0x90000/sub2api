import { describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/api/admin', () => ({
  adminAPI: {
    windsurf: {
      refreshToken: vi.fn()
    }
  }
}))

import { useWindsurfOAuth } from '@/composables/useWindsurfOAuth'

describe('useWindsurfOAuth helpers', () => {
  it('buildCredentials keeps runtime token and firebase tokens', () => {
    const oauth = useWindsurfOAuth()

    const credentials = oauth.buildCredentials({
      token: 'ws-runtime-token',
      access_token: 'firebase-id-token',
      refresh_token: 'firebase-refresh-token',
      id_token: 'firebase-id-token',
      expires_in: 3600,
      expires_at: 1760000000,
      api_server_url: 'https://api.codeium.com'
    })

    expect(credentials).toEqual({
      token: 'ws-runtime-token',
      access_token: 'firebase-id-token',
      refresh_token: 'firebase-refresh-token',
      id_token: 'firebase-id-token',
      expires_in: 3600,
      expires_at: 1760000000,
      api_server_url: 'https://api.codeium.com'
    })
  })

  it('buildExtraInfo writes oauth snapshot metadata', () => {
    const oauth = useWindsurfOAuth()

    const extra = oauth.buildExtraInfo({
      display_name: 'Windsurf OAuth',
      email: 'windsurf@example.com',
      avatar_url: 'https://example.com/avatar.png'
    })

    expect(extra).toEqual(
      expect.objectContaining({
        oauth_display_name: 'Windsurf OAuth',
        oauth_email: 'windsurf@example.com',
        oauth_avatar_url: 'https://example.com/avatar.png',
        oauth_last_refresh_error: ''
      })
    )
    expect(typeof extra.oauth_last_refresh_at).toBe('string')
  })
})
