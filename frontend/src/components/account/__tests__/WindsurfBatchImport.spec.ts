import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'

import CreateAccountModal from '../CreateAccountModal.vue'

const {
  createAccountMock,
  batchCreateWindsurfTokensMock,
  checkMixedChannelRiskMock,
  getWebSearchEmulationConfigMock,
  listTLSFingerprintProfilesMock
} = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  batchCreateWindsurfTokensMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  getWebSearchEmulationConfigMock: vi.fn(),
  listTLSFingerprintProfilesMock: vi.fn()
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
      batchCreateWindsurfTokens: batchCreateWindsurfTokensMock,
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
    writeToExtra: vi.fn()
  })
}))

const createOAuthStub = () => ({
  authUrl: ref(''),
  sessionId: ref('session-id'),
  loading: ref(false),
  error: ref(''),
  oauthState: ref(''),
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
  template: '<div data-testid="oauth-flow-stub" />'
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

describe('Windsurf batch import', () => {
  beforeEach(() => {
    createAccountMock.mockReset()
    batchCreateWindsurfTokensMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    getWebSearchEmulationConfigMock.mockReset()
    listTLSFingerprintProfilesMock.mockReset()

    createAccountMock.mockResolvedValue({})
    batchCreateWindsurfTokensMock.mockResolvedValue({ success: 2, failed: 0, results: [] })
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTLSFingerprintProfilesMock.mockResolvedValue([])
  })

  it('submits multiline windsurf tokens through batch import api', async () => {
    const wrapper = mountCreateModal()
    await flushPromises()

    const windsurfButton = wrapper.findAll('button').find(node => node.text().includes('Windsurf'))
    expect(windsurfButton).toBeTruthy()

    await windsurfButton!.trigger('click')
    await flushPromises()

    const batchToggle = wrapper.find('[data-testid="windsurf-batch-toggle"]')
    expect(batchToggle.exists()).toBe(true)

    await batchToggle.trigger('click')
    await flushPromises()

    ;(wrapper.vm as any).form.name = 'windsurf-batch'
    ;(wrapper.vm as any).apiKeyValue = 'ws-token-1\nws-token-2'

    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(batchCreateWindsurfTokensMock).toHaveBeenCalledTimes(1)
    expect(batchCreateWindsurfTokensMock).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'windsurf-batch',
        platform: 'windsurf',
        tokens: ['ws-token-1', 'ws-token-2']
      })
    )
    expect(createAccountMock).not.toHaveBeenCalled()
  })
})
