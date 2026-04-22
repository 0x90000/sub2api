import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminGroup } from '@/types'
import GroupsView from '../GroupsView.vue'

const {
  listGroups,
  getUsageSummary,
  getCapacitySummary,
  getAllGroups,
  listAccounts,
  getAccountByID
} = vi.hoisted(() => ({
  listGroups: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getAllGroups: vi.fn(),
  listAccounts: vi.fn(),
  getAccountByID: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      getUsageSummary,
      getCapacitySummary,
      getAll: getAllGroups,
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
      updateSortOrder: vi.fn()
    },
    accounts: {
      list: listAccounts,
      getById: getAccountByID
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn()
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

const SelectStub = {
  props: ['options'],
  template: `
    <div class="select-stub">
      {{ (options || []).map(option => option.value).join('|') }}
    </div>
  `
}

const createAdminGroup = (): AdminGroup => ({
  id: 7,
  name: 'windsurf-group',
  description: null,
  platform: 'windsurf',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-04-22T00:00:00Z',
  updated_at: '2026-04-22T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: false,
  supported_model_scopes: [],
  sort_order: 1
})

describe('admin GroupsView windsurf platform options', () => {
  beforeEach(() => {
    listGroups.mockReset()
    getUsageSummary.mockReset()
    getCapacitySummary.mockReset()
    getAllGroups.mockReset()
    listAccounts.mockReset()
    getAccountByID.mockReset()

    listGroups.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })
    getUsageSummary.mockResolvedValue([])
    getCapacitySummary.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    listAccounts.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })
    getAccountByID.mockResolvedValue(null)
  })

  it('shows windsurf in group platform filter and modal platform options', async () => {
    const wrapper = mount(GroupsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: { template: '<div />' },
          Pagination: true,
          EmptyState: true,
          Icon: true,
          Select: SelectStub,
          PlatformIcon: true,
          GroupCapacityBadge: true,
          VueDraggable: { template: '<div><slot /></div>' },
          BaseDialog: { template: '<div><slot /></div>' },
          ConfirmDialog: true,
          GroupRateMultipliersModal: true,
          Teleport: true
        }
      }
    })

    await flushPromises()

    ;(wrapper.vm as any).editingGroup = createAdminGroup()
    ;(wrapper.vm as any).showEditModal = true
    await flushPromises()

    const selectValues = wrapper.findAll('.select-stub').map(node => node.text())

    expect(selectValues).toContain('|anthropic|openai|gemini|antigravity|windsurf')
    expect(
      selectValues.filter(text => text === 'anthropic|openai|gemini|antigravity|windsurf').length
    ).toBeGreaterThanOrEqual(2)
  })
})
