import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import CreateAccountModal from '../CreateAccountModal.vue'

const {
  createAccountMock,
  checkMixedChannelRiskMock,
  getWebSearchEmulationConfigMock,
  listTLSFingerprintProfilesMock,
  windsurfValidateRefreshTokenMock,
  windsurfBuildCredentialsMock,
  windsurfBuildExtraInfoMock
} = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  getWebSearchEmulationConfigMock: vi.fn(),
  listTLSFingerprintProfilesMock: vi.fn(),
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

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: true
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock,
      exchangeCode: vi.fn()
    },
    settings: {
      getWebSearchEmulationConfig: getWebSearchEmulationConfigMock
    },
    tlsFingerprintProfiles: {
      list: listTLSFingerprintProfilesMock
    }
  }
}))

vi.mock('@/composables/useQuotaNotifyState', () => ({
  useQuotaNotifyState: () => ({
    globalEnabled: ref(false),
    state: ref({
      daily: { enabled: false, threshold: null, thresholdType: 'percent' },
      weekly: { enabled: false, threshold: null, thresholdType: 'percent' },
      total: { enabled: false, threshold: null, thresholdType: 'percent' }
    }).value,
    loadGlobalState: vi.fn(),
    writeToExtra: vi.fn(),
    loadFromExtra: vi.fn(),
    reset: vi.fn()
  })
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

vi.mock('@/components/account/credentialsBuilder', () => ({
  applyInterceptWarmup: vi.fn()
}))

vi.mock('@/utils/format', () => ({
  formatDateTimeLocalInput: vi.fn(() => ''),
  parseDateTimeLocalInput: vi.fn(() => null)
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
      refreshToken: 'ws-refresh-token',
      reset: vi.fn()
    })
    return {
      emitRefreshToken: () => emit('validate-refresh-token', 'ws-refresh-token')
    }
  },
  template: '<button data-testid="windsurf-rt-submit" @click="emitRefreshToken">submit refresh token</button>'
})

const mountCreateModal = () =>
  mount(CreateAccountModal, {
    props: {
      show: true,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Select: true,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub
      }
    }
  })

describe('Windsurf OAuth create flow', () => {
  beforeEach(() => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    getWebSearchEmulationConfigMock.mockReset()
    listTLSFingerprintProfilesMock.mockReset()
    windsurfValidateRefreshTokenMock.mockReset()
    windsurfBuildCredentialsMock.mockReset()
    windsurfBuildExtraInfoMock.mockReset()

    createAccountMock.mockResolvedValue({})
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTLSFingerprintProfilesMock.mockResolvedValue([])
    windsurfValidateRefreshTokenMock.mockResolvedValue({
      token: 'ws-runtime-token',
      access_token: 'firebase-id-token',
      refresh_token: 'firebase-refresh-token',
      id_token: 'firebase-id-token',
      expires_in: 3600,
      expires_at: 1760000000,
      display_name: 'Windsurf OAuth'
    })
    windsurfBuildCredentialsMock.mockReturnValue({
      token: 'ws-runtime-token',
      access_token: 'firebase-id-token',
      refresh_token: 'firebase-refresh-token',
      id_token: 'firebase-id-token',
      expires_in: 3600,
      expires_at: 1760000000
    })
    windsurfBuildExtraInfoMock.mockReturnValue({
      oauth_display_name: 'Windsurf OAuth',
      oauth_last_refresh_at: '2026-04-23T00:00:00Z',
      oauth_last_refresh_error: ''
    })
  })

  it('creates windsurf oauth accounts from refresh token validation', async () => {
    const wrapper = mountCreateModal()
    await flushPromises()

    const windsurfButton = wrapper.findAll('button').find(node => node.text().includes('Windsurf'))
    expect(windsurfButton).toBeTruthy()

    await windsurfButton!.trigger('click')
    await flushPromises()

    ;(wrapper.vm as any).accountCategory = 'oauth-based'
    ;(wrapper.vm as any).form.name = 'windsurf-oauth'
    await flushPromises()

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    await wrapper.get('[data-testid="windsurf-rt-submit"]').trigger('click')
    await flushPromises()

    expect(windsurfValidateRefreshTokenMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'windsurf-oauth',
        platform: 'windsurf',
        type: 'oauth',
        credentials: expect.objectContaining({
          token: 'ws-runtime-token',
          access_token: 'firebase-id-token',
          refresh_token: 'firebase-refresh-token'
        }),
        extra: expect.objectContaining({
          oauth_display_name: 'Windsurf OAuth',
          oauth_last_refresh_error: ''
        })
      })
    )
  })
})
