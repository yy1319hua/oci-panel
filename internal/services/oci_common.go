package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/adiecho/oci-panel/internal/config"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/util"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/identitydomains"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"
)

type OCIService struct {
	cfg         *config.Config
	clientCache sync.Map
}

// Include the credential revision so an in-flight request holding an old user
// snapshot cannot repopulate the cache entry used by updated credentials.
type clientCacheKey struct {
	userID      string
	region      string
	tenantID    string
	ociUserID   string
	fingerprint string
	keyPath     string
}

type cachedClients struct {
	configProvider common.ConfigurationProvider
	compute        *core.ComputeClient
	network        *core.VirtualNetworkClient
	blockstorage   *core.BlockstorageClient
	identity       *identity.IdentityClient
	monitoring     *monitoring.MonitoringClient
	nlb            *networkloadbalancer.NetworkLoadBalancerClient
	mu             sync.Mutex
}

func NewOCIService(cfg *config.Config) *OCIService {
	return &OCIService{cfg: cfg}
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

// paginate 反复调用 listPage 聚合所有分页结果，消除「只取首页、忽略 OpcNextPage」导致的结果截断。
// listPage 接收上一页返回的下一页 token（首次为 nil），返回当前页条目与下一页 token（无更多页时为 nil）。
func paginate[T any](listPage func(page *string) (items []T, nextPage *string, err error)) ([]T, error) {
	var all []T
	var page *string
	for {
		items, next, err := listPage(page)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if next == nil {
			return all, nil
		}
		page = next
	}
}

// waitForState 在重试间隔内也响应取消；最后一次失败后不再额外等待。
func waitForState[T any](ctx context.Context, maxAttempts int, interval time.Duration, getState func() (T, error), isReady func(T) bool) (T, bool) {
	var last T
	for i := 0; i < maxAttempts; i++ {
		if ctx.Err() != nil {
			return last, false
		}
		if v, err := getState(); err == nil {
			last = v
			if isReady(v) {
				return last, true
			}
		}
		if i+1 < maxAttempts {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return last, false
			case <-timer.C:
			}
		}
	}
	return last, false
}

func (s *OCIService) GetConfigProvider(user *models.OciUser) (common.ConfigurationProvider, error) {
	keyPath, err := util.KeyFilePath(user.OciKeyPath)
	if err != nil {
		return nil, fmt.Errorf("invalid private key path: %w", err)
	}
	privateKey, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	return common.NewRawConfigurationProvider(
		user.OciTenantID,
		user.OciUserID,
		user.OciRegion,
		user.OciFingerprint,
		string(privateKey),
		nil,
	), nil
}

func (s *OCIService) getCachedClients(user *models.OciUser) (*cachedClients, error) {
	return s.getCachedClientsForRegion(user, user.OciRegion)
}

// getCachedClientsForRegion 返回 (user, region) 维度缓存的客户端集合。同一 user+region
// 复用同一组 OCI 客户端与 configProvider，避免每次重新读取磁盘私钥并重建客户端。
// 当 region == user.OciRegion 时，与 getCachedClients 命中同一缓存条目。
func (s *OCIService) getCachedClientsForRegion(user *models.OciUser, region string) (*cachedClients, error) {
	cacheKey := clientCacheKey{
		userID: user.ID, region: region, tenantID: user.OciTenantID,
		ociUserID: user.OciUserID, fingerprint: user.OciFingerprint, keyPath: user.OciKeyPath,
	}
	if cached, ok := s.clientCache.Load(cacheKey); ok {
		return cached.(*cachedClients), nil
	}

	configProvider, err := s.GetConfigProviderForRegion(user, region)
	if err != nil {
		return nil, err
	}

	cc := &cachedClients{configProvider: configProvider}
	actual, _ := s.clientCache.LoadOrStore(cacheKey, cc)
	return actual.(*cachedClients), nil
}

func (s *OCIService) GetComputeClient(user *models.OciUser) (core.ComputeClient, error) {
	cc, err := s.getCachedClients(user)
	if err != nil {
		return core.ComputeClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.compute != nil {
		return *cc.compute, nil
	}

	client, err := core.NewComputeClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return core.ComputeClient{}, err
	}
	cc.compute = &client
	return client, nil
}

func (s *OCIService) GetVirtualNetworkClient(user *models.OciUser) (core.VirtualNetworkClient, error) {
	cc, err := s.getCachedClients(user)
	if err != nil {
		return core.VirtualNetworkClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.network != nil {
		return *cc.network, nil
	}

	client, err := core.NewVirtualNetworkClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return core.VirtualNetworkClient{}, err
	}
	cc.network = &client
	return client, nil
}

func (s *OCIService) GetBlockstorageClient(user *models.OciUser) (core.BlockstorageClient, error) {
	cc, err := s.getCachedClients(user)
	if err != nil {
		return core.BlockstorageClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.blockstorage != nil {
		return *cc.blockstorage, nil
	}

	client, err := core.NewBlockstorageClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return core.BlockstorageClient{}, err
	}
	cc.blockstorage = &client
	return client, nil
}

func (s *OCIService) GetIdentityClient(user *models.OciUser) (identity.IdentityClient, error) {
	cc, err := s.getCachedClients(user)
	if err != nil {
		return identity.IdentityClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.identity != nil {
		return *cc.identity, nil
	}

	client, err := identity.NewIdentityClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return identity.IdentityClient{}, err
	}
	cc.identity = &client
	return client, nil
}

func (s *OCIService) GetIdentityDomainsClient(user *models.OciUser, endpoint string) (identitydomains.IdentityDomainsClient, error) {
	configProvider, err := s.GetConfigProvider(user)
	if err != nil {
		return identitydomains.IdentityDomainsClient{}, err
	}

	client, err := identitydomains.NewIdentityDomainsClientWithConfigurationProvider(configProvider, endpoint)
	if err != nil {
		return identitydomains.IdentityDomainsClient{}, err
	}

	return client, nil
}

func (s *OCIService) GetMonitoringClient(user *models.OciUser) (monitoring.MonitoringClient, error) {
	cc, err := s.getCachedClients(user)
	if err != nil {
		return monitoring.MonitoringClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.monitoring != nil {
		return *cc.monitoring, nil
	}

	client, err := monitoring.NewMonitoringClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return monitoring.MonitoringClient{}, err
	}
	cc.monitoring = &client
	return client, nil
}

func (s *OCIService) GetNetworkLoadBalancerClient(user *models.OciUser) (networkloadbalancer.NetworkLoadBalancerClient, error) {
	cc, err := s.getCachedClients(user)
	if err != nil {
		return networkloadbalancer.NetworkLoadBalancerClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.nlb != nil {
		return *cc.nlb, nil
	}

	client, err := networkloadbalancer.NewNetworkLoadBalancerClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return networkloadbalancer.NetworkLoadBalancerClient{}, err
	}
	cc.nlb = &client
	return client, nil
}

func (s *OCIService) GetConfigProviderForRegion(user *models.OciUser, region string) (common.ConfigurationProvider, error) {
	keyPath, err := util.KeyFilePath(user.OciKeyPath)
	if err != nil {
		return nil, fmt.Errorf("invalid private key path: %w", err)
	}
	privateKey, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	return common.NewRawConfigurationProvider(
		user.OciTenantID,
		user.OciUserID,
		region,
		user.OciFingerprint,
		string(privateKey),
		nil,
	), nil
}

func (s *OCIService) GetComputeClientForRegion(user *models.OciUser, region string) (core.ComputeClient, error) {
	cc, err := s.getCachedClientsForRegion(user, region)
	if err != nil {
		return core.ComputeClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.compute != nil {
		return *cc.compute, nil
	}

	client, err := core.NewComputeClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return core.ComputeClient{}, err
	}
	cc.compute = &client
	return client, nil
}

func (s *OCIService) GetVirtualNetworkClientForRegion(user *models.OciUser, region string) (core.VirtualNetworkClient, error) {
	cc, err := s.getCachedClientsForRegion(user, region)
	if err != nil {
		return core.VirtualNetworkClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.network != nil {
		return *cc.network, nil
	}

	client, err := core.NewVirtualNetworkClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return core.VirtualNetworkClient{}, err
	}
	cc.network = &client
	return client, nil
}

func (s *OCIService) GetIdentityClientForRegion(user *models.OciUser, region string) (identity.IdentityClient, error) {
	cc, err := s.getCachedClientsForRegion(user, region)
	if err != nil {
		return identity.IdentityClient{}, err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.identity != nil {
		return *cc.identity, nil
	}

	client, err := identity.NewIdentityClientWithConfigurationProvider(cc.configProvider)
	if err != nil {
		return identity.IdentityClient{}, err
	}
	cc.identity = &client
	return client, nil
}

// InvalidateClientCache removes cached clients for a user (call when credentials change)
func (s *OCIService) InvalidateClientCache(userID string) {
	s.clientCache.Range(func(key, value any) bool {
		if k, ok := key.(clientCacheKey); ok && k.userID == userID {
			s.clientCache.Delete(key)
		}
		return true
	})
}

// isPrivateIP checks if an IP address is private
func isPrivateIP(ip string) bool {
	return strings.HasPrefix(ip, "10.") ||
		strings.HasPrefix(ip, "172.16.") ||
		strings.HasPrefix(ip, "172.17.") ||
		strings.HasPrefix(ip, "172.18.") ||
		strings.HasPrefix(ip, "172.19.") ||
		strings.HasPrefix(ip, "172.20.") ||
		strings.HasPrefix(ip, "172.21.") ||
		strings.HasPrefix(ip, "172.22.") ||
		strings.HasPrefix(ip, "172.23.") ||
		strings.HasPrefix(ip, "172.24.") ||
		strings.HasPrefix(ip, "172.25.") ||
		strings.HasPrefix(ip, "172.26.") ||
		strings.HasPrefix(ip, "172.27.") ||
		strings.HasPrefix(ip, "172.28.") ||
		strings.HasPrefix(ip, "172.29.") ||
		strings.HasPrefix(ip, "172.30.") ||
		strings.HasPrefix(ip, "172.31.") ||
		strings.HasPrefix(ip, "192.168.")
}
