import { BusinessCode } from './api'
import type { ApiError, ApiErrorInterceptor } from './api'

interface ProfileState {
  items: Array<{ id: string }>
  selectedId: string
  refresh: () => Promise<unknown>
}

interface RouterState {
  currentRoute: { value: { path: string; meta: Record<string, unknown> } }
  replace: (location: string) => Promise<unknown> | unknown
}

type Notify = (message: string) => Promise<unknown> | unknown

export async function enforceProfileRoute(profiles: ProfileState, router: RouterState) {
  const route = router.currentRoute.value
  if (!route.meta.requiresProfile || profiles.items.length || route.path === '/settings') return false
  await router.replace('/settings')
  return true
}

export function createProfileErrorInterceptor(profiles: ProfileState, router: RouterState, notify: Notify): ApiErrorInterceptor {
  let recovery: Promise<void> | null = null

  return async (error: ApiError) => {
    if (error.code !== BusinessCode.ProfileNotFound) return false
    if (!recovery) {
      profiles.selectedId = ''
      recovery = (async () => {
        await profiles.refresh()
        await notify(error.message)
        await enforceProfileRoute(profiles, router)
      })().finally(() => { recovery = null })
    }
    await recovery
    return true
  }
}
