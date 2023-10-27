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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume"
	mount "k8s.io/mount-utils"
	volumehelper "sigs.k8s.io/azurelustre-csi-driver/pkg/util"
	"sigs.k8s.io/cloud-provider-azure/pkg/metrics"
)

// NodePublishVolume mount the volume from staging to target path
func (d *Driver) NodePublishVolume(
	_ context.Context,
	req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	mc := metrics.NewMetricContext(azureLustreCSIDriverName,
		"node_publish_volume",
		d.resourceGroup,
		d.cloud.SubscriptionID,
		d.Name)

	userMountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"Volume ID missing in request")
	}

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"Target path not provided")
	}

	context := req.GetVolumeContext()
	if context == nil {
		return nil, status.Error(codes.InvalidArgument,
			"Volume context must be provided")
	}

	vol, err := getVolume(volumeID, context)
	if err != nil {
		return nil, err
	}

	lockKey := fmt.Sprintf("%s-%s", volumeID, target)
	if acquired := d.volumeLocks.TryAcquire(lockKey); !acquired {
		return nil, status.Errorf(codes.Aborted,
			volumeOperationAlreadyExistsFmt,
			volumeID)
	}
	defer d.volumeLocks.Release(lockKey)

	isOperationSucceeded := false
	defer func() {
		mc.ObserveOperationWithResult(isOperationSucceeded)
	}()

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	mountOptions := []string{"bind"}
	_, readOnly := getMountOptions(req, userMountFlags)
	if readOnly {
		mountOptions = append(mountOptions, "ro")
	}

	if len(vol.subDir) > 0 && !d.enableAzureLustreMockMount {
		interpolatedSubDir := interpolateSubDirVariables(context, vol)

		if isSubpath := ensureStrictSubpath(interpolatedSubDir); !isSubpath {
			return nil, status.Error(
				codes.InvalidArgument,
				"Context sub-dir must be strict subpath",
			)
		}

		if readOnly {
			klog.V(2).Info("NodePublishVolume: not attempting to create sub-dir on read-only volume, assuming existing path")
		} else {
			klog.V(2).Infof(
				"NodePublishVolume: sub-dir will be created at %q",
				interpolatedSubDir,
			)

			if err = d.createSubDir(source, interpolatedSubDir); err != nil {
				return nil, err
			}
		}

		source = filepath.Join(source, interpolatedSubDir)
		klog.V(2).Infof(
			"NodePublishVolume: full mount source with sub-dir: %q",
			source,
		)
	}

	mnt, err := d.ensureMountPoint(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"Could not mount target %q: %v",
			target,
			err)
	}
	if mnt {
		klog.V(2).Infof(
			"NodePublishVolume: volume %s is already mounted on %s",
			volumeID,
			target,
		)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	klog.V(2).Infof(
		"NodePublishVolume: volume %s mounting %s at %s with mountOptions: %v",
		volumeID, source, target, mountOptions,
	)
	if d.enableAzureLustreMockMount {
		klog.Warningf(
			"NodePublishVolume: mock mount on volumeID(%s), this is only for"+
				"TESTING!!!",
			volumeID,
		)
		if err := volumehelper.MakeDir(target); err != nil {
			klog.Errorf("MakeDir failed on target: %s (%v)", target, err)
			return nil, err
		}
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = mountVolumeAtPath(d, source, target, "", []string{}, mountOptions)
	if err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return nil, status.Errorf(
				codes.Internal,
				"Could not remove mount target %q: %v",
				target,
				removeErr,
			)
		}
		return nil, status.Errorf(codes.Internal,
			"Could not mount %q at %q: %v", source, target, err)
	}

	klog.V(2).Infof(
		"NodePublishVolume: volume %s mount %s at %s successfully",
		volumeID,
		source,
		target,
	)
	isOperationSucceeded = true

	return &csi.NodePublishVolumeResponse{}, nil
}

func interpolateSubDirVariables(context map[string]string, vol *lustreVolume) string {
	subDirReplaceMap := map[string]string{}

	// get metadata values
	for k, v := range context {
		switch strings.ToLower(k) {
		case podNameKey:
			subDirReplaceMap[podNameMetadata] = v
		case podNamespaceKey:
			subDirReplaceMap[podNamespaceMetadata] = v
		case podUIDKey:
			subDirReplaceMap[podUIDMetadata] = v
		case serviceAccountNameKey:
			subDirReplaceMap[serviceAccountNameMetadata] = v
		case pvcNamespaceKey:
			subDirReplaceMap[pvcNamespaceMetadata] = v
		case pvcNameKey:
			subDirReplaceMap[pvcNameMetadata] = v
		case pvNameKey:
			subDirReplaceMap[pvNameMetadata] = v
		}
	}

	interpolatedSubDir := volumehelper.ReplaceWithMap(vol.subDir, subDirReplaceMap)
	return interpolatedSubDir
}

func getMountOptions(req *csi.NodePublishVolumeRequest, userMountFlags []string) ([]string, bool) {
	readOnly := false
	mountOptions := []string{"no_share_fsid"}
	if req.GetReadonly() {
		readOnly = true
		mountOptions = append(mountOptions, "ro")
	}
	for _, userMountFlag := range userMountFlags {
		if userMountFlag == "ro" {
			readOnly = true

			if req.GetReadonly() {
				continue
			}
		}
		mountOptions = append(mountOptions, userMountFlag)
	}
	return mountOptions, readOnly
}

func getVolume(volumeID string, context map[string]string) (*lustreVolume, error) {
	volName := ""

	volFromID, err := getLustreVolFromID(volumeID)
	if err != nil {
		klog.Warningf("error parsing volume ID '%v'", err)
	} else {
		volName = volFromID.name
	}

	vol, err := newLustreVolume(volumeID, volName, context)
	if err != nil {
		return nil, err
	}

	if volFromID != nil && *volFromID != *vol {
		klog.Warningf("volume context does not match values in volume ID for volumeID %v", volumeID)
	}

	return vol, nil
}

func mountVolumeAtPath(d *Driver, source, target, fstype string, mountFlags, mountOptions []string) error {
	d.kernelModuleLock.Lock()
	defer d.kernelModuleLock.Unlock()
	err := d.mounter.MountSensitiveWithoutSystemdWithMountFlags(
		source,
		target,
		fstype,
		mountOptions,
		nil,
		mountFlags,
	)
	return err
}

// NodeUnpublishVolume unmount the volume from the target path
func (d *Driver) NodeUnpublishVolume(
	_ context.Context,
	req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	err := d.nodeUnmountVolume(
		"NodeUnpublishVolume",
		"node_unpublish_volume",
		req.GetVolumeId(),
		req.GetTargetPath(),
	)
	if err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func unmountVolumeAtPath(d *Driver, targetPath string) error {
	shouldUnmountBadPath := false

	d.kernelModuleLock.Lock()
	defer d.kernelModuleLock.Unlock()

	parent := filepath.Dir(targetPath)
	klog.V(2).Infof("Listing dir: %s", parent)
	entries, err := os.ReadDir(parent)
	if err != nil {
		klog.Warningf("could not list directory %s, will explicitly unmount path before cleanup %s: %q", parent, targetPath, err)
		shouldUnmountBadPath = true
	}

	for _, e := range entries {
		if e.Name() == filepath.Base(targetPath) {
			_, err := e.Info()
			if err != nil {
				klog.Warningf("could not get info for entry %s, will explicitly unmount path before cleanup %s: %q", e.Name(), targetPath, err)
				shouldUnmountBadPath = true
			}
		}
	}

	if shouldUnmountBadPath {
		// In these cases, if we only ran mount.CleanupMountWithForce,
		// it would have issues trying to stat the directory before
		// cleanup, so we need to explicitly unmount the path, with
		// force if necessary. Then the directory can be cleaned up
		// by the mount.CleanupMountWithForce call.
		klog.V(4).Infof("unmounting bad mount: %s)", targetPath)
		forceUnmounter := *d.forceMounter
		if err := forceUnmounter.UnmountWithForce(targetPath, 30*time.Second); err != nil {
			klog.Warningf("couldn't unmount %s: %q", targetPath, err)
		}
	}

	err = mount.CleanupMountWithForce(targetPath, *d.forceMounter,
		true /*extensiveMountPointCheck*/, 10*time.Second)
	return err
}

func (d *Driver) nodeUnmountVolume(
	operationMetricName string,
	operationName string,
	volumeID string,
	targetPath string,
) error {
	mc := metrics.NewMetricContext(azureLustreCSIDriverName,
		operationMetricName,
		d.resourceGroup,
		d.cloud.SubscriptionID,
		d.Name)

	if len(volumeID) == 0 {
		return status.Error(codes.InvalidArgument,
			"Volume ID missing in request")
	}

	if len(targetPath) == 0 {
		return status.Error(codes.InvalidArgument,
			"Target path missing in request")
	}

	lockKey := fmt.Sprintf("%s-%s", volumeID, targetPath)
	if acquired := d.volumeLocks.TryAcquire(lockKey); !acquired {
		return status.Errorf(codes.Aborted,
			volumeOperationAlreadyExistsFmt,
			volumeID)
	}
	defer d.volumeLocks.Release(lockKey)

	isOperationSucceeded := false
	defer func() {
		mc.ObserveOperationWithResult(isOperationSucceeded)
	}()

	klog.V(2).Infof("%s: unmounting volume %s on %s",
		operationName, volumeID, targetPath)
	err := unmountVolumeAtPath(d, targetPath)
	if err != nil {
		return status.Errorf(codes.Internal,
			"failed to unmount target %q: %v", targetPath, err)
	}
	klog.V(2).Infof(
		"%s: unmount volume %s on %s successfully",
		operationName,
		volumeID,
		targetPath,
	)

	isOperationSucceeded = true

	return nil
}

// NodeStageVolume mounts the volume to the staging path
func (d *Driver) NodeStageVolume(_ context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	mc := metrics.NewMetricContext(azureLustreCSIDriverName,
		"node_stage_volume",
		d.resourceGroup,
		d.cloud.SubscriptionID,
		d.Name)

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument,
			"Volume capability missing in request")
	}

	userMountFlags := volCap.GetMount().GetMountFlags()

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"Volume ID missing in request")
	}

	stagingTarget := req.GetStagingTargetPath()
	if len(stagingTarget) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"Staging target path not provided")
	}

	context := req.GetVolumeContext()
	if context == nil {
		return nil, status.Error(codes.InvalidArgument,
			"Volume context must be provided")
	}

	vol, err := getVolume(volumeID, context)
	if err != nil {
		return nil, err
	}

	lockKey := fmt.Sprintf("%s-%s", volumeID, stagingTarget)
	if acquired := d.volumeLocks.TryAcquire(lockKey); !acquired {
		return nil, status.Errorf(codes.Aborted,
			volumeOperationAlreadyExistsFmt,
			volumeID)
	}
	defer d.volumeLocks.Release(lockKey)

	isOperationSucceeded := false
	defer func() {
		mc.ObserveOperationWithResult(isOperationSucceeded)
	}()

	source := getSourceString(vol.mgsIPAddress, vol.azureLustreName)

	mountOptions := append(userMountFlags, "no_share_fsid")

	mnt, err := d.ensureMountPoint(stagingTarget)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"Could not mount staging target %q: %v",
			stagingTarget,
			err)
	}

	if mnt {
		klog.V(2).Infof(
			"NodeStageVolume: volume %s is already mounted on %s",
			volumeID,
			stagingTarget,
		)

		return &csi.NodeStageVolumeResponse{}, nil
	}

	klog.V(2).Infof(
		"NodeStageVolume: volume %s mounting %s at %s with mountOptions: %v",
		volumeID, source, stagingTarget, mountOptions,
	)

	if d.enableAzureLustreMockMount {
		klog.Warningf(
			"NodeStageVolume: mock mount on volumeID(%s), this is only for"+
				"TESTING!!!",
			volumeID,
		)

		if err := volumehelper.MakeDir(stagingTarget); err != nil {
			klog.Errorf("MakeDir failed on target: %s (%v)", stagingTarget, err)

			return nil, err
		}

		return &csi.NodeStageVolumeResponse{}, nil
	}

	err = mountVolumeAtPath(d, source, stagingTarget, "lustre", []string{"--no-mtab"}, mountOptions)
	if err != nil {
		if removeErr := os.Remove(stagingTarget); removeErr != nil {
			return nil, status.Errorf(
				codes.Internal,
				"Could not remove mount target %q: %v",
				stagingTarget,
				removeErr,
			)
		}
		return nil, status.Errorf(codes.Internal,
			"Could not mount %q at %q: %v", source, stagingTarget, err)
	}

	klog.V(2).Infof(
		"NodeStageVolume: volume %s mount %s at %s successfully",
		volumeID,
		source,
		stagingTarget,
	)

	isOperationSucceeded = true

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unmount the volume from the staging path
func (d *Driver) NodeUnstageVolume(_ context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	err := d.nodeUnmountVolume(
		"NodeUnstageVolume",
		"node_unstage_volume",
		req.GetVolumeId(),
		req.GetStagingTargetPath(),
	)
	if err != nil {
		return nil, err
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodeGetCapabilities return the capabilities of the Node plugin
func (d *Driver) NodeGetCapabilities(
	_ context.Context, _ *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: d.NSCap,
	}, nil
}

// NodeGetInfo return info of the node on which this plugin is running
func (d *Driver) NodeGetInfo(
	_ context.Context,
	_ *csi.NodeGetInfoRequest,
) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: d.NodeID,
	}, nil
}

// NodeGetVolumeStats get volume stats
func (d *Driver) NodeGetVolumeStats(
	_ context.Context,
	req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"NodeGetVolumeStats volume ID was empty")
	}
	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument,
			"NodeGetVolumeStats volume path was empty")
	}

	if _, err := os.Lstat(volumePath); err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound,
				"path %s does not exist", volumePath)
		}
		return nil, status.Errorf(codes.Internal,
			"failed to stat file %s: %v", volumePath, err)
	}

	volumeMetrics, err := volume.NewMetricsStatFS(volumePath).GetMetrics()
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to get metrics: %v", err)
	}

	available, ok := volumeMetrics.Available.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal,
			"failed to transform volume available size(%v)",
			volumeMetrics.Available)
	}
	capacity, ok := volumeMetrics.Capacity.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal,
			"failed to transform volume capacity size(%v)",
			volumeMetrics.Capacity)
	}
	used, ok := volumeMetrics.Used.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal,
			"failed to transform volume used size(%v)", volumeMetrics.Used)
	}

	inodesFree, ok := volumeMetrics.InodesFree.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal,
			"failed to transform disk inodes free(%v)",
			volumeMetrics.InodesFree)
	}
	inodes, ok := volumeMetrics.Inodes.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal,
			"failed to transform disk inodes(%v)", volumeMetrics.Inodes)
	}
	inodesUsed, ok := volumeMetrics.InodesUsed.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal,
			"failed to transform disk inodes used(%v)",
			volumeMetrics.InodesUsed)
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Available: available,
				Total:     capacity,
				Used:      used,
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Available: inodesFree,
				Total:     inodes,
				Used:      inodesUsed,
			},
		},
	}, nil
}

// ensureMountPoint: create mount point if not exists
// return <true, nil> if it's already a mounted point
// otherwise return <false, nil>
func (d *Driver) ensureMountPoint(target string) (bool, error) {
	notMnt, err := d.mounter.IsLikelyNotMountPoint(target)
	if err != nil && !os.IsNotExist(err) {
		if IsCorruptedDir(target) {
			notMnt = false
			klog.Warningf("detected corrupted mount for targetPath [%s]",
				target)
		} else {
			return !notMnt, err
		}
	}

	if !notMnt {
		// testing original mount point, make sure the mount link is valid
		_, err := os.ReadDir(target)
		if err == nil {
			klog.V(2).Infof("already mounted to target %s", target)
			return !notMnt, nil
		}
		// mount link is invalid, now unmount and remount later
		klog.Warningf("ReadDir %s failed with %v, unmount this directory",
			target, err)
		if err := d.mounter.Unmount(target); err != nil {
			klog.Errorf("Unmount directory %s failed with %v", target, err)
			return !notMnt, err
		}
		notMnt = true
		return !notMnt, err
	}
	if err := volumehelper.MakeDir(target); err != nil {
		klog.Errorf("MakeDir failed on target: %s (%v)", target, err)
		return !notMnt, err
	}
	return !notMnt, nil
}

func (d *Driver) createSubDir(mountPath, subDirPath string) error {
	internalVolumePath, err := getInternalVolumePath(mountPath, subDirPath)
	if err != nil {
		return err
	}

	klog.V(2).Infof("Making subdirectory at %q", internalVolumePath)

	if err := volumehelper.MakeDir(internalVolumePath); err != nil {
		return status.Errorf(codes.Internal, "failed to make subdirectory: %v", err.Error())
	}

	return nil
}

func getSourceString(mgsIPAddress, azureLustreName string) string {
	return fmt.Sprintf("%s@tcp:/%s", mgsIPAddress, azureLustreName)
}

func getInternalVolumePath(mountPath, subDirPath string) (string, error) {
	if isSubpath := ensureStrictSubpath(subDirPath); !isSubpath {
		return "", status.Errorf(
			codes.InvalidArgument,
			"sub-dir %q must be strict subpath",
			subDirPath,
		)
	}

	return filepath.Join(mountPath, subDirPath), nil
}

// Ensures that the given subpath, when joined with any base path, will be a path
// within the given base path, and not equal to it. This ensures that this
// subpath value can be safely created or deleted under the base path without
// affecting other data in the base path.
func ensureStrictSubpath(subPath string) bool {
	return filepath.IsLocal(subPath) && filepath.Clean(subPath) != "."
}

// Convert context parameters to a lustreVolume
func newLustreVolume(volumeID, volumeName string, params map[string]string) (*lustreVolume, error) {
	var mgsIPAddress, subDir, resourceGroupName string
	createdByDynamicProvisioning := false

	// validate parameters (case-insensitive).
	for k, v := range params {
		switch strings.ToLower(k) {
		case VolumeContextMGSIPAddress:
			mgsIPAddress = v
		case VolumeContextSubDir:
			subDir = v
			subDir = strings.Trim(subDir, "/")

			if len(subDir) == 0 {
				return nil, status.Error(
					codes.InvalidArgument,
					"Context sub-dir must not be empty or root if provided",
				)
			}
		case VolumeContextInternalDynamicallyCreated:
			if v == "t" {
				createdByDynamicProvisioning = true
			}
			if v != "" && v != "f" {
				klog.Warningf("invalid value for %s, should be 't' or 'f': %s", VolumeContextInternalDynamicallyCreated, v)
			}
		case VolumeContextResourceGroupName:
			resourceGroupName = v
		}
	}

	if len(mgsIPAddress) == 0 {
		return nil, status.Error(
			codes.InvalidArgument,
			"Context mgs-ip-address must be provided",
		)
	}

	vol := &lustreVolume{
		name:                         volumeName,
		mgsIPAddress:                 mgsIPAddress,
		azureLustreName:              DefaultLustreFsName,
		subDir:                       subDir,
		id:                           volumeID,
		createdByDynamicProvisioning: createdByDynamicProvisioning,
		resourceGroupName:            resourceGroupName,
	}

	return vol, nil
}
