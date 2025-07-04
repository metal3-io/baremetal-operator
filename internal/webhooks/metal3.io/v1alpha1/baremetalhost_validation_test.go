/*
Copyright 2025 The Metal3 Authors.

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

package webhooks

import (
	"fmt"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func errorArrContains(out []error, want string) bool {
	if out == nil {
		return want == ""
	}
	if want == "" {
		return false
	}
	for _, err := range out {
		if err.Error() == want {
			return true
		}
	}
	return false
}

func TestValidateCreate(t *testing.T) {
	tm := metav1.TypeMeta{
		Kind:       "BareMetalHost",
		APIVersion: "metal3.io/v1alpha1",
	}

	om := metav1.ObjectMeta{
		Name:      "test",
		Namespace: "test-namespace",
	}

	inom := metav1.ObjectMeta{
		Name:      "test~",
		Namespace: "test-namespace",
	}

	inom2 := metav1.ObjectMeta{
		Name:      "07564256-96ae-4315-ab03-8d34ece60fbb",
		Namespace: "test-namespace",
	}

	enable := true

	// for RAID validation test cases
	numberOfPhysicalDisks := 3

	tests := []struct {
		name      string
		newBMH    *metal3api.BareMetalHost
		oldBMH    *metal3api.BareMetalHost
		wantedErr string
	}{
		{
			name:      "valid",
			newBMH:    &metal3api.BareMetalHost{TypeMeta: tm, ObjectMeta: om, Spec: metal3api.BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name:      "validExternallyProvisioned",
			newBMH:    &metal3api.BareMetalHost{TypeMeta: tm, ObjectMeta: om, Spec: metal3api.BareMetalHostSpec{ExternallyProvisioned: true}},
			wantedErr: "",
		},
		{
			name:      "invalidName",
			newBMH:    &metal3api.BareMetalHost{TypeMeta: tm, ObjectMeta: inom, Spec: metal3api.BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "BareMetalHost resource name cannot contain characters other than [A-Za-z0-9._-]",
		},
		{
			name:      "invalidName2",
			newBMH:    &metal3api.BareMetalHost{TypeMeta: tm, ObjectMeta: inom2, Spec: metal3api.BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "BareMetalHost resource name cannot be a UUID",
		},
		{
			name: "invalidRAID",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BootMACAddress: "01:02:03:04:05:06",
					BMC: metal3api.BMCDetails{
						Address:         "irmc:127.0.1.1",
						CredentialsName: "test1",
					},
					RAID: &metal3api.RAIDConfig{
						HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								NumberOfPhysicalDisks: nil,
							},
						},
						SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
							{
								SizeGibibytes: nil,
								Level:         "",
								PhysicalDisks: nil,
							},
						},
					}}},
			oldBMH:    nil,
			wantedErr: "hardwareRAIDVolumes and softwareRAIDVolumes can not be set at the same time",
		},
		{
			name: "supportBMCType",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BootMACAddress: "01:02:03:04:05:06",
					BMC: metal3api.BMCDetails{
						Address:         "irmc:127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "unsupportBMCType",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "test:127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "Unknown BMC type 'test' for address test:127.0.1.1",
		},
		{
			name: "RAIDWithSupportBMC",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					RAID: &metal3api.RAIDConfig{
						HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								NumberOfPhysicalDisks: nil,
							},
						},
					},
					BootMACAddress: "01:02:03:04:05:06",
					BMC: metal3api.BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "RAIDWithUnsupportBMC",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					RAID: &metal3api.RAIDConfig{
						HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								NumberOfPhysicalDisks: nil,
							},
						},
					},
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver ipmi does not support configuring RAID",
		},
		{
			name: "FirmwareWithSupportBMC",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					Firmware: &metal3api.FirmwareConfig{
						VirtualizationEnabled: &enable,
					},
					BootMACAddress: "01:02:03:04:05:06",
					BMC: metal3api.BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "FirmwareWithUnsupportBMC",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					Firmware: &metal3api.FirmwareConfig{
						VirtualizationEnabled: &enable,
					},
					BMC: metal3api.BMCDetails{
						Address:         "ipmi://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "firmware settings for ipmi are not supported",
		},
		{
			name: "BootMACAddressRequiredWithoutBootMACAddress",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "libvirt://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver libvirt requires a BootMACAddress value",
		},
		{
			name: "BootMACAddressRequiredWithBootMACAddress",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "libvirt://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00:00",
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "BootMACAddressRequired",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "libvirt://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00:00",
					BootMode:       metal3api.UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver libvirt does not support secure boot",
		},
		{
			name: "InvalidBootMACAddress",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00",
					BootMode:       metal3api.UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "address 00:00:00:00:00: invalid MAC address",
		},
		{
			name: "UEFISecureBootWithSupportBMC",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00:00",
					BootMode:       metal3api.UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "'physicalDisks' in HardwareRAID without 'controller'.",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "redfish://127.0.0.1",
						CredentialsName: "test1",
					},
					RAID: &metal3api.RAIDConfig{
						HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
							{
								SizeGibibytes: nil,
								Level:         "",
								Name:          "",
								Rotational:    nil,
								PhysicalDisks: []string{"Disk-1", "Disk-2"},
							}, // end of RAID volume
						}, // end of RAID volume slice
					}, // end of RAID config
				}, // end of BMH spec
			},
			oldBMH:    nil,
			wantedErr: "'physicalDisks' specified without 'controller' in hardware RAID volume 0",
		},
		{
			name: "'numberOfPhysicalDisks' not same as length of 'physicalDisks'",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address:         "redfish://127.0.0.1",
						CredentialsName: "test1",
					},
					RAID: &metal3api.RAIDConfig{
						HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								Controller:            "Controller-1",
								PhysicalDisks:         []string{"Disk-1", "Disk-2"},
								NumberOfPhysicalDisks: &numberOfPhysicalDisks, // defined as 3 above
							}, // end of RAID volume
						}, // end of RAID volume slice
					}, // end of RAID config
				}, // end of BMH spec
			},
			oldBMH:    nil,
			wantedErr: fmt.Sprintf("the 'numberOfPhysicalDisks'[%d] and number of 'physicalDisks'[2] is not same for volume 0", numberOfPhysicalDisks),
		},
		{
			name: "validDNSName",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "ipmi://host-0.example.com.org:6223"}}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "validDNSName2",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "ipmi://baremetalhost"}}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "validDNSName3",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "ipmi://[fe80::fc33:62ff:fe83:8a76]:6233"}}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "invalidDNSNameinvalidhyphenuse",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "ipmi://-host.example.com.org"}}},
			oldBMH:    nil,
			wantedErr: "failed to parse BMC address information: BMC address hostname/IP : [-host.example.com.org] is invalid",
		},
		{
			name: "invalidDNSNameinvalidcharacter",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "ipmi://host+1.example.com.org"}}},
			oldBMH:    nil,
			wantedErr: "failed to parse BMC address information: BMC address hostname/IP : [host+1.example.com.org] is invalid",
		},
		{
			name: "invalidDNSNameinvalidformat",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "[@]host.example.com"}}},
			oldBMH:    nil,
			wantedErr: "failed to parse BMC address information: parse \"ipmi://[@]host.example.com\": net/url: invalid userinfo",
		},
		{
			name: "invalidDNSNameinvalidbmc",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "ipm:host.example.com:6223"}}},
			oldBMH:    nil,
			wantedErr: "Unknown BMC type 'ipm' for address ipm:host.example.com:6223",
		},
		{
			name: "invalidDNSNameinvalidipv6",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "ipmi://[fe80::fc33:62ff:fe33:8xff]:6223"}}},
			oldBMH:    nil,
			wantedErr: "failed to parse BMC address information: BMC address hostname/IP : [fe80::fc33:62ff:fe33:8xff] is invalid",
		},
		{
			name: "validRootDeviceHint",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					RootDeviceHints: &metal3api.RootDeviceHints{
						DeviceName: "/dev/sda",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "validRootDeviceHintByPath",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					RootDeviceHints: &metal3api.RootDeviceHints{
						DeviceName: "/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "invalidRootDeviceHintByUUID",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					RootDeviceHints: &metal3api.RootDeviceHints{
						DeviceName: "/dev/disk/by-uuid/cdaacd50-3a4c-421c-91c0-fe9ba7b8b2f1",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "device name of root device hint must be path in /dev/ or /dev/disk/by-path/, not \"/dev/disk/by-uuid/cdaacd50-3a4c-421c-91c0-fe9ba7b8b2f1\"",
		},
		{
			name: "invalidRootDeviceHintNoPath",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					RootDeviceHints: &metal3api.RootDeviceHints{
						DeviceName: "sda",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "device name of root device hint must be a /dev/ path, not \"sda\"",
		},
		{
			name: "validImageURL",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL:      "https://example.com/image",
						Checksum: "be254ebfd73e66ca91f6d91f5050aa2ee1ec4813ee65ba472f608ed340cbff09",
					},
				},
			},
			oldBMH: nil,
		},
		{
			name: "validImageLiveISO",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL:        "https://example.com/image",
						DiskFormat: ptr.To("live-iso"),
					},
				},
			},
			oldBMH: nil,
		},
		{
			name: "invalidImageURL",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL:      "test1",
						Checksum: "be254ebfd73e66ca91f6d91f5050aa2ee1ec4813ee65ba472f608ed340cbff09",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "image URL test1 is invalid: parse \"test1\": invalid URI for request",
		},
		{
			name: "emptyImageURL",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						Checksum: "be254ebfd73e66ca91f6d91f5050aa2ee1ec4813ee65ba472f608ed340cbff09",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "image URL  is invalid: parse \"\": empty url",
		},
		{
			name: "imageNoChecksum",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "https://example.com/image",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "checksum is required for normal images",
		},
		{
			name: "imageInvalidChecksum",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL:          "https://example.com/image",
						Checksum:     "be254ebfd73e66ca91f6d91f5050aa2ee1ec4813ee65ba472f608ed340cbff09",
						ChecksumType: "SHA42",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "unknown checksumType SHA42, supported are auto, md5, sha256, sha512",
		},
		{
			name: "validStatusAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.StatusAnnotation: `{"operationalStatus": "OK"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "invalidFieldStatusAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.StatusAnnotation: `{"InvalidField":"NotOK"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "error decoding status annotation: json: unknown field \"InvalidField\"",
		},
		{
			name: "invalidOpstatusStatusAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.StatusAnnotation: `{"operationalStatus":"NotOK"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid operationalStatus 'NotOK' in the baremetalhost.metal3.io/status annotation",
		},
		{
			name: "invalidErrtypeStatusAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.StatusAnnotation: `{"errorType":"No Error"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid errorType 'No Error' in the baremetalhost.metal3.io/status annotation",
		},
		{
			name: "invalidFormatStatusAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.StatusAnnotation: `{"operationalStatus":"OK"`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "error decoding status annotation: unexpected EOF",
		},
		{
			name: "invalidValueRebootAnnotationPrefix",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.RebootAnnotationPrefix: `{"mode":"medium"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid mode in the reboot.metal3.io annotation, allowed are \"hard\", \"soft\" or \"\"",
		},
		{
			name: "invalidValueRebootAnnotationWithKey",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.RebootAnnotationPrefix + "/my-key": `{"mode":"medium"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid mode in the reboot.metal3.io annotation, allowed are \"hard\", \"soft\" or \"\"",
		},
		{
			name: "inspectionNotDisabledHardwareDetailsAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.HardwareDetailsAnnotation: `{"systemVendor":{"manufacturer":"QEMU","productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6","ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true}],"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,"flags":["foo"],"count":4},"hostname":"hwdAnnotation-0"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "when hardware details are provided, the inspect.metal3.io annotation must be set to disabled",
		},
		{
			name: "invalidFieldHardwareDetailsAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.InspectAnnotationPrefix:   "disabled",
						metal3api.HardwareDetailsAnnotation: `{"INVALIDField":{"manufacturer":"QEMU","productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6","ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true}],"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,"flags":["foo"],"count":4},"hostname":"hwdAnnotation-0"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "error decoding the inspect.metal3.io/hardwaredetails annotation: json: unknown field \"INVALIDField\"",
		},
		{
			name: "invalidJsonHardwareDetailsAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm, ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.InspectAnnotationPrefix:   "disabled",
						metal3api.HardwareDetailsAnnotation: `{"INVALIDField":{"manufacturer":"QEMU","productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6","ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true}],"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,"flags":["foo"],"count":4},"hostname":"hwdAnnotation-0"`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "error decoding the inspect.metal3.io/hardwaredetails annotation: unexpected EOF",
		},
		{
			name: "invalidValueInspectAnnotation",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						metal3api.InspectAnnotationPrefix: "enabled",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid value for the inspect.metal3.io annotation, allowed are \"disabled\" or \"\"",
		},
		{
			name: "crossNamespaceUserData",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					UserData: &corev1.SecretReference{
						Name:      "test-secret",
						Namespace: "different-namespace", // Different from host's namespace
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "baremetalhosts.metal3.io \"test\" is forbidden: userData: cross-namespace Secret references are not allowed",
		},
		{
			name: "crossNamespaceNetworkData",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					NetworkData: &corev1.SecretReference{
						Name:      "test-secret",
						Namespace: "different-namespace", // Different from host's namespace
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "baremetalhosts.metal3.io \"test\" is forbidden: networkData: cross-namespace Secret references are not allowed",
		},
		{
			name: "crossNamespaceMetaData",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					MetaData: &corev1.SecretReference{
						Name:      "test-secret",
						Namespace: "different-namespace", // Different from host's namespace
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "baremetalhosts.metal3.io \"test\" is forbidden: metaData: cross-namespace Secret references are not allowed",
		},
		{
			name: "multipleSecretsCrossNamespace",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					UserData: &corev1.SecretReference{
						Name:      "test-secret1",
						Namespace: "different-namespace1",
					},
					NetworkData: &corev1.SecretReference{
						Name:      "test-secret2",
						Namespace: "different-namespace2",
					},
					MetaData: &corev1.SecretReference{
						Name:      "test-secret3",
						Namespace: "different-namespace3",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "baremetalhosts.metal3.io \"test\" is forbidden: userData: cross-namespace Secret references are not allowed", // Should catch at least one error
		},
		{
			name: "sameNamespaceSecrets",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om, // namespace is "test-namespace"
				Spec: metal3api.BareMetalHostSpec{
					UserData: &corev1.SecretReference{
						Name:      "test-secret1",
						Namespace: "test-namespace", // Same as host's namespace
					},
					NetworkData: &corev1.SecretReference{
						Name:      "test-secret2",
						Namespace: "test-namespace", // Same as host's namespace
					},
					MetaData: &corev1.SecretReference{
						Name:      "test-secret3",
						Namespace: "test-namespace", // Same as host's namespace
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "", // Should be valid
		},
		{
			name: "disablePowerOff",
			newBMH: &metal3api.BareMetalHost{
				Spec: metal3api.BareMetalHostSpec{
					DisablePowerOff: true,
					Online:          true,
				},
			},
			wantedErr: "",
		},
		{
			name: "disablePowerOffErr",
			newBMH: &metal3api.BareMetalHost{
				Spec: metal3api.BareMetalHostSpec{
					DisablePowerOff: true,
					Online:          false,
				},
			},
			wantedErr: "node can't simultaneously have online set to false and have power off disabled",
		},
	}

	for _, tt := range tests {
		webhook := &BareMetalHost{}
		t.Run(tt.name, func(t *testing.T) {
			if err := webhook.validateHost(tt.newBMH); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("metal3api.BareMetalHost.Validatemetal3api.BareMetalHost() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	tm := metav1.TypeMeta{
		Kind:       "BareMetalHost",
		APIVersion: "metal3.io/v1alpha1",
	}

	om := metav1.ObjectMeta{
		Name:      "test",
		Namespace: "test-namespace",
	}

	tests := []struct {
		name      string
		newBMH    *metal3api.BareMetalHost
		oldBMH    *metal3api.BareMetalHost
		wantedErr string
	}{
		{
			name: "updateAddress",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "test-address-changed"}}},
			oldBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "test-address"}}},
			wantedErr: "BMC address can not be changed if the BMH is not in the Registering state, or if the BMH is not detached",
		},
		{
			name: "updateAddressBMHRegistering",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "test-address-changed"}},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						State: metal3api.StateRegistering}}},
			oldBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "test-address"}}},
			wantedErr: "",
		},
		{
			name: "updateAddressBMHDetached",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "test-address-changed"}},
				Status: metal3api.BareMetalHostStatus{
					OperationalStatus: metal3api.OperationalStatusDetached}},
			oldBMH: &metal3api.BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BareMetalHostSpec{
					BMC: metal3api.BMCDetails{
						Address: "test-address"}}},
			wantedErr: "",
		},
		{
			name: "updateBootMAC",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm, ObjectMeta: om, Spec: metal3api.BareMetalHostSpec{BootMACAddress: "test-mac-changed"}},
			oldBMH: &metal3api.BareMetalHost{
				TypeMeta: tm, ObjectMeta: om, Spec: metal3api.BareMetalHostSpec{BootMACAddress: "test-mac"}},
			wantedErr: "bootMACAddress can not be changed once it is set",
		},
		{
			name: "updateExternallyProvisioned",
			newBMH: &metal3api.BareMetalHost{
				TypeMeta: tm, ObjectMeta: om, Spec: metal3api.BareMetalHostSpec{}},
			oldBMH: &metal3api.BareMetalHost{
				TypeMeta: tm, ObjectMeta: om, Spec: metal3api.BareMetalHostSpec{ExternallyProvisioned: true}},
			wantedErr: "externallyProvisioned can not be changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &BareMetalHost{}
			if err := webhook.validateChanges(tt.oldBMH, tt.newBMH); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("metal3api.BareMetalHost.Validatemetal3api.BareMetalHost() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
