import { describe, expect, it, vi, beforeEach } from 'vitest'
import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import type { Account } from '@/types'
import CreateAccountModal from '../CreateAccountModal.vue'
import EditAccountModal from '../EditAccountModal.vue'

const {
  createAccountMock,
  updateAccountMock,
  checkMixedChannelRiskMock,
  getWebSearchEmulationConfigMock,
  listTLSFingerprintProfilesMock
} = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  updateAccountMock: vi.fn(),
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
      update: updateAccountMock,
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

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: '<div data-testid="model-whitelist-stub">{{ Array.isArray(modelValue) ? modelValue.join(\',\') : \'\' }}</div>'
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
        ModelWhitelistSelector: ModelWhitelistSelectorStub,
        QuotaLimitCard: true,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub
      }
    }
  })

function buildWindsurfAccount(): Account {
  return {
    id: 9,
    name: 'windsurf-account',
    notes: '',
    platform: 'windsurf',
    type: 'apikey',
    credentials: {
      token: 'ws-token',
      model_mapping: {
        'claude-3.5-sonnet': 'gpt-4.1'
      }
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  } as Account
}

const mountEditModal = (account = buildWindsurfAccount()) =>
  mount(EditAccountModal, {
    props: {
      show: false,
      account,
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
        ModelWhitelistSelector: ModelWhitelistSelectorStub,
        QuotaLimitCard: true
      }
    }
  })

describe('Windsurf account modals', () => {
  beforeEach(() => {
    createAccountMock.mockReset()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    getWebSearchEmulationConfigMock.mockReset()
    listTLSFingerprintProfilesMock.mockReset()

    createAccountMock.mockResolvedValue({})
    updateAccountMock.mockResolvedValue(buildWindsurfAccount())
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    getWebSearchEmulationConfigMock.mockResolvedValue({ enabled: false, providers: [] })
    listTLSFingerprintProfilesMock.mockResolvedValue([])
  })

  it('creates windsurf accounts with token credentials and no oauth step', async () => {
    const wrapper = mountCreateModal()
    await flushPromises()

    expect(wrapper.text()).toContain('Windsurf')

    const windsurfButton = wrapper.findAll('button').find(node => node.text().includes('Windsurf'))
    expect(windsurfButton).toBeTruthy()

    await windsurfButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('admin.accounts.oauth.authMethod')

    ;(wrapper.vm as any).form.name = 'windsurf-create'
    ;(wrapper.vm as any).apiKeyValue = 'ws-token-create'

    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]).toEqual(
      expect.objectContaining({
        name: 'windsurf-create',
        platform: 'windsurf',
        type: 'apikey',
        credentials: expect.objectContaining({
          token: 'ws-token-create'
        })
      })
    )
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials?.api_key).toBeUndefined()
  })

  it('updates windsurf accounts using credentials.token instead of api_key', async () => {
    const wrapper = mountEditModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    ;(wrapper.vm as any).editApiKey = 'ws-token-updated'

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials).toEqual(
      expect.objectContaining({
        token: 'ws-token-updated',
        model_mapping: {
          'claude-3.5-sonnet': 'gpt-4.1'
        }
      })
    )
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.api_key).toBeUndefined()
  })
})
