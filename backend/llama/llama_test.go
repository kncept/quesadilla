package llama

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNvidiaSmiCudaVersion(t *testing.T) {
	output := `Sun Nov 15 11:16:55 2026
+-----------------------------------------------------------------------------------------+
| NVIDIA-SMI 580.196.07             Driver Version: 580.196.07     CUDA Version: 13.0     |
+-----------------------------------------+------------------------+----------------------+`

	if got, want := parseNvidiaSmiCudaVersion(output), "13.0"; got != want {
		t.Errorf("parseNvidiaSmiCudaVersion() = %q, want %q", got, want)
	}

	if got := parseNvidiaSmiCudaVersion("no NVIDIA devices"); got != "" {
		t.Errorf("parseNvidiaSmiCudaVersion() without CUDA = %q, want empty", got)
	}
}

func TestParseVulkanSummary(t *testing.T) {
	summary := `==========
VULKANINFO
==========

Devices:
========
GPU0:
	apiVersion         = 1.3.280
	deviceName         = NVIDIA GeForce RTX 4090
	deviceType         = PHYSICAL_DEVICE_TYPE_DISCRETE_GPU`

	if !parseVulkanSummary(summary) {
		t.Error("parseVulkanSummary() = false, want true")
	}

	if parseVulkanSummary("ERROR: vkCreateInstance failed") {
		t.Error("parseVulkanSummary() with no device = true, want false")
	}
}

func TestSelectLlamaAsset(t *testing.T) {
	assets := map[string]string{
		"llama-b12387-bin-ubuntu-x64.tar.gz":                  "",
		"llama-b12387-bin-ubuntu-arm64.tar.gz":                "",
		"llama-b12387-bin-ubuntu-cuda-12.8-x64.tar.gz":        "",
		"llama-b12387-bin-ubuntu-cuda-13.0-x64.tar.gz":        "",
		"llama-b12387-bin-ubuntu-cuda-13.5-x64.tar.gz":        "",
		"llama-b12387-bin-ubuntu-vulkan-x64.tar.gz":           "",
		"llama-b12387-bin-ubuntu-sycl-fp32-x64.tar.gz":        "",
		"llama-b12387-bin-macos-arm64.tar.gz":                 "",
		"llama-b12387-bin-macos-x64.tar.gz":                   "",
		"llama-b12387-bin-win-cpu-x64.zip":                    "",
		"llama-b12387-bin-win-cuda-13.5-x64.zip":              "",
		"cudart-llama-b12387-bin-ubuntu-cuda-12.8-x64.tar.gz": "",
		"cudart-llama-b12387-bin-ubuntu-cuda-13.0-x64.tar.gz": "",
		"cudart-llama-b12387-bin-ubuntu-cuda-13.5-x64.tar.gz": "",
	}

	tests := []struct {
		name         string
		goos         string
		goarch       string
		capabilities llamaCapabilities
		wantAsset    string
		wantAccel    llamaAcceleration
		wantCuda     string
		wantCudart   string
	}{
		{
			name:      "linux cpu",
			goos:      "linux",
			goarch:    "amd64",
			wantAsset: "llama-b12387-bin-ubuntu-x64.tar.gz",
			wantAccel: llamaAccelerationCPU,
		},
		{
			name:         "linux cuda 13.0 driver uses exact cuda 13.0 build",
			goos:         "linux",
			goarch:       "amd64",
			capabilities: llamaCapabilities{cudaVersion: "13.0"},
			wantAsset:    "llama-b12387-bin-ubuntu-cuda-13.0-x64.tar.gz",
			wantAccel:    llamaAccelerationCUDA,
			wantCuda:     "13.0",
			wantCudart:   "cudart-llama-b12387-bin-ubuntu-cuda-13.0-x64.tar.gz",
		},
		{
			name:         "linux cuda 13.1 driver uses cuda 13.5 build",
			goos:         "linux",
			goarch:       "amd64",
			capabilities: llamaCapabilities{cudaVersion: "13.1"},
			wantAsset:    "llama-b12387-bin-ubuntu-cuda-13.5-x64.tar.gz",
			wantAccel:    llamaAccelerationCUDA,
			wantCuda:     "13.5",
			wantCudart:   "cudart-llama-b12387-bin-ubuntu-cuda-13.5-x64.tar.gz",
		},
		{
			name:         "linux cuda 12.4 driver uses cuda 12.8 build",
			goos:         "linux",
			goarch:       "amd64",
			capabilities: llamaCapabilities{cudaVersion: "12.4"},
			wantAsset:    "llama-b12387-bin-ubuntu-cuda-12.8-x64.tar.gz",
			wantAccel:    llamaAccelerationCUDA,
			wantCuda:     "12.8",
			wantCudart:   "cudart-llama-b12387-bin-ubuntu-cuda-12.8-x64.tar.gz",
		},
		{
			name:         "linux cuda 11.8 with no compatible build falls back to cpu",
			goos:         "linux",
			goarch:       "amd64",
			capabilities: llamaCapabilities{cudaVersion: "11.8"},
			wantAsset:    "llama-b12387-bin-ubuntu-x64.tar.gz",
			wantAccel:    llamaAccelerationCPU,
		},
		{
			name:         "linux cuda 11.8 with no compatible build falls back to vulkan",
			goos:         "linux",
			goarch:       "amd64",
			capabilities: llamaCapabilities{cudaVersion: "11.8", vulkan: true},
			wantAsset:    "llama-b12387-bin-ubuntu-vulkan-x64.tar.gz",
			wantAccel:    llamaAccelerationVulkan,
		},
		{
			name:         "linux vulkan",
			goos:         "linux",
			goarch:       "amd64",
			capabilities: llamaCapabilities{vulkan: true},
			wantAsset:    "llama-b12387-bin-ubuntu-vulkan-x64.tar.gz",
			wantAccel:    llamaAccelerationVulkan,
		},
		{
			name:      "macos",
			goos:      "darwin",
			goarch:    "arm64",
			wantAsset: "llama-b12387-bin-macos-arm64.tar.gz",
			wantAccel: llamaAccelerationCPU,
		},
		{
			name:      "windows",
			goos:      "windows",
			goarch:    "amd64",
			wantAsset: "llama-b12387-bin-win-cpu-x64.zip",
			wantAccel: llamaAccelerationCPU,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			choice, err := selectLlamaAsset("b12387", test.goos, test.goarch, assets, test.capabilities)
			if err != nil {
				t.Fatalf("selectLlamaAsset() error = %v", err)
			}
			if choice.name != test.wantAsset {
				t.Errorf("selectLlamaAsset() asset = %q, want %q", choice.name, test.wantAsset)
			}
			if choice.acceleration != test.wantAccel {
				t.Errorf("selectLlamaAsset() acceleration = %v, want %v", choice.acceleration, test.wantAccel)
			}
			if choice.cudaVersion != test.wantCuda {
				t.Errorf("selectLlamaAsset() CUDA version = %q, want %q", choice.cudaVersion, test.wantCuda)
			}
			if choice.cudartName != test.wantCudart {
				t.Errorf("selectLlamaAsset() cudart asset = %q, want %q", choice.cudartName, test.wantCudart)
			}
		})
	}
}

func TestSelectLlamaAssetRejectsUnsupportedPlatform(t *testing.T) {
	_, err := selectLlamaAsset("b12387", "linux", "386", map[string]string{}, llamaCapabilities{})
	if err == nil {
		t.Error("selectLlamaAsset() error = nil, want unsupported architecture error")
	}
}

func TestSelectLlamaAssetWithoutCudartCompanion(t *testing.T) {
	assets := map[string]string{
		"llama-b12387-bin-ubuntu-cuda-13.5-x64.tar.gz": "",
	}

	choice, err := selectLlamaAsset("b12387", "linux", "amd64", assets, llamaCapabilities{cudaVersion: "13.0"})
	if err != nil {
		t.Fatalf("selectLlamaAsset() error = %v", err)
	}
	if choice.cudartName != "" {
		t.Errorf("selectLlamaAsset() cudart asset = %q, want empty", choice.cudartName)
	}
}

func TestInstallCudartLibraries(t *testing.T) {
	workDirectory := t.TempDir()
	binDir := filepath.Join(workDirectory, "llama-b12387")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(workDirectory, "cudart-test.tar.gz")
	wantFiles := map[string]string{
		"cudart-test/libcudart.so.13":   "cudart",
		"cudart-test/libcublas.so.13":   "cublas",
		"cudart-test/libcublasLt.so.13": "cublasLt",
	}
	writeTarGz(t, archivePath, wantFiles)

	if err := installCudartLibraries(archivePath, workDirectory, binDir); err != nil {
		t.Fatalf("installCudartLibraries() error = %v", err)
	}

	for name, want := range wantFiles {
		got, err := os.ReadFile(filepath.Join(binDir, filepath.Base(name)))
		if err != nil {
			t.Errorf("ReadFile(%q) error = %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("File %q contents = %q, want %q", name, got, want)
		}
	}
}

func writeTarGz(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	writtenDirectories := make(map[string]bool)
	for name, contents := range files {
		directory := filepath.ToSlash(filepath.Dir(name))
		if directory != "." {
			parts := strings.Split(directory, "/")
			current := ""
			for _, part := range parts {
				if current == "" {
					current = part
				} else {
					current += "/" + part
				}
				if writtenDirectories[current] {
					continue
				}
				header := &tar.Header{
					Name:     current + "/",
					Mode:     0o755,
					Typeflag: tar.TypeDir,
				}
				if err := tarWriter.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
				writtenDirectories[current] = true
			}
		}

		header := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(contents)),
			Typeflag: tar.TypeReg,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDetectsFilesForRename(t *testing.T) {
	for _, filename := range llamaFileThatNeedSymlinking() {
		if !shouldSymlinkFile(filename) {
			t.Errorf("Should have needed symlinking: %s", filename)
		}
	}
}

func TestRenameFiles(t *testing.T) {
	fmt.Printf("\n")
	for _, filename := range llamaFileThatNeedSymlinking() {
		newname := newFileName(filename)
		fmt.Printf("%s  ->. %s\n", filename, newname)
	}
	fmt.Printf("\n")
}

func llamaFileThatNeedSymlinking() []string {
	return []string{
		// macOS dylib files
		"libggml-base.0.25.3.dylib",
		"libggml-blas.0.25.3.dylib",
		"libggml-cpu.0.25.3.dylib",
		"libggml-metal.0.25.3.dylib",
		"libggml-rpc.0.25.3.dylib",
		"libggml.0.25.3.dylib",
		"libllama-common.0.5.0.dylib",
		"libllama.0.5.0.dylib",
		"libmtmd.0.5.0.dylib",
		// Linux so files
		"libggml-base.so.0.25.3",
		"libggml-blas.so.0.25.3",
		"libggml-cpu.so.0.25.3",
		"libggml.so.0.25.3",
		"libllama-common.so.0.5.0",
		"libllama.so.0.5.0",
		"libmtmd.so.0.5.0",
	}
}
