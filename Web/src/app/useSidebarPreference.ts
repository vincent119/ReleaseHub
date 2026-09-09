import { useEffect, useState } from 'react'

export const SIDEBAR_STORAGE_KEY = 'releasehub.sidebar.collapsed'

export function parseSidebarCollapsed(value: string | null): boolean {
  return value === 'true'
}

export function readSidebarCollapsed(
  storage: Pick<Storage, 'getItem'> = window.localStorage,
): boolean {
  try {
    return parseSidebarCollapsed(storage.getItem(SIDEBAR_STORAGE_KEY))
  } catch {
    return false
  }
}

export function writeSidebarCollapsed(
  storage: Pick<Storage, 'setItem'>,
  collapsed: boolean,
): void {
  try {
    storage.setItem(SIDEBAR_STORAGE_KEY, String(collapsed))
  } catch {
    // 瀏覽器儲存空間不可用時，仍保留目前頁面的記憶體狀態。
  }
}

export function useSidebarPreference() {
  const [collapsed, setCollapsed] = useState(readSidebarCollapsed)

  useEffect(() => {
    writeSidebarCollapsed(window.localStorage, collapsed)
  }, [collapsed])

  return { collapsed, setCollapsed }
}
