import { apiClient } from '../client'

export interface WindsurfTokenInfo {
  token?: string
  access_token?: string
  refresh_token?: string
  id_token?: string
  token_type?: string
  scope?: string
  expires_in?: number
  expires_at?: number
  subject?: string
  email?: string
  display_name?: string
  avatar_url?: string
  api_server_url?: string
  [key: string]: unknown
}

export async function refreshToken(
  refreshToken: string,
  proxyId?: number | null
): Promise<WindsurfTokenInfo> {
  const payload: Record<string, unknown> = {
    refresh_token: refreshToken
  }
  if (proxyId) {
    payload.proxy_id = proxyId
  }

  const { data } = await apiClient.post<WindsurfTokenInfo>(
    '/admin/windsurf/oauth/refresh-token',
    payload
  )
  return data
}

export default {
  refreshToken
}
