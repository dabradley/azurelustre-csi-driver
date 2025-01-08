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
)

var recordedAmlfsConfigurations []armstoragecache.AmlFilesystem

const (
	expectedMgsAddress              = "127.0.0.3"
	expectedResourceGroupName       = "fake-resource-group"
	expectedAmlFilesystemName       = "fake-amlfs"
	expectedAmlFilesystemSubnetSize = 24
	expectedUsedIPCount             = 10
	expectedFullIPCount             = 256
	expectedTotalIPCount            = 256
	expectedSku                     = "fake-sku"
	expectedClusterSize             = 48
	expectedVnetName                = "fake-vnet"
	expectedAmlFilesystemSubnetName = "fake-subnet-name"
	expectedAmlFilesystemSubnetID   = "fake-subnet-id"
	fullVnetName                    = "full-vnet"
	invalidSku                      = "invalid-sku"
	missingAmlFilesystemSubnetID    = "missing-subnet-id"
	vnetListUsageErrorName          = "vnet-list-usage-error"
	immediateCreateFailureName      = "immediate-create-failure"
	eventualCreateFailureName       = "eventual-create-failure"
	immediateDeleteFailureName      = "immediate-delete-failure"
	eventualDeleteFailureName       = "eventual-delete-failure"
	clusterGetFailureName           = "cluster-get-failure"

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

func newDynamicProvisioner(t *testing.T) *DynamicProvisioner {
	dynamicProvisioner := &DynamicProvisioner{
		amlFilesystemsClient: newFakeAmlFilesystemsClient(t),
		vnetClient:           newFakeVnetClient(t),
		mgmtClient:           newFakeMgmtClient(t),
		pollFrequency:        quickPollFrequency,
	}

	return dynamicProvisioner
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
	recordedAmlfsConfigurations = []armstoragecache.AmlFilesystem{}
	fakeAmlfsServer := fake.AmlFilesystemsServer{}

	fakeAmlfsServer.BeginDelete = func(_ context.Context, _, amlFilesystemName string, _ *armstoragecache.AmlFilesystemsClientBeginDeleteOptions) (azfake.PollerResponder[armstoragecache.AmlFilesystemsClientDeleteResponse], azfake.ErrorResponder) {
		errResp := azfake.ErrorResponder{}
		resp := azfake.PollerResponder[armstoragecache.AmlFilesystemsClientDeleteResponse]{}
		if amlFilesystemName == immediateDeleteFailureName {
			errResp.SetError(errors.New("fake immediate delete error"))
			return resp, errResp
		}

		resp.AddNonTerminalResponse(http.StatusAccepted, nil)
		resp.AddNonTerminalResponse(http.StatusOK, nil)
		resp.AddNonTerminalResponse(http.StatusOK, nil)
		if amlFilesystemName == eventualDeleteFailureName {
			resp.SetTerminalError(http.StatusRequestTimeout, "fake eventual delete error")
			return resp, errResp
		}

		for i, amlfs := range recordedAmlfsConfigurations {
			if *amlfs.Name == amlFilesystemName {
				recordedAmlfsConfigurations = append(recordedAmlfsConfigurations[:i], recordedAmlfsConfigurations[i+1:]...)
				break
			}
		}
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
			errResp.SetError(errors.New("fake immediate create error"))
			return resp, errResp
		}

		resp.AddNonTerminalResponse(http.StatusCreated, nil)
		resp.AddNonTerminalResponse(http.StatusOK, nil)
		resp.AddNonTerminalResponse(http.StatusOK, nil)
		if amlFilesystemName == eventualCreateFailureName {
			resp.SetTerminalError(http.StatusRequestTimeout, "fake eventual create error")
			return resp, errResp
		}

		recordedAmlfsConfigurations = append(recordedAmlfsConfigurations, amlFilesystem)
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

	dynamicProvisioner := newDynamicProvisioner(t)

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
	actualAmlFilesystem := recordedAmlfsConfigurations[0]
	assert.Equal(t, expectedLocation, *actualAmlFilesystem.Location)
	assert.Equal(t, expectedAmlFilesystemSubnetID, *actualAmlFilesystem.Properties.FilesystemSubnet)
	assert.Equal(t, expectedSku, *recordedAmlfsConfigurations[0].SKU.Name)
	assert.Nil(t, recordedAmlfsConfigurations[0].Identity)
	assert.Nil(t, recordedAmlfsConfigurations[0].Properties.RootSquashSettings)
	assert.Empty(t, recordedAmlfsConfigurations[0].Zones)
	assert.Empty(t, recordedAmlfsConfigurations[0].Tags)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_SendsNilPropertiesForRootSquashMode(t *testing.T) {
	expectedSquashModes := []armstoragecache.AmlFilesystemSquashMode{armstoragecache.AmlFilesystemSquashModeRootOnly, armstoragecache.AmlFilesystemSquashModeAll}
	expectedNIDList := "fake-nid-list"
	expectedSuashUID := int64(3000)
	expectedSquashGID := int64(4000)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, squashMode := range expectedSquashModes {
		t.Run(string(squashMode), func(t *testing.T) {
			dynamicProvisioner := newDynamicProvisioner(t)

			_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
				ResourceGroupName: expectedResourceGroupName,
				AmlFilesystemName: expectedAmlFilesystemName,
				SubnetInfo:        buildExpectedSubnetInfo(),
				RootSquashSettings: &RootSquashSettings{
					SquashMode:       squashMode,
					NoSquashNidLists: expectedNIDList,
					SquashUID:        expectedSuashUID,
					SquashGID:        expectedSquashGID,
				},
			})

			require.NoError(t, err)
			require.Len(t, recordedAmlfsConfigurations, 1)
			require.NotNil(t, recordedAmlfsConfigurations[0].Properties.RootSquashSettings)
			assert.Equal(t, squashMode, *recordedAmlfsConfigurations[0].Properties.RootSquashSettings.Mode)
			assert.Equal(t, expectedNIDList, *recordedAmlfsConfigurations[0].Properties.RootSquashSettings.NoSquashNidLists)
			assert.Equal(t, expectedSuashUID, *recordedAmlfsConfigurations[0].Properties.RootSquashSettings.SquashUID)
			assert.Equal(t, expectedSquashGID, *recordedAmlfsConfigurations[0].Properties.RootSquashSettings.SquashGID)
		})
	}
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_SendsNilPropertiesForRootSquashModeNone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
		RootSquashSettings: &RootSquashSettings{
			SquashMode: armstoragecache.AmlFilesystemSquashModeNone,
		},
	},
	)
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
	require.NotNil(t, recordedAmlfsConfigurations[0].Properties.RootSquashSettings)
	assert.Equal(t, armstoragecache.AmlFilesystemSquashModeNone, *recordedAmlfsConfigurations[0].Properties.RootSquashSettings.Mode)
	assert.Nil(t, recordedAmlfsConfigurations[0].Properties.RootSquashSettings.NoSquashNidLists)
	assert.Nil(t, recordedAmlfsConfigurations[0].Properties.RootSquashSettings.SquashUID)
	assert.Nil(t, recordedAmlfsConfigurations[0].Properties.RootSquashSettings.SquashGID)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_Tags(t *testing.T) {
	expectedTags := map[string]string{"tag1": "value1", "tag2": "value2"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		Tags:              expectedTags,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
	assert.Len(t, recordedAmlfsConfigurations[0].Tags, len(expectedTags))
	for tagName, tagValue := range recordedAmlfsConfigurations[0].Tags {
		assert.Equal(t, expectedTags[tagName], *tagValue)
	}
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_Zones(t *testing.T) {
	expectedZones := []string{"zone1", "zone2"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		Zones:             expectedZones,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
	assert.Len(t, recordedAmlfsConfigurations[0].Zones, len(expectedZones))
	for zone := range recordedAmlfsConfigurations[0].Zones {
		assert.Equal(t, expectedZones[zone], *recordedAmlfsConfigurations[0].Zones[zone])
	}
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success_Identities(t *testing.T) {
	expectedIdentities := []string{"identity1", "identity2"}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: expectedAmlFilesystemName,
		Identities:        expectedIdentities,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.NoError(t, err)
	require.Len(t, recordedAmlfsConfigurations, 1)
	assert.Equal(t, armstoragecache.AmlFilesystemIdentityTypeUserAssigned, *recordedAmlfsConfigurations[0].Identity.Type)
	assert.Len(t, recordedAmlfsConfigurations[0].Identity.UserAssignedIdentities, len(expectedIdentities))
	for identityKey, identityValue := range recordedAmlfsConfigurations[0].Identity.UserAssignedIdentities {
		assert.Equal(t, &armstoragecache.UserAssignedIdentitiesValue{}, identityValue)
		assert.Contains(t, expectedIdentities, identityKey)
	}
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_EmptySubnetInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := immediateCreateFailureName
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: amlFilesystemName,
		SubnetInfo:        SubnetProperties{},
	})
	require.ErrorContains(t, err, "invalid subnet info")
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_ImmediateFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := immediateCreateFailureName
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: amlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.ErrorContains(t, err, "fake immediate create error")
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Err_EventualFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := eventualCreateFailureName
	_, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: expectedResourceGroupName,
		AmlFilesystemName: amlFilesystemName,
		SubnetInfo:        buildExpectedSubnetInfo(),
	})
	require.ErrorContains(t, err, "fake eventual create error")
	assert.Empty(t, recordedAmlfsConfigurations)
}

func TestDynamicProvisioner_DeleteAmlFilesystem_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
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

func TestDynamicProvisioner_DeleteAmlFilesystem_Err_ImmediateFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
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
	require.ErrorContains(t, err, "fake immediate delete error")
	assert.Len(t, recordedAmlfsConfigurations, 1)
}

func TestDynamicProvisioner_DeleteAmlFilesystem_Err_EventualFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
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
	require.ErrorContains(t, err, "fake eventual delete error")
	assert.Len(t, recordedAmlfsConfigurations, 1)
}

func TestDynamicProvisioner_DeleteAmlFilesystem_SuccessMultiple(t *testing.T) {
	otherAmlFilesystemName := expectedAmlFilesystemName + "2"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
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
	assert.Equal(t, otherAmlFilesystemName, *recordedAmlfsConfigurations[0].Name)
}

func TestDynamicProvisioner_GetAmlFilesystem_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
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

func TestDynamicProvisioner_GetAmlFilesystem_SuccessNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	exists, err := dynamicProvisioner.ClusterExists(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDynamicProvisioner_GetAmlFilesystem_Err(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	amlFilesystemName := clusterGetFailureName
	_, err := dynamicProvisioner.ClusterExists(context.Background(), expectedResourceGroupName, amlFilesystemName)
	assert.ErrorContains(t, err, clusterGetFailureName)
}

func TestDynamicProvisioner_CheckSubnetCapacity_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	hasSufficientCapacity, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), buildExpectedSubnetInfo(), expectedSku, expectedClusterSize)
	require.NoError(t, err)
	assert.True(t, hasSufficientCapacity)
}

func TestDynamicProvisioner_CheckSubnetCapacity_FullVnet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	subnetInfo := buildExpectedSubnetInfo()
	subnetInfo.VnetName = fullVnetName
	hasSufficientCapacity, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), subnetInfo, expectedSku, expectedClusterSize)
	require.NoError(t, err)
	assert.False(t, hasSufficientCapacity)
}

func TestDynamicProvisioner_CheckSubnetCapacity_Err_InvalidSku(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	sku := invalidSku
	_, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), buildExpectedSubnetInfo(), sku, expectedClusterSize)
	assert.ErrorContains(t, err, "fake invalid sku error")
}

func TestDynamicProvisioner_CheckSubnetCapacity_Err_ListUsageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	subnetInfo := buildExpectedSubnetInfo()
	subnetInfo.VnetName = vnetListUsageErrorName
	_, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), subnetInfo, expectedSku, expectedClusterSize)
	assert.ErrorContains(t, err, "fake vnet list usage error")
}

func TestDynamicProvisioner_CheckSubnetCapacity_Err_SubnetNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dynamicProvisioner := newDynamicProvisioner(t)
	require.Empty(t, recordedAmlfsConfigurations)

	subnetInfo := buildExpectedSubnetInfo()
	subnetInfo.SubnetID = missingAmlFilesystemSubnetID
	_, err := dynamicProvisioner.CheckSubnetCapacity(context.Background(), subnetInfo, expectedSku, expectedClusterSize)
	assert.ErrorContains(t, err, missingAmlFilesystemSubnetID+" not found in vnet")
}
