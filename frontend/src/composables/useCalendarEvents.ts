// 多日历日程拉取与合并：Promise.all 并行、取消态过滤、过期请求丢弃（stale guard）。
import { ref } from 'vue'
import { calApi, type Calendar } from '../api/calendars'
import type { EventInstance } from '../api/types'
import type { CalEvent } from './eventLayout'

function normalize(list: EventInstance[] | null, cal: Calendar): CalEvent[] {
  return (list ?? []).filter(ev => ev.status !== 'cancelled').map(ev => ({ ev, cal }))
}

export async function loadEvents(cals: Calendar[], from: string, to: string): Promise<CalEvent[]> {
  const lists = await Promise.all(
    cals.map(async cal => {
      try {
        return normalize(await calApi.events(cal.id, from, to), cal)
      } catch (err) {
        // 日历被删除或权限变化时静默跳过，其余错误上抛
        if (typeof err === 'object' && err !== null && 'status' in err && ((err as { status: number }).status === 404 || (err as { status: number }).status === 403)) {
          return [] as CalEvent[]
        }
        throw err
      }
    }),
  )
  return lists.flat()
}

export function useCalendarEvents() {
  const loading = ref(false)
  const error = ref<unknown>(null)
  const events = ref<CalEvent[]>([])
  let seq = 0

  async function load(cals: Calendar[], from: string, to: string): Promise<void> {
    const token = ++seq
    loading.value = true
    error.value = null
    try {
      const result = await loadEvents(cals, from, to)
      if (token === seq) events.value = result
    } catch (err) {
      if (token === seq) error.value = err
    } finally {
      if (token === seq) loading.value = false
    }
  }

  return { loading, error, events, load }
}
