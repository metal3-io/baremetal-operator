package v1alpha1

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		newBMH    *BareMetalHost
		oldBMH    *BareMetalHost
		wantedErr string
	}{
		{
			name:      "valid",
			newBMH:    &BareMetalHost{TypeMeta: tm, ObjectMeta: om, Spec: BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name:      "invalidName",
			newBMH:    &BareMetalHost{TypeMeta: tm, ObjectMeta: inom, Spec: BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "BareMetalHost resource name cannot contain characters other than [A-Za-z0-9._-]",
		},
		{
			name:      "invalidName2",
			newBMH:    &BareMetalHost{TypeMeta: tm, ObjectMeta: inom2, Spec: BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "BareMetalHost resource name cannot be a UUID",
		},
		{
			name: "invalidRAID",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BootMACAddress: "01:02:03:04:05:06",
					BMC: BMCDetails{
						Address:         "irmc:127.0.1.1",
						CredentialsName: "test1",
					},
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								NumberOfPhysicalDisks: nil,
							},
						},
						SoftwareRAIDVolumes: []SoftwareRAIDVolume{
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
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BootMACAddress: "01:02:03:04:05:06",
					BMC: BMCDetails{
						Address:         "irmc:127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "unsupportBMCType",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "test:127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "Unknown BMC type 'test' for address test:127.0.1.1",
		},
		{
			name: "RAIDWithSupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
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
					BMC: BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "RAIDWithUnsupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								NumberOfPhysicalDisks: nil,
							},
						},
					},
					BMC: BMCDetails{
						Address:         "ipmi://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver ipmi does not support configuring RAID",
		},
		{
			name: "FirmwareWithSupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					Firmware: &FirmwareConfig{
						VirtualizationEnabled: &enable,
					},
					BootMACAddress: "01:02:03:04:05:06",
					BMC: BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "FirmwareWithUnsupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					Firmware: &FirmwareConfig{
						VirtualizationEnabled: &enable,
					},
					BMC: BMCDetails{
						Address:         "ipmi://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "firmware settings for ipmi are not supported",
		},
		{
			name: "BootMACAddressRequiredWithoutBootMACAddress",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "libvirt://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver libvirt requires a BootMACAddress value",
		},
		{
			name: "BootMACAddressRequiredWithBootMACAddress",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
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
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "libvirt://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00:00",
					BootMode:       UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver libvirt does not support secure boot",
		},
		{
			name: "InvalidBootMACAddress",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00",
					BootMode:       UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "address 00:00:00:00:00: invalid MAC address",
		},
		{
			name: "UEFISecureBootWithSupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00:00",
					BootMode:       UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "'physicalDisks' in HardwareRAID without 'controller'.",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "redfish://127.0.0.1",
						CredentialsName: "test1",
					},
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
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
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "redfish://127.0.0.1",
						CredentialsName: "test1",
					},
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
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
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "ipmi://host-0.example.com.org:6223"}}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "validDNSName2",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "ipmi://baremetalhost"}}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "validDNSName3",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "ipmi://[fe80::fc33:62ff:fe83:8a76]:6233"}}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "invalidDNSNameinvalidhyphenuse",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "ipmi://-host.example.com.org"}}},
			oldBMH:    nil,
			wantedErr: "failed to parse BMC address information: BMC address hostname/IP : [-host.example.com.org] is invalid",
		},
		{
			name: "invalidDNSNameinvalidcharacter",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "ipmi://host+1.example.com.org"}}},
			oldBMH:    nil,
			wantedErr: "failed to parse BMC address information: BMC address hostname/IP : [host+1.example.com.org] is invalid",
		},
		{
			name: "invalidDNSNameinvalidformat",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "[@]host.example.com"}}},
			oldBMH:    nil,
			wantedErr: "failed to parse BMC address information: parse \"ipmi://[@]host.example.com\": net/url: invalid userinfo",
		},
		{
			name: "invalidDNSNameinvalidbmc",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "ipm:host.example.com:6223"}}},
			oldBMH:    nil,
			wantedErr: "Unknown BMC type 'ipm' for address ipm:host.example.com:6223",
		},
		{
			name: "invalidDNSNameinvalidipv6",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "ipmi://[fe80::fc33:62ff:fe33:8xff]:6223"}}},
			oldBMH:    nil,
			wantedErr: "failed to parse BMC address information: BMC address hostname/IP : [fe80::fc33:62ff:fe33:8xff] is invalid",
		},
		{
			name: "validRootDeviceHint",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					RootDeviceHints: &RootDeviceHints{
						DeviceName: "/dev/sda",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "validRootDeviceHintByPath",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					RootDeviceHints: &RootDeviceHints{
						DeviceName: "/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "invalidRootDeviceHintByUUID",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					RootDeviceHints: &RootDeviceHints{
						DeviceName: "/dev/disk/by-uuid/cdaacd50-3a4c-421c-91c0-fe9ba7b8b2f1",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "device name of root device hint must be path in /dev/ or /dev/disk/by-path/, not \"/dev/disk/by-uuid/cdaacd50-3a4c-421c-91c0-fe9ba7b8b2f1\"",
		},
		{
			name: "invalidRootDeviceHintNoPath",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					RootDeviceHints: &RootDeviceHints{
						DeviceName: "sda",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "device name of root device hint must be a /dev/ path, not \"sda\"",
		},
		{
			name: "invalidImageURL",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "redfish://127.0.0.1",
						CredentialsName: "test1",
					},
					Image: &Image{
						URL: "test1",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "image URL test1 is invalid: parse \"test1\": invalid URI for request",
		},
		{
			name: "validStatusAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						StatusAnnotation: `{"operationalStatus": "OK"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "invalidFieldStatusAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						StatusAnnotation: `{"InvalidField":"NotOK"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "error decoding status annotation: json: unknown field \"InvalidField\"",
		},
		{
			name: "invalidOpstatusStatusAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						StatusAnnotation: `{"operationalStatus":"NotOK"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid operationalStatus 'NotOK' in the baremetalhost.metal3.io/status annotation",
		},
		{
			name: "invalidErrtypeStatusAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						StatusAnnotation: `{"errorType":"No Error"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid errorType 'No Error' in the baremetalhost.metal3.io/status annotation",
		},
		{
			name: "invalidFormatStatusAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						StatusAnnotation: `{"operationalStatus":"OK"`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "error decoding status annotation: unexpected EOF",
		},
		{
			name: "invalidValueRebootAnnotationPrefix",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						RebootAnnotationPrefix: `{"mode":"medium"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid mode in the reboot.metal3.io annotation, allowed are \"hard\", \"soft\" or \"\"",
		},
		{
			name: "invalidValueRebootAnnotationWithKey",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						RebootAnnotationPrefix + "/my-key": `{"mode":"medium"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid mode in the reboot.metal3.io annotation, allowed are \"hard\", \"soft\" or \"\"",
		},
		{
			name: "inspectionNotDisabledHardwareDetailsAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						HardwareDetailsAnnotation: `{"systemVendor":{"manufacturer":"QEMU","productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6","ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true}],"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,"flags":["foo"],"count":4},"hostname":"hwdAnnotation-0"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "when hardware details are provided, the inspect.metal3.io annotation must be set to disabled",
		},
		{
			name: "invalidFieldHardwareDetailsAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						InspectAnnotationPrefix:   "disabled",
						HardwareDetailsAnnotation: `{"INVALIDField":{"manufacturer":"QEMU","productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6","ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true}],"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,"flags":["foo"],"count":4},"hostname":"hwdAnnotation-0"}`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "error decoding the inspect.metal3.io/hardwaredetails annotation: json: unknown field \"INVALIDField\"",
		},
		{
			name: "invalidJsonHardwareDetailsAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm, ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						InspectAnnotationPrefix:   "disabled",
						HardwareDetailsAnnotation: `{"INVALIDField":{"manufacturer":"QEMU","productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6","ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true}],"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,"flags":["foo"],"count":4},"hostname":"hwdAnnotation-0"`,
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "error decoding the inspect.metal3.io/hardwaredetails annotation: unexpected EOF",
		},
		{
			name: "invalidValueInspectAnnotation",
			newBMH: &BareMetalHost{
				TypeMeta: tm,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						InspectAnnotationPrefix: "enabled",
					},
				},
			},
			oldBMH:    nil,
			wantedErr: "invalid value for the inspect.metal3.io annotation, allowed are \"disabled\" or \"\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.newBMH.validateHost(); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("BareMetalHost.ValidateBareMetalHost() error = %v, wantErr %v", err, tt.wantedErr)
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
		newBMH    *BareMetalHost
		oldBMH    *BareMetalHost
		wantedErr string
	}{
		{
			name: "updateAddress",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "test-address-changed"}}},
			oldBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "test-address"}}},
			wantedErr: "BMC address can not be changed if the BMH is not in the Registering state, or if the BMH is not detached",
		},
		{
			name: "updateAddressBMHRegistering",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "test-address-changed"}},
				Status: BareMetalHostStatus{
					Provisioning: ProvisionStatus{
						State: StateRegistering}}},
			oldBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "test-address"}}},
			wantedErr: "",
		},
		{
			name: "updateAddressBMHDetached",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "test-address-changed"}},
				Status: BareMetalHostStatus{
					OperationalStatus: OperationalStatusDetached}},
			oldBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "test-address"}}},
			wantedErr: "",
		},
		{
			name: "updateBootMAC",
			newBMH: &BareMetalHost{
				TypeMeta: tm, ObjectMeta: om, Spec: BareMetalHostSpec{BootMACAddress: "test-mac-changed"}},
			oldBMH: &BareMetalHost{
				TypeMeta: tm, ObjectMeta: om, Spec: BareMetalHostSpec{BootMACAddress: "test-mac"}},
			wantedErr: "bootMACAddress can not be changed once it is set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.newBMH.validateChanges(tt.oldBMH); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("BareMetalHost.ValidateBareMetalHost() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
