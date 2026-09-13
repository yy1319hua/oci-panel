package router

import (
	"github.com/adiecho/oci-panel/internal/config"
	"github.com/adiecho/oci-panel/internal/controllers"
	"github.com/adiecho/oci-panel/internal/logger"
	"github.com/adiecho/oci-panel/internal/middleware"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

type Services struct {
	Scheduler *services.SchedulerService
	Telegram  *services.TelegramService
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
	r.StaticFile("/favicon.ico", "./frontend/dist/favicon.ico")

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
	ipService := services.NewIpService(ociService)
	wsService := services.NewWebSocketService()
	schedulerService := services.NewSchedulerService(ociService)
	telegramService := services.NewTelegramService(ociService)

	// 把「后端日志」与「API 访问日志」统一汇入实时日志页：
	//  - logger.SetBroadcaster：所有 log.Print* / slog 输出实时推送到 WebSocket；
	//  - SetAccessLogSink：/api 访问日志同样推送。
	// 两者都通过 wsService 的广播通道发出，前端实时日志页即可看到。
	logger.SetBroadcaster(func(level, message string) {
		wsService.SendLog(level, message)
	})
	middleware.SetAccessLogSink(func(level, message string) {
		wsService.SendLog(level, message)
	})

	wsCtrl := controllers.NewWebSocketController(wsService, cfg.Web.AllowOrigins)
	r.GET("/ws/logs", wsCtrl.HandleWebSocket)

	api := r.Group("/api")
	{
		sysCtrl := controllers.NewSysController(cfg, schedulerService)
		sys := api.Group("/sys")
		{
		sys.POST("/login", sysCtrl.Login)
		sys.POST("/checkMfaCode", sysCtrl.CheckMfaCode)
		sys.POST("/requestPasswordReset", sysCtrl.RequestPasswordReset)
		sys.POST("/resetPassword", sysCtrl.ResetPassword)
		sys.POST("/wsTicket", wsCtrl.IssueTicket)
		sys.GET("/getGlance", sysCtrl.GetGlance)
		sys.GET("/recentLogs", wsCtrl.GetRecentLogs)
		sys.GET("/getVersion", sysCtrl.GetVersion)
		sys.GET("/getSysCfg", sysCtrl.GetSysCfg)
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
			oci.POST("/details/clearCache", ociCtrl.ClearConfigCache)
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
			oci.POST("/images", ociCtrl.ListImages)
		}

		instanceCtrl := controllers.NewInstanceController(instanceService, wsService)
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
			instance.POST("/createCloudShell", instanceCtrl.CreateCloudShell)
			instance.POST("/attachIPv6", instanceCtrl.AttachIPv6)
			instance.POST("/autoRescue", instanceCtrl.AutoRescue)
			instance.POST("/check500MbpsSupport", instanceCtrl.Check500MbpsSupport)
			instance.POST("/enable500Mbps", instanceCtrl.Enable500Mbps)
			instance.POST("/disable500Mbps", instanceCtrl.Disable500Mbps)
		}

		bootVolume := api.Group("/bootVolume")
		{
			bootVolume.POST("/update", instanceCtrl.UpdateBootVolumeById)
		}

		ipCtrl := controllers.NewIpController(ipService)
		ip := api.Group("/ip")
		{
			ip.POST("/change", ipCtrl.ChangePublicIp)
			ip.POST("/attachIpv6", ipCtrl.AttachIpv6)
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
			telegram.GET("/status", telegramCtrl.GetBotStatus)
		}

		botCtrl := controllers.NewBotController(instanceService)
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
			token.DELETE("/:id", tokenCtrl.RevokeTokenByID)
		}
	}

	// SPA fallback - 所有未匹配的路由都返回 index.html，让前端路由接管
	r.NoRoute(serveIndex)

	return &Services{
		Scheduler: schedulerService,
		Telegram:  telegramService,
	}
}
