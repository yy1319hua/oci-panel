package services

import (
	"context"
	"fmt"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/core"
)

type IpService struct {
	ociService *OCIService
}

func NewIpService(ociService *OCIService) *IpService {
	return &IpService{ociService: ociService}
}

func (s *IpService) GetVnicAttachments(userId string, compartmentId string, instanceId string) ([]core.VnicAttachment, error) {
	var user models.OciUser
	if err := database.GetDB().Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	client, err := s.ociService.GetComputeClient(&user)
	if err != nil {
		return nil, err
	}

	req := core.ListVnicAttachmentsRequest{
		CompartmentId: &compartmentId,
		InstanceId:    &instanceId,
	}

	resp, err := client.ListVnicAttachments(context.Background(), req)
	if err != nil {
		return nil, err
	}

	return resp.Items, nil
}

func (s *IpService) GetVnic(userId string, vnicId string) (*core.Vnic, error) {
	var user models.OciUser
	if err := database.GetDB().Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	client, err := s.ociService.GetVirtualNetworkClient(&user)
	if err != nil {
		return nil, err
	}

	req := core.GetVnicRequest{
		VnicId: &vnicId,
	}

	resp, err := client.GetVnic(context.Background(), req)
	if err != nil {
		return nil, err
	}

	return &resp.Vnic, nil
}

func (s *IpService) ChangePublicIp(userId string, instanceId string, compartmentId string) (string, error) {
	var user models.OciUser
	if err := database.GetDB().Where("id = ?", userId).First(&user).Error; err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}

	vnicAttachments, err := s.GetVnicAttachments(userId, compartmentId, instanceId)
	if err != nil {
		return "", err
	}

	if len(vnicAttachments) == 0 {
		return "", fmt.Errorf("no vnic attachments found")
	}

	vnicId := vnicAttachments[0].VnicId
	if vnicId == nil {
		return "", fmt.Errorf("vnic id is nil")
	}

	ctx := context.Background()

	// 使用OCIService的ChangePublicIP方法（已修复使用正确的PrivateIpId）
	newIp, err := s.ociService.ChangePublicIP(ctx, &user, *vnicId)
	if err != nil {
		return "", fmt.Errorf("failed to change public ip: %w", err)
	}

	return newIp, nil
}

func (s *IpService) AttachIpv6(userId string, vnicId string, ipv6SubnetCidr string) error {
	var user models.OciUser
	if err := database.GetDB().Where("id = ?", userId).First(&user).Error; err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	networkClient, err := s.ociService.GetVirtualNetworkClient(&user)
	if err != nil {
		return err
	}

	req := core.CreateIpv6Request{
		CreateIpv6Details: core.CreateIpv6Details{
			VnicId:         &vnicId,
			Ipv6SubnetCidr: &ipv6SubnetCidr,
		},
	}

	_, err = networkClient.CreateIpv6(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to attach ipv6: %w", err)
	}

	return nil
}
