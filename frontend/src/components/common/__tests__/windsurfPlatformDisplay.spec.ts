import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import type { AccountPlatform, GroupPlatform } from '@/types'
import PlatformIcon from '../PlatformIcon.vue'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'
import GroupBadge from '../GroupBadge.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'groups.subscription') return 'Subscription'
      if (key === 'admin.users.expired') return 'Expired'
      if (key === 'admin.users.daysRemaining') return `${params?.days ?? ''}d`
      if (key === 'admin.accounts.subscriptionExpires') return 'Expires'
      return key
    }
  })
}))

describe('windsurf platform display', () => {
  it('extends frontend platform unions with windsurf', () => {
    const groupPlatform: GroupPlatform = 'windsurf'
    const accountPlatform: AccountPlatform = 'windsurf'

    expect(groupPlatform).toBe('windsurf')
    expect(accountPlatform).toBe('windsurf')
  })

  it('renders a dedicated windsurf icon', () => {
    const wrapper = mount(PlatformIcon, {
      props: {
        platform: 'windsurf',
        size: 'sm'
      }
    })

    expect(wrapper.find('[data-platform-icon=\"windsurf\"]').exists()).toBe(true)
  })

  it('renders windsurf-specific badge styles', () => {
    const platformTypeBadge = mount(PlatformTypeBadge, {
      props: {
        platform: 'windsurf',
        type: 'apikey'
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(platformTypeBadge.text()).toContain('Windsurf')
    expect(platformTypeBadge.html()).toContain('bg-cyan-100')

    const groupBadge = mount(GroupBadge, {
      props: {
        name: 'windsurf-default',
        platform: 'windsurf',
        subscriptionType: 'standard',
        rateMultiplier: 1
      }
    })

    expect(groupBadge.html()).toContain('bg-cyan-50')
    expect(groupBadge.text()).toContain('windsurf-default')
  })
})
