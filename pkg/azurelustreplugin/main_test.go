/*
Copyright 2025 The Kubernetes Authors.

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

package main

import (
	"flag"
	"testing"

	"k8s.io/klog/v2"
)

func TestInitKlogFlags(t *testing.T) {
	klog.InitFlags(nil)

	err := initKlogFlags()
	if err != nil {
		t.Fatalf("initKlogFlags() returned unexpected error: %v", err)
	}

	// Verify logtostderr is set to true
	f := flag.Lookup("logtostderr")
	if f == nil {
		t.Fatal("logtostderr flag not found")
	}
	if f.Value.String() != "true" {
		t.Errorf("logtostderr = %q, want %q", f.Value.String(), "true")
	}

	// Verify legacy_stderr_threshold_behavior is set to false
	f = flag.Lookup("legacy_stderr_threshold_behavior")
	if f == nil {
		t.Fatal("legacy_stderr_threshold_behavior flag not found")
	}
	if f.Value.String() != "false" {
		t.Errorf("legacy_stderr_threshold_behavior = %q, want %q", f.Value.String(), "false")
	}

	// Verify stderrthreshold is set to INFO (severity 0)
	f = flag.Lookup("stderrthreshold")
	if f == nil {
		t.Fatal("stderrthreshold flag not found")
	}
	// klog's severityValue.String() returns the numeric value;
	// INFO = severity 0
	if f.Value.String() != "0" {
		t.Errorf("stderrthreshold = %q, want %q (INFO)", f.Value.String(), "0")
	}
}
