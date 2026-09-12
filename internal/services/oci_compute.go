package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

func (s *OCIService) ListInstances(ctx context.Context, user *models.OciUser, compartmentId string) ([]core.Instance, error) {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return nil, err
	}

	// 全量分页：此前只取首页、忽略 OpcNextPage，实例较多时结果被截断。
	return paginate(func(page *string) ([]core.Instance, *string, error) {
		resp, err := client.ListInstances(ctx, core.ListInstancesRequest{
			CompartmentId: &compartmentId,
			Page:          page,
		})
		if err != nil {
			return nil, nil, err
		}
		return resp.Items, resp.OpcNextPage, nil
	})
}

func (s *OCIService) GetInstance(ctx context.Context, user *models.OciUser, instanceId string) (*core.Instance, error) {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return nil, err
	}

	req := core.GetInstanceRequest{
		InstanceId: &instanceId,
	}

	resp, err := client.GetInstance(ctx, req)
	if err != nil {
		return nil, err
	}

	return &resp.Instance, nil
}

func (s *OCIService) InstanceAction(ctx context.Context, user *models.OciUser, instanceId string, action string) error {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return err
	}

	req := core.InstanceActionRequest{
		InstanceId: &instanceId,
		Action:     core.InstanceActionActionEnum(action),
	}

	_, err = client.InstanceAction(ctx, req)
	return err
}

func (s *OCIService) TerminateInstance(ctx context.Context, user *models.OciUser, instanceId string) error {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return err
	}

	req := core.TerminateInstanceRequest{
		InstanceId: &instanceId,
	}

	_, err = client.TerminateInstance(ctx, req)
	return err
}

func (s *OCIService) UpdateInstance(ctx context.Context, user *models.OciUser, instanceId string, displayName string) error {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return err
	}

	req := core.UpdateInstanceRequest{
		InstanceId: &instanceId,
		UpdateInstanceDetails: core.UpdateInstanceDetails{
			DisplayName: &displayName,
		},
	}

	_, err = client.UpdateInstance(ctx, req)
	return err
}

type LaunchInstanceParams struct {
	CompartmentId      string
	AvailabilityDomain string
	DisplayName        string
	ImageId            string
	Shape              string
	SubnetId           string
	Ocpus              float32
	MemoryInGBs        float32
	SshPublicKey       string
	BootVolumeSizeGBs  int64
	BootVolumeVpuPerGB int64
}

func (s *OCIService) LaunchInstance(ctx context.Context, user *models.OciUser, params LaunchInstanceParams) (*core.Instance, error) {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return nil, err
	}

	sourceDetails := core.InstanceSourceViaImageDetails{
		ImageId: &params.ImageId,
	}
	if params.BootVolumeSizeGBs > 0 {
		sourceDetails.BootVolumeSizeInGBs = &params.BootVolumeSizeGBs
	}
	if params.BootVolumeVpuPerGB > 0 {
		sourceDetails.BootVolumeVpusPerGB = &params.BootVolumeVpuPerGB
	}

	req := core.LaunchInstanceRequest{
		LaunchInstanceDetails: core.LaunchInstanceDetails{
			CompartmentId:      &params.CompartmentId,
			AvailabilityDomain: &params.AvailabilityDomain,
			DisplayName:        &params.DisplayName,
			SourceDetails:      &sourceDetails,
			Shape:              &params.Shape,
			CreateVnicDetails: &core.CreateVnicDetails{
				SubnetId: &params.SubnetId,
			},
			ShapeConfig: &core.LaunchInstanceShapeConfigDetails{
				Ocpus:       &params.Ocpus,
				MemoryInGBs: &params.MemoryInGBs,
			},
			Metadata: map[string]string{
				"ssh_authorized_keys": params.SshPublicKey,
			},
		},
	}

	resp, err := client.LaunchInstance(ctx, req)
	if err != nil {
		return nil, err
	}

	return &resp.Instance, nil
}

// CreateInstance creates an instance with auto VCN/subnet setup. Uses region-specific clients for thread safety.
func (s *OCIService) CreateInstance(ctx context.Context, user *models.OciUser, region, architecture, operationSystem string, ocpus, memory float64, disk int, vpusPerGB int64, sshPublicKey string, imageIdParam string) error {
	compartmentId := user.OciTenantID

	identityClient, err := s.GetIdentityClientForRegion(user, region)
	if err != nil {
		return fmt.Errorf("获取身份客户端失败: %w", err)
	}

	adResp, err := identityClient.ListAvailabilityDomains(ctx, identity.ListAvailabilityDomainsRequest{
		CompartmentId: &compartmentId,
	})
	if err != nil {
		return fmt.Errorf("获取可用域失败: %w", err)
	}
	if len(adResp.Items) == 0 {
		return fmt.Errorf("没有可用的可用域")
	}
	availabilityDomain := *adResp.Items[0].Name

	vnClient, err := s.GetVirtualNetworkClientForRegion(user, region)
	if err != nil {
		return fmt.Errorf("获取网络客户端失败: %w", err)
	}

	vcnLifecycleState := core.VcnLifecycleStateAvailable
	vcnResp, err := vnClient.ListVcns(ctx, core.ListVcnsRequest{
		CompartmentId:  &compartmentId,
		LifecycleState: vcnLifecycleState,
	})
	if err != nil {
		return fmt.Errorf("获取VCN列表失败: %w", err)
	}

	var subnetId string
	var targetVcn *core.Vcn
	for i := range vcnResp.Items {
		vcn := &vcnResp.Items[i]
		subnetResp, err := vnClient.ListSubnets(ctx, core.ListSubnetsRequest{
			CompartmentId:  &compartmentId,
			VcnId:          vcn.Id,
			LifecycleState: core.SubnetLifecycleStateAvailable,
		})
		if err != nil {
			continue
		}
		for _, subnet := range subnetResp.Items {
			if subnet.ProhibitPublicIpOnVnic != nil && !*subnet.ProhibitPublicIpOnVnic {
				subnetId = *subnet.Id
				break
			}
		}
		if subnetId != "" {
			break
		}
		if targetVcn == nil {
			targetVcn = vcn
		}
	}

	if subnetId == "" {
		cidrBlock := "10.0.0.0/16"

		if targetVcn == nil {
			vcnName := "oci-panel-vcn"
			isIpv6Enabled := true
			createVcnResp, err := vnClient.CreateVcn(ctx, core.CreateVcnRequest{
				CreateVcnDetails: core.CreateVcnDetails{
					CompartmentId: &compartmentId,
					DisplayName:   &vcnName,
					CidrBlock:     &cidrBlock,
					IsIpv6Enabled: &isIpv6Enabled,
				},
			})
			if err != nil {
				return fmt.Errorf("创建VCN失败: %w", err)
			}
			vcnResp, ok := waitForState(ctx, 30, time.Second,
				func() (core.GetVcnResponse, error) {
					return vnClient.GetVcn(ctx, core.GetVcnRequest{VcnId: createVcnResp.Id})
				},
				func(r core.GetVcnResponse) bool { return r.LifecycleState == core.VcnLifecycleStateAvailable })
			if !ok {
				return fmt.Errorf("等待VCN创建超时")
			}
			targetVcn = &vcnResp.Vcn
		} else {
			if len(targetVcn.CidrBlocks) > 0 {
				cidrBlock = targetVcn.CidrBlocks[0]
			}
		}

		igwResp, err := vnClient.ListInternetGateways(ctx, core.ListInternetGatewaysRequest{
			CompartmentId: &compartmentId,
			VcnId:         targetVcn.Id,
		})
		if err != nil {
			return fmt.Errorf("获取Internet网关列表失败: %w", err)
		}

		var internetGatewayId *string
		if len(igwResp.Items) == 0 {
			igwName := "oci-panel-gateway"
			isEnabled := true
			createIgwResp, err := vnClient.CreateInternetGateway(ctx, core.CreateInternetGatewayRequest{
				CreateInternetGatewayDetails: core.CreateInternetGatewayDetails{
					CompartmentId: &compartmentId,
					VcnId:         targetVcn.Id,
					DisplayName:   &igwName,
					IsEnabled:     &isEnabled,
				},
			})
			if err != nil {
				return fmt.Errorf("创建Internet网关失败: %w", err)
			}
			_, ok := waitForState(ctx, 30, time.Second,
				func() (core.GetInternetGatewayResponse, error) {
					return vnClient.GetInternetGateway(ctx, core.GetInternetGatewayRequest{IgId: createIgwResp.Id})
				},
				func(r core.GetInternetGatewayResponse) bool {
					return r.LifecycleState == core.InternetGatewayLifecycleStateAvailable
				})
			if !ok {
				return fmt.Errorf("等待Internet网关创建超时")
			}
			internetGatewayId = createIgwResp.Id
		} else {
			internetGatewayId = igwResp.Items[0].Id
		}

		if targetVcn.DefaultRouteTableId != nil {
			getRtResp, err := vnClient.GetRouteTable(ctx, core.GetRouteTableRequest{RtId: targetVcn.DefaultRouteTableId})
			if err == nil {
				hasDefaultRoute := false
				for _, rule := range getRtResp.RouteRules {
					if rule.Destination != nil && *rule.Destination == "0.0.0.0/0" {
						hasDefaultRoute = true
						break
					}
				}
				if !hasDefaultRoute {
					destination := "0.0.0.0/0"
					newRule := core.RouteRule{
						Destination:     &destination,
						DestinationType: core.RouteRuleDestinationTypeCidrBlock,
						NetworkEntityId: internetGatewayId,
					}
					updatedRules := append(getRtResp.RouteRules, newRule)
					_, err = vnClient.UpdateRouteTable(ctx, core.UpdateRouteTableRequest{
						RtId: targetVcn.DefaultRouteTableId,
						UpdateRouteTableDetails: core.UpdateRouteTableDetails{
							RouteRules: updatedRules,
						},
					})
					if err != nil {
						return fmt.Errorf("更新路由表失败: %w", err)
					}
				}
			}
		}

		subnetName := "oci-panel-subnet"
		prohibitPublicIp := false
		createSubnetResp, err := vnClient.CreateSubnet(ctx, core.CreateSubnetRequest{
			CreateSubnetDetails: core.CreateSubnetDetails{
				CompartmentId:          &compartmentId,
				VcnId:                  targetVcn.Id,
				DisplayName:            &subnetName,
				CidrBlock:              &cidrBlock,
				RouteTableId:           targetVcn.DefaultRouteTableId,
				ProhibitPublicIpOnVnic: &prohibitPublicIp,
			},
		})
		if err != nil {
			return fmt.Errorf("创建子网失败: %w", err)
		}
		_, ok := waitForState(ctx, 30, time.Second,
			func() (core.GetSubnetResponse, error) {
				return vnClient.GetSubnet(ctx, core.GetSubnetRequest{SubnetId: createSubnetResp.Id})
			},
			func(r core.GetSubnetResponse) bool { return r.LifecycleState == core.SubnetLifecycleStateAvailable })
		if !ok {
			return fmt.Errorf("等待子网创建超时")
		}
		subnetId = *createSubnetResp.Id
	}

	shape := "VM.Standard.A1.Flex"
	if architecture == "AMD" {
		shape = "VM.Standard.E2.1.Micro"
	}

	var imageId string
	if imageIdParam != "" {
		imageId = imageIdParam
	} else {
		computeClient, err := s.GetComputeClientForRegion(user, region)
		if err != nil {
			return fmt.Errorf("获取计算客户端失败: %w", err)
		}

		osName := "Canonical Ubuntu"
		if operationSystem == "CentOS" {
			osName = "CentOS"
		} else if operationSystem == "Oracle Linux" {
			osName = "Oracle Linux"
		}

		imageResp, err := computeClient.ListImages(ctx, core.ListImagesRequest{
			CompartmentId:   &compartmentId,
			OperatingSystem: &osName,
			Shape:           &shape,
			SortBy:          core.ListImagesSortByTimecreated,
			SortOrder:       core.ListImagesSortOrderDesc,
		})
		if err != nil {
			return fmt.Errorf("获取镜像列表失败: %w", err)
		}
		if len(imageResp.Items) == 0 {
			return fmt.Errorf("没有找到合适的镜像")
		}
		imageId = *imageResp.Items[0].Id
	}

	displayName := fmt.Sprintf("instance-%s-%d", architecture, time.Now().Unix())

	params := LaunchInstanceParams{
		CompartmentId:      compartmentId,
		AvailabilityDomain: availabilityDomain,
		DisplayName:        displayName,
		ImageId:            imageId,
		Shape:              shape,
		SubnetId:           subnetId,
		Ocpus:              float32(ocpus),
		MemoryInGBs:        float32(memory),
		SshPublicKey:       sshPublicKey,
		BootVolumeSizeGBs:  int64(disk),
		BootVolumeVpuPerGB: vpusPerGB,
	}

	_, err = s.LaunchInstance(ctx, user, params)
	if err != nil {
		return fmt.Errorf("创建实例失败: %w", err)
	}

	return nil
}

func (s *OCIService) GetInstanceDetails(ctx context.Context, user *models.OciUser, instanceId string) (*models.InstanceInfo, error) {
	computeClient, err := s.GetComputeClient(user)
	if err != nil {
		return nil, err
	}

	instance, err := s.GetInstance(ctx, user, instanceId)
	if err != nil {
		return nil, err
	}

	info := &models.InstanceInfo{
		ID:                 *instance.Id,
		DisplayName:        *instance.DisplayName,
		State:              string(instance.LifecycleState),
		Shape:              *instance.Shape,
		Region:             user.OciRegion,
		AvailabilityDomain: *instance.AvailabilityDomain,
		CreateTime:         instance.TimeCreated.Format("2006-01-02 15:04:05"),
		PublicIPs:          []string{},
		PrivateIPs:         []string{},
		VnicList:           []models.VnicInfo{},
	}

	if instance.ShapeConfig != nil {
		if instance.ShapeConfig.Ocpus != nil {
			info.Ocpus = *instance.ShapeConfig.Ocpus
		}
		if instance.ShapeConfig.MemoryInGBs != nil {
			info.Memory = *instance.ShapeConfig.MemoryInGBs
		}
	}

	// 此前这里是串行的 N+1：GetImage → ListBootVolumeAttachments+GetBootVolume →
	// ListVnicAttachments + 每 VNIC(GetVnic+ListIpv6s)。这三组互不依赖，且每个 VNIC 的查询也互不依赖，
	// 故并发执行。注意：各分支写入的是 info 的「不同字段」（ImageName / BootVolume* / Vnic*），
	// 每个 VNIC 写入 results 的「不同下标」，均为不同内存地址，无数据竞争；
	// VnicList 等切片的最终汇总只在 VNIC 分支内串行进行（单一写者）。
	var wg sync.WaitGroup

	// 分支 1：镜像名
	if instance.SourceDetails != nil {
		if sourceDetails, ok := instance.SourceDetails.(core.InstanceSourceViaImageDetails); ok && sourceDetails.ImageId != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				imageResp, err := computeClient.GetImage(ctx, core.GetImageRequest{ImageId: sourceDetails.ImageId})
				if err == nil && imageResp.DisplayName != nil {
					info.ImageName = *imageResp.DisplayName
				}
			}()
		}
	}

	// 分支 2：引导卷大小/VPU
	wg.Add(1)
	go func() {
		defer wg.Done()
		bootVolumeClient, err := s.GetBlockstorageClient(user)
		if err != nil {
			return
		}
		bootVolumeResp, err := computeClient.ListBootVolumeAttachments(ctx, core.ListBootVolumeAttachmentsRequest{
			CompartmentId:      instance.CompartmentId,
			InstanceId:         instance.Id,
			AvailabilityDomain: instance.AvailabilityDomain,
		})
		if err != nil || len(bootVolumeResp.Items) == 0 {
			return
		}
		bvId := bootVolumeResp.Items[0].BootVolumeId
		if bvId == nil {
			return
		}
		bvResp, err := bootVolumeClient.GetBootVolume(ctx, core.GetBootVolumeRequest{BootVolumeId: bvId})
		if err != nil {
			return
		}
		if bvResp.SizeInGBs != nil {
			info.BootVolumeSize = *bvResp.SizeInGBs
		}
		if bvResp.VpusPerGB != nil {
			info.BootVolumeVpu = *bvResp.VpusPerGB
		}
	}()

	// 分支 3：VNIC 列表（每个 VNIC 的 GetVnic + ListIpv6s 并发，按附件顺序稳定汇总）
	wg.Add(1)
	go func() {
		defer wg.Done()
		vnicResp, err := computeClient.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
			CompartmentId: instance.CompartmentId,
			InstanceId:    instance.Id,
		})
		if err != nil {
			return
		}
		vnClient, err := s.GetVirtualNetworkClient(user)
		if err != nil {
			return
		}

		type vnicResult struct {
			info  models.VnicInfo
			ipv6  string
			valid bool
		}
		results := make([]vnicResult, len(vnicResp.Items))
		var vwg sync.WaitGroup
		for i, vnicAttachment := range vnicResp.Items {
			if vnicAttachment.VnicId == nil {
				continue
			}
			vwg.Add(1)
			go func(idx int, vnicID *string) {
				defer vwg.Done()
				vnic, err := vnClient.GetVnic(ctx, core.GetVnicRequest{VnicId: vnicID})
				if err != nil {
					return
				}
				r := vnicResult{valid: true}
				r.info.VnicID = *vnicID
				if vnic.DisplayName != nil {
					r.info.Name = *vnic.DisplayName
				}
				if vnic.SubnetId != nil {
					r.info.SubnetID = *vnic.SubnetId
				}
				if vnic.PublicIp != nil && *vnic.PublicIp != "" {
					r.info.PublicIP = *vnic.PublicIp
				}
				if vnic.PrivateIp != nil && *vnic.PrivateIp != "" {
					r.info.PrivateIP = *vnic.PrivateIp
				}
				if ipv6Resp, err := vnClient.ListIpv6s(ctx, core.ListIpv6sRequest{VnicId: vnicID}); err == nil && len(ipv6Resp.Items) > 0 {
					if ipv6Resp.Items[0].IpAddress != nil {
						r.ipv6 = *ipv6Resp.Items[0].IpAddress
					}
				}
				results[idx] = r
			}(i, vnicAttachment.VnicId)
		}
		vwg.Wait()

		// 按原始顺序汇总（仅本 goroutine 写这些字段）
		for _, r := range results {
			if !r.valid {
				continue
			}
			info.VnicList = append(info.VnicList, r.info)
			if r.info.PublicIP != "" {
				info.PublicIPs = append(info.PublicIPs, r.info.PublicIP)
			}
			if r.info.PrivateIP != "" {
				info.PrivateIPs = append(info.PrivateIPs, r.info.PrivateIP)
			}
			if info.IPv6 == "" && r.ipv6 != "" {
				info.IPv6 = r.ipv6
			}
		}
	}()

	wg.Wait()
	return info, nil
}

// GetInstancesDetailsConcurrent 并发获取多个实例的详情（并发上限 instanceDetailConcurrency），
// 结果保持与输入实例相同的顺序，获取失败的实例被跳过（与此前 `if err == nil` 行为一致）。
// 替代调用方按实例串行调用 GetInstanceDetails 的 N×(6~8) 往返模式。
func (s *OCIService) GetInstancesDetailsConcurrent(ctx context.Context, user *models.OciUser, instances []core.Instance) []models.InstanceInfo {
	results := make([]*models.InstanceInfo, len(instances))
	sem := make(chan struct{}, instanceDetailConcurrency)
	var wg sync.WaitGroup
	for i, inst := range instances {
		if inst.Id == nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, instanceID string) {
			defer wg.Done()
			defer func() { <-sem }()
			if detail, err := s.GetInstanceDetails(ctx, user, instanceID); err == nil {
				results[idx] = detail
			}
		}(i, *inst.Id)
	}
	wg.Wait()

	infos := make([]models.InstanceInfo, 0, len(instances))
	for _, r := range results {
		if r != nil {
			infos = append(infos, *r)
		}
	}
	return infos
}

// instanceDetailConcurrency 限制同时获取实例详情的数量，避免对 OCI API 造成过高并发（触发限流）。
const instanceDetailConcurrency = 6

func (s *OCIService) GetInstanceById(user *models.OciUser, instanceID string) (*core.Instance, error) {
	ctx := context.Background()
	client, err := s.GetComputeClient(user)
	if err != nil {
		return nil, err
	}

	resp, err := client.GetInstance(ctx, core.GetInstanceRequest{InstanceId: &instanceID})
	if err != nil {
		return nil, err
	}

	return &resp.Instance, nil
}

func (s *OCIService) GetBootVolumeByInstanceId(user *models.OciUser, instanceID string) (*core.BootVolume, error) {
	ctx := context.Background()

	computeClient, err := s.GetComputeClient(user)
	if err != nil {
		return nil, err
	}

	blockClient, err := s.GetBlockstorageClient(user)
	if err != nil {
		return nil, err
	}

	instance, err := computeClient.GetInstance(ctx, core.GetInstanceRequest{InstanceId: &instanceID})
	if err != nil {
		return nil, err
	}

	bvaResp, err := computeClient.ListBootVolumeAttachments(ctx, core.ListBootVolumeAttachmentsRequest{
		CompartmentId:      instance.CompartmentId,
		AvailabilityDomain: instance.AvailabilityDomain,
		InstanceId:         &instanceID,
	})
	if err != nil {
		return nil, err
	}

	if len(bvaResp.Items) == 0 {
		return nil, fmt.Errorf("no boot volume attachment found")
	}

	bvResp, err := blockClient.GetBootVolume(ctx, core.GetBootVolumeRequest{
		BootVolumeId: bvaResp.Items[0].BootVolumeId,
	})
	if err != nil {
		return nil, err
	}

	return &bvResp.BootVolume, nil
}

// UpdateInstanceShape updates instance CPU/memory config with timeout-protected polling
func (s *OCIService) UpdateInstanceShape(ctx context.Context, user *models.OciUser, instanceId string, ocpus float32, memoryInGBs float32, autoRestart bool) error {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return err
	}

	instance, err := s.GetInstance(ctx, user, instanceId)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	wasRunning := instance.LifecycleState == core.InstanceLifecycleStateRunning

	if instance.LifecycleState == core.InstanceLifecycleStateRunning {
		_, err = client.InstanceAction(ctx, core.InstanceActionRequest{
			InstanceId: instance.Id,
			Action:     core.InstanceActionActionStop,
		})
		if err != nil {
			return fmt.Errorf("failed to stop instance: %w", err)
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		for {
			select {
			case <-timeoutCtx.Done():
				return fmt.Errorf("timeout waiting for instance to stop")
			default:
			}
			instResp, err := client.GetInstance(timeoutCtx, core.GetInstanceRequest{InstanceId: instance.Id})
			if err != nil {
				return fmt.Errorf("failed to get instance status: %w", err)
			}
			if instResp.LifecycleState == core.InstanceLifecycleStateStopped {
				break
			}
			if instResp.LifecycleState == core.InstanceLifecycleStateTerminated {
				return fmt.Errorf("instance was terminated unexpectedly")
			}
			time.Sleep(3 * time.Second)
		}
	} else if instance.LifecycleState != core.InstanceLifecycleStateStopped {
		return fmt.Errorf("instance is in %s state, cannot update config", instance.LifecycleState)
	}

	req := core.UpdateInstanceRequest{
		InstanceId: &instanceId,
		UpdateInstanceDetails: core.UpdateInstanceDetails{
			ShapeConfig: &core.UpdateInstanceShapeConfigDetails{
				Ocpus:       &ocpus,
				MemoryInGBs: &memoryInGBs,
			},
		},
	}

	_, err = client.UpdateInstance(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to update instance config: %w", err)
	}

	if autoRestart && wasRunning {
		_, err = client.InstanceAction(ctx, core.InstanceActionRequest{
			InstanceId: instance.Id,
			Action:     core.InstanceActionActionStart,
		})
		if err != nil {
			return fmt.Errorf("config updated but failed to restart instance: %w", err)
		}
	}

	return nil
}

func (s *OCIService) UpdateBootVolume(ctx context.Context, user *models.OciUser, bootVolumeId string, sizeInGBs int64, vpusPerGB int64) error {
	client, err := s.GetBlockstorageClient(user)
	if err != nil {
		return err
	}

	req := core.UpdateBootVolumeRequest{
		BootVolumeId: &bootVolumeId,
		UpdateBootVolumeDetails: core.UpdateBootVolumeDetails{
			SizeInGBs: &sizeInGBs,
			VpusPerGB: &vpusPerGB,
		},
	}

	_, err = client.UpdateBootVolume(ctx, req)
	return err
}

func (s *OCIService) CreateConsoleConnection(ctx context.Context, user *models.OciUser, instanceId string, publicKey string) (string, error) {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return "", err
	}

	req := core.CreateInstanceConsoleConnectionRequest{
		CreateInstanceConsoleConnectionDetails: core.CreateInstanceConsoleConnectionDetails{
			InstanceId: &instanceId,
			PublicKey:  &publicKey,
		},
	}

	resp, err := client.CreateInstanceConsoleConnection(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create console connection: %w", err)
	}

	if resp.Id == nil {
		return "", fmt.Errorf("console connection ID is nil")
	}

	return *resp.Id, nil
}

func (s *OCIService) GetConsoleConnectionString(ctx context.Context, user *models.OciUser, connectionId string) (string, error) {
	client, err := s.GetComputeClient(user)
	if err != nil {
		return "", err
	}

	var connectionString string
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		req := core.GetInstanceConsoleConnectionRequest{
			InstanceConsoleConnectionId: &connectionId,
		}
		resp, err := client.GetInstanceConsoleConnection(ctx, req)
		if err != nil {
			return "", fmt.Errorf("failed to get console connection: %w", err)
		}

		if resp.LifecycleState == core.InstanceConsoleConnectionLifecycleStateActive {
			if resp.ConnectionString != nil {
				connectionString = *resp.ConnectionString
				break
			}
		}

		time.Sleep(2 * time.Second)
	}

	if connectionString == "" {
		return "", fmt.Errorf("console connection did not become active within timeout")
	}

	return connectionString, nil
}
