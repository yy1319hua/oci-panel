package controllers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/adiecho/oci-panel/internal/util"
	"github.com/gin-gonic/gin"
)

type InstanceController struct {
	instanceService *services.InstanceService
	logService      *services.LogStreamService
}

func NewInstanceController(instanceService *services.InstanceService, logService *services.LogStreamService) *InstanceController {
	return &InstanceController{instanceService: instanceService, logService: logService}
}

type ListInstancesRequest struct {
	UserId        string `json:"userId" binding:"required"`
	CompartmentId string `json:"compartmentId" binding:"required"`
}

func (ic *InstanceController) ListInstances(c *gin.Context) {
	var req ListInstancesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	instances, err := ic.instanceService.ListInstances(req.UserId, req.CompartmentId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(instances, "获取实例列表成功"))
}

type InstanceActionRequest struct {
	UserId     string `json:"userId" binding:"required"`
	InstanceId string `json:"instanceId" binding:"required"`
}

func (ic *InstanceController) StartInstance(c *gin.Context) {
	var req InstanceActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.instanceService.StartInstance(req.UserId, req.InstanceId); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "实例启动成功"))
}

func (ic *InstanceController) StopInstance(c *gin.Context) {
	var req InstanceActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.instanceService.StopInstance(req.UserId, req.InstanceId); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "实例停止成功"))
}

func (ic *InstanceController) RebootInstance(c *gin.Context) {
	var req InstanceActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.instanceService.RebootInstance(req.UserId, req.InstanceId); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "实例重启成功"))
}

func (ic *InstanceController) TerminateInstance(c *gin.Context) {
	var req InstanceActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.instanceService.TerminateInstance(req.UserId, req.InstanceId); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "实例删除成功"))
}

type UpdateInstanceNameRequest struct {
	UserId      string `json:"userId" binding:"required"`
	InstanceId  string `json:"instanceId" binding:"required"`
	DisplayName string `json:"displayName" binding:"required"`
}

func (ic *InstanceController) UpdateInstanceName(c *gin.Context) {
	var req UpdateInstanceNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.instanceService.UpdateInstanceName(req.UserId, req.InstanceId, req.DisplayName); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "实例名称更新成功"))
}

type ChangeIPRequest struct {
	UserId     string `json:"userId" binding:"required"`
	InstanceId string `json:"instanceId" binding:"required"`
}

func (ic *InstanceController) ChangePublicIP(c *gin.Context) {
	var req ChangeIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	newIP, err := ic.instanceService.ChangePublicIP(req.UserId, req.InstanceId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(map[string]string{"newIP": newIP}, "IP更改成功"))
}

type UpdateInstanceConfigRequest struct {
	UserId      string  `json:"userId" binding:"required"`
	InstanceId  string  `json:"instanceId" binding:"required"`
	Ocpus       float32 `json:"ocpus" binding:"required,gt=0"`
	MemoryInGBs float32 `json:"memoryInGBs" binding:"required,gt=0"`
	AutoRestart bool    `json:"autoRestart"` // 是否自动重启实例，默认false
}

func (ic *InstanceController) UpdateInstanceConfig(c *gin.Context) {
	var req UpdateInstanceConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.instanceService.UpdateInstanceConfig(req.UserId, req.InstanceId, req.Ocpus, req.MemoryInGBs, req.AutoRestart); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	msg := "实例配置更新成功"
	if req.AutoRestart {
		msg = "实例配置更新成功，正在重启实例"
	}
	c.JSON(http.StatusOK, models.SuccessResponse(nil, msg))
}

type UpdateBootVolumeRequest struct {
	UserId     string `json:"userId" binding:"required"`
	InstanceId string `json:"instanceId" binding:"required"`
	SizeInGBs  int64  `json:"sizeInGBs" binding:"required,gt=0"`
	VpusPerGB  int64  `json:"vpusPerGB" binding:"required,gt=0"`
}

func (ic *InstanceController) UpdateBootVolume(c *gin.Context) {
	var req UpdateBootVolumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.instanceService.UpdateBootVolumeConfig(req.UserId, req.InstanceId, req.SizeInGBs, req.VpusPerGB); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "引导卷配置更新成功"))
}

type UpdateBootVolumeByIdRequest struct {
	UserId       string `json:"userId" binding:"required"`
	BootVolumeId string `json:"bootVolumeId" binding:"required"`
	SizeInGBs    int64  `json:"sizeInGBs" binding:"required,gt=0"`
	VpusPerGB    int64  `json:"vpusPerGB" binding:"required,gt=0"`
}

func (ic *InstanceController) UpdateBootVolumeById(c *gin.Context) {
	var req UpdateBootVolumeByIdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.instanceService.UpdateBootVolumeById(req.UserId, req.BootVolumeId, req.SizeInGBs, req.VpusPerGB); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "引导卷配置更新成功"))
}

type CreateCloudShellRequest struct {
	UserId     string `json:"userId" binding:"required"`
	InstanceId string `json:"instanceId" binding:"required"`
	PublicKey  string `json:"publicKey" binding:"required"`
}

func (ic *InstanceController) CreateCloudShell(c *gin.Context) {
	var req CreateCloudShellRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	result, err := ic.instanceService.CreateCloudShellConnection(req.UserId, req.InstanceId, req.PublicKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(result, "Cloud Shell连接创建成功"))
}

type AttachIPv6Request struct {
	UserId     string `json:"userId" binding:"required"`
	InstanceId string `json:"instanceId" binding:"required"`
}

func (ic *InstanceController) AttachIPv6(c *gin.Context) {
	var req AttachIPv6Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	ipv6Address, err := ic.instanceService.AttachIPv6(req.UserId, req.InstanceId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(map[string]string{"ipv6": ipv6Address}, "IPv6附加成功"))
}

type AutoRescueRequest struct {
	UserId       string `json:"userId" binding:"required"`
	InstanceId   string `json:"instanceId" binding:"required"`
	InstanceName string `json:"instanceName"`
	KeepBackup   bool   `json:"keepBackup"`
}

func (ic *InstanceController) AutoRescue(c *gin.Context) {
	var req AutoRescueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	// 异步执行救援任务
	util.Go("AutoRescue", func() {
		progressChan := make(chan services.AutoRescueProgress, 10)
		util.Go("AutoRescue.progress", func() {
			for progress := range progressChan {
				ic.logService.SendInfo(fmt.Sprintf("AutoRescue [%s] Step %d/%d: %s", req.InstanceId, progress.Step, progress.TotalSteps, progress.Message))
			}
		})

		err := ic.instanceService.AutoRescue(req.UserId, req.InstanceId, req.InstanceName, req.KeepBackup, progressChan)
		close(progressChan)
		if err != nil {
			log.Printf("AutoRescue failed for instance %s: %v", req.InstanceId, err)
			ic.logService.SendError(fmt.Sprintf("AutoRescue failed for instance %s: %v", req.InstanceId, err))
		} else {
			ic.logService.SendSuccess(fmt.Sprintf("AutoRescue completed for instance %s", req.InstanceId))
		}
	})

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "自动救援任务已启动，请等待完成"))
}

// Enable500MbpsRequest 一键开启500Mbps请求（简化版，仅需要userId和instanceId）
type Enable500MbpsRequest struct {
	UserId     string `json:"userId" binding:"required"`
	InstanceId string `json:"instanceId" binding:"required"`
}

// Enable500Mbps 一键开启下行500Mbps
// 警告：此操作仅支持 VM.Standard.E2.1.Micro (AMD) 实例
// 操作会自动：1. 创建NAT网关 2. 创建网络负载均衡器 3. 配置路由表 4. 放行安全规则
// 开启后实例原公网IP将失效，请使用新分配的负载均衡器IP访问
func (ic *InstanceController) Enable500Mbps(c *gin.Context) {
	var req Enable500MbpsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	// 使用默认SSH端口22
	sshPort := 22

	// 异步执行
	util.Go("Enable500Mbps", func() {
		publicIP, err := ic.instanceService.Enable500Mbps(req.UserId, req.InstanceId, sshPort)
		if err != nil {
			log.Printf("Enable500Mbps failed for instance %s: %v", req.InstanceId, err)
			ic.logService.SendError(fmt.Sprintf("Enable500Mbps failed for instance %s: %v", req.InstanceId, err))
		} else {
			log.Printf("Enable500Mbps succeeded for instance %s, new IP: %s", req.InstanceId, publicIP)
			ic.logService.SendSuccess(fmt.Sprintf("Enable500Mbps succeeded for instance %s, new IP: %s", req.InstanceId, publicIP))
		}
	})

	c.JSON(http.StatusOK, models.SuccessResponse(map[string]interface{}{
		"warning": "开启后实例原公网IP将失效，请使用新分配的负载均衡器IP访问。此操作仅支持 VM.Standard.E2.1.Micro 实例。",
	}, "500Mbps开启任务已启动，正在创建NAT网关和网络负载均衡器，请稍候..."))
}

// Disable500MbpsRequest 一键关闭500Mbps请求（简化版，仅需要userId和instanceId）
type Disable500MbpsRequest struct {
	UserId     string `json:"userId" binding:"required"`
	InstanceId string `json:"instanceId" binding:"required"`
}

// Disable500Mbps 一键关闭下行500Mbps
// 警告：此操作会删除NAT网关和网络负载均衡器
// 关闭后需要重新为实例分配公网IP才能访问
func (ic *InstanceController) Disable500Mbps(c *gin.Context) {
	var req Disable500MbpsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	// 默认清理所有资源（NAT网关和网络负载均衡器）
	retainNatGw := false
	retainNlb := false

	// 异步执行
	util.Go("Disable500Mbps", func() {
		err := ic.instanceService.Disable500Mbps(req.UserId, req.InstanceId, retainNatGw, retainNlb)
		if err != nil {
			log.Printf("Disable500Mbps failed for instance %s: %v", req.InstanceId, err)
			ic.logService.SendError(fmt.Sprintf("Disable500Mbps failed for instance %s: %v", req.InstanceId, err))
		} else {
			ic.logService.SendSuccess(fmt.Sprintf("Disable500Mbps completed for instance %s", req.InstanceId))
		}
	})

	c.JSON(http.StatusOK, models.SuccessResponse(map[string]interface{}{
		"warning": "关闭后NAT网关和网络负载均衡器将被删除，实例将失去公网访问能力，需要重新分配公网IP。",
	}, "500Mbps关闭任务已启动，正在清理NAT网关和网络负载均衡器，请稍候..."))
}
