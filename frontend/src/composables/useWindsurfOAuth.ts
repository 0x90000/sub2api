import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { WindsurfTokenInfo } from '@/api/admin/windsurf'

export function useWindsurfOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    authUrl.value = ''
    sessionId.value = ''
    loading.value = false
    error.value = ''
  }

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<WindsurfTokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.windsurf.pleaseEnterRefreshToken')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const tokenInfo = await adminAPI.windsurf.refreshToken(refreshToken.trim(), proxyId)
      return tokenInfo as WindsurfTokenInfo
    } catch (err: any) {
      error.value =
        err.response?.data?.detail || t('admin.accounts.oauth.windsurf.failedToValidateRT')
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: WindsurfTokenInfo): Record<string, unknown> => {
    const credentials: Record<string, unknown> = {}

    if (tokenInfo.token) {
      credentials.token = tokenInfo.token
    }
    if (tokenInfo.access_token) {
      credentials.access_token = tokenInfo.access_token
    }
    if (tokenInfo.refresh_token) {
      credentials.refresh_token = tokenInfo.refresh_token
    }
    if (tokenInfo.id_token) {
      credentials.id_token = tokenInfo.id_token
    }
    if (tokenInfo.token_type) {
      credentials.token_type = tokenInfo.token_type
    }
    if (tokenInfo.scope) {
      credentials.scope = tokenInfo.scope
    }
    if (typeof tokenInfo.expires_in === 'number') {
      credentials.expires_in = tokenInfo.expires_in
    }
    if (typeof tokenInfo.expires_at === 'number') {
      credentials.expires_at = tokenInfo.expires_at
    }
    if (tokenInfo.api_server_url) {
      credentials.api_server_url = tokenInfo.api_server_url
    }

    return credentials
  }

  const buildExtraInfo = (tokenInfo: WindsurfTokenInfo): Record<string, unknown> => {
    const extra: Record<string, unknown> = {
      oauth_last_refresh_at: new Date().toISOString(),
      oauth_last_refresh_error: ''
    }

    if (tokenInfo.display_name) {
      extra.oauth_display_name = tokenInfo.display_name
    }
    if (tokenInfo.email) {
      extra.oauth_email = tokenInfo.email
    }
    if (tokenInfo.avatar_url) {
      extra.oauth_avatar_url = tokenInfo.avatar_url
    }

    return extra
  }

  const generateAuthUrl = async (): Promise<boolean> => {
    error.value = t('admin.accounts.oauth.windsurf.authCodeUnsupported')
    appStore.showError(error.value)
    return false
  }

  const exchangeAuthCode = async (): Promise<WindsurfTokenInfo | null> => {
    error.value = t('admin.accounts.oauth.windsurf.authCodeUnsupported')
    appStore.showError(error.value)
    return null
  }

  return {
    authUrl,
    sessionId,
    loading,
    error,
    resetState,
    validateRefreshToken,
    buildCredentials,
    buildExtraInfo,
    generateAuthUrl,
    exchangeAuthCode
  }
}
