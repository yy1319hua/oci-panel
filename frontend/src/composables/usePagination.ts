import { ref } from 'vue'

/** 后端分页响应中分页计算所需的字段（list 等其余字段由调用方各自处理）。 */
export interface PageResult {
  page: number
  total: number
}

/**
 * 分页状态 composable（C8）：集中 currentPage / pageSize / totalPages 以及
 * `totalPages = ceil(total / pageSize)` 计算，消除 Configs / Keys / Tasks 中重复的分页样板。
 * 仅持有分页状态；具体的数据加载函数仍由各视图自行实现（API 各异），
 * 加载成功后调用 `applyResult(response.data)` 同步当前页与总页数。行为与原内联实现一致。
 *
 * @param pageSize 每页条数（默认 10，与原各视图常量一致）
 */
export function usePagination(pageSize = 10) {
  const currentPage = ref(1)
  const totalPages = ref(0)

  /** 用后端分页响应回填当前页与总页数。 */
  const applyResult = (result: PageResult) => {
    currentPage.value = result.page
    totalPages.value = Math.ceil(result.total / pageSize)
  }

  return { currentPage, pageSize, totalPages, applyResult }
}
