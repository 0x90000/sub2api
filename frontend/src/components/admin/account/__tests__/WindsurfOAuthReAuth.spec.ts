import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import type { Account } from '@/types'
import ReAuthAccountModal from '../ReAuthAccountModal.vue'

const {
  updateAccountMock,
  clearErrorMock,
  windsurfValidateRefreshTokenMock,
  windsurfBuildCredentialsMock,
  windsurfBuildExtraInfoMock
} = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  clearErrorMock: vi.fn(),
  windsurfValidateRefreshTokenMock: vi.fn(),
  windsurfBuildCredentialsMock: vi.fn(),
  windsurfBuildExtraInfoMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      clearError: clearErrorMock,
      exchangeCode: vi.fn()
    }
  }
}))

const createOAuthStub = () => ({
  authUrl: ref(''),
  sessionId: ref('session-id'),
  loading: ref(false),
  error: ref(''),
  oauthState: ref(''),
  state: ref(''),
  resetState: vi.fn(),
  generateAuthUrl: vi.fn(),
  exchangeAuthCode: vi.fn(),
  validateRefreshToken: vi.fn(),
  buildCredentials: vi.fn((tokenInfo?: Record<string, unknown>) => ({ ...(tokenInfo || {}) })),
  buildExtraInfo: vi.fn(() => ({})),
  parseSessionKeys: vi.fn(() => [])
})

vi.mock('@/composables/useAccountOAuth', () => ({
  useAccountOAuth: () => createOAuthStub()
}))

vi.mock('@/composables/useOpenAIOAuth', () => ({
  useOpenAIOAuth: () => createOAuthStub()
}))

vi.mock('@/composables/useGeminiOAuth', () => ({
  useGeminiOAuth: () => createOAuthStub()
}))

vi.mock('@/composables/useAntigravityOAuth', () => ({
  useAntigravityOAuth: () => createOAuthStub()
}))

vi.mock('@/composables/useWindsurfOAuth', () => ({
  useWindsurfOAuth: () => ({
    authUrl: ref(''),
    sessionId: ref(''),
    loading: ref(false),
    error: ref(''),
    resetState: vi.fn(),
    validateRefreshToken: windsurfValidateRefreshTokenMock,
    buildCredentials: windsurfBuildCredentialsMock,
    buildExtraInfo: windsurfBuildExtraInfoMock
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

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  emits: ['validate-refresh-token'],
  setup(_props, { emit, expose }) {
    expose({
      inputMethod: 'refresh_token',
      authCode: '',
      oauthState: '',
      projectId: '',
      sessionKey: '',
      refreshToken: 'ws-refresh-token-new',
      reset: vi.fn()
    })
    return {
      emitRefreshToken: () => emit('validate-refresh-token', 'ws-refresh-token-new')
    }
  },
  template: '<button data-testid="windsurf-reauth-submit" @click="emitRefreshToken">reauth refresh token</button>'
})

function buildWindsurfOAuthAccount(): Account {
  return {
    id: 22,
    name: 'windsurf-oauth-account',
    notes: '',
    platform: 'windsurf',
    type: 'oauth',
    credentials: {
      token: 'ws-runtime-token-old',
      access_token: 'firebase-id-token-old',
      refresh_token: 'firebase-refresh-token-old'
    },
    extra: {
      oauth_display_name: 'Old Windsurf'
    },
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'error',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  } as Account
}

describe('Windsurf OAuth reauth flow', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    clearErrorMock.mockReset()
    windsurfValidateRefreshTokenMock.mockReset()
    windsurfBuildCredentialsMock.mockReset()
    windsurfBuildExtraInfoMock.mockReset()

    windsurfValidateRefreshTokenMock.mockResolvedValue({
      token: 'ws-runtime-token-new',
      access_token: 'firebase-id-token-new',
      refresh_token: 'firebase-refresh-token-new',
      id_token: 'firebase-id-token-new',
      expires_in: 3600,
      expires_at: 1760003600,
      display_name: 'New Windsurf'
    })
    windsurfBuildCredentialsMock.mockReturnValue({
      token: 'ws-runtime-token-new',
      access_token: 'firebase-id-token-new',
      refresh_token: 'firebase-refresh-token-new',
      id_token: 'firebase-id-token-new',
      expires_in: 3600,
      expires_at: 1760003600
    })
    windsurfBuildExtraInfoMock.mockReturnValue({
      oauth_display_name: 'New Windsurf',
      oauth_last_refresh_at: '2026-04-23T00:00:00Z',
      oauth_last_refresh_error: ''
    })
    updateAccountMock.mockResolvedValue(buildWindsurfOAuthAccount())
    clearErrorMock.mockResolvedValue(buildWindsurfOAuthAccount())
  })

  it('reauthorizes windsurf oauth accounts from a refresh token', async () => {
    const wrapper = mount(ReAuthAccountModal, {
      props: {
        show: true,
        account: buildWindsurfOAuthAccount()
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Icon: true,
          OAuthAuthorizationFlow: OAuthAuthorizationFlowStub
        }
      }
    })

    await flushPromises()

    await wrapper.get('[data-testid="windsurf-reauth-submit"]').trigger('click')
    await flushPromises()

    expect(windsurfValidateRefreshTokenMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock).toHaveBeenCalledWith(
      22,
      expect.objectContaining({
        type: 'oauth',
        credentials: expect.objectContaining({
          token: 'ws-runtime-token-new',
          access_token: 'firebase-id-token-new',
          refresh_token: 'firebase-refresh-token-new'
        }),
        extra: expect.objectContaining({
          oauth_display_name: 'New Windsurf',
          oauth_last_refresh_error: ''
        })
      })
    )
    expect(clearErrorMock).toHaveBeenCalledWith(22)
  })
})
