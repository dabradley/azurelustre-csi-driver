package azurelustre

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newFakeAmlFilesystemsServer(t *testing.T, expectedMgsAddress string) *fake.AmlFilesystemsServer {
	// var recordedAmlfsConfiguration armstoragecache.AmlFilesystem
	fakeAmlfsServer := fake.AmlFilesystemsServer{}
	// fakeAmlfs.BeginCreateOrUpdate = func(ctx context.Context, resourceGroupName string, amlFilesystemName string, amlFilesystem armstoragecache.AmlFilesystem, options *armstoragecache.AmlFilesystemsClientBeginCreateOrUpdateOptions) (resp azfake.PollerResponder[armstoragecache.AmlFilesystemsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
	fakeAmlfsServer.BeginCreateOrUpdate = func(_ context.Context, resourceGroupName, amlFilesystemName string, amlFilesystem armstoragecache.AmlFilesystem, _ *armstoragecache.AmlFilesystemsClientBeginCreateOrUpdateOptions) (azfake.PollerResponder[armstoragecache.AmlFilesystemsClientCreateOrUpdateResponse], azfake.ErrorResponder) {
		// recordedAmlfsConfiguration = amlFilesystem
		t.Logf("resource group: %v", resourceGroupName)
		t.Logf("aml filesystem name: %v", amlFilesystemName)
		t.Logf("aml filesystem: %v", amlFilesystem)

		responseFromServer := amlFilesystem
		responseFromServer.Name = to.Ptr(amlFilesystemName)
		responseFromServer.Properties.ClientInfo = &armstoragecache.AmlFilesystemClientInfo{
			ContainerStorageInterface: (*armstoragecache.AmlFilesystemContainerStorageInterface)(nil),
			LustreVersion:             (*string)(nil),
			MgsAddress:                to.Ptr(expectedMgsAddress),
			MountCommand:              (*string)(nil),
		}

		resp := azfake.PollerResponder[armstoragecache.AmlFilesystemsClientCreateOrUpdateResponse]{}
		resp.SetTerminalResponse(http.StatusOK, armstoragecache.AmlFilesystemsClientCreateOrUpdateResponse{
			AmlFilesystem: responseFromServer,
		}, nil)
		errResp := azfake.ErrorResponder{}
		return resp, errResp
	}

	fakeAmlfsServer.NewListByResourceGroupPager = func(string, *armstoragecache.AmlFilesystemsClientListByResourceGroupOptions) azfake.PagerResponder[armstoragecache.AmlFilesystemsClientListByResourceGroupResponse] {
		return azfake.PagerResponder[armstoragecache.AmlFilesystemsClientListByResourceGroupResponse]{}
	}

	// fakeAmlfs.Get = func(ctx context.Context, resource-groupName string, vmName string, options *armcompute.VirtualMachinesClientGetOptions) (resp azfake.Responder[armcompute.VirtualMachinesClientGetResponse], errResp azfake.ErrorResponder) {
	// 	// TODO: resp.SetResponse(/* your fake armcompute.VirtualMachinesClientGetResponse response */)
	// 	return
	// }
	return &fakeAmlfsServer
}

// func TestDynamicProvisioner_DeleteAmlFilesystem(t *testing.T) {
// 	type fields struct {
// 		DynamicProvisionerInterface DynamicProvisionerInterface
// 		amlFilesystemsClient        *armstoragecache.AmlFilesystemsClient
// 	}
// 	type args struct {
// 		cxt               context.Context
// 		resourceGroupName string
// 		amlFilesystemName string
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			d := &DynamicProvisioner{
// 				DynamicProvisionerInterface: tt.fields.DynamicProvisionerInterface,
// 				amlFilesystemsClient:        tt.fields.amlFilesystemsClient,
// 			}
// 			if err := d.DeleteAmlFilesystem(tt.args.cxt, tt.args.resourceGroupName, tt.args.amlFilesystemName); (err != nil) != tt.wantErr {
// 				t.Errorf("DynamicProvisioner.DeleteAmlFilesystem() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

func TestDynamicProvisioner_CreateAmlFilesystem_FakeSetup(t *testing.T) {
	const expectedMgsAddress = "127.0.0.3"
	const expectedResourceGroupName = "fake-resource-group"
	const expectedAmlFilesystemName = "fake-amlfs"
	expectedResponse := armstoragecache.AmlFilesystem{
		Location: (*string)(nil),
		Identity: (*armstoragecache.AmlFilesystemIdentity)(nil),
		SKU: &armstoragecache.SKUName{
			Name: (*string)(nil),
		},
		Tags:  map[string]*string(nil),
		Zones: []*string(nil),
		SystemData: &armstoragecache.SystemData{
			CreatedAt:          (*time.Time)(nil),
			CreatedBy:          (*string)(nil),
			CreatedByType:      (*armstoragecache.CreatedByType)(nil),
			LastModifiedAt:     (*time.Time)(nil),
			LastModifiedBy:     (*string)(nil),
			LastModifiedByType: (*armstoragecache.CreatedByType)(nil),
		},
		Type: (*string)(nil),
		Properties: &armstoragecache.AmlFilesystemProperties{
			FilesystemSubnet:   (*string)(nil),
			MaintenanceWindow:  (*armstoragecache.AmlFilesystemPropertiesMaintenanceWindow)(nil),
			StorageCapacityTiB: (*float32)(nil),
			EncryptionSettings: (*armstoragecache.AmlFilesystemEncryptionSettings)(nil),
			Hsm:                (*armstoragecache.AmlFilesystemPropertiesHsm)(nil),
			RootSquashSettings: (*armstoragecache.AmlFilesystemRootSquashSettings)(nil),
			ClientInfo: &armstoragecache.AmlFilesystemClientInfo{
				ContainerStorageInterface: (*armstoragecache.AmlFilesystemContainerStorageInterface)(nil),
				LustreVersion:             (*string)(nil),
				MgsAddress:                to.Ptr(string(expectedMgsAddress)),
				MountCommand:              (*string)(nil),
			},
			Health:                    (*armstoragecache.AmlFilesystemHealth)(nil),
			ProvisioningState:         (*armstoragecache.AmlFilesystemProvisioningStateType)(nil),
			ThroughputProvisionedMBps: (*int32)(nil),
		},
		Name: to.Ptr(expectedAmlFilesystemName),
		ID:   to.Ptr("fake-id"),
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	amlFilesystemsClientFactory, err := armstoragecache.NewClientFactory("fake-subscription-id", &azfake.TokenCredential{},
		&arm.ClientOptions{
			ClientOptions: azcore.ClientOptions{
				Transport: fake.NewAmlFilesystemsServerTransport(newFakeAmlFilesystemsServer(t, expectedMgsAddress)),
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, amlFilesystemsClientFactory)

	fakeAmlFilesystemsClient := amlFilesystemsClientFactory.NewAmlFilesystemsClient()

	require.NotNil(t, fakeAmlFilesystemsClient)

	poller, err := fakeAmlFilesystemsClient.BeginCreateOrUpdate(context.Background(), expectedResourceGroupName, expectedAmlFilesystemName, expectedResponse, nil)
	require.NoError(t, err)
	assert.NotNil(t, poller)
	amlFilesystem, err := poller.PollUntilDone(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, expectedResponse, amlFilesystem.AmlFilesystem)
}

func TestDynamicProvisioner_CreateAmlFilesystem_Success(t *testing.T) {
	const expectedMgsAddress = "127.0.0.3"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	amlFilesystemsClientFactory, err := armstoragecache.NewClientFactory("fake-subscription-id", &azfake.TokenCredential{},
		&arm.ClientOptions{
			ClientOptions: azcore.ClientOptions{
				Transport: fake.NewAmlFilesystemsServerTransport(newFakeAmlFilesystemsServer(t, expectedMgsAddress)),
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, amlFilesystemsClientFactory)

	fakeAmlFilesystemsClient := amlFilesystemsClientFactory.NewAmlFilesystemsClient()

	dynamicProvisioner := &DynamicProvisioner{
		amlFilesystemsClient: fakeAmlFilesystemsClient,
	}
	mgsIPAddress, err := dynamicProvisioner.CreateAmlFilesystem(context.Background(), &AmlFilesystemProperties{
		ResourceGroupName: "fake-resource-group",
		AmlFilesystemName: "fake-amlfs",
	})
	require.NoError(t, err)
	assert.Equal(t, expectedMgsAddress, mgsIPAddress)
}

// func TestDynamicProvisioner_CreateAmlFilesystem(t *testing.T) {
// 	type fields struct {
// 		DynamicProvisionerInterface DynamicProvisionerInterface
// 		amlFilesystemsClient        *armstoragecache.AmlFilesystemsClient
// 	}
// 	type args struct {
// 		cxt                     context.Context
// 		amlFilesystemProperties *AmlFilesystemProperties
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		want    string
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			d := &DynamicProvisioner{
// 				DynamicProvisionerInterface: tt.fields.DynamicProvisionerInterface,
// 				amlFilesystemsClient:        tt.fields.amlFilesystemsClient,
// 			}
// 			got, err := d.CreateAmlFilesystem(tt.args.cxt, tt.args.amlFilesystemProperties)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("DynamicProvisioner.CreateAmlFilesystem() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if got != tt.want {
// 				t.Errorf("DynamicProvisioner.CreateAmlFilesystem() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }
