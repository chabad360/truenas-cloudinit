package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"truenas-cloudinit/vmconfig"

	"github.com/deevus/truenas-go"
	"github.com/deevus/truenas-go/client"
	"github.com/google/uuid"
	"github.com/kdomanski/iso9660"
	cloudinit "go.pilab.hu/cloud/cloud-init"
	"gopkg.in/yaml.v3"
)

func uploadImages(ctx context.Context, cfg *vmconfig.CreateVMConfig, id uuid.UUID, c client.Client) error {
	iso, err := createCloudInitIso(&cfg.CloudInit, id)
	if err != nil {
		return err
	}

	err = c.PutFile(ctx, path.Join(settings.ISODir, fmt.Sprintf("%s-%s.iso", cfg.VM.Name, id.String()[24:])), truenas.PutFileOptions{}, func(w io.Writer) error {
		_, err := iso.WriteTo(w)
		return err
	})
	if err != nil {
		return err
	}

	img := vmconfig.Images[cfg.VM.ImageId].URL
	exists, err := c.FileExists(ctx, path.Join(settings.ISODir, path.Base(img)))
	if err != nil {
		return err
	}

	if !exists {

		err = c.PutFile(ctx, path.Join(settings.ISODir, path.Base(img)), truenas.PutFileOptions{}, func(w io.Writer) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, img, nil)
			if err != nil {
				return err
			}
			r, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer r.Body.Close()

			if r.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code: %d", r.StatusCode)
			}

			_, err = io.Copy(w, r.Body)
			return err
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func createCloudInitIso(config *vmconfig.CloudConfig, id uuid.UUID) (*bytes.Buffer, error) {
	iso, err := iso9660.NewWriter()
	if err != nil {
		return nil, err
	}

	userData, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}

	userData = append([]byte("#cloud-config\n"), userData...)

	err = iso.AddFile(bytes.NewBuffer(userData), "user-data")
	if err != nil {
		return nil, err
	}

	metaData := cloudinit.Metadata{
		InstanceID: id.String(),
	}
	metaDataBytes, err := yaml.Marshal(metaData)
	if err != nil {
		return nil, err
	}

	err = iso.AddFile(bytes.NewBuffer(metaDataBytes), "meta-data")
	if err != nil {
		return nil, err
	}

	var isoBuffer bytes.Buffer
	err = iso.WriteTo(&isoBuffer, "CIDATA")
	if err != nil {
		return nil, err
	}

	return &isoBuffer, nil
}
