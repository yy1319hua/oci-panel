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
		ingressRules = append(ingressRules, buildIngressRule(rule))
	} else {
		egressRules = append(egressRules, buildEgressRule(rule))
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

// buildIngressRule 由模型构造一条 OCI 入站规则（供新增/修改复用）。
func buildIngressRule(rule *models.SecurityRule) core.IngressSecurityRule {
	r := core.IngressSecurityRule{
		Protocol:    &rule.Protocol,
		Source:      &rule.Source,
		IsStateless: &rule.IsStateless,
	}
	if rule.Description != "" {
		r.Description = &rule.Description
	}
	switch rule.Protocol {
	case "6":
		if rule.PortRangeMin > 0 || rule.PortRangeMax > 0 {
			r.TcpOptions = &core.TcpOptions{DestinationPortRange: &core.PortRange{Min: &rule.PortRangeMin, Max: &rule.PortRangeMax}}
		}
	case "17":
		if rule.PortRangeMin > 0 || rule.PortRangeMax > 0 {
			r.UdpOptions = &core.UdpOptions{DestinationPortRange: &core.PortRange{Min: &rule.PortRangeMin, Max: &rule.PortRangeMax}}
		}
	case "1":
		if rule.IcmpType != nil {
			r.IcmpOptions = &core.IcmpOptions{Type: rule.IcmpType, Code: rule.IcmpCode}
		}
	}
	return r
}

// buildEgressRule 由模型构造一条 OCI 出站规则（供新增/修改复用）。
func buildEgressRule(rule *models.SecurityRule) core.EgressSecurityRule {
	r := core.EgressSecurityRule{
		Protocol:    &rule.Protocol,
		Destination: &rule.Destination,
		IsStateless: &rule.IsStateless,
	}
	if rule.Description != "" {
		r.Description = &rule.Description
	}
	switch rule.Protocol {
	case "6":
		if rule.PortRangeMin > 0 || rule.PortRangeMax > 0 {
			r.TcpOptions = &core.TcpOptions{DestinationPortRange: &core.PortRange{Min: &rule.PortRangeMin, Max: &rule.PortRangeMax}}
		}
	case "17":
		if rule.PortRangeMin > 0 || rule.PortRangeMax > 0 {
			r.UdpOptions = &core.UdpOptions{DestinationPortRange: &core.PortRange{Min: &rule.PortRangeMin, Max: &rule.PortRangeMax}}
		}
	case "1":
		if rule.IcmpType != nil {
			r.IcmpOptions = &core.IcmpOptions{Type: rule.IcmpType, Code: rule.IcmpCode}
		}
	}
	return r
}

// matchIngressRule 判断现有入站规则是否与「定位键」匹配（协议+来源+端口范围）。
func matchIngressRule(r core.IngressSecurityRule, key *models.SecurityRule) bool {
	if r.Protocol == nil || *r.Protocol != key.Protocol {
		return false
	}
	if r.Source == nil || *r.Source != key.Source {
		return false
	}
	pmin, pmax := 0, 0
	if r.TcpOptions != nil && r.TcpOptions.DestinationPortRange != nil {
		if r.TcpOptions.DestinationPortRange.Min != nil {
			pmin = *r.TcpOptions.DestinationPortRange.Min
		}
		if r.TcpOptions.DestinationPortRange.Max != nil {
			pmax = *r.TcpOptions.DestinationPortRange.Max
		}
	}
	if r.UdpOptions != nil && r.UdpOptions.DestinationPortRange != nil {
		if r.UdpOptions.DestinationPortRange.Min != nil {
			pmin = *r.UdpOptions.DestinationPortRange.Min
		}
		if r.UdpOptions.DestinationPortRange.Max != nil {
			pmax = *r.UdpOptions.DestinationPortRange.Max
		}
	}
	return pmin == key.PortRangeMin && pmax == key.PortRangeMax
}

// matchEgressRule 判断现有出站规则是否与「定位键」匹配（协议+目标+端口范围）。
func matchEgressRule(r core.EgressSecurityRule, key *models.SecurityRule) bool {
	if r.Protocol == nil || *r.Protocol != key.Protocol {
		return false
	}
	if r.Destination == nil || *r.Destination != key.Destination {
		return false
	}
	pmin, pmax := 0, 0
	if r.TcpOptions != nil && r.TcpOptions.DestinationPortRange != nil {
		if r.TcpOptions.DestinationPortRange.Min != nil {
			pmin = *r.TcpOptions.DestinationPortRange.Min
		}
		if r.TcpOptions.DestinationPortRange.Max != nil {
			pmax = *r.TcpOptions.DestinationPortRange.Max
		}
	}
	if r.UdpOptions != nil && r.UdpOptions.DestinationPortRange != nil {
		if r.UdpOptions.DestinationPortRange.Min != nil {
			pmin = *r.UdpOptions.DestinationPortRange.Min
		}
		if r.UdpOptions.DestinationPortRange.Max != nil {
			pmax = *r.UdpOptions.DestinationPortRange.Max
		}
	}
	return pmin == key.PortRangeMin && pmax == key.PortRangeMax
}

// ModifySecurityRule 修改一条安全规则：用 oldRule 作为定位键找到规则，替换为 newRule。
// OCI 的 UpdateSecurityList 为全量替换且不返回规则 ID，故采用「读→定位→替换→写」。
func (s *OCIService) ModifySecurityRule(ctx context.Context, user *models.OciUser, vcnId string, isIngress bool, oldRule, newRule *models.SecurityRule) error {
	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return fmt.Errorf("failed to get virtual network client: %w", err)
	}

	vcnResp, err := vnClient.GetVcn(ctx, core.GetVcnRequest{VcnId: &vcnId})
	if err != nil {
		return fmt.Errorf("failed to get VCN: %w", err)
	}

	secListResp, err := vnClient.GetSecurityList(ctx, core.GetSecurityListRequest{
		SecurityListId: vcnResp.DefaultSecurityListId,
	})
	if err != nil {
		return fmt.Errorf("failed to get security list: %w", err)
	}

	ingressRules := secListResp.IngressSecurityRules
	egressRules := secListResp.EgressSecurityRules
	found := false

	if isIngress {
		for i, r := range ingressRules {
			if matchIngressRule(r, oldRule) {
				ingressRules[i] = buildIngressRule(newRule)
				found = true
				break
			}
		}
	} else {
		for i, r := range egressRules {
			if matchEgressRule(r, oldRule) {
				egressRules[i] = buildEgressRule(newRule)
				found = true
				break
			}
		}
	}

	if !found {
		return fmt.Errorf("未找到要修改的安全规则（请确认协议/来源或目标/端口是否正确）")
	}

	_, err = vnClient.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
		SecurityListId: vcnResp.DefaultSecurityListId,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: ingressRules,
			EgressSecurityRules:  egressRules,
		},
	})
	return err
}

// DeleteSecurityRule 删除一条安全规则：用 rule 作为定位键找到并移除。
func (s *OCIService) DeleteSecurityRule(ctx context.Context, user *models.OciUser, vcnId string, isIngress bool, rule *models.SecurityRule) error {
	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return fmt.Errorf("failed to get virtual network client: %w", err)
	}

	vcnResp, err := vnClient.GetVcn(ctx, core.GetVcnRequest{VcnId: &vcnId})
	if err != nil {
		return fmt.Errorf("failed to get VCN: %w", err)
	}

	secListResp, err := vnClient.GetSecurityList(ctx, core.GetSecurityListRequest{
		SecurityListId: vcnResp.DefaultSecurityListId,
	})
	if err != nil {
		return fmt.Errorf("failed to get security list: %w", err)
	}

	found := false
	if isIngress {
		kept := make([]core.IngressSecurityRule, 0, len(secListResp.IngressSecurityRules))
		for _, r := range secListResp.IngressSecurityRules {
			if !found && matchIngressRule(r, rule) {
				found = true
				continue // 跳过（删除）
			}
			kept = append(kept, r)
		}
		secListResp.IngressSecurityRules = kept
	} else {
		kept := make([]core.EgressSecurityRule, 0, len(secListResp.EgressSecurityRules))
		for _, r := range secListResp.EgressSecurityRules {
			if !found && matchEgressRule(r, rule) {
				found = true
				continue
			}
			kept = append(kept, r)
		}
		secListResp.EgressSecurityRules = kept
	}

	if !found {
		return fmt.Errorf("未找到要删除的安全规则")
	}

	_, err = vnClient.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
		SecurityListId: vcnResp.DefaultSecurityListId,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: secListResp.IngressSecurityRules,
			EgressSecurityRules:  secListResp.EgressSecurityRules,
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
