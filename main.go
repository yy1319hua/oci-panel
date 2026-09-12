package main

import (
	"log"

	"github.com/adiecho/oci-panel/internal/config"
	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/logger"
	"github.com/adiecho/oci-panel/internal/middleware"
	"github.com/adiecho/oci-panel/internal/router"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	// 初始化分级日志器（级别来自 config.toml，并支持运行时动态调整）。
	logger.Setup(cfg.Logging.Level)

	// 必须先初始化数据库：InitJwtSecret 在未配置密钥时会把自动生成的密钥持久化到数据库。
	if err := database.InitDB(cfg.Database.DSN); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	middleware.InitJwtSecret(cfg)

	// 用 config.toml 中的初始账号密码创建首个管理员（若数据库为空）。
	// 之后管理员凭据以数据库为准，支持运行时改密、邮箱重置，无需重启。
	if err := services.SeedAdminFromConfig(cfg.Web.Account, cfg.Web.Password); err != nil {
		log.Fatalf("Failed to seed admin user: %v", err)
	}

	r := gin.Default()
	services := router.Setup(r, cfg)

	// 启动定时任务服务
	services.Scheduler.Start()
	defer services.Scheduler.Stop()

	// 启动创建实例任务服务
	if err := services.Task.Start(); err != nil {
		log.Fatalf("Failed to start task service: %v", err)
	}
	defer services.Task.Stop()

	// 启动 Telegram Bot（如果已配置并启用）
	_, _, tgEnabled := services.Telegram.GetConfig()
	if tgEnabled {
		services.Telegram.StartBot()
		defer services.Telegram.StopBot()
	}

	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := r.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
