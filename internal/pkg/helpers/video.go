package helpers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

func ConvertToH264(input []byte) ([]byte, error) {
	tmpDir := "/dddance-back/tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		tmpDir = "."
	}

	tmpIn, err := os.CreateTemp(tmpDir, "dance-input-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer os.Remove(tmpIn.Name())
	defer tmpIn.Close()

	if _, err := tmpIn.Write(input); err != nil {
		return nil, fmt.Errorf("failed to write temp input file: %w", err)
	}
	tmpIn.Close()

	tmpOut, err := os.CreateTemp(tmpDir, "dance-output-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp output file: %w", err)
	}
	defer os.Remove(tmpOut.Name())
	tmpOut.Close()

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", tmpIn.Name(),
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-r", "30",
		"-vsync", "cfr",
		"-c:a", "aac",
		"-movflags", "+faststart",
		"-f", "mp4",
		tmpOut.Name(),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, stderr: %s", err, stderr.String())
	}

	result, err := os.ReadFile(tmpOut.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	return result, nil
}

func TrimAndConvertVideo(input []byte, startSec, endSec float64) ([]byte, error) {
	tmpDir := "/dddance-back/tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		tmpDir = "."
	}

	tmpIn, err := os.CreateTemp(tmpDir, "dance-input-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp input: %w", err)
	}
	defer os.Remove(tmpIn.Name())
	defer tmpIn.Close()

	if _, err := tmpIn.Write(input); err != nil {
		return nil, fmt.Errorf("failed to write input: %w", err)
	}
	tmpIn.Close()

	tmpOut, err := os.CreateTemp(tmpDir, "dance-output-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp output: %w", err)
	}
	defer os.Remove(tmpOut.Name())
	tmpOut.Close()

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", tmpIn.Name(),
		"-ss", fmt.Sprintf("%.3f", startSec),
		"-to", fmt.Sprintf("%.3f", endSec),
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-r", "30",
		"-vsync", "cfr",
		"-c:a", "aac",
		"-movflags", "+faststart",
		"-f", "mp4",
		tmpOut.Name(),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, stderr: %s", err, stderr.String())
	}

	return os.ReadFile(tmpOut.Name())
}
