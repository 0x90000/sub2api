import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountActionMenu from '../AccountActionMenu.vue'
import AccountTestModal from '../AccountTestModal.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import type { Account } from '@/types'

const { getAvailableModels, refreshCredentials, copyToClipboard } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  refreshCredentials: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels,
      refreshCredentials
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.accounts.windsurf.allowedModelsCount') {
          return `allowed-models-${params?.count ?? 0}`
        }
        return key
      }
    })
  }
})

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

function makeWindsurfAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 42,
    name: 'windsurf-account',
    platform: 'windsurf',
    type: 'apikey',
    credentials: {
      token: 'ws-token'
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-04-22T00:00:00Z',
    updated_at: '2026-04-22T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides
  }
}

describe('Windsurf account ops', () => {
  beforeEach(() => {
    getAvailableModels.mockReset()
    refreshCredentials.mockReset()
    copyToClipboard.mockReset()
    getAvailableModels.mockResolvedValue([])
    refreshCredentials.mockResolvedValue(makeWindsurfAccount())
    Object.defineProperty(window, 'matchMedia', {
      value: vi.fn().mockImplementation(() => ({
        matches: true,
        media: '(min-width: 768px)',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn()
      })),
      configurable: true
    })
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gpt-4.1"}\n',
        'data: {"type":"content","text":"hello from windsurf"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any
  })

  it('shows refresh credits and models action for windsurf accounts', () => {
    const wrapper = mount(AccountActionMenu, {
      props: {
        show: true,
        account: makeWindsurfAccount(),
        position: { top: 12, left: 16 }
      },
      global: {
        stubs: {
          Teleport: true,
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('admin.accounts.testConnection')
    expect(wrapper.text()).toContain('admin.accounts.windsurf.refreshCatalog')
    expect(wrapper.text()).not.toContain('admin.accounts.refreshToken')
  })

  it('renders windsurf plan tier, credits and allowed model count in usage cell', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeWindsurfAccount({
          extra: {
            plan_tier: 'pro',
            credit_balance: 12.5,
            allowed_models: ['gpt-4.1', 'claude-3.7-sonnet']
          }
        })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.windsurf.plan.pro')
    expect(wrapper.text()).toContain('12.50')
    expect(wrapper.text()).toContain('allowed-models-2')
  })

  it('refreshes windsurf catalog and tests with a real model when catalog is initially empty', async () => {
    getAvailableModels
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([
        {
          id: 'gpt-4.1',
          type: 'model',
          display_name: 'gpt-4.1',
          created_at: ''
        }
      ])

    const wrapper = mount(AccountTestModal, {
      props: {
        show: false,
        account: makeWindsurfAccount()
      } as any,
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Select: { template: '<div class="select-stub"></div>' },
          TextArea: {
            props: ['modelValue'],
            emits: ['update:modelValue'],
            template: '<textarea class="textarea-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
          },
          Icon: true
        }
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    await flushPromises()

    const startButton = wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()
    expect(startButton!.attributes('disabled')).toBeUndefined()
    expect(refreshCredentials).toHaveBeenCalledWith(42)

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'gpt-4.1',
      prompt: ''
    })
  })
})
