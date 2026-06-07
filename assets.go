package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"bytes"

)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(thumbnailFileName string, mediaType string) string {
	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", thumbnailFileName, ext)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func (cfg apiConfig) getVidoAspectRatio(filePath string) (string, error){
	cmd := exec.Command("ffprobe","-v", "error", "-print_format", "json", "-show_streams",filePath)
	var b []byte
	cmd.Stdout = bytes.NewBuffer(b)
	return "",nil
}
func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

