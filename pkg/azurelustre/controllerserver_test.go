/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package azurelustre

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/azurelustre-csi-driver/pkg/util"
	azure "sigs.k8s.io/cloud-provider-azure/pkg/provider"
)

func TestControllerGetCapabilities(t *testing.T) {
	d := NewFakeDriver()
	d.AddControllerServiceCapabilities(controllerServiceCapabilities)
	req := csi.ControllerGetCapabilitiesRequest{}
	resp, err := d.ControllerGetCapabilities(context.Background(), &req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	capabilitiesSupported := make([]csi.ControllerServiceCapability_RPC_Type, 0, len(resp.GetCapabilities()))
	for _, capabilitySupported := range resp.GetCapabilities() {
		capabilitiesSupported = append(capabilitiesSupported, capabilitySupported.GetRpc().GetType())
	}
	sort.Slice(capabilitiesSupported,
		func(i, j int) bool {
			return capabilitiesSupported[i] < capabilitiesSupported[j]
		})
	capabilitiesWanted := controllerServiceCapabilities
	sort.Slice(capabilitiesWanted,
		func(i, j int) bool {
			return capabilitiesWanted[i] < capabilitiesWanted[j]
		})
	assert.Equal(t, capabilitiesWanted, capabilitiesSupported)
}

func buildCreateVolumeRequest() *csi.CreateVolumeRequest {
	req := &csi.CreateVolumeRequest{
		Name: "test_volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
			},
		},
		Parameters: map[string]string{
			"fs-name":        "tfs",
			"mgs-ip-address": "127.0.0.1",
			"sub-dir":        "testSubDir",
		},
	}
	return req
}

func buildDynamicProvCreateVolumeRequest() *csi.CreateVolumeRequest {
	req := &csi.CreateVolumeRequest{
		Name: "test_volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
			},
		},
		Parameters: map[string]string{
			"resource-group-name":     "test-resource-group",
			"amlfilesystem-name":      "test-filesystem",
			"location":                "test-location",
			"vnet-resource-group":     "test-vnet-rg",
			"vnet-name":               "test-vnet-name",
			"subnet-name":             "test-subnet-name",
			"maintenance-day-of-week": "Monday",
			"time-of-day-utc":         "12:00",
			"sku-name":                "AMLFS-Durable-Premium-250",
			"identities":              "identity1,identity2",
			"tags":                    "key1=value1,key2=value2",
			"zones":                   "zone1,zone2",
			"sub-dir":                 "testSubDir",
		},
	}
	return req
}

func TestCreateVolume_Success(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	rep, err := d.CreateVolume(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, rep.GetVolume())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeId())
	assert.NotZero(t, rep.GetVolume().GetCapacityBytes())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeContext())
}

func TestDynamicCreateVolume_Success(t *testing.T) {
	d := NewFakeDriver()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	req := buildDynamicProvCreateVolumeRequest()
	rep, err := d.CreateVolume(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, rep.GetVolume())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeId())
	assert.NotZero(t, rep.GetVolume().GetCapacityBytes())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeContext())
}

func TestDynamicCreateVolume_Success_SendsCorrectProperties(t *testing.T) {
	expectedAmlfsProperties := &AmlFilesystemProperties{
		ResourceGroupName:    "test-resource-group",
		AmlFilesystemName:    "test-filesystem",
		Location:             "test-location",
		VnetResourceGroup:    "test-vnet-rg",
		VnetName:             "test-vnet-name",
		SubnetName:           "test-subnet-name",
		MaintenanceDayOfWeek: "Monday",
		TimeOfDayUTC:         "12:00",
		SKUName:              "AMLFS-Durable-Premium-250",
		SubnetID:             "/subscriptions/subscription/resourceGroups/test-vnet-rg/providers/Microsoft.Network/virtualNetworks/test-vnet-name/subnets/test-subnet-name",
		StorageCapacityTiB:   8,
		Identities:           []string{"identity1", "identity2"},
		Tags:                 map[string]string{"key1": "value1", "key2": "value2"},
		Zones:                []string{"zone1", "zone2"},
	}

	d := NewFakeDriver()
	fakeDynamicProvisioner := &FakeDynamicProvisioner{}
	d.dynamicProvisioner = fakeDynamicProvisioner
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	req := buildDynamicProvCreateVolumeRequest()
	rep, err := d.CreateVolume(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, rep.GetVolume())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeId())
	assert.NotZero(t, rep.GetVolume().GetCapacityBytes())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeContext())
	require.Len(t, fakeDynamicProvisioner.Filesystems, 1)
	assert.Equal(t, expectedAmlfsProperties, fakeDynamicProvisioner.Filesystems[0])
}

func TestDynamicCreateVolume_Success_DefaultLocation(t *testing.T) {
	d := NewFakeDriver()
	fakeDynamicProvisioner := &FakeDynamicProvisioner{}
	d.dynamicProvisioner = fakeDynamicProvisioner

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	req := buildDynamicProvCreateVolumeRequest()
	delete(req.GetParameters(), "location")
	rep, err := d.CreateVolume(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, rep.GetVolume())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeId())
	require.NotEmpty(t, fakeDynamicProvisioner.Filesystems)
	assert.Equal(t, d.location, fakeDynamicProvisioner.Filesystems[0].Location)
}

func TestDynamicCreateVolume_Success_DefaultResourceGroup(t *testing.T) {
	d := NewFakeDriver()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	req := buildDynamicProvCreateVolumeRequest()
	delete(req.GetParameters(), "resource-group-name")
	rep, err := d.CreateVolume(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, rep.GetVolume())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeId())
	assert.Contains(t, rep.GetVolume().GetVolumeId(), d.resourceGroup)
}

func TestDynamicCreateVolume_Success_UsesReturnedIPAddress(t *testing.T) {
	d := NewFakeDriver()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	req := buildDynamicProvCreateVolumeRequest()
	delete(req.GetParameters(), "resource-group-name")
	rep, err := d.CreateVolume(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, rep.GetVolume())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeId())
	assert.Contains(t, rep.GetVolume().GetVolumeId(), "127.0.0.2")
}

func TestDynamicCreateVolume_Success_NameMapping(t *testing.T) {
	pvcName := "pvc_name"
	pvcNamespace := "pvc_namespace"
	pvName := "pv_name"
	nameInputs := []string{
		// "test-filesystem",
		"test-filesystem-${pvc.metadata.name}",
		"test-filesystem-${pvc.metadata.namespace}",
		"test-filesystem-${pv.metadata.name}",
	}
	expectedOutputs := []string{
		// "test-filesystem",
		"test-filesystem-" + pvcName,
		"test-filesystem-" + pvcNamespace,
		"test-filesystem-" + pvName,
	}
	d := NewFakeDriver()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	for idx, nameInput := range nameInputs {
		t.Run(nameInput, func(t *testing.T) {
			req := buildDynamicProvCreateVolumeRequest()
			req.Parameters[pvcNameKey] = pvcName
			req.Parameters[pvcNamespaceKey] = pvcNamespace
			req.Parameters[pvNameKey] = pvName
			req.Parameters["amlfilesystem-name"] = nameInput
			rep, err := d.CreateVolume(context.Background(), req)
			require.NoError(t, err)
			assert.NotEmpty(t, rep.GetVolume())
			expectedOutput := fmt.Sprintf("test_volume#lustrefs#127.0.0.2#testSubDir#%s#test-resource-group", expectedOutputs[idx])
			assert.Equal(t, expectedOutput, rep.GetVolume().GetVolumeId())
		})
	}
}

func TestCreateVolume_Success_CapacityRoundUp(t *testing.T) {
	capacityInputs := []int64{
		0, laaSOBlockSizeInBytes - 1, laaSOBlockSizeInBytes, laaSOBlockSizeInBytes + 1,
	}
	expectedOutputs := []int64{
		defaultSizeInBytes, laaSOBlockSizeInBytes, laaSOBlockSizeInBytes, laaSOBlockSizeInBytes * 2,
	}

	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	for idx, capacityInput := range capacityInputs {
		req.CapacityRange = &csi.CapacityRange{
			RequiredBytes: capacityInput,
		}
		rep, err := d.CreateVolume(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, expectedOutputs[idx], rep.GetVolume().GetCapacityBytes())
	}
}

func TestDynamicCreateVolume_Err_CreateError(t *testing.T) {
	d := NewFakeDriver()
	fakeDynamicProvisioner := &FakeDynamicProvisioner{}
	d.dynamicProvisioner = fakeDynamicProvisioner

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	req := buildDynamicProvCreateVolumeRequest()
	req.Parameters["amlfilesystem-name"] = "testShouldFail"
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unknown, grpcStatus.Code())
	require.ErrorContains(t, err, "Error when creating AMLFS")
	require.ErrorContains(t, err, "testShouldFail")
}

func TestCreateVolume_Err_NoName(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.Name = ""
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Name")
}

func TestCreateVolume_Err_NoVolumeCapabilities(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.VolumeCapabilities = nil
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Volume capabilities")
}

func TestCreateVolume_Err_EmptyVolumeCapabilities(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.VolumeCapabilities = []*csi.VolumeCapability{}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Volume capabilities")
}

func TestCreateVolume_Err_NoParameters(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.Parameters = nil
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Parameters must be provided")
}

func TestCreateVolume_Err_MixDynamicAndStatic(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.Parameters[VolumeContextAmlFilesystemName] = "test-filesystem"
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "must not be provided when using a static AMLFS")
}

func TestCreateVolume_Err_NoAmlfsName(t *testing.T) {
	d := NewFakeDriver()
	req := buildDynamicProvCreateVolumeRequest()
	delete(req.GetParameters(), VolumeContextAmlFilesystemName)
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "must be provided for dynamically provisioned AMLFS")
}

func TestCreateVolume_Err_BadSku(t *testing.T) {
	d := NewFakeDriver()
	req := buildDynamicProvCreateVolumeRequest()
	req.Parameters["sku-name"] = "bad-sku"
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "sku-name must be one of")
}

func TestCreateVolume_Err_CapacityAboveSkuMax(t *testing.T) {
	d := NewFakeDriver()
	req := buildDynamicProvCreateVolumeRequest()
	req.CapacityRange = &csi.CapacityRange{
		RequiredBytes: math.MaxInt64,
	}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "exceeds maximum capacity")
}

func TestCreateVolume_Err_CapacityAboveLimit(t *testing.T) {
	d := NewFakeDriver()
	req := buildDynamicProvCreateVolumeRequest()
	req.CapacityRange = &csi.CapacityRange{
		RequiredBytes: 2 * util.TiB,
		LimitBytes:    1 * util.TiB,
	}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "greater than capacity limit")
}

// func TestCreateVolume_Err_ParametersNoIP(t *testing.T) {
// 	d := NewFakeDriver()
// 	req := buildCreateVolumeRequest()
// 	delete(req.GetParameters(), VolumeContextMGSIPAddress)
// 	_, err := d.CreateVolume(context.Background(), req)
// 	require.Error(t, err)
// 	grpcStatus, ok := status.FromError(err)
// 	assert.True(t, ok)
// 	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
// 	require.ErrorContains(t, err, "mgs-ip-address")
// }

// func TestCreateVolume_Err_ParametersEmptyIP(t *testing.T) {
// 	d := NewFakeDriver()
// 	req := buildCreateVolumeRequest()
// 	req.Parameters[VolumeContextMGSIPAddress] = ""
// 	_, err := d.CreateVolume(context.Background(), req)
// 	require.Error(t, err)
// 	grpcStatus, ok := status.FromError(err)
// 	assert.True(t, ok)
// 	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
// 	require.ErrorContains(t, err, "mgs-ip-address")
// }

func TestCreateVolume_Err_ParametersNoFSName(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	delete(req.GetParameters(), VolumeContextFSName)
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "fs-name")
}

func TestCreateVolume_Err_ParametersEmptyFSName(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.GetParameters()[VolumeContextFSName] = ""
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "fs-name")
}

func TestCreateVolume_Err_ParametersEmptySubDir(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.Parameters[VolumeContextSubDir] = ""
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "sub-dir")
}

func TestCreateVolume_Err_UnknownParameters(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.Parameters["FirstNonexistentParameter"] = "Invalid"
	req.Parameters["AnotherNonexistentParameter"] = "Invalid"
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Invalid parameter")
	require.ErrorContains(t, err, "FirstNonexistentParameter")
	require.ErrorContains(t, err, "AnotherNonexistentParameter")
}

func TestCreateVolume_Err_UnknownParametersDynamicProvisioning(t *testing.T) {
	d := NewFakeDriver()
	req := buildDynamicProvCreateVolumeRequest()
	req.Parameters["FirstNonexistentParameter"] = "Invalid"
	req.Parameters["AnotherNonexistentParameter"] = "Invalid"
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Invalid parameter")
	require.ErrorContains(t, err, "FirstNonexistentParameter")
	require.ErrorContains(t, err, "AnotherNonexistentParameter")
}

func TestCreateVolume_Err_HasVolumeContentSource(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.VolumeContentSource = &csi.VolumeContentSource{}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "existing volume")
}

func TestCreateVolume_Err_HasSecrets(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.Secrets = map[string]string{}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "secrets")
}

func TestCreateVolume_Err_HasSecretsValue(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.Secrets = map[string]string{"test": "test"}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "secrets")
}

func TestCreateVolume_Err_HasAccessibilityRequirements(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.AccessibilityRequirements = &csi.TopologyRequirement{}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "accessibility_requirements")
}

func TestCreateVolume_Err_BlockVolume(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.VolumeCapabilities = []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Block{
				Block: &csi.VolumeCapability_BlockVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			},
		},
	}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "block volume")
}

func TestCreateVolume_Err_BlockMountVolume(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.VolumeCapabilities = append(req.VolumeCapabilities,
		&csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Block{
				Block: &csi.VolumeCapability_BlockVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: volumeCapabilities[0],
			},
		})
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "block volume")
}

func TestCreateVolume_Err_NotSupportedAccessMode(t *testing.T) {
	capabilitiesNotSupported := []csi.VolumeCapability_AccessMode_Mode{}
	for capability := range csi.VolumeCapability_AccessMode_Mode_name {
		supported := false
		for _, supportedCapability := range volumeCapabilities {
			if csi.VolumeCapability_AccessMode_Mode(capability) ==
				supportedCapability {
				supported = true
				break
			}
		}
		if !supported {
			capabilitiesNotSupported = append(capabilitiesNotSupported,
				csi.VolumeCapability_AccessMode_Mode(capability))
		}
	}

	require.NotEmpty(t, capabilitiesNotSupported, "No unsupported AccessMode.")

	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	req.VolumeCapabilities = []*csi.VolumeCapability{}
	t.Logf("Unsupported access modes: %s", capabilitiesNotSupported)
	for _, capabilityNotSupported := range capabilitiesNotSupported {
		req.VolumeCapabilities = append(req.VolumeCapabilities,
			&csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: capabilityNotSupported,
				},
			},
		)
	}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, capabilitiesNotSupported[0].String())
}

func TestCreateVolume_Err_OperationExists(t *testing.T) {
	d := NewFakeDriver()
	req := buildCreateVolumeRequest()
	if acquired := d.volumeLocks.TryAcquire(req.GetName()); !acquired {
		assert.Fail(t, "Can't acquire volume lock")
	}
	_, err := d.CreateVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Aborted, grpcStatus.Code())
	assert.Regexp(t, "operation.*already exists", err.Error())
}

func TestDeleteVolume_Success(t *testing.T) {
	d := NewFakeDriver()
	req := &csi.DeleteVolumeRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"testVolume", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
	}
	_, err := d.DeleteVolume(context.Background(), req)
	require.NoError(t, err)
}

func TestDynamicDeleteVolume_Success(t *testing.T) {
	d := NewFakeDriver()
	fakeDynamicProvisioner := &FakeDynamicProvisioner{}
	d.dynamicProvisioner = fakeDynamicProvisioner

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	createReq := buildDynamicProvCreateVolumeRequest()
	delete(createReq.GetParameters(), "location")
	rep, err := d.CreateVolume(context.Background(), createReq)
	require.NoError(t, err)
	assert.NotEmpty(t, rep.GetVolume())
	assert.NotEmpty(t, rep.GetVolume().GetVolumeId())
	require.NotEmpty(t, fakeDynamicProvisioner.Filesystems)

	// d = NewFakeDriver()
	deleteRequest := &csi.DeleteVolumeRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"testVolume", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
	}
	_, err = d.DeleteVolume(context.Background(), deleteRequest)
	require.NoError(t, err)
	assert.Empty(t, fakeDynamicProvisioner.Filesystems)
}

func TestDynamicDeleteVolume_Err_DeleteError(t *testing.T) {
	d := NewFakeDriver()
	fakeDynamicProvisioner := &FakeDynamicProvisioner{}
	d.dynamicProvisioner = fakeDynamicProvisioner

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	d.cloud = azure.GetTestCloud(ctrl)
	req := &csi.DeleteVolumeRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"testVolume", "testFs", "127.0.0.1", "testSubDir", "testShouldFail", "testResourceGroupName"),
	}
	_, err := d.DeleteVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Unknown, grpcStatus.Code())
	require.ErrorContains(t, err, "Error when deleting AMLFS")
	require.ErrorContains(t, err, "testShouldFail")
}

func TestDeleteVolume_Err_NoVolumeID(t *testing.T) {
	d := NewFakeDriver()
	req := &csi.DeleteVolumeRequest{
		VolumeId: "",
	}
	_, err := d.DeleteVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Volume ID missing")
}

func TestDeleteVolume_Success_InvalidVolumeID(t *testing.T) {
	d := NewFakeDriver()
	req := &csi.DeleteVolumeRequest{
		VolumeId: "#",
	}
	_, err := d.DeleteVolume(context.Background(), req)
	require.NoError(t, err)
}

func TestDeleteVolume_Err_HasSecrets(t *testing.T) {
	d := NewFakeDriver()
	req := &csi.DeleteVolumeRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"testVolume", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		Secrets: map[string]string{},
	}
	_, err := d.DeleteVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "secrets")
}

func TestDeleteVolume_Err_HasSecretsValue(t *testing.T) {
	d := NewFakeDriver()
	req := &csi.DeleteVolumeRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"testVolume", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		Secrets: map[string]string{
			"test": "test",
		},
	}
	_, err := d.DeleteVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "secrets")
}

func TestDeleteVolume_Err_OperationExists(t *testing.T) {
	d := NewFakeDriver()
	req := &csi.DeleteVolumeRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"testVolume", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
	}
	if acquired := d.volumeLocks.TryAcquire(req.GetVolumeId()); !acquired {
		assert.Fail(t, "Can't acquire volume lock")
	}
	_, err := d.DeleteVolume(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Aborted, grpcStatus.Code())
	assert.Regexp(t, "operation.*already exists", err.Error())
}

func TestValidateVolumeCapabilities_Success(t *testing.T) {
	d := NewFakeDriver()
	capabilities := []*csi.VolumeCapability{}
	for _, capability := range volumeCapabilities {
		capabilities = append(
			capabilities,
			&csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: capability,
				},
			},
		)
	}
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"test", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		VolumeCapabilities: capabilities,
	}

	_, err := d.ValidateVolumeCapabilities(context.Background(), req)
	require.NoError(t, err)
}

func TestValidateVolumeCapabilities_Err_NoVolumeID(t *testing.T) {
	d := NewFakeDriver()
	capabilities := []*csi.VolumeCapability{}
	for _, capability := range volumeCapabilities {
		capabilities = append(
			capabilities,
			&csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: capability,
				},
			},
		)
	}
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:           "",
		VolumeCapabilities: capabilities,
	}

	_, err := d.ValidateVolumeCapabilities(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Volume ID")
}

func TestValidateVolumeCapabilities_Err_NoVolumeCapabilities(t *testing.T) {
	d := NewFakeDriver()
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"test", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		VolumeCapabilities: nil,
	}

	_, err := d.ValidateVolumeCapabilities(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "capabilities")
}

func TestValidateVolumeCapabilities_Err_EmptyVolumeCapabilities(t *testing.T) {
	d := NewFakeDriver()
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"test", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		VolumeCapabilities: []*csi.VolumeCapability{},
	}

	_, err := d.ValidateVolumeCapabilities(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "capabilities")
}

func TestValidateVolumeCapabilities_Err_HasSecretes(t *testing.T) {
	d := NewFakeDriver()
	capabilities := []*csi.VolumeCapability{}
	for _, capability := range volumeCapabilities {
		capabilities = append(
			capabilities,
			&csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: capability,
				},
			},
		)
	}
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"test", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		VolumeCapabilities: capabilities,
		Secrets:            map[string]string{},
	}

	_, err := d.ValidateVolumeCapabilities(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "secrets")
}

func TestValidateVolumeCapabilities_Err_HasSecretesValue(t *testing.T) {
	d := NewFakeDriver()
	capabilities := []*csi.VolumeCapability{}
	for _, capability := range volumeCapabilities {
		capabilities = append(
			capabilities,
			&csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: capability,
				},
			},
		)
	}
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"test", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		VolumeCapabilities: capabilities,
		Secrets:            map[string]string{"test": "test"},
	}

	_, err := d.ValidateVolumeCapabilities(context.Background(), req)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "secrets")
}

func TestValidateVolumeCapabilities_Success_BlockCapabilities(t *testing.T) {
	d := NewFakeDriver()
	capabilities := []*csi.VolumeCapability{}
	for _, capability := range volumeCapabilities {
		capabilities = append(
			capabilities,
			&csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Block{
					Block: &csi.VolumeCapability_BlockVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: capability,
				},
			},
		)
	}
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"test", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		VolumeCapabilities: capabilities,
	}

	res, err := d.ValidateVolumeCapabilities(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, res.GetConfirmed())
}

func TestValidateVolumeCapabilities_Success_HasUnsupportedAccessMode(
	t *testing.T,
) {
	capabilitiesNotSupported := []csi.VolumeCapability_AccessMode_Mode{}
	for capability := range csi.VolumeCapability_AccessMode_Mode_name {
		supported := false
		for _, supportedCapability := range volumeCapabilities {
			if csi.VolumeCapability_AccessMode_Mode(capability) ==
				supportedCapability {
				supported = true
				break
			}
		}
		if !supported {
			capabilitiesNotSupported = append(capabilitiesNotSupported,
				csi.VolumeCapability_AccessMode_Mode(capability))
		}
	}

	require.NotEmpty(t, capabilitiesNotSupported, "No unsupported AccessMode.")

	d := NewFakeDriver()
	capabilities := []*csi.VolumeCapability{}
	for _, capability := range capabilitiesNotSupported {
		capabilities = append(
			capabilities,
			&csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Block{
					Block: &csi.VolumeCapability_BlockVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: capability,
				},
			},
		)
	}
	req := &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: fmt.Sprintf(volumeIDTemplate,
			"test", "testFs", "127.0.0.1", "testSubDir", "test-filesystem", "testResourceGroupName"),
		VolumeCapabilities: capabilities,
	}

	res, err := d.ValidateVolumeCapabilities(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, res.GetConfirmed())
}

func TestParseAmlfilesystemProperties_Success(t *testing.T) {
	properties := map[string]string{
		// "subscription-id":         "test-subscription-id",
		// "resource-group-name":     "test-resource-group",
		"amlfilesystem-name":      "test-filesystem",
		"location":                "test-location",
		"vnet-resource-group":     "test-vnet-rg",
		"vnet-name":               "test-vnet-name",
		"subnet-name":             "test-subnet-name",
		"maintenance-day-of-week": "Monday",
		"time-of-day-utc":         "12:00",
		"sku-name":                "Standard",
		"identities":              "identity1,identity2",
		"tags":                    "key1=value1,key2=value2",
		"zones":                   "zone1,zone2,",
	}

	expected := &AmlFilesystemProperties{
		// SubscriptionID:       "test-subscription-id",
		// ResourceGroupName:    "test-resource-group",
		AmlFilesystemName:    "test-filesystem",
		Location:             "test-location",
		VnetResourceGroup:    "test-vnet-rg",
		VnetName:             "test-vnet-name",
		SubnetName:           "test-subnet-name",
		MaintenanceDayOfWeek: "Monday",
		TimeOfDayUTC:         "12:00",
		// StorageCapacityTiB:   10,
		SKUName:    "Standard",
		Identities: []string{"identity1", "identity2"},
		Tags:       map[string]string{"key1": "value1", "key2": "value2"},
		Zones:      []string{"zone1", "zone2"},
	}

	result, err := parseAmlFilesystemProperties(properties)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestParseAmlfilesystemProperties_Err_InvalidParameters(t *testing.T) {
	properties := map[string]string{
		"invalid-param": "invalid",
		// "resource-group-name":     "test-resource-group",
		"amlfilesystem-name":      "test-filesystem",
		"location":                "test-location",
		"vnet-resource-group":     "test-vnet-rg",
		"vnet-name":               "test-vnet-name",
		"subnet-name":             "test-subnet-name",
		"maintenance-day-of-week": "Monday",
		"time-of-day-utc":         "12:00",
		"sku-name":                "Standard",
		"zones":                   "zone1,zone2",
	}

	_, err := parseAmlFilesystemProperties(properties)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "Invalid parameter(s)")
	require.ErrorContains(t, err, "invalid-param")
}

// func TestParseAmlfilesystemProperties_Err_MissingSubscriptionId(t *testing.T) {
// 	properties := map[string]string{
// 		"resource-group-name":     "test-resource-group",
// 		"amlfilesystem-name":      "test-filesystem",
// 		"location":                "test-location",
// 		"filesystem-subnet-id":    "test-subnet-id",
// 		"maintenance-day-of-week": "Monday",
// 		"time-of-day-utc":         "12:00",
// 		"storage-capacity-tib":    "10",
// 		"sku-name":                "Standard",
// 		"zones":                   "zone1,zone2",
// 	}

// 	_, err := parseAmlFilesystemProperties(properties)
// 	require.Error(t, err)
// 	grpcStatus, ok := status.FromError(err)
// 	assert.True(t, ok)
// 	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
// 	require.ErrorContains(t, err, "subscription-id must be provided")
// }

// func TestParseAmlfilesystemProperties_Err_MissingRsourceGroupName(t *testing.T) {
// 	properties := map[string]string{
// 		// "subscription-id":         "test-subscription-id",
// 		"amlfilesystem-name":      "test-filesystem",
// 		"location":                "test-location",
// 		"filesystem-subnet-id":    "test-subnet-id",
// 		"maintenance-day-of-week": "Monday",
// 		"time-of-day-utc":         "12:00",
// 		// "storage-capacity-tib":    "10",
// 		"sku-name": "Standard",
// 		"zones":    "zone1,zone2",
// 	}

// 	_, err := parseAmlFilesystemProperties(properties)
// 	require.Error(t, err)
// 	grpcStatus, ok := status.FromError(err)
// 	assert.True(t, ok)
// 	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
// 	require.ErrorContains(t, err, "resource-group-name must be provided")
// }

func TestParseAmlfilesystemProperties_Err_MissingMaintenanceDayOfWeek(t *testing.T) {
	properties := map[string]string{
		// "subscription-id":      "test-subscription-id",
		// "resource-group-name":  "test-resource-group",
		"amlfilesystem-name":  "test-filesystem",
		"location":            "test-location",
		"vnet-resource-group": "test-vnet-rg",
		"vnet-name":           "test-vnet-name",
		"subnet-name":         "test-subnet-name",
		"time-of-day-utc":     "12:00",
		// "storage-capacity-tib": "10",
		"sku-name": "Standard",
		"zones":    "zone1,zone2",
	}

	_, err := parseAmlFilesystemProperties(properties)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "maintenance-day-of-week must be provided")
}

func TestParseAmlfilesystemProperties_Err_EmptyMaintenanceDayOfWeek(t *testing.T) {
	properties := map[string]string{
		// "subscription-id":      "test-subscription-id",
		// "resource-group-name":  "test-resource-group",
		"amlfilesystem-name":      "test-filesystem",
		"location":                "test-location",
		"vnet-resource-group":     "test-vnet-rg",
		"vnet-name":               "test-vnet-name",
		"subnet-name":             "test-subnet-name",
		"maintenance-day-of-week": "",
		"time-of-day-utc":         "12:00",
		// "storage-capacity-tib": "10",
		"sku-name": "Standard",
		"zones":    "zone1,zone2",
	}

	_, err := parseAmlFilesystemProperties(properties)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "maintenance-day-of-week must be one of")
}

func TestParseAmlfilesystemProperties_Err_InvalidMaintenanceDayOfWeek(t *testing.T) {
	properties := map[string]string{
		// "subscription-id":      "test-subscription-id",
		// "resource-group-name":  "test-resource-group",
		"amlfilesystem-name":      "test-filesystem",
		"location":                "test-location",
		"vnet-resource-group":     "test-vnet-rg",
		"vnet-name":               "test-vnet-name",
		"subnet-name":             "test-subnet-name",
		"maintenance-day-of-week": "invalid-day-of-week",
		"time-of-day-utc":         "12:00",
		// "storage-capacity-tib": "10",
		"sku-name": "Standard",
		"zones":    "zone1,zone2",
	}

	_, err := parseAmlFilesystemProperties(properties)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "maintenance-day-of-week must be one of")
}

func TestParseAmlfilesystemProperties_Err_MissingTimeOfDay(t *testing.T) {
	properties := map[string]string{
		// "subscription-id":      "test-subscription-id",
		// "resource-group-name":  "test-resource-group",
		"amlfilesystem-name":      "test-filesystem",
		"location":                "test-location",
		"vnet-resource-group":     "test-vnet-rg",
		"vnet-name":               "test-vnet-name",
		"subnet-name":             "test-subnet-name",
		"maintenance-day-of-week": "Monday",
		// "storage-capacity-tib": "10",
		"sku-name": "Standard",
		"zones":    "zone1,zone2",
	}

	_, err := parseAmlFilesystemProperties(properties)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "time-of-day-utc must be provided")
}

func TestParseAmlfilesystemProperties_Err_InvalidTimeOfDay(t *testing.T) {
	properties := map[string]string{
		// "subscription-id":      "test-subscription-id",
		// "resource-group-name":  "test-resource-group",
		"amlfilesystem-name":      "test-filesystem",
		"location":                "test-location",
		"vnet-resource-group":     "test-vnet-rg",
		"vnet-name":               "test-vnet-name",
		"subnet-name":             "test-subnet-name",
		"maintenance-day-of-week": "Monday",
		"time-of-day-utc":         "11",
		// "storage-capacity-tib": "10",
		"sku-name": "Standard",
		"zones":    "zone1,zone2",
	}

	_, err := parseAmlFilesystemProperties(properties)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "time-of-day-utc must be in the form HH:MM")
}

func TestParseAmlfilesystemProperties_Err_MissingZones(t *testing.T) {
	properties := map[string]string{
		// "subscription-id":         "test-subscription-id",
		// "resource-group-name":     "test-resource-group",
		"amlfilesystem-name":      "test-filesystem",
		"location":                "test-location",
		"vnet-resource-group":     "test-vnet-rg",
		"vnet-name":               "test-vnet-name",
		"subnet-name":             "test-subnet-name",
		"maintenance-day-of-week": "Monday",
		"time-of-day-utc":         "12:00",
		// "storage-capacity-tib":    "10",
		"sku-name": "Standard",
	}

	_, err := parseAmlFilesystemProperties(properties)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "zones must be provided")
}

func TestParseAmlfilesystemProperties_Err_InvalidTags(t *testing.T) {
	properties := map[string]string{
		// "subscription-id":      "test-subscription-id",
		// "resource-group-name":  "test-resource-group",
		"amlfilesystem-name":      "test-filesystem",
		"location":                "test-location",
		"vnet-resource-group":     "test-vnet-rg",
		"vnet-name":               "test-vnet-name",
		"subnet-name":             "test-subnet-name",
		"maintenance-day-of-week": "Monday",
		"time-of-day-utc":         "12:00",
		// "storage-capacity-tib": "10",
		"sku-name": "Standard",
		"zones":    "zone1,zone2",
		"tags":     "key1:value1,=value2",
	}

	_, err := parseAmlFilesystemProperties(properties)
	require.Error(t, err)
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, grpcStatus.Code())
	require.ErrorContains(t, err, "are invalid, the format should be: 'key1=value1,key2=value2")
}

func TestGetValidAmlFilesystemName(t *testing.T) {
	tests := []struct {
		desc              string
		amlFilesystemName string
		volumeName        string
		expected          string
	}{
		{
			desc:              "valid alpha name",
			amlFilesystemName: "aqz",
			volumeName:        "volName",
			expected:          "aqz",
		},
		{
			desc:              "valid numeric name",
			amlFilesystemName: "029",
			volumeName:        "volName",
			expected:          "029",
		},
		{
			desc:              "max length name",
			amlFilesystemName: strings.Repeat("a", amlFilesystemNameMaxLength),
			volumeName:        "volName",
			expected:          strings.Repeat("a", amlFilesystemNameMaxLength),
		},
		{
			desc:              "name too long",
			amlFilesystemName: strings.Repeat("a", amlFilesystemNameMaxLength+1),
			volumeName:        "volName",
			expected:          strings.Repeat("a", amlFilesystemNameMaxLength),
		},
		{
			desc:              "removes invalid characters when using volume name",
			amlFilesystemName: "!@#$*volName%@#$",
			volumeName:        "volName-",
			expected:          "pvc-amlfs-volName",
		},
		{
			desc:              "removes non alpha-numeric at end when using volume name",
			amlFilesystemName: "a",
			volumeName:        "vol-name-_-",
			expected:          "pvc-amlfs-vol-name",
		},
		{
			desc:              "truncates when using volume name",
			amlFilesystemName: "b",
			volumeName:        strings.Repeat("a", amlFilesystemNameMaxLength+1),
			expected:          "pvc-amlfs-" + strings.Repeat("a", amlFilesystemNameMaxLength-len("pvc-amlfs-")),
		},
		{
			desc:              "name too short",
			amlFilesystemName: "a",
			volumeName:        "volName",
			expected:          "pvc-amlfs-volName",
		},
		{
			desc:              "invalid start character",
			amlFilesystemName: "-aq",
			volumeName:        "volName",
			expected:          "pvc-amlfs-volName",
		},
		{
			desc:              "invalid end character",
			amlFilesystemName: "aq-",
			volumeName:        "volName",
			expected:          "pvc-amlfs-volName",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := getValidAmlFilesystemName(test.amlFilesystemName, test.volumeName)
			assert.Equal(t, test.expected, result, "input: %q, getValidAmlFilesystemName result: %q, expected: %q", test.amlFilesystemName, result, test.expected)
		})
	}
}

func TestRoundToAmlfsBlockSizeForSku(t *testing.T) {
	d := NewFakeDriver()
	d.lustreSkuValues = map[string]lustreSkuValue{
		"Standard": {
			IncrementInTib: laaSOBlockSizeInBytes / util.TiB,
			MaximumInTib:   10 * laaSOBlockSizeInBytes / util.TiB,
		},
		"Premium": {
			IncrementInTib: 2 * laaSOBlockSizeInBytes / util.TiB,
			MaximumInTib:   20 * laaSOBlockSizeInBytes / util.TiB,
		},
	}

	tests := []struct {
		desc            string
		capacityInBytes int64
		sku             string
		expected        int64
		expectError     bool
	}{
		{
			desc:            "default size",
			capacityInBytes: 0,
			sku:             "",
			expected:        defaultSizeInBytes,
			expectError:     false,
		},
		{
			desc:            "round up to Standard block size",
			capacityInBytes: laaSOBlockSizeInBytes - 1,
			sku:             "Standard",
			expected:        laaSOBlockSizeInBytes,
			expectError:     false,
		},
		{
			desc:            "exact Standard block size",
			capacityInBytes: laaSOBlockSizeInBytes,
			sku:             "Standard",
			expected:        laaSOBlockSizeInBytes,
			expectError:     false,
		},
		{
			desc:            "round up to next Standard block size",
			capacityInBytes: laaSOBlockSizeInBytes + 1,
			sku:             "Standard",
			expected:        2 * laaSOBlockSizeInBytes,
			expectError:     false,
		},
		{
			desc:            "round up to Premium block size",
			capacityInBytes: 2*laaSOBlockSizeInBytes - 1,
			sku:             "Premium",
			expected:        2 * laaSOBlockSizeInBytes,
			expectError:     false,
		},
		{
			desc:            "exceeds maximum capacity for Standard",
			capacityInBytes: 11 * laaSOBlockSizeInBytes,
			sku:             "Standard",
			expected:        0,
			expectError:     true,
		},
		{
			desc:            "exceeds maximum capacity for Premium",
			capacityInBytes: 21 * laaSOBlockSizeInBytes,
			sku:             "Premium",
			expected:        0,
			expectError:     true,
		},
		{
			desc:            "capacity overflow",
			capacityInBytes: math.MaxInt64,
			sku:             "Premium",
			expected:        0,
			expectError:     true,
		},
		{
			desc:            "invalid SKU",
			capacityInBytes: laaSOBlockSizeInBytes,
			sku:             "InvalidSKU",
			expected:        0,
			expectError:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result, err := d.roundToAmlfsBlockSizeForSku(test.capacityInBytes, test.sku)
			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expected, result)
			}
		})
	}
}
