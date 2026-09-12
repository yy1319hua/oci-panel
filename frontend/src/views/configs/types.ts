// 视图与 API 共用同一份类型，避免 UUID 和资源字段在组件间漂移。
export type { ConfigItem as Config, InstanceInfo as Instance } from '@/api/oci'
