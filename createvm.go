package main

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
	"truenas-cloudinit/vmconfig"

	"charm.land/huh/v2/spinner"
	truenas "github.com/deevus/truenas-go"
	"github.com/deevus/truenas-go/client"
	"github.com/google/uuid"
)

func createVM(wsClient client.Client) {
	cfg, err := configureVM(wsClient)
	if err != nil {
		log.Fatal(err)
	}

	id := uuid.New()

	err = spinner.New().
		Title("Fetching images...").
		ActionWithErr(func(ctx context.Context) error {
			return uploadImages(ctx, cfg, id, wsClient)
		}).
		Run()
	if err != nil {
		log.Fatal(err)
	}

	err = spinner.New().
		Title("Creating VM...").
		ActionWithErr(func(ctx context.Context) error {
			return createVMWithConfig(ctx, wsClient, cfg, id)
		}).
		Run()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("VM %s created successfully", cfg.VM.Name)

}

func createVMWithConfig(ctx context.Context, c client.Client, cfg *vmconfig.CreateVMConfig, id uuid.UUID) error {
	s := truenas.NewVMService(c, c.Version())

	diskPath := path.Join("/dev/zvol/",
		strings.TrimPrefix(cfg.VM.DiskDir, "/mnt/"),
		fmt.Sprintf("%s-%s", cfg.VM.Name, id.String()[24:]))
	exists, err := c.FileExists(ctx, diskPath)
	if err != nil {
		return err
	}

	if exists {
		return fmt.Errorf("disk already exists at %s", diskPath)
	}

	v := truenas.NewDatasetService(c, c.Version())
	_, err = v.CreateZvol(ctx, truenas.CreateZvolOpts{
		Name:    strings.TrimPrefix(diskPath, "/dev/zvol/"),
		Volsize: int64(cfg.VM.DiskSize) * 1024 * 1024 * 1024,
		Sparse:  true,
	})
	if err != nil {
		return err
	}

	ok, err := s.ConvertDevice(ctx, truenas.ConvertDeviceOpts{
		Source:      path.Join(settings.ISODir, path.Base(vmconfig.Images[cfg.VM.ImageId].URL)),
		Destination: diskPath,
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("device conversion failed")
	}

	var minMem *int64
	if cfg.VM.MinMemory != 0 {
		minMem = truenas.Int64Ptr(int64(cfg.VM.MinMemory))
	}

	vm, err := s.CreateVM(ctx, truenas.CreateVMOpts{
		Name: cfg.VM.Name,

		VCPUs:   int64(cfg.VM.VCPUs),
		Cores:   int64(cfg.VM.Cores),
		Threads: int64(cfg.VM.Threads),

		Memory:    int64(cfg.VM.Memory),
		MinMemory: minMem,

		Bootloader:          "UEFI",
		CPUMode:             "HOST-PASSTHROUGH",
		Time:                "UTC",
		ShutdownTimeout:     90,
		EnsureDisplayDevice: truenas.BoolPtr(false),
		// cpu_model	null
	})
	if err != nil {
		return err
	}

	_, err = s.CreateDevice(ctx, truenas.CreateVMDeviceOpts{
		VM:         vm.ID,
		DeviceType: truenas.DeviceTypeDisk,
		Disk: &truenas.CreateDiskDevice{
			Type: "VIRTIO",
			Path: diskPath,
		}})
	if err != nil {
		return err
	}

	_, err = s.CreateDevice(ctx, truenas.CreateVMDeviceOpts{
		VM:         vm.ID,
		DeviceType: truenas.DeviceTypeCDROM,
		CDROM: &truenas.CDROMDevice{
			Path: path.Join(settings.ISODir, cfg.VM.Name+".iso"),
		},
	})
	if err != nil {
		return err
	}

	_, err = s.CreateDevice(ctx, truenas.CreateVMDeviceOpts{
		VM:         vm.ID,
		DeviceType: truenas.DeviceTypeNIC,
		NIC: &truenas.NICDevice{
			Type:                "VIRTIO",
			NICAttach:           cfg.VM.NIC,
			MAC:                 cfg.VM.MACAddress,
			TrustGuestRxFilters: true,
		},
	})

	return nil
}
