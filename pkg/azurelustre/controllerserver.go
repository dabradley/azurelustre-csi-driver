/*
Copyright 2017 The Kubernetes Authors.

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
	"regexp"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"sigs.k8s.io/azurelustre-csi-driver/pkg/util"
	"sigs.k8s.io/cloud-provider-azure/pkg/metrics"
)

const (
	VolumeContextMGSIPAddress         = "mgs-ip-address"
	VolumeContextFSName               = "fs-name"
	VolumeContextSubDir               = "sub-dir"
	VolumeContextLocation             = "location"
	VolumeContextAmlFilesystemName    = "amlfilesystem-name"
	VolumeContextResourceGroupName    = "resource-group-name"
	VolumeContextVnetResourceGroup    = "vnet-resource-group"
	VolumeContextVnetName             = "vnet-name"
	VolumeContextSubnetName           = "subnet-name"
	VolumeContextMaintenanceDayOfWeek = "maintenance-day-of-week"
	VolumeContextTimeOfDayUtc         = "time-of-day-utc"
	VolumeContextSkuName              = "sku-name"
	VolumeContextZones                = "zones"
	VolumeContextTags                 = "tags"
	VolumeContextIdentities           = "identities"
	VolumeContextRootSquashMode       = "root-squash-mode"
	VolumeContextRootSquashNidLists   = "root-squash-nid-lists"
	VolumeContextRootSquashUID        = "root-squash-uid"
	VolumeContextRootSquashGID        = "root-squash-gid"
	defaultSizeInBytes                = 4 * util.TiB
	laaSOBlockSizeInBytes             = 4 * util.TiB
)

var (
	timeRegexp             = regexp.MustCompile(`^([01]?[0-9]|2[0-3]):[0-5][0-9]$`)
	amlFilesystemNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,78}[a-zA-Z0-9]$`)
	rootSquashNidsReges    = regexp.MustCompile(`^[0-9.,;\[\]@tcp*-]+$`)
)

type SubnetProperties struct {
	SubnetID          string
	VnetName          string
	VnetResourceGroup string
	SubnetName        string
}

type RootSquashSettings struct {
	SquashMode       armstoragecache.AmlFilesystemSquashMode
	NoSquashNidLists string
	SquashUID        int64
	SquashGID        int64
}

type AmlFilesystemProperties struct {
	ResourceGroupName    string
	AmlFilesystemName    string
	Location             string
	Tags                 map[string]string
	Identities           []string // Can only be "UserAssigned" identity
	SubnetInfo           SubnetProperties
	MaintenanceDayOfWeek armstoragecache.MaintenanceDayOfWeekType
	TimeOfDayUTC         string
	StorageCapacityTiB   float32
	SKUName              string
	Zones                []string
	RootSquashSettings   *RootSquashSettings
	// HSM values?
	//    Container string
	//    ImportPrefix string
	//    LoggingContainer string
	// Encryption values?
	//    KeyUrl string
	//    SourceVaultId string
}

func parseAmlFilesystemProperties(properties map[string]string) (*AmlFilesystemProperties, error) {
	var amlFilesystemProperties AmlFilesystemProperties
	var amlFilesystemName string
	var errorParameters []string
	var squashMode armstoragecache.AmlFilesystemSquashMode
	var noSquashNidLists string
	var squashUID int64
	var squashGID int64

	amlFilesystemNameReplaceMap := map[string]string{}
	shouldCreateAmlfsCluster := true

	klog.V(2).Infof("properties: %#v", properties)

	for propertyName, propertyValue := range properties {
		switch strings.ToLower(propertyName) {
		case VolumeContextResourceGroupName:
			amlFilesystemProperties.ResourceGroupName = propertyValue
		case VolumeContextMGSIPAddress:
			shouldCreateAmlfsCluster = false
		case VolumeContextAmlFilesystemName:
			amlFilesystemName = propertyValue
		case VolumeContextLocation:
			amlFilesystemProperties.Location = propertyValue
		case VolumeContextVnetName:
			amlFilesystemProperties.SubnetInfo.VnetName = propertyValue
		case VolumeContextVnetResourceGroup:
			amlFilesystemProperties.SubnetInfo.VnetResourceGroup = propertyValue
		case VolumeContextSubnetName:
			amlFilesystemProperties.SubnetInfo.SubnetName = propertyValue
		case VolumeContextMaintenanceDayOfWeek:
			possibleDayValues := armstoragecache.PossibleMaintenanceDayOfWeekTypeValues()
			for _, dayOfWeekValue := range possibleDayValues {
				if string(dayOfWeekValue) == propertyValue {
					amlFilesystemProperties.MaintenanceDayOfWeek = dayOfWeekValue
					break
				}
			}
			if len(amlFilesystemProperties.MaintenanceDayOfWeek) == 0 {
				return nil, status.Errorf(
					codes.InvalidArgument,
					"CreateVolume Parameter %s must be one of: %v",
					VolumeContextMaintenanceDayOfWeek,
					possibleDayValues,
				)
			}
		case VolumeContextTimeOfDayUtc:
			if !timeRegexp.MatchString(propertyValue) {
				return nil, status.Errorf(
					codes.InvalidArgument,
					"CreateVolume Parameter %s must be in the form HH:MM, was: '%s'",
					VolumeContextTimeOfDayUtc,
					propertyValue,
				)
			}
			amlFilesystemProperties.TimeOfDayUTC = propertyValue
		case VolumeContextSkuName:
			amlFilesystemProperties.SKUName = propertyValue
		case VolumeContextZones:
			zoneList := strings.Split(propertyValue, ",")
			for _, zone := range zoneList {
				if len(zone) > 0 {
					amlFilesystemProperties.Zones = append(amlFilesystemProperties.Zones, zone)
				}
			}
		case VolumeContextTags:
			tags, err := util.ConvertTagsToMap(propertyValue)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume %v", err)
			}
			if len(tags) > 0 {
				amlFilesystemProperties.Tags = tags
			}
		case VolumeContextIdentities:
			amlFilesystemProperties.Identities = strings.Split(propertyValue, ",")
		case VolumeContextRootSquashMode:
			possibleRootSquashModeValues := armstoragecache.PossibleAmlFilesystemSquashModeValues()
			for _, rootSquashValue := range possibleRootSquashModeValues {
				if string(rootSquashValue) == propertyValue {
					squashMode = rootSquashValue
					break
				}
			}
			if len(squashMode) == 0 {
				return nil, status.Errorf(
					codes.InvalidArgument,
					"CreateVolume Parameter %s must be one of: %v",
					VolumeContextRootSquashMode,
					possibleRootSquashModeValues,
				)
			}
		case VolumeContextRootSquashNidLists:
			if !rootSquashNidsReges.MatchString(propertyValue) {
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume %v must be in the form '10.0.2.4@tcp;10.0.2.[6-8]@tcp;10.0.2.10@tcp', was: %s",
					VolumeContextRootSquashNidLists,
					propertyValue)
			}
			noSquashNidLists = propertyValue
		case VolumeContextRootSquashUID:
			value, err := parseSquashID(propertyValue)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume %v value must be number between 1 and 4294967295, was: %s",
					VolumeContextRootSquashUID,
					propertyValue)
			}
			squashUID = value
		case VolumeContextRootSquashGID:
			value, err := parseSquashID(propertyValue)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "CreateVolume %v value must be number between 1 and 4294967295, was: %s",
					VolumeContextRootSquashGID,
					propertyValue)
			}
			squashGID = value
		case pvcNamespaceKey:
			amlFilesystemNameReplaceMap[pvcNamespaceMetadata] = propertyValue
		case pvcNameKey:
			amlFilesystemNameReplaceMap[pvcNameMetadata] = propertyValue
		case pvNameKey:
			amlFilesystemNameReplaceMap[pvNameMetadata] = propertyValue
		// These will be used by the node methods
		case VolumeContextFSName:
		case VolumeContextSubDir:
			continue
		default:
			errorParameters = append(
				errorParameters,
				fmt.Sprintf("%s = %s", propertyName, propertyValue),
			)
		}
	}

	if len(errorParameters) > 0 {
		return nil, status.Error(
			codes.InvalidArgument,
			fmt.Sprintf("Invalid parameter(s) {%s} in storage class",
				strings.Join(errorParameters, ", ")),
		)
	}

	if !shouldCreateAmlfsCluster && amlFilesystemName != "" {
		return nil, status.Errorf(codes.InvalidArgument,
			"CreateVolume %s must not be provided when using a static AMLFS ('%s' present)",
			VolumeContextAmlFilesystemName,
			VolumeContextMGSIPAddress)
	}

	if shouldCreateAmlfsCluster {
		if len(amlFilesystemName) == 0 {
			return nil, status.Errorf(codes.InvalidArgument,
				"CreateVolume %s must be provided for dynamically provisioned AMLFS",
				VolumeContextAmlFilesystemName)
		}

		if len(amlFilesystemProperties.MaintenanceDayOfWeek) == 0 {
			return nil, status.Errorf(codes.InvalidArgument,
				"CreateVolume %s must be provided for dynamically provisioned AMLFS",
				VolumeContextMaintenanceDayOfWeek)
		}

		if len(amlFilesystemProperties.SKUName) == 0 {
			return nil, status.Errorf(codes.InvalidArgument,
				"CreateVolume %s must be provided for dynamically provisioned AMLFS",
				VolumeContextSkuName)
		}

		if len(amlFilesystemProperties.TimeOfDayUTC) == 0 {
			return nil, status.Errorf(codes.InvalidArgument,
				"CreateVolume %s must be provided for dynamically provisioned AMLFS",
				VolumeContextTimeOfDayUtc)
		}

		if len(amlFilesystemProperties.Zones) == 0 {
			return nil, status.Errorf(codes.InvalidArgument,
				"CreateVolume %s must be provided for dynamically provisioned AMLFS",
				VolumeContextZones)
		}

		// Add root squash settings if set
		if squashMode != "" {
			// When root squash mode is set to RootOnly or All, all other root squash settings must be set
			if squashMode != armstoragecache.AmlFilesystemSquashModeNone &&
				(noSquashNidLists == "" || squashUID == 0 || squashGID == 0) {
				return nil, status.Errorf(codes.InvalidArgument, "invalid root squash info, must have valid %s, %s, and %s when %s is set to %s or %s",
					VolumeContextRootSquashNidLists,
					VolumeContextRootSquashUID,
					VolumeContextRootSquashGID,
					VolumeContextRootSquashMode,
					armstoragecache.AmlFilesystemSquashModeRootOnly,
					armstoragecache.AmlFilesystemSquashModeAll)
			}
			amlFilesystemProperties.RootSquashSettings = &RootSquashSettings{
				SquashMode:       squashMode,
				NoSquashNidLists: noSquashNidLists,
				SquashUID:        squashUID,
				SquashGID:        squashGID,
			}
		}

		amlFilesystemName = strings.TrimSpace(util.ReplaceWithMap(amlFilesystemName, amlFilesystemNameReplaceMap))
		amlFilesystemProperties.AmlFilesystemName = amlFilesystemName
	}

	return &amlFilesystemProperties, nil
}

func parseSquashID(propertyValue string) (int64, error) {
	value, err := strconv.ParseInt(propertyValue, 10, 64)
	if err != nil {
		return 0, err
	}
	if value < 1 || value > 4294967295 {
		return 0, fmt.Errorf("value must be between 1 and 4294967295")
	}
	return value, nil
}

func validateAndPrependVolumeName(amlFilesystemName, volName string) string {
	validAmlFilesystemName := volName + "-" + amlFilesystemName
	validAmlFilesystemName = truncateAmlFilesystemName(validAmlFilesystemName)
	if !amlFilesystemNameRegex.MatchString(validAmlFilesystemName) {
		defaultName := "azurelustre-csi"
		validAmlFilesystemName = volName + "-" + defaultName
		validAmlFilesystemName = regexp.MustCompile(`[^a-zA-Z0-9-_]+`).ReplaceAllString(validAmlFilesystemName, "")
		validAmlFilesystemName = truncateAmlFilesystemName(validAmlFilesystemName)
		validAmlFilesystemName = strings.Trim(validAmlFilesystemName, "-_")
		klog.Warningf("the requested volume name (%q) is invalid, so it is regenerated as (%q)", amlFilesystemName, validAmlFilesystemName)
	}
	return validAmlFilesystemName
}

func truncateAmlFilesystemName(amlFilesystemName string) string {
	if len(amlFilesystemName) > amlFilesystemNameMaxLength {
		amlFilesystemName = amlFilesystemName[0:amlFilesystemNameMaxLength]
	}
	return amlFilesystemName
}

func validateVolumeCapabilities(capabilities []*csi.VolumeCapability) error {
	for _, capability := range capabilities {
		if capability.GetMount() == nil {
			// Lustre just support mount type. i.e. block type is unsupported.
			return status.Error(codes.InvalidArgument,
				"Doesn't support block volume.")
		}
		support := false
		for _, supportedCapability := range volumeCapabilities {
			if capability.GetAccessMode().GetMode() == supportedCapability {
				support = true
				break
			}
		}
		if !support {
			return status.Error(codes.InvalidArgument,
				"Volume doesn't support "+
					capability.GetAccessMode().GetMode().String())
		}
	}
	return nil
}

// CreateVolume provisions a volume
func (d *Driver) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
	mc := metrics.NewMetricContext(
		azureLustreCSIDriverName,
		"controller_create_volume",
		d.resourceGroup,
		d.cloud.SubscriptionID,
		d.Name,
	)

	volName := req.GetName()
	if len(volName) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"CreateVolume Name must be provided")
	}

	err := checkVolumeRequest(req)
	if err != nil {
		return nil, err
	}

	if acquired := d.volumeLocks.TryAcquire(volName); !acquired {
		return nil, status.Errorf(codes.Aborted,
			volumeOperationAlreadyExistsFmt,
			volName)
	}
	defer d.volumeLocks.Release(volName)

	isOperationSucceeded := false
	defer func() {
		mc.ObserveOperationWithResult(isOperationSucceeded)
	}()

	parameters := req.GetParameters()
	if parameters == nil {
		return nil, status.Error(codes.InvalidArgument,
			"CreateVolume Parameters must be provided")
	}

	shouldCreateAmlfsCluster := false
	mgsIPAddress := util.GetValueInMap(parameters, VolumeContextMGSIPAddress)
	if mgsIPAddress == "" {
		shouldCreateAmlfsCluster = true
	}

	// Check parameters to ensure validity of static and dynamic configs
	amlFilesystemProperties, err := parseAmlFilesystemProperties(parameters)
	if err != nil {
		return nil, err
	}

	capacityRange := req.GetCapacityRange()

	capacityInBytes := capacityRange.GetRequiredBytes()
	klog.V(2).Infof("capacityInBytes: %#v", capacityInBytes)
	if capacityInBytes == 0 {
		capacityInBytes = defaultSizeInBytes
	}
	klog.V(2).Infof("capacityInBytes: %#v", capacityInBytes)

	capacityInBytes, err = d.roundToAmlfsBlockSizeForSku(capacityInBytes, amlFilesystemProperties.SKUName)
	if err != nil {
		return nil, err
	}
	klog.V(2).Infof("capacityInBytes: %#v", capacityInBytes)

	storageCapacityTib := float32(capacityInBytes) / util.TiB
	klog.V(2).Infof("storageCapacityTib: %#v", storageCapacityTib)

	// check if capacity is within the limit
	if capacityRange.GetLimitBytes() != 0 && capacityInBytes > capacityRange.GetLimitBytes() {
		return nil, status.Errorf(codes.InvalidArgument,
			"CreateVolume required capacity %v is greater than capacity limit %v",
			capacityInBytes, capacityRange.GetLimitBytes())
	}

	if shouldCreateAmlfsCluster {
		if len(amlFilesystemProperties.Location) == 0 {
			amlFilesystemProperties.Location = d.location
		}

		if len(amlFilesystemProperties.ResourceGroupName) == 0 {
			amlFilesystemProperties.ResourceGroupName = d.resourceGroup
		}

		amlFilesystemProperties.SubnetInfo = d.populateSubnetPropertiesFromCloudConfig(amlFilesystemProperties.SubnetInfo)

		amlFilesystemProperties.StorageCapacityTiB = storageCapacityTib
		klog.V(2).Infof("storageCapacityTib: %#v", storageCapacityTib)

		amlFilesystemName := amlFilesystemProperties.AmlFilesystemName
		amlFilesystemName = validateAndPrependVolumeName(amlFilesystemName, volName)
		if !strings.Contains(amlFilesystemName, volName) {
			return nil, status.Errorf(codes.InvalidArgument,
				"CreateVolume invalid volume name %s, cannot create valid AMLFS name. Check length and characters",
				volName)
		}
		amlFilesystemProperties.AmlFilesystemName = amlFilesystemName

		klog.V(2).Infof(
			"beginning to create AMLFS cluster (%s)", amlFilesystemProperties.AmlFilesystemName,
		)

		mgsIPAddress, err = d.dynamicProvisioner.CreateAmlFilesystem(ctx, amlFilesystemProperties)
		if err != nil {
			errCode := status.Code(err)
			if errCode == codes.Unknown {
				klog.Errorf("unknown error occurred when creating AMLFS %s: %v", amlFilesystemProperties.AmlFilesystemName, err)
				return nil, status.Error(codes.Unknown, err.Error())
			}
			klog.Warningf("error when creating AMLFS %s: %v", amlFilesystemProperties.AmlFilesystemName, err)
			return nil, status.Errorf(errCode, "CreateVolume error when creating AMLFS %s: %v", amlFilesystemProperties.AmlFilesystemName, err)
		}

		util.SetKeyValueInMap(parameters, VolumeContextAmlFilesystemName, amlFilesystemProperties.AmlFilesystemName)
		util.SetKeyValueInMap(parameters, VolumeContextResourceGroupName, amlFilesystemProperties.ResourceGroupName)
		util.SetKeyValueInMap(parameters, VolumeContextMGSIPAddress, mgsIPAddress)
		util.SetKeyValueInMap(parameters, VolumeContextFSName, DefaultLustreFsName)
	}

	volumeID, err := createVolumeIDFromParams(volName, parameters)
	if err != nil {
		return nil, err
	}

	klog.V(2).Infof("created volumeID(%s) successfully", volumeID)

	isOperationSucceeded = true

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: capacityInBytes,
			VolumeContext: parameters,
		},
	}, nil
}

func (d *Driver) roundToAmlfsBlockSizeForSku(capacityInBytes int64, skuName string) (int64, error) {
	if capacityInBytes == 0 {
		capacityInBytes = defaultSizeInBytes
	}
	skuBlockSizeInBytes := int64(defaultSizeInBytes)
	maxCapacityInBytes := int64(0)
	valuesForSku, ok := d.lustreSkuValues[skuName]
	if !ok {
		if skuName != "" {
			validSkus := make([]string, 0, len(d.lustreSkuValues))
			for k := range d.lustreSkuValues {
				validSkus = append(validSkus, k)
			}
			return 0, status.Errorf(
				codes.InvalidArgument,
				"CreateVolume Parameter %s must be one of: %v",
				VolumeContextSkuName,
				validSkus,
			)
		}
	} else {
		blockSizeInTiB := valuesForSku.IncrementInTib
		skuBlockSizeInBytes = blockSizeInTiB * util.TiB
		maxCapacityInBytes = valuesForSku.MaximumInTib * util.TiB
	}
	capacityInBytes = ((capacityInBytes + skuBlockSizeInBytes - 1) /
		skuBlockSizeInBytes) * skuBlockSizeInBytes
	if maxCapacityInBytes > 0 {
		maxCapacityInBytes := valuesForSku.MaximumInTib * util.TiB
		if capacityInBytes > maxCapacityInBytes || capacityInBytes < 0 {
			return 0, status.Errorf(codes.InvalidArgument, "Requested capacity %d exceeds maximum capacity %d for SKU %s", capacityInBytes, maxCapacityInBytes, skuName)
		}
	}
	return capacityInBytes, nil
}

func checkVolumeRequest(req *csi.CreateVolumeRequest) error {
	volumeCapabilities := req.GetVolumeCapabilities()
	if len(volumeCapabilities) == 0 {
		return status.Error(
			codes.InvalidArgument,
			"CreateVolume Volume capabilities must be provided",
		)
	}
	if req.GetVolumeContentSource() != nil {
		return status.Error(
			codes.InvalidArgument,
			"CreateVolume doesn't support being created from an existing volume",
		)
	}
	if req.GetSecrets() != nil {
		return status.Error(
			codes.InvalidArgument,
			"CreateVolume doesn't support secrets",
		)
	}
	if req.GetAccessibilityRequirements() != nil {
		return status.Error(
			codes.InvalidArgument,
			"CreateVolume doesn't support accessibility_requirements",
		)
	}
	capabilityError := validateVolumeCapabilities(volumeCapabilities)
	if capabilityError != nil {
		return capabilityError
	}
	return nil
}

// DeleteVolume delete a volume
func (d *Driver) DeleteVolume(
	ctx context.Context, req *csi.DeleteVolumeRequest,
) (*csi.DeleteVolumeResponse, error) {
	mc := metrics.NewMetricContext(azureLustreCSIDriverName,
		"controller_delete_volume",
		d.resourceGroup,
		d.cloud.SubscriptionID,
		d.Name)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"Volume ID missing in request")
	}
	if req.GetSecrets() != nil {
		return nil, status.Error(
			codes.InvalidArgument,
			"CreateVolume doesn't support secrets",
		)
	}

	amlFilesystemName := ""

	lustreVolume, err := getLustreVolFromID(volumeID)
	if err != nil {
		klog.Warningf("error parsing volume ID '%v'", err)
	} else {
		amlFilesystemName = lustreVolume.amlFilesystemName
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted,
			volumeOperationAlreadyExistsFmt,
			volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	isOperationSucceeded := false
	defer func() {
		mc.ObserveOperationWithResult(isOperationSucceeded)
	}()

	klog.V(2).Infof("deleting volumeID(%s)", volumeID)

	if amlFilesystemName != "" {
		err := d.dynamicProvisioner.DeleteAmlFilesystem(ctx, lustreVolume.resourceGroupName, amlFilesystemName)
		if err != nil {
			errCode := status.Code(err)
			if errCode == codes.Unknown {
				klog.Errorf("unknown error occurred when deleting AMLFS %s in resource group %s: %v", amlFilesystemName, lustreVolume.resourceGroupName, err)
				return nil, status.Error(codes.Unknown, err.Error())
			}
			klog.Warningf("error when deleting AMLFS %s in resource group %s: %v", amlFilesystemName, lustreVolume.resourceGroupName, err)
			return nil, status.Errorf(errCode, "DeleteVolume error when deleting AMLFS %s in resource group %s: %v", amlFilesystemName, lustreVolume.resourceGroupName, err)
		}
	}

	isOperationSucceeded = true
	klog.V(2).Infof("volumeID(%s) is deleted successfully", volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities return the capabilities of the volume
func (d *Driver) ValidateVolumeCapabilities(
	_ context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest,
) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetSecrets() != nil {
		return nil, status.Error(
			codes.InvalidArgument,
			"Doesn't support secrets",
		)
	}
	// TODO_CHYIN: need to check if the volumeID is a exist volume
	//             need LaaSo's support
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"Volume ID missing in request")
	}
	capabilities := req.GetVolumeCapabilities()
	if len(capabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"Volume capabilities missing in request")
	}

	confirmed := &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
		VolumeCapabilities: capabilities,
	}
	capabilityError := validateVolumeCapabilities(capabilities)
	if capabilityError != nil {
		confirmed = nil
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: confirmed,
		Message:   "",
	}, nil
}

// ControllerGetCapabilities returns the capabilities of the Controller plugin
func (d *Driver) ControllerGetCapabilities(
	_ context.Context,
	_ *csi.ControllerGetCapabilitiesRequest,
) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: d.Cap,
	}, nil
}

// Convert VolumeCreate parameters to a volume id
func createVolumeIDFromParams(volName string, params map[string]string) (string, error) {
	var mgsIPAddress, azureLustreName, amlFilesystemName, resourceGroupName, subDir string

	// validate parameters (case-insensitive).
	for k, v := range params {
		switch strings.ToLower(k) {
		case VolumeContextMGSIPAddress:
			mgsIPAddress = v
		case VolumeContextFSName:
			azureLustreName = v
		case VolumeContextAmlFilesystemName:
			amlFilesystemName = v
		case VolumeContextResourceGroupName:
			resourceGroupName = v
		case VolumeContextSubDir:
			subDir = v
			subDir = strings.Trim(subDir, "/")

			if len(subDir) == 0 {
				return "", status.Error(
					codes.InvalidArgument,
					"CreateVolume Parameter sub-dir must not be empty if provided",
				)
			}
		}
	}

	azureLustreName = strings.Trim(azureLustreName, "/")
	if len(azureLustreName) == 0 {
		return "", status.Error(
			codes.InvalidArgument,
			"CreateVolume Parameter fs-name must be provided",
		)
	}

	volumeID := fmt.Sprintf(volumeIDTemplate, volName, azureLustreName, mgsIPAddress, subDir, amlFilesystemName, resourceGroupName)

	return volumeID, nil
}
