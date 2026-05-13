package main

type Image struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

var images = []Image{
	{
		URL:  "https://cloud.debian.org/images/cloud/trixie/20260512-2476/debian-13-generic-amd64-20260512-2476.qcow2",
		Size: 412_000_000,
		Name: "Debian 13 (Trixie)",
	},
}
