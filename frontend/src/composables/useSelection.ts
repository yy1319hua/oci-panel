import { ref, computed, watch, type Ref } from 'vue'

/**
 * 当前页多选。翻页、筛选或删除后移除已不可见的选项，避免批量操作误用旧页 ID。
 */
export function useSelection<T>(items: Ref<T[]>, getId: (item: T) => string) {
  const selectedIds = ref<string[]>([])
  const visibleIds = computed(() => new Set(items.value.map(getId)))
  const selectedCount = computed(() => selectedIds.value.filter(id => visibleIds.value.has(id)).length)

  const isAllSelected = computed(() => visibleIds.value.size > 0 && selectedCount.value === visibleIds.value.size)
  const isIndeterminate = computed(() => selectedCount.value > 0 && selectedCount.value < visibleIds.value.size)

  watch(
    visibleIds,
    ids => {
      selectedIds.value = selectedIds.value.filter(id => ids.has(id))
    },
    { flush: 'sync' }
  )

  /** 切换单项选中状态（已选则取消，未选则选中）。 */
  const toggle = (id: string) => {
    if (!visibleIds.value.has(id)) return
    const index = selectedIds.value.indexOf(id)
    if (index === -1) selectedIds.value.push(id)
    else selectedIds.value.splice(index, 1)
  }

  /** 全选 / 取消全选：当前已全选则清空，否则选中当前列表全部项。 */
  const toggleAll = () => {
    if (isAllSelected.value) selectedIds.value = []
    else selectedIds.value = [...visibleIds.value]
  }

  const clear = () => {
    selectedIds.value = []
  }

  return { selectedIds, isAllSelected, isIndeterminate, toggle, toggleAll, clear }
}
