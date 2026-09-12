package services

import (
	"context"
	"fmt"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/core"
)

// ListVCNs 列出虚拟云网络
func (s *OCIService) ListVCNs(ctx context.Context, user *models.OciUser, compartmentId string) ([]models.VCNInfo, error) {
	client, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return nil, err
	}

	// 全量分页：消除只取首页导致的 VCN 截断。
	vcnItems, err := paginate(func(page *string) ([]core.Vcn, *string, error) {
		resp, err := client.ListVcns(ctx, core.ListVcnsRequest{
			CompartmentId: &compartmentId,
			Page:          page,
		})
		if err != nil {
			return nil, nil, err
		}
		return resp.Items, resp.OpcNextPage, nil
	})
	if err != nil {
		return nil, err
	}

	vcns := make([]models.VCNInfo, 0, len(vcnItems))
	for _, vcn := range vcnItems {
		vcnInfo := models.VCNInfo{
			ID:          *vcn.Id,
			DisplayName: *vcn.DisplayName,
			State:       string(vcn.LifecycleState),
			Subnets:     []models.SubnetInfo{},
		}
		// 使用 CidrBlocks 替代已弃用的 CidrBlock
		if len(vcn.CidrBlocks) > 0 {
			vcnInfo.CIDRBlock = vcn.CidrBlocks[0]
		}
		if vcn.TimeCreated != nil {
			vcnInfo.CreateTime = vcn.TimeCreated.Format("2006-01-02 15:04:05")
		}

		// 获取子网列表（全量分页）
		vcnID := vcn.Id
		subnets, _ := paginate(func(page *string) ([]core.Subnet, *string, error) {
			resp, err := client.ListSubnets(ctx, core.ListSubnetsRequest{
				CompartmentId: &compartmentId,
				VcnId:         vcnID,
				Page:          page,
			})
			if err != nil {
				return nil, nil, err
			}
			return resp.Items, resp.OpcNextPage, nil
		})
		for _, subnet := range subnets {
			subnetInfo := models.SubnetInfo{
				ID:    *subnet.Id,
				State: string(subnet.LifecycleState),
			}
			if subnet.DisplayName != nil {
				subnetInfo.DisplayName = *subnet.DisplayName
			}
			// 使用 Ipv4CidrBlocks 替代已弃用的 CidrBlock
			if len(subnet.Ipv4CidrBlocks) > 0 {
				subnetInfo.CIDRBlock = subnet.Ipv4CidrBlocks[0]
			}
			if subnet.AvailabilityDomain != nil {
				subnetInfo.AvailabilityDomain = *subnet.AvailabilityDomain
			}
			if subnet.ProhibitPublicIpOnVnic != nil {
				subnetInfo.IsPublic = !*subnet.ProhibitPublicIpOnVnic
			}
			vcnInfo.Subnets = append(vcnInfo.Subnets, subnetInfo)
		}

		vcns = append(vcns, vcnInfo)
	}

	return vcns, nil
}

// GetPrivateIpIdForVnic 获取VNIC的私有IP的OCID
func (s *OCIService) GetPrivateIpIdForVnic(ctx context.Context, user *models.OciUser, vnicId string) (string, error) {
	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return "", err
	}

	req := core.ListPrivateIpsRequest{
		VnicId: &vnicId,
	}

	resp, err := vnClient.ListPrivateIps(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to list private IPs: %w", err)
	}

	if len(resp.Items) == 0 {
		return "", fmt.Errorf("no private IP found for VNIC: %s", vnicId)
	}

	return *resp.Items[0].Id, nil
}

// ChangePublicIP 更改实例公网IP（参考oci-helper实现）
func (s *OCIService) ChangePublicIP(ctx context.Context, user *models.OciUser, vnicId string) (string, error) {
	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return "", err
	}

	// 获取当前VNIC信息
	vnicReq := core.GetVnicRequest{VnicId: &vnicId}
	vnicResp, err := vnClient.GetVnic(ctx, vnicReq)
	if err != nil {
		return "", fmt.Errorf("failed to get VNIC: %w", err)
	}

	// Step 1: 如果有现有公网IP，先删除
	if vnicResp.PublicIp != nil && *vnicResp.PublicIp != "" {
		// 通过IP地址获取Public IP的OCID
		getPublicIpReq := core.GetPublicIpByIpAddressRequest{
			GetPublicIpByIpAddressDetails: core.GetPublicIpByIpAddressDetails{
				IpAddress: vnicResp.PublicIp,
			},
		}
		getPublicIpResp, err := vnClient.GetPublicIpByIpAddress(ctx, getPublicIpReq)
		if err != nil {
			return "", fmt.Errorf("failed to get public IP by address: %w", err)
		}

		// 删除现有公网IP
		deleteReq := core.DeletePublicIpRequest{PublicIpId: getPublicIpResp.Id}
		_, err = vnClient.DeletePublicIp(ctx, deleteReq)
		if err != nil {
			return "", fmt.Errorf("failed to delete public IP: %w", err)
		}
	}

	// Step 2: 获取VNIC的私有IP的OCID（这是关键，不能使用IP地址字符串）
	privateIpId, err := s.GetPrivateIpIdForVnic(ctx, user, vnicId)
	if err != nil {
		return "", fmt.Errorf("failed to get private IP ID: %w", err)
	}

	// Step 3: 创建新的临时公网IP
	displayName := "publicIp"
	createReq := core.CreatePublicIpRequest{
		CreatePublicIpDetails: core.CreatePublicIpDetails{
			CompartmentId: &user.OciTenantID,
			Lifetime:      core.CreatePublicIpDetailsLifetimeEphemeral,
			DisplayName:   &displayName,
			PrivateIpId:   &privateIpId,
		},
	}
	createResp, err := vnClient.CreatePublicIp(ctx, createReq)
	if err != nil {
		return "", fmt.Errorf("failed to create new public IP: %w", err)
	}

	if createResp.IpAddress == nil {
		return "", fmt.Errorf("new public IP address is nil")
	}

	return *createResp.IpAddress, nil
}

// CreateIpv6ByInstanceId 通过实例ID创建并附加IPv6地址
func (s *OCIService) CreateIpv6ByInstanceId(ctx context.Context, user *models.OciUser, instanceId string) (string, error) {
	computeClient, err := s.GetComputeClient(user)
	if err != nil {
		return "", err
	}

	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return "", err
	}

	// 获取实例信息
	instance, err := s.GetInstance(ctx, user, instanceId)
	if err != nil {
		return "", fmt.Errorf("failed to get instance: %w", err)
	}

	// 获取VNIC附件
	listVnicReq := core.ListVnicAttachmentsRequest{
		CompartmentId: instance.CompartmentId,
		InstanceId:    instance.Id,
	}
	vnicResp, err := computeClient.ListVnicAttachments(ctx, listVnicReq)
	if err != nil {
		return "", fmt.Errorf("failed to list VNIC attachments: %w", err)
	}

	if len(vnicResp.Items) == 0 {
		return "", fmt.Errorf("no VNIC found for instance")
	}

	// 获取第一个VNIC
	vnicId := vnicResp.Items[0].VnicId
	if vnicId == nil {
		return "", fmt.Errorf("VNIC ID is nil")
	}

	// 获取VNIC详情
	vnicReq := core.GetVnicRequest{VnicId: vnicId}
	vnic, err := vnClient.GetVnic(ctx, vnicReq)
	if err != nil {
		return "", fmt.Errorf("failed to get VNIC: %w", err)
	}

	// 检查是否已有IPv6
	ipv6ListReq := core.ListIpv6sRequest{VnicId: vnicId}
	ipv6ListResp, err := vnClient.ListIpv6s(ctx, ipv6ListReq)
	if err == nil && len(ipv6ListResp.Items) > 0 {
		if ipv6ListResp.Items[0].IpAddress != nil {
			return "", fmt.Errorf("instance already has IPv6 address: %s", *ipv6ListResp.Items[0].IpAddress)
		}
	}

	// 获取子网信息
	subnetReq := core.GetSubnetRequest{SubnetId: vnic.SubnetId}
	subnetResp, err := vnClient.GetSubnet(ctx, subnetReq)
	if err != nil {
		return "", fmt.Errorf("failed to get subnet: %w", err)
	}

	// 获取VCN信息
	vcnReq := core.GetVcnRequest{VcnId: subnetResp.VcnId}
	vcnResp, err := vnClient.GetVcn(ctx, vcnReq)
	if err != nil {
		return "", fmt.Errorf("failed to get VCN: %w", err)
	}

	// 检查VCN是否启用了IPv6
	if len(vcnResp.Ipv6CidrBlocks) == 0 {
		return "", fmt.Errorf("VCN does not have IPv6 enabled. Please enable IPv6 on VCN first via Oracle Cloud Console")
	}

	ipv6SubnetCidr := vcnResp.Ipv6CidrBlocks[0]

	// 创建IPv6
	createIpv6Req := core.CreateIpv6Request{
		CreateIpv6Details: core.CreateIpv6Details{
			VnicId:         vnicId,
			Ipv6SubnetCidr: &ipv6SubnetCidr,
		},
	}
	createIpv6Resp, err := vnClient.CreateIpv6(ctx, createIpv6Req)
	if err != nil {
		return "", fmt.Errorf("failed to create IPv6: %w", err)
	}

	if createIpv6Resp.IpAddress == nil {
		return "", fmt.Errorf("IPv6 address is nil")
	}

	return *createIpv6Resp.IpAddress, nil
}

// GetVcnByInstanceId 根据实例ID获取VCN
func (s *OCIService) GetVcnByInstanceId(user *models.OciUser, instanceID string) (*core.Vcn, error) {
	ctx := context.Background()

	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual network client: %w", err)
	}

	vnic, err := s.GetVnicByInstanceId(user, instanceID)
	if err != nil {
		return nil, err
	}

	// 获取子网
	subnetResp, err := vnClient.GetSubnet(ctx, core.GetSubnetRequest{SubnetId: vnic.SubnetId})
	if err != nil {
		return nil, fmt.Errorf("failed to get subnet: %w", err)
	}

	// 获取VCN
	vcnResp, err := vnClient.GetVcn(ctx, core.GetVcnRequest{VcnId: subnetResp.VcnId})
	if err != nil {
		return nil, fmt.Errorf("failed to get VCN: %w", err)
	}

	return &vcnResp.Vcn, nil
}

// GetVnicByInstanceId 根据实例ID获取VNIC
func (s *OCIService) GetVnicByInstanceId(user *models.OciUser, instanceID string) (*core.Vnic, error) {
	ctx := context.Background()

	computeClient, err := s.GetComputeClient(user)
	if err != nil {
		return nil, fmt.Errorf("failed to get compute client: %w", err)
	}

	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual network client: %w", err)
	}

	// 获取实例
	instResp, err := computeClient.GetInstance(ctx, core.GetInstanceRequest{InstanceId: &instanceID})
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	// 获取VNIC附件
	vnicAttachResp, err := computeClient.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
		CompartmentId: instResp.CompartmentId,
		InstanceId:    &instanceID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list VNIC attachments: %w", err)
	}

	if len(vnicAttachResp.Items) == 0 {
		return nil, fmt.Errorf("no VNIC attachment found")
	}

	// 获取VNIC
	vnicResp, err := vnClient.GetVnic(ctx, core.GetVnicRequest{VnicId: vnicAttachResp.Items[0].VnicId})
	if err != nil {
		return nil, fmt.Errorf("failed to get VNIC: %w", err)
	}

	return &vnicResp.Vnic, nil
}
