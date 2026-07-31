const API_BASE = '/_profiler/api'

export interface ProfileSummary {
  id: string
  method: string
  url: string
  status_code: number
  timestamp: string
  duration: number
}

export interface Profile {
  id: string
  method: string
  url: string
  status_code: number
  timestamp: string
  duration: number
  collector_data: Record<string, unknown>
}

export interface CollectorMeta {
  name: string
  label: string
  icon: string
  component?: string
}

export interface ListResponse {
  profiles: ProfileSummary[]
  total: number
}

export interface CollectorsResponse {
  collectors: CollectorMeta[]
}

export interface ListParams {
  method?: string
  url?: string
  status?: number
  min_status?: number
  max_status?: number
  limit?: number
  offset?: number
}

async function fetchJSON<T>(url: string): Promise<T> {
  const response = await fetch(url)
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || response.statusText)
  }
  return response.json()
}

export async function listProfiles(params: ListParams = {}): Promise<ListResponse> {
  const searchParams = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') {
      searchParams.set(key, String(value))
    }
  }
  const query = searchParams.toString()
  const url = `${API_BASE}/profiles${query ? '?' + query : ''}`
  return fetchJSON<ListResponse>(url)
}

export async function getProfile(id: string): Promise<Profile> {
  return fetchJSON<Profile>(`${API_BASE}/profiles/${id}`)
}

export async function getCollectors(): Promise<CollectorsResponse> {
  return fetchJSON<CollectorsResponse>(`${API_BASE}/collectors`)
}

export async function purgeProfiles(maxAge: string = '24h'): Promise<{ removed: number }> {
  const response = await fetch(`${API_BASE}/profiles?max_age=${maxAge}`, {
    method: 'DELETE',
  })
  return response.json()
}

export async function clearAllProfiles(): Promise<{ cleared: boolean }> {
  const response = await fetch(`${API_BASE}/profiles/all`, {
    method: 'DELETE',
  })
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || response.statusText)
  }
  return response.json()
}
