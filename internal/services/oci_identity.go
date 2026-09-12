package services

import (
	"context"
	"fmt"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/identitydomains"
)

func (s *OCIService) GetTenantInfo(ctx context.Context, user *models.OciUser) (*models.TenantInfo, error) {
	identityClient, err := s.GetIdentityClient(user)
	if err != nil {
		return nil, err
	}

	// 获取租户信息
	tenantReq := identity.GetTenancyRequest{TenancyId: &user.OciTenantID}
	tenantResp, err := identityClient.GetTenancy(ctx, tenantReq)
	if err != nil {
		return nil, err
	}

	tenantInfo := &models.TenantInfo{
		ID:       *tenantResp.Id,
		UserList: []models.TenantUserInfo{},
	}
	if tenantResp.Name != nil {
		tenantInfo.Name = *tenantResp.Name
	}
	if tenantResp.Description != nil {
		tenantInfo.Description = *tenantResp.Description
	}
	if tenantResp.HomeRegionKey != nil {
		tenantInfo.HomeRegionKey = *tenantResp.HomeRegionKey
	}

	// 获取区域列表
	regionsReq := identity.ListRegionSubscriptionsRequest{TenancyId: &user.OciTenantID}
	regionsResp, err := identityClient.ListRegionSubscriptions(ctx, regionsReq)
	if err == nil {
		for _, region := range regionsResp.Items {
			if region.RegionName != nil {
				tenantInfo.Regions = append(tenantInfo.Regions, *region.RegionName)
			}
		}
	}

	// 获取用户列表（全量分页，最佳努力：失败则用户列表为空但不影响其余租户信息）
	allUsers, _ := paginate(func(page *string) ([]identity.User, *string, error) {
		resp, perr := identityClient.ListUsers(ctx, identity.ListUsersRequest{
			CompartmentId: &user.OciTenantID,
			Page:          page,
		})
		if perr != nil {
			return nil, nil, perr
		}
		return resp.Items, resp.OpcNextPage, nil
	})
	for _, u := range allUsers {
		userInfo := models.TenantUserInfo{
			ID:    *u.Id,
			State: string(u.LifecycleState),
		}
		if u.Name != nil {
			userInfo.Name = *u.Name
		}
		if u.Email != nil {
			userInfo.Email = *u.Email
		}
		if u.EmailVerified != nil {
			userInfo.EmailVerified = *u.EmailVerified
		}
		if u.IsMfaActivated != nil {
			userInfo.IsMfaActivated = *u.IsMfaActivated
		}
		if u.TimeCreated != nil {
			userInfo.CreateTime = u.TimeCreated.Format("2006-01-02 15:04:05")
		}
		if u.LastSuccessfulLoginTime != nil {
			userInfo.LastSuccessfulLoginTime = u.LastSuccessfulLoginTime.Format("2006-01-02 15:04:05")
		}
		tenantInfo.UserList = append(tenantInfo.UserList, userInfo)
	}

	// 获取密码过期策略
	passwordExpiresAfter, err := s.GetPasswordExpiresAfter(ctx, user)
	if err != nil {
		// 如果获取失败，设置为0（表示永不过期）
		tenantInfo.PasswordExpiresAfter = 0
	} else {
		tenantInfo.PasswordExpiresAfter = passwordExpiresAfter
	}

	// 获取租户创建时间（通过compartment的创建时间）
	compartmentReq := identity.GetCompartmentRequest{CompartmentId: &user.OciTenantID}
	compartmentResp, err := identityClient.GetCompartment(ctx, compartmentReq)
	if err == nil && compartmentResp.TimeCreated != nil {
		tenantInfo.CreateTime = compartmentResp.TimeCreated.Format("2006-01-02 15:04:05")
	}

	return tenantInfo, nil
}

// GetDomainURL 获取 Identity Domain URL
func (s *OCIService) GetDomainURL(ctx context.Context, user *models.OciUser) (string, error) {
	identityClient, err := s.GetIdentityClient(user)
	if err != nil {
		return "", err
	}

	// 列出所有 domains
	listDomainsReq := identity.ListDomainsRequest{
		CompartmentId: &user.OciTenantID,
	}
	listDomainsResp, err := identityClient.ListDomains(ctx, listDomainsReq)
	if err != nil {
		return "", fmt.Errorf("failed to list domains: %w", err)
	}

	// 找到第一个 ACTIVE 状态的 domain
	for _, domain := range listDomainsResp.Items {
		if domain.LifecycleState == identity.DomainLifecycleStateActive && domain.Url != nil {
			return *domain.Url, nil
		}
	}

	return "", fmt.Errorf("no active domain found")
}

// GetPasswordExpiresAfter 获取密码过期天数
func (s *OCIService) GetPasswordExpiresAfter(ctx context.Context, user *models.OciUser) (int, error) {
	// 获取 Domain URL
	domainURL, err := s.GetDomainURL(ctx, user)
	if err != nil {
		return 0, err
	}

	// 创建 Identity Domains Client
	domainsClient, err := s.GetIdentityDomainsClient(user, domainURL)
	if err != nil {
		return 0, err
	}

	// 列出密码策略
	listPoliciesReq := identitydomains.ListPasswordPoliciesRequest{}
	listPoliciesResp, err := domainsClient.ListPasswordPolicies(ctx, listPoliciesReq)
	if err != nil {
		return 0, fmt.Errorf("failed to list password policies: %w", err)
	}

	// 查找 Custom 类型的策略（通过 PasswordStrength 字段判断）
	if listPoliciesResp.PasswordPolicies.Resources != nil {
		for _, policy := range listPoliciesResp.PasswordPolicies.Resources {
			// 检查是否为 Custom 策略（使用 PasswordStrength 字段）
			if policy.PasswordStrength == identitydomains.PasswordPolicyPasswordStrengthCustom {
				if policy.PasswordExpiresAfter != nil {
					return *policy.PasswordExpiresAfter, nil
				}
			}
		}
	}

	// 如果没有找到 Custom 策略，返回0
	return 0, nil
}

// UpdatePasswordExpiresAfter 更新密码过期天数
func (s *OCIService) UpdatePasswordExpiresAfter(ctx context.Context, user *models.OciUser, expiresAfter int) error {
	// 获取 Domain URL
	domainURL, err := s.GetDomainURL(ctx, user)
	if err != nil {
		return err
	}

	// 创建 Identity Domains Client
	domainsClient, err := s.GetIdentityDomainsClient(user, domainURL)
	if err != nil {
		return err
	}

	// 列出密码策略
	listPoliciesReq := identitydomains.ListPasswordPoliciesRequest{}
	listPoliciesResp, err := domainsClient.ListPasswordPolicies(ctx, listPoliciesReq)
	if err != nil {
		return fmt.Errorf("failed to list password policies: %w", err)
	}

	// 查找并更新 Custom 策略
	if listPoliciesResp.PasswordPolicies.Resources == nil {
		return fmt.Errorf("no password policies found")
	}

	for _, policy := range listPoliciesResp.PasswordPolicies.Resources {
		// 检查是否为 Custom 策略（使用 PasswordStrength 字段）
		if policy.PasswordStrength == identitydomains.PasswordPolicyPasswordStrengthCustom {
			if policy.Id == nil {
				continue
			}

			// 获取当前策略的完整信息
			getPolicyReq := identitydomains.GetPasswordPolicyRequest{
				PasswordPolicyId: policy.Id,
			}
			getPolicyResp, err := domainsClient.GetPasswordPolicy(ctx, getPolicyReq)
			if err != nil {
				return fmt.Errorf("failed to get password policy: %w", err)
			}

			currentPolicy := getPolicyResp.PasswordPolicy

			// 只更新 PasswordExpiresAfter 字段，保留其他字段
			currentPolicy.PasswordExpiresAfter = &expiresAfter
			currentPolicy.ForcePasswordReset = boolPtr(false)

			// 构建更新请求
			putPolicyReq := identitydomains.PutPasswordPolicyRequest{
				PasswordPolicyId: policy.Id,
				PasswordPolicy:   currentPolicy,
			}

			_, err = domainsClient.PutPasswordPolicy(ctx, putPolicyReq)
			if err != nil {
				return fmt.Errorf("failed to update password policy: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("no custom password policy found")
}

// DeleteUser 删除用户
func (s *OCIService) DeleteUser(ctx context.Context, user *models.OciUser, userId string) error {
	identityClient, err := s.GetIdentityClient(user)
	if err != nil {
		return err
	}

	deleteUserReq := identity.DeleteUserRequest{
		UserId: &userId,
	}

	_, err = identityClient.DeleteUser(ctx, deleteUserReq)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// UpdateUserInfo 更新用户信息
func (s *OCIService) UpdateUserInfo(ctx context.Context, user *models.OciUser, userId string, email string, dbUserName string, description string) error {
	domainURL, err := s.GetDomainURL(ctx, user)
	if err != nil {
		return err
	}

	domainsClient, err := s.GetIdentityDomainsClient(user, domainURL)
	if err != nil {
		return err
	}

	operations := []identitydomains.Operations{}

	if email != "" {
		emailVal := interface{}(email)
		operations = append(operations, identitydomains.Operations{
			Op:    identitydomains.OperationsOpReplace,
			Path:  stringPtr("emails[primary eq true].value"),
			Value: &emailVal,
		})
	}

	if dbUserName != "" {
		userNameVal := interface{}(dbUserName)
		operations = append(operations, identitydomains.Operations{
			Op:    identitydomains.OperationsOpReplace,
			Path:  stringPtr("userName"),
			Value: &userNameVal,
		})
	}

	if description != "" {
		descVal := interface{}(description)
		operations = append(operations, identitydomains.Operations{
			Op:    identitydomains.OperationsOpReplace,
			Path:  stringPtr("urn:ietf:params:scim:schemas:oracle:idcs:extension:user:User:description"),
			Value: &descVal,
		})
	}

	if len(operations) == 0 {
		return fmt.Errorf("no fields to update")
	}

	patchReq := identitydomains.PatchUserRequest{
		UserId: &userId,
		PatchOp: identitydomains.PatchOp{
			Schemas:    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			Operations: operations,
		},
	}

	_, err = domainsClient.PatchUser(ctx, patchReq)
	if err != nil {
		return fmt.Errorf("failed to update user info: %w", err)
	}

	return nil
}

// ResetUserPassword 重置用户密码
func (s *OCIService) ResetUserPassword(ctx context.Context, user *models.OciUser, userId string) error {
	identityClient, err := s.GetIdentityClient(user)
	if err != nil {
		return err
	}

	createPasswordReq := identity.CreateOrResetUIPasswordRequest{
		UserId: &userId,
	}

	_, err = identityClient.CreateOrResetUIPassword(ctx, createPasswordReq)
	if err != nil {
		return fmt.Errorf("failed to reset password: %w", err)
	}

	return nil
}

// DeleteUserMfaDevices 删除用户的所有MFA设备
func (s *OCIService) DeleteUserMfaDevices(ctx context.Context, user *models.OciUser, userId string) error {
	domainURL, err := s.GetDomainURL(ctx, user)
	if err != nil {
		return err
	}

	domainsClient, err := s.GetIdentityDomainsClient(user, domainURL)
	if err != nil {
		return err
	}

	listMfaReq := identitydomains.ListMyDevicesRequest{
		Filter: stringPtr(fmt.Sprintf("user.value eq \"%s\"", userId)),
	}

	listMfaResp, err := domainsClient.ListMyDevices(ctx, listMfaReq)
	if err != nil {
		return fmt.Errorf("failed to list MFA devices: %w", err)
	}

	if listMfaResp.MyDevices.Resources == nil || len(listMfaResp.MyDevices.Resources) == 0 {
		return nil
	}

	for _, device := range listMfaResp.MyDevices.Resources {
		if device.Id != nil {
			deleteReq := identitydomains.DeleteMyDeviceRequest{
				MyDeviceId: device.Id,
			}
			_, err := domainsClient.DeleteMyDevice(ctx, deleteReq)
			if err != nil {
				return fmt.Errorf("failed to delete MFA device %s: %w", *device.Id, err)
			}
		}
	}

	return nil
}

// DeleteUserApiKeys 删除用户的所有API密钥
func (s *OCIService) DeleteUserApiKeys(ctx context.Context, user *models.OciUser, userId string) error {
	identityClient, err := s.GetIdentityClient(user)
	if err != nil {
		return err
	}

	listKeysReq := identity.ListApiKeysRequest{
		UserId: &userId,
	}

	listKeysResp, err := identityClient.ListApiKeys(ctx, listKeysReq)
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

	if len(listKeysResp.Items) == 0 {
		return nil
	}

	for _, apiKey := range listKeysResp.Items {
		if apiKey.KeyId != nil {
			deleteKeyReq := identity.DeleteApiKeyRequest{
				UserId:      &userId,
				Fingerprint: apiKey.Fingerprint,
			}
			_, err := identityClient.DeleteApiKey(ctx, deleteKeyReq)
			if err != nil {
				return fmt.Errorf("failed to delete API key %s: %w", *apiKey.KeyId, err)
			}
		}
	}

	return nil
}
