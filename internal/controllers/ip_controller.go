package controllers

import (
	"net/http"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

type IpController struct {
	ipService *services.IpService
}

func NewIpController(ipService *services.IpService) *IpController {
	return &IpController{ipService: ipService}
}

type ChangeIpRequest struct {
	UserId        string `json:"userId" binding:"required"`
	InstanceId    string `json:"instanceId" binding:"required"`
	CompartmentId string `json:"compartmentId" binding:"required"`
}

func (ic *IpController) ChangePublicIp(c *gin.Context) {
	var req ChangeIpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	newIp, err := ic.ipService.ChangePublicIp(req.UserId, req.InstanceId, req.CompartmentId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(map[string]string{
		"newIp": newIp,
	}, "IP更换成功"))
}

type AttachIpv6Request struct {
	UserId         string `json:"userId" binding:"required"`
	VnicId         string `json:"vnicId" binding:"required"`
	Ipv6SubnetCidr string `json:"ipv6SubnetCidr" binding:"required"`
}

func (ic *IpController) AttachIpv6(c *gin.Context) {
	var req AttachIpv6Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := ic.ipService.AttachIpv6(req.UserId, req.VnicId, req.Ipv6SubnetCidr); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "IPv6附加成功"))
}
