package services

import (
        "context"
        "fmt"

        "github.com/adiecho/oci-panel/internal/database"
        "github.com/adiecho/oci-panel/internal/models"
        "github.com/oracle/oci-go-sdk/v65/common"
        "github.com/oracle/oci-go-sdk/v65/core"
        "github.com/oracle/oci-go-sdk/v65/identity"
)

// LookupOption 通用下拉选项（抢机表单的 AD / 镜像 / 子网选择）。
type LookupOption struct {
        Value string `json:"value"`
        Label string `json:"label"`
}

func (s *OCIService) loadUser(userID string) (*models.OciUser, error) {
        var user models.OciUser
        if err := database.GetDB().Where("id = ?", userID).First(&user).Error; err != nil {
                return nil, fmt.Errorf("配置不存在: %w", err)
        }
        return &user, nil
}

// ListAvailabilityDomains 列出配置所在区域全部可用域。
func (s *OCIService) ListAvailabilityDomains(userID string) ([]LookupOption, error) {
        user, err := s.loadUser(userID)
        if err != nil {
                return nil, err
        }
        client, err := s.GetIdentityClient(user)
        if err != nil {
                return nil, err
        }
        resp, err := client.ListAvailabilityDomains(context.Background(), identity.ListAvailabilityDomainsRequest{
                CompartmentId: common.String(user.OciTenantID),
        })
        if err != nil {
                return nil, err
        }
        out := make([]LookupOption, 0, len(resp.Items))
        for _, ad := range resp.Items {
                out = append(out, LookupOption{Value: *ad.Name, Label: *ad.Name})
        }
        return out, nil
}

// ListImagesByShape 按形状列出可用平台镜像（过滤掉已弃用），供抢机表单下拉。
func (s *OCIService) ListImagesByShape(userID, shape string, osFilter string) ([]LookupOption, error) {
        user, err := s.loadUser(userID)
        if err != nil {
                return nil, err
        }
        client, err := s.GetComputeClient(user)
        if err != nil {
                return nil, err
        }
        ctx := context.Background()
        req := core.ListImagesRequest{
                CompartmentId:          common.String(user.OciTenantID),
                Shape:                  common.String(shape),
                SortBy:                 core.ListImagesSortByTimecreated,
                SortOrder:              core.ListImagesSortOrderDesc,
                OperatingSystem:        stringPtr(osFilter),
        }
        var out []LookupOption
        page := ""
        for {
                req.Page = stringPtr(page)
                resp, err := client.ListImages(ctx, req)
                if err != nil {
                        return nil, err
                }
                for _, img := range resp.Items {
                        if img.LifecycleState != core.ImageLifecycleStateAvailable {
                                continue // 只保留 AVAILABLE 镜像（自动排除弃用/导入中/禁用）
                        }
                        label := fmt.Sprintf("%s %s", *img.OperatingSystem, *img.OperatingSystemVersion)
                        out = append(out, LookupOption{Value: *img.Id, Label: label})
                }
                if resp.OpcNextPage == nil {
                        break
                }
                page = *resp.OpcNextPage
        }
        return out, nil
}

// ListSubnets 列出全部 VCN 的子网（两级查询合并为一张下拉表）。
func (s *OCIService) ListSubnets(userID string) ([]LookupOption, error) {
        user, err := s.loadUser(userID)
        if err != nil {
                return nil, err
        }
        client, err := s.GetVirtualNetworkClient(user)
        if err != nil {
                return nil, err
        }
        ctx := context.Background()
        // VCN 名用于 label 前缀，帮助区分多 VCN 场景。
        vcnNames := map[string]string{}
        if vresp, err := client.ListVcns(ctx, core.ListVcnsRequest{CompartmentId: common.String(user.OciTenantID)}); err == nil {
                for _, v := range vresp.Items {
                        if v.Id != nil && v.DisplayName != nil {
                                vcnNames[*v.Id] = *v.DisplayName
                        }
                }
        }
        sresp, err := client.ListSubnets(ctx, core.ListSubnetsRequest{CompartmentId: common.String(user.OciTenantID)})
        if err != nil {
                return nil, err
        }
        out := make([]LookupOption, 0, len(sresp.Items))
        for _, sn := range sresp.Items {
                if sn.Id == nil || sn.DisplayName == nil {
                        continue
                }
                label := *sn.DisplayName
                if vn, ok := vcnNames[*sn.VcnId]; ok {
                        label = fmt.Sprintf("%s / %s (%s)", vn, *sn.DisplayName, *sn.CidrBlock)
                }
                out = append(out, LookupOption{Value: *sn.Id, Label: label})
        }
        return out, nil
}
