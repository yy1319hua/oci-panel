package services

import (
	"context"
	"fmt"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/core"
)

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
