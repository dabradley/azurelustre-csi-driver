/*
Copyright 2019 The Kubernetes Authors.

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
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	azure "sigs.k8s.io/cloud-provider-azure/pkg/provider"
)

// TODO_JUSJIN: update and add tests

const (
	fakeNodeID     = "fakeNodeID"
	fakeDriverName = "fake"
	vendorVersion  = "0.3.0"
)

func NewFakeDriver() *Driver {
	driverOptions := DriverOptions{
		NodeID:                       fakeNodeID,
		DriverName:                   DefaultDriverName,
		EnableAzureLustreMockMount:   false,
		EnableAzureLustreMockDynProv: true,
	}
	driver := NewDriver(&driverOptions)
	driver.Name = fakeDriverName
	driver.Version = vendorVersion
	driver.cloud = &azure.Cloud{}
	driver.cloud.SubscriptionID = "defaultFakeSubID"
	driver.location = "defaultFakeLocation"
	driver.resourceGroup = "defaultFakeResourceGroup"
	driver.dynamicProvisioner = &FakeDynamicProvisioner{}

	driver.lustreSkuValues = map[string]lustreSkuValue{
		"AMLFS-Durable-Premium-40":  {IncrementInTib: 48, MaximumInTib: 768},
		"AMLFS-Durable-Premium-125": {IncrementInTib: 16, MaximumInTib: 128},
		"AMLFS-Durable-Premium-250": {IncrementInTib: 8, MaximumInTib: 128},
		"AMLFS-Durable-Premium-500": {IncrementInTib: 4, MaximumInTib: 128},
	}

	return driver
}

type FakeDynamicProvisioner struct {
	DynamicProvisionerInterface
	Filesystems []*AmlFilesystemProperties
}

func (f *FakeDynamicProvisioner) CreateAmlFilesystem(_ context.Context, amlFilesystemProperties *AmlFilesystemProperties) (string, error) {
	if amlFilesystemProperties.AmlFilesystemName == "testShouldFail" {
		return "", fmt.Errorf("testShouldFail")
	}
	f.Filesystems = append(f.Filesystems, amlFilesystemProperties)
	return "127.0.0.2", nil
}

func (f *FakeDynamicProvisioner) DeleteAmlFilesystem(_ context.Context, _, amlFilesystemName string) error {
	if amlFilesystemName == "testShouldFail" {
		return fmt.Errorf("testShouldFail")
	}
	var filesystems []*AmlFilesystemProperties
	for _, filesystem := range f.Filesystems {
		if filesystem.AmlFilesystemName != amlFilesystemName {
			filesystems = append(filesystems, filesystem)
		}
	}
	f.Filesystems = filesystems
	return nil
}

func TestNewDriver(t *testing.T) {
	driverOptions := DriverOptions{
		NodeID:                       fakeNodeID,
		DriverName:                   DefaultDriverName,
		EnableAzureLustreMockMount:   false,
		EnableAzureLustreMockDynProv: true,
	}
	d := NewDriver(&driverOptions)
	assert.NotNil(t, d)
}

func TestIsCorruptedDir(t *testing.T) {
	existingMountPath, err := os.MkdirTemp(os.TempDir(), "azurelustre-csi-mount-test")
	if err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(existingMountPath)

	tests := []struct {
		desc           string
		dir            string
		expectedResult bool
	}{
		{
			desc:           "NotExist dir",
			dir:            "/tmp/NotExist",
			expectedResult: false,
		},
		{
			desc:           "Existing dir",
			dir:            existingMountPath,
			expectedResult: false,
		},
	}

	for i, test := range tests {
		isCorruptedDir := IsCorruptedDir(test.dir)
		assert.Equal(t, test.expectedResult, isCorruptedDir, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGetLustreVolFromID(t *testing.T) {
	cases := []struct {
		desc                 string
		volumeID             string
		expectedLustreVolume *lustreVolume
		expectedErr          error
	}{
		{
			desc:     "correct old volume id",
			volumeID: "vol_1#lustrefs#1.1.1.1",
			expectedLustreVolume: &lustreVolume{
				id:                "vol_1#lustrefs#1.1.1.1",
				name:              "vol_1",
				azureLustreName:   "lustrefs",
				mgsIPAddress:      "1.1.1.1",
				subDir:            "",
				amlFilesystemName: "",
			},
		},
		{
			desc:     "correct simple volume id",
			volumeID: "vol_1#lustrefs#1.1.1.1###",
			expectedLustreVolume: &lustreVolume{
				id:                "vol_1#lustrefs#1.1.1.1###",
				name:              "vol_1",
				azureLustreName:   "lustrefs",
				mgsIPAddress:      "1.1.1.1",
				subDir:            "",
				amlFilesystemName: "",
			},
		},
		{
			desc:     "correct volume id",
			volumeID: "vol_1#lustrefs#1.1.1.1#testSubDir#testAmlFs#testAmlfsRg",
			expectedLustreVolume: &lustreVolume{
				id:                "vol_1#lustrefs#1.1.1.1#testSubDir#testAmlFs#testAmlfsRg",
				name:              "vol_1",
				azureLustreName:   "lustrefs",
				mgsIPAddress:      "1.1.1.1",
				subDir:            "testSubDir",
				amlFilesystemName: "testAmlFs",
				resourceGroupName: "testAmlfsRg",
			},
		},
		{
			desc:     "correct volume id with extra slashes",
			volumeID: "vol_1#lustrefs/#1.1.1.1#/testSubDir/",
			expectedLustreVolume: &lustreVolume{
				id:              "vol_1#lustrefs/#1.1.1.1#/testSubDir/",
				name:            "vol_1",
				azureLustreName: "lustrefs",
				mgsIPAddress:    "1.1.1.1",
				subDir:          "testSubDir",
			},
		},
		{
			desc:     "correct volume id with empty sub-dir",
			volumeID: "vol_1#lustrefs/#1.1.1.1##",
			expectedLustreVolume: &lustreVolume{
				id:                "vol_1#lustrefs/#1.1.1.1##",
				name:              "vol_1",
				azureLustreName:   "lustrefs",
				mgsIPAddress:      "1.1.1.1",
				subDir:            "",
				amlFilesystemName: "",
			},
		},
		{
			desc:     "correct volume id with empty sub-dir, old format",
			volumeID: "vol_1#lustrefs/#1.1.1.1#",
			expectedLustreVolume: &lustreVolume{
				id:              "vol_1#lustrefs/#1.1.1.1#",
				name:            "vol_1",
				azureLustreName: "lustrefs",
				mgsIPAddress:    "1.1.1.1",
				subDir:          "",
			},
		},
		{
			desc:     "correct volume id with filesystem name but empty sub-dir",
			volumeID: "vol_1#lustrefs/#1.1.1.1##testAmlFs#testAmlfsRg",
			expectedLustreVolume: &lustreVolume{
				id:                "vol_1#lustrefs/#1.1.1.1##testAmlFs#testAmlfsRg",
				name:              "vol_1",
				azureLustreName:   "lustrefs",
				mgsIPAddress:      "1.1.1.1",
				subDir:            "",
				amlFilesystemName: "testAmlFs",
				resourceGroupName: "testAmlfsRg",
			},
		},
		{
			desc:     "correct volume id with multiple sub-dir levels",
			volumeID: "vol_1#lustrefs#1.1.1.1#testSubDir/nestedSubDir",
			expectedLustreVolume: &lustreVolume{
				id:              "vol_1#lustrefs#1.1.1.1#testSubDir/nestedSubDir",
				name:            "vol_1",
				azureLustreName: "lustrefs",
				mgsIPAddress:    "1.1.1.1",
				subDir:          "testSubDir/nestedSubDir",
			},
		},
		{
			desc:                 "incorrect volume id",
			volumeID:             "vol_1",
			expectedLustreVolume: nil,
			expectedErr:          errors.New("could not split volume ID \"vol_1\" into lustre name and ip address"),
		},
	}
	for _, test := range cases {
		t.Run(test.desc, func(t *testing.T) {
			lustreVolume, err := getLustreVolFromID(test.volumeID)

			if !reflect.DeepEqual(err, test.expectedErr) {
				t.Errorf("Desc: %v, Expected error: %v, Actual error: %v", test.desc, test.expectedErr, err)
			}
			assert.Equal(t, test.expectedLustreVolume, lustreVolume, "Desc: %s - Incorrect lustre volume: %v - Expected: %v", test.desc, lustreVolume, test.expectedLustreVolume)
		})
	}
}

func TestGetSubnetResourceID(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "NetworkResourceSubscriptionID is Empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "fakeSubID"
				d.cloud.NetworkResourceSubscriptionID = ""
				d.cloud.ResourceGroup = "foo"
				d.cloud.VnetResourceGroup = "foo"
				actualOutput := d.getSubnetResourceID("", "", "")
				expectedOutput := fmt.Sprintf(subnetTemplate, d.cloud.SubscriptionID, "foo", d.cloud.VnetName, d.cloud.SubnetName)
				assert.Equal(t, expectedOutput, actualOutput, "cloud.SubscriptionID should be used as the SubID")
			},
		},
		{
			name: "NetworkResourceSubscriptionID is not Empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "fakeSubID"
				d.cloud.NetworkResourceSubscriptionID = "fakeNetSubID"
				d.cloud.ResourceGroup = "foo"
				d.cloud.VnetResourceGroup = "foo"
				actualOutput := d.getSubnetResourceID("", "", "")
				expectedOutput := fmt.Sprintf(subnetTemplate, d.cloud.NetworkResourceSubscriptionID, "foo", d.cloud.VnetName, d.cloud.SubnetName)
				assert.Equal(t, expectedOutput, actualOutput, "cloud.NetworkResourceSubscriptionID should be used as the SubID")
			},
		},
		{
			name: "VnetResourceGroup is Empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "bar"
				d.cloud.NetworkResourceSubscriptionID = "bar"
				d.cloud.ResourceGroup = "fakeResourceGroup"
				d.cloud.VnetResourceGroup = ""
				actualOutput := d.getSubnetResourceID("", "", "")
				expectedOutput := fmt.Sprintf(subnetTemplate, "bar", d.cloud.ResourceGroup, d.cloud.VnetName, d.cloud.SubnetName)
				assert.Equal(t, expectedOutput, actualOutput, "cloud.Resourcegroup should be used as the rg")
			},
		},
		{
			name: "VnetResourceGroup is not Empty",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "bar"
				d.cloud.NetworkResourceSubscriptionID = "bar"
				d.cloud.ResourceGroup = "fakeResourceGroup"
				d.cloud.VnetResourceGroup = "fakeVnetResourceGroup"
				actualOutput := d.getSubnetResourceID("", "", "")
				expectedOutput := fmt.Sprintf(subnetTemplate, "bar", d.cloud.VnetResourceGroup, d.cloud.VnetName, d.cloud.SubnetName)
				assert.Equal(t, expectedOutput, actualOutput, "cloud.VnetResourceGroup should be used as the rg")
			},
		},
		{
			name: "VnetResourceGroup, vnetName, subnetName is specified",
			testFunc: func(t *testing.T) {
				d := NewFakeDriver()
				d.cloud = &azure.Cloud{}
				d.cloud.SubscriptionID = "bar"
				d.cloud.NetworkResourceSubscriptionID = "bar"
				d.cloud.ResourceGroup = "fakeResourceGroup"
				d.cloud.VnetResourceGroup = "fakeVnetResourceGroup"
				actualOutput := d.getSubnetResourceID("vnetrg", "vnetName", "subnetName")
				expectedOutput := fmt.Sprintf(subnetTemplate, "bar", "vnetrg", "vnetName", "subnetName")
				assert.Equal(t, expectedOutput, actualOutput, "VnetResourceGroup, vnetName, subnetName is specified")
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}
