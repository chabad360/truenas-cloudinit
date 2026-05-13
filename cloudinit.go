package main

import cloudinit "go.pilab.hu/cloud/cloud-init"

func createCloudInitConfig(defaultConfig cloudinit.CloudConfig) (*cloudinit.CloudConfig, error) {
	cloudInitConfig := defaultConfig

	return &cloudInitConfig, nil
}
