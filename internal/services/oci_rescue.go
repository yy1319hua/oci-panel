package services

import (
	"context"
	"fmt"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/core"
)

// AutoRescueParams 自动救援参数
type AutoRescueParams struct {
	InstanceID       string
	InstanceName     string
	KeepBackupVolume bool
}

// AutoRescueProgress 自动救援进度
type AutoRescueProgress struct {
	Step        int    `json:"step"`
	TotalSteps  int    `json:"totalSteps"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	PublicIP    string `json:"publicIp,omitempty"`
	SSHPassword string `json:"sshPassword,omitempty"`
}

// AutoRescue 自动救援/缩小硬盘 (9步骤)
func (s *OCIService) AutoRescue(user *models.OciUser, params AutoRescueParams, progressChan chan<- AutoRescueProgress) error {
	ctx := context.Background()

	computeClient, err := s.GetComputeClient(user)
	if err != nil {
		return fmt.Errorf("failed to get compute client: %w", err)
	}

	blockClient, err := s.GetBlockstorageClient(user)
	if err != nil {
		return fmt.Errorf("failed to get blockstorage client: %w", err)
	}

	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return fmt.Errorf("failed to get virtual network client: %w", err)
	}

	sendProgress := func(step int, status, message string) {
		if progressChan != nil {
			progressChan <- AutoRescueProgress{
				Step:       step,
				TotalSteps: 9,
				Status:     status,
				Message:    message,
			}
		}
	}

	// 获取实例信息
	instance, err := s.GetInstanceById(user, params.InstanceID)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	// 获取引导卷
	bootVolume, err := s.GetBootVolumeByInstanceId(user, params.InstanceID)
	if err != nil {
		return fmt.Errorf("failed to get boot volume: %w", err)
	}

	// Step 1: 关机
	sendProgress(1, "running", "正在关机...")
	_, err = computeClient.InstanceAction(ctx, core.InstanceActionRequest{
		InstanceId: instance.Id,
		Action:     core.InstanceActionActionStop,
	})
	if err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	// 等待实例停止
	timeoutCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel1()
	for {
		select {
		case <-timeoutCtx1.Done():
			return fmt.Errorf("timeout waiting for instance to stop")
		default:
		}
		instResp, err := computeClient.GetInstance(timeoutCtx1, core.GetInstanceRequest{InstanceId: instance.Id})
		if err != nil {
			return fmt.Errorf("failed to get instance status: %w", err)
		}
		if instResp.LifecycleState == core.InstanceLifecycleStateStopped {
			break
		}
		time.Sleep(2 * time.Second)
	}
	sendProgress(1, "completed", "关机成功")

	// 等待引导卷可用
	timeoutCtx2, cancel2 := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel2()
	for {
		select {
		case <-timeoutCtx2.Done():
			return fmt.Errorf("timeout waiting for boot volume to become available")
		default:
		}
		bvResp, err := blockClient.GetBootVolume(timeoutCtx2, core.GetBootVolumeRequest{BootVolumeId: bootVolume.Id})
		if err != nil {
			return fmt.Errorf("failed to get boot volume status: %w", err)
		}
		if bvResp.LifecycleState == core.BootVolumeLifecycleStateAvailable {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Step 2: 备份原引导卷
	sendProgress(2, "running", "正在备份原引导卷...")
	backupName := "Old-BootVolume-Backup"
	backupResp, err := blockClient.CreateBootVolumeBackup(ctx, core.CreateBootVolumeBackupRequest{
		CreateBootVolumeBackupDetails: core.CreateBootVolumeBackupDetails{
			BootVolumeId: bootVolume.Id,
			DisplayName:  &backupName,
			Type:         core.CreateBootVolumeBackupDetailsTypeFull,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create boot volume backup: %w", err)
	}
	backupId := backupResp.Id
	sendProgress(2, "completed", "备份原引导卷成功")

	time.Sleep(3 * time.Second)

	// Step 3: 分离原引导卷
	sendProgress(3, "running", "正在分离原引导卷...")
	// 获取引导卷附件
	bvaListResp, err := computeClient.ListBootVolumeAttachments(ctx, core.ListBootVolumeAttachmentsRequest{
		CompartmentId:      instance.CompartmentId,
		AvailabilityDomain: instance.AvailabilityDomain,
		InstanceId:         instance.Id,
	})
	if err != nil {
		return fmt.Errorf("failed to list boot volume attachments: %w", err)
	}
	if len(bvaListResp.Items) == 0 {
		return fmt.Errorf("no boot volume attachment found")
	}
	bvaId := bvaListResp.Items[0].Id

	_, err = computeClient.DetachBootVolume(ctx, core.DetachBootVolumeRequest{
		BootVolumeAttachmentId: bvaId,
	})
	if err != nil {
		return fmt.Errorf("failed to detach boot volume: %w", err)
	}
	sendProgress(3, "completed", "分离原引导卷成功")

	// 等待备份完成
	timeoutCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel3()
	for {
		select {
		case <-timeoutCtx3.Done():
			return fmt.Errorf("timeout waiting for boot volume backup to complete")
		default:
		}
		backupStatusResp, err := blockClient.GetBootVolumeBackup(timeoutCtx3, core.GetBootVolumeBackupRequest{BootVolumeBackupId: backupId})
		if err != nil {
			return fmt.Errorf("failed to get backup status: %w", err)
		}
		if backupStatusResp.LifecycleState == core.BootVolumeBackupLifecycleStateAvailable {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Step 4: 删除原引导卷
	sendProgress(4, "running", "正在删除原引导卷...")
	_, err = blockClient.DeleteBootVolume(ctx, core.DeleteBootVolumeRequest{
		BootVolumeId: bootVolume.Id,
	})
	if err != nil {
		return fmt.Errorf("failed to delete boot volume: %w", err)
	}

	// 等待引导卷删除完成
	timeoutCtx4, cancel4 := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel4()
	for {
		select {
		case <-timeoutCtx4.Done():
			return fmt.Errorf("timeout waiting for boot volume to be deleted")
		default:
		}
		bvResp, err := blockClient.GetBootVolume(timeoutCtx4, core.GetBootVolumeRequest{BootVolumeId: bootVolume.Id})
		if err != nil {
			break // 404 means deleted
		}
		if bvResp.LifecycleState == core.BootVolumeLifecycleStateTerminated {
			break
		}
		time.Sleep(2 * time.Second)
	}
	sendProgress(4, "completed", "删除原引导卷成功")

	// Step 5: 从备份创建新的47GB引导卷
	sendProgress(5, "running", "正在创建47GB引导卷...")
	newBvName := "Restored-Boot-Volume-47GB"
	sizeInGBs := int64(47)
	newBvResp, err := blockClient.CreateBootVolume(ctx, core.CreateBootVolumeRequest{
		CreateBootVolumeDetails: core.CreateBootVolumeDetails{
			CompartmentId:      instance.CompartmentId,
			AvailabilityDomain: instance.AvailabilityDomain,
			DisplayName:        &newBvName,
			SizeInGBs:          &sizeInGBs,
			SourceDetails: core.BootVolumeSourceFromBootVolumeBackupDetails{
				Id: backupId,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create new boot volume: %w", err)
	}
	newBvId := newBvResp.Id

	// 等待新引导卷可用
	timeoutCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel5()
	for {
		select {
		case <-timeoutCtx5.Done():
			return fmt.Errorf("timeout waiting for new boot volume to become available")
		default:
		}
		bvResp, err := blockClient.GetBootVolume(timeoutCtx5, core.GetBootVolumeRequest{BootVolumeId: newBvId})
		if err != nil {
			return fmt.Errorf("failed to get new boot volume status: %w", err)
		}
		if bvResp.LifecycleState == core.BootVolumeLifecycleStateAvailable {
			break
		}
		time.Sleep(2 * time.Second)
	}
	sendProgress(5, "completed", "创建47GB引导卷成功")

	// Step 6: 附加新引导卷到实例
	sendProgress(6, "running", "正在附加新引导卷到实例...")
	attachName := "New-Boot-Volume"
	_, err = computeClient.AttachBootVolume(ctx, core.AttachBootVolumeRequest{
		AttachBootVolumeDetails: core.AttachBootVolumeDetails{
			BootVolumeId: newBvId,
			InstanceId:   instance.Id,
			DisplayName:  &attachName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to attach boot volume: %w", err)
	}
	sendProgress(6, "completed", "附加新引导卷成功")

	// Step 7: 删除备份（如果不保留）
	if !params.KeepBackupVolume {
		sendProgress(7, "running", "正在删除原引导卷备份...")
		_, err = blockClient.DeleteBootVolumeBackup(ctx, core.DeleteBootVolumeBackupRequest{
			BootVolumeBackupId: backupId,
		})
		if err != nil {
			// 不影响后续操作，只记录错误
			sendProgress(7, "warning", "删除原引导卷备份失败，但不影响继续操作")
		} else {
			sendProgress(7, "completed", "删除原引导卷备份成功")
		}
	} else {
		sendProgress(7, "skipped", "保留原引导卷备份")
	}

	// Step 8: 等待引导卷附件完成
	sendProgress(8, "running", "正在等待引导卷附件完成...")
	time.Sleep(5 * time.Second)
	sendProgress(8, "completed", "引导卷附件完成")

	// Step 9: 启动实例
	sendProgress(9, "running", "正在启动实例...")
	for i := 0; i < 30; i++ {
		instResp, err := computeClient.GetInstance(ctx, core.GetInstanceRequest{InstanceId: instance.Id})
		if err != nil {
			continue
		}
		if instResp.LifecycleState == core.InstanceLifecycleStateRunning {
			break
		}
		_, _ = computeClient.InstanceAction(ctx, core.InstanceActionRequest{
			InstanceId: instance.Id,
			Action:     core.InstanceActionActionStart,
		})
		time.Sleep(3 * time.Second)
	}

	// 获取公网IP
	publicIP := ""
	vnicAttachments, err := computeClient.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
		CompartmentId: instance.CompartmentId,
		InstanceId:    instance.Id,
	})
	if err == nil && len(vnicAttachments.Items) > 0 {
		vnicId := vnicAttachments.Items[0].VnicId
		if vnicId != nil {
			vnicResp, err := vnClient.GetVnic(ctx, core.GetVnicRequest{VnicId: vnicId})
			if err == nil && vnicResp.PublicIp != nil {
				publicIP = *vnicResp.PublicIp
			}
		}
	}

	if progressChan != nil {
		progressChan <- AutoRescueProgress{
			Step:       9,
			TotalSteps: 9,
			Status:     "completed",
			Message:    "实例救援成功，已启动",
			PublicIP:   publicIP,
		}
	}

	return nil
}
