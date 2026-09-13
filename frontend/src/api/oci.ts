import { get, post, type PageResult, type ValueLabel } from './http'

/** OCI 配置（账号凭据）列表项。 */
export interface ConfigItem {
  id: string
  username: string
  tenantName?: string
  tenantCreateTime?: string
  ociRegion: string
  ociUserId?: string
  ociFingerprint?: string
  ociTenantId?: string
  ociKeyPath?: string
  instanceCount?: number
  runningInstances?: number
}

/** 配置详情（基础信息；其余字段由后端补充，故带索引签名保持宽松）。 */
export interface ConfigDetails {
  userId: string
  username: string
  region: string
  fingerprint: string
  keyPath: string
  [key: string]: unknown
}

/** 实例信息（详情页实例列表项）。 */
export interface InstanceInfo {
  id: string
  displayName: string
  state: string
  shape: string
  ocpus: number
  memory: number
  bootVolumeSize?: number
  bootVolumeVpu?: number
  region: string
  publicIps?: string[]
  ipv6?: string
  imageName?: string
  createTime?: string
}

/** 引导卷信息（字段较多且前端松散使用，保持宽松）。 */
export interface VolumeInfo {
  id: string
  displayName: string
  [key: string]: unknown
}

/** VCN 信息（含子网，字段较多，保持宽松）。 */
export interface VCNInfo {
  id: string
  displayName: string
  [key: string]: unknown
}

/** 租户信息（区域/用户列表/密码策略等，字段较多，保持宽松）。 */
export interface TenantInfo {
  name?: string
  id?: string
  [key: string]: unknown
}

/** 流量查询条件。 */
export interface TrafficCondition {
  regions?: ValueLabel[]
  instances: ValueLabel[]
}

/** 流量数据（按时间点的入站/出站序列，单位 MB）。 */
export interface TrafficData {
  time: string[]
  inbound: string[]
  outbound: string[]
}

/** 单实例流量明细（字节）。 */
export interface InstanceTrafficStat {
  instanceId: string
  displayName: string
  inbound: number
  outbound: number
  billable: number
}

/** 账号级月度流量（实际 + 计费 + 每实例明细 + 逐日序列）。 */
export interface MonthlyTrafficStats {
  instanceCount: number
  inboundTraffic: number
  outboundTraffic: number
  billableTraffic: number
  freeAllowance: number
  instances: InstanceTrafficStat[]
  dailyLabels: string[]
  dailyInbound: number[]
  dailyOutbound: number[]
}

/** 单日成本（金额为账号维度合计）。 */
export interface DailyCost {
  date: string
  amount: number
  currency: string
}

/** 成本统计（逐日序列 + 本月累计）。 */
export interface CostStats {
  days: DailyCost[]
  monthToDate: number
  currency: string
  daysCount: number
}

/** 安全规则（与后端 models.SecurityRule 对应；协议+来源/目标+端口范围构成定位键）。 */
export interface SecurityRule {
  isStateless?: boolean
  protocol: string
  protocolName?: string
  source?: string
  destination?: string
  portRangeMin?: number
  portRangeMax?: number
  icmpType?: number | null
  icmpCode?: number | null
  description?: string
}

/** VCN 安全列表（入/出站规则）。 */
export interface SecurityListData {
  id?: string
  displayName?: string
  vcnId?: string
  ingressRules?: SecurityRule[]
  egressRules?: SecurityRule[]
  [key: string]: unknown
}

/** 添加安全规则入参。 */
export interface AddSecurityRuleReq {
  configId: string
  vcnId?: string
  isIngress: boolean
  protocol: string
  source?: string
  destination?: string
  portMin?: number
  portMax?: number
  description?: string
}

/** 修改安全规则入参：oldRule 用于定位，newRule 为改后的值。 */
export interface UpdateSecurityRuleReq {
  configId: string
  vcnId: string
  isIngress: boolean
  oldRule: SecurityRule
  newRule: SecurityRule
}

/** 删除安全规则入参：rule 用于定位。 */
export interface DeleteSecurityRuleReq {
  configId: string
  vcnId: string
  isIngress: boolean
  rule: SecurityRule
}

/** 配置（账号）与其资源（实例/卷/VCN/租户/镜像/流量）相关接口。 */
export const ociApi = {
  userPage: (req: { page: number; pageSize: number; username?: string }) =>
    post<PageResult<ConfigItem>>('/oci/userPage', req),
  uploadKey: (formData: FormData) =>
    post<string>('/oci/uploadKey', formData, { headers: { 'Content-Type': 'multipart/form-data' } }),
  addCfg: (req: {
    username: string
    tenantName: string
    ociTenantId: string
    ociUserId: string
    ociFingerprint: string
    ociRegion: string
    ociKeyPath: string
  }) => post('/oci/addCfg', req),
  updateCfgName: (req: { id: string; username: string; ociKeyPath?: string }) => post('/oci/updateCfgName', req),
  removeCfg: (ids: string[]) => post('/oci/removeCfg', { ids }),

  details: (configId: string) => post<ConfigDetails>('/oci/details', { configId }),
  detailsInstances: (req: { configId: string; clearCache?: boolean }) =>
    post<InstanceInfo[]>('/oci/details/instances', req),
  detailsVolumes: (req: { configId: string; clearCache?: boolean }) => post<VolumeInfo[]>('/oci/details/volumes', req),
  detailsVCNs: (req: { configId: string; clearCache?: boolean }) => post<VCNInfo[]>('/oci/details/vcns', req),

  tenantInfo: (req: { configId: string; clearCache?: boolean }) => post<TenantInfo>('/oci/tenant/info', req),
  updatePwdEx: (req: { cfgId: string; passwordExpiresAfter: number }) => post('/oci/tenant/updatePwdEx', req),
  updateUserInfo: (req: { ociCfgId: string; userId: string; email: string; dbUserName: string; description: string }) =>
    post('/oci/tenant/updateUserInfo', req),
  resetPassword: (req: { ociCfgId: string; userId: string }) => post('/oci/tenant/resetPassword', req),
  deleteMfaDevice: (req: { ociCfgId: string; userId: string }) => post('/oci/tenant/deleteMfaDevice', req),
  deleteApiKey: (req: { ociCfgId: string; userId: string }) => post('/oci/tenant/deleteApiKey', req),
  deleteUser: (req: { ociCfgId: string; userId: string }) => post('/oci/tenant/deleteUser', req),

  trafficCondition: (configId: string) => get<TrafficCondition>('/oci/traffic/condition', { params: { configId } }),
  trafficVnics: (req: { configId: string; instanceId: string }) =>
    get<ValueLabel[]>('/oci/traffic/vnics', { params: req }),
  trafficData: (req: { configId: string; instanceId: string; vnicId: string; startTime: string; endTime: string }) =>
    post<TrafficData>('/oci/traffic/data', req),
  monthlyTraffic: (configId: string) => post<MonthlyTrafficStats>('/oci/traffic/monthly', { configId }),
  dailyCost: (req: { configId: string; days?: number }) => post<CostStats>('/oci/traffic/cost', req)
}

/** VCN 安全规则相关接口（独立分组）。 */
export const vcnApi = {
  securityList: (req: { configId: string; vcnId: string }) => post<SecurityListData>('/oci/vcn/securityList', req),
  addSecurityRule: (req: AddSecurityRuleReq) => post('/oci/vcn/addSecurityRule', req),
  updateSecurityRule: (req: UpdateSecurityRuleReq) => post('/oci/vcn/updateSecurityRule', req),
  deleteSecurityRule: (req: DeleteSecurityRuleReq) => post('/oci/vcn/deleteSecurityRule', req),
  releaseSecurityRules: (req: { configId: string; vcnId?: string }) => post('/oci/vcn/releaseSecurityRules', req),
  delete: (req: { configId: string; vcnId?: string }) => post('/oci/vcn/delete', req)
}
