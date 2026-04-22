import { defineComponent, nextTick, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import UsageFilters from '../UsageFilters.vue'

const { groupsList, getModelStats } = vi.hoisted(() => ({
  groupsList: vi.fn(),
  getModelStats: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: groupsList,
    },
    dashboard: {
      getModelStats,
    },
    usage: {
      searchUsers: vi.fn(),
      searchApiKeys: vi.fn(),
    },
    accounts: {
      list: vi.fn(),
    },
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

const SelectStub = defineComponent({
  name: 'Select',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: null,
    },
    options: {
      type: Array,
      required: true,
    },
  },
  template: `
    <div
      class="select-stub"
      :data-model-value="modelValue == null ? '' : String(modelValue)"
      :data-options="JSON.stringify(options)"
    />
  `,
})

const Host = defineComponent({
  components: { UsageFilters },
  setup() {
    const filters = ref<Record<string, any>>({
      platform: undefined,
      model: 'gpt-4o',
      group_id: 11,
      start_date: '2026-04-22',
      end_date: '2026-04-23',
    })

    const setPlatform = async (platform?: string) => {
      filters.value = {
        ...filters.value,
        platform,
      }
      await nextTick()
    }

    return {
      filters,
      setPlatform,
    }
  },
  template: `
    <UsageFilters
      v-model="filters"
      :exporting="false"
      start-date="2026-04-22"
      end-date="2026-04-23"
      :show-actions="false"
    />
  `,
})

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T) => void
}

const deferred = <T,>(): Deferred<T> => {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

describe('UsageFilters', () => {
  beforeEach(() => {
    groupsList.mockReset()
    getModelStats.mockReset()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('filters groups by platform and clears stale group/model selections after platform change', async () => {
    groupsList
      .mockResolvedValueOnce({
        items: [{ id: 11, name: 'OpenAI Group' }],
      })
      .mockResolvedValueOnce({
        items: [{ id: 22, name: 'Windsurf Group' }],
      })

    getModelStats
      .mockResolvedValueOnce({
        models: [{ model: 'gpt-4o' }],
      })
      .mockResolvedValueOnce({
        models: [{ model: 'windsurf-sonnet' }],
      })

    const wrapper = mount(Host, {
      global: {
        stubs: {
          Select: SelectStub,
        },
      },
    })

    await flushPromises()

    await (wrapper.vm as any).setPlatform('windsurf')
    await flushPromises()

    expect(groupsList).toHaveBeenLastCalledWith(1, 1000, { platform: 'windsurf' })
    expect(getModelStats).toHaveBeenLastCalledWith({
      start_date: '2026-04-22',
      end_date: '2026-04-23',
      platform: 'windsurf',
    })

    expect((wrapper.vm as any).filters.group_id).toBeUndefined()
    expect((wrapper.vm as any).filters.model).toBeUndefined()

    const selectStubs = wrapper.findAll('.select-stub')
    const modelOptions = JSON.parse(selectStubs[1].attributes('data-options'))
    const groupOptions = JSON.parse(selectStubs[5].attributes('data-options'))

    expect(modelOptions.some((option: { value: string }) => option.value === 'windsurf-sonnet')).toBe(true)
    expect(groupOptions.some((option: { value: number }) => option.value === 22)).toBe(true)
    expect(groupOptions.some((option: { value: number }) => option.value === 11)).toBe(false)
  })

  it('ignores stale option responses when platform changes quickly', async () => {
    const openaiGroups = deferred<{ items: Array<{ id: number; name: string }> }>()
    const windsurfGroups = deferred<{ items: Array<{ id: number; name: string }> }>()
    const openaiModels = deferred<{ models: Array<{ model: string }> }>()
    const windsurfModels = deferred<{ models: Array<{ model: string }> }>()

    groupsList
      .mockResolvedValueOnce({ items: [] })
      .mockImplementationOnce(() => openaiGroups.promise)
      .mockImplementationOnce(() => windsurfGroups.promise)

    getModelStats
      .mockResolvedValueOnce({ models: [{ model: 'gpt-4o' }] })
      .mockImplementationOnce(() => openaiModels.promise)
      .mockImplementationOnce(() => windsurfModels.promise)

    const wrapper = mount(Host, {
      global: {
        stubs: {
          Select: SelectStub,
        },
      },
    })

    await flushPromises()

    await (wrapper.vm as any).setPlatform('openai')
    await (wrapper.vm as any).setPlatform('windsurf')

    windsurfGroups.resolve({ items: [{ id: 22, name: 'Windsurf Group' }] })
    windsurfModels.resolve({ models: [{ model: 'windsurf-sonnet' }] })
    await flushPromises()

    openaiGroups.resolve({ items: [{ id: 11, name: 'OpenAI Group' }] })
    openaiModels.resolve({ models: [{ model: 'gpt-4o' }] })
    await flushPromises()

    const selectStubs = wrapper.findAll('.select-stub')
    const modelOptions = JSON.parse(selectStubs[1].attributes('data-options'))
    const groupOptions = JSON.parse(selectStubs[5].attributes('data-options'))

    expect(modelOptions.some((option: { value: string }) => option.value === 'windsurf-sonnet')).toBe(true)
    expect(modelOptions.some((option: { value: string }) => option.value === 'gpt-4o')).toBe(false)
    expect(groupOptions.some((option: { value: number }) => option.value === 22)).toBe(true)
    expect(groupOptions.some((option: { value: number }) => option.value === 11)).toBe(false)
  })
})
