package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"
)

// oci-panel 为受管网络负载均衡器打的 FreeformTag 标记。
// 删除操作只针对带这些标记、且实例标签匹配的 NLB，绝不误删租户内无关的负载均衡器。
const (
	nlbManagedTagKey     = "oci-panel-managed"
	nlbManagedTagValue   = "true"
	nlbInstanceTagKey    = "oci-panel-instance"
	nlbManagedNamePrefix = "oci-panel-nlb-"
)

// managedNlbFreeformTags 返回为指定实例创建 NLB 时应附加的 FreeformTags。
func managedNlbFreeformTags(instanceID string) map[string]string {
	return map[string]string{
		nlbManagedTagKey:  nlbManagedTagValue,
		nlbInstanceTagKey: instanceID,
	}
}

// isManagedNlbForInstance 判断某个网络负载均衡器是否由 oci-panel 为指定实例创建。
// 只有返回 true 的 NLB 才允许被自动删除。判定严格基于 FreeformTags：
// 必须同时满足「受管标记为 true」且「实例标签等于该实例」，从而即使同一租户/compartment
// 内存在其他实例的受管 NLB 或用户自建 NLB，也不会被误删。
func isManagedNlbForInstance(freeformTags map[string]string, instanceID string) bool {
	if freeformTags == nil {
		return false
	}
	if freeformTags[nlbManagedTagKey] != nlbManagedTagValue {
		return false
	}
	return freeformTags[nlbInstanceTagKey] == instanceID && instanceID != ""
}

// Enable500Mbps 一键开启下行500Mbps
func (s *OCIService) Enable500Mbps(user *models.OciUser, instanceID string, sshPort int) (string, error) {
	ctx := context.Background()

	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return "", fmt.Errorf("failed to get virtual network client: %w", err)
	}

	nlbClient, err := s.GetNetworkLoadBalancerClient(user)
	if err != nil {
		return "", fmt.Errorf("failed to get network load balancer client: %w", err)
	}

	// 获取实例信息
	instance, err := s.GetInstanceById(user, instanceID)
	if err != nil {
		return "", fmt.Errorf("failed to get instance: %w", err)
	}

	// 检查是否为AMD实例
	if !strings.Contains(*instance.Shape, "E2.1.Micro") {
		return "", fmt.Errorf("only AMD E2.1.Micro instances support 500Mbps")
	}

	// 获取VCN
	vcn, err := s.GetVcnByInstanceId(user, instanceID)
	if err != nil {
		return "", fmt.Errorf("failed to get VCN: %w", err)
	}

	// 获取VNIC
	vnic, err := s.GetVnicByInstanceId(user, instanceID)
	if err != nil {
		return "", fmt.Errorf("failed to get VNIC: %w", err)
	}

	// 获取私有IP
	privateIpResp, err := vnClient.ListPrivateIps(ctx, core.ListPrivateIpsRequest{VnicId: vnic.Id})
	if err != nil || len(privateIpResp.Items) == 0 {
		return "", fmt.Errorf("failed to get private IP: %w", err)
	}
	privateIP := *privateIpResp.Items[0].IpAddress

	compartmentID := *instance.CompartmentId

	// 创建或获取NAT网关
	natGatewayResp, err := vnClient.ListNatGateways(ctx, core.ListNatGatewaysRequest{
		CompartmentId:  &compartmentID,
		VcnId:          vcn.Id,
		LifecycleState: core.NatGatewayLifecycleStateAvailable,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list NAT gateways: %w", err)
	}

	var natGatewayId *string
	if len(natGatewayResp.Items) > 0 {
		natGatewayId = natGatewayResp.Items[0].Id
	} else {
		// 创建NAT网关
		natName := "nat-gateway"
		createNatResp, err := vnClient.CreateNatGateway(ctx, core.CreateNatGatewayRequest{
			CreateNatGatewayDetails: core.CreateNatGatewayDetails{
				CompartmentId: &compartmentID,
				VcnId:         vcn.Id,
				DisplayName:   &natName,
			},
		})
		if err != nil {
			return "", fmt.Errorf("failed to create NAT gateway: %w", err)
		}
		natGatewayId = createNatResp.Id

		// 等待NAT网关可用
		timeoutCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel1()
		for {
			select {
			case <-timeoutCtx1.Done():
				return "", fmt.Errorf("timeout waiting for NAT gateway to become available")
			default:
			}
			natResp, err := vnClient.GetNatGateway(timeoutCtx1, core.GetNatGatewayRequest{NatGatewayId: natGatewayId})
			if err != nil {
				return "", fmt.Errorf("failed to get NAT gateway status: %w", err)
			}
			if natResp.LifecycleState == core.NatGatewayLifecycleStateAvailable {
				break
			}
			time.Sleep(2 * time.Second)
		}
	}

	// 获取子网
	subnetResp, err := vnClient.ListSubnets(ctx, core.ListSubnetsRequest{
		CompartmentId: &compartmentID,
		VcnId:         vcn.Id,
	})
	if err != nil || len(subnetResp.Items) == 0 {
		return "", fmt.Errorf("failed to list subnets: %w", err)
	}
	subnetId := subnetResp.Items[0].Id

	// 删除此前由 oci-panel 为「本实例」创建的旧 NLB（按受管标签过滤），
	// 不再像以往那样删除 compartment 内所有 active NLB（会误删其他实例/用户自建的负载均衡器）。
	existingNlbResp, err := nlbClient.ListNetworkLoadBalancers(ctx, networkloadbalancer.ListNetworkLoadBalancersRequest{
		CompartmentId:  &compartmentID,
		LifecycleState: networkloadbalancer.ListNetworkLoadBalancersLifecycleStateActive,
	})
	if err == nil && existingNlbResp.NetworkLoadBalancerCollection.Items != nil {
		deletedAny := false
		for _, nlb := range existingNlbResp.NetworkLoadBalancerCollection.Items {
			if !isManagedNlbForInstance(nlb.FreeformTags, instanceID) {
				continue
			}
			_, _ = nlbClient.DeleteNetworkLoadBalancer(ctx, networkloadbalancer.DeleteNetworkLoadBalancerRequest{
				NetworkLoadBalancerId: nlb.Id,
			})
			deletedAny = true
		}
		if deletedAny {
			time.Sleep(5 * time.Second)
		}
	}

	// 创建网络负载均衡器（带 oci-panel 受管标签 + 可识别名称前缀，便于后续精确清理）
	nlbName := fmt.Sprintf("%s%s", nlbManagedNamePrefix, time.Now().Format("20060102150405"))
	isPrivate := false
	port := 0
	weight := 1
	isPreserveSource := true
	isFailOpen := true

	createNlbResp, err := nlbClient.CreateNetworkLoadBalancer(ctx, networkloadbalancer.CreateNetworkLoadBalancerRequest{
		CreateNetworkLoadBalancerDetails: networkloadbalancer.CreateNetworkLoadBalancerDetails{
			CompartmentId: &compartmentID,
			DisplayName:   &nlbName,
			SubnetId:      subnetId,
			IsPrivate:     &isPrivate,
			FreeformTags:  managedNlbFreeformTags(instanceID),
			Listeners: map[string]networkloadbalancer.ListenerDetails{
				"listener1": {
					Name:                  stringPtr("listener1"),
					DefaultBackendSetName: stringPtr("backend1"),
					Protocol:              networkloadbalancer.ListenerProtocolsTcpAndUdp,
					Port:                  &port,
				},
			},
			BackendSets: map[string]networkloadbalancer.BackendSetDetails{
				"backend1": {
					Policy:           networkloadbalancer.NetworkLoadBalancingPolicyTwoTuple,
					IsPreserveSource: &isPreserveSource,
					IsFailOpen:       &isFailOpen,
					HealthChecker: &networkloadbalancer.HealthChecker{
						Protocol: networkloadbalancer.HealthCheckProtocolsTcp,
						Port:     &sshPort,
					},
					Backends: []networkloadbalancer.Backend{
						{
							IpAddress: &privateIP,
							TargetId:  instance.Id,
							Port:      &port,
							Weight:    &weight,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create network load balancer: %w", err)
	}

	nlbId := createNlbResp.Id

	// 等待NLB可用
	timeoutCtx2, cancel2 := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel2()
	for {
		select {
		case <-timeoutCtx2.Done():
			return "", fmt.Errorf("timeout waiting for network load balancer to become active")
		default:
		}
		nlbResp, err := nlbClient.GetNetworkLoadBalancer(timeoutCtx2, networkloadbalancer.GetNetworkLoadBalancerRequest{
			NetworkLoadBalancerId: nlbId,
		})
		if err != nil {
			return "", fmt.Errorf("failed to get NLB status: %w", err)
		}
		if nlbResp.LifecycleState == networkloadbalancer.LifecycleStateActive {
			break
		}
		time.Sleep(3 * time.Second)
	}

	// 获取NLB公网IP
	nlbResp, err := nlbClient.GetNetworkLoadBalancer(ctx, networkloadbalancer.GetNetworkLoadBalancerRequest{
		NetworkLoadBalancerId: nlbId,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get NLB: %w", err)
	}

	var publicIP string
	for _, ip := range nlbResp.IpAddresses {
		if ip.IpAddress != nil && !isPrivateIP(*ip.IpAddress) {
			publicIP = *ip.IpAddress
			break
		}
	}

	// 创建或更新NAT路由表
	routeTableResp, err := vnClient.ListRouteTables(ctx, core.ListRouteTablesRequest{
		CompartmentId:  &compartmentID,
		VcnId:          vcn.Id,
		LifecycleState: core.RouteTableLifecycleStateAvailable,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list route tables: %w", err)
	}

	var natRouteTableId *string
	for _, rt := range routeTableResp.Items {
		for _, rule := range rt.RouteRules {
			if rule.NetworkEntityId != nil && *rule.NetworkEntityId == *natGatewayId &&
				rule.Destination != nil && *rule.Destination == "0.0.0.0/0" {
				natRouteTableId = rt.Id
				break
			}
		}
		if natRouteTableId != nil {
			break
		}
	}

	if natRouteTableId == nil {
		// 创建新路由表
		rtName := "nat-route"
		destination := "0.0.0.0/0"
		createRtResp, err := vnClient.CreateRouteTable(ctx, core.CreateRouteTableRequest{
			CreateRouteTableDetails: core.CreateRouteTableDetails{
				CompartmentId: &compartmentID,
				VcnId:         vcn.Id,
				DisplayName:   &rtName,
				RouteRules: []core.RouteRule{
					{
						Destination:     &destination,
						NetworkEntityId: natGatewayId,
						DestinationType: core.RouteRuleDestinationTypeCidrBlock,
					},
				},
			},
		})
		if err != nil {
			return "", fmt.Errorf("failed to create route table: %w", err)
		}
		natRouteTableId = createRtResp.Id

		// 等待路由表可用
		timeoutCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel3()
		for {
			select {
			case <-timeoutCtx3.Done():
				return "", fmt.Errorf("timeout waiting for route table to become available")
			default:
			}
			rtResp, err := vnClient.GetRouteTable(timeoutCtx3, core.GetRouteTableRequest{RtId: natRouteTableId})
			if err != nil {
				return "", fmt.Errorf("failed to get route table status: %w", err)
			}
			if rtResp.LifecycleState == core.RouteTableLifecycleStateAvailable {
				break
			}
			time.Sleep(2 * time.Second)
		}
	}

	// 更新VNIC绑定路由表并跳过源/目的地检查
	skipSourceDestCheck := true
	_, err = vnClient.UpdateVnic(ctx, core.UpdateVnicRequest{
		VnicId: vnic.Id,
		UpdateVnicDetails: core.UpdateVnicDetails{
			SkipSourceDestCheck: &skipSourceDestCheck,
			RouteTableId:        natRouteTableId,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to update VNIC: %w", err)
	}

	// 放行安全规则
	_ = s.ReleaseSecurityRules(user, *vcn.Id)

	return publicIP, nil
}

// Disable500Mbps 关闭下行500Mbps
func (s *OCIService) Disable500Mbps(user *models.OciUser, instanceID string, retainNatGw, retainNlb bool) error {
	ctx := context.Background()

	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return fmt.Errorf("failed to get virtual network client: %w", err)
	}

	nlbClient, err := s.GetNetworkLoadBalancerClient(user)
	if err != nil {
		return fmt.Errorf("failed to get network load balancer client: %w", err)
	}

	// 获取实例信息
	instance, err := s.GetInstanceById(user, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	// 检查是否为AMD实例
	if !strings.Contains(*instance.Shape, "E2.1.Micro") {
		return fmt.Errorf("only AMD E2.1.Micro instances support this operation")
	}

	// 获取VCN
	vcn, err := s.GetVcnByInstanceId(user, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get VCN: %w", err)
	}

	// 获取VNIC
	vnic, err := s.GetVnicByInstanceId(user, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get VNIC: %w", err)
	}

	compartmentID := *instance.CompartmentId

	// 获取所有路由表
	routeTableResp, err := vnClient.ListRouteTables(ctx, core.ListRouteTablesRequest{
		CompartmentId:  &compartmentID,
		VcnId:          vcn.Id,
		LifecycleState: core.RouteTableLifecycleStateAvailable,
	})
	if err != nil {
		return fmt.Errorf("failed to list route tables: %w", err)
	}

	// 查找默认路由表（不含NAT规则的）
	var defaultRouteTableId *string
	var natRouteTableIds []*string
	for _, rt := range routeTableResp.Items {
		hasNatRule := false
		for _, rule := range rt.RouteRules {
			if rule.Destination != nil && *rule.Destination == "0.0.0.0/0" {
				// 检查是否指向NAT网关
				natGwResp, _ := vnClient.ListNatGateways(ctx, core.ListNatGatewaysRequest{
					CompartmentId:  &compartmentID,
					VcnId:          vcn.Id,
					LifecycleState: core.NatGatewayLifecycleStateAvailable,
				})
				for _, natGw := range natGwResp.Items {
					if rule.NetworkEntityId != nil && *rule.NetworkEntityId == *natGw.Id {
						hasNatRule = true
						break
					}
				}
			}
		}
		if hasNatRule {
			natRouteTableIds = append(natRouteTableIds, rt.Id)
		} else if defaultRouteTableId == nil {
			defaultRouteTableId = rt.Id
		}
	}

	if defaultRouteTableId == nil && len(routeTableResp.Items) > 0 {
		defaultRouteTableId = routeTableResp.Items[0].Id
	}

	// 更新VNIC绑定到默认路由表
	if defaultRouteTableId != nil {
		skipSourceDestCheck := true
		_, err = vnClient.UpdateVnic(ctx, core.UpdateVnicRequest{
			VnicId: vnic.Id,
			UpdateVnicDetails: core.UpdateVnicDetails{
				SkipSourceDestCheck: &skipSourceDestCheck,
				RouteTableId:        defaultRouteTableId,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update VNIC: %w", err)
		}
	}

	// 删除NAT路由表
	if !retainNatGw {
		for _, rtId := range natRouteTableIds {
			// 先清空路由规则
			_, _ = vnClient.UpdateRouteTable(ctx, core.UpdateRouteTableRequest{
				RtId: rtId,
				UpdateRouteTableDetails: core.UpdateRouteTableDetails{
					RouteRules: []core.RouteRule{},
				},
			})
			time.Sleep(2 * time.Second)
			// 删除路由表
			_, _ = vnClient.DeleteRouteTable(ctx, core.DeleteRouteTableRequest{RtId: rtId})
		}

		// 删除NAT网关
		natGwResp, _ := vnClient.ListNatGateways(ctx, core.ListNatGatewaysRequest{
			CompartmentId:  &compartmentID,
			VcnId:          vcn.Id,
			LifecycleState: core.NatGatewayLifecycleStateAvailable,
		})
		for _, natGw := range natGwResp.Items {
			_, _ = vnClient.DeleteNatGateway(ctx, core.DeleteNatGatewayRequest{NatGatewayId: natGw.Id})
		}
	}

	// 删除网络负载均衡器：只删除由 oci-panel 为「本实例」创建的 NLB（按受管标签精确过滤）。
	// 此前实现用仅含 CompartmentId 的 List 然后删除「全部」结果，会误删该 compartment 内
	// 与本功能无关的负载均衡器，属数据丢失级 bug。
	if !retainNlb {
		nlbResp, _ := nlbClient.ListNetworkLoadBalancers(ctx, networkloadbalancer.ListNetworkLoadBalancersRequest{
			CompartmentId: &compartmentID,
		})
		if nlbResp.NetworkLoadBalancerCollection.Items != nil {
			for _, nlb := range nlbResp.NetworkLoadBalancerCollection.Items {
				if !isManagedNlbForInstance(nlb.FreeformTags, instanceID) {
					continue
				}
				_, _ = nlbClient.DeleteNetworkLoadBalancer(ctx, networkloadbalancer.DeleteNetworkLoadBalancerRequest{
					NetworkLoadBalancerId: nlb.Id,
				})
			}
		}
	}

	return nil
}

// ReleaseSecurityRules 放行安全规则
func (s *OCIService) ReleaseSecurityRules(user *models.OciUser, vcnId string) error {
	ctx := context.Background()

	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return fmt.Errorf("failed to get virtual network client: %w", err)
	}

	// 获取VCN
	vcnResp, err := vnClient.GetVcn(ctx, core.GetVcnRequest{VcnId: &vcnId})
	if err != nil {
		return fmt.Errorf("failed to get VCN: %w", err)
	}

	// 获取安全列表
	secListResp, err := vnClient.ListSecurityLists(ctx, core.ListSecurityListsRequest{
		CompartmentId: vcnResp.CompartmentId,
		VcnId:         &vcnId,
	})
	if err != nil {
		return fmt.Errorf("failed to list security lists: %w", err)
	}

	allProtocol := "all"
	ipv4Cidr := "0.0.0.0/0"
	ipv6Cidr := "::/0"
	internalCidr := "10.0.0.0/16"

	for _, secList := range secListResp.Items {
		// 更新入站规则
		ingressRules := []core.IngressSecurityRule{
			{
				Protocol: &allProtocol,
				Source:   &ipv4Cidr,
			},
			{
				Protocol: &allProtocol,
				Source:   &ipv6Cidr,
			},
			{
				Protocol: &allProtocol,
				Source:   &internalCidr,
			},
		}

		// 更新出站规则
		egressRules := []core.EgressSecurityRule{
			{
				Protocol:    &allProtocol,
				Destination: &ipv4Cidr,
			},
			{
				Protocol:    &allProtocol,
				Destination: &ipv6Cidr,
			},
		}

		_, err = vnClient.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
			SecurityListId: secList.Id,
			UpdateSecurityListDetails: core.UpdateSecurityListDetails{
				IngressSecurityRules: ingressRules,
				EgressSecurityRules:  egressRules,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update security list: %w", err)
		}
	}

	return nil
}
