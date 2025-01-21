package azurelustre

import (
	"context"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type DynamicProvisionerInterface interface {
	DeleteAmlFilesystem(ctx context.Context, resourceGroupName, amlFilesystemName string) error
	CreateAmlFilesystem(ctx context.Context, amlFilesystemProperties *AmlFilesystemProperties) (string, error)
	ClusterExists(ctx context.Context, resourceGroupName, amlFilesystemName string) (bool, error)
}

type DynamicProvisioner struct {
	DynamicProvisionerInterface
	amlFilesystemsClient *armstoragecache.AmlFilesystemsClient
	mgmtClient           *armstoragecache.ManagementClient
	vnetClient           *armnetwork.VirtualNetworksClient
	pollFrequency        time.Duration
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
		return false, err
	}
	klog.V(2).Infof("response when retrieving the aml filesystem: %#v", resp)
	return true, nil
}

func (d *DynamicProvisioner) DeleteAmlFilesystem(ctx context.Context, resourceGroupName, amlFilesystemName string) error {
	if d.amlFilesystemsClient == nil {
		return status.Error(codes.Internal, "aml filesystem client is nil")
	}

	exists, err := d.ClusterExists(ctx, resourceGroupName, amlFilesystemName)
	klog.V(2).Infof("exists: %v, err: %v", exists, err)

	poller, err := d.amlFilesystemsClient.BeginDelete(ctx, resourceGroupName, amlFilesystemName, nil)
	if err != nil {
		klog.Warningf("failed to finish the request: %v", err)
		return err
	}

	pollerOptions := &runtime.PollUntilDoneOptions{
		Frequency: d.pollFrequency,
	}
	res, err := poller.PollUntilDone(ctx, pollerOptions)
	if err != nil {
		klog.Warningf("failed to poll the result: %v", err)
		return err
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
	if amlFilesystemProperties.RootSquashSettings != nil {
		properties.RootSquashSettings = &armstoragecache.AmlFilesystemRootSquashSettings{
			Mode: to.Ptr(amlFilesystemProperties.RootSquashSettings.SquashMode),
		}
		if amlFilesystemProperties.RootSquashSettings.SquashMode != armstoragecache.AmlFilesystemSquashModeNone {
			properties.RootSquashSettings.NoSquashNidLists = to.Ptr(amlFilesystemProperties.RootSquashSettings.NoSquashNidLists)
			properties.RootSquashSettings.SquashUID = to.Ptr(amlFilesystemProperties.RootSquashSettings.SquashUID)
			properties.RootSquashSettings.SquashGID = to.Ptr(amlFilesystemProperties.RootSquashSettings.SquashGID)
		}
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
		klog.Warningf("failed to finish the request: %v", err)
		return "", err
	}

	pollerOptions := &runtime.PollUntilDoneOptions{
		Frequency: d.pollFrequency,
	}
	res, err := poller.PollUntilDone(ctx, pollerOptions)
	if err != nil {
		klog.Warningf("failed to poll the result: %v", err)
		return "", err
	}

	klog.V(2).Infof("response to dyn: %v", res)
	mgsAddress := *res.Properties.ClientInfo.MgsAddress
	return mgsAddress, nil
}

func getAmlfsSubnetSize(ctx context.Context, sku string, clusterSize float32, mgmtClient *armstoragecache.ManagementClient) (int, error) {
	reqSize, err := mgmtClient.GetRequiredAmlFSSubnetsSize(ctx, &armstoragecache.ManagementClientGetRequiredAmlFSSubnetsSizeOptions{
		RequiredAMLFilesystemSubnetsSizeInfo: &armstoragecache.RequiredAmlFilesystemSubnetsSizeInfo{
			SKU: &armstoragecache.SKUName{
				Name: to.Ptr(sku),
			},
			StorageCapacityTiB: to.Ptr[float32](clusterSize),
		},
	})
	if err != nil {
		return 0, err
	}

	return int(*reqSize.RequiredAmlFilesystemSubnetsSize.FilesystemSubnetSize), nil
}

func checkSubnetAddresses(ctx context.Context, vnetResourceGroup, vnetName, subnetID string, vnetClient *armnetwork.VirtualNetworksClient) (int, error) {
	usagesPager := vnetClient.NewListUsagePager(vnetResourceGroup, vnetName, nil)

	klog.V(2).Infof("got pager for: %s, %s", vnetResourceGroup, vnetName)

	for usagesPager.More() {
		klog.V(2).Infof("getting next page")
		page, err := usagesPager.NextPage(ctx)
		if err != nil {
			klog.Errorf("error getting next page: %v", err)
			return 0, err
		}

		klog.V(2).Infof("got page: %v", page)
		for i := range page.Value {
			klog.V(2).Infof("checking subnet: %s", *page.Value[i].ID)
			currentSubnet := page.Value[i]
			if *currentSubnet.ID == subnetID {
				klog.V(2).Infof("found subnet: %s", *currentSubnet.ID)
				usedIPs := *currentSubnet.CurrentValue
				limitIPs := *currentSubnet.Limit
				availableIPs := int(limitIPs) - int(usedIPs)
				return availableIPs, nil
			}
		}
	}
	klog.Warningf("subnet %s not found in vnet %s, resource group %s", subnetID, vnetName, vnetResourceGroup)
	return 0, status.Errorf(codes.FailedPrecondition, "subnet %s not found in vnet %s, resource group %s", subnetID, vnetName, vnetResourceGroup)
}

func (d *DynamicProvisioner) CheckSubnetCapacity(ctx context.Context, subnetInfo SubnetProperties, sku string, clusterSize float32) (bool, error) {
	requiredSubnetIPSize, err := getAmlfsSubnetSize(ctx, sku, clusterSize, d.mgmtClient)
	if err != nil {
		return false, err
	}
	klog.Warningf("Required IPs: %d\n", requiredSubnetIPSize)

	availableIPs, err := checkSubnetAddresses(ctx, subnetInfo.VnetResourceGroup, subnetInfo.VnetName, subnetInfo.SubnetID, d.vnetClient)
	if err != nil {
		return false, err
	}
	klog.Warningf("Available IPs: %d\n", availableIPs)

	if requiredSubnetIPSize > availableIPs {
		klog.Warningf("There is not enough room in the %s subnetID to fit a %s SKU cluster.\n", subnetInfo.SubnetID, sku)
		return false, nil
	}
	klog.Warningf("There is enough room in the %s subnetID to fit a %s SKU cluster.\n", subnetInfo.SubnetID, sku)
	return true, nil
}
