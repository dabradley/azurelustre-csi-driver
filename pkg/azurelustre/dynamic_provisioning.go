package azurelustre

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4"
	"k8s.io/klog/v2"
)

type DynamicProvisionerInterface interface {
	DeleteAmlFilesystem(cxt context.Context, resourceGroupName, amlFilesystemName string) error
	CreateAmlFilesystem(cxt context.Context, amlFilesystemProperties *AmlFilesystemProperties) (string, error)
	ClusterExists(ctx context.Context, resourceGroupName, amlFilesystemName string) (bool, error)
}

type DynamicProvisioner struct {
	DynamicProvisionerInterface
	amlFilesystemsClient *armstoragecache.AmlFilesystemsClient
}

func (d *DynamicProvisioner) ClusterExists(ctx context.Context, resourceGroupName, amlFilesystemName string) (bool, error) {
	if d.amlFilesystemsClient == nil {
		klog.V(2).Infof("skipping list request, mocked")
		return false, nil // Mocked
	}

	_, err := d.amlFilesystemsClient.Get(ctx, resourceGroupName, amlFilesystemName, nil)
	if err != nil {
		klog.V(2).Infof("error when retrieving the aml filesystem: %v", err)
		return false, err
	}
	return true, nil

	// 	Value: []*armstoragecache.AmlFilesystem{
	// 		{
	// 			Name: to.Ptr("fs1"),
	// 			Type: to.Ptr("Microsoft.StorageCache/amlFilesystem"),
	// 			ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.StorageCache/amlFilesystems/fs1"),
	// 			Location: to.Ptr("eastus"),
	// 			Tags: map[string]*string{
	// 				"Dept": to.Ptr("ContosoAds"),
	// 			},
	// 			Identity: &armstoragecache.AmlFilesystemIdentity{
	// 				Type: to.Ptr(armstoragecache.AmlFilesystemIdentityTypeUserAssigned),
	// 				UserAssignedIdentities: map[string]*armstoragecache.UserAssignedIdentitiesValue{
	// 					"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/identity1": &armstoragecache.UserAssignedIdentitiesValue{
	// 					},
	// 				},
	// 			},
	// 			Properties: &armstoragecache.AmlFilesystemProperties{
	// 				ClientInfo: &armstoragecache.AmlFilesystemClientInfo{
	// 					ContainerStorageInterface: &armstoragecache.AmlFilesystemContainerStorageInterface{
	// 						PersistentVolume: to.Ptr("<Base64 encoded YAML>"),
	// 						PersistentVolumeClaim: to.Ptr("<Base64 encoded YAML>"),
	// 						StorageClass: to.Ptr("<Base64 encoded YAML>"),
	// 					},
	// 					LustreVersion: to.Ptr("2.15.0"),
	// 					MgsAddress: to.Ptr("10.0.0.4"),
	// 					MountCommand: to.Ptr("mount -t lustre 10.0.0.4@tcp:/lustrefs /lustre/lustrefs"),
	// 				},
	// 				EncryptionSettings: &armstoragecache.AmlFilesystemEncryptionSettings{
	// 					KeyEncryptionKey: &armstoragecache.KeyVaultKeyReference{
	// 						KeyURL: to.Ptr("https://examplekv.vault.azure.net/keys/kvk/3540a47df75541378d3518c6a4bdf5af"),
	// 						SourceVault: &armstoragecache.KeyVaultKeyReferenceSourceVault{
	// 							ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.KeyVault/vaults/keyvault-cmk"),
	// 						},
	// 					},
	// 				},
	// 				FilesystemSubnet: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.Network/virtualNetworks/scvnet/subnets/fsSub1"),
	// 				Health: &armstoragecache.AmlFilesystemHealth{
	// 					State: to.Ptr(armstoragecache.AmlFilesystemHealthStateTypeAvailable),
	// 					StatusDescription: to.Ptr("amlFilesystem is ok."),
	// 				},
	// 				Hsm: &armstoragecache.AmlFilesystemPropertiesHsm{
	// 					ArchiveStatus: []*armstoragecache.AmlFilesystemArchive{
	// 						{
	// 							FilesystemPath: to.Ptr("/"),
	// 							Status: &armstoragecache.AmlFilesystemArchiveStatus{
	// 								LastCompletionTime: to.Ptr(func() time.Time { t, _ := time.Parse(time.RFC3339Nano, "2019-04-21T18:25:43.511Z"); return t}()),
	// 								LastStartedTime: to.Ptr(func() time.Time { t, _ := time.Parse(time.RFC3339Nano, "2019-04-21T17:25:43.511Z"); return t}()),
	// 								State: to.Ptr(armstoragecache.ArchiveStatusTypeCompleted),
	// 							},
	// 					}},
	// 					Settings: &armstoragecache.AmlFilesystemHsmSettings{
	// 						Container: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.Storage/storageAccounts/storageaccountname/blobServices/default/containers/containername"),
	// 						ImportPrefix: to.Ptr("/"),
	// 						LoggingContainer: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.Storage/storageAccounts/storageaccountname/blobServices/default/containers/loggingcontainername"),
	// 					},
	// 				},
	// 				MaintenanceWindow: &armstoragecache.AmlFilesystemPropertiesMaintenanceWindow{
	// 					DayOfWeek: to.Ptr(armstoragecache.MaintenanceDayOfWeekTypeFriday),
	// 					TimeOfDayUTC: to.Ptr("22:00"),
	// 				},
	// 				ProvisioningState: to.Ptr(armstoragecache.AmlFilesystemProvisioningStateTypeSucceeded),
	// 				StorageCapacityTiB: to.Ptr[float32](16),
	// 				ThroughputProvisionedMBps: to.Ptr[int32](500),
	// 			},
	// 			SKU: &armstoragecache.SKUName{
	// 				Name: to.Ptr("AMLFS-Durable-Premium-250"),
	// 			},
	// 			Zones: []*string{
	// 				to.Ptr("1")},
	// 			},
	// 			{
	// 				Name: to.Ptr("fs2"),
	// 				Type: to.Ptr("Microsoft.StorageCache/amlFilesystem"),
	// 				ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.StorageCache/amlFilesystems/fs2"),
	// 				Location: to.Ptr("eastus"),
	// 				Tags: map[string]*string{
	// 					"Dept": to.Ptr("ContosoAds"),
	// 				},
	// 				Identity: &armstoragecache.AmlFilesystemIdentity{
	// 					Type: to.Ptr(armstoragecache.AmlFilesystemIdentityTypeUserAssigned),
	// 					UserAssignedIdentities: map[string]*armstoragecache.UserAssignedIdentitiesValue{
	// 						"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/identity1": &armstoragecache.UserAssignedIdentitiesValue{
	// 						},
	// 					},
	// 				},
	// 				Properties: &armstoragecache.AmlFilesystemProperties{
	// 					ClientInfo: &armstoragecache.AmlFilesystemClientInfo{
	// 						ContainerStorageInterface: &armstoragecache.AmlFilesystemContainerStorageInterface{
	// 							PersistentVolume: to.Ptr("<Base64 encoded YAML>"),
	// 							PersistentVolumeClaim: to.Ptr("<Base64 encoded YAML>"),
	// 							StorageClass: to.Ptr("<Base64 encoded YAML>"),
	// 						},
	// 						LustreVersion: to.Ptr("2.15.0"),
	// 						MgsAddress: to.Ptr("10.0.0.4"),
	// 						MountCommand: to.Ptr("mount -t lustre 10.0.0.4@tcp:/lustrefs /lustre/lustrefs"),
	// 					},
	// 					EncryptionSettings: &armstoragecache.AmlFilesystemEncryptionSettings{
	// 						KeyEncryptionKey: &armstoragecache.KeyVaultKeyReference{
	// 							KeyURL: to.Ptr("https://examplekv.vault.azure.net/keys/kvk/3540a47df75541378d3518c6a4bdf5af"),
	// 							SourceVault: &armstoragecache.KeyVaultKeyReferenceSourceVault{
	// 								ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.KeyVault/vaults/keyvault-cmk"),
	// 							},
	// 						},
	// 					},
	// 					FilesystemSubnet: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.Network/virtualNetworks/scvnet/subnets/fsSub2"),
	// 					Health: &armstoragecache.AmlFilesystemHealth{
	// 						State: to.Ptr(armstoragecache.AmlFilesystemHealthStateTypeAvailable),
	// 						StatusDescription: to.Ptr("amlFilesystem is ok."),
	// 					},
	// 					Hsm: &armstoragecache.AmlFilesystemPropertiesHsm{
	// 						ArchiveStatus: []*armstoragecache.AmlFilesystemArchive{
	// 							{
	// 								FilesystemPath: to.Ptr("/"),
	// 								Status: &armstoragecache.AmlFilesystemArchiveStatus{
	// 									LastCompletionTime: to.Ptr(func() time.Time { t, _ := time.Parse(time.RFC3339Nano, "2019-04-21T18:25:43.511Z"); return t}()),
	// 									LastStartedTime: to.Ptr(func() time.Time { t, _ := time.Parse(time.RFC3339Nano, "2019-04-21T17:25:43.511Z"); return t}()),
	// 									State: to.Ptr(armstoragecache.ArchiveStatusTypeCompleted),
	// 								},
	// 						}},
	// 						Settings: &armstoragecache.AmlFilesystemHsmSettings{
	// 							Container: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.Storage/storageAccounts/storageaccountname/blobServices/default/containers/containername"),
	// 							ImportPrefix: to.Ptr("/"),
	// 							LoggingContainer: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.Storage/storageAccounts/storageaccountname/blobServices/default/containers/loggingcontainername"),
	// 						},
	// 					},
	// 					MaintenanceWindow: &armstoragecache.AmlFilesystemPropertiesMaintenanceWindow{
	// 						DayOfWeek: to.Ptr(armstoragecache.MaintenanceDayOfWeekTypeFriday),
	// 						TimeOfDayUTC: to.Ptr("22:00"),
	// 					},
	// 					ProvisioningState: to.Ptr(armstoragecache.AmlFilesystemProvisioningStateTypeSucceeded),
	// 					StorageCapacityTiB: to.Ptr[float32](16),
	// 					ThroughputProvisionedMBps: to.Ptr[int32](500),
	// 				},
	// 				SKU: &armstoragecache.SKUName{
	// 					Name: to.Ptr("AMLFS-Durable-Premium-250"),
	// 				},
	// 				Zones: []*string{
	// 					to.Ptr("1")},
	// 			}},
	// 		}
}

// Generated from example definition: https://github.com/Azure/azure-rest-api-specs/blob/c7f3e601fd326ca910c3d2939b516e15581e7e41/specification/storagecache/resource-manager/Microsoft.StorageCache/stable/2023-05-01/examples/amlFilesystems_Delete.json
func (d *DynamicProvisioner) DeleteAmlFilesystem(cxt context.Context, resourceGroupName, amlFilesystemName string) error {
	if d.amlFilesystemsClient == nil {
		klog.V(2).Infof("skipping delete request, mocked")
		klog.V(2).Infof("resource group: %v", resourceGroupName)
		klog.V(2).Infof("aml filesystem name: %v", amlFilesystemName)

		klog.Errorf("resource group: %v, aml filesystem name: %v", resourceGroupName, amlFilesystemName)

		return nil // Mocked
	}

	poller, err := d.amlFilesystemsClient.BeginDelete(cxt, resourceGroupName, amlFilesystemName, nil)
	if err != nil {
		klog.V(2).Infof("failed to finish the request: %v", err)
		return err
	}
	res, err := poller.PollUntilDone(cxt, nil)
	if err != nil {
		klog.V(2).Infof("failed to poll the result: %v", err)
		return err
	}
	klog.V(2).Infof("response to dyn: %v", res)

	return nil
}

func (d *DynamicProvisioner) CreateAmlFilesystem(cxt context.Context, amlFilesystemProperties *AmlFilesystemProperties) (string, error) {
	tags := make(map[string]*string, len(amlFilesystemProperties.Tags))
	for key, value := range amlFilesystemProperties.Tags {
		tags[key] = to.Ptr(value)
		klog.V(2).Infof("tag: %#v, key: %#v, value, %#v, ptrval:%#v", tags[key], key, value, *tags[key])
	}
	amlFilesystem := armstoragecache.AmlFilesystem{
		Location: to.Ptr(amlFilesystemProperties.Location),
		Tags:     tags,
		Properties: &armstoragecache.AmlFilesystemProperties{
			// EncryptionSettings: &armstoragecache.AmlFilesystemEncryptionSettings{
			// 	KeyEncryptionKey: &armstoragecache.KeyVaultKeyReference{
			// 		KeyURL: to.Ptr("https://examplekv.vault.azure.net/keys/kvk/3540a47df75541378d3518c6a4bdf5af"),
			// 		SourceVault: &armstoragecache.KeyVaultKeyReferenceSourceVault{
			// 			ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/scgroup/providers/Microsoft.KeyVault/vaults/keyvault-cmk"),
			// 		},
			// 	},
			// },
			FilesystemSubnet: to.Ptr(amlFilesystemProperties.SubnetID),
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
			// RootSquashSettings: &armstoragecache.AmlFilesystemRootSquashSettings{
			// 	Mode:             to.Ptr(armstoragecache.AmlFilesystemSquashModeRootOnly),
			// 	NoSquashNidLists: to.Ptr("10.222.222.222"),
			// 	SquashGID:        to.Ptr(int64(1000)),
			// 	SquashUID:        to.Ptr(int64(1000)),
			// },
		},
	}
	zones := make([]*string, len(amlFilesystemProperties.Zones))
	for i, zone := range amlFilesystemProperties.Zones {
		zones[i] = to.Ptr(zone)
	}
	amlFilesystem.Zones = zones
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
	if len(amlFilesystemProperties.SKUName) > 0 {
		amlFilesystem.SKU = &armstoragecache.SKUName{
			Name: to.Ptr(amlFilesystemProperties.SKUName),
		}
	}

	if d.amlFilesystemsClient == nil {
		klog.V(2).Infof("skipping create request, mocked")
		klog.V(2).Infof("resource group: %v", amlFilesystemProperties.ResourceGroupName)
		klog.V(2).Infof("aml filesystem name: %v", amlFilesystemProperties.AmlFilesystemName)
		klog.V(2).Infof("aml filesystem: %v", amlFilesystem)

		klog.Errorf("resource group: %#v, aml filesystem name: %#v, aml filesystem: %#v\n", amlFilesystemProperties.ResourceGroupName, amlFilesystemProperties.AmlFilesystemName, amlFilesystem)

		return "127.0.0.1", nil
	}

	poller, err := d.amlFilesystemsClient.BeginCreateOrUpdate(
		cxt,
		amlFilesystemProperties.ResourceGroupName,
		amlFilesystemProperties.AmlFilesystemName,
		amlFilesystem,
		nil)
	if err != nil {
		klog.V(2).Infof("failed to finish the request: %v", err)
		return "", err
	}

	res, err := poller.PollUntilDone(cxt, nil)
	if err != nil {
		klog.V(2).Infof("failed to poll the result: %v", err)
		return "", err
	}

	klog.V(2).Infof("response to dyn: %v", res)
	mgsAddress := *res.Properties.ClientInfo.MgsAddress
	return mgsAddress, nil
}
