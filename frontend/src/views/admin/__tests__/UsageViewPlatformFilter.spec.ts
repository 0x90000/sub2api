import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import UsageView from '../UsageView.vue'

const { list, getStats, getSnapshotV2, getModelStats, getById, groupsList } = vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  })

  return {
    list: vi.fn(),
    getStats: vi.fn(),
    getSnapshotV2: vi.fn(),
    getModelStats: vi.fn(),
    getById: vi.fn(),
    groupsList: vi.fn(),
  }
})

const messages: Record<string, string> = {
  'admin.dashboard.timeRange': 'Time Range',
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.usage.failedToLoadUser': 'Failed to load user',
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      list,
      getStats,
    },
    dashboard: {
      getSnapshotV2,
      getModelStats,
    },
    users: {
      getById,
    },
    groups: {
      list: groupsList,
    },
  },
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    list: vi.fn(),
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/utils/format', () => ({
  formatReasoningEffort: (value: string | null | undefined) => value ?? '-',
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {}
  })
}))

const AppLayoutStub = { template: '<div><slot /></div>' }
const UsageFiltersStub = defineComponent({
  props: {
    modelValue: {
      type: Object,
      required: true,
    },
  },
  emits: ['update:modelValue', 'change', 'reset'],
  methods: {
    applyPlatform() {
      this.$emit('update:modelValue', {
        ...this.modelValue,
        platform: 'windsurf',
      })
      this.$emit('change')
    },
    resetFilters() {
      this.$emit('reset')
    },
  },
  template: `
    <div>
      <button class="apply-platform" @click="applyPlatform">apply</button>
      <button class="reset-filters" @click="resetFilters">reset</button>
      <slot name="after-reset" />
    </div>
  `,
})

describe('admin UsageView platform filter', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getModelStats.mockReset()
    getById.mockReset()
    groupsList.mockReset()

    list.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
    })
    getStats.mockResolvedValue({
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      average_duration_ms: 0,
      endpoints: [],
      upstream_endpoints: [],
      endpoint_paths: [],
    })
    getSnapshotV2.mockResolvedValue({
      trend: [],
      groups: [],
    })
    getModelStats.mockResolvedValue({
      models: [],
      start_date: '2026-04-22',
      end_date: '2026-04-23',
    })
    groupsList.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 1000,
      pages: 0,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('passes platform to usage list, stats, model stats, and snapshot then clears it on reset', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: true,
          GroupDistributionChart: true,
          EndpointDistributionChart: true,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    list.mockClear()
    getStats.mockClear()
    getModelStats.mockClear()
    getSnapshotV2.mockClear()

    await wrapper.find('.apply-platform').trigger('click')
    await flushPromises()

    expect(list).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'windsurf',
    }), expect.anything())
    expect(getStats).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'windsurf',
    }))
    expect(getModelStats).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'windsurf',
    }))
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'windsurf',
    }))

    list.mockClear()
    getStats.mockClear()
    getModelStats.mockClear()
    getSnapshotV2.mockClear()

    await wrapper.find('.reset-filters').trigger('click')
    await flushPromises()

    expect(list).toHaveBeenCalledWith(expect.not.objectContaining({
      platform: 'windsurf',
    }), expect.anything())
    expect(getStats).toHaveBeenCalledWith(expect.not.objectContaining({
      platform: 'windsurf',
    }))
    expect(getModelStats).toHaveBeenCalledWith(expect.not.objectContaining({
      platform: 'windsurf',
    }))
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.not.objectContaining({
      platform: 'windsurf',
    }))
  })
})
