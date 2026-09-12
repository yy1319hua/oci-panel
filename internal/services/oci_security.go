package services

import (
	"context"
	"fmt"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func (s *OCIService) GetSecurityListByVcnId(ctx context.Context, user *models.OciUser, vcnId string) (*models.SecurityListInfo, error) {
	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual network client: %w", err)
	}

	// 获取VCN
	vcnResp, err := vnClient.GetVcn(ctx, core.GetVcnRequest{VcnId: &vcnId})
	if err != nil {
		return nil, fmt.Errorf("failed to get VCN: %w", err)
	}

	// 获取默认安全列表
	secListResp, err := vnClient.GetSecurityList(ctx, core.GetSecurityListRequest{
		SecurityListId: vcnResp.DefaultSecurityListId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get security list: %w", err)
	}

	secList := secListResp.SecurityList
	result := &models.SecurityListInfo{
		ID:           *secList.Id,
		DisplayName:  *secList.DisplayName,
		VcnId:        vcnId,
		IngressRules: []models.SecurityRule{},
		EgressRules:  []models.SecurityRule{},
	}

	// 解析入站规则
	for _, rule := range secList.IngressSecurityRules {
		sr := models.SecurityRule{
			IsStateless: false,
			Protocol:    *rule.Protocol,
			Source:      *rule.Source,
		}
		if rule.IsStateless != nil {
			sr.IsStateless = *rule.IsStateless
		}
		if rule.Description != nil {
			sr.Description = *rule.Description
		}
		sr.ProtocolName = getProtocolName(*rule.Protocol)
		// TCP端口
		if *rule.Protocol == "6" && rule.TcpOptions != nil {
			if rule.TcpOptions.DestinationPortRange != nil {
				sr.PortRangeMin = *rule.TcpOptions.DestinationPortRange.Min
				sr.PortRangeMax = *rule.TcpOptions.DestinationPortRange.Max
			}
		}
		// UDP端口
		if *rule.Protocol == "17" && rule.UdpOptions != nil {
			if rule.UdpOptions.DestinationPortRange != nil {
				sr.PortRangeMin = *rule.UdpOptions.DestinationPortRange.Min
				sr.PortRangeMax = *rule.UdpOptions.DestinationPortRange.Max
			}
		}
		// ICMP
		if *rule.Protocol == "1" && rule.IcmpOptions != nil {
			sr.IcmpType = rule.IcmpOptions.Type
			sr.IcmpCode = rule.IcmpOptions.Code
		}
		result.IngressRules = append(result.IngressRules, sr)
	}

	// 解析出站规则
	for _, rule := range secList.EgressSecurityRules {
		sr := models.SecurityRule{
			IsStateless: false,
			Protocol:    *rule.Protocol,
			Destination: *rule.Destination,
		}
		if rule.IsStateless != nil {
			sr.IsStateless = *rule.IsStateless
		}
		if rule.Description != nil {
			sr.Description = *rule.Description
		}
		sr.ProtocolName = getProtocolName(*rule.Protocol)
		// TCP端口
		if *rule.Protocol == "6" && rule.TcpOptions != nil {
			if rule.TcpOptions.DestinationPortRange != nil {
				sr.PortRangeMin = *rule.TcpOptions.DestinationPortRange.Min
				sr.PortRangeMax = *rule.TcpOptions.DestinationPortRange.Max
			}
		}
		// UDP端口
		if *rule.Protocol == "17" && rule.UdpOptions != nil {
			if rule.UdpOptions.DestinationPortRange != nil {
				sr.PortRangeMin = *rule.UdpOptions.DestinationPortRange.Min
				sr.PortRangeMax = *rule.UdpOptions.DestinationPortRange.Max
			}
		}
		// ICMP
		if *rule.Protocol == "1" && rule.IcmpOptions != nil {
			sr.IcmpType = rule.IcmpOptions.Type
			sr.IcmpCode = rule.IcmpOptions.Code
		}
		result.EgressRules = append(result.EgressRules, sr)
	}

	return result, nil
}

// getProtocolName 获取协议名称
func getProtocolName(protocol string) string {
	switch protocol {
	case "all":
		return "所有协议"
	case "1":
		return "ICMP"
	case "6":
		return "TCP"
	case "17":
		return "UDP"
	case "58":
		return "ICMPv6"
	default:
		return protocol
	}
}

// AddSecurityRule 添加安全规则
func (s *OCIService) AddSecurityRule(ctx context.Context, user *models.OciUser, vcnId string, rule *models.SecurityRule, isIngress bool) error {
	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return fmt.Errorf("failed to get virtual network client: %w", err)
	}

	// 获取VCN
	vcnResp, err := vnClient.GetVcn(ctx, core.GetVcnRequest{VcnId: &vcnId})
	if err != nil {
		return fmt.Errorf("failed to get VCN: %w", err)
	}

	// 获取当前安全列表
	secListResp, err := vnClient.GetSecurityList(ctx, core.GetSecurityListRequest{
		SecurityListId: vcnResp.DefaultSecurityListId,
	})
	if err != nil {
		return fmt.Errorf("failed to get security list: %w", err)
	}

	ingressRules := secListResp.IngressSecurityRules
	egressRules := secListResp.EgressSecurityRules

	if isIngress {
		newRule := core.IngressSecurityRule{
			Protocol:    &rule.Protocol,
			Source:      &rule.Source,
			IsStateless: &rule.IsStateless,
		}
		if rule.Description != "" {
			newRule.Description = &rule.Description
		}
		// TCP
		if rule.Protocol == "6" && (rule.PortRangeMin > 0 || rule.PortRangeMax > 0) {
			newRule.TcpOptions = &core.TcpOptions{
				DestinationPortRange: &core.PortRange{
					Min: &rule.PortRangeMin,
					Max: &rule.PortRangeMax,
				},
			}
		}
		// UDP
		if rule.Protocol == "17" && (rule.PortRangeMin > 0 || rule.PortRangeMax > 0) {
			newRule.UdpOptions = &core.UdpOptions{
				DestinationPortRange: &core.PortRange{
					Min: &rule.PortRangeMin,
					Max: &rule.PortRangeMax,
				},
			}
		}
		// ICMP
		if rule.Protocol == "1" && rule.IcmpType != nil {
			newRule.IcmpOptions = &core.IcmpOptions{
				Type: rule.IcmpType,
				Code: rule.IcmpCode,
			}
		}
		ingressRules = append(ingressRules, newRule)
	} else {
		newRule := core.EgressSecurityRule{
			Protocol:    &rule.Protocol,
			Destination: &rule.Destination,
			IsStateless: &rule.IsStateless,
		}
		if rule.Description != "" {
			newRule.Description = &rule.Description
		}
		// TCP
		if rule.Protocol == "6" && (rule.PortRangeMin > 0 || rule.PortRangeMax > 0) {
			newRule.TcpOptions = &core.TcpOptions{
				DestinationPortRange: &core.PortRange{
					Min: &rule.PortRangeMin,
					Max: &rule.PortRangeMax,
				},
			}
		}
		// UDP
		if rule.Protocol == "17" && (rule.PortRangeMin > 0 || rule.PortRangeMax > 0) {
			newRule.UdpOptions = &core.UdpOptions{
				DestinationPortRange: &core.PortRange{
					Min: &rule.PortRangeMin,
					Max: &rule.PortRangeMax,
				},
			}
		}
		// ICMP
		if rule.Protocol == "1" && rule.IcmpType != nil {
			newRule.IcmpOptions = &core.IcmpOptions{
				Type: rule.IcmpType,
				Code: rule.IcmpCode,
			}
		}
		egressRules = append(egressRules, newRule)
	}

	// 更新安全列表
	_, err = vnClient.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
		SecurityListId: vcnResp.DefaultSecurityListId,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: ingressRules,
			EgressSecurityRules:  egressRules,
		},
	})
	return err
}

// DeleteVcn 删除VCN及其相关资源
func (s *OCIService) DeleteVcn(ctx context.Context, user *models.OciUser, vcnId string) error {
	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return fmt.Errorf("failed to get virtual network client: %w", err)
	}

	// 获取VCN
	vcnResp, err := vnClient.GetVcn(ctx, core.GetVcnRequest{VcnId: &vcnId})
	if err != nil {
		return fmt.Errorf("failed to get VCN: %w", err)
	}
	vcn := vcnResp.Vcn

	// 1. 清空路由表规则
	if vcn.DefaultRouteTableId != nil {
		_, err = vnClient.UpdateRouteTable(ctx, core.UpdateRouteTableRequest{
			RtId: vcn.DefaultRouteTableId,
			UpdateRouteTableDetails: core.UpdateRouteTableDetails{
				RouteRules: []core.RouteRule{},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to clear route table: %w", err)
		}
	}

	// 2. 删除所有子网
	subnetsResp, err := vnClient.ListSubnets(ctx, core.ListSubnetsRequest{
		CompartmentId: vcn.CompartmentId,
		VcnId:         &vcnId,
	})
	if err == nil {
		for _, subnet := range subnetsResp.Items {
			_, err = vnClient.DeleteSubnet(ctx, core.DeleteSubnetRequest{
				SubnetId: subnet.Id,
			})
			if err != nil {
				return fmt.Errorf("failed to delete subnet %s: %w", *subnet.DisplayName, err)
			}
			// 等待子网删除完成
			time.Sleep(2 * time.Second)
		}
	}

	// 3. 删除Internet网关
	igwsResp, err := vnClient.ListInternetGateways(ctx, core.ListInternetGatewaysRequest{
		CompartmentId: vcn.CompartmentId,
		VcnId:         &vcnId,
	})
	if err == nil {
		for _, igw := range igwsResp.Items {
			_, err = vnClient.DeleteInternetGateway(ctx, core.DeleteInternetGatewayRequest{
				IgId: igw.Id,
			})
			if err != nil {
				return fmt.Errorf("failed to delete internet gateway: %w", err)
			}
		}
	}

	// 4. 删除NAT网关
	natGwsResp, err := vnClient.ListNatGateways(ctx, core.ListNatGatewaysRequest{
		CompartmentId: vcn.CompartmentId,
		VcnId:         &vcnId,
	})
	if err == nil {
		for _, natGw := range natGwsResp.Items {
			_, err = vnClient.DeleteNatGateway(ctx, core.DeleteNatGatewayRequest{
				NatGatewayId: natGw.Id,
			})
			if err != nil {
				return fmt.Errorf("failed to delete NAT gateway: %w", err)
			}
		}
	}

	// 5. 删除服务网关
	sgwsResp, err := vnClient.ListServiceGateways(ctx, core.ListServiceGatewaysRequest{
		CompartmentId: vcn.CompartmentId,
		VcnId:         &vcnId,
	})
	if err == nil {
		for _, sgw := range sgwsResp.Items {
			_, err = vnClient.DeleteServiceGateway(ctx, core.DeleteServiceGatewayRequest{
				ServiceGatewayId: sgw.Id,
			})
			if err != nil {
				return fmt.Errorf("failed to delete service gateway: %w", err)
			}
		}
	}

	// 6. 删除网络安全组
	nsgsResp, err := vnClient.ListNetworkSecurityGroups(ctx, core.ListNetworkSecurityGroupsRequest{
		CompartmentId: vcn.CompartmentId,
		VcnId:         &vcnId,
	})
	if err == nil {
		for _, nsg := range nsgsResp.Items {
			// 先清空安全规则
			_, _ = vnClient.UpdateNetworkSecurityGroupSecurityRules(ctx, core.UpdateNetworkSecurityGroupSecurityRulesRequest{
				NetworkSecurityGroupId: nsg.Id,
				UpdateNetworkSecurityGroupSecurityRulesDetails: core.UpdateNetworkSecurityGroupSecurityRulesDetails{
					SecurityRules: []core.UpdateSecurityRuleDetails{},
				},
			})
			_, err = vnClient.DeleteNetworkSecurityGroup(ctx, core.DeleteNetworkSecurityGroupRequest{
				NetworkSecurityGroupId: nsg.Id,
			})
			if err != nil {
				return fmt.Errorf("failed to delete network security group: %w", err)
			}
		}
	}

	// 7. 删除VCN
	_, err = vnClient.DeleteVcn(ctx, core.DeleteVcnRequest{
		VcnId: &vcnId,
	})
	if err != nil {
		return fmt.Errorf("failed to delete VCN: %w", err)
	}

	return nil
}
