package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func createBundle(id string, redact bool) (string, error) {
	return createBundleWithOptions(id, BundleOptions{Redact: redact})
}

func createBundleWithOptions(id string, options BundleOptions) (string, error) {
	config, err := loadConfig()
	if err != nil {
		return "", err
	}
	if len(options.Include) > 0 {
		config.Bundle.Include = options.Include
	}
	if len(options.Exclude) > 0 {
		config.Bundle.Exclude = append(config.Bundle.Exclude, options.Exclude...)
	}
	return createBundleWithConfig(id, options.Redact, config)
}

func createBundleWithConfig(id string, redact bool, config CapsuleConfig) (string, error) {
	src := capsuleSnapshotDir(id)
	if _, err := os.Stat(src); err != nil {
		return "", err
	}
	destDir := filepath.Join(capsuleDir, "bundles")
	if err := ensureDirs(destDir); err != nil {
		return "", err
	}
	name := id + ".zip"
	if redact {
		name = id + "-redacted.zip"
	}
	dest := filepath.Join(destDir, name)
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()
	prefix := filepath.Join("capsule", id)
	session, err := loadCapsule(id)
	if err != nil {
		return "", err
	}
	var redactor Redactor
	if redact {
		redactor, err = newRedactorWithConfig(session, config)
		if err != nil {
			return "", err
		}
	}
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !pathIncluded(rel, config.Bundle.Include, config.Bundle.Exclude) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, rel))
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if redact && shouldRedactFileWithConfig(rel, config) {
			data = []byte(redactor.RedactText(string(data)))
		}
		_, err = writer.Write(data)
		return err
	})
	if err != nil {
		return "", err
	}
	return dest, nil
}

func importBundle(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	var id string
	for _, file := range reader.File {
		parts := strings.Split(filepath.ToSlash(file.Name), "/")
		if len(parts) >= 2 && parts[0] == "capsule" && parts[1] != "" {
			id = parts[1]
			break
		}
	}
	if id == "" {
		return "", errors.New("bundle does not contain capsule/<id>/ entries")
	}

	dest := capsuleSnapshotDir(id)
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("capsule snapshot %s already exists", id)
	}
	if err := ensureDirs(dest); err != nil {
		return "", err
	}
	for _, file := range reader.File {
		parts := strings.Split(filepath.ToSlash(file.Name), "/")
		if len(parts) < 3 || parts[0] != "capsule" || parts[1] != id {
			continue
		}
		rel := filepath.Clean(filepath.Join(parts[2:]...))
		if rel == "." || rel == "" || rel == string(filepath.Separator) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return "", fmt.Errorf("bundle contains invalid path %q", file.Name)
		}
		target := filepath.Join(dest, rel)
		if file.FileInfo().IsDir() {
			if err := ensureDirs(target); err != nil {
				return "", err
			}
			continue
		}
		if err := ensureDirs(filepath.Dir(target)); err != nil {
			return "", err
		}
		rc, err := file.Open()
		if err != nil {
			return "", err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return id, nil
}
