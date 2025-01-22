package azurelustre

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	networkfake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var recordedAmlfsConfigurations map[string]armstoragecache.AmlFilesystem

const (
	expectedMgsAddress              = "127.0.0.3"
	expectedResourceGroupName       = "fake-resource-group"
	expectedAmlFilesystemName       = "fake-amlfs"
	expectedLocation                = "fake-location"
	expectedAmlFilesystemSubnetSize = 24
	expectedUsedIPCount             = 10
	expectedFullIPCount             = 256
	expectedTotalIPCount            = 256
	expectedSku                     = "fake-sku"
	expectedClusterSize             = 48
	expectedSkuIncrement            = "4"
	expectedSkuMaximum              = "128"
	expectedVnetName                = "fake-vnet"
	expectedAmlFilesystemSubnetName = "fake-subnet-name"
	expectedAmlFilesystemSubnetID   = "fake-subnet-id"
	fullVnetName                    = "full-vnet"
	invalidSku                      = "invalid-sku"
	missingAmlFilesystemSubnetID    = "missing-subnet-id"
	vnetListUsageErrorName          = "vnet-list-usage-error"
	noSubnetInfoName                = "no-subnet-info"
	immediateCreateFailureName      = "immediate-create-failure"
	eventualCreateFailureName       = "eventual-create-failure"
	immediateDeleteFailureName      = "immediate-delete-failure"
	eventualDeleteFailureName       = "eventual-delete-failure"
	clusterGetFailureName           = "cluster-get-failure"
	errorLocation                   = "sku-error-location"
	noAmlfsSkus                     = "no-amlfs-skus"
	noAmlfsSkusForLocation          = "no-amlfs-skus-for-location"
	invalidSkuIncrement             = "invalid-sku-increment"
	invalidSkuMaximum               = "invalid-sku-maximum"

	quickPollFrequency = 1 * time.Millisecond
)

func buildExpectedSubnetInfo() SubnetProperties {
	return SubnetProperties{
		VnetResourceGroup: expectedResourceGroupName,
		VnetName:          expectedVnetName,
		SubnetName:        expectedAmlFilesystemSubnetName,
		SubnetID:          expectedAmlFilesystemSubnetID,
	}
}

func newTestDynamicProvisioner(t *testing.T) *DynamicProvisioner {
	dynamicProvisioner := &DynamicProvisioner{
		amlFilesystemsClient: newFakeAmlFilesystemsClient(t),
		vnetClient:           newFakeVnetClient(t),
		mgmtClient:           newFakeMgmtClient(t),
		skusClient:           newFakeSkusClient(t, ""),
		defaultSkuValues:     DefaultSkuValues,
		pollFrequency:        quickPollFrequency,
	}

	return dynamicProvisioner
}

func newFakeSkusClient(t *testing.T, failureBehavior string) *armstoragecache.SKUsClient {
	skusClientFactory, err := armstoragecache.NewClientFactory("fake-subscription-id", &azfake.TokenCredential{},
		&arm.ClientOptions{
			ClientOptions: azcore.ClientOptions{
				Transport: fake.NewSKUsServerTransport(newFakeSkusServer(t, failureBehavior)),
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, skusClientFactory)

	fakeSkusClient := skusClientFactory.NewSKUsClient()
	require.NotNil(t, fakeSkusClient)

	return fakeSkusClient
}

func newResourceSku(resourceType, skuName, location, increment, maximum string) *armstoragecache.ResourceSKU {
	resourceSku := &armstoragecache.ResourceSKU{
		ResourceType: to.Ptr(resourceType),
	}
	if resourceType == AmlfsSkuResourceType {
		resourceSku.Name = to.Ptr(skuName)
		resourceSku.Locations = []*string{to.Ptr(location)}
		resourceSku.Capabilities = []*armstoragecache.ResourceSKUCapabilities{
			{
				Name:  to.Ptr("OSS capacity increment (TiB)"),
				Value: to.Ptr(increment),
			},
			{
				Name:  to.Ptr("bandwidth increment (MB/s/TiB)"),
				Value: to.Ptr("500"),
			},
			{
				Name:  to.Ptr("durable"),
				Value: to.Ptr("True"),
			},
			{
				Name:  to.Ptr("MDS capacity increment (TiB)"),
				Value: to.Ptr("1024"),
			},
			{
				Name:  to.Ptr("default maximum capacity (TiB)"),
				Value: to.Ptr(maximum),
			},
			{
				Name:  to.Ptr("large cluster maximum capacity (TiB)"),
				Value: to.Ptr("1024"),
			},
			{
				Name:  to.Ptr("large cluster XL maximum capacity (TiB)"),
				Value: to.Ptr("1024"),
			},
		}
	}
	return resourceSku
}

func newFakeSkusServer(_ *testing.T, failureBehavior string) *fake.SKUsServer {
	fakeSkusServer := fake.SKUsServer{}

	fakeSkusServer.NewListPager = func(_ *armstoragecache.SKUsClientListOptions) azfake.PagerResponder[armstoragecache.SKUsClientListResponse] {
		resp := azfake.PagerResponder[armstoragecache.SKUsClientListResponse]{}
		if failureBehavior == errorLocation {
			resp.AddError(errors.New("fake location error"))
			return resp
		}
		if failureBehavior == noAmlfsSkus {
			resp.AddPage(http.StatusOK, armstoragecache.SKUsClientListResponse{
				ResourceSKUsResult: armstoragecache.ResourceSKUsResult{
					Value: []*armstoragecache.ResourceSKU{
						newResourceSku("caches", "", expectedLocation, "", ""),
					},
				},
			}, nil)
			return resp
		}
		if failureBehavior == noAmlfsSkusForLocation {
			otherSkuLocation := "other-sku-location"
			resp.AddPage(http.StatusOK, armstoragecache.SKUsClientListResponse{
				ResourceSKUsResult: armstoragecache.ResourceSKUsResult{
					Value: []*armstoragecache.ResourceSKU{
						newResourceSku(AmlfsSkuResourceType,
							expectedSku,
							otherSkuLocation,
							expectedSkuIncrement,
							expectedSkuMaximum),
					},
				},
			}, nil)
			return resp
		}
		if failureBehavior == invalidSkuIncrement {
			invalidSkuIncrementValue := "a"
			resp.AddPage(http.StatusOK, armstoragecache.SKUsClientListResponse{
				ResourceSKUsResult: armstoragecache.ResourceSKUsResult{
					Value: []*armstoragecache.ResourceSKU{
						newResourceSku(AmlfsSkuResourceType,
							expectedSku,
							expectedLocation,
							invalidSkuIncrementValue,
							expectedSkuMaximum),
					},
				},
			}, nil)
			return resp
		}
		if failureBehavior == invalidSkuMaximum {
			invalidSkuMaximumValue := "a"
			resp.AddPage(http.StatusOK, armstoragecache.SKUsClientListResponse{
				ResourceSKUsResult: armstoragecache.ResourceSKUsResult{
					Value: []*armstoragecache.ResourceSKU{
						newResourceSku(AmlfsSkuResourceType,
							expectedSku,
							expectedLocation,
							expectedSkuIncrement,
							invalidSkuMaximumValue),
					},
				},
			}, nil)
			return resp
		}
		resp.AddPage(http.StatusOK, armstoragecache.SKUsClientListResponse{
			ResourceSKUsResult: armstoragecache.ResourceSKUsResult{
				Value: []*armstoragecache.ResourceSKU{
					newResourceSku("caches", "", expectedLocation, "", ""),
					newResourceSku(AmlfsSkuResourceType,
						expectedSku,
						expectedLocation,
						expectedSkuIncrement,
						expectedSkuMaximum),
				},
			},
		}, nil)
		return resp
	}
	return &fakeSkusServer
}

func newFakeVnetClient(t *testing.T) *armnetwork.VirtualNetworksClient {
	vnetClientFactory, err := armnetwork.NewClientFactory("fake-subscription-id", &azfake.TokenCredential{},
		&arm.ClientOptions{
			ClientOptions: azcore.ClientOptions{
				Transport: networkfake.NewVirtualNetworksServerTransport(newFakeVnetServer(t)),
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, vnetClientFactory)

	fakeVnetClient := vnetClientFactory.NewVirtualNetworksClient()
	require.NotNil(t, fakeVnetClient)

	return fakeVnetClient
}

func newFakeVnetServer(_ *testing.T) *networkfake.VirtualNetworksServer {
	fakeVnetServer := networkfake.VirtualNetworksServer{}

	fakeVnetServer.NewListUsagePager = func(_, vnetName string, _ *armnetwork.VirtualNetworksClientListUsageOptions) azfake.PagerResponder[armnetwork.VirtualNetworksClientListUsageResponse] {
		resp := azfake.PagerResponder[armnetwork.VirtualNetworksClientListUsageResponse]{}

		if vnetName == vnetListUsageErrorName {
			resp.AddError(errors.New("fake vnet list usage error"))
			return resp
		}

		if vnetName == noSubnetInfoName {
			resp.AddPage(http.StatusOK, armnetwork.VirtualNetworksClientListUsageResponse{
				VirtualNetworkListUsageResult: armnetwork.VirtualNetworkListUsageResult{
					Value: []*armnetwork.VirtualNetworkUsage{},
				},
			}, nil)
			return resp
		}

		usedIPCount := expectedUsedIPCount
		if vnetName == fullVnetName {
			usedIPCount = expectedFullIPCount
		}
		resp.AddPage(http.StatusOK, armnetwork.VirtualNetworksClientListUsageResponse{
			VirtualNetworkListUsageResult: armnetwork.VirtualNetworkListUsageResult{
				Value: []*armnetwork.VirtualNetworkUsage{
					{
						ID:           to.Ptr(string("other-" + expectedAmlFilesystemName)),
						CurrentValue: to.Ptr(float64(usedIPCount)),
						Limit:        to.Ptr(float64(expectedTotalIPCount)),
					},
				},
			},
		}, nil)
		resp.AddPage(http.StatusOK, armnetwork.VirtualNetworksClientListUsageResponse{
			VirtualNetworkListUsageResult: armnetwork.VirtualNetworkListUsageResult{
				Value: []*armnetwork.VirtualNetworkUsage{
					{
						ID:           to.Ptr(string("another" + expectedAmlFilesystemSubnetID)),
						CurrentValue: to.Ptr(float64(usedIPCount)),
						Limit:        to.Ptr(float64(expectedTotalIPCount)),
					},
					{
						ID:           to.Ptr(string(expectedAmlFilesystemSubnetID)),
						CurrentValue: to.Ptr(float64(usedIPCount)),
						Limit:        to.Ptr(float64(expectedTotalIPCount)),
					},
				},
			},
		}, nil)
		return resp
	}
	return &fakeVnetServer
}

func newFakeMgmtClient(t *testing.T) *armstoragecache.ManagementClient {
	mgmtClientFactory, err := armstoragecache.NewClientFactory("fake-subscription-id", &azfake.TokenCredential{},
		&arm.ClientOptions{
			ClientOptions: azcore.ClientOptions{
				Transport: fake.NewManagementServerTransport(newFakeManagementServer(t)),
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, mgmtClientFactory)

	fakeMgmtClient := mgmtClientFactory.NewManagementClient()
	require.NotNil(t, fakeMgmtClient)

	return fakeMgmtClient
}

func newFakeManagementServer(_ *testing.T) *fake.ManagementServer {
	fakeMgmtServer := fake.ManagementServer{}

	fakeMgmtServer.GetRequiredAmlFSSubnetsSize = func(_ context.Context, options *armstoragecache.ManagementClientGetRequiredAmlFSSubnetsSizeOptions) (azfake.Responder[armstoragecache.ManagementClientGetRequiredAmlFSSubnetsSizeResponse], azfake.ErrorResponder) {
		errResp := azfake.ErrorResponder{}
		resp := azfake.Responder[armstoragecache.ManagementClientGetRequiredAmlFSSubnetsSizeResponse]{}
		if *options.RequiredAMLFilesystemSubnetsSizeInfo.SKU.Name == invalidSku {
			errResp.SetError(errors.New("fake invalid sku error"))
			return resp, errResp
		}
		resp.SetResponse(http.StatusOK, armstoragecache.ManagementClientGetRequiredAmlFSSubnetsSizeResponse{
			RequiredAmlFilesystemSubnetsSize: armstoragecache.RequiredAmlFilesystemSubnetsSize{
				FilesystemSubnetSize: to.Ptr(int32(expectedAmlFilesystemSubnetSize)),
			},
		}, nil)
		return resp, errResp
	}
	return &fakeMgmtServer
}

func newFakeAmlFilesystemsClient(t *testing.T) *armstoragecache.AmlFilesystemsClient {
	amlFilesystemsClientFactory, err := armstoragecache.NewClientFactory("fake-subscription-id", &azfake.TokenCredential{},
		&arm.ClientOptions{
			ClientOptions: azcore.ClientOptions{
				Transport: fake.NewAmlFilesystemsServerTransport(newFakeAmlFilesystemsServer(t)),
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, amlFilesystemsClientFactory)

	fakeAmlFilesystemsClient := amlFilesystemsClientFactory.NewAmlFilesystemsClient()
	require.NotNil(t, fakeAmlFilesystemsClient)

	return fakeAmlFilesystemsClient
}

func newFakeAmlFilesystemsServer(_ *testing.T) *fake.AmlFilesystemsServer {
	recordedAmlfsConfigurations = make(map[string]armstoragecache.AmlFilesystem)
	fakeAmlfsServer := fake.AmlFilesystemsServer{}

	fakeAmlfsServer.BeginDelete = func(_ context.Context, _, amlFilesystemName string, _ *armstoragecache.AmlFilesystemsClientBeginDeleteOptions) (azfake.PollerResponder[armstoragecache.AmlFilesystemsClientDeleteResponse], azfake.ErrorResponder) {
		errResp := azfake.ErrorResponder{}
		resp := azfake.PollerResponder[armstoragecache.AmlFilesystemsClientDeleteResponse]{}
		if amlFilesystemName == immediateDeleteFailureName {
			errResp.SetError(&azcore.ResponseError{StatusCode: http.StatusConflict})
			return resp, errResp
		}

		resp.AddNonTerminalResponse(http.StatusAccepted, nil)
		resp.AddNonTerminalResponse(http.StatusOK, nil)
		resp.AddNonTerminalResponse(http.StatusOK, nil)
		if amlFilesystemName == eventualDeleteFailureName {
			resp.SetTerminalError(http.StatusInternalServerError, eventualDeleteFailureName)
			return resp, errResp
		}

		delete(recordedAmlfsConfigurations, amlFilesystemName)
		resp.SetTerminalResponse(http.StatusOK, armstoragecache.AmlFilesystemsClientDeleteResponse{}, nil)

		return resp, errResp
	}

	fakeAmlfsServer.BeginCreateOrUpdate = func(_ context.Context, _, amlFilesystemName string, amlFilesystem armstoragecache.AmlFilesystem, _ *armstoragecache.AmlFilesystemsClientBeginCreateOrUpdateOptions) (azfake.PollerResponder[armstoragecache.AmlFilesystemsClientCreateOrUpdateResponse], azfake.ErrorResponder) {
		amlFilesystem.Name = to.Ptr(amlFilesystemName)
		amlFilesystem.Properties.ClientInfo = &armstoragecache.AmlFilesystemClientInfo{
			ContainerStorageInterface: (*armstoragecache.AmlFilesystemContainerStorageInterface)(nil),
			LustreVersion:             (*string)(nil),
			MgsAddress:                to.Ptr(expectedMgsAddress),
			MountCommand:              (*string)(nil),
		}

		errResp := azfake.ErrorResponder{}
		resp := azfake.PollerResponder[armstoragecache.AmlFilesystemsClientCreateOrUpdateResponse]{}
		if amlFilesystemName == immediateCreateFailureName {
			errResp.SetError(&azcore.ResponseError{StatusCode: http.StatusInternalServerError})
			return resp, errResp
		}

		resp.AddNonTerminalResponse(http.StatusCreated, nil)
		resp.AddNonTerminalResponse(http.StatusOK, nil)
		resp.AddNonTerminalResponse(http.StatusOK, nil)
		if amlFilesystemName == eventualCreateFailureName {
			resp.SetTerminalError(http.StatusRequestTimeout, eventualCreateFailureName)
			return resp, errResp
		}

		recordedAmlfsConfigurations[amlFilesystemName] = amlFilesystem
		resp.SetTerminalResponse(http.StatusOK, armstoragecache.AmlFilesystemsClientCreateOrUpdateResponse{
			AmlFilesystem: amlFilesystem,
		}, nil)
		return resp, errResp
	}

	fakeAmlfsServer.Get = func(_ context.Context, _, amlFilesystemName string, _ *armstoragecache.AmlFilesystemsClientGetOptions) (azfake.Responder[armstoragecache.AmlFilesystemsClientGetResponse], azfake.ErrorResponder) {
		var amlFilesystem *armstoragecache.AmlFilesystem
		errResp := azfake.ErrorResponder{}
		resp := azfake.Responder[armstoragecache.AmlFilesystemsClientGetResponse]{}
		if amlFilesystemName == clusterGetFailureName {
			errResp.SetError(errors.New(clusterGetFailureName))
			return resp, errResp
		}

		for _, amlfs := range recordedAmlfsConfigurations {
			if *amlfs.Name == amlFilesystemName {
				amlFilesystem = &amlfs
			}
		}
		if amlFilesystem == nil {
			errResp.SetError(errors.New("ResourceNotFound"))
			return resp, errResp
		}
		resp.SetResponse(http.StatusOK,
			armstoragecache.AmlFilesystemsClientGetResponse{
				AmlFilesystem: *amlFilesystem,
			}, nil)
		return resp, errResp
	}
	return &fakeAmlfsServer
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success(t *testing.T) {
	expectedLocation := "fake-location"
	expectedMaintenanceDayOfWeek := armstoragecache.MaintenanceDayOfWeekTypeSaturday
	expectedTimeOfDayUTC := "12:00"
	expectedStorageCapacityTiB := float32(48)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)

	mgsIPAddress, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName:    expectedResourceGroupName,
		AmlFilesystemName:    expectedAmlFilesystemName,
		Location:             expectedLocation,
		MaintenanceDayOfWeek: expectedMaintenanceDayOfWeek,
		TimeOfDayUTC:         expectedTimeOfDayUTC,
		SKUName:              expectedSku,
		StorageCapacityTiB:   expectedStorageCapacityTiB,
		SubnetInfo:           buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	assert.Equal(t, expectedMgsAddress, mgsIPAddress)
	require.Len(t, recordedAmlfsConfigurations, 1)
	actualAmlFilesystem := recordedAmlfsConfigurations[expectedAmlFilesystemName]
	assert.Equal(t, expectedLocation, *actualAmlFilesystem.Location)
	assert.Equal(t, expectedAmlFilesystemSubnetID, *actualAmlFilesystem.Properties.FilesystemSubnet)
	assert.Equal(t, expectedSku, *recordedAmlfsConfigurations[expectedAmlFilesystemName].SKU.Name)
	assert.Nil(t, recordedAmlfsConfigurations[expectedAmlFilesystemName].Identity)
	assert.Empty(t, recordedAmlfsConfigurations[expectedAmlFilesystemName].Zones)
	assert.Empty(t, recordedAmlfsConfigurations[expectedAmlFilesystemName].Tags)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_Tags(t *testing.T) {
	expectedTags := map[string]string{"tag1": "value1", "tag2": "value2"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		Tags:              expectedTags,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
	assert.Len(t, recordedAmlfsConfigurations[expectedAmlFilesystemName].Tags, len(expectedTags))
	for tagName, tagValue := range recordedAmlfsConfigurations[expectedAmlFilesystemName].Tags {
		assert.Equal(t, expectedTags[tagName], *tagValue)
	}
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_Zones(t *testing.T) {
	expectedZones := []string{"zone1", "zone2"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		Zones:             expectedZones,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
	assert.Len(t, recordedAmlfsConfigurations[expectedAmlFilesystemName].Zones, len(expectedZones))
	for zone := range recordedAmlfsConfigurations[expectedAmlFilesystemName].Zones {
		assert.Equal(t, expectedZones[zone], *recordedAmlfsConfigurations[expectedAmlFilesystemName].Zones[zone])
	}
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_Identities(t *testing.T) {
	expectedIdentities := []string{"identity1", "identity2"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		Identities:        expectedIdentities,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
	assert.Equal(t, armstoragecache.AmlFilesystemIdentityTypeUserAssigned, *recordedAmlfsConfigurations[expectedAmlFilesystemName].Identity.Type)
	assert.Len(t, recordedAmlfsConfigurations[expectedAmlFilesystemName].Identity.UserAssignedIdentities, len(expectedIdentities))
	for identityKey, identityValue := range recordedAmlfsConfigurations[expectedAmlFilesystemName].Identity.UserAssignedIdentities {
		assert.Equal(t, &armstoragecache.UserAssignedIdentitiesValue{}, identityValue)
		assert.Contains(t, expectedIdentities, identityKey)
	}
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_NilClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.amlFilesystemsClient = nil
	require.Empty(t, recordedAmlfsConfigurations)

	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        SubnetProperties{},
	})
	require.ErrorContains(t, err, "aml filesystem client is nil")
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_EmptySubnetInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        SubnetProperties{},
	})
	require.ErrorContains(t, err, "invalid subnet info")
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_EmptyInsufficientCapacity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	subnetProperties := buildExpectedSubnetInfo()
	subnetProperties.VnetName = fullVnetName

	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        subnetProperties,
	})
	require.ErrorContains(t, err, subnetProperties.SubnetID)
	require.ErrorContains(t, err, "not enough IP addresses available")
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_NoCapacityCheckIfClusterExistsBeforeCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	subnetProperties := buildExpectedSubnetInfo()

	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        subnetProperties,
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)

	exists, err := dynamicProvisioner.ClusterExists(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName)
	require.NoError(t, err)
	require.True(t, exists)

	subnetProperties.VnetName = fullVnetName

	require.Len(t, recordedAmlfsConfigurations, 1)
	_, err = dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        subnetProperties,
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_NoSubnetFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	subnetProperties := buildExpectedSubnetInfo()
	subnetProperties.VnetName = noSubnetInfoName

	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: eventualCreateFailureName,
		SubnetInfo:        subnetProperties,
	})
	require.ErrorContains(t, err, noSubnetInfoName)
	require.ErrorContains(t, err, "not found in vnet")
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_ImmediateFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := immediateCreateFailureName
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: amlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.ErrorContains(t, err, immediateCreateFailureName)
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_EventualFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := eventualCreateFailureName
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: amlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.ErrorContains(t, err, eventualCreateFailureName)
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_DeleteAmlFilesystem_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})

	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)

	err = dynamicProvisioner.DeleteAmlFilesystem(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName)

	require.NoError(t, err)
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_DeleteAmlFilesystem_Err_NilCLient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.amlFilesystemsClient = nil

	err := dynamicProvisioner.DeleteAmlFilesystem(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName)
	require.ErrorContains(t, err, "aml filesystem client is nil")
}

func TestDynamicProvisioner_DeleteAmlFilesystem_Err_ImmediateFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := immediateDeleteFailureName

	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: amlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)

	err = dynamicProvisioner.DeleteAmlFilesystem(context.Background(), expectedResourceGroupName, amlFilesystemName)
	require.ErrorContains(t, err, immediateDeleteFailureName)
	assert.Len(t, recordedAmlfsConfigurations, 1)
}

func TestDynamicProvisioner_DeleteAmlFilesystem_Err_EventualFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := eventualDeleteFailureName

	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: amlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)

	err = dynamicProvisioner.DeleteAmlFilesystem(context.Background(), expectedResourceGroupName, amlFilesystemName)
	require.ErrorContains(t, err, eventualDeleteFailureName)
	assert.Len(t, recordedAmlfsConfigurations, 1)
}

func TestDynamicProvisioner_DeleteAmlFilesystem_SuccessMultiple(t *testing.T) {
	otherAmlFilesystemName := expectedAmlFilesystemName + "2"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	_, err = dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: otherAmlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 2)

	err = dynamicProvisioner.DeleteAmlFilesystem(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName)
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
	assert.Equal(t, otherAmlFilesystemName, *recordedAmlfsConfigurations[otherAmlFilesystemName].Name)
}

func TestDynamicProvisioner_ClusterExists_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)

	exists, err := dynamicProvisioner.ClusterExists(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestDynamicProvisioner_ClusterExists_SuccessNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	exists, err := dynamicProvisioner.ClusterExists(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDynamicProvisioner_ClusterExists_Err(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := clusterGetFailureName
	_, err := dynamicProvisioner.ClusterExists(context.Background(), expectedResourceGroupName, amlFilesystemName)
	assert.ErrorContains(t, err, clusterGetFailureName)
}

func TestDynamicProvisioner_ClusterExists_ErrNilClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.amlFilesystemsClient = nil

	_, err := dynamicProvisioner.ClusterExists(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName)
	assert.ErrorContains(t, err, "aml filesystem client is nil")
}

func TestDynamicProvisioner_CheckSubnetCapacity_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)

	hasSufficientCapacity, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), buildExpectedSubnetInfo(), expectedSku, expectedClusterSize)
	require.NoError(t, err)
	assert.True(t, hasSufficientCapacity)
}

func TestDynamicProvisioner_CheckSubnetCapacity_FullVnet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)

	subnetInfo := buildExpectedSubnetInfo()
	subnetInfo.VnetName = fullVnetName
	hasSufficientCapacity, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), subnetInfo, expectedSku, expectedClusterSize)
	require.NoError(t, err)
	assert.False(t, hasSufficientCapacity)
}

func TestDynamicProvisioner_CheckSubnetCapacity_Err_NilMgmtClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.mgmtClient = nil

	_, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), buildExpectedSubnetInfo(), expectedSku, expectedClusterSize)
	assert.ErrorContains(t, err, "storage management client is nil")
}

func TestDynamicProvisioner_CheckSubnetCapacity_Err_NilVnetClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.vnetClient = nil

	_, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), buildExpectedSubnetInfo(), expectedSku, expectedClusterSize)
	assert.ErrorContains(t, err, "vnet client is nil")
}

func TestDynamicProvisioner_CheckSubnetCapacity_Err_InvalidSku(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	sku := invalidSku
	_, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), buildExpectedSubnetInfo(), sku, expectedClusterSize)
	assert.ErrorContains(t, err, "fake invalid sku error")
}

func TestDynamicProvisioner_CheckSubnetCapacity_Err_ListUsageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)

	subnetInfo := buildExpectedSubnetInfo()
	subnetInfo.VnetName = vnetListUsageErrorName
	_, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), subnetInfo, expectedSku, expectedClusterSize)
	assert.ErrorContains(t, err, "fake vnet list usage error")
}

func TestDynamicProvisioner_CheckSubnetCapacity_Err_SubnetNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)

	subnetInfo := buildExpectedSubnetInfo()
	subnetInfo.SubnetID = missingAmlFilesystemSubnetID
	_, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), subnetInfo, expectedSku, expectedClusterSize)
	assert.ErrorContains(t, err, missingAmlFilesystemSubnetID+" not found in vnet")
}

func TestDynamicProvisioner_GetSkuValuesForLocation_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)

	expectedSkuValues := map[string]*LustreSkuValue{expectedSku: {IncrementInTib: 4, MaximumInTib: 128}}

	skuValues := dynamicProvisioner.GetSkuValuesForLocation(context.Background(), expectedLocation)
	t.Log(skuValues)
	require.Len(t, skuValues, 1)
	assert.Equal(t, expectedSkuValues, skuValues)
}

func TestDynamicProvisioner_GetSkuValuesForLocation_NilClientReturnsDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.skusClient = nil

	skuValues := dynamicProvisioner.GetSkuValuesForLocation(context.Background(), expectedLocation)
	t.Log(skuValues)
	require.Len(t, skuValues, 4)
	assert.Equal(t, DefaultSkuValues, skuValues)
	assert.NotContains(t, skuValues, expectedSku)
}

func TestDynamicProvisioner_GetSkuValuesForLocation_ErrorResponseReturnsDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.skusClient = newFakeSkusClient(t, errorLocation)

	skuValues := dynamicProvisioner.GetSkuValuesForLocation(context.Background(), expectedLocation)
	t.Log(skuValues)
	require.Len(t, skuValues, 4)
	assert.Equal(t, DefaultSkuValues, skuValues)
	assert.NotContains(t, skuValues, expectedSku)
}

func TestDynamicProvisioner_GetSkuValuesForLocation_NoAmlfsSkusReturnsDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.skusClient = newFakeSkusClient(t, noAmlfsSkus)

	skuValues := dynamicProvisioner.GetSkuValuesForLocation(context.Background(), expectedLocation)
	t.Log(skuValues)
	require.Len(t, skuValues, 4)
	assert.Equal(t, DefaultSkuValues, skuValues)
	assert.NotContains(t, skuValues, expectedSku)
}

func TestDynamicProvisioner_GetSkuValuesForLocation_NoAmlfsSkusForLocationReturnsDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.skusClient = newFakeSkusClient(t, noAmlfsSkusForLocation)

	skuValues := dynamicProvisioner.GetSkuValuesForLocation(context.Background(), expectedLocation)
	t.Log(skuValues)
	require.Len(t, skuValues, 4)
	assert.Equal(t, DefaultSkuValues, skuValues)
	assert.NotContains(t, skuValues, expectedSku)
}

func TestDynamicProvisioner_GetSkuValuesForLocation_SkipsInvalidSkuIncrement(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.skusClient = newFakeSkusClient(t, invalidSkuIncrement)

	skuValues := dynamicProvisioner.GetSkuValuesForLocation(context.Background(), expectedLocation)
	t.Log(skuValues)
	require.Len(t, skuValues, 4)
	assert.Equal(t, DefaultSkuValues, skuValues)
	assert.NotContains(t, skuValues, expectedSku)
}

func TestDynamicProvisioner_GetSkuValuesForLocation_SkipsInvalidSkuMaximum(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newTestDynamicProvisioner(t)
	dynamicProvisioner.skusClient = newFakeSkusClient(t, invalidSkuMaximum)

	skuValues := dynamicProvisioner.GetSkuValuesForLocation(context.Background(), expectedLocation)
	t.Log(skuValues)
	require.Len(t, skuValues, 4)
	assert.Equal(t, DefaultSkuValues, skuValues)
	assert.NotContains(t, skuValues, expectedSku)
}

func TestConvertStatusCodeErrorToGrpcCodeError(t *testing.T) {
	tests := []struct {
		name         string
		inputError   error
		expectedCode codes.Code
	}{
		{
			name:         "BadRequest",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusBadRequest},
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "Conflict with quota limit exceeded",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusConflict, ErrorCode: "Operation results in exceeding quota limits of resource type AmlFilesystem"},
			expectedCode: codes.ResourceExhausted,
		},
		{
			name:         "Conflict without quota limit exceeded",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusConflict},
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "NotFound",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusNotFound},
			expectedCode: codes.NotFound,
		},
		{
			name:         "Forbidden",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusForbidden},
			expectedCode: codes.PermissionDenied,
		},
		{
			name:         "Unauthorized",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusUnauthorized},
			expectedCode: codes.Unauthenticated,
		},
		{
			name:         "TooManyRequests",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusTooManyRequests},
			expectedCode: codes.Unavailable,
		},
		{
			name:         "InternalServerError",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusInternalServerError},
			expectedCode: codes.Internal,
		},
		{
			name:         "BadGateway",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusBadGateway},
			expectedCode: codes.Unavailable,
		},
		{
			name:         "ServiceUnavailable",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable},
			expectedCode: codes.Unavailable,
		},
		{
			name:         "GatewayTimeout",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusGatewayTimeout},
			expectedCode: codes.DeadlineExceeded,
		},
		{
			name:         "UnknownClientError",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusTeapot},
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "UnknownServerError",
			inputError:   &azcore.ResponseError{StatusCode: http.StatusLoopDetected},
			expectedCode: codes.Unknown,
		},
		{
			name:         "GrpcError",
			inputError:   status.Error(codes.DeadlineExceeded, "test error"),
			expectedCode: codes.DeadlineExceeded,
		},
		{
			name:       "NilError",
			inputError: nil,
		},
		{
			name:         "NonResponseError",
			inputError:   errors.New("some other error"),
			expectedCode: codes.Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := convertHTTPResponseErrorToGrpcCodeError(tt.inputError)
			if tt.inputError == nil {
				require.NoError(t, err)
				return
			}
			status, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, tt.expectedCode, status.Code())
		})
	}
}
