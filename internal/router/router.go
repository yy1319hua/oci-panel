package router

import (
        "fmt"
        "net/http"
        "strings"

        "github.com/adiecho/oci-panel/internal/config"
        "github.com/adiecho/oci-panel/internal/controllers"
        "github.com/adiecho/oci-panel/internal/logger"
        "github.com/adiecho/oci-panel/internal/middleware"
        "github.com/adiecho/oci-panel/internal/models"
        "github.com/adiecho/oci-panel/internal/services"
        "github.com/gin-gonic/gin"
)

type Services struct {
        Scheduler  *services.SchedulerService
        Telegram   *services.TelegramService
        Automation *services.AutomationService
}

func Setup(r *gin.Engine, cfg *config.Config) *Services {
        r.Use(middleware.RequestBodyLimit(2 << 20))
        r.Use(middleware.CORS(cfg.Web.AllowOrigins))
        r.Use(middleware.AuthMiddleware())
        // API 访问日志：在实时日志页展示每个 /api 调用的方法/路径/状态码/耗时/来源。
        r.Use(middleware.AccessLogger())

        // 静态资源 - 前端构建文件
        // assets 文件名带内容 hash，可安全长缓存（内容变了文件名必变）
        r.Static("/assets", "./frontend/dist/assets")
        // 站点图标。必须逐个显式映射：NoRoute 的 SPA 兜底会把未映射的路径当成前端路由，
        // 返回 index.html（200 + text/html），否则浏览器拿到的"图标"其实是网页。
        r.StaticFile("/favicon.ico", "./frontend/dist/favicon.ico")
        r.StaticFile("/favicon.svg", "./frontend/dist/favicon.svg")
        r.StaticFile("/apple-touch-icon.png", "./frontend/dist/apple-touch-icon.png")

        // serveIndex 输出 index.html 并禁止缓存，确保每次部署后浏览器都拉取到最新资源清单
        serveIndex := func(c *gin.Context) {
                c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
                c.Header("Pragma", "no-cache")
                c.Header("Expires", "0")
                c.File("./frontend/dist/index.html")
        }

        // 直接访问根路径返回前端页面
        r.GET("/", serveIndex)

        ociService := services.NewOCIService(cfg)
        instanceService := services.NewInstanceService(ociService)
        logService := services.NewLogStreamService()
        schedulerService := services.NewSchedulerService(ociService)
        telegramService := services.NewTelegramService(ociService)
        automationService := services.NewAutomationService(ociService, telegramService)

        // 把「后端日志」与「API 访问日志」统一汇入实时日志页：
        //  - logger.SetBroadcaster：所有 log.Print* / slog 输出实时推送到日志流；
        //  - SetAccessLogSink：/api 访问日志同样推送。
        // 两者都通过 logService 的广播通道发出，前端实时日志页即可看到。
        logger.SetBroadcaster(func(level, message string) {
                logService.SendLog(level, message)
        })
        middleware.SetAccessLogSink(func(level, message string) {
                logService.SendLog(level, message)
        })

        logCtrl := controllers.NewLogStreamController(logService, cfg.Web.AllowOrigins)

        api := r.Group("/api")
        {
                sysCtrl := controllers.NewSysController(cfg, schedulerService)
                sys := api.Group("/sys")
                {
                        sys.POST("/login", sysCtrl.Login)
                        sys.POST("/checkMfaCode", sysCtrl.CheckMfaCode)
                        sys.POST("/requestPasswordReset", sysCtrl.RequestPasswordReset)
                        sys.POST("/resetPassword", sysCtrl.ResetPassword)
                        sys.GET("/recentLogs", logCtrl.GetRecentLogs)
                        // 实时日志流（SSE）。作为普通 HTTP 请求，直接复用全局鉴权中间件，
                        // 不再需要 WebSocket 时代那套一次性 ticket（/wsTicket 已随之移除）。
                        sys.GET("/logs/stream", logCtrl.StreamLogs)
                        sys.GET("/getVersion", sysCtrl.GetVersion)
                        sys.GET("/getSysCfg", sysCtrl.GetSysCfg)
                automation := api.Group("/automation")
                autoCtrl := controllers.NewAutomationController(automationService)
                // 保活
                automation.GET("/keepalive/list", autoCtrl.ListKeepalive)
                automation.POST("/keepalive/save", autoCtrl.SaveKeepalive)
                automation.POST("/keepalive/delete", autoCtrl.DeleteKeepalive)
                automation.POST("/keepalive/run", autoCtrl.RunKeepalive)
                // 抢机
                automation.GET("/grab/list", autoCtrl.ListGrab)
                automation.POST("/grab/save", autoCtrl.SaveGrab)
                automation.POST("/grab/delete", autoCtrl.DeleteGrab)
                automation.POST("/grab/run", autoCtrl.RunGrab)
                // 备份
                automation.GET("/backup/list", autoCtrl.ListBackup)
                automation.POST("/backup/save", autoCtrl.SaveBackup)
                automation.POST("/backup/delete", autoCtrl.DeleteBackup)
                automation.POST("/backup/run", autoCtrl.RunBackup)
                automation.GET("/backup/volumes", autoCtrl.ListVolumes)
                // 告警 / 配额
                automation.GET("/quota", autoCtrl.GetQuota)
                automation.POST("/settings", autoCtrl.SaveSettings)
                automation.POST("/alert/clear", autoCtrl.ClearAlertState)
                automation.POST("/metrics/cpu-memory", autoCtrl.GetCpuMemory)
                automation.GET("/pushplus/get", autoCtrl.GetPushplus)
                automation.POST("/pushplus/save", autoCtrl.SavePushplus)
                automation.POST("/pushplus/test", autoCtrl.TestPushplus)
                // 抢机表单下拉数据（AD / 镜像 / 子网）
                controllers.SetLookupService(ociService)
                automation.GET("/lookup/ads", autoCtrl.ListAds)
                automation.GET("/lookup/images", autoCtrl.ListImages)
                automation.GET("/lookup/subnets", autoCtrl.ListSubnets)
                        sys.POST("/updateCacheCfg", sysCtrl.UpdateCacheCfg)
                        sys.POST("/refreshCache", sysCtrl.RefreshCache)
                        sys.GET("/getAuthStatus", sysCtrl.GetAuthStatus)
                        sys.POST("/generateMfaSecret", sysCtrl.GenerateMfaSecret)
                        sys.POST("/enableMfa", sysCtrl.EnableMfa)
                        sys.POST("/disableMfa", sysCtrl.DisableMfa)
                        sys.GET("/getProfile", sysCtrl.GetProfile)
                        sys.POST("/changePassword", sysCtrl.ChangePassword)
                        sys.POST("/updateEmail", sysCtrl.UpdateEmail)
                        sys.POST("/updateLogLevel", sysCtrl.UpdateLogLevel)
                        sys.POST("/updateAccount", sysCtrl.UpdateAccount)
                }

                passkeyCtrl := controllers.NewPasskeyController(cfg)
                passkey := api.Group("/passkey")
                {
                        passkey.GET("/status", passkeyCtrl.GetStatus)
                        passkey.POST("/beginRegistration", passkeyCtrl.BeginRegistration)
                        passkey.POST("/finishRegistration", passkeyCtrl.FinishRegistration)
                        passkey.POST("/beginLogin", passkeyCtrl.BeginLogin)
                        passkey.POST("/finishLogin", passkeyCtrl.FinishLogin)
                        passkey.POST("/disable", passkeyCtrl.Disable)
                }

                ociCtrl := controllers.NewOciController(ociService, schedulerService)
                oci := api.Group("/oci")
                {
                        oci.POST("/userPage", ociCtrl.UserPage)
                        oci.POST("/addCfg", ociCtrl.AddCfg)
                        oci.POST("/updateCfgName", ociCtrl.UpdateCfgName)
                        oci.POST("/removeCfg", ociCtrl.RemoveCfg)
                        oci.POST("/uploadKey", ociCtrl.UploadKey)
                        oci.POST("/details", ociCtrl.GetConfigDetails)
                        oci.POST("/details/instances", ociCtrl.GetConfigInstances)
                        oci.POST("/details/volumes", ociCtrl.GetConfigVolumes)
                        oci.POST("/details/vcns", ociCtrl.GetConfigVCNs)
                        oci.POST("/tenant/info", ociCtrl.GetTenantInfo)
                        oci.POST("/tenant/updatePwdEx", ociCtrl.UpdatePasswordExpiry)
                        oci.POST("/tenant/updateUserInfo", ociCtrl.UpdateUserInfo)
                        oci.POST("/tenant/deleteUser", ociCtrl.DeleteUser)
                        oci.POST("/tenant/resetPassword", ociCtrl.ResetPassword)
                        oci.POST("/tenant/deleteMfaDevice", ociCtrl.DeleteMfaDevice)
                        oci.POST("/tenant/deleteApiKey", ociCtrl.DeleteApiKey)
                        oci.POST("/traffic/data", ociCtrl.GetTrafficData)
                        oci.GET("/traffic/condition", ociCtrl.GetTrafficCondition)
                        oci.GET("/traffic/vnics", ociCtrl.GetInstanceVnics)
                        oci.POST("/traffic/monthly", ociCtrl.GetMonthlyTraffic)
                        oci.POST("/traffic/cost", ociCtrl.GetDailyCost)
                        oci.POST("/vcn/securityList", ociCtrl.GetSecurityList)
                        oci.POST("/vcn/addSecurityRule", ociCtrl.AddSecurityRule)
                        oci.POST("/vcn/updateSecurityRule", ociCtrl.UpdateSecurityRule)
                        oci.POST("/vcn/deleteSecurityRule", ociCtrl.DeleteSecurityRule)
                        oci.POST("/vcn/releaseSecurityRules", ociCtrl.ReleaseSecurityRules)
                        oci.POST("/vcn/delete", ociCtrl.DeleteVcn)
                }

                instanceCtrl := controllers.NewInstanceController(instanceService, logService)
                instance := api.Group("/instance")
                {
                        instance.POST("/list", instanceCtrl.ListInstances)
                        instance.POST("/start", instanceCtrl.StartInstance)
                        instance.POST("/stop", instanceCtrl.StopInstance)
                        instance.POST("/reboot", instanceCtrl.RebootInstance)
                        instance.POST("/terminate", instanceCtrl.TerminateInstance)
                        instance.POST("/updateName", instanceCtrl.UpdateInstanceName)
                        instance.POST("/changeIP", instanceCtrl.ChangePublicIP)
                        instance.POST("/updateConfig", instanceCtrl.UpdateInstanceConfig)
                        instance.POST("/updateBootVolume", instanceCtrl.UpdateBootVolume)
                        instance.POST("/attachIPv6", instanceCtrl.AttachIPv6)
                        instance.POST("/autoRescue", instanceCtrl.AutoRescue)
                }

                bootVolume := api.Group("/bootVolume")
                {
                        bootVolume.POST("/update", instanceCtrl.UpdateBootVolumeById)
                }

                telegramCtrl := controllers.NewTelegramController(telegramService)
                telegram := api.Group("/telegram")
                {
                        telegram.GET("/getConfig", telegramCtrl.GetConfig)
                        telegram.POST("/updateConfig", telegramCtrl.UpdateConfig)
                        telegram.POST("/testConnection", telegramCtrl.TestConnection)
                        telegram.POST("/sendTestMessage", telegramCtrl.SendTestMessage)
                        telegram.POST("/startBot", telegramCtrl.StartBot)
                        telegram.POST("/stopBot", telegramCtrl.StopBot)
                }

                botCtrl := controllers.NewBotController(ociService)
                bot := api.Group("/bot")
                {
                        // 紧凑只读摘要：机器人通过 API Token 拉取实例概况
                        bot.GET("/summary", botCtrl.Summary)
                }

                // API Token 管理：仅管理员 JWT 可调用（防止用一把 API Key 管理其它 Key）
                tokenCtrl := controllers.NewTokenController()
                token := api.Group("/token", middleware.RequireAdmin())
                {
                        token.POST("", tokenCtrl.CreateToken)
                        token.GET("/list", tokenCtrl.ListTokens)
                        token.GET("/calls", tokenCtrl.ListTokenCalls)
                        token.POST("/revoke", tokenCtrl.RevokeToken)
                }
        }

        // 兜底路由：分两类处理，避免把 index.html 当成接口响应返回。
        //
        // 此前对所有未匹配路径都返回 SPA 的 index.html（200 + text/html），
        // 导致 /api 下的路径写错、或接口已被删除时，前端 axios 拿到的是 HTML，
        // 解析失败报出一堆莫名其妙的错误，排查成本极高（状态码还是 200，看着像"通了"）。
        //
        // - /api/** ：返回标准 JSON 404，前端能看到明确的"接口不存在"；
        // - 其余路径：照旧返回 index.html，交给前端路由接管。
        r.NoRoute(func(c *gin.Context) {
                path := c.Request.URL.Path
                if path == "/api" || strings.HasPrefix(path, "/api/") {
                        c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
                        c.Header("Pragma", "no-cache")
                        c.Header("Expires", "0")
                        c.JSON(
                                http.StatusNotFound,
                                models.ErrorResponse(http.StatusNotFound, fmt.Sprintf("接口不存在: %s %s", c.Request.Method, path)),
                        )
                        return
                }
                serveIndex(c)
        })

        return &Services{
                Scheduler:  schedulerService,
                Telegram:   telegramService,
                Automation: automationService,
        }
}
