package azurelustre

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	AmlfsSkuResourceType          = "amlFilesystems"
	AmlfsSkuCapacityIncrementName = "OSS capacity increment (TiB)"
	AmlfsSkuCapacityMaximumName   = "default maximum capacity (TiB)"
)

type DynamicProvisionerInterface interface {
	DeleteAmlFilesystem(ctx context.Context, resourceGroupName, amlFilesystemName string) error
	CreateAmlFilesystem(ctx context.Context, amlFilesystemProperties *AmlFilesystemProperties) (string, error)
	ClusterExists(ctx context.Context, resourceGroupName, amlFilesystemName string) (bool, error)
	GetSkuValuesForLocation(ctx context.Context, location string) map[string]*LustreSkuValue
}

type DynamicProvisioner struct {
	DynamicProvisionerInterface
	amlFilesystemsClient *armstoragecache.AmlFilesystemsClient
	mgmtClient           *armstoragecache.ManagementClient
	skusClient           *armstoragecache.SKUsClient
	vnetClient           *armnetwork.VirtualNetworksClient
	defaultSkuValues     map[string]*LustreSkuValue
	pollFrequency        time.Duration
}

func convertHTTPResponseErrorToGrpcCodeError(err error) error {
	klog.Errorf("converting error: %#v", err)

	if err == nil {
		return nil
	}

	_, ok := status.FromError(err)
	if ok {
		klog.V(2).Infof("error is already GRPC error: %v", err)
		return err
	}

	var httpError *azcore.ResponseError
	if !errors.As(err, &httpError) {
		klog.Errorf("error is not a response error: %v", err)
		return status.Errorf(codes.Unknown, "error occurred calling API: %v", err)
	}

	klog.Errorf("converted error: %#v", httpError)

	statusCode := httpError.StatusCode

	var grpcErrorCode codes.Code
	if statusCode >= 400 && statusCode < 500 {
		switch statusCode {
		case http.StatusBadRequest:
			grpcErrorCode = codes.InvalidArgument
		case http.StatusConflict:
			if strings.Contains(err.Error(), "Operation results in exceeding quota limits of resource type AmlFilesystem") {
				grpcErrorCode = codes.ResourceExhausted
			} else {
				grpcErrorCode = codes.InvalidArgument
			}
		case http.StatusNotFound:
			grpcErrorCode = codes.NotFound
		case http.StatusForbidden:
			grpcErrorCode = codes.PermissionDenied
		case http.StatusUnauthorized:
			grpcErrorCode = codes.Unauthenticated
		case http.StatusTooManyRequests:
			grpcErrorCode = codes.Unavailable
		default:
			grpcErrorCode = codes.InvalidArgument
		}
	} else if statusCode >= 500 {
		switch statusCode {
		case http.StatusInternalServerError:
			grpcErrorCode = codes.Internal
		case http.StatusBadGateway:
			grpcErrorCode = codes.Unavailable
		case http.StatusServiceUnavailable:
			grpcErrorCode = codes.Unavailable
		case http.StatusGatewayTimeout:
			grpcErrorCode = codes.DeadlineExceeded
		default:
			// Prefer to default to Unknown rather than Internal so provisioner will retry
			grpcErrorCode = codes.Unknown
		}
	}

	return status.Errorf(grpcErrorCode, "error occurred calling API: %v", httpError)
}

func (d *DynamicProvisioner) ClusterExists(ctx context.Context, resourceGroupName, amlFilesystemName string) (bool, error) {
	if d.amlFilesystemsClient == nil {
		return false, status.Error(codes.Internal, "aml filesystem client is nil")
	}

	resp, err := d.amlFilesystemsClient.Get(ctx, resourceGroupName, amlFilesystemName, nil)
	if err != nil {
		if strings.Contains(err.Error(), "ResourceNotFound") {
			klog.V(2).Infof("Cluster not found!")
			return false, nil
		}

		klog.V(2).Infof("error when retrieving the aml filesystem: %v", err)
		return false, convertHTTPResponseErrorToGrpcCodeError(err)
	}
	klog.V(2).Infof("response when retrieving the aml filesystem: %#v", resp)
	return true, nil
}

func (d *DynamicProvisioner) DeleteAmlFilesystem(ctx context.Context, resourceGroupName, amlFilesystemName string) error {
	if d.amlFilesystemsClient == nil {
		return status.Error(codes.Internal, "aml filesystem client is nil")
	}

	poller, err := d.amlFilesystemsClient.BeginDelete(ctx, resourceGroupName, amlFilesystemName, nil)
	if err != nil {
		klog.Warningf("failed to finish the request: %v", err)
		return convertHTTPResponseErrorToGrpcCodeError(err)
	}

	pollerOptions := &runtime.PollUntilDoneOptions{
		Frequency: d.pollFrequency,
	}
	res, err := poller.PollUntilDone(ctx, pollerOptions)
	if err != nil {
		klog.Warningf("failed to poll the result: %v", err)
		return convertHTTPResponseErrorToGrpcCodeError(err)
	}
	klog.V(2).Infof("response to dyn: %v", res)

	return nil
}

func (d *DynamicProvisioner) CreateAmlFilesystem(ctx context.Context, amlFilesystemProperties *AmlFilesystemProperties) (string, error) {
	if d.amlFilesystemsClient == nil {
		return "", status.Error(codes.Internal, "aml filesystem client is nil")
	}
	if amlFilesystemProperties.SubnetInfo.SubnetID == "" || amlFilesystemProperties.SubnetInfo.SubnetName == "" || amlFilesystemProperties.SubnetInfo.VnetName == "" || amlFilesystemProperties.SubnetInfo.VnetResourceGroup == "" {
		return "", status.Error(codes.InvalidArgument, "invalid subnet info, must have valid subnet ID, subnet name, vnet name, and vnet resource group")
	}

	tags := make(map[string]*string, len(amlFilesystemProperties.Tags))
	for key, value := range amlFilesystemProperties.Tags {
		tags[key] = to.Ptr(value)
	}
	zones := make([]*string, len(amlFilesystemProperties.Zones))
	for i, zone := range amlFilesystemProperties.Zones {
		zones[i] = to.Ptr(zone)
	}
	properties := &armstoragecache.AmlFilesystemProperties{
		// EncryptionSettings: &armstoragecache.AmlFilesystemEncryptionSettings{
		// 	KeyEncryptionKey: &armstoragecache.KeyVaultKeyReference{
		// 		KeyURL: to.Ptr("https://examplekv.vault.azure.net/keys/kvk/3540a47df75541378d3518c6a4bdf5af"),
		// 		SourceVault: &armstoragecache.KeyVaultKeyReferenceSourceVault{
		// 			ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.KeyVault/vaults/keyvault-cmk"),
		// 		},
		// 	},
		// },
		FilesystemSubnet: to.Ptr(amlFilesystemProperties.SubnetInfo.SubnetID),
		// Hsm: &armstoragecache.AmlFilesystemPropertiesHsm{
		// 	Settings: &armstoragecache.AmlFilesystemHsmSettings{
		// 		Container:        to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.Storage/storageAccounts/storageaccountname/blobServices/default/containers/containername"),
		// 		ImportPrefix:     to.Ptr("/"),
		// 		LoggingContainer: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.Storage/storageAccounts/storageaccountname/blobServices/default/containers/loggingcontainername"),
		// 	},
		// },
		MaintenanceWindow: &armstoragecache.AmlFilesystemPropertiesMaintenanceWindow{
			DayOfWeek:    to.Ptr(amlFilesystemProperties.MaintenanceDayOfWeek),
			TimeOfDayUTC: to.Ptr(amlFilesystemProperties.TimeOfDayUTC),
		},
		StorageCapacityTiB: to.Ptr[float32](amlFilesystemProperties.StorageCapacityTiB),
	}
	amlFilesystem := armstoragecache.AmlFilesystem{
		Location:   to.Ptr(amlFilesystemProperties.Location),
		Tags:       tags,
		Properties: properties,
		Zones:      zones,
		SKU:        &armstoragecache.SKUName{Name: to.Ptr(amlFilesystemProperties.SKUName)},
	}
	if amlFilesystemProperties.Identities != nil {
		userAssignedIdentities := make(map[string]*armstoragecache.UserAssignedIdentitiesValue, len(amlFilesystemProperties.Identities))
		for _, identity := range amlFilesystemProperties.Identities {
			userAssignedIdentities[identity] = &armstoragecache.UserAssignedIdentitiesValue{}
		}
		amlFilesystem.Identity = &armstoragecache.AmlFilesystemIdentity{
			Type:                   to.Ptr(armstoragecache.AmlFilesystemIdentityTypeUserAssigned),
			UserAssignedIdentities: userAssignedIdentities,
		}
	}

	exists, err := d.ClusterExists(ctx, amlFilesystemProperties.ResourceGroupName, amlFilesystemProperties.AmlFilesystemName)
	klog.V(2).Infof("exists: %v, err: %v", exists, err)
	if !exists {
		hasSufficientCapacity, err := d.CheckSubnetCapacity(ctx, amlFilesystemProperties.SubnetInfo, amlFilesystemProperties.SKUName, amlFilesystemProperties.StorageCapacityTiB)
		klog.V(2).Infof("hasSufficientCapacity: %v, err: %v", hasSufficientCapacity, err)
		if err != nil {
			return "", err
		}
		if !hasSufficientCapacity {
			return "", status.Errorf(codes.ResourceExhausted, "cannot create AMLFS cluster %s in subnet %s, not enough IP addresses available",
				amlFilesystemProperties.AmlFilesystemName,
				amlFilesystemProperties.SubnetInfo.SubnetID,
			)
		}
	} else {
		// TODO: check if the existing cluster has the same configuration
		klog.V(2).Infof("amlfs cluster %s already exists, will attempt update request", amlFilesystemProperties.AmlFilesystemName)
	}

	poller, err := d.amlFilesystemsClient.BeginCreateOrUpdate(
		ctx,
		amlFilesystemProperties.ResourceGroupName,
		amlFilesystemProperties.AmlFilesystemName,
		amlFilesystem,
		nil)
	if err != nil {
		return "", convertHTTPResponseErrorToGrpcCodeError(err)
	}

	pollerOptions := &runtime.PollUntilDoneOptions{
		Frequency: d.pollFrequency,
	}
	res, err := poller.PollUntilDone(ctx, pollerOptions)
	if err != nil {
		klog.Warningf("failed to poll the result: %v", err)
		return "", convertHTTPResponseErrorToGrpcCodeError(err)
	}

	klog.V(2).Infof("response to dyn: %v", res)
	mgsAddress := *res.Properties.ClientInfo.MgsAddress
	return mgsAddress, nil
}

func (d *DynamicProvisioner) GetSkuValuesForLocation(ctx context.Context, location string) map[string]*LustreSkuValue {
	if d.skusClient == nil {
		klog.Warning("skus client is nil, using defaults")
		return d.defaultSkuValues
	}

	skusPager := d.skusClient.NewListPager(nil)
	skuValues := make(map[string]*LustreSkuValue)

	var amlfsSkus []*armstoragecache.ResourceSKU
	var skusForLocation []*armstoragecache.ResourceSKU

	for skusPager.More() {
		page, err := skusPager.NextPage(ctx)
		if err != nil {
			klog.Warningf("error getting SKUs for location %s, using defaults: %v", location, err)
			return d.defaultSkuValues
		}

		for _, sku := range page.Value {
			if *sku.ResourceType == AmlfsSkuResourceType {
				amlfsSkus = append(amlfsSkus, sku)
			}
		}
	}

	if len(amlfsSkus) == 0 {
		klog.Warning("no AMLFS SKUs found, using defaults")
		return d.defaultSkuValues
	}

	for _, sku := range amlfsSkus {
		for _, skuLocation := range sku.Locations {
			if strings.EqualFold(*skuLocation, location) {
				skusForLocation = append(skusForLocation, sku)
			}
		}
	}

	if len(skusForLocation) == 0 {
		klog.Warningf("found no AMLFS SKUs for location %s, using defaults", location)
		return d.defaultSkuValues
	}

	for _, sku := range skusForLocation {
		var incrementInTib int64
		var maximumInTib int64
		for _, capability := range sku.Capabilities {
			if *capability.Name == AmlfsSkuCapacityIncrementName {
				parsedValue, err := strconv.ParseInt(*capability.Value, 10, 64)
				if err != nil {
					klog.Warningf("failed to parse capability value: %v", err)
					continue
				}
				incrementInTib = parsedValue
			} else if *capability.Name == AmlfsSkuCapacityMaximumName {
				parsedValue, err := strconv.ParseInt(*capability.Value, 10, 64)
				if err != nil {
					klog.Warningf("failed to parse capability value: %v", err)
					continue
				}
				maximumInTib = parsedValue
			}
		}
		if incrementInTib != 0 && maximumInTib != 0 {
			skuValues[*sku.Name] = &LustreSkuValue{
				IncrementInTib: incrementInTib,
				MaximumInTib:   maximumInTib,
			}
			klog.Warningf("Adding sku value %s for location %s: %#v", *sku.Name, location, *skuValues[*sku.Name])
		}
	}

	if len(skuValues) == 0 {
		klog.Warningf("found no AMLFS SKUs for location %s, using defaults", location)
		return d.defaultSkuValues
	}

	klog.Warningf("Found SKU values for location %s: %#v", location, skuValues)
	return skuValues
}

func (d *DynamicProvisioner) getAmlfsSubnetSize(ctx context.Context, sku string, clusterSize float32) (int, error) {
	if d.mgmtClient == nil {
		return 0, status.Error(codes.Internal, "storage management client is nil")
	}

	reqSize, err := d.mgmtClient.GetRequiredAmlFSSubnetsSize(ctx, &armstoragecache.ManagementClientGetRequiredAmlFSSubnetsSizeOptions{
		RequiredAMLFilesystemSubnetsSizeInfo: &armstoragecache.RequiredAmlFilesystemSubnetsSizeInfo{
			SKU: &armstoragecache.SKUName{
				Name: to.Ptr(sku),
			},
			StorageCapacityTiB: to.Ptr[float32](clusterSize),
		},
	})
	if err != nil {
		return 0, convertHTTPResponseErrorToGrpcCodeError(err)
	}

	return int(*reqSize.RequiredAmlFilesystemSubnetsSize.FilesystemSubnetSize), nil
}

func (d *DynamicProvisioner) checkSubnetAddresses(ctx context.Context, vnetResourceGroup, vnetName, subnetID string) (int, error) {
	if d.vnetClient == nil {
		return 0, status.Error(codes.Internal, "vnet client is nil")
	}
	usagesPager := d.vnetClient.NewListUsagePager(vnetResourceGroup, vnetName, nil)

	klog.V(2).Infof("got pager for: %s, %s", vnetResourceGroup, vnetName)

	for usagesPager.More() {
		klog.V(2).Infof("getting next page")
		page, err := usagesPager.NextPage(ctx)
		if err != nil {
			klog.Errorf("error getting next page: %v", err)
			return 0, convertHTTPResponseErrorToGrpcCodeError(err)
		}

		klog.V(2).Infof("got page: %#v", page)
		for _, usageValue := range page.Value {
			klog.V(2).Infof("checking subnet: %s", *usageValue.ID)
			if *usageValue.ID == subnetID {
				klog.V(2).Infof("found subnet: %s", *usageValue.ID)
				usedIPs := *usageValue.CurrentValue
				limitIPs := *usageValue.Limit
				availableIPs := int(limitIPs) - int(usedIPs)
				return availableIPs, nil
			}
		}
	}
	klog.Warningf("subnet %s not found in vnet %s, resource group %s", subnetID, vnetName, vnetResourceGroup)
	return 0, status.Errorf(codes.FailedPrecondition, "subnet %s not found in vnet %s, resource group %s", subnetID, vnetName, vnetResourceGroup)
}

func (d *DynamicProvisioner) CheckSubnetCapacity(ctx context.Context, subnetInfo SubnetProperties, sku string, clusterSize float32) (bool, error) {
	requiredSubnetIPSize, err := d.getAmlfsSubnetSize(ctx, sku, clusterSize)
	if err != nil {
		return false, convertHTTPResponseErrorToGrpcCodeError(err)
	}
	klog.Warningf("Required IPs: %d", requiredSubnetIPSize)

	availableIPs, err := d.checkSubnetAddresses(ctx, subnetInfo.VnetResourceGroup, subnetInfo.VnetName, subnetInfo.SubnetID)
	if err != nil {
		return false, convertHTTPResponseErrorToGrpcCodeError(err)
	}
	klog.Warningf("Available IPs: %d", availableIPs)

	if requiredSubnetIPSize > availableIPs {
		klog.Warningf("There is not enough room in the %s subnetID to fit a %s SKU cluster: %v needed, %v available", subnetInfo.SubnetID, sku, requiredSubnetIPSize, availableIPs)
		return false, nil
	}
	klog.V(2).Infof("There is enough room in the %s subnetID to fit a %s SKU cluster: %v needed, %v available", subnetInfo.SubnetID, sku, requiredSubnetIPSize, availableIPs)
	return true, nil
}
